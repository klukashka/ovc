import { retryAfterSeconds } from '../../components/ui/Form.tsx';
import { type ApiClient, type ApiError, apiErrorFrom, asApiError } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import type { QueueFilter } from './keys.ts';

type TutorSubmission = components['schemas']['TutorSubmission'];
type IssueBlock = components['schemas']['IssueBlock'];
type SubmissionFile = components['schemas']['SubmissionFile'];
type TutorReviewRevision = components['schemas']['TutorReviewRevision'];

export type QueueItem = components['schemas']['ReviewQueueItem'];
export type QueuePage = components['schemas']['ReviewQueuePage'];
export type ReviewStatus = TutorSubmission['status'];
export type AnswerPolicy = TutorSubmission['answer_policy'];

export const queuePageSize = 20;

const nextWalkPageSize = 100;

export interface ReviewFile {
  readonly attachmentId: string;
  readonly filename: string;
  readonly mediaType: string;
  readonly sizeBytes: number;
  readonly url: string;
  readonly expiresAt: string;
  readonly width: number | null;
  readonly height: number | null;
}

export interface ReviewOption {
  readonly id: string;
  readonly text: string;
  readonly correct: boolean;
}

export type QuestionType = 'single_choice' | 'multiple_choice' | 'short_text' | 'short_number' | 'long_text' | 'file';

export interface ReviewQuestion {
  readonly id: string;
  readonly type: QuestionType;
  readonly prompt: string;
  readonly points: number;
  readonly options: readonly ReviewOption[];
  readonly acceptedAnswers: readonly string[];
  readonly value: string;
  readonly tolerance: string;
  readonly images: readonly ReviewFile[];
}

export interface ReviewAnswer {
  readonly optionIds: readonly string[];
  readonly text: string;
  readonly normalised: string | null;
  readonly autoPoints: number | null;
  readonly autoCorrect: boolean | null;
  readonly files: readonly ReviewFile[];
}

export interface RevisionItem {
  readonly points: number | null;
  readonly overridePoints: number | null;
  readonly overrideReason: string | null;
  readonly comment: string;
  readonly correctedFiles: readonly ReviewFile[];
}

export interface Revision {
  readonly revision: number;
  readonly version: number;
  readonly feedback: string;
  readonly items: ReadonlyMap<string, RevisionItem>;
}

export interface ReviewSubmission {
  readonly id: string;
  readonly issueId: string;
  readonly title: string;
  readonly status: ReviewStatus;
  readonly late: boolean;
  readonly issuedBeforeReclaim: boolean;
  readonly answerPolicy: AnswerPolicy;
  readonly firstSubmittedAt: string | null;
  readonly maxPoints: number;
  readonly finalPoints: number | null;
  readonly percentage: number | null;
  readonly studentName: string;
  readonly questions: readonly ReviewQuestion[];
  readonly answers: ReadonlyMap<string, ReviewAnswer>;
  readonly draft: Revision | null;
  readonly published: Revision | null;
  readonly version: number;
}

const questionTypes: readonly QuestionType[] = [
  'single_choice',
  'multiple_choice',
  'short_text',
  'short_number',
  'long_text',
  'file',
];

function isQuestionType(type: IssueBlock['type']): type is QuestionType {
  return questionTypes.some((entry) => entry === type);
}

export function isManual(question: ReviewQuestion): boolean {
  return question.type === 'long_text' || question.type === 'file';
}

function malformed(): ApiError {
  return apiErrorFrom(0, undefined);
}

function parseFile(file: SubmissionFile): ReviewFile {
  return {
    attachmentId: file.attachment_id,
    filename: file.filename,
    mediaType: file.media_type,
    sizeBytes: file.size_bytes,
    url: file.url,
    expiresAt: file.expires_at,
    width: file.width,
    height: file.height,
  };
}

function parseQuestion(block: IssueBlock): ReviewQuestion | null {
  if (block.type === 'instruction' || block.type === 'material') {
    return null;
  }
  if (!isQuestionType(block.type) || block.points === undefined || block.prompt === undefined) {
    throw malformed();
  }
  return {
    id: block.id,
    type: block.type,
    prompt: block.prompt,
    points: block.points,
    options: (block.options ?? []).map((option) => ({
      id: option.id,
      text: option.text,
      correct: option.correct === true,
    })),
    acceptedAnswers: block.accepted_answers ?? [],
    value: block.value ?? '',
    tolerance: block.tolerance ?? '',
    images: (block.images ?? []).map(parseFile),
  };
}

function parseRevision(revision: TutorReviewRevision): Revision {
  const items = new Map<string, RevisionItem>();
  for (const item of revision.items) {
    items.set(item.question_id, {
      points: item.points,
      overridePoints: item.override_points,
      overrideReason: item.override_reason,
      comment: item.comment,
      correctedFiles: item.corrected_files.map(parseFile),
    });
  }
  return { revision: revision.revision, version: revision.version, feedback: revision.feedback, items };
}

export function parseReviewSubmission(data: TutorSubmission): ReviewSubmission {
  const questions: ReviewQuestion[] = [];
  for (const block of data.blocks) {
    const question = parseQuestion(block);
    if (question !== null) {
      questions.push(question);
    }
  }
  const answers = new Map<string, ReviewAnswer>();
  for (const answer of data.answers) {
    answers.set(answer.question_id, {
      optionIds: answer.value.option_ids ?? [],
      text: answer.value.text ?? '',
      normalised: answer.normalised,
      autoPoints: answer.auto_points,
      autoCorrect: answer.auto_correct,
      files: answer.files.map(parseFile),
    });
  }
  const draft = data.revisions.find((revision) => revision.state === 'draft');
  const published = data.revisions
    .filter((revision) => revision.state === 'published')
    .reduce<TutorReviewRevision | null>(
      (latest, revision) => (latest === null || revision.revision > latest.revision ? revision : latest),
      null,
    );
  return {
    id: data.id,
    issueId: data.issue_id,
    title: data.title,
    status: data.status,
    late: data.late,
    issuedBeforeReclaim: data.issued_before_reclaim,
    answerPolicy: data.answer_policy,
    firstSubmittedAt: data.first_submitted_at,
    maxPoints: data.max_points,
    finalPoints: data.final_points,
    percentage: data.percentage,
    studentName: data.student.display_name,
    questions,
    answers,
    draft: draft === undefined ? null : parseRevision(draft),
    published: published === null ? null : parseRevision(published),
    version: data.version,
  };
}

export function parseSubmissionDetail(data: components['schemas']['SubmissionDetail']): ReviewSubmission {
  if (!('answer_policy' in data) || !('revisions' in data) || !('max_points' in data)) {
    throw malformed();
  }
  return parseReviewSubmission(data);
}

export async function fetchReviewSubmission(
  client: ApiClient,
  id: string,
  signal: AbortSignal,
): Promise<ReviewSubmission> {
  const { data, error, response } = await client.GET('/api/v1/submissions/{id}', {
    params: { path: { id } },
    signal,
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return parseSubmissionDetail(data);
}

function filterQuery(filter: QueueFilter): { issue?: string; group?: string; student?: string } {
  return {
    ...(filter.issue === null ? {} : { issue: filter.issue }),
    ...(filter.group === null ? {} : { group: filter.group }),
    ...(filter.student === null ? {} : { student: filter.student }),
  };
}

export async function fetchQueuePage(
  client: ApiClient,
  filter: QueueFilter,
  limit: number,
  cursor: string | null,
  signal: AbortSignal,
): Promise<QueuePage> {
  const { data, error, response } = await client.GET('/api/v1/review-queue', {
    params: { query: { ...filterQuery(filter), limit, ...(cursor === null ? {} : { cursor }) } },
    signal,
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

function orderedAfter(item: QueueItem, submittedAt: string, id: string): boolean {
  const first = item.first_submitted_at ?? '';
  const instant = Date.parse(first);
  const current = Date.parse(submittedAt);
  return instant > current || (instant === current && item.submission_id > id);
}

export async function findNextSubmission(
  client: ApiClient,
  filter: QueueFilter,
  current: { readonly id: string; readonly firstSubmittedAt: string },
  signal: AbortSignal,
): Promise<string | null> {
  let cursor: string | null = null;
  for (;;) {
    const page: QueuePage = await fetchQueuePage(client, filter, nextWalkPageSize, cursor, signal);
    const next = page.items.find(
      (item) => item.submission_id !== current.id && orderedAfter(item, current.firstSubmittedAt, current.id),
    );
    if (next !== undefined) {
      return next.submission_id;
    }
    if (page.next_cursor === null) {
      return null;
    }
    cursor = page.next_cursor;
  }
}

export interface ReviewItemBody {
  readonly question_id: string;
  readonly points: number | null;
  readonly override_points: number | null;
  readonly override_reason: string | null;
  readonly comment: string;
  readonly corrected_attachment_ids: readonly string[];
}

export interface DraftBody {
  readonly feedback: string;
  readonly items: readonly ReviewItemBody[];
}

export interface SaveRequest {
  readonly revision: number;
  readonly version: number;
  readonly saveId: string;
  readonly body: DraftBody;
}

export type SaveOutcome =
  | { readonly kind: 'saved'; readonly submission: ReviewSubmission }
  | { readonly kind: 'conflict' }
  | { readonly kind: 'notEditable' }
  | { readonly kind: 'fileGone'; readonly error: ApiError }
  | { readonly kind: 'storageFull'; readonly error: ApiError }
  | { readonly kind: 'unconfirmed' }
  | { readonly kind: 'invalid'; readonly error: ApiError }
  | { readonly kind: 'refused'; readonly error: ApiError }
  | { readonly kind: 'retry'; readonly retryAfterSeconds: number | null; readonly error: ApiError };

export async function saveDraft(client: ApiClient, id: string, request: SaveRequest): Promise<SaveOutcome> {
  try {
    const { data, error, response } = await client.PUT('/api/v1/submissions/{id}/review', {
      params: { path: { id } },
      body: {
        revision: request.revision,
        version: request.version,
        save_id: request.saveId,
        feedback: request.body.feedback,
        items: request.body.items.map((item) => ({
          ...item,
          corrected_attachment_ids: [...item.corrected_attachment_ids],
        })),
      },
    });
    if (data !== undefined) {
      return { kind: 'saved', submission: parseReviewSubmission(data) };
    }
    const refusal = apiErrorFrom(response.status, error);
    if (response.status === 429) {
      return { kind: 'retry', retryAfterSeconds: retryAfterSeconds(response), error: refusal };
    }
    if (refusal.code === 'conflict') {
      return { kind: 'conflict' };
    }
    if (refusal.code === 'not_editable') {
      return { kind: 'notEditable' };
    }
    if (response.status === 404) {
      return { kind: 'fileGone', error: refusal };
    }
    if (response.status === 507) {
      return { kind: 'storageFull', error: refusal };
    }
    if (refusal.code === 'email_not_verified') {
      return { kind: 'unconfirmed' };
    }
    if (response.status === 422) {
      return { kind: 'invalid', error: refusal };
    }
    if (response.status === 0 || response.status >= 500) {
      return { kind: 'retry', retryAfterSeconds: null, error: refusal };
    }
    return { kind: 'refused', error: refusal };
  } catch (thrown: unknown) {
    return { kind: 'retry', retryAfterSeconds: null, error: asApiError(thrown) };
  }
}

export type PublishAction = 'check' | 'return';

export type PublishOutcome =
  | { readonly kind: 'published'; readonly submission: ReviewSubmission }
  | { readonly kind: 'conflict' }
  | { readonly kind: 'notEditable' }
  | { readonly kind: 'gone' }
  | { readonly kind: 'returnNotAllowed' }
  | { readonly kind: 'incomplete' }
  | { readonly kind: 'failed'; readonly error: ApiError; readonly retryAfterSeconds: number | null };

export async function publishReview(
  client: ApiClient,
  id: string,
  action: PublishAction,
  revision: number,
): Promise<PublishOutcome> {
  try {
    const options = { params: { path: { id } }, body: { revision } };
    const { data, error, response } =
      action === 'check'
        ? await client.POST('/api/v1/submissions/{id}/check', options)
        : await client.POST('/api/v1/submissions/{id}/return', options);
    if (data !== undefined) {
      return { kind: 'published', submission: parseReviewSubmission(data) };
    }
    const refusal = apiErrorFrom(response.status, error);
    switch (refusal.code) {
      case 'conflict':
        return { kind: 'conflict' };
      case 'not_editable':
        return { kind: 'notEditable' };
      case 'return_not_allowed':
        return { kind: 'returnNotAllowed' };
      default:
    }
    if (response.status === 404) {
      return { kind: 'gone' };
    }
    if (refusal.fieldCode('revision') === 'required') {
      return { kind: 'incomplete' };
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

export function reviewAddress(id: string, filter: QueueFilter): string {
  const query = new URLSearchParams();
  for (const [name, value] of Object.entries(filter)) {
    if (typeof value === 'string') {
      query.set(name, value);
    }
  }
  const search = query.toString();
  return `/homework/review/${encodeURIComponent(id)}${search === '' ? '' : `?${search}`}`;
}

export function queueAddress(filter: QueueFilter): string {
  const query = new URLSearchParams();
  for (const [name, value] of Object.entries(filter)) {
    if (typeof value === 'string') {
      query.set(name, value);
    }
  }
  const search = query.toString();
  return `/homework/review${search === '' ? '' : `?${search}`}`;
}

export function filterFrom(params: URLSearchParams): QueueFilter {
  return { issue: params.get('issue'), group: params.get('group'), student: params.get('student') };
}
