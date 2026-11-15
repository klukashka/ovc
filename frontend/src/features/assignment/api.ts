import { retryAfterSeconds } from '../../components/ui/Form.tsx';
import { type ApiClient, type ApiError, apiErrorFrom, asApiError } from '../../lib/api/client.ts';
import { type components, pathsApiV1AssignmentsGetParametersQueryStatusValues } from '../../lib/api/schema.ts';
import { type StudentBlock, parseStudentBlocks } from './studentView.ts';

export { assignmentListKey, assignmentsKey, submissionKey } from './keys.ts';

type StudentSubmissionResponse = components['schemas']['StudentSubmission'];
type SubmissionFileResponse = components['schemas']['SubmissionFile'];
type StudentReviewResponse = components['schemas']['StudentReview'];
type StudentRevealedResponse = components['schemas']['StudentRevealed'];

export type AssignmentItem = components['schemas']['StudentAssignmentItem'];
export type AssignmentPage = components['schemas']['StudentAssignmentPage'];
export type AssignmentStatus = (typeof pathsApiV1AssignmentsGetParametersQueryStatusValues)[number];
export type SubmissionStatus = StudentSubmissionResponse['status'];

export const assignmentStatuses: readonly AssignmentStatus[] = ['todo', 'submitted', 'checked', 'closed'];

export const assignmentPageSize = 20;

export function isAssignmentStatus(value: string | null): value is AssignmentStatus {
  return pathsApiV1AssignmentsGetParametersQueryStatusValues.some((status) => status === value);
}

export function assignmentAddress(submissionId: string): string {
  return `/homework/${encodeURIComponent(submissionId)}`;
}

export interface AnswerFile {
  readonly attachmentId: string;
  readonly filename: string;
  readonly mediaType: string;
  readonly sizeBytes: number;
  readonly url: string;
  readonly expiresAt: string;
}

export interface Answer {
  readonly optionIds: readonly string[];
  readonly text: string;
  readonly files: readonly AnswerFile[];
}

export const emptyAnswer: Answer = { optionIds: [], text: '', files: [] };

export interface AssignmentTutor {
  readonly name: string;
  readonly colour: string | null;
  readonly relationshipId: string;
}

export interface ReviewItem {
  readonly comment: string;
  readonly correctedFiles: readonly AnswerFile[];
}

export interface Review {
  readonly feedback: string;
  readonly items: ReadonlyMap<string, ReviewItem>;
}

export interface RevealedAnswer {
  readonly correctOptionIds: readonly string[];
  readonly acceptedAnswers: readonly string[];
  readonly correctValue: string | null;
  readonly tolerance: string | null;
  readonly explanation: string;
}

export interface Submission {
  readonly id: string;
  readonly title: string;
  readonly tutor: AssignmentTutor;
  readonly status: SubmissionStatus;
  readonly deadlineAt: string | null;
  readonly late: boolean;
  readonly issuedBeforeReclaim: boolean;
  readonly version: number;
  readonly editable: ReadonlySet<string>;
  readonly blocks: readonly StudentBlock[];
  readonly answers: ReadonlyMap<string, Answer>;
  readonly review: Review | null;
  readonly revealed: ReadonlyMap<string, RevealedAnswer> | null;
}

export type QuestionBlock = Exclude<StudentBlock, { readonly type: 'instruction' | 'material' }>;

export function isQuestion(block: StudentBlock): block is QuestionBlock {
  return block.type !== 'instruction' && block.type !== 'material';
}

function answerFile(entry: SubmissionFileResponse): AnswerFile {
  return {
    attachmentId: entry.attachment_id,
    filename: entry.filename,
    mediaType: entry.media_type,
    sizeBytes: entry.size_bytes,
    url: entry.url,
    expiresAt: entry.expires_at,
  };
}

function parseReview(review: StudentReviewResponse | null): Review | null {
  if (review === null) {
    return null;
  }
  return {
    feedback: review.feedback,
    items: new Map(
      review.items.map((item) => [
        item.question_id,
        { comment: item.comment, correctedFiles: item.corrected_files.map(answerFile) },
      ]),
    ),
  };
}

function parseRevealed(revealed: StudentRevealedResponse | undefined): ReadonlyMap<string, RevealedAnswer> | null {
  if (revealed === undefined) {
    return null;
  }
  return new Map(
    revealed.questions.map((question) => [
      question.question_id,
      {
        correctOptionIds: question.correct_option_ids ?? [],
        acceptedAnswers: question.accepted_answers ?? [],
        correctValue: question.correct_value ?? null,
        tolerance: question.tolerance ?? null,
        explanation: question.explanation ?? '',
      },
    ]),
  );
}

export function parseSubmission(response: StudentSubmissionResponse): Submission {
  return {
    id: response.id,
    title: response.title,
    tutor: {
      name: response.tutor.name,
      colour: response.tutor.colour,
      relationshipId: response.tutor.relationship_id,
    },
    status: response.status,
    deadlineAt: response.deadline_at,
    late: response.late,
    issuedBeforeReclaim: response.issued_before_reclaim,
    version: response.version,
    editable: new Set(response.editable_question_ids),
    blocks: parseStudentBlocks(response.blocks),
    answers: new Map(
      response.answers.map((answer) => [
        answer.question_id,
        {
          optionIds: answer.value.option_ids ?? [],
          text: answer.value.text ?? '',
          files: answer.files.map(answerFile),
        },
      ]),
    ),
    review: parseReview(response.review),
    revealed: parseRevealed(response.revealed),
  };
}

export function answered(block: QuestionBlock, answer: Answer | undefined): boolean {
  if (answer === undefined) {
    return false;
  }
  switch (block.type) {
    case 'single_choice':
    case 'multiple_choice':
      return answer.optionIds.length > 0;
    case 'file':
      return answer.files.length > 0;
    case 'short_text':
    case 'short_number':
    case 'long_text':
      return answer.text.trim() !== '';
  }
}

export function sameAnswer(block: QuestionBlock, left: Answer, right: Answer): boolean {
  switch (block.type) {
    case 'single_choice':
    case 'multiple_choice':
      return (
        left.optionIds.length === right.optionIds.length && left.optionIds.every((id) => right.optionIds.includes(id))
      );
    case 'file':
      return (
        left.files.length === right.files.length &&
        left.files.every((file, index) => file.attachmentId === right.files[index]?.attachmentId)
      );
    case 'short_text':
    case 'short_number':
    case 'long_text':
      return left.text === right.text;
  }
}

function answerValue(block: QuestionBlock, answer: Answer): components['schemas']['StudentAnswerValue'] {
  switch (block.type) {
    case 'single_choice':
    case 'multiple_choice':
      return { option_ids: [...answer.optionIds] };
    case 'file':
      return { attachment_ids: answer.files.map((file) => file.attachmentId) };
    case 'short_text':
    case 'short_number':
    case 'long_text':
      return { text: answer.text };
  }
}

export async function fetchAssignmentPage(
  client: ApiClient,
  status: AssignmentStatus,
  tutor: string | null,
  limit: number,
  cursor: string | null,
  signal?: AbortSignal,
): Promise<AssignmentPage> {
  const { data, error, response } = await client.GET('/api/v1/assignments', {
    params: {
      query: {
        status,
        limit,
        ...(tutor === null ? {} : { tutor }),
        ...(cursor === null ? {} : { cursor }),
      },
    },
    ...(signal === undefined ? {} : { signal }),
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

export async function fetchSubmission(client: ApiClient, id: string, signal: AbortSignal): Promise<Submission> {
  const { data, error, response } = await client.GET('/api/v1/submissions/{id}', {
    params: { path: { id } },
    signal,
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  if (!('editable_question_ids' in data)) {
    throw apiErrorFrom(404, undefined);
  }
  return parseSubmission(data);
}

export interface SentAnswer {
  readonly block: QuestionBlock;
  readonly answer: Answer;
}

export interface SaveRequest {
  readonly version: number;
  readonly saveId: string;
  readonly answers: readonly SentAnswer[];
}

export type SaveOutcome =
  | { readonly kind: 'saved'; readonly submission: Submission }
  | { readonly kind: 'conflict' }
  | { readonly kind: 'changed' }
  | { readonly kind: 'gone' }
  | { readonly kind: 'invalid'; readonly questions: readonly string[]; readonly error: ApiError }
  | { readonly kind: 'storageFull'; readonly error: ApiError }
  | { readonly kind: 'retry'; readonly retryAfterSeconds: number | null; readonly error: ApiError };

const answerFieldPattern = /^answers\[(\d+)\]/;

function invalidQuestions(error: ApiError, answers: readonly SentAnswer[]): readonly string[] {
  const questions: string[] = [];
  for (const problem of error.fields) {
    const match = answerFieldPattern.exec(problem.field);
    const sent = match === null ? undefined : answers[Number(match[1])];
    if (sent !== undefined && !questions.includes(sent.block.id)) {
      questions.push(sent.block.id);
    }
  }
  return questions;
}

export async function saveAnswers(client: ApiClient, id: string, request: SaveRequest): Promise<SaveOutcome> {
  try {
    const { data, error, response } = await client.PUT('/api/v1/submissions/{id}/answers', {
      params: { path: { id } },
      body: {
        version: request.version,
        save_id: request.saveId,
        answers: request.answers.map((sent) => ({
          question_id: sent.block.id,
          value: answerValue(sent.block, sent.answer),
        })),
      },
    });
    if (data !== undefined) {
      return { kind: 'saved', submission: parseSubmission(data) };
    }
    const refusal = apiErrorFrom(response.status, error);
    if (response.status === 429) {
      return { kind: 'retry', retryAfterSeconds: retryAfterSeconds(response), error: refusal };
    }
    if (refusal.code === 'conflict') {
      return { kind: 'conflict' };
    }
    if (refusal.code === 'not_editable' || refusal.code === 'answer_frozen') {
      return { kind: 'changed' };
    }
    if (response.status === 404) {
      return { kind: 'gone' };
    }
    if (response.status === 507) {
      return { kind: 'storageFull', error: refusal };
    }
    if (response.status === 422) {
      return { kind: 'invalid', questions: invalidQuestions(refusal, request.answers), error: refusal };
    }
    return { kind: 'retry', retryAfterSeconds: null, error: refusal };
  } catch (thrown: unknown) {
    return { kind: 'retry', retryAfterSeconds: null, error: asApiError(thrown) };
  }
}

export type SubmitOutcome =
  | { readonly kind: 'submitted'; readonly submission: Submission }
  | { readonly kind: 'conflict' }
  | { readonly kind: 'changed' }
  | { readonly kind: 'gone' }
  | { readonly kind: 'failed'; readonly error: ApiError; readonly retryAfterSeconds: number | null };

export async function submitWork(client: ApiClient, id: string, version: number): Promise<SubmitOutcome> {
  try {
    const { data, error, response } = await client.POST('/api/v1/submissions/{id}/submit', {
      params: { path: { id } },
      body: { version },
    });
    if (data !== undefined) {
      return { kind: 'submitted', submission: parseSubmission(data) };
    }
    const refusal = apiErrorFrom(response.status, error);
    if (refusal.code === 'conflict') {
      return { kind: 'conflict' };
    }
    if (refusal.code === 'not_editable') {
      return { kind: 'changed' };
    }
    if (response.status === 404) {
      return { kind: 'gone' };
    }
    return {
      kind: 'failed',
      error: refusal,
      retryAfterSeconds: response.status === 429 ? retryAfterSeconds(response) : null,
    };
  } catch (thrown: unknown) {
    return { kind: 'failed', error: asApiError(thrown), retryAfterSeconds: null };
  }
}
