import type { components } from './schema.ts';
import type { Me } from './session.ts';

type StudentSubmission = components['schemas']['StudentSubmission'];
type TutorSubmission = components['schemas']['TutorSubmission'];
type StudentAnswer = components['schemas']['StudentAnswer'];
type StudentAnswerValue = components['schemas']['StudentAnswerValue'];
type StudentTemplateBlock = components['schemas']['StudentTemplateBlock'];
type SubmissionFile = components['schemas']['SubmissionFile'];

export interface FieldProblem {
  readonly path: string;
  readonly code: string;
}

interface StoredSubmission {
  readonly studentId: string;
  readonly tutorId: string;
  submission: StudentSubmission;
  lastSaveId: string | null;
  firstSubmitted: boolean;
  ended: boolean;
}

export interface FeedbackSeenRequest {
  readonly id: string;
  readonly version: number;
}

export interface EndedRelationship {
  readonly studentId: string;
  readonly relationshipId: string;
}

type FeedbackSeenBody =
  | { readonly kind: 'malformed' }
  | { readonly kind: 'invalid'; readonly problems: readonly FieldProblem[] }
  | { readonly kind: 'parsed'; readonly version: number };

interface PendingAnswerUpload {
  readonly studentId: string;
  readonly submissionId: string;
  readonly questionId: string;
  readonly file: SubmissionFile;
}

interface SentAnswer {
  readonly questionId: string;
  readonly value: StudentAnswerValue;
}

interface SaveBody {
  readonly version: number;
  readonly saveId: string;
  readonly answers: readonly SentAnswer[];
}

const submissionPathPrefix = '/api/v1/submissions/';
const answerScopePrefix = 'submission:';
const editableStatuses: ReadonlySet<StudentSubmission['status']> = new Set(['not_started', 'draft', 'returned']);
const submittedStatuses: ReadonlySet<StudentSubmission['status']> = new Set(['submitted', 'returned', 'checked']);
const openStatuses: ReadonlySet<StudentSubmission['status']> = new Set([
  'not_started',
  'draft',
  'returned',
  'submitted',
]);
const automaticTypes: ReadonlySet<StudentTemplateBlock['type']> = new Set([
  'single_choice',
  'multiple_choice',
  'short_text',
  'short_number',
]);
const manualTypes: ReadonlySet<StudentTemplateBlock['type']> = new Set(['long_text', 'file']);
const shortTextLimit = 500;
const numberTextLimit = 64;
const answerUploadLimitBytes = 25_000_000;
const answerFileKinds: Readonly<Record<string, string>> = {
  'application/pdf': 'pdf',
  'image/jpeg': 'images',
  'image/png': 'images',
  'image/webp': 'images',
  'image/gif': 'images',
};
const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function answer(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

export function fakeRefusal(status: number, code: string, fields: readonly FieldProblem[] = []): Response {
  return answer(status, {
    error: {
      code,
      message: code,
      request_id: 'fake-request',
      ...(fields.length === 0 ? {} : { fields: fields.map((field) => ({ path: field.path, code: field.code })) }),
    },
  });
}

function stringList(value: unknown): string[] | null {
  if (!Array.isArray(value)) {
    return null;
  }
  const entries: unknown[] = value;
  return entries.every((entry) => typeof entry === 'string')
    ? entries.filter((entry) => typeof entry === 'string')
    : null;
}

function sentValue(value: unknown): StudentAnswerValue | null {
  if (typeof value !== 'object' || value === null) {
    return null;
  }
  const parsed: { option_ids?: string[]; text?: string; attachment_ids?: string[] } = {};
  if ('option_ids' in value) {
    const options = stringList(value.option_ids);
    if (options === null) return null;
    parsed.option_ids = options;
  }
  if ('text' in value) {
    if (typeof value.text !== 'string') return null;
    parsed.text = value.text;
  }
  if ('attachment_ids' in value) {
    const attachments = stringList(value.attachment_ids);
    if (attachments === null) return null;
    parsed.attachment_ids = attachments;
  }
  return parsed;
}

function saveBody(body: unknown): SaveBody | null {
  if (
    typeof body !== 'object' ||
    body === null ||
    !('version' in body) ||
    typeof body.version !== 'number' ||
    !('save_id' in body) ||
    typeof body.save_id !== 'string' ||
    !('answers' in body) ||
    !Array.isArray(body.answers)
  ) {
    return null;
  }
  const entries: unknown[] = body.answers;
  const answers: SentAnswer[] = [];
  for (const entry of entries) {
    if (typeof entry !== 'object' || entry === null || !('question_id' in entry) || !('value' in entry)) {
      return null;
    }
    const value = sentValue(entry.value);
    if (typeof entry.question_id !== 'string' || value === null) {
      return null;
    }
    answers.push({ questionId: entry.question_id, value });
  }
  return { version: body.version, saveId: body.save_id, answers };
}

function feedbackSeenBody(text: string): FeedbackSeenBody {
  let body: unknown;
  try {
    body = JSON.parse(text);
  } catch {
    return { kind: 'malformed' };
  }
  if (typeof body !== 'object' || body === null || Array.isArray(body)) {
    return { kind: 'invalid', problems: [{ path: 'body', code: 'invalid' }] };
  }
  const sent: unknown = 'version' in body ? body.version : undefined;
  const version = typeof sent === 'number' && Number.isInteger(sent) && sent >= 0 ? sent : null;
  const problems: FieldProblem[] = [];
  if (!('version' in body)) {
    problems.push({ path: 'body', code: 'invalid' });
  } else if (version === null) {
    problems.push({ path: 'body.version', code: 'invalid' });
  }
  for (const key of Object.keys(body)) {
    if (key !== 'version') {
      problems.push({ path: 'body', code: 'unexpected_property' });
    }
  }
  return problems.length > 0 || version === null ? { kind: 'invalid', problems } : { kind: 'parsed', version };
}

function sentVersion(body: unknown): number | null {
  return typeof body === 'object' &&
    body !== null &&
    'version' in body &&
    typeof body.version === 'number' &&
    Number.isInteger(body.version)
    ? body.version
    : null;
}

function isQuestion(block: StudentTemplateBlock): boolean {
  return block.type !== 'instruction' && block.type !== 'material';
}

function valueFits(block: StudentTemplateBlock, value: StudentAnswerValue): boolean {
  const kinds = [value.option_ids, value.text, value.attachment_ids].filter((entry) => entry !== undefined).length;
  if (kinds !== 1) {
    return false;
  }
  switch (block.type) {
    case 'single_choice':
    case 'multiple_choice':
      return value.option_ids !== undefined;
    case 'short_text':
    case 'short_number':
    case 'long_text':
      return value.text !== undefined;
    case 'file':
      return value.attachment_ids !== undefined;
    case 'instruction':
    case 'material':
      return false;
  }
}

function textProblem(block: StudentTemplateBlock, text: string, path: string): FieldProblem | null {
  const limit =
    block.type === 'short_text' ? shortTextLimit : block.type === 'short_number' ? numberTextLimit : block.max_length;
  if (limit === undefined) {
    return { path, code: 'invalid' };
  }
  if (text.includes('\u0000')) {
    return { path: `${path}.text`, code: 'invalid' };
  }
  return Array.from(text).length > limit ? { path: `${path}.text`, code: 'too_long' } : null;
}

function valueProblem(block: StudentTemplateBlock, value: StudentAnswerValue, path: string): FieldProblem | null {
  if (!valueFits(block, value)) {
    return { path, code: 'invalid' };
  }
  if (value.option_ids !== undefined) {
    const options = (block.options ?? []).map((option) => option.id);
    const selected = value.option_ids;
    const unknown = selected.some((option, index) => !options.includes(option) || selected.indexOf(option) !== index);
    return unknown || (block.type === 'single_choice' && selected.length > 1)
      ? { path: `${path}.option_ids`, code: 'invalid' }
      : null;
  }
  if (value.text !== undefined) {
    return textProblem(block, value.text, path);
  }
  const files = value.attachment_ids ?? [];
  if (files.some((file, index) => !uuidPattern.test(file) || files.indexOf(file) !== index)) {
    return { path: `${path}.attachment_ids`, code: 'invalid' };
  }
  return block.max_files === undefined || files.length > block.max_files
    ? { path: `${path}.attachment_ids`, code: 'too_many' }
    : null;
}

function structuralProblem(submission: StudentSubmission, answers: readonly SentAnswer[]): FieldProblem | null {
  const named: string[] = [];
  const files: string[] = [];
  for (const [index, sent] of answers.entries()) {
    const path = `body.answers[${String(index)}]`;
    const block = submission.blocks.find((candidate) => candidate.id === sent.questionId && isQuestion(candidate));
    if (block === undefined || named.includes(sent.questionId)) {
      return { path: `${path}.question_id`, code: 'invalid' };
    }
    const problem = valueProblem(block, sent.value, `${path}.value`);
    if (problem !== null) {
      return problem;
    }
    for (const attachmentId of sent.value.attachment_ids ?? []) {
      if (files.includes(attachmentId.toLowerCase())) {
        return { path: `${path}.value.attachment_ids`, code: 'invalid' };
      }
      files.push(attachmentId.toLowerCase());
    }
    named.push(sent.questionId);
  }
  return null;
}

export class FakeSubmissionServer {
  private readonly feedbackSeenListener: (studentId: string, submissionId: string) => void;
  private readonly submissions = new Map<string, StoredSubmission>();
  private readonly reviewed = new Map<string, { readonly tutorId: string; readonly submission: TutorSubmission }>();
  private readonly detailReads = new Map<string, number>();
  private readonly pendingUploads = new Map<string, PendingAnswerUpload>();
  private readonly fullQuotas = new Set<string>();
  private readonly saveRefusals: Response[] = [];
  private uploaded = 0;
  private readonly savedBodies: string[] = [];
  private readonly feedbackSeenLog: FeedbackSeenRequest[] = [];
  private readonly feedbackSeenRefusals: Response[] = [];
  private feedbackSeenGate: Promise<void> | null = null;
  private feedbackSeenReceived: () => void = () => undefined;

  constructor(feedbackSeenListener: (studentId: string, submissionId: string) => void = () => undefined) {
    this.feedbackSeenListener = feedbackSeenListener;
  }

  get saves(): readonly string[] {
    return this.savedBodies;
  }

  get feedbackSeenRequests(): readonly FeedbackSeenRequest[] {
    return this.feedbackSeenLog;
  }

  add(studentId: string, tutorId: string, submission: StudentSubmission): void {
    this.submissions.set(submission.id, {
      studentId,
      tutorId,
      submission,
      lastSaveId: null,
      firstSubmitted: submittedStatuses.has(submission.status),
      ended: false,
    });
  }

  refuseNextFeedbackSeen(status: number, code: string): void {
    this.feedbackSeenRefusals.push(fakeRefusal(status, code));
  }

  pauseFeedbackSeen(): { readonly resume: () => void; readonly received: Promise<void> } {
    let resume = (): void => undefined;
    let notifyReceived = (): void => undefined;
    this.feedbackSeenGate = new Promise((resolve) => {
      resume = (): void => {
        this.feedbackSeenGate = null;
        this.feedbackSeenReceived = (): void => undefined;
        resolve();
      };
    });
    const received = new Promise<void>((resolve) => {
      notifyReceived = resolve;
    });
    this.feedbackSeenReceived = notifyReceived;
    return { resume, received };
  }

  returnSubmission(id: string): void {
    const stored = this.required(id);
    if (stored.submission.status !== 'submitted') {
      throw new Error('only submitted fake work can be returned');
    }
    stored.firstSubmitted = true;
    stored.submission = {
      ...stored.submission,
      status: 'returned',
      version: stored.submission.version + 1,
      editable_question_ids: stored.submission.blocks
        .filter((block) => manualTypes.has(block.type))
        .map((block) => block.id),
    };
  }

  endRelationshipOf(id: string): EndedRelationship {
    const target = this.required(id);
    const relationshipId = target.submission.tutor.relationship_id;
    for (const stored of this.submissions.values()) {
      if (stored.studentId === target.studentId && stored.submission.tutor.relationship_id === relationshipId) {
        stored.ended = true;
      }
    }
    return { studentId: target.studentId, relationshipId };
  }

  openCount(tutorId: string, relationshipId: string): number {
    return [...this.submissions.values()].filter(
      (stored) =>
        stored.tutorId === tutorId &&
        stored.submission.tutor.relationship_id === relationshipId &&
        openStatuses.has(stored.submission.status),
    ).length;
  }

  addReviewed(tutorId: string, submission: TutorSubmission): void {
    this.reviewed.set(submission.id, { tutorId, submission });
  }

  readsOf(id: string): number {
    return this.detailReads.get(id) ?? 0;
  }

  refuseNextSave(status: number, code: string, fields: readonly FieldProblem[] = []): void {
    this.saveRefusals.push(fakeRefusal(status, code, fields));
  }

  fillQuota(tutorId: string): void {
    this.fullQuotas.add(tutorId);
  }

  handles(path: string): boolean {
    return path.startsWith(submissionPathPrefix);
  }

  async handle(request: Request, caller: Me): Promise<Response> {
    const address = new URL(request.url);
    const [id = '', action, ...rest] = address.pathname.slice(submissionPathPrefix.length).split('/');
    const stored = this.submissions.get(id);
    const reviewed = this.reviewed.get(id);
    if (request.method === 'GET' && action === undefined && rest.length === 0) {
      this.detailReads.set(id, this.readsOf(id) + 1);
      if (reviewed?.tutorId === caller.id) {
        return answer(200, reviewed.submission);
      }
    }
    if (request.method === 'POST' && action === 'feedback-seen' && rest.length === 0 && caller.role === 'student') {
      return this.markFeedbackSeen(id, caller, await request.text());
    }
    if (stored?.studentId !== caller.id || rest.length > 0) {
      return fakeRefusal(404, 'not_found');
    }
    if (request.method === 'GET' && action === undefined) {
      return answer(200, stored.submission);
    }
    if (request.method === 'PUT' && action === 'answers') {
      return this.save(stored, await request.json().catch(() => null));
    }
    if (request.method === 'POST' && action === 'submit') {
      return this.submit(stored, await request.json().catch(() => null));
    }
    return fakeRefusal(405, 'method_not_allowed');
  }

  upload(request: Request, caller: Me, body: FormData, now: number): Response {
    const scope = new URL(request.url).searchParams.get('scope') ?? '';
    const [submissionId = '', questionId = ''] = scope.startsWith(answerScopePrefix)
      ? scope.slice(answerScopePrefix.length).split(':')
      : [];
    const stored = this.submissions.get(submissionId);
    const block = stored?.submission.blocks.find((candidate) => candidate.id === questionId);
    if (stored?.studentId !== caller.id || block?.type !== 'file') {
      return fakeRefusal(404, 'not_found');
    }
    if (!this.writable(stored)) {
      return fakeRefusal(409, 'not_editable');
    }
    const file = body.get('file');
    if ([...body.keys()].length !== 1 || file === null || typeof file === 'string') {
      return fakeRefusal(422, 'validation_failed', [{ path: 'body.file', code: 'required' }]);
    }
    if (file.size > answerUploadLimitBytes) {
      return fakeRefusal(413, 'payload_too_large');
    }
    const kind = answerFileKinds[file.type];
    if (kind === undefined || !(block.allowed_kinds ?? []).includes(kind)) {
      return fakeRefusal(415, 'unsupported_media_type');
    }
    this.uploaded += 1;
    const attachmentId = `00000000-0000-4000-a000-${String(this.uploaded).padStart(12, '0')}`;
    this.pendingUploads.set(attachmentId, {
      studentId: caller.id,
      submissionId,
      questionId,
      file: {
        attachment_id: attachmentId,
        filename: file.name,
        media_type: file.type,
        size_bytes: file.size,
        width: null,
        height: null,
        url: `/api/v1/files/${attachmentId}?t=fake-download`,
        expires_at: new Date(now + 300_000).toISOString(),
      },
    });
    return answer(201, { id: attachmentId, filename: file.name, media_type: file.type, size_bytes: file.size });
  }

  private required(id: string): StoredSubmission {
    const stored = this.submissions.get(id);
    if (stored === undefined) {
      throw new Error('no fake submission has this id');
    }
    return stored;
  }

  private writable(stored: StoredSubmission): boolean {
    return editableStatuses.has(stored.submission.status) && !stored.ended;
  }

  private async markFeedbackSeen(id: string, caller: Me, text: string): Promise<Response> {
    const parsed = feedbackSeenBody(text);
    if (parsed.kind === 'malformed') {
      return fakeRefusal(400, 'bad_request');
    }
    if (parsed.kind === 'invalid') {
      return fakeRefusal(422, 'validation_failed', parsed.problems);
    }
    this.feedbackSeenLog.push({ id, version: parsed.version });
    this.feedbackSeenReceived();
    await this.feedbackSeenGate;
    const planned = this.feedbackSeenRefusals.shift();
    if (planned !== undefined) {
      return planned;
    }
    const stored = this.submissions.get(id);
    if (stored?.studentId !== caller.id) {
      return fakeRefusal(404, 'not_found');
    }
    if (parsed.version !== stored.submission.version) {
      return fakeRefusal(409, 'conflict');
    }
    this.feedbackSeenListener(caller.id, id);
    return new Response(null, { status: 204, headers: { 'Cache-Control': 'no-store' } });
  }

  private submit(stored: StoredSubmission, body: unknown): Response {
    const version = sentVersion(body);
    if (version === null) {
      return fakeRefusal(422, 'validation_failed', [{ path: 'body.version', code: 'invalid' }]);
    }
    const submission = stored.submission;
    if (submission.status === 'submitted' || submission.status === 'checked') {
      return answer(200, submission);
    }
    if (!this.writable(stored)) {
      return fakeRefusal(409, 'not_editable');
    }
    if (version !== submission.version) {
      return fakeRefusal(409, 'conflict');
    }
    stored.firstSubmitted = true;
    stored.submission = {
      ...submission,
      status: submission.blocks.some((block) => manualTypes.has(block.type)) ? 'submitted' : 'checked',
      version: submission.version + 1,
      editable_question_ids: [],
    };
    return answer(200, stored.submission);
  }

  private save(stored: StoredSubmission, body: unknown): Response {
    this.savedBodies.push(JSON.stringify(body));
    const planned = this.saveRefusals.shift();
    if (planned !== undefined) {
      return planned;
    }
    const parsed = saveBody(body);
    if (parsed === null) {
      return fakeRefusal(422, 'validation_failed');
    }
    const submission = stored.submission;
    if (parsed.saveId === stored.lastSaveId) {
      return answer(200, submission);
    }
    if (!this.writable(stored)) {
      return fakeRefusal(409, 'not_editable');
    }
    if (parsed.version !== submission.version) {
      return fakeRefusal(409, 'conflict');
    }
    const frozen =
      stored.firstSubmitted &&
      parsed.answers.some((sent) =>
        submission.blocks.some((block) => block.id === sent.questionId && automaticTypes.has(block.type)),
      );
    if (frozen) {
      return fakeRefusal(409, 'answer_frozen');
    }
    const problem = structuralProblem(submission, parsed.answers);
    if (problem !== null) {
      return fakeRefusal(422, 'validation_failed', [problem]);
    }
    const bindings = this.fileBindings(stored, parsed.answers);
    if (bindings === null) {
      return fakeRefusal(404, 'not_found');
    }
    if (bindings.newlyBound.length > 0 && this.fullQuotas.has(stored.tutorId)) {
      return fakeRefusal(507, 'quota_exceeded');
    }
    for (const attachmentId of bindings.newlyBound) {
      this.pendingUploads.delete(attachmentId);
    }
    stored.submission = this.withAnswers(submission, parsed.answers, bindings.files);
    stored.lastSaveId = parsed.saveId;
    return answer(200, stored.submission);
  }

  private fileBindings(
    stored: StoredSubmission,
    answers: readonly SentAnswer[],
  ): { readonly newlyBound: readonly string[]; readonly files: ReadonlyMap<string, SubmissionFile> } | null {
    const newlyBound: string[] = [];
    const files = new Map<string, SubmissionFile>();
    for (const sent of answers) {
      const current = stored.submission.answers.find((entry) => entry.question_id === sent.questionId);
      for (const attachmentId of sent.value.attachment_ids ?? []) {
        const bound = current?.files.find((file) => file.attachment_id === attachmentId);
        const pending = this.pendingUploads.get(attachmentId);
        if (bound !== undefined) {
          files.set(attachmentId, bound);
          continue;
        }
        if (
          pending?.studentId !== stored.studentId ||
          pending.submissionId !== stored.submission.id ||
          pending.questionId !== sent.questionId
        ) {
          return null;
        }
        files.set(attachmentId, pending.file);
        newlyBound.push(attachmentId);
      }
    }
    return { newlyBound, files };
  }

  private withAnswers(
    submission: StudentSubmission,
    answers: readonly SentAnswer[],
    files: ReadonlyMap<string, SubmissionFile>,
  ): StudentSubmission {
    const saved = new Map<string, StudentAnswer>(submission.answers.map((entry) => [entry.question_id, entry]));
    for (const sent of answers) {
      saved.set(sent.questionId, {
        question_id: sent.questionId,
        value: sent.value,
        files: (sent.value.attachment_ids ?? []).flatMap((attachmentId) => {
          const file = files.get(attachmentId);
          return file === undefined ? [] : [file];
        }),
      });
    }
    return {
      ...submission,
      status: submission.status === 'not_started' ? 'draft' : submission.status,
      version: submission.version + 1,
      answers: [...saved.values()],
    };
  }
}

export const pastSimpleQuestions = {
  choice: '0d3c3c1e-2b7a-4c55-9a21-5d2e8f4a6b70',
  text: '7e1d4c2a-0b7d-4c55-9a21-5d2e8f4a6b71',
  file: '3a9b8c7d-6e5f-4a3b-9c2d-1e0f9a8b7c72',
} as const;

export function pastSimpleSubmission(id: string): StudentSubmission {
  return {
    id,
    title: 'Past simple',
    tutor: { name: 'Maria', colour: null, relationship_id: '5b4a3c2d-1e0f-4a9b-8c7d-6e5f4a3b2c1d' },
    status: 'not_started',
    deadline_at: null,
    late: false,
    issued_before_reclaim: false,
    version: 1,
    editable_question_ids: Object.values(pastSimpleQuestions),
    blocks: [
      { id: '9c8b7a6f-5e4d-4c3b-8a2f-1e0d9c8b7a69', type: 'instruction', text: 'Answer every question.' },
      {
        id: pastSimpleQuestions.choice,
        type: 'single_choice',
        prompt: 'Yesterday I ___ to school.',
        images: [],
        options: [
          { id: '1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d', text: 'goed' },
          { id: '2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e', text: 'went' },
        ],
      },
      { id: pastSimpleQuestions.text, type: 'short_text', prompt: 'Write the past form of go.', images: [] },
      {
        id: pastSimpleQuestions.file,
        type: 'file',
        prompt: 'Photograph your answer',
        images: [],
        allowed_kinds: ['images', 'pdf'],
        max_files: 2,
      },
    ],
    answers: [],
    review: null,
  };
}

export function pastSimpleReview(id: string): TutorSubmission {
  return {
    id,
    issue_id: '6d5c4b3a-2f1e-4d0c-9b8a-7f6e5d4c3b2a',
    title: 'Past simple',
    student: { display_name: 'Anna', relationship_id: '5b4a3c2d-1e0f-4a9b-8c7d-6e5f4a3b2c1d' },
    status: 'submitted',
    answer_policy: 'after_check',
    deadline_at: null,
    late: false,
    issued_before_reclaim: false,
    first_submitted_at: '2026-11-01T10:00:00Z',
    submitted_at: '2026-11-01T10:00:00Z',
    checked_at: null,
    version: 3,
    max_points: 4,
    auto_points: 0,
    final_points: null,
    percentage: null,
    blocks: [
      { id: '9c8b7a6f-5e4d-4c3b-8a2f-1e0d9c8b7a69', type: 'instruction', text: 'Answer every question.' },
      { id: pastSimpleQuestions.text, type: 'long_text', prompt: 'Describe your weekend.', points: 4 },
    ],
    answers: [
      {
        question_id: pastSimpleQuestions.text,
        value: { text: 'I went to the sea.' },
        files: [],
        frozen_at: '2026-11-01T10:00:00Z',
        normalised: null,
        normaliser_version: null,
        auto_correct: null,
        auto_points: null,
      },
    ],
    revisions: [],
  };
}
