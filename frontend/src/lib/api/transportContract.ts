import type { AuthTransport } from './transport.ts';

export interface ContractSubject {
  readonly transport: AuthTransport;
  readonly origin: string;
  readonly email: string;
  readonly password: string;
  readonly otherTutorEmail: string;
  readonly otherTutorPassword: string;
  readonly student: () => Promise<{ readonly email: string; readonly password: string }>;
  readonly expireAccessTokens: () => Promise<void>;
  readonly passSeconds: (seconds: number) => Promise<void>;
  readonly blockAccount: () => Promise<void>;
  readonly unsubscribeToken: () => Promise<string>;
  readonly seedTwoLessons: () => Promise<SeededLessons>;
  readonly seedSubmission: () => Promise<SeededSubmission>;
  readonly fillTutorFileQuota: () => Promise<void>;
  readonly seedUpcomingDeadline: () => Promise<SeededDeadline>;
  readonly seedStudentHome: () => Promise<SeededStudentHome>;
  readonly unconfirmedTutor: () => Promise<{ readonly email: string; readonly password: string }>;
  readonly seedSharedStudentIssues: () => Promise<SeededSharedStudent>;
  readonly seedCheckedSubmission: () => Promise<SeededSubmission>;
  readonly seedOtherStudentSubmission: () => Promise<SeededSubmission>;
  readonly returnSeededSubmission: (seeded: SeededSubmission) => Promise<void>;
  readonly endSeededRelationship: (seeded: SeededSubmission) => Promise<void>;
  readonly readyExport: (accessToken: string) => Promise<void>;
  readonly seedGroupWithMember: () => Promise<SeededGroup>;
  readonly endGroupMemberRelationship: (seeded: SeededGroup) => Promise<void>;
  readonly seedGroupWithFutureLesson: () => Promise<SeededGroup>;
  readonly seedLibraryFolderAndTemplate: () => Promise<SeededLibrary>;
}

export interface SeededGroup {
  readonly groupId: string;
  readonly relationshipId: string;
}

export interface SeededLibrary {
  readonly folderId: string;
  readonly childFolderId: string;
  readonly templateId: string;
  readonly templateVersion: number;
}

export interface SeededSubmission {
  readonly email: string;
  readonly password: string;
  readonly submissionId: string;
  readonly textQuestionId: string;
  readonly fileQuestionId: string;
  readonly relationshipId: string;
}

export interface SeededDeadline {
  readonly issueId: string;
}

export interface SeededStudentHome {
  readonly email: string;
  readonly password: string;
  readonly relationshipId: string;
  readonly endedRelationshipId: string;
  readonly otherStudentRelationshipId: string;
}

export interface SeededSharedStudent {
  readonly cardId: string;
  readonly otherTutorCardId: string;
}

export interface SeededLessons {
  readonly from: string;
  readonly to: string;
  readonly laterStart: string;
  readonly earlierStart: string;
}

export type ContractScenario = (subject: ContractSubject) => Promise<void>;

class ContractViolation extends Error {
  override readonly name = 'ContractViolation';
}

function check(condition: boolean, message: string): void {
  if (!condition) {
    throw new ContractViolation(message);
  }
}

async function errorCode(response: Response): Promise<string | undefined> {
  const body: unknown = await response.json();
  if (typeof body !== 'object' || body === null || !('error' in body)) {
    return undefined;
  }
  const error = body.error;
  return typeof error === 'object' && error !== null && 'code' in error && typeof error.code === 'string'
    ? error.code
    : undefined;
}

async function accessToken(response: Response): Promise<string> {
  check(response.status === 200, `expected 200 with a token, got ${String(response.status)}`);
  const body: unknown = await response.json();
  check(
    typeof body === 'object' &&
      body !== null &&
      'access_token' in body &&
      typeof body.access_token === 'string' &&
      'expires_in' in body &&
      body.expires_in === 900,
    'expected an access token valid for 900 seconds',
  );
  return typeof body === 'object' && body !== null && 'access_token' in body && typeof body.access_token === 'string'
    ? body.access_token
    : '';
}

function timeout(): AbortSignal {
  return AbortSignal.timeout(10_000);
}

function logoutRequest(subject: ContractSubject, token: string): Request {
  return new Request(`${subject.origin}/api/v1/auth/logout`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  });
}

function meRequest(subject: ContractSubject, token: string): Request {
  return new Request(`${subject.origin}/api/v1/users/me`, { headers: { Authorization: `Bearer ${token}` } });
}

function cardRequest(subject: ContractSubject, token: string, path: string, body?: unknown): Request {
  return new Request(`${subject.origin}/api/v1/students${path}`, {
    method: body === undefined ? 'GET' : 'POST',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
}

function studentListRequest(subject: ContractSubject, token: string, search: string): Request {
  const query = new URLSearchParams({ state: 'invited', q: search });
  return new Request(`${subject.origin}/api/v1/students?${query.toString()}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

function listedNames(body: unknown): unknown[] {
  if (typeof body !== 'object' || body === null || !('items' in body) || !Array.isArray(body.items)) {
    return [];
  }
  const items: unknown[] = body.items;
  return items.map((item) => (typeof item === 'object' && item !== null ? property(item, 'display_name') : undefined));
}

function joinRequestsRequest(subject: ContractSubject, token: string): Request {
  return new Request(`${subject.origin}/api/v1/join-requests`, { headers: { Authorization: `Bearer ${token}` } });
}

function notificationsRequest(subject: ContractSubject, token: string): Request {
  return new Request(`${subject.origin}/api/v1/notifications`, { headers: { Authorization: `Bearer ${token}` } });
}

const emptyWeek = { from: '2026-11-01T21:00:00.000Z', to: '2026-11-08T21:00:00.000Z' };

function calendarRequest(subject: ContractSubject, token: string, query: Record<string, string> = emptyWeek): Request {
  return new Request(`${subject.origin}/api/v1/calendar?${new URLSearchParams(query).toString()}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

async function fieldProblems(response: Response): Promise<string> {
  const body: unknown = await response.json();
  if (typeof body !== 'object' || body === null || !('error' in body)) {
    return '';
  }
  const error = body.error;
  if (typeof error !== 'object' || error === null || !('code' in error) || !('fields' in error)) {
    return '';
  }
  const fields: unknown = error.fields;
  const described = Array.isArray(fields)
    ? fields.map((field: unknown) =>
        typeof field === 'object' && field !== null && 'path' in field && 'code' in field
          ? `${String(field.path)}:${String(field.code)}`
          : '?',
      )
    : [];
  return `${String(error.code)} ${described.join(',')}`;
}

function startsOf(body: unknown): number[] {
  if (typeof body !== 'object' || body === null || !('items' in body) || !Array.isArray(body.items)) {
    return [];
  }
  const items: unknown[] = body.items;
  return items.map((item) =>
    typeof item === 'object' && item !== null && 'starts_at' in item && typeof item.starts_at === 'string'
      ? Date.parse(item.starts_at)
      : Number.NaN,
  );
}

const dayMilliseconds = 24 * 60 * 60 * 1_000;

function emailPreferencesRequest(subject: ContractSubject, token: string, body?: unknown): Request {
  return new Request(`${subject.origin}/api/v1/users/me/email-preferences`, {
    method: body === undefined ? 'GET' : 'PUT',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
}

function unsubscribeRequest(subject: ContractSubject, method: 'GET' | 'POST', token: string): Request {
  const path = method === 'GET' ? '/api/v1/unsubscribe/preview' : '/api/v1/unsubscribe';
  return new Request(`${subject.origin}${path}?${new URLSearchParams({ token }).toString()}`, { method });
}

async function emailGroups(response: Response): Promise<string> {
  check(response.status === 200, `expected 200 with the email groups, got ${String(response.status)}`);
  return JSON.stringify(await response.json());
}

function cardFields(value: unknown): { id: unknown; name: unknown; state: unknown; url: unknown } {
  const card = typeof value === 'object' && value !== null ? value : {};
  const invitation = 'invitation' in card && typeof card.invitation === 'object' ? card.invitation : null;
  return {
    id: 'id' in card ? card.id : undefined,
    name: 'display_name' in card ? card.display_name : undefined,
    state: 'state' in card ? card.state : undefined,
    url: invitation !== null && 'url' in invitation ? invitation.url : undefined,
  };
}

function submissionRequest(subject: ContractSubject, token: string, id: string): Request {
  return new Request(`${subject.origin}/api/v1/submissions/${encodeURIComponent(id)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

interface SentAnswerValue {
  readonly question_id: string;
  readonly value: Readonly<Record<string, unknown>>;
}

function answersRequest(
  subject: ContractSubject,
  token: string,
  id: string,
  version: number,
  answers: readonly SentAnswerValue[],
): Request {
  return new Request(`${subject.origin}/api/v1/submissions/${encodeURIComponent(id)}/answers`, {
    method: 'PUT',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ version, save_id: crypto.randomUUID(), answers }),
  });
}

function answerUploadRequest(subject: ContractSubject, token: string, seeded: SeededSubmission): Request {
  const scope = `submission:${seeded.submissionId}:${seeded.fileQuestionId}`;
  const body = new FormData();
  body.append(
    'file',
    new File(['%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n'], 'Essay.pdf', {
      type: 'application/pdf',
    }),
  );
  return new Request(
    `${subject.origin}/api/v1/uploads?${new URLSearchParams({ context: 'answer', scope }).toString()}`,
    {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
      body,
    },
  );
}

const randomSubmissionId = '6f1e2d3c-4b5a-4978-8a6b-5c4d3e2f1a0b';
const randomRelationshipId = '2a1b0c9d-8e7f-4a6b-9c5d-4e3f2a1b0c9d';

const tutorHomeKeys: readonly string[] = [
  'today_lessons',
  'today_lessons_count',
  'needs_checking',
  'upcoming_deadlines',
  'unread_channels',
  'join_requests',
  'tutor_code',
  'checklist',
];

const studentHomeKeys: readonly string[] = [
  'next_lesson',
  'today_lessons_count',
  'todo',
  'recently_checked',
  'next_seven_days',
  'next_seven_days_count',
  'tutors',
  'tutors_count',
];

const assignmentItemKeys: readonly string[] = [
  'submission_id',
  'title',
  'tutor',
  'status',
  'late',
  'issued_before_reclaim',
  'deadline_at',
  'issued_at',
  'submitted_at',
  'checked_at',
  'feedback_excerpt',
  'has_corrected_files',
  'feedback_new',
];

const assignmentTutorKeys: readonly string[] = ['relationship_id', 'name', 'colour'];

const calendarItemKeys: readonly string[] = [
  'id',
  'title',
  'starts_at',
  'ends_at',
  'status',
  'took_place',
  'timezone',
  'audience',
  'tutor',
];

const calendarAudienceKeys: readonly string[] = ['kind', 'label', 'label_kind', 'colour'];

const homeTutorKeys: readonly string[] = [
  'relationship_id',
  'state',
  'name',
  'avatar_id',
  'colour',
  'subjects',
  'channel_id',
];

const issueItemKeys: readonly string[] = [
  'id',
  'title',
  'audience',
  'lesson_id',
  'deadline_at',
  'answer_policy',
  'status',
  'issued_at',
  'withdrawn_at',
  'has_manual_questions',
  'max_points',
  'version',
  'progress',
];

const tutorSettingsKeys: readonly string[] = ['file_quota_bytes', 'file_usage_bytes', 'onboarding_dismissed'];

function homeRequest(subject: ContractSubject, token: string, query: Record<string, string> = {}): Request {
  const search = new URLSearchParams(query).toString();
  return new Request(`${subject.origin}/api/v1/home${search === '' ? '' : `?${search}`}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

function tutorSettingsRequest(subject: ContractSubject, token: string, body?: unknown): Request {
  return new Request(`${subject.origin}/api/v1/tutor/settings`, {
    method: body === undefined ? 'GET' : 'PATCH',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
}

function submissionActionRequest(
  subject: ContractSubject,
  token: string,
  id: string,
  action: 'submit' | 'feedback-seen',
  body: unknown,
): Request {
  return new Request(`${subject.origin}/api/v1/submissions/${encodeURIComponent(id)}/${action}`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

async function bodyOf(response: Response): Promise<unknown> {
  const body: unknown = await response.json();
  return body;
}

function property(value: unknown, name: string): unknown {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return undefined;
  }
  return Object.entries(value).find(([key]) => key === name)?.[1];
}

function listOf(value: unknown): unknown[] {
  if (!Array.isArray(value)) {
    return [];
  }
  const entries: unknown[] = value;
  return entries;
}

function keySet(value: unknown): string {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? Object.keys(value).sort().join(',')
    : `not an object: ${JSON.stringify(value)}`;
}

function expectedKeys(keys: readonly string[]): string {
  return [...keys].sort().join(',');
}

function checkKeys(value: unknown, keys: readonly string[], label: string): void {
  check(keySet(value) === expectedKeys(keys), `expected exactly the keys of ${label}, got ${keySet(value)}`);
}

function checkEachKeys(values: readonly unknown[], keys: readonly string[], label: string): void {
  for (const value of values) {
    checkKeys(value, keys, label);
  }
}

function checkCalendarItems(values: readonly unknown[], label: string): void {
  checkEachKeys(values, calendarItemKeys, label);
  checkEachKeys(
    values.map((value) => property(value, 'audience')),
    calendarAudienceKeys,
    `${label} audience`,
  );
}

function checkAssignmentItems(values: readonly unknown[], label: string): void {
  checkEachKeys(values, assignmentItemKeys, label);
  checkEachKeys(
    values.map((value) => property(value, 'tutor')),
    assignmentTutorKeys,
    `${label} tutor`,
  );
}

function feedbackNewOf(home: unknown, submissionId: string): unknown {
  const checked = listOf(property(home, 'recently_checked'));
  const item = checked.find((entry) => property(entry, 'submission_id') === submissionId);
  return property(item, 'feedback_new');
}

async function studentListRow(subject: ContractSubject, token: string, cardId: string): Promise<unknown> {
  const read = await subject.transport.send(cardRequest(subject, token, ''));
  check(read.status === 200, `expected the tutor's student list, got ${String(read.status)}`);
  return listOf(property(await bodyOf(read), 'items')).find((item) => property(item, 'id') === cardId);
}

const studentSubmissionFields: readonly string[] = [
  'id',
  'title',
  'tutor',
  'status',
  'late',
  'issued_before_reclaim',
  'deadline_at',
  'version',
  'blocks',
  'answers',
  'editable_question_ids',
  'review',
];

const answerKeyFields: readonly string[] = [
  'points',
  'max_points',
  'auto_points',
  'override_points',
  'final_points',
  'percentage',
  'correct',
  'auto_correct',
  'correct_option_ids',
  'accepted_answers',
  'correct_value',
  'tolerance',
  'explanation',
  'revealed',
];

function keysOf(value: unknown): string[] {
  if (Array.isArray(value)) {
    const entries: unknown[] = value;
    return entries.flatMap(keysOf);
  }
  if (typeof value !== 'object' || value === null) {
    return [];
  }
  return Object.entries(value).flatMap(([key, entry]) => [key, ...keysOf(entry)]);
}

function versionOf(body: unknown): number {
  return typeof body === 'object' && body !== null && 'version' in body && typeof body.version === 'number'
    ? body.version
    : Number.NaN;
}

function fileAnswerCount(body: unknown, questionId: string): number {
  if (typeof body !== 'object' || body === null || !('answers' in body) || !Array.isArray(body.answers)) {
    return Number.NaN;
  }
  const answers: unknown[] = body.answers;
  const found = answers.find(
    (entry) =>
      typeof entry === 'object' && entry !== null && 'question_id' in entry && entry.question_id === questionId,
  );
  return typeof found === 'object' && found !== null && 'files' in found && Array.isArray(found.files)
    ? found.files.length
    : 0;
}

async function studentSubmission(subject: ContractSubject): Promise<{ seeded: SeededSubmission; token: string }> {
  const seeded = await subject.seedSubmission();
  const token = await accessToken(await subject.transport.login(seeded.email, seeded.password));
  return { seeded, token };
}

async function currentVersion(subject: ContractSubject, token: string, seeded: SeededSubmission): Promise<number> {
  const read = await subject.transport.send(submissionRequest(subject, token, seeded.submissionId));
  check(read.status === 200, `expected the student's submission, got ${String(read.status)}`);
  return versionOf(await read.json());
}

const headersThatVary: ReadonlySet<string> = new Set(['date', 'content-length', 'x-request-id']);

interface RefusalShape {
  readonly status: number;
  readonly code: string | undefined;
  readonly same: (other: RefusalShape) => boolean;
  readonly describe: () => string;
}

async function refusalShape(response: Response): Promise<RefusalShape> {
  const body: unknown = await response.json();
  const error = typeof body === 'object' && body !== null && 'error' in body ? body.error : null;
  const code = typeof error === 'object' && error !== null && 'code' in error ? error.code : undefined;
  const message = typeof error === 'object' && error !== null && 'message' in error ? error.message : undefined;
  const headers = [...response.headers.entries()]
    .filter(([name]) => !headersThatVary.has(name))
    .map(([name, value]) => `${name}: ${value}`)
    .sort()
    .join('; ');
  const summary = `${String(response.status)} ${String(code)} ${String(message)} [${headers}]`;
  return {
    status: response.status,
    code: typeof code === 'string' ? code : undefined,
    same: (other) => other.describe() === summary,
    describe: () => summary,
  };
}

const exportKeys = [
  'id',
  'state',
  'failure',
  'requested_at',
  'built_at',
  'expires_at',
  'size_bytes',
  'progress',
  'next_allowed_at',
] as const;

function exportRequest(
  subject: ContractSubject,
  token: string,
  method: 'GET' | 'POST' | 'DELETE',
  path: '' | '/current' | '/current/download',
  body?: unknown,
): Request {
  return new Request(`${subject.origin}/api/v1/exports${path}`, {
    method,
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
}

function requestExport(subject: ContractSubject, token: string, password: string): Promise<Response> {
  return subject.transport.send(exportRequest(subject, token, 'POST', '', { password }));
}

function wrongPassword(subject: ContractSubject): string {
  return `${subject.password} but wrong`;
}

async function currentExport(subject: ContractSubject, token: string): Promise<unknown> {
  const response = await subject.transport.send(exportRequest(subject, token, 'GET', '/current'));
  check(response.status === 200, `expected the current export, got ${String(response.status)}`);
  return bodyOf(response);
}

function checkRetryAfter(response: Response): void {
  const header = response.headers.get('Retry-After') ?? '';
  check(/^[1-9][0-9]*$/.test(header), `expected a positive Retry-After, got ${JSON.stringify(header)}`);
}

type ContractMethod = 'GET' | 'POST' | 'PATCH';

interface ContractAnswer {
  readonly status: number;
  readonly body: unknown;
}

async function callContract(
  subject: ContractSubject,
  token: string,
  method: ContractMethod,
  path: string,
  body?: unknown,
): Promise<ContractAnswer> {
  const response = await subject.transport.send(
    new Request(`${subject.origin}/api/v1${path}`, {
      method,
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    }),
  );
  return { status: response.status, body: await bodyOf(response) };
}

function refusalCode(answer: ContractAnswer): unknown {
  return property(property(answer.body, 'error'), 'code');
}

function problemsOf(answer: ContractAnswer): string {
  const error = property(answer.body, 'error');
  const described = listOf(property(error, 'fields')).map(
    (field) => `${String(property(field, 'path'))}:${String(property(field, 'code'))}`,
  );
  return `${String(property(error, 'code'))} ${described.join(',')}`;
}

function groupPath(seeded: SeededGroup, action = ''): string {
  return `/groups/${encodeURIComponent(seeded.groupId)}${action}`;
}

function memberIdsOf(answer: ContractAnswer): string {
  return JSON.stringify(listOf(property(answer.body, 'members')).map((member) => property(member, 'relationship_id')));
}

function onlyMember(seeded: SeededGroup): string {
  return JSON.stringify([seeded.relationshipId]);
}

function idsOf(values: unknown): unknown[] {
  return listOf(values).map((value) => property(value, 'id'));
}

function templatePath(seeded: SeededLibrary): string {
  return `/library/templates/${encodeURIComponent(seeded.templateId)}`;
}

function folderListingPath(folderId: string): string {
  return `/library?${new URLSearchParams({ folder: folderId }).toString()}`;
}

const groupKeys: readonly string[] = ['id', 'name', 'colour', 'archived_at', 'members'];

const groupDetailKeys: readonly string[] = [
  ...groupKeys,
  'channel_id',
  'next_lessons',
  'future_lesson_count',
  'has_open_series',
];

const groupMemberKeys: readonly string[] = ['relationship_id', 'display_name', 'name', 'avatar_id', 'added_at'];

const randomFolderId = '5e4d3c2b-1a09-4f8e-9d7c-6b5a4f3e2d1c';

export const authTransportContract: Readonly<Record<string, ContractScenario>> = {
  'a request sent with an expired token is refused as token_expired': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    await subject.expireAccessTokens();

    const response = await subject.transport.send(meRequest(subject, token));

    check(response.status === 401, `expected 401, got ${String(response.status)}`);
    check((await errorCode(response)) === 'token_expired', 'expected token_expired');
  },

  'two refreshes with the same cookie rotate once and revoke the session as reuse': async (subject) => {
    await accessToken(await subject.transport.login(subject.email, subject.password));

    const answers = await Promise.all([subject.transport.refresh(timeout()), subject.transport.refresh(timeout())]);

    const statuses = answers.map((answer) => answer.status).sort();
    check(statuses.join(',') === '200,401', `expected one 200 and one 401, got ${statuses.join(',')}`);
    const winner = answers.find((answer) => answer.status === 200);
    const revoked = await subject.transport.me(winner === undefined ? '' : await accessToken(winner), timeout());
    check(revoked.status === 401, `expected the rotated token to be revoked, got ${String(revoked.status)}`);
    const afterwards = await subject.transport.refresh(timeout());
    check(
      afterwards.status === 204 || afterwards.status === 401,
      `expected no token from the revoked session, got ${String(afterwards.status)}`,
    );
  },

  'a tutor reads back the card it created and an unknown card is not found': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const created = await subject.transport.send(cardRequest(subject, token, '', { display_name: 'Contract student' }));
    const createdCard = cardFields(await created.json());
    const read = await subject.transport.send(cardRequest(subject, token, `/${String(createdCard.id)}`));
    const missing = await subject.transport.send(cardRequest(subject, token, '/00000000-0000-4000-8000-00000000ffff'));

    check(created.status === 201, `expected 201, got ${String(created.status)}`);
    check(typeof createdCard.id === 'string', 'expected a card identifier');
    check(createdCard.name === 'Contract student' && createdCard.state === 'invited', 'expected an invited card');
    check(typeof createdCard.url === 'string', 'expected an open invitation link');
    check(read.status === 200, `expected 200, got ${String(read.status)}`);
    const readCard = cardFields(await read.json());
    check(readCard.id === createdCard.id && readCard.url === createdCard.url, 'expected the same card and link');
    check(missing.status === 404, `expected 404, got ${String(missing.status)}`);
    check((await errorCode(missing)) === 'not_found', 'expected not_found');
  },

  'the student list search finds a card by its name, finds nothing for other text and refuses a search over 254 characters':
    async (subject) => {
      const token = await accessToken(await subject.transport.login(subject.email, subject.password));
      const suffix = crypto.randomUUID().slice(0, 8);
      await subject.transport.send(cardRequest(subject, token, '', { display_name: `Searchable ${suffix}` }));
      await subject.transport.send(cardRequest(subject, token, '', { display_name: `Unrelated ${suffix}` }));

      const found = await subject.transport.send(studentListRequest(subject, token, `searchable ${suffix}`));
      const none = await subject.transport.send(studentListRequest(subject, token, `absent ${suffix}`));
      const tooLong = await subject.transport.send(studentListRequest(subject, token, 'a'.repeat(255)));

      check(found.status === 200, `expected 200, got ${String(found.status)}`);
      const foundNames = listedNames(await found.json());
      check(
        foundNames.length === 1 && foundNames[0] === `Searchable ${suffix}`,
        `expected only the searched card, got ${JSON.stringify(foundNames)}`,
      );
      check(none.status === 200, `expected 200, got ${String(none.status)}`);
      check(listedNames(await none.json()).length === 0, 'expected no card for other text');
      check(tooLong.status === 422, `expected 422, got ${String(tooLong.status)}`);
      check((await errorCode(tooLong)) === 'validation_failed', 'expected validation_failed');
    },

  'another tutor cannot read a tutor’s card and gets not_found': async (subject) => {
    const owner = await accessToken(await subject.transport.login(subject.email, subject.password));
    const created = cardFields(
      await (await subject.transport.send(cardRequest(subject, owner, '', { display_name: 'Private student' }))).json(),
    );
    const other = await accessToken(await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword));

    const read = await subject.transport.send(cardRequest(subject, other, `/${String(created.id)}`));

    check(typeof created.id === 'string', 'expected a card identifier');
    check(read.status === 404, `expected 404, got ${String(read.status)}`);
    check((await errorCode(read)) === 'not_found', 'expected not_found');
  },

  'a student is refused on the card routes as on an unknown route with not_found': async (subject) => {
    const tutor = await accessToken(await subject.transport.login(subject.email, subject.password));
    const card = cardFields(
      await (await subject.transport.send(cardRequest(subject, tutor, '', { display_name: 'A tutor’s card' }))).json(),
    );
    const { email, password } = await subject.student();
    const student = await accessToken(await subject.transport.login(email, password));

    const created = await refusalShape(
      await subject.transport.send(cardRequest(subject, student, '', { display_name: 'Someone' })),
    );
    const read = await refusalShape(await subject.transport.send(cardRequest(subject, student, `/${String(card.id)}`)));
    const unknown = await refusalShape(
      await subject.transport.send(
        new Request(`${subject.origin}/api/v1/no-such-route`, { headers: { Authorization: `Bearer ${student}` } }),
      ),
    );

    check(typeof card.id === 'string', 'expected a card identifier');
    check(unknown.status === 404 && unknown.code === 'not_found', 'expected not_found on an unknown route');
    check(created.same(unknown), `expected creation to be refused as an unknown route: ${created.describe()}`);
    check(read.same(unknown), `expected reading to be refused as an unknown route: ${read.describe()}`);
  },

  'a tutor without join requests reads an empty page (a refused read uses the refusal envelope the 404 scenarios check)':
    async (subject) => {
      const token = await accessToken(
        await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword),
      );

      const read = await subject.transport.send(joinRequestsRequest(subject, token));

      check(read.status === 200, `expected 200, got ${String(read.status)}`);
      const body: unknown = await read.json();
      check(
        typeof body === 'object' &&
          body !== null &&
          Object.keys(body).sort().join(',') === 'items,next_cursor' &&
          'items' in body &&
          Array.isArray(body.items) &&
          body.items.length === 0 &&
          'next_cursor' in body &&
          body.next_cursor === null,
        `expected exactly {items: [], next_cursor: null}, got ${JSON.stringify(body)}`,
      );
    },

  'a tutor without lessons reads an empty calendar week that is not truncated': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword));

    const read = await subject.transport.send(calendarRequest(subject, token));

    check(read.status === 200, `expected 200, got ${String(read.status)}`);
    const body: unknown = await read.json();
    check(
      typeof body === 'object' &&
        body !== null &&
        Object.keys(body).sort().join(',') === 'items,truncated' &&
        'items' in body &&
        Array.isArray(body.items) &&
        body.items.length === 0 &&
        'truncated' in body &&
        body.truncated === false,
      `expected exactly {items: [], truncated: false}, got ${JSON.stringify(body)}`,
    );
  },

  'a tutor reads the calendar lessons of a range ordered by start, whatever order they were scheduled in': async (
    subject,
  ) => {
    const seeded = await subject.seedTwoLessons();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const read = await subject.transport.send(calendarRequest(subject, token, { from: seeded.from, to: seeded.to }));

    check(read.status === 200, `expected 200, got ${String(read.status)}`);
    const starts = startsOf(await read.json());
    check(
      starts.length === 2 &&
        starts[0] === Date.parse(seeded.earlierStart) &&
        starts[1] === Date.parse(seeded.laterStart),
      `expected the two seeded lessons earliest first, got starts ${JSON.stringify(starts)}`,
    );
  },

  'a calendar range that ends at or before its start, or spans more than 64 days, is refused at query.to': async (
    subject,
  ) => {
    const token = await accessToken(await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword));
    const from = Date.parse(emptyWeek.from);

    const reversed = await subject.transport.send(
      calendarRequest(subject, token, { from: emptyWeek.to, to: emptyWeek.from }),
    );
    const empty = await subject.transport.send(
      calendarRequest(subject, token, { from: emptyWeek.from, to: emptyWeek.from }),
    );
    const longest = await subject.transport.send(
      calendarRequest(subject, token, {
        from: emptyWeek.from,
        to: new Date(from + 64 * dayMilliseconds).toISOString(),
      }),
    );
    const overlong = await subject.transport.send(
      calendarRequest(subject, token, {
        from: emptyWeek.from,
        to: new Date(from + 64 * dayMilliseconds + 1_000).toISOString(),
      }),
    );

    check(reversed.status === 422, `expected 422 for a reversed range, got ${String(reversed.status)}`);
    check((await fieldProblems(reversed)) === 'validation_failed query.to:invalid', 'expected query.to invalid');
    check(empty.status === 422, `expected 422 for an empty range, got ${String(empty.status)}`);
    check((await fieldProblems(empty)) === 'validation_failed query.to:invalid', 'expected query.to invalid');
    check(longest.status === 200, `expected 200 for a 64-day range, got ${String(longest.status)}`);
    check(overlong.status === 422, `expected 422 for an overlong range, got ${String(overlong.status)}`);
    check((await fieldProblems(overlong)) === 'validation_failed query.to:too_long', 'expected query.to too_long');
  },

  'a tutor filter on a tutor’s calendar is refused as not allowed': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword));

    const read = await subject.transport.send(
      calendarRequest(subject, token, { ...emptyWeek, tutor: '00000000-0000-4000-8000-000000000099' }),
    );

    check(read.status === 422, `expected 422, got ${String(read.status)}`);
    const problems = await fieldProblems(read);
    check(
      problems === 'validation_failed query.tutor:not_allowed',
      `expected query.tutor not_allowed, got ${problems}`,
    );
  },

  'a tutor without notifications reads an empty page with no unread rows': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword));

    const read = await subject.transport.send(notificationsRequest(subject, token));

    check(read.status === 200, `expected 200, got ${String(read.status)}`);
    const body: unknown = await read.json();
    check(
      typeof body === 'object' &&
        body !== null &&
        Object.keys(body).sort().join(',') === 'items,next_cursor,unread_count' &&
        'items' in body &&
        Array.isArray(body.items) &&
        body.items.length === 0 &&
        'next_cursor' in body &&
        body.next_cursor === null &&
        'unread_count' in body &&
        body.unread_count === 0,
      `expected exactly {items: [], next_cursor: null, unread_count: 0}, got ${JSON.stringify(body)}`,
    );
  },

  'a student reading the join requests is refused as on an unknown route with not_found': async (subject) => {
    const { email, password } = await subject.student();
    const student = await accessToken(await subject.transport.login(email, password));

    const read = await refusalShape(await subject.transport.send(joinRequestsRequest(subject, student)));
    const unknown = await refusalShape(
      await subject.transport.send(
        new Request(`${subject.origin}/api/v1/no-such-route`, { headers: { Authorization: `Bearer ${student}` } }),
      ),
    );

    check(unknown.status === 404 && unknown.code === 'not_found', 'expected not_found on an unknown route');
    check(read.same(unknown), `expected the read to be refused as an unknown route: ${read.describe()}`);
  },

  'the link of a new card shows its tutor without a session': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    const created = cardFields(
      await (await subject.transport.send(cardRequest(subject, token, '', { display_name: 'Invited student' }))).json(),
    );
    const invitationToken =
      typeof created.url === 'string' ? (new URL(created.url).pathname.split('/').at(-1) ?? '') : '';
    const me: unknown = await (await subject.transport.me(token, timeout())).json();
    const tutorName = typeof me === 'object' && me !== null && 'name' in me ? me.name : undefined;

    const page = await subject.transport.send(
      new Request(`${subject.origin}/api/v1/invitations/${encodeURIComponent(invitationToken)}`),
    );

    check(page.status === 200, `expected 200, got ${String(page.status)}`);
    const body: unknown = await page.json();
    const tutor = typeof body === 'object' && body !== null && 'tutor' in body ? body.tutor : undefined;
    check(
      typeof tutor === 'object' &&
        tutor !== null &&
        'name' in tutor &&
        tutor.name === tutorName &&
        'subjects' in tutor &&
        Array.isArray(tutor.subjects),
      'expected the tutor of the card',
    );
  },

  'an unknown invitation token answers not_found without a session': async (subject) => {
    const response = await subject.transport.send(
      new Request(`${subject.origin}/api/v1/invitations/an-unknown-invitation-token`),
    );

    check(response.status === 404, `expected 404, got ${String(response.status)}`);
    check((await errorCode(response)) === 'not_found', 'expected not_found');
  },

  'an unsubscribe link previews its group and turns it off only on POST, and a tampered link is token_invalid': async (
    subject,
  ) => {
    const unsubscribeToken = await subject.unsubscribeToken();
    const access = await accessToken(await subject.transport.login(subject.email, subject.password));
    const before = await emailGroups(await subject.transport.send(emailPreferencesRequest(subject, access)));

    const preview = await subject.transport.send(unsubscribeRequest(subject, 'GET', unsubscribeToken));
    const afterPreview = await emailGroups(await subject.transport.send(emailPreferencesRequest(subject, access)));
    const applied = await subject.transport.send(unsubscribeRequest(subject, 'POST', unsubscribeToken));
    const afterPost = await emailGroups(await subject.transport.send(emailPreferencesRequest(subject, access)));
    const saved = await emailGroups(
      await subject.transport.send(
        emailPreferencesRequest(subject, access, { groups: [{ group: 'lesson_reminders', enabled: true }] }),
      ),
    );
    const tampered = await subject.transport.send(unsubscribeRequest(subject, 'GET', `${unsubscribeToken}x`));

    const allOn =
      '{"groups":[{"group":"lesson_reminders","enabled":true},{"group":"submissions","enabled":true},{"group":"message_digest","enabled":true}]}';
    check(before === allOn, `expected the tutor's three groups on, got ${before}`);
    check(preview.status === 200, `expected 200, got ${String(preview.status)}`);
    check(JSON.stringify(await preview.json()) === '{"group":"lesson_reminders"}', 'expected exactly the group');
    check(afterPreview === allOn, `expected the preview to change nothing, got ${afterPreview}`);
    check(applied.status === 204, `expected 204, got ${String(applied.status)}`);
    check(
      afterPost.includes('{"group":"lesson_reminders","enabled":false}'),
      `expected lesson reminders off, got ${afterPost}`,
    );
    check(saved === allOn, `expected the save to answer the full list, got ${saved}`);
    check(tampered.status === 422, `expected 422, got ${String(tampered.status)}`);
    check((await errorCode(tampered)) === 'token_invalid', 'expected token_invalid');
  },

  'a refresh without a session answers 204 with no token': async (subject) => {
    const response = await subject.transport.refresh(timeout());

    check(response.status === 204, `expected 204, got ${String(response.status)}`);
  },

  'a wrong password is refused as invalid credentials': async (subject) => {
    const response = await subject.transport.login(subject.email, `${subject.password} wrong`);

    check(response.status === 401, `expected 401, got ${String(response.status)}`);
    check((await errorCode(response)) === 'invalid_credentials', 'expected invalid_credentials');
  },

  'sign-in answers a token that reads the account': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const me = await subject.transport.me(token, timeout());

    check(me.status === 200, `expected 200, got ${String(me.status)}`);
    const body: unknown = await me.json();
    check(
      typeof body === 'object' && body !== null && 'email' in body && body.email === subject.email,
      'expected the account',
    );
  },

  'a refresh after sign-in answers a working token': async (subject) => {
    await accessToken(await subject.transport.login(subject.email, subject.password));

    const refreshed = await accessToken(await subject.transport.refresh(timeout()));

    check((await subject.transport.me(refreshed, timeout())).status === 200, 'expected the refreshed token to work');
  },

  'an expired token is refused as token_expired and a refresh renews it': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    await subject.expireAccessTokens();

    const expired = await subject.transport.me(token, timeout());
    const renewed = await accessToken(await subject.transport.refresh(timeout()));

    check(expired.status === 401, `expected 401, got ${String(expired.status)}`);
    check((await errorCode(expired)) === 'token_expired', 'expected token_expired');
    check((await subject.transport.me(renewed, timeout())).status === 200, 'expected the renewed token to work');
  },

  'a second sign-in in the same browser replaces the refresh cookie': async (subject) => {
    const earlier = await accessToken(await subject.transport.login(subject.email, subject.password));
    const later = await accessToken(await subject.transport.login(subject.email, subject.password));

    const refreshed = await accessToken(await subject.transport.refresh(timeout()));
    const signedOut = await subject.transport.send(logoutRequest(subject, refreshed));

    check(signedOut.status === 204, `expected 204, got ${String(signedOut.status)}`);
    check(
      (await subject.transport.me(later, timeout())).status === 401,
      'expected the later session to be the one refreshed',
    );
    check((await subject.transport.me(earlier, timeout())).status === 200, 'expected the earlier session to stay');
  },

  'a blocked account loses its session and is refused as account_blocked after the correct password': async (
    subject,
  ) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    await subject.blockAccount();

    const reused = await subject.transport.me(token, timeout());
    check(reused.status === 401, `expected the session to be revoked, got ${String(reused.status)}`);
    const refreshed = await subject.transport.refresh(timeout());
    check(refreshed.status === 401, `expected the revoked session to be refused, got ${String(refreshed.status)}`);
    const wrong = await subject.transport.login(subject.email, `${subject.password} wrong`);
    check((await errorCode(wrong)) === 'invalid_credentials', 'expected a wrong password to stay invalid_credentials');
    const right = await subject.transport.login(subject.email, subject.password);
    check(right.status === 403, `expected 403, got ${String(right.status)}`);
    check((await errorCode(right)) === 'account_blocked', 'expected account_blocked');
  },

  'sign-out ends the session for its token and its refresh cookie': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const signedOut = await subject.transport.send(logoutRequest(subject, token));

    check(signedOut.status === 204, `expected 204, got ${String(signedOut.status)}`);
    const reused = await subject.transport.me(token, timeout());
    check(reused.status === 401, `expected 401, got ${String(reused.status)}`);
    check((await errorCode(reused)) === 'unauthenticated', 'expected unauthenticated');
    check((await subject.transport.refresh(timeout())).status === 204, 'expected no session to refresh');
  },
  'abort after refresh headers rejects the unread response body and preserves the rotated cookie': async (subject) => {
    await accessToken(await subject.transport.login(subject.email, subject.password));
    const controller = new AbortController();

    const response = await subject.transport.refresh(controller.signal);
    check(response.status === 200, 'expected refresh headers before abort');
    controller.abort();
    let bodyFailed = false;
    try {
      await response.arrayBuffer();
    } catch {
      bodyFailed = true;
    }
    const next = await subject.transport.refresh(timeout());

    check(bodyFailed, 'expected the aborted unread response body to fail');
    check(next.status === 200, 'expected the already rotated cookie to remain usable');
    await accessToken(next);
  },
  'a student reads their own submission without answer keys, points or scores': async (subject) => {
    const { seeded, token } = await studentSubmission(subject);

    const response = await subject.transport.send(submissionRequest(subject, token, seeded.submissionId));

    check(response.status === 200, `expected the student's submission, got ${String(response.status)}`);
    const body: unknown = await response.json();
    const keys = keysOf(body);
    check(keys.includes('editable_question_ids'), 'expected the student schema with editable_question_ids');
    const topLevel = typeof body === 'object' && body !== null ? Object.keys(body).sort() : [];
    check(
      JSON.stringify(topLevel) === JSON.stringify([...studentSubmissionFields].sort()),
      `expected exactly the unrevealed student schema fields, got ${topLevel.join(', ')}`,
    );
    const leaked = answerKeyFields.filter((field) => keys.includes(field));
    check(leaked.length === 0, `expected no answer key, points or scores, found ${leaked.join(', ')}`);
  },
  'the tutor who issued a submission reads it in the tutor schema': async (subject) => {
    const seeded = await subject.seedSubmission();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const response = await subject.transport.send(submissionRequest(subject, token, seeded.submissionId));

    check(response.status === 200, `expected the tutor's submission, got ${String(response.status)}`);
    const body: unknown = await response.json();
    const topLevel = typeof body === 'object' && body !== null ? Object.keys(body) : [];
    check(
      ['id', 'answer_policy', 'revisions', 'max_points', 'student'].every((field) => topLevel.includes(field)),
      `expected the tutor schema, got ${topLevel.join(', ')}`,
    );
    check(!topLevel.includes('editable_question_ids'), 'expected no student-only field in the tutor schema');
  },
  "a submission outside the student's work is refused as not_found on read and save": async (subject) => {
    const { token } = await studentSubmission(subject);
    const other = await subject.seedOtherStudentSubmission();

    const read = await subject.transport.send(submissionRequest(subject, token, randomSubmissionId));
    const save = await subject.transport.send(answersRequest(subject, token, randomSubmissionId, 1, []));
    const foreignRead = await subject.transport.send(submissionRequest(subject, token, other.submissionId));
    const foreignSave = await subject.transport.send(answersRequest(subject, token, other.submissionId, 1, []));

    check(read.status === 404 && (await errorCode(read)) === 'not_found', 'expected a read out of scope refused');
    check(save.status === 404 && (await errorCode(save)) === 'not_found', 'expected a save out of scope refused');
    check(
      foreignRead.status === 404 && (await errorCode(foreignRead)) === 'not_found',
      `expected a read of another student's submission refused, got ${String(foreignRead.status)}`,
    );
    check(
      foreignSave.status === 404 && (await errorCode(foreignSave)) === 'not_found',
      `expected a save of another student's submission refused, got ${String(foreignSave.status)}`,
    );
  },
  'a saved answer advances the version and a save at the old version is refused as conflict': async (subject) => {
    const { seeded, token } = await studentSubmission(subject);
    const version = await currentVersion(subject, token, seeded);
    const answer = [{ question_id: seeded.textQuestionId, value: { text: 'went' } }];

    const saved = await subject.transport.send(answersRequest(subject, token, seeded.submissionId, version, answer));
    const stale = await subject.transport.send(answersRequest(subject, token, seeded.submissionId, version, answer));

    check(saved.status === 200, `expected the answer saved, got ${String(saved.status)}`);
    check(versionOf(await saved.json()) > version, 'expected the saved submission at a later version');
    check(stale.status === 409 && (await errorCode(stale)) === 'conflict', 'expected the stale save refused');
  },
  'answers outside the question limits are refused as validation_failed naming the field': async (subject) => {
    const { seeded, token } = await studentSubmission(subject);
    const version = await currentVersion(subject, token, seeded);
    const tooLong = [{ question_id: seeded.textQuestionId, value: { text: 'a'.repeat(501) } }];
    const unknown = [{ question_id: randomSubmissionId, value: { text: 'went' } }];

    const long = await subject.transport.send(answersRequest(subject, token, seeded.submissionId, version, tooLong));
    const stranger = await subject.transport.send(
      answersRequest(subject, token, seeded.submissionId, version, unknown),
    );

    const longProblems = await fieldProblems(long);
    const strangerProblems = await fieldProblems(stranger);
    check(
      long.status === 422 && longProblems === 'validation_failed body.answers[0].value.text:too_long',
      `expected a too long text refused, got ${String(long.status)} ${longProblems}`,
    );
    check(
      stranger.status === 422 && strangerProblems === 'validation_failed body.answers[0].question_id:invalid',
      `expected an unknown question refused, got ${String(stranger.status)} ${strangerProblems}`,
    );
  },
  "binding an answer file over the tutor's full quota is refused as quota_exceeded and binds nothing": async (
    subject,
  ) => {
    const { seeded, token } = await studentSubmission(subject);
    const uploaded = await subject.transport.send(answerUploadRequest(subject, token, seeded));
    check(uploaded.status === 201, `expected the answer file uploaded, got ${String(uploaded.status)}`);
    const uploadBody: unknown = await uploaded.json();
    const attachmentId =
      typeof uploadBody === 'object' && uploadBody !== null && 'id' in uploadBody && typeof uploadBody.id === 'string'
        ? uploadBody.id
        : '';
    const version = await currentVersion(subject, token, seeded);
    await subject.fillTutorFileQuota();

    const bound = await subject.transport.send(
      answersRequest(subject, token, seeded.submissionId, version, [
        { question_id: seeded.fileQuestionId, value: { attachment_ids: [attachmentId] } },
      ]),
    );

    check(bound.status === 507, `expected the binding refused over the quota, got ${String(bound.status)}`);
    check((await errorCode(bound)) === 'quota_exceeded', 'expected quota_exceeded');
    const read = await subject.transport.send(submissionRequest(subject, token, seeded.submissionId));
    const after: unknown = await read.json();
    check(versionOf(after) === version, 'expected the refused binding to leave the version');
    check(fileAnswerCount(after, seeded.fileQuestionId) === 0, 'expected the refused binding to bind no file');
  },
  'an automatic answer of returned work is refused as answer_frozen after the first submission': async (subject) => {
    const { seeded, token } = await studentSubmission(subject);
    const text = [{ question_id: seeded.textQuestionId, value: { text: 'went' } }];
    const saved = await subject.transport.send(
      answersRequest(subject, token, seeded.submissionId, await currentVersion(subject, token, seeded), text),
    );
    check(saved.status === 200, `expected the answer saved, got ${String(saved.status)}`);
    const savedVersion = versionOf(await saved.json());
    const submitted = await subject.transport.send(
      submissionActionRequest(subject, token, seeded.submissionId, 'submit', { version: savedVersion }),
    );
    check(submitted.status === 200, `expected the work submitted, got ${String(submitted.status)}`);
    const submittedBody = await bodyOf(submitted);
    await subject.passSeconds(1);

    const whileSubmitted = await subject.transport.send(
      answersRequest(subject, token, seeded.submissionId, versionOf(submittedBody), text),
    );
    await subject.returnSeededSubmission(seeded);
    const returnedVersion = await currentVersion(subject, token, seeded);
    await subject.passSeconds(1);
    const frozen = await subject.transport.send(
      answersRequest(subject, token, seeded.submissionId, returnedVersion, text),
    );
    await subject.passSeconds(1);
    const manual = await subject.transport.send(
      answersRequest(subject, token, seeded.submissionId, returnedVersion, [
        { question_id: seeded.fileQuestionId, value: { attachment_ids: [] } },
      ]),
    );

    check(property(submittedBody, 'status') === 'submitted', 'expected work with a file question to wait for checking');
    check(
      whileSubmitted.status === 409 && (await errorCode(whileSubmitted)) === 'not_editable',
      `expected a save of submitted work refused as not_editable, got ${String(whileSubmitted.status)}`,
    );
    check(
      frozen.status === 409 && (await errorCode(frozen)) === 'answer_frozen',
      `expected a save of an automatic answer of returned work refused as answer_frozen, got ${String(frozen.status)}`,
    );
    check(manual.status === 200, `expected the manual answer of returned work saved, got ${String(manual.status)}`);
  },
  "a save on an ended relationship's submission is refused as not_editable": async (subject) => {
    const { seeded, token } = await studentSubmission(subject);
    await subject.endSeededRelationship(seeded);
    const version = await currentVersion(subject, token, seeded);

    const save = await subject.transport.send(
      answersRequest(subject, token, seeded.submissionId, version, [
        { question_id: seeded.textQuestionId, value: { text: 'went' } },
      ]),
    );

    check(
      save.status === 409 && (await errorCode(save)) === 'not_editable',
      `expected the save refused as not_editable, got ${String(save.status)}`,
    );
  },
  'marking checked feedback as seen clears feedback_new at the current version only and only for the own submission':
    async (subject) => {
      const seeded = await subject.seedCheckedSubmission();
      const other = await subject.seedOtherStudentSubmission();
      const token = await accessToken(await subject.transport.login(seeded.email, seeded.password));
      const version = await currentVersion(subject, token, seeded);
      const before = feedbackNewOf(
        await bodyOf(await subject.transport.send(homeRequest(subject, token))),
        seeded.submissionId,
      );

      const stale = await subject.transport.send(
        submissionActionRequest(subject, token, seeded.submissionId, 'feedback-seen', { version: version + 1 }),
      );
      const malformed = await subject.transport.send(
        submissionActionRequest(subject, token, seeded.submissionId, 'feedback-seen', { version: -1 }),
      );
      const foreign = await subject.transport.send(
        submissionActionRequest(subject, token, other.submissionId, 'feedback-seen', { version: 1 }),
      );
      const seen = await subject.transport.send(
        submissionActionRequest(subject, token, seeded.submissionId, 'feedback-seen', { version }),
      );
      const repeated = await subject.transport.send(
        submissionActionRequest(subject, token, seeded.submissionId, 'feedback-seen', { version }),
      );
      const after = feedbackNewOf(
        await bodyOf(await subject.transport.send(homeRequest(subject, token))),
        seeded.submissionId,
      );

      const malformedProblems = await fieldProblems(malformed);
      check(before === true, `expected the checked feedback new on the home page, got ${String(before)}`);
      check(
        stale.status === 409 && (await errorCode(stale)) === 'conflict',
        `expected a stale version refused as conflict, got ${String(stale.status)}`,
      );
      check(
        malformed.status === 422 && malformedProblems === 'validation_failed body.version:invalid',
        `expected a negative version refused at body.version, got ${String(malformed.status)} ${malformedProblems}`,
      );
      check(
        foreign.status === 404 && (await errorCode(foreign)) === 'not_found',
        `expected another student's submission refused as not_found, got ${String(foreign.status)}`,
      );
      check(seen.status === 204, `expected 204, got ${String(seen.status)}`);
      check(repeated.status === 204, `expected a repeated mark to answer 204, got ${String(repeated.status)}`);
      check(after === false, `expected the feedback no longer new on the home page, got ${String(after)}`);
    },
  'the tutor home holds exactly the tutor parts and lists an issue due within three days': async (subject) => {
    const seeded = await subject.seedUpcomingDeadline();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const read = await subject.transport.send(homeRequest(subject, token));

    check(read.status === 200, `expected the tutor home, got ${String(read.status)}`);
    const body = await bodyOf(read);
    checkKeys(body, tutorHomeKeys, 'the tutor home');
    const deadlines = property(body, 'upcoming_deadlines');
    const items = listOf(property(deadlines, 'items'));
    check(
      property(deadlines, 'count') === 1 && items.length === 1 && property(items[0], 'id') === seeded.issueId,
      `expected the seeded issue as the only upcoming deadline, got ${JSON.stringify(deadlines)}`,
    );
    checkEachKeys(items, issueItemKeys, 'an upcoming deadline');
  },
  'the student home holds exactly the student parts without points, and a tutor filter outside the current tutors is not_found':
    async (subject) => {
      const seeded = await subject.seedStudentHome();
      const student = await accessToken(await subject.transport.login(seeded.email, seeded.password));
      const tutor = await accessToken(await subject.transport.login(subject.email, subject.password));

      const read = await subject.transport.send(homeRequest(subject, student));
      const own = await subject.transport.send(homeRequest(subject, student, { tutor: seeded.relationshipId }));
      const random = await refusalShape(
        await subject.transport.send(homeRequest(subject, student, { tutor: randomRelationshipId })),
      );
      const refusals = {
        "another student's relationship": seeded.otherStudentRelationshipId,
        'an ended relationship': seeded.endedRelationshipId,
        'a malformed value': 'not-a-uuid',
      };
      const refused = await Promise.all(
        Object.entries(refusals).map(async ([name, value]) => ({
          name,
          shape: await refusalShape(await subject.transport.send(homeRequest(subject, student, { tutor: value }))),
        })),
      );
      const fromTutor = await subject.transport.send(homeRequest(subject, tutor, { tutor: seeded.relationshipId }));

      check(read.status === 200, `expected the student home, got ${String(read.status)}`);
      const body = await bodyOf(read);
      checkKeys(body, studentHomeKeys, 'the student home');
      const todo = property(body, 'todo');
      checkKeys(todo, ['count', 'items'], 'the to-do part');
      const todoItems = listOf(property(todo, 'items'));
      const checked = listOf(property(body, 'recently_checked'));
      const week = listOf(property(body, 'next_seven_days'));
      const tutors = listOf(property(body, 'tutors'));
      check(
        todoItems.length > 0 && checked.length > 0 && week.length > 0 && tutors.length === 1,
        `expected seeded work, lessons and one current tutor, got ${JSON.stringify(body)}`,
      );
      checkAssignmentItems(todoItems, 'a to-do item');
      checkAssignmentItems(checked, 'a recently checked item');
      checkCalendarItems([property(body, 'next_lesson')], 'the next lesson');
      checkCalendarItems(week, 'a lesson of the next seven days');
      checkEachKeys(tutors, homeTutorKeys, 'a current tutor');
      check(property(tutors[0], 'relationship_id') === seeded.relationshipId, 'expected the current tutor');
      const leaked = answerKeyFields.filter((field) => keysOf(body).includes(field));
      check(leaked.length === 0, `expected no answer key, points or scores, found ${leaked.join(', ')}`);
      check(own.status === 200, `expected the own tutor filter to answer 200, got ${String(own.status)}`);
      check(random.status === 404 && random.code === 'not_found', `expected not_found, got ${random.describe()}`);
      for (const { name, shape } of refused) {
        check(shape.same(random), `expected ${name} refused as a random identifier: ${shape.describe()}`);
      }
      check(fromTutor.status === 422, `expected a tutor filter from a tutor refused, got ${String(fromTutor.status)}`);
      const tutorProblems = await fieldProblems(fromTutor);
      check(
        tutorProblems === 'validation_failed query.tutor:not_allowed',
        `expected query.tutor not_allowed, got ${tutorProblems}`,
      );
    },
  'dismissing the checklist is final and shows on the settings and the tutor home, and every other body names the field':
    async (subject) => {
      const token = await accessToken(await subject.transport.login(subject.email, subject.password));
      const before = property(
        await bodyOf(await subject.transport.send(tutorSettingsRequest(subject, token))),
        'onboarding_dismissed',
      );
      const bodies = [
        { body: { onboarding_dismissed: false }, expected: 'validation_failed body.onboarding_dismissed:invalid' },
        { body: { onboarding_dismissed: null }, expected: 'validation_failed body.onboarding_dismissed:invalid' },
        { body: {}, expected: 'validation_failed body.onboarding_dismissed:required' },
        {
          body: { onboarding_dismissed: true, recording_retention_days: 30 },
          expected: 'validation_failed body:unexpected_property',
        },
      ];

      const refused: { readonly status: number; readonly problems: string; readonly expected: string }[] = [];
      for (const { body, expected } of bodies) {
        const response = await subject.transport.send(tutorSettingsRequest(subject, token, body));
        refused.push({ status: response.status, problems: await fieldProblems(response), expected });
      }
      const dismissal = { onboarding_dismissed: true };
      const dismissed = await subject.transport.send(tutorSettingsRequest(subject, token, dismissal));
      const repeated = await subject.transport.send(tutorSettingsRequest(subject, token, dismissal));
      const settings = await bodyOf(await subject.transport.send(tutorSettingsRequest(subject, token)));
      const home = await bodyOf(await subject.transport.send(homeRequest(subject, token)));

      check(before === false, `expected a new tutor's checklist shown, got ${String(before)}`);
      for (const { status, problems, expected } of refused) {
        check(status === 422 && problems === expected, `expected ${expected}, got ${String(status)} ${problems}`);
      }
      check(dismissed.status === 200, `expected the dismissal to answer 200, got ${String(dismissed.status)}`);
      const dismissedBody = await bodyOf(dismissed);
      checkKeys(dismissedBody, tutorSettingsKeys, 'the tutor settings');
      check(property(dismissedBody, 'onboarding_dismissed') === true, 'expected the dismissal answered as dismissed');
      check(repeated.status === 200, `expected a repeated dismissal to answer 200, got ${String(repeated.status)}`);
      check(property(settings, 'onboarding_dismissed') === true, 'expected the settings to show the dismissal');
      check(
        property(property(home, 'checklist'), 'onboarding_dismissed') === true,
        `expected the tutor home to show the dismissal, got ${JSON.stringify(property(home, 'checklist'))}`,
      );
    },
  'an unconfirmed tutor home has no parts the API refuses to an unconfirmed tutor': async (subject) => {
    const { email, password } = await subject.unconfirmedTutor();
    const token = await accessToken(await subject.transport.login(email, password));

    const read = await subject.transport.send(homeRequest(subject, token));

    check(read.status === 200, `expected the unconfirmed tutor home, got ${String(read.status)}`);
    const body = await bodyOf(read);
    checkKeys(body, tutorHomeKeys, 'the tutor home');
    check(
      property(body, 'needs_checking') === null &&
        property(body, 'upcoming_deadlines') === null &&
        property(body, 'join_requests') === null &&
        property(property(property(body, 'checklist'), 'steps'), 'student') === null,
      `expected the confirmed-only parts null, got ${JSON.stringify(body)}`,
    );
    check(
      property(property(body, 'tutor_code'), 'active') === false,
      'expected the tutor code inactive while the email is unconfirmed',
    );
  },
  "open_assignments counts the calling tutor's open work for a shared student and never another tutor's": async (
    subject,
  ) => {
    const seeded = await subject.seedSharedStudentIssues();
    const own = await accessToken(await subject.transport.login(subject.email, subject.password));
    const other = await accessToken(await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword));

    const ownRow = await studentListRow(subject, own, seeded.cardId);
    const otherRow = await studentListRow(subject, other, seeded.otherTutorCardId);

    check(
      property(ownRow, 'open_assignments') === 1,
      `expected the tutor's one open submission, got ${JSON.stringify(ownRow)}`,
    );
    check(
      property(otherRow, 'open_assignments') === 2,
      `expected the other tutor's two open submissions, got ${JSON.stringify(otherRow)}`,
    );
  },
  'account me and send retain abort ownership after headers and release cancelled readers': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    for (const method of ['me', 'send'] as const) {
      const controller = new AbortController();
      const response =
        method === 'me'
          ? await subject.transport.me(token, controller.signal)
          : await subject.transport.send(new Request(meRequest(subject, token), { signal: controller.signal }));
      controller.abort();
      const reader = response.body?.getReader();
      check(reader !== undefined, 'expected an account response body');
      let failed = false;
      let completed = false;
      let bytes = 0;
      try {
        while (!failed && !completed) {
          const outcome = await reader?.read().then(
            (part) => ({ kind: 'read' as const, part }),
            () => ({ kind: 'failed' as const }),
          );
          check(outcome !== undefined, 'expected an account body read outcome');
          if (outcome?.kind === 'failed') failed = true;
          else if (outcome?.kind === 'read') {
            completed = outcome.part.done;
            if (!outcome.part.done) bytes += outcome.part.value.byteLength;
            check(bytes <= 64 * 1024, 'account body exceeded the observation limit');
          }
        }
      } finally {
        reader?.releaseLock();
      }
      check(failed, 'expected account body abort to reject reading');
      check(response.body?.locked === false, 'expected aborted account reader release');
      const next =
        method === 'me'
          ? await subject.transport.me(token, timeout())
          : await subject.transport.send(meRequest(subject, token));
      const cancellationReader = next.body?.getReader();
      check(cancellationReader !== undefined, 'expected a cancellable account response body');
      try {
        await cancellationReader?.cancel();
      } finally {
        cancellationReader?.releaseLock();
      }
      check(next.body?.locked === false, 'expected cancelled account reader release');
      const wholeController = new AbortController();
      const whole =
        method === 'me'
          ? await subject.transport.me(token, wholeController.signal)
          : await subject.transport.send(new Request(meRequest(subject, token), { signal: wholeController.signal }));
      wholeController.abort();
      const wholeRead = await whole.arrayBuffer().then(
        () => 'read',
        () => 'failed',
      );
      check(wholeRead === 'failed', 'expected account body abort to reject a whole read');
    }
  },

  'a new account reads no current export': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const response = await subject.transport.send(exportRequest(subject, token, 'GET', '/current'));

    check(response.status === 200, `expected 200, got ${String(response.status)}`);
    const body = await bodyOf(response);
    checkKeys(body, ['export', 'next_allowed_at'], 'the current export');
    check(property(body, 'export') === null, 'expected no export');
    check(property(body, 'next_allowed_at') === null, 'expected no limit');
  },

  'an export request with a wrong password is refused at body.password and the right password queues it': async (
    subject,
  ) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const wrong = await requestExport(subject, token, wrongPassword(subject));
    const right = await requestExport(subject, token, subject.password);
    const current = await currentExport(subject, token);

    check(wrong.status === 422, `expected 422, got ${String(wrong.status)}`);
    const problems = await fieldProblems(wrong);
    check(problems === 'validation_failed body.password:incorrect', `expected an incorrect password, got ${problems}`);
    check(right.status === 202, `expected 202, got ${String(right.status)}`);
    const requested = await bodyOf(right);
    checkKeys(requested, exportKeys, 'the requested export');
    check(property(requested, 'state') === 'queued', 'expected the export queued');
    const shown = property(current, 'export');
    check(property(shown, 'id') === property(requested, 'id'), 'expected the requested export to be current');
    check(
      ['queued', 'building', 'ready'].includes(String(property(shown, 'state'))),
      `expected the export queued, building or ready, got ${String(property(shown, 'state'))}`,
    );
  },

  'the sixth wrong password for an export request is refused as rate_limited with Retry-After': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    const refusals: number[] = [];
    for (let attempt = 0; attempt < 5; attempt += 1) {
      refusals.push((await requestExport(subject, token, wrongPassword(subject))).status);
    }

    const sixth = await requestExport(subject, token, wrongPassword(subject));

    check(refusals.join(',') === '422,422,422,422,422', `expected five 422, got ${refusals.join(',')}`);
    check(sixth.status === 429, `expected 429, got ${String(sixth.status)}`);
    checkRetryAfter(sixth);
    check((await errorCode(sixth)) === 'rate_limited', 'expected rate_limited');
  },

  'deleting the current export is idempotent and removes a requested export': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    const nothing = await subject.transport.send(exportRequest(subject, token, 'DELETE', '/current'));
    const requested = await requestExport(subject, token, subject.password);

    const deleted = await subject.transport.send(exportRequest(subject, token, 'DELETE', '/current'));

    check(nothing.status === 204, `expected 204 without an export, got ${String(nothing.status)}`);
    check(requested.status === 202, `expected 202, got ${String(requested.status)}`);
    check(deleted.status === 204, `expected 204, got ${String(deleted.status)}`);
    check(property(await currentExport(subject, token), 'export') === null, 'expected no export after the delete');
  },

  'the download mint is not found without an export and after a delete': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    const withoutExport = await subject.transport.send(exportRequest(subject, token, 'POST', '/current/download'));
    await requestExport(subject, token, subject.password);
    await subject.transport.send(exportRequest(subject, token, 'DELETE', '/current'));

    const afterDelete = await subject.transport.send(exportRequest(subject, token, 'POST', '/current/download'));

    check(withoutExport.status === 404, `expected 404 without an export, got ${String(withoutExport.status)}`);
    check((await errorCode(withoutExport)) === 'not_found', 'expected not_found without an export');
    check(afterDelete.status === 404, `expected 404 after the delete, got ${String(afterDelete.status)}`);
    check((await errorCode(afterDelete)) === 'not_found', 'expected not_found after the delete');
  },

  'a new export request within 24 hours of a ready export is refused as rate_limited and the limit is shown': async (
    subject,
  ) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    const first = await requestExport(subject, token, subject.password);
    await subject.readyExport(token);

    const second = await requestExport(subject, token, subject.password);

    check(first.status === 202, `expected 202, got ${String(first.status)}`);
    check(second.status === 429, `expected 429, got ${String(second.status)}`);
    checkRetryAfter(second);
    check((await errorCode(second)) === 'rate_limited', 'expected rate_limited');
    const current = await currentExport(subject, token);
    check(property(property(current, 'export'), 'state') === 'ready', 'expected the export to stay ready');
    check(typeof property(current, 'next_allowed_at') === 'string', 'expected the instant a new export may start');
  },
  'a group detail carries its members, archive state and the lesson counts of the archive confirmation': async (
    subject,
  ) => {
    const seeded = await subject.seedGroupWithMember();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const read = await callContract(subject, token, 'GET', groupPath(seeded));

    check(read.status === 200, `expected 200, got ${String(read.status)}`);
    checkKeys(read.body, groupDetailKeys, 'the group detail');
    checkEachKeys(listOf(property(read.body, 'members')), groupMemberKeys, 'a group member');
    check(memberIdsOf(read) === onlyMember(seeded), `expected the seeded member, got ${memberIdsOf(read)}`);
    check(property(read.body, 'archived_at') === null, 'expected an unarchived group');
    check(property(read.body, 'future_lesson_count') === 0, 'expected no future lesson');
    check(property(read.body, 'has_open_series') === false, 'expected no open series');
    check(listOf(property(read.body, 'next_lessons')).length === 0, 'expected no next lesson');
  },

  'a group with a future lesson counts it until archiving cancels it': async (subject) => {
    const seeded = await subject.seedGroupWithFutureLesson();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const before = await callContract(subject, token, 'GET', groupPath(seeded));
    const archived = await callContract(subject, token, 'POST', groupPath(seeded, '/archive'));
    const after = await callContract(subject, token, 'GET', groupPath(seeded));

    check(before.status === 200, `expected 200, got ${String(before.status)}`);
    check(property(before.body, 'future_lesson_count') === 1, 'expected one future lesson');
    check(property(before.body, 'has_open_series') === false, 'expected no open series');
    const nextLessons = listOf(property(before.body, 'next_lessons'));
    check(nextLessons.length === 1, `expected one next lesson, got ${String(nextLessons.length)}`);
    checkCalendarItems(nextLessons, 'a next group lesson');
    check(archived.status === 200, `expected 200 for the archive, got ${String(archived.status)}`);
    checkKeys(archived.body, groupKeys, 'the archived group');
    check(typeof property(archived.body, 'archived_at') === 'string', 'expected the archive instant');
    check(property(after.body, 'future_lesson_count') === 0, 'expected the future lesson cancelled');
    check(listOf(property(after.body, 'next_lessons')).length === 0, 'expected no next lesson after the archive');
  },

  'removing a member and adding it back restores exactly that member, and a repeated add changes nothing': async (
    subject,
  ) => {
    const seeded = await subject.seedGroupWithMember();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    const member = [seeded.relationshipId];

    const removed = await callContract(subject, token, 'PATCH', groupPath(seeded), { remove: member });
    const restored = await callContract(subject, token, 'PATCH', groupPath(seeded), { add: member });
    const repeated = await callContract(subject, token, 'PATCH', groupPath(seeded), { add: member });
    const read = await callContract(subject, token, 'GET', groupPath(seeded));

    check(removed.status === 200, `expected 200 for the removal, got ${String(removed.status)}`);
    check(memberIdsOf(removed) === '[]', `expected no member after the removal, got ${memberIdsOf(removed)}`);
    check(restored.status === 200, `expected 200 for the add, got ${String(restored.status)}`);
    checkKeys(restored.body, groupKeys, 'the changed group');
    check(memberIdsOf(restored) === onlyMember(seeded), `expected the member back, got ${memberIdsOf(restored)}`);
    check(repeated.status === 200, `expected 200 for the repeated add, got ${String(repeated.status)}`);
    check(memberIdsOf(repeated) === onlyMember(seeded), `expected one member, got ${memberIdsOf(repeated)}`);
    check(memberIdsOf(read) === onlyMember(seeded), `expected the member stored, got ${memberIdsOf(read)}`);
  },

  'ending a relationship removes the member and adding it back is refused as relationship_not_active': async (
    subject,
  ) => {
    const seeded = await subject.seedGroupWithMember();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    await subject.endGroupMemberRelationship(seeded);

    const afterEnding = await callContract(subject, token, 'GET', groupPath(seeded));
    const added = await callContract(subject, token, 'PATCH', groupPath(seeded), { add: [seeded.relationshipId] });
    const read = await callContract(subject, token, 'GET', groupPath(seeded));

    check(
      memberIdsOf(afterEnding) === '[]',
      `expected the ending to remove the member, got ${memberIdsOf(afterEnding)}`,
    );
    check(added.status === 409, `expected 409, got ${String(added.status)}`);
    check(refusalCode(added) === 'relationship_not_active', 'expected relationship_not_active');
    const refused = JSON.stringify(property(property(property(added.body, 'error'), 'details'), 'relationship_ids'));
    check(refused === onlyMember(seeded), `expected the ended relationship in the details, got ${refused}`);
    check(memberIdsOf(read) === '[]', `expected no member added, got ${memberIdsOf(read)}`);
  },

  'adding an unknown relationship is refused as not_found and adds nobody': async (subject) => {
    const seeded = await subject.seedGroupWithMember();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    const outsideScope = '00000000-0000-4000-8000-0000000000ff';

    const added = await callContract(subject, token, 'PATCH', groupPath(seeded), { add: [outsideScope] });
    const read = await callContract(subject, token, 'GET', groupPath(seeded));

    check(added.status === 404, `expected 404, got ${String(added.status)}`);
    check(refusalCode(added) === 'not_found', 'expected not_found');
    check(memberIdsOf(read) === onlyMember(seeded), `expected only the seeded member, got ${memberIdsOf(read)}`);
  },

  'an archived group refuses new members as audience_not_active and still removes members': async (subject) => {
    const seeded = await subject.seedGroupWithMember();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    const member = [seeded.relationshipId];
    const archived = await callContract(subject, token, 'POST', groupPath(seeded, '/archive'));

    const refused = await callContract(subject, token, 'PATCH', groupPath(seeded), { add: member });
    const removed = await callContract(subject, token, 'PATCH', groupPath(seeded), { remove: member });
    const refusedAgain = await callContract(subject, token, 'PATCH', groupPath(seeded), { add: member });

    check(archived.status === 200, `expected 200 for the archive, got ${String(archived.status)}`);
    check(refused.status === 409, `expected 409, got ${String(refused.status)}`);
    check(refusalCode(refused) === 'audience_not_active', 'expected audience_not_active');
    check(removed.status === 200, `expected 200 for the removal, got ${String(removed.status)}`);
    check(memberIdsOf(removed) === '[]', `expected no member after the removal, got ${memberIdsOf(removed)}`);
    check(typeof property(removed.body, 'archived_at') === 'string', 'expected the group to stay archived');
    check(refusedAgain.status === 409, `expected 409 again, got ${String(refusedAgain.status)}`);
    check(refusalCode(refusedAgain) === 'audience_not_active', 'expected audience_not_active again');
  },

  'archiving and unarchiving are idempotent and keep the members': async (subject) => {
    const seeded = await subject.seedGroupWithMember();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const first = await callContract(subject, token, 'POST', groupPath(seeded, '/archive'));
    const second = await callContract(subject, token, 'POST', groupPath(seeded, '/archive'));
    const unarchived = await callContract(subject, token, 'POST', groupPath(seeded, '/unarchive'));
    const unarchivedAgain = await callContract(subject, token, 'POST', groupPath(seeded, '/unarchive'));

    const statuses = [first, second, unarchived, unarchivedAgain].map((answer) => answer.status).join(',');
    check(statuses === '200,200,200,200', `expected four 200, got ${statuses}`);
    const archivedAt = property(first.body, 'archived_at');
    check(typeof archivedAt === 'string', 'expected the archive instant');
    check(property(second.body, 'archived_at') === archivedAt, 'expected a repeated archive to keep the first instant');
    check(property(unarchived.body, 'archived_at') === null, 'expected the group unarchived');
    check(property(unarchivedAgain.body, 'archived_at') === null, 'expected the group to stay unarchived');
    check(
      memberIdsOf(unarchivedAgain) === onlyMember(seeded),
      `expected the member kept, got ${memberIdsOf(unarchivedAgain)}`,
    );
  },

  "another tutor's group is not found for every group route and stays unchanged": async (subject) => {
    const seeded = await subject.seedGroupWithMember();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    const other = await accessToken(await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword));
    const before = await callContract(subject, token, 'GET', groupPath(seeded));

    const answers = [
      await callContract(subject, other, 'GET', groupPath(seeded)),
      await callContract(subject, other, 'PATCH', groupPath(seeded), { name: 'Taken over' }),
      await callContract(subject, other, 'POST', groupPath(seeded, '/archive')),
      await callContract(subject, other, 'POST', groupPath(seeded, '/unarchive')),
    ];
    const after = await callContract(subject, token, 'GET', groupPath(seeded));

    const statuses = answers.map((answer) => answer.status).join(',');
    check(statuses === '404,404,404,404', `expected four 404, got ${statuses}`);
    check(
      answers.every((answer) => refusalCode(answer) === 'not_found'),
      'expected not_found on every route',
    );
    check(property(after.body, 'name') === property(before.body, 'name'), 'expected the name unchanged');
    check(property(after.body, 'archived_at') === null, 'expected the group unarchived');
    check(memberIdsOf(after) === onlyMember(seeded), `expected the member kept, got ${memberIdsOf(after)}`);
  },

  'moving a template into a folder and back increments its version, the listings follow it and a stale version conflicts':
    async (subject) => {
      const seeded = await subject.seedLibraryFolderAndTemplate();
      const token = await accessToken(await subject.transport.login(subject.email, subject.password));
      const version = seeded.templateVersion;

      const intoFolder = await callContract(subject, token, 'PATCH', templatePath(seeded), {
        folder_id: seeded.folderId,
        version,
      });
      const folderListing = await callContract(subject, token, 'GET', folderListingPath(seeded.folderId));
      const stale = await callContract(subject, token, 'PATCH', templatePath(seeded), { folder_id: null, version });
      const back = await callContract(subject, token, 'PATCH', templatePath(seeded), {
        folder_id: null,
        version: version + 1,
      });
      const rootListing = await callContract(subject, token, 'GET', '/library');
      const folderAfter = await callContract(subject, token, 'GET', folderListingPath(seeded.folderId));

      check(intoFolder.status === 200, `expected 200 for the move, got ${String(intoFolder.status)}`);
      check(property(intoFolder.body, 'folder_id') === seeded.folderId, 'expected the template in the folder');
      check(property(intoFolder.body, 'version') === version + 1, 'expected the move to increment the version');
      check(folderListing.status === 200, `expected 200 for the folder listing, got ${String(folderListing.status)}`);
      const listed = listOf(property(folderListing.body, 'items')).find(
        (item) => property(item, 'id') === seeded.templateId,
      );
      check(property(listed, 'kind') === 'template', 'expected the template listed in the folder');
      check(property(listed, 'version') === version + 1, 'expected the listing to show the new version');
      check(stale.status === 409, `expected 409 for a stale version, got ${String(stale.status)}`);
      check(refusalCode(stale) === 'conflict', 'expected conflict');
      check(back.status === 200, `expected 200 for the move back, got ${String(back.status)}`);
      check(property(back.body, 'folder_id') === null, 'expected the template back in the root');
      check(property(back.body, 'version') === version + 2, 'expected the move back to increment the version');
      check(
        idsOf(property(rootListing.body, 'items')).includes(seeded.templateId),
        'expected the template listed in the root',
      );
      check(
        !idsOf(property(folderAfter.body, 'items')).includes(seeded.templateId),
        'expected the template gone from the folder',
      );
    },

  "a template move is not found for another tutor and for a folder outside the caller's library": async (subject) => {
    const seeded = await subject.seedLibraryFolderAndTemplate();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    const other = await accessToken(await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword));
    const version = seeded.templateVersion;

    const outsider = await callContract(subject, other, 'PATCH', templatePath(seeded), { folder_id: null, version });
    const unknownFolder = await callContract(subject, token, 'PATCH', templatePath(seeded), {
      folder_id: randomFolderId,
      version,
    });
    const read = await callContract(subject, token, 'GET', templatePath(seeded));

    check(outsider.status === 404, `expected 404 for another tutor, got ${String(outsider.status)}`);
    check(refusalCode(outsider) === 'not_found', 'expected not_found for another tutor');
    check(unknownFolder.status === 404, `expected 404 for an unknown folder, got ${String(unknownFolder.status)}`);
    check(refusalCode(unknownFolder) === 'not_found', 'expected not_found for an unknown folder');
    check(property(read.body, 'version') === version, 'expected the version unchanged');
    check(property(read.body, 'folder_id') === null, 'expected the template to stay in the root');
  },

  'moving a folder into the root lists it there, and a move into its own subfolder is refused': async (subject) => {
    const seeded = await subject.seedLibraryFolderAndTemplate();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    const folderPath = (id: string): string => `/library/folders/${encodeURIComponent(id)}`;

    const intoOwnChild = await callContract(subject, token, 'PATCH', folderPath(seeded.folderId), {
      parent_id: seeded.childFolderId,
    });
    const toRoot = await callContract(subject, token, 'PATCH', folderPath(seeded.childFolderId), { parent_id: null });
    const folders = await callContract(subject, token, 'GET', '/library/folders');
    const rootListing = await callContract(subject, token, 'GET', '/library');
    const formerParent = await callContract(subject, token, 'GET', folderListingPath(seeded.folderId));

    check(intoOwnChild.status === 422, `expected 422, got ${String(intoOwnChild.status)}`);
    const problems = problemsOf(intoOwnChild);
    check(problems === 'validation_failed body.parent_id:invalid', `expected an invalid parent, got ${problems}`);
    check(toRoot.status === 200, `expected 200 for the move, got ${String(toRoot.status)}`);
    check(property(toRoot.body, 'id') === seeded.childFolderId, 'expected the moved folder');
    check(property(toRoot.body, 'parent_id') === null, 'expected the folder in the root');
    const moved = listOf(property(folders.body, 'folders')).find(
      (folder) => property(folder, 'id') === seeded.childFolderId,
    );
    check(property(moved, 'parent_id') === null, 'expected the folder list to show the folder in the root');
    const rootFolders = idsOf(property(rootListing.body, 'folders'));
    check(
      rootFolders.includes(seeded.childFolderId) && rootFolders.includes(seeded.folderId),
      'expected both folders listed in the root',
    );
    check(
      !idsOf(property(formerParent.body, 'folders')).includes(seeded.childFolderId),
      'expected the folder gone from its former parent',
    );
  },
};
