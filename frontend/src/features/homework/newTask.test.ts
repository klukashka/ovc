import { describe, expect, test } from 'vitest';

import {
  type QuestionCounts,
  type QuestionType,
  attachmentsFolderOf,
  countsOfQuestions,
  countsOfTypes,
  materialTitleOf,
  newQuestion,
  policyBasisOf,
  policyViewOf,
  questionProblemsOf,
  resultHintOf,
} from './newTask.ts';

function sequentialIds(): () => string {
  let next = 0;
  return () => {
    next += 1;
    return `00000000-0000-4000-8000-${String(next).padStart(12, '0')}`;
  };
}

function countsOf(types: readonly QuestionType[]): QuestionCounts {
  const newId = sequentialIds();
  return countsOfQuestions(types.map((type) => newQuestion(type, newId)));
}

const mixed = countsOf(['single_choice', 'short_text', 'long_text', 'file']);
const automaticOnly = countsOf(['single_choice', 'multiple_choice', 'short_number']);
const manualOnly = countsOf(['long_text']);
const contentOnly = countsOf([]);

describe('the answer policy of a new task', () => {
  test('defaults to never only when every question is checked automatically', () => {
    const views = [mixed, automaticOnly, manualOnly, contentOnly].map((counts) => policyViewOf(null, counts));

    expect(views.map((view) => view.policy)).toEqual(['after_check', 'never', 'after_check', 'after_check']);
    expect(views.map((view) => view.warning)).toEqual([null, null, null, null]);
  });

  test('warns when after checking is chosen for work with no question the tutor checks', () => {
    const chosen = policyViewOf('after_check', automaticOnly);

    expect(chosen).toEqual({ policy: 'after_check', warning: 'noManualQuestion' });
  });

  test('warns for after submission whatever the questions are', () => {
    const views = [mixed, automaticOnly, contentOnly].map((counts) => policyViewOf('after_submission', counts));

    expect(views.map((view) => view.warning)).toEqual(['afterSubmission', 'afterSubmission', 'afterSubmission']);
  });

  test('keeps a chosen after checking and warns once the last manual question is removed', () => {
    const before = policyViewOf('after_check', mixed);

    const after = policyViewOf('after_check', countsOf(['single_choice', 'short_text']));

    expect(before.warning).toBeNull();
    expect(after).toEqual({ policy: 'after_check', warning: 'noManualQuestion' });
  });

  test('an unknown or failed copy of the saved template keeps the default and the warning as last derived', () => {
    const lastBasis = automaticOnly;

    const unknown = policyViewOf(null, policyBasisOf(lastBasis, mixed, { kind: 'unknown' }));
    const failed = policyViewOf(null, policyBasisOf(lastBasis, mixed, { kind: 'failed' }));

    expect(unknown).toEqual({ policy: 'never', warning: null });
    expect(failed).toEqual({ policy: 'never', warning: null });
  });

  test('a refetched copy without a manual question switches an unchosen default to never and warns for a chosen after checking', () => {
    const refetched = { kind: 'known', counts: automaticOnly } as const;

    const unchosen = policyViewOf(null, policyBasisOf(mixed, mixed, refetched));
    const chosen = policyViewOf('after_check', policyBasisOf(mixed, mixed, refetched));

    expect(unchosen).toEqual({ policy: 'never', warning: null });
    expect(chosen).toEqual({ policy: 'after_check', warning: 'noManualQuestion' });
  });

  test('a saved template without questions keeps after checking and shows no warning', () => {
    const server = { kind: 'known', counts: countsOfTypes(['instruction', 'material']) } as const;

    const view = policyViewOf(null, policyBasisOf(contentOnly, contentOnly, server));

    expect(view).toEqual({ policy: 'after_check', warning: null });
  });

  test('names the result for each mix of questions', () => {
    const hints = [contentOnly, automaticOnly, manualOnly, mixed].map(resultHintOf);

    expect(hints).toEqual(['none', 'automatic', 'manual', 'mixed']);
  });
});

describe('the completeness of a new task question', () => {
  test('a new question of every type starts with its prompt missing and the defaults of the page', () => {
    const newId = sequentialIds();
    const types: readonly QuestionType[] = [
      'single_choice',
      'multiple_choice',
      'short_text',
      'short_number',
      'long_text',
      'file',
    ];

    const questions = types.map((type) => newQuestion(type, newId));

    expect(questions.map((question) => question.points)).toEqual([1, 1, 1, 1, 10, 10]);
    expect(questions.map((question) => questionProblemsOf(question).map((problem) => problem.code))).toEqual([
      ['prompt_missing', 'option_empty'],
      ['prompt_missing', 'option_empty'],
      ['prompt_missing', 'no_accepted_answer'],
      ['prompt_missing', 'value_invalid'],
      ['prompt_missing'],
      ['prompt_missing'],
    ]);
    expect(questions[1]).toMatchObject({ scoring: 'all_or_nothing' });
    expect(questions[4]).toMatchObject({ maxLength: 5_000 });
    expect(questions[5]).toMatchObject({ allowedKinds: ['images', 'pdf', 'office', 'audio'], maxFiles: 5 });
  });

  test('choice questions need filled options with the right number of correct ones', () => {
    const base = { id: 'question', prompt: 'Past of go', imageIds: [], points: 1, explanation: '' };
    const filled = [
      { id: 'first', text: 'went', correct: true },
      { id: 'second', text: 'gone', correct: true },
    ];

    const twoCorrect = questionProblemsOf({ ...base, type: 'single_choice', options: filled });
    const oneCorrect = questionProblemsOf({
      ...base,
      type: 'single_choice',
      options: filled.map((choice, index) => ({ ...choice, correct: index === 0 })),
    });
    const noneCorrect = questionProblemsOf({
      ...base,
      type: 'multiple_choice',
      scoring: 'all_or_nothing',
      options: filled.map((choice) => ({ ...choice, correct: false })),
    });

    expect(twoCorrect.map((problem) => problem.code)).toEqual(['correct_count']);
    expect(oneCorrect).toEqual([]);
    expect(noneCorrect.map((problem) => problem.code)).toEqual(['no_correct_option']);
  });

  test('number questions need decimals of at most six places and a tolerance that is not negative', () => {
    const base = { id: 'question', prompt: 'Two and a half', imageIds: [], points: 1, explanation: '' };

    const valid = questionProblemsOf({ ...base, type: 'short_number', value: '2.5', tolerance: '0.000001' });
    const invalid = questionProblemsOf({ ...base, type: 'short_number', value: '2,5', tolerance: '-1' });

    expect(valid).toEqual([]);
    expect(invalid.map((problem) => [problem.field, problem.code])).toEqual([
      ['value', 'value_invalid'],
      ['tolerance', 'tolerance_invalid'],
    ]);
  });

  test('blank lines among accepted answers are ignored', () => {
    const base = { id: 'question', prompt: 'Synonym of brief', imageIds: [], points: 1, explanation: '' };

    const blankOnly = questionProblemsOf({ ...base, type: 'short_text', acceptedAnswers: ['', '  '] });
    const withBlank = questionProblemsOf({ ...base, type: 'short_text', acceptedAnswers: ['short', ''] });

    expect(blankOnly.map((problem) => problem.code)).toEqual(['no_accepted_answer']);
    expect(withBlank).toEqual([]);
  });
});

describe('task attachments', () => {
  test('a material title is the file name without its extension, never empty', () => {
    const titles = ['chapter 7.pdf', '.pdf', 'notes', `${'a'.repeat(250)}.png`].map((name) =>
      materialTitleOf(name, 'Attachment'),
    );

    expect(titles).toEqual(['chapter 7', '.pdf', 'notes', 'a'.repeat(200)]);
  });

  test('the attachments folder is found at the root by its exact name first, then by its folded name', () => {
    const folders = [
      { id: 'nested', name: 'Task attachments', parent_id: 'parent' },
      { id: 'folded', name: 'task ATTACHMENTS', parent_id: null },
    ];

    const folded = attachmentsFolderOf(folders, 'Task attachments');
    const exact = attachmentsFolderOf(
      [...folders, { id: 'exact', name: 'Task attachments', parent_id: null }],
      'Task attachments',
    );
    const missing = attachmentsFolderOf(folders, 'Вложения заданий');

    expect(folded).toBe('folded');
    expect(exact).toBe('exact');
    expect(missing).toBeNull();
  });
});
