import {
  type Block,
  type ChoiceOption,
  type QuestionBlock,
  type TemplateBlockInput,
  blockInput,
  cutToCodePoints,
  fileKinds,
  limits,
  newMaterialBlock,
} from '../builder/blocks.ts';
import type { AnswerPolicy, IssueTemplate } from './api.ts';

export const questionTypes = [
  'single_choice',
  'multiple_choice',
  'short_text',
  'short_number',
  'long_text',
  'file',
] as const;

export type QuestionType = (typeof questionTypes)[number];

export const automaticTypes: readonly QuestionType[] = [
  'single_choice',
  'multiple_choice',
  'short_text',
  'short_number',
];

export const manualTypes: readonly QuestionType[] = ['long_text', 'file'];

export function isQuestionType(value: string): value is QuestionType {
  return questionTypes.some((type) => type === value);
}

export function isManualType(type: string): boolean {
  return manualTypes.some((manual) => manual === type);
}

const automaticPoints = 1;
const manualPoints = 10;
const longTextMaxLength = 5_000;
const fileMaxFiles = 5;

export const taskTitleLimit = limits.title;
export const instructionLimit = limits.text;
export const blockLimit = limits.blocks;

function option(newId: () => string): ChoiceOption {
  return { id: newId(), text: '', correct: false };
}

export function newQuestion(type: QuestionType, newId: () => string): QuestionBlock {
  const base = {
    id: newId(),
    prompt: '',
    imageIds: [],
    points: isManualType(type) ? manualPoints : automaticPoints,
    explanation: '',
  };
  switch (type) {
    case 'single_choice':
      return { ...base, type, options: [option(newId), option(newId)] };
    case 'multiple_choice':
      return { ...base, type, options: [option(newId), option(newId)], scoring: 'all_or_nothing' };
    case 'short_text':
      return { ...base, type, acceptedAnswers: [] };
    case 'short_number':
      return { ...base, type, value: '', tolerance: '0' };
    case 'long_text':
      return { ...base, type, maxLength: longTextMaxLength };
    case 'file':
      return { ...base, type, allowedKinds: [...fileKinds], maxFiles: fileMaxFiles };
  }
}

export interface QuestionCounts {
  readonly automatic: number;
  readonly manual: number;
}

export function countsOfTypes(types: readonly string[]): QuestionCounts {
  const questions = types.filter(isQuestionType);
  const manual = questions.filter(isManualType).length;
  return { automatic: questions.length - manual, manual };
}

export function countsOfQuestions(questions: readonly QuestionBlock[]): QuestionCounts {
  return countsOfTypes(questions.map((question) => question.type));
}

export function countsOfTemplate(template: IssueTemplate): QuestionCounts {
  return countsOfTypes(template.blocks.map((block) => block.type));
}

export type ResultHint = 'none' | 'automatic' | 'manual' | 'mixed';

export function resultHintOf(counts: QuestionCounts): ResultHint {
  if (counts.automatic === 0 && counts.manual === 0) {
    return 'none';
  }
  if (counts.manual === 0) {
    return 'automatic';
  }
  return counts.automatic === 0 ? 'manual' : 'mixed';
}

export function defaultPolicyOf(counts: QuestionCounts): AnswerPolicy {
  return counts.automatic > 0 && counts.manual === 0 ? 'never' : 'after_check';
}

export type RevealWarning = 'afterSubmission' | 'noManualQuestion' | null;

export function revealWarningOf(policy: AnswerPolicy, counts: QuestionCounts): RevealWarning {
  if (policy === 'after_submission') {
    return 'afterSubmission';
  }
  return policy === 'after_check' && counts.automatic > 0 && counts.manual === 0 ? 'noManualQuestion' : null;
}

export type ServerCopy =
  | { readonly kind: 'none' }
  | { readonly kind: 'unknown' }
  | { readonly kind: 'known'; readonly counts: QuestionCounts }
  | { readonly kind: 'failed' };

export function policyBasisOf(previous: QuestionCounts, client: QuestionCounts, server: ServerCopy): QuestionCounts {
  switch (server.kind) {
    case 'none':
      return client;
    case 'known':
      return server.counts;
    case 'unknown':
    case 'failed':
      return previous;
  }
}

export interface PolicyView {
  readonly policy: AnswerPolicy;
  readonly warning: RevealWarning;
}

export function policyViewOf(chosen: AnswerPolicy | null, basis: QuestionCounts): PolicyView {
  const policy = chosen ?? defaultPolicyOf(basis);
  return { policy, warning: revealWarningOf(policy, basis) };
}

export function serverCopyReady(server: ServerCopy): boolean {
  return server.kind === 'none' || server.kind === 'known';
}

export type QuestionField = 'prompt' | 'options' | 'answers' | 'value' | 'tolerance';

export type CompletenessCode =
  | 'prompt_missing'
  | 'too_few_options'
  | 'option_empty'
  | 'correct_count'
  | 'no_correct_option'
  | 'no_accepted_answer'
  | 'accepted_answer_empty'
  | 'value_invalid'
  | 'tolerance_invalid';

export interface QuestionProblem {
  readonly questionId: string;
  readonly field: QuestionField;
  readonly code: CompletenessCode;
}

const decimalPattern = /^[+-]?[0-9]+(\.[0-9]{1,6})?$/;

function blank(text: string): boolean {
  return text.trim() === '';
}

function filledAnswers(answers: readonly string[]): readonly string[] {
  return answers.filter((answer) => !blank(answer));
}

export function cleanQuestion(question: QuestionBlock): QuestionBlock {
  return question.type === 'short_text'
    ? { ...question, acceptedAnswers: filledAnswers(question.acceptedAnswers) }
    : question;
}

function choiceProblems(question: Extract<QuestionBlock, { readonly options: unknown }>): readonly QuestionProblem[] {
  const id = question.id;
  if (question.options.length < 2) {
    return [{ questionId: id, field: 'options', code: 'too_few_options' }];
  }
  if (question.options.some((choice) => blank(choice.text))) {
    return [{ questionId: id, field: 'options', code: 'option_empty' }];
  }
  const correct = question.options.filter((choice) => choice.correct).length;
  if (question.type === 'single_choice' && correct !== 1) {
    return [{ questionId: id, field: 'options', code: 'correct_count' }];
  }
  if (question.type === 'multiple_choice' && correct === 0) {
    return [{ questionId: id, field: 'options', code: 'no_correct_option' }];
  }
  return [];
}

function fieldProblems(question: QuestionBlock): readonly QuestionProblem[] {
  const id = question.id;
  switch (question.type) {
    case 'single_choice':
    case 'multiple_choice':
      return choiceProblems(question);
    case 'short_text': {
      const answers = filledAnswers(question.acceptedAnswers);
      if (answers.length === 0) {
        return [{ questionId: id, field: 'answers', code: 'no_accepted_answer' }];
      }
      return answers.some((answer) => answer.normalize('NFKC').trim() === '')
        ? [{ questionId: id, field: 'answers', code: 'accepted_answer_empty' }]
        : [];
    }
    case 'short_number':
      return [
        ...(decimalPattern.test(question.value)
          ? []
          : [{ questionId: id, field: 'value', code: 'value_invalid' } as const]),
        ...(decimalPattern.test(question.tolerance) && !question.tolerance.startsWith('-')
          ? []
          : [{ questionId: id, field: 'tolerance', code: 'tolerance_invalid' } as const]),
      ];
    case 'long_text':
    case 'file':
      return [];
  }
}

export function questionProblemsOf(question: QuestionBlock): readonly QuestionProblem[] {
  const prompt: readonly QuestionProblem[] = blank(question.prompt)
    ? [{ questionId: question.id, field: 'prompt', code: 'prompt_missing' }]
    : [];
  return [...prompt, ...fieldProblems(question)];
}

export function taskProblemsOf(questions: readonly QuestionBlock[]): readonly QuestionProblem[] {
  return questions.flatMap(questionProblemsOf);
}

const fieldOfCode: Readonly<Record<CompletenessCode, QuestionField>> = {
  prompt_missing: 'prompt',
  too_few_options: 'options',
  option_empty: 'options',
  correct_count: 'options',
  no_correct_option: 'options',
  no_accepted_answer: 'answers',
  accepted_answer_empty: 'answers',
  value_invalid: 'value',
  tolerance_invalid: 'tolerance',
};

function isCompletenessCode(code: string): code is CompletenessCode {
  return code in fieldOfCode;
}

export function serverProblemsOf(
  problems: IssueTemplate['problems'],
  questionIds: ReadonlySet<string>,
): readonly QuestionProblem[] {
  return problems.flatMap((problem) =>
    problem.block_id !== null && questionIds.has(problem.block_id) && isCompletenessCode(problem.code)
      ? [{ questionId: problem.block_id, field: fieldOfCode[problem.code], code: problem.code }]
      : [],
  );
}

export function problemKey(questionId: string, field: QuestionField): string {
  return `${questionId}.${field}`;
}

export function blockCount(materials: number, questions: number): number {
  return 1 + materials + questions;
}

export function taskBlocks(
  instructionId: string,
  instruction: string,
  materialIds: readonly string[],
  questions: readonly QuestionBlock[],
  newId: () => string,
): TemplateBlockInput[] {
  const blocks: Block[] = [
    { type: 'instruction', id: instructionId, text: instruction },
    ...materialIds.map((materialId) => newMaterialBlock(materialId, newId)),
    ...questions.map(cleanQuestion),
  ];
  return blocks.map(blockInput);
}

function withoutExtension(filename: string): string {
  const dot = filename.lastIndexOf('.');
  return dot > 0 ? filename.slice(0, dot) : filename;
}

export function materialTitleOf(filename: string, fallback: string): string {
  const stem = cutToCodePoints(withoutExtension(filename), limits.title).trim();
  if (stem !== '') {
    return stem;
  }
  const whole = cutToCodePoints(filename, limits.title).trim();
  return whole === '' ? fallback : whole;
}

function folded(name: string): string {
  return name.normalize('NFKC').toLocaleLowerCase().trim();
}

export interface NamedFolder {
  readonly id: string;
  readonly name: string;
  readonly parent_id: string | null;
}

export function attachmentsFolderOf(folders: readonly NamedFolder[], name: string): string | null {
  const roots = folders.filter((folder) => folder.parent_id === null);
  const exact = roots.find((folder) => folder.name === name);
  if (exact !== undefined) {
    return exact.id;
  }
  const wanted = folded(name);
  return roots.find((folder) => folded(folder.name) === wanted)?.id ?? null;
}
