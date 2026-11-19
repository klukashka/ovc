import { FakeChatServer } from './fakeChat.ts';
import { type FeedbackSeenRequest, FakeSubmissionServer, type FieldProblem, fakeRefusal } from './fakeSubmissions.ts';

import type { components } from './schema.ts';
import type { LockManagerLike, Me, Scheduler } from './session.ts';
import type { AuthTransport, UploadTransfer } from './transport.ts';

const accessTokenSeconds = 900;

interface FakeAccount {
  readonly me: Me;
  readonly password: string;
  blocked: boolean;
}

interface FakeSession {
  readonly id: number;
  readonly accountId: string;
  generation: number;
  revoked: boolean;
}

interface FakeLesson {
  readonly detail: LessonDetail;
  readonly members: readonly string[];
}

interface FakeLessonRefusal {
  readonly status: number;
  readonly code: string;
  readonly retryAfterSeconds: number | null;
}

interface FakeAccessToken {
  readonly sessionId: number;
  readonly issuedAt: number;
}

interface RefreshCookie {
  readonly sessionId: number;
  readonly generation: number;
}

type StudentCard = components['schemas']['StudentCard'];
type EmailGroup = components['schemas']['EmailPreference']['group'];
type EmailPreference = components['schemas']['EmailPreference'];
type CalendarItem = components['schemas']['CalendarItem'];
type LessonDetail = components['schemas']['LessonDetail'];
type Notification = components['schemas']['Notification'];
type StudentCardSummary = components['schemas']['StudentCardSummary'];
type IssueItem = components['schemas']['IssueItem'];
type MyTutor = components['schemas']['MyTutor'];
type ReviewQueueItem = components['schemas']['ReviewQueueItem'];
type StudentAssignmentItem = components['schemas']['StudentAssignmentItem'];
type StudentHome = components['schemas']['StudentHome'];
type StudentHomeTutor = components['schemas']['StudentHomeTutor'];
type TutorCode = components['schemas']['TutorCode'];
type TutorHome = components['schemas']['TutorHome'];
type TutorJoinRequest = components['schemas']['TutorJoinRequest'];
type TutorSettings = components['schemas']['TutorSettings'];
type CurrentTutor = MyTutor & { readonly state: 'active' | 'provisional' };

interface HomeFilter {
  readonly relationshipId: string | null;
}

const cardPathPrefix = '/api/v1/students/';
const calendarPath = '/api/v1/calendar';
const issuesPath = '/api/v1/issues';
const templatePathPrefix = '/api/v1/library/templates/';
const lessonPathPrefix = '/api/v1/lessons/';
const joinRequestsPath = '/api/v1/join-requests';
const notificationsPath = '/api/v1/notifications';
const tutorSettingsPath = '/api/v1/tutor/settings';
const tutorCodePath = '/api/v1/tutor/code';
const homePath = '/api/v1/home';
const submissionPathPrefix = '/api/v1/submissions/';
const invitationPathPrefix = '/api/v1/invitations/';
const fakeTokenPrefix = 'fake-';
const emailPreferencesPath = '/api/v1/users/me/email-preferences';
const unsubscribePath = '/api/v1/unsubscribe';
const unsubscribePreviewPath = '/api/v1/unsubscribe/preview';

const emailGroupsOf: Readonly<Record<Me['role'], readonly EmailGroup[]>> = {
  tutor: ['lesson_reminders', 'submissions', 'message_digest'],
  student: ['lesson_reminders', 'lesson_changes', 'homework', 'results', 'message_digest'],
};

interface UnsubscribeGrant {
  readonly accountId: string;
  readonly group: EmailGroup;
}

function isEmailPreference(value: unknown): value is EmailPreference {
  return (
    typeof value === 'object' &&
    value !== null &&
    'group' in value &&
    typeof value.group === 'string' &&
    'enabled' in value &&
    typeof value.enabled === 'boolean'
  );
}

function requestedPreferences(body: unknown): EmailPreference[] | null {
  if (typeof body !== 'object' || body === null || !('groups' in body) || !Array.isArray(body.groups)) {
    return null;
  }
  const groups: unknown[] = body.groups;
  return groups.every(isEmailPreference) ? groups : null;
}

interface FakeCard {
  readonly tutorId: string;
  readonly card: StudentCard;
}

function fakeCard(id: string, displayName: string): StudentCard {
  return {
    id,
    state: 'invited',
    display_name: displayName,
    email: null,
    account: null,
    groups: [],
    notes: '',
    invitation: {
      url: `https://ovc.example/invite/${fakeTokenPrefix}${id}`,
      expires_at: '2026-11-07T15:00:00Z',
      status: 'open',
    },
    linked_at: null,
    ended_at: null,
    ended_by: null,
    created_at: '2026-10-24T15:00:00Z',
  };
}

export interface FakeAccountSeed {
  readonly id: string;
  readonly name: string;
  readonly email: string;
  readonly password: string;
  readonly role?: Me['role'];
  readonly state?: Me['state'];
  readonly bio?: string;
}

const homeworkUploadContexts: ReadonlySet<string> = new Set(['answer', 'corrected']);

const defaultPageLimit = 30;
const maximumPageLimit = 100;
const unreadCountCap = 100;
const cursorPrefix = 'fake-cursor-';
const supportedStudentListParameters: ReadonlySet<string> = new Set(['limit', 'cursor', 'state']);
const studentListStates: Readonly<Record<string, ReadonlySet<StudentCard['state']>>> = {
  active: new Set(['active', 'provisional', 'reclaim_pending']),
  invited: new Set(['invited']),
  ended: new Set(['ended']),
};
const homeListLimit = 50;
const homePanelLimit = 5;
const homeCountCap = 100;
const homeLookaheadDays = 63;
const hourMilliseconds = 60 * 60 * 1_000;
const homeDeadlineWindowMilliseconds = 72 * hourMilliseconds;
const homeWeekMilliseconds = 7 * 24 * hourMilliseconds;
const defaultTutorCode = 'K7M2-QX4P';
const defaultTutorSettings: TutorSettings = {
  file_quota_bytes: 1_073_741_824,
  file_usage_bytes: 0,
  onboarding_dismissed: false,
};
const todoStatuses: ReadonlySet<StudentAssignmentItem['status']> = new Set(['not_started', 'draft', 'returned']);
const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

interface KeysetPage<Item> {
  readonly items: Item[];
  readonly next_cursor: string | null;
}

const decimalDigits = /^\d+$/;

function decimalOrNull(text: string): number | null {
  return decimalDigits.test(text) ? Number(text) : null;
}

function pageOf<Item>(items: readonly Item[], query: URLSearchParams): KeysetPage<Item> | null {
  const limitText = query.get('limit');
  const limit = limitText === null ? defaultPageLimit : decimalOrNull(limitText);
  const cursor = query.get('cursor');
  const offset =
    cursor === null ? 0 : cursor.startsWith(cursorPrefix) ? decimalOrNull(cursor.slice(cursorPrefix.length)) : null;
  if (limit === null || offset === null || limit < 1 || limit > maximumPageLimit) {
    return null;
  }
  const end = offset + limit;
  return {
    items: items.slice(offset, end),
    next_cursor: end < items.length ? `${cursorPrefix}${String(end)}` : null,
  };
}

function zoneOffset(instant: number, timeZone: string): number {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    hourCycle: 'h23',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).formatToParts(new Date(instant));
  const part = (type: Intl.DateTimeFormatPartTypes): number =>
    Number(parts.find((candidate) => candidate.type === type)?.value ?? '0');
  const local = Date.UTC(part('year'), part('month') - 1, part('day'), part('hour'), part('minute'), part('second'));
  return local - Math.floor(instant / 1_000) * 1_000;
}

function localDayStart(instant: number, timeZone: string, daysAhead: number): number {
  const local = new Date(instant + zoneOffset(instant, timeZone));
  const midnight = Date.UTC(local.getUTCFullYear(), local.getUTCMonth(), local.getUTCDate() + daysAhead);
  return midnight - zoneOffset(midnight - zoneOffset(instant, timeZone), timeZone);
}

function startsWithin(from: number, to: number): (item: CalendarItem) => boolean {
  return (item) => Date.parse(item.starts_at) >= from && Date.parse(item.starts_at) < to;
}

function byStart(items: readonly CalendarItem[]): CalendarItem[] {
  return [...items].sort(
    (left, right) => Date.parse(left.starts_at) - Date.parse(right.starts_at) || left.id.localeCompare(right.id),
  );
}

function instantOrLast(value: string | null): number {
  return value === null ? Number.POSITIVE_INFINITY : Date.parse(value);
}

function todoOrder(left: StudentAssignmentItem, right: StudentAssignmentItem): number {
  return (
    instantOrLast(left.deadline_at) - instantOrLast(right.deadline_at) ||
    Date.parse(left.issued_at) - Date.parse(right.issued_at) ||
    left.submission_id.localeCompare(right.submission_id)
  );
}

function checkedOrder(left: StudentAssignmentItem, right: StudentAssignmentItem): number {
  return (
    Date.parse(right.checked_at ?? '') - Date.parse(left.checked_at ?? '') ||
    right.submission_id.localeCompare(left.submission_id)
  );
}

function queueOrder(left: ReviewQueueItem, right: ReviewQueueItem): number {
  return (
    instantOrLast(left.first_submitted_at) - instantOrLast(right.first_submitted_at) ||
    left.submission_id.localeCompare(right.submission_id)
  );
}

function issueListItem(issue: IssueItem): IssueItem {
  const item: IssueItem = { ...issue };
  delete item.submission;
  return item;
}

function isCurrentTutor(tutor: MyTutor): tutor is CurrentTutor {
  return tutor.state === 'active' || tutor.state === 'provisional';
}

function homeTutor(tutor: CurrentTutor): StudentHomeTutor {
  return {
    relationship_id: tutor.relationship_id,
    state: tutor.state,
    name: tutor.tutor.name,
    avatar_id: tutor.tutor.avatar_id,
    colour: tutor.colour,
    subjects: [...tutor.tutor.subjects],
    channel_id: tutor.channel_id,
  };
}

function homeFilter(current: readonly CurrentTutor[], values: readonly string[]): HomeFilter | null {
  if (values.length === 0) {
    return { relationshipId: null };
  }
  const [value = ''] = values;
  if (values.length !== 1 || !uuidPattern.test(value)) {
    return null;
  }
  const found = current.find((tutor) => tutor.relationship_id.toLowerCase() === value.toLowerCase());
  return found === undefined ? null : { relationshipId: found.relationship_id };
}

function settingsPatchRefusal(text: string): Response | null {
  let body: unknown;
  try {
    body = JSON.parse(text);
  } catch {
    return refusal(400, 'bad_request');
  }
  if (typeof body !== 'object' || body === null || Array.isArray(body)) {
    return fieldRefusal('body', 'invalid');
  }
  const flag: unknown = 'onboarding_dismissed' in body ? body.onboarding_dismissed : undefined;
  const problems: FieldProblem[] = Object.keys(body)
    .filter((key) => key !== 'onboarding_dismissed')
    .map(() => ({ path: 'body', code: 'unexpected_property' }));
  if (flag !== undefined && flag !== null && typeof flag !== 'boolean') {
    problems.unshift({ path: 'body.onboarding_dismissed', code: 'invalid' });
  }
  if (problems.length > 0) {
    return fakeRefusal(422, 'validation_failed', problems);
  }
  if (!('onboarding_dismissed' in body)) {
    return fieldRefusal('body.onboarding_dismissed', 'required');
  }
  return flag === true ? null : fieldRefusal('body.onboarding_dismissed', 'invalid');
}

function summaryOf(card: StudentCard, openAssignments: number): StudentCardSummary {
  return {
    id: card.id,
    state: card.state,
    display_name: card.display_name,
    email: card.email,
    account: card.account,
    groups: card.groups,
    linked_at: card.linked_at,
    ended_at: card.ended_at,
    ended_by: card.ended_by,
    created_at: card.created_at,
    next_lesson: null,
    open_assignments: openAssignments,
  };
}

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function refusal(status: number, code: string): Response {
  return json(status, { error: { code, message: code, request_id: 'fake-request' } });
}

function refusalRetryingAfter(status: number, code: string, seconds: number): Response {
  return new Response(JSON.stringify({ error: { code, message: code, request_id: 'fake-request' } }), {
    status,
    headers: { 'Content-Type': 'application/json', 'Retry-After': String(seconds) },
  });
}

function fieldRefusal(path: string, code: string): Response {
  return json(422, {
    error: {
      code: 'validation_failed',
      message: 'validation_failed',
      request_id: 'fake-request',
      fields: [{ path, code }],
    },
  });
}

function fakeAudienceRefusal(method: string, path: string, caller: Me): Response | null {
  const tutorOnly =
    path === '/api/v1/students' ||
    path.startsWith(cardPathPrefix) ||
    path === joinRequestsPath ||
    path.startsWith('/api/v1/groups') ||
    path.startsWith('/api/v1/library') ||
    path.startsWith('/api/v1/tutor/') ||
    path === issuesPath;
  if ((tutorOnly && caller.role !== 'tutor') || (path === '/api/v1/tutors' && caller.role !== 'student')) {
    return refusal(404, 'not_found');
  }
  const unverifiedOK =
    path === '/api/v1/users/me' ||
    path === '/api/v1/auth/logout' ||
    path === emailPreferencesPath ||
    path === tutorSettingsPath ||
    (path.startsWith(submissionPathPrefix) && caller.role === 'student') ||
    (method === 'GET' &&
      (path === notificationsPath ||
        path === calendarPath ||
        path === '/api/v1/tutors' ||
        path === homePath ||
        path === tutorCodePath)) ||
    path.startsWith('/api/v1/library') ||
    (method === 'GET' &&
      (path === '/api/v1/channels' ||
        path === '/api/v1/channels/unread' ||
        /^\/api\/v1\/channels\/[^/]+(?:\/messages)?$/.test(path))) ||
    (method === 'POST' && /^\/api\/v1\/channels\/[^/]+\/(?:messages|read)$/.test(path)) ||
    ((method === 'PATCH' || method === 'DELETE') && /^\/api\/v1\/messages\/[^/]+$/.test(path));
  return caller.state === 'unverified' && !unverifiedOK ? refusal(403, 'email_not_verified') : null;
}

const calendarMaxRangeMilliseconds = (62 + 2) * 24 * 60 * 60 * 1_000;

async function nextTurn(): Promise<void> {
  await Promise.resolve();
}

export class SignalAwareRefreshBodies {
  private lastResponse: Response | null = null;
  private headerResume = (): void => undefined;
  private readonly headerGate: Promise<void>;
  private readonly cancellationFails: boolean;
  private readonly declaresLength: boolean;
  readonly headers: Promise<void>;
  readonly reading: Promise<void>;
  private markHeaders = (): void => undefined;
  private markReading = (): void => undefined;
  private resume = (): void => undefined;
  private readonly gate: Promise<void>;
  private failed = false;
  private readonly size: number | null;
  private active = 0;
  private completed = 0;
  private cancelled = 0;
  private aborted = 0;
  private readFailures = 0;

  constructor(
    options: {
      readonly hold?: boolean;
      readonly bytes?: number;
      readonly holdHeaders?: boolean;
      readonly cancellationFails?: boolean;
      readonly declaresLength?: boolean;
    } = {},
  ) {
    this.cancellationFails = options.cancellationFails ?? false;
    this.declaresLength = options.declaresLength ?? false;
    this.headerGate = new Promise((resolve) => {
      this.headerResume = resolve;
    });
    if (options.holdHeaders !== true) this.headerResume();
    this.size = options.bytes ?? null;
    if (this.size !== null && (!Number.isInteger(this.size) || this.size < 0 || this.size > 64 * 1024 + 1))
      throw new Error('fake_refresh_body_size_invalid');
    this.headers = new Promise((resolve) => {
      this.markHeaders = resolve;
    });
    this.reading = new Promise((resolve) => {
      this.markReading = resolve;
    });
    this.gate = new Promise((resolve) => {
      this.resume = resolve;
    });
    if (options.hold !== true) this.resume();
  }
  async dispose(): Promise<void> {
    this.releaseHeaders();
    this.release();
    if (this.lastResponse !== null && !this.lastResponse.bodyUsed) await this.lastResponse.body?.cancel();
  }
  releaseHeaders(): void {
    this.headerResume();
  }
  release(): void {
    this.resume();
  }
  fail(): void {
    this.failed = true;
    this.resume();
  }
  snapshot(): {
    readonly consumed: boolean;
    readonly active: number;
    readonly completed: number;
    readonly cancelled: number;
    readonly aborted: number;
    readonly readerLocked: boolean;
    readonly readFailures: number;
  } {
    return {
      consumed: this.lastResponse?.bodyUsed ?? false,
      active: this.active,
      completed: this.completed,
      cancelled: this.cancelled,
      aborted: this.aborted,
      readerLocked: this.lastResponse?.body?.locked ?? false,
      readFailures: this.readFailures,
    };
  }
  async response(original: Response, signal: AbortSignal): Promise<Response> {
    signal.throwIfAborted();
    if (original.body === null) {
      this.markHeaders();
      return original;
    }
    const content = this.size !== null ? new Uint8Array(this.size) : new Uint8Array(await original.arrayBuffer());
    signal.throwIfAborted();
    let offset = 0;
    let finished = false;
    let abort = (): void => undefined;
    const end = (): void => {
      if (!finished) {
        finished = true;
        this.active -= 1;
        signal.removeEventListener('abort', abort);
      }
    };
    this.active += 1;
    const body = new ReadableStream<Uint8Array>(
      {
        start: (controller): void => {
          abort = (): void => {
            if (!finished) {
              this.aborted += 1;
              end();
              this.resume();
              controller.error(new DOMException('fake refresh body aborted', 'AbortError'));
            }
          };
          signal.addEventListener('abort', abort, { once: true });
          if (signal.aborted) abort();
        },
        pull: async (controller): Promise<void> => {
          this.markReading();
          await this.gate;
          if (finished) return;
          if (this.failed) {
            this.readFailures += 1;
            end();
            controller.error(new Error('fake_refresh_body_read_failed'));
            return;
          }
          if (offset === content.byteLength) {
            this.completed += 1;
            end();
            controller.close();
            return;
          }
          const next = Math.min(offset + 4096, content.byteLength);
          controller.enqueue(content.subarray(offset, next));
          offset = next;
        },
        cancel: (): void => {
          this.cancelled += 1;
          end();
          this.resume();
          if (this.cancellationFails) throw new Error('fake_body_cancellation_failed');
        },
      },
      { highWaterMark: 0 },
    );
    const headers = new Headers(original.headers);
    if (this.declaresLength) headers.set('Content-Length', String(content.byteLength));
    const response = new Response(body, {
      status: original.status,
      statusText: original.statusText,
      headers,
    });
    this.lastResponse = response;
    this.markHeaders();
    await this.headerGate;
    return response;
  }
}

export class AccountResponseBodies {
  private pending: SignalAwareRefreshBodies | null = null;
  arm(body: SignalAwareRefreshBodies): void {
    if (this.pending !== null) throw new Error('account_body_already_armed');
    this.pending = body;
  }
  async response(original: Response, signal: AbortSignal): Promise<Response> {
    const body = this.pending;
    this.pending = null;
    return (body ?? new SignalAwareRefreshBodies()).response(original, signal);
  }
}

export class FakeAuthServer {
  private readonly accountBodies: AccountResponseBodies | null;
  private readonly refreshBodies: SignalAwareRefreshBodies | null;
  constructor(
    options: { readonly refreshBodies?: SignalAwareRefreshBodies; readonly accountBodies?: AccountResponseBodies } = {},
  ) {
    this.accountBodies = options.accountBodies ?? null;
    this.refreshBodies = options.refreshBodies ?? null;
  }
  readonly chat = new FakeChatServer();
  readonly submissions = new FakeSubmissionServer((studentId, submissionId) => {
    this.clearFeedbackNew(studentId, submissionId);
  });
  private readonly tutors = new Map<string, components['schemas']['MyTutor'][]>();
  private readonly templateCounts = new Map<string, number>();
  private readonly templates = new Map<string, Map<string, components['schemas']['Template']>>();
  private failingTemplateReads = 0;
  private readonly libraryUploadRefusals: Response[] = [];
  private readonly homeworkUploadRefusals: Response[] = [];
  private libraryUploads = 0;
  private readonly accounts = new Map<string, FakeAccount>();
  private readonly sessions = new Map<number, FakeSession>();
  private readonly tokens = new Map<string, FakeAccessToken>();
  private storedRefresh: RefreshCookie | null = null;
  private nowSeconds = 0;
  private readonly issueItems = new Map<string, components['schemas']['IssueItem'][]>();
  private readonly queueItems = new Map<string, ReviewQueueItem[]>();
  private readonly joinRequests = new Map<string, TutorJoinRequest[]>();
  private readonly tutorCodes = new Map<string, TutorCode>();
  private readonly assignments = new Map<string, StudentAssignmentItem[]>();
  private readonly lessonRelationships = new WeakMap<CalendarItem, string>();
  private readonly openAssignmentCounts = new Map<string, number>();
  private homeReadCount = 0;
  private readonly homeReadQueries: string[] = [];
  private failingHomeReads = 0;
  private homeReadGate: Promise<void> | null = null;
  private homeReadReceived: () => void = () => undefined;
  private readonly writtenSettings: string[] = [];
  private readonly settingsWriteRefusals: Response[] = [];
  private settingsWriteGate: Promise<void> | null = null;
  private settingsWriteReceived: () => void = () => undefined;
  private readonly lessonDetailReads = new Map<string, number>();
  private issued = 0;
  private refreshGate: Promise<void> | null = null;
  private refreshReceived: () => void = () => undefined;
  private grantedRefreshes = 0;
  private failingAccountChecks = 0;
  private failingAccountReads = 0;
  private failingRefreshes = 0;
  private lostRefreshes = 0;
  private accountReadGate: Promise<void> | null = null;
  private signOutGate: Promise<void> | null = null;
  private signOutReceived: () => void = () => undefined;
  private accountReadReceived: () => void = () => undefined;
  private joinRequestReadGate: Promise<void> | null = null;
  private joinRequestReadReceived: () => void = () => undefined;
  private failingJoinRequestReads = 0;
  private notificationReadsFailing = false;
  private readonly notifications = new Map<string, Notification[]>();
  private failingCalendarReads = 0;
  private failingStudentListReads = 0;
  private failingCardReads = 0;
  private failingCardWrites = 0;
  private cardWriteGate: Promise<void> | null = null;
  private cardWriteReceived: () => void = () => undefined;
  private readonly writtenCards: { readonly cardId: string; readonly body: string }[] = [];
  private refreshCount = 0;
  private signOutCount = 0;
  private reuse = false;
  private readonly sent: string[] = [];
  private accountWriteGate: Promise<void> | null = null;
  private accountWriteReceived: () => void = () => undefined;
  private readonly writtenAccounts: FakeAccountWrite[] = [];
  private answeredAccountWrites = 0;
  private readonly addresses: URL[] = [];
  private readonly calendarItems = new Map<string, CalendarItem[]>();
  private readonly tutorSettings = new Map<string, components['schemas']['TutorSettings']>();
  private readonly lessons = new Map<string, FakeLesson>();
  private lessonWriteGate: Promise<void> | null = null;
  private lessonWriteReceived: () => void = () => undefined;
  private refusedLessonWrite: FakeLessonRefusal | null = null;
  private roomGeneration = 0;
  private readonly embeddingRefused = new Set<string>();
  nextRoomAddress: string | null = null;
  private calendarReadGate: Promise<void> | null = null;
  private calendarReadReceived: () => void = () => undefined;
  private readonly cards = new Map<string, FakeCard>();
  private failingInvitationReads = 0;
  private failingPreferenceSaves = 0;
  private failingUnsubscribes = 0;
  private readonly disabledGroups = new Map<string, Set<EmailGroup>>();
  private readonly unsubscribeGrants = new Map<string, UnsubscribeGrant>();

  get refreshes(): number {
    return this.refreshCount;
  }

  get signOuts(): number {
    return this.signOutCount;
  }

  get refreshesGranted(): number {
    return this.grantedRefreshes;
  }

  get reuseDetected(): boolean {
    return this.reuse;
  }

  get sentTokens(): readonly string[] {
    return this.sent;
  }

  get sentAddresses(): readonly URL[] {
    return this.addresses;
  }

  get accountWrites(): readonly FakeAccountWrite[] {
    return this.writtenAccounts;
  }

  get accountWritesAnswered(): number {
    return this.answeredAccountWrites;
  }

  pauseAccountWrites(): PausedReads {
    let resume = (): void => undefined;
    let notifyReceived = (): void => undefined;
    this.accountWriteGate = new Promise((resolve) => {
      resume = (): void => {
        this.accountWriteGate = null;
        this.accountWriteReceived = (): void => undefined;
        resolve();
      };
    });
    const received = new Promise<void>((resolve) => {
      notifyReceived = resolve;
    });
    this.accountWriteReceived = notifyReceived;
    return { resume, received };
  }

  get cardWrites(): readonly string[] {
    return this.writtenCards.map((write) => write.body);
  }

  cardWritesTo(cardId: string): readonly string[] {
    return this.writtenCards.filter((write) => write.cardId === cardId).map((write) => write.body);
  }

  addAccount(seed: FakeAccountSeed): void {
    this.accounts.set(seed.email, {
      password: seed.password,
      blocked: false,
      me: {
        id: seed.id,
        name: seed.name,
        email: seed.email,
        email_verified: (seed.state ?? 'active') === 'active',
        timezone: 'Europe/Moscow',
        avatar_id: null,
        reclaim_pending: false,
        role: seed.role ?? 'tutor',
        state: seed.state ?? 'active',
        language: 'ru',
        theme: 'system',
        ...(seed.bio === undefined ? {} : { bio: seed.bio }),
      },
    });
  }

  seedTutors(email: string, tutors: components['schemas']['MyTutor'][]): void {
    this.tutors.set(this.account(email).me.id, tutors);
  }

  advance(seconds: number): void {
    this.nowSeconds += seconds;
  }

  pauseRefreshes(): PausedAnswers {
    let resume = (): void => undefined;
    let received = (): void => undefined;
    this.refreshGate = new Promise((resolve) => {
      resume = (): void => {
        this.refreshGate = null;
        resolve();
      };
    });
    const answered = new Promise<void>((resolve) => {
      received = resolve;
    });
    this.refreshReceived = received;
    return { resume, answered };
  }

  pauseSignOuts(): PausedAnswers {
    let resume = (): void => undefined;
    let received = (): void => undefined;
    this.signOutGate = new Promise((resolve) => {
      resume = (): void => {
        this.signOutGate = null;
        resolve();
      };
    });
    const answered = new Promise<void>((resolve) => {
      received = resolve;
    });
    this.signOutReceived = received;
    return { resume, answered };
  }

  pauseAccountReads(): PausedAnswers {
    let resume = (): void => undefined;
    let received = (): void => undefined;
    this.accountReadGate = new Promise((resolve) => {
      resume = (): void => {
        this.accountReadGate = null;
        resolve();
      };
    });
    const answered = new Promise<void>((resolve) => {
      received = resolve;
    });
    this.accountReadReceived = received;
    return { resume, answered };
  }

  pauseJoinRequestReads(): PausedAnswers {
    let resume = (): void => undefined;
    let received = (): void => undefined;
    this.joinRequestReadGate = new Promise((resolve) => {
      resume = (): void => {
        this.joinRequestReadGate = null;
        resolve();
      };
    });
    const answered = new Promise<void>((resolve) => {
      received = resolve;
    });
    this.joinRequestReadReceived = received;
    return { resume, answered };
  }

  setTutorSettings(email: string, settings: components['schemas']['TutorSettings']): void {
    this.tutorSettings.set(this.account(email).me.id, settings);
  }

  addCalendarItem(email: string, item: CalendarItem, relationshipId?: string): void {
    const accountId = this.account(email).me.id;
    this.calendarItems.set(accountId, [...(this.calendarItems.get(accountId) ?? []), item]);
    if (relationshipId !== undefined) {
      this.lessonRelationships.set(item, relationshipId);
    }
  }

  addQueueItem(tutorEmail: string, item: ReviewQueueItem): void {
    const accountId = this.account(tutorEmail).me.id;
    this.queueItems.set(accountId, [...(this.queueItems.get(accountId) ?? []), item]);
  }

  addJoinRequest(tutorEmail: string, request: TutorJoinRequest): void {
    const accountId = this.account(tutorEmail).me.id;
    this.joinRequests.set(accountId, [...(this.joinRequests.get(accountId) ?? []), request]);
  }

  setTutorCode(email: string, code: TutorCode): void {
    this.tutorCodes.set(this.account(email).me.id, code);
  }

  addAssignment(studentEmail: string, item: StudentAssignmentItem): void {
    const accountId = this.account(studentEmail).me.id;
    this.assignments.set(accountId, [...(this.assignments.get(accountId) ?? []), item]);
  }

  setOpenAssignments(cardId: string, count: number): void {
    this.openAssignmentCounts.set(cardId, count);
  }

  homeReads(): number {
    return this.homeReadCount;
  }

  homeQueries(): readonly string[] {
    return this.homeReadQueries;
  }

  failNextHomeRead(): void {
    this.failingHomeReads += 1;
  }

  pauseHomeReads(): PausedReads {
    let resume = (): void => undefined;
    let notifyReceived = (): void => undefined;
    this.homeReadGate = new Promise((resolve) => {
      resume = (): void => {
        this.homeReadGate = null;
        this.homeReadReceived = (): void => undefined;
        resolve();
      };
    });
    const received = new Promise<void>((resolve) => {
      notifyReceived = resolve;
    });
    this.homeReadReceived = notifyReceived;
    return { resume, received };
  }

  settingsWrites(): readonly string[] {
    return this.writtenSettings;
  }

  refuseNextSettingsWrite(status: number, code: string): void {
    this.settingsWriteRefusals.push(refusal(status, code));
  }

  pauseSettingsWrites(): PausedReads {
    let resume = (): void => undefined;
    let notifyReceived = (): void => undefined;
    this.settingsWriteGate = new Promise((resolve) => {
      resume = (): void => {
        this.settingsWriteGate = null;
        this.settingsWriteReceived = (): void => undefined;
        resolve();
      };
    });
    const received = new Promise<void>((resolve) => {
      notifyReceived = resolve;
    });
    this.settingsWriteReceived = notifyReceived;
    return { resume, received };
  }

  feedbackSeenRequests(): readonly FeedbackSeenRequest[] {
    return this.submissions.feedbackSeenRequests;
  }

  refuseNextFeedbackSeen(status: number, code: string): void {
    this.submissions.refuseNextFeedbackSeen(status, code);
  }

  returnSubmission(submissionId: string): void {
    this.submissions.returnSubmission(submissionId);
  }

  endRelationshipOf(submissionId: string): void {
    const ended = this.submissions.endRelationshipOf(submissionId);
    const tutors = this.tutors.get(ended.studentId);
    if (tutors === undefined) {
      return;
    }
    const endedAt = new Date(this.serverNow()).toISOString();
    this.tutors.set(
      ended.studentId,
      tutors.map((tutor): MyTutor =>
        tutor.relationship_id === ended.relationshipId
          ? { ...tutor, state: 'ended', ended_at: endedAt, ended_by: 'tutor' }
          : tutor,
      ),
    );
  }

  addLesson(detail: LessonDetail, emails: readonly string[]): void {
    this.lessons.set(detail.id, { detail, members: emails.map((email) => this.account(email).me.id) });
  }

  changeLesson(id: string, change: Partial<LessonDetail>): void {
    const lesson = this.lessons.get(id);
    if (lesson === undefined) {
      throw new Error('the fake server has no such lesson');
    }
    this.lessons.set(id, { ...lesson, detail: { ...lesson.detail, ...change, version: lesson.detail.version + 1 } });
  }

  refuseEmbedding(id: string): void {
    this.embeddingRefused.add(id);
  }

  refuseNextLessonWrite(status: number, code: string, retryAfterSeconds: number | null = null): void {
    this.refusedLessonWrite = { status, code, retryAfterSeconds };
  }

  pauseLessonWrites(): PausedReads {
    let resume = (): void => undefined;
    let notifyReceived = (): void => undefined;
    this.lessonWriteGate = new Promise((resolve) => {
      resume = (): void => {
        this.lessonWriteGate = null;
        this.lessonWriteReceived = (): void => undefined;
        resolve();
      };
    });
    const received = new Promise<void>((resolve) => {
      notifyReceived = resolve;
    });
    this.lessonWriteReceived = notifyReceived;
    return { resume, received };
  }

  pauseCalendarReads(): PausedReads {
    let resume = (): void => undefined;
    let notifyReceived = (): void => undefined;
    this.calendarReadGate = new Promise((resolve) => {
      resume = (): void => {
        this.calendarReadGate = null;
        this.calendarReadReceived = (): void => undefined;
        resolve();
      };
    });
    const received = new Promise<void>((resolve) => {
      notifyReceived = resolve;
    });
    this.calendarReadReceived = notifyReceived;
    return { resume, received };
  }

  failJoinRequestReads(count: number): void {
    this.failingJoinRequestReads += count;
  }

  failNotificationReads(failing: boolean): void {
    this.notificationReadsFailing = failing;
  }

  failNextCalendarRead(): void {
    this.failingCalendarReads += 1;
  }

  failNextStudentListRead(): void {
    this.failingStudentListReads += 1;
  }

  addIssue(email: string, item: components['schemas']['IssueItem']): void {
    const accountId = this.account(email).me.id;
    this.issueItems.set(accountId, [...(this.issueItems.get(accountId) ?? []), item]);
  }

  lessonReads(id: string): number {
    return this.lessonDetailReads.get(id) ?? 0;
  }

  serverNow(): number {
    return Date.UTC(2026, 10, 1) + this.nowSeconds * 1_000;
  }

  addTemplate(email: string, template: components['schemas']['Template']): void {
    const accountId = this.account(email).me.id;
    const owned = this.templates.get(accountId) ?? new Map<string, components['schemas']['Template']>();
    owned.set(template.id, template);
    this.templates.set(accountId, owned);
  }

  failNextTemplateRead(): void {
    this.failingTemplateReads += 1;
  }

  addTemplates(email: string, count: number): void {
    const accountId = this.account(email).me.id;
    this.templateCounts.set(accountId, (this.templateCounts.get(accountId) ?? 0) + count);
  }

  refuseNextLibraryUpload(status: number, code: string): void {
    this.libraryUploadRefusals.push(refusal(status, code));
  }

  refuseNextHomeworkUpload(status: number, code: string): void {
    this.homeworkUploadRefusals.push(refusal(status, code));
  }

  addSubmission(
    studentEmail: string,
    tutorEmail: string,
    submission: components['schemas']['StudentSubmission'],
  ): void {
    this.submissions.add(this.account(studentEmail).me.id, this.account(tutorEmail).me.id, submission);
  }

  addReviewedSubmission(tutorEmail: string, submission: components['schemas']['TutorSubmission']): void {
    this.submissions.addReviewed(this.account(tutorEmail).me.id, submission);
  }

  submissionReads(id: string): number {
    return this.submissions.readsOf(id);
  }

  refuseNextAnswerSave(status: number, code: string, fields: readonly FieldProblem[] = []): void {
    this.submissions.refuseNextSave(status, code, fields);
  }

  fillFileQuota(tutorEmail: string): void {
    this.submissions.fillQuota(this.account(tutorEmail).me.id);
  }

  refuseNextLibraryUploadAsTooLarge(fieldCode: string): void {
    this.libraryUploadRefusals.push(
      json(413, {
        error: {
          code: 'payload_too_large',
          message: 'payload_too_large',
          request_id: 'fake-request',
          fields: [{ path: 'body.file', code: fieldCode }],
        },
      }),
    );
  }

  addNotification(email: string, notification: Notification): void {
    const accountId = this.account(email).me.id;
    const newestFirst = [...(this.notifications.get(accountId) ?? []), notification].sort(
      (left, right) => Date.parse(right.created_at) - Date.parse(left.created_at) || right.id.localeCompare(left.id),
    );
    this.notifications.set(accountId, newestFirst);
  }

  failNextCardRead(): void {
    this.failingCardReads += 1;
  }

  failNextCardWrite(): void {
    this.failingCardWrites += 1;
  }

  pauseCardWrites(): PausedReads {
    let resume = (): void => undefined;
    let notifyReceived = (): void => undefined;
    this.cardWriteGate = new Promise((resolve) => {
      resume = (): void => {
        this.cardWriteGate = null;
        this.cardWriteReceived = (): void => undefined;
        resolve();
      };
    });
    const received = new Promise<void>((resolve) => {
      notifyReceived = resolve;
    });
    this.cardWriteReceived = notifyReceived;
    return { resume, received };
  }

  failNextAccountCheck(): void {
    this.failingAccountChecks += 1;
  }

  failNextAccountRead(): void {
    this.failingAccountReads += 1;
  }

  failNextInvitationRead(): void {
    this.failingInvitationReads += 1;
  }

  failNextPreferenceSave(): void {
    this.failingPreferenceSaves += 1;
  }

  failNextUnsubscribe(): void {
    this.failingUnsubscribes += 1;
  }

  unsubscribeToken(email: string, group: EmailGroup): string {
    const token = `${fakeTokenPrefix}unsubscribe-${String(this.unsubscribeGrants.size + 1)}`;
    this.unsubscribeGrants.set(token, { accountId: this.account(email).me.id, group });
    return token;
  }

  emailGroupEnabled(email: string, group: EmailGroup): boolean {
    return !(this.disabledGroups.get(this.account(email).me.id)?.has(group) ?? false);
  }

  failNextRefresh(): void {
    this.failingRefreshes += 1;
  }

  loseNextRefresh(): void {
    this.lostRefreshes += 1;
  }

  block(email: string): void {
    this.blockWithoutRevoking(email);
    this.revokeSessionsOf(email);
  }

  blockWithoutRevoking(email: string): void {
    this.account(email).blocked = true;
  }

  revokeSessionsOf(email: string): void {
    const account = this.account(email);
    for (const session of this.sessions.values()) {
      if (session.accountId === account.me.id) {
        session.revoked = true;
      }
    }
  }

  private account(email: string): FakeAccount {
    const account = this.accounts.get(email);
    if (account === undefined) {
      throw new Error('no fake account has this email');
    }
    return account;
  }

  liveSessionOf(token: string): { readonly accountId: string; readonly sessionId: number } | null {
    const access = this.tokens.get(token);
    const session = this.sessionOf(token);
    if (access === undefined || session === undefined || this.refusedToken(token) !== null) {
      return null;
    }
    return { accountId: session.accountId, sessionId: access.sessionId };
  }

  sessionIsLive(sessionId: number): boolean {
    const session = this.sessions.get(sessionId);
    return session !== undefined && !session.revoked;
  }

  issuedBy(token: string): string | undefined {
    const access = this.tokens.get(token);
    const session = access === undefined ? undefined : this.sessions.get(access.sessionId);
    return session?.accountId;
  }

  transport(): AuthTransport {
    return {
      login: (email, password) => this.login(email, password),
      refresh: (signal) => this.refresh(signal),
      me: (token, signal) => this.checkAccount(token, signal),
      send: (request, upload) => this.send(request, upload),
    };
  }

  private async login(email: string, password: string): Promise<Response> {
    await nextTurn();
    const account = this.accounts.get(email);
    if (account?.password !== password) {
      return refusal(401, 'invalid_credentials');
    }
    if (account.blocked) {
      return refusal(403, 'account_blocked');
    }
    const session: FakeSession = {
      id: this.sessions.size + 1,
      accountId: account.me.id,
      generation: 1,
      revoked: false,
    };
    this.sessions.set(session.id, session);
    this.storedRefresh = { sessionId: session.id, generation: 1 };
    return json(200, { access_token: this.issue(session.id), expires_in: accessTokenSeconds });
  }

  private async refresh(signal: AbortSignal): Promise<Response> {
    if (this.refreshBodies !== null) signal.throwIfAborted();
    const presented = this.storedRefresh;
    await nextTurn();
    const lost = this.lostRefreshes > 0;
    if (lost) {
      this.lostRefreshes -= 1;
    }
    const answer = this.failingRefreshes > 0 ? this.failRefresh() : this.rotate(lost ? null : presented);
    this.refreshReceived();
    await this.refreshGate;
    if (lost) {
      throw new TypeError('the connection was lost before the refresh answered');
    }
    return this.refreshBodies === null ? answer : this.refreshBodies.response(answer, signal);
  }

  private failRefresh(): Response {
    this.failingRefreshes -= 1;
    this.refreshCount += 1;
    return refusal(503, 'service_unavailable');
  }

  private rotate(presented: RefreshCookie | null): Response {
    this.refreshCount += 1;
    if (presented === null) {
      return new Response(null, { status: 204 });
    }
    const session = this.sessions.get(presented.sessionId);
    if (session === undefined || session.revoked || presented.generation !== session.generation) {
      if (session !== undefined && !session.revoked) {
        session.revoked = true;
        this.reuse = true;
      }
      this.storedRefresh = null;
      return refusal(401, 'unauthenticated');
    }
    if ([...this.accounts.values()].some((account) => account.me.id === session.accountId && account.blocked)) {
      return refusal(403, 'account_blocked');
    }
    session.generation += 1;
    this.grantedRefreshes += 1;
    this.storedRefresh = { sessionId: session.id, generation: session.generation };
    return json(200, { access_token: this.issue(session.id), expires_in: accessTokenSeconds });
  }

  private async checkAccount(token: string, signal: AbortSignal): Promise<Response> {
    await nextTurn();
    if (this.failingAccountChecks > 0) {
      this.failingAccountChecks -= 1;
      return refusal(503, 'service_unavailable');
    }
    const answer = this.answerMe(token);
    return this.accountBodies === null ? answer : this.accountBodies.response(answer, signal);
  }

  private async send(request: Request, upload?: UploadTransfer): Promise<Response> {
    this.chat.observeInitialReadInvocation(request.method, request.url, request.signal);
    await nextTurn();
    const token = request.headers.get('Authorization')?.replace(/^Bearer /, '') ?? '';
    this.sent.push(token);
    const address = new URL(request.url);
    this.addresses.push(address);
    const path = address.pathname;
    if (path === '/api/v1/users/me' && request.method === 'PATCH') {
      return this.writeAccount(token, request);
    }
    if (path === '/api/v1/users/me') {
      return this.readAccount(token, request.signal);
    }
    const accountId = this.sessionOf(token)?.accountId;
    const caller = [...this.accounts.values()].find((account) => account.me.id === accountId)?.me;
    const publicRead = request.method === 'GET' && path.startsWith(invitationPathPrefix);
    const publicUnsubscribe = path === unsubscribePreviewPath || path === unsubscribePath;
    if (!publicRead && !publicUnsubscribe) {
      const invalid = this.refusedToken(token);
      if (invalid !== null) {
        return invalid;
      }
    }
    if (!publicRead && !publicUnsubscribe && caller !== undefined) {
      const audienceRefusal = fakeAudienceRefusal(request.method, path, caller);
      if (audienceRefusal !== null) {
        return audienceRefusal;
      }
    }
    if (
      path === '/api/v1/uploads' &&
      request.method === 'POST' &&
      address.searchParams.get('context') === 'library' &&
      caller !== undefined
    ) {
      return this.uploadToLibrary(request, caller, upload);
    }
    if (request.method === 'GET' && path === '/api/v1/library' && caller !== undefined) {
      return json(200, {
        folder: null,
        path: [],
        folders: [],
        items: [],
        next_cursor: null,
        template_count: this.templateCounts.get(caller.id) ?? 0,
      });
    }
    if (request.method === 'GET' && path.startsWith(templatePathPrefix) && caller !== undefined) {
      return this.readTemplate(caller, decodeURIComponent(path.slice(templatePathPrefix.length)));
    }
    if (request.method === 'GET' && path === issuesPath && caller !== undefined) {
      return json(200, { items: this.issueItems.get(caller.id) ?? [], next_cursor: null });
    }
    if (request.method === 'GET' && path === homePath && caller !== undefined) {
      return this.readHome(caller, address);
    }
    if (path === tutorSettingsPath && caller !== undefined) {
      return this.answerTutorSettings(request, caller);
    }
    if (request.method === 'GET' && path === tutorCodePath && caller !== undefined) {
      return json(200, this.codeOf(caller));
    }
    if (path === '/api/v1/uploads' && request.method === 'POST' && caller !== undefined) {
      request.signal.throwIfAborted();
      const homeworkRefusal = homeworkUploadContexts.has(address.searchParams.get('context') ?? '')
        ? this.homeworkUploadRefusals.shift()
        : undefined;
      if (homeworkRefusal !== undefined) {
        return homeworkRefusal;
      }
      const body = upload?.body ?? (await request.formData());
      const file = body.get('file');
      if (upload !== undefined && file !== null && typeof file !== 'string') upload.progress(file.size, file.size);
      if (address.searchParams.get('context') === 'answer') {
        return this.submissions.upload(request, caller, body, Date.UTC(2026, 10, 1) + this.nowSeconds * 1_000);
      }
      return this.chat.upload(request, caller, body, Date.UTC(2026, 10, 1) + this.nowSeconds * 1_000);
    }
    if (path === '/api/v1/channels' || path.startsWith('/api/v1/channels/') || path.startsWith('/api/v1/messages/')) {
      const invalid = this.refusedToken(token);
      if (invalid !== null) {
        return invalid;
      }
      if (caller === undefined) {
        return refusal(401, 'unauthenticated');
      }
      return this.chat.handle(request, caller, Date.UTC(2026, 10, 1) + this.nowSeconds * 1_000);
    }
    if (this.submissions.handles(path)) {
      if (caller === undefined) {
        return refusal(401, 'unauthenticated');
      }
      return this.submissions.handle(request, caller);
    }
    if (path === '/api/v1/tutors' && request.method === 'GET') {
      const invalid = this.refusedToken(token);
      if (invalid !== null) {
        return invalid;
      }
      return json(200, { tutors: this.tutors.get(accountId ?? '') ?? [] });
    }
    if (path === '/api/v1/students' || path.startsWith(cardPathPrefix)) {
      return this.answerCards(request, token, address);
    }
    if (request.method === 'GET' && path.startsWith(invitationPathPrefix)) {
      return this.answerInvitation(decodeURIComponent(path.slice(invitationPathPrefix.length)));
    }
    if (request.method === 'GET' && path === joinRequestsPath) {
      return this.readJoinRequests(token, address.searchParams);
    }
    if (request.method === 'GET' && path === notificationsPath) {
      return this.readNotifications(token, address.searchParams);
    }
    if (request.method === 'GET' && path === calendarPath) {
      return this.readCalendar(token, address.searchParams);
    }
    if (path.startsWith(lessonPathPrefix)) {
      return this.answerLesson(request.method, path.slice(lessonPathPrefix.length), token);
    }
    if (path === emailPreferencesPath) {
      return this.answerPreferences(request, token);
    }
    if (path === unsubscribePreviewPath || path === unsubscribePath) {
      return this.answerUnsubscribe(request, path);
    }
    if (path !== '/api/v1/auth/logout') {
      return refusal(404, 'not_found');
    }
    const refused = this.refusedToken(token);
    if (refused !== null) {
      return refused;
    }
    this.signOutCount += 1;
    this.signOutReceived();
    await this.signOutGate;
    const session = this.sessionOf(token);
    if (session !== undefined) {
      session.revoked = true;
    }
    this.storedRefresh = null;
    return new Response(null, { status: 204 });
  }

  private async answerCards(request: Request, token: string, address: URL): Promise<Response> {
    const path = address.pathname;
    const refused = this.refusedToken(token);
    if (refused !== null) {
      return refused;
    }
    const tutorId = this.sessionOf(token)?.accountId ?? '';
    const caller = [...this.accounts.values()].find((account) => account.me.id === tutorId);
    if (caller?.me.role !== 'tutor') {
      return refusal(404, 'not_found');
    }
    if (request.method === 'POST' && path === '/api/v1/students') {
      const body: unknown = await request.json();
      if (
        typeof body !== 'object' ||
        body === null ||
        !('display_name' in body) ||
        typeof body.display_name !== 'string'
      ) {
        return refusal(422, 'validation_failed');
      }
      const id = `00000000-0000-4000-8000-${String(this.cards.size + 1).padStart(12, '0')}`;
      const card = fakeCard(id, body.display_name);
      this.cards.set(id, { tutorId, card });
      return json(201, card);
    }
    if (request.method === 'GET' && path === '/api/v1/students') {
      const requestedState = address.searchParams.get('state');
      const listedStates = requestedState === null ? null : studentListStates[requestedState];
      if (
        [...address.searchParams.keys()].some((name) => !supportedStudentListParameters.has(name)) ||
        listedStates === undefined
      ) {
        return refusal(422, 'validation_failed');
      }
      if (this.failingStudentListReads > 0) {
        this.failingStudentListReads -= 1;
        return refusal(503, 'service_unavailable');
      }
      const newestFirst = [...this.cards.values()]
        .filter((found) => found.tutorId === tutorId)
        .filter((found) => listedStates === null || listedStates.has(found.card.state))
        .map((found) => summaryOf(found.card, this.openAssignmentsOf(tutorId, found.card.id)))
        .reverse();
      const page = pageOf(newestFirst, address.searchParams);
      return page === null ? refusal(422, 'validation_failed') : json(200, page);
    }
    const found = this.cards.get(path.slice(cardPathPrefix.length));
    if (found?.tutorId !== tutorId) {
      return refusal(404, 'not_found');
    }
    if (request.method === 'PATCH') {
      return this.writeCard(found, await request.text());
    }
    if (request.method !== 'GET') {
      return refusal(404, 'not_found');
    }
    if (this.failingCardReads > 0) {
      this.failingCardReads -= 1;
      return refusal(503, 'service_unavailable');
    }
    return json(200, { ...found.card, next_lessons: [] });
  }

  private async writeCard(found: FakeCard, body: string): Promise<Response> {
    this.writtenCards.push({ cardId: found.card.id, body });
    this.cardWriteReceived();
    await this.cardWriteGate;
    if (this.failingCardWrites > 0) {
      this.failingCardWrites -= 1;
      return refusal(503, 'service_unavailable');
    }
    const patch: unknown = JSON.parse(body);
    if (typeof patch !== 'object' || patch === null || !('notes' in patch) || typeof patch.notes !== 'string') {
      return refusal(422, 'validation_failed');
    }
    const card: StudentCard = { ...found.card, notes: patch.notes };
    this.cards.set(card.id, { tutorId: found.tutorId, card });
    return json(200, card);
  }

  private answerInvitation(invitationToken: string): Response {
    if (this.failingInvitationReads > 0) {
      this.failingInvitationReads -= 1;
      return refusal(503, 'service_unavailable');
    }
    const found = invitationToken.startsWith(fakeTokenPrefix)
      ? this.cards.get(invitationToken.slice(fakeTokenPrefix.length))
      : undefined;
    const tutor = [...this.accounts.values()].find((account) => account.me.id === found?.tutorId);
    if (found?.card.state !== 'invited' || tutor === undefined) {
      return refusal(404, 'not_found');
    }
    return json(200, { tutor: { name: tutor.me.name, avatar_id: tutor.me.avatar_id, subjects: [] } });
  }

  private async readJoinRequests(token: string, query: URLSearchParams): Promise<Response> {
    const refused = this.refusedToken(token);
    this.joinRequestReadReceived();
    await this.joinRequestReadGate;
    if (refused !== null) {
      return refused;
    }
    const callerId = this.sessionOf(token)?.accountId ?? '';
    const caller = [...this.accounts.values()].find((account) => account.me.id === callerId);
    if (caller?.me.role !== 'tutor') {
      return refusal(404, 'not_found');
    }
    if (this.failingJoinRequestReads > 0) {
      this.failingJoinRequestReads -= 1;
      return refusal(500, 'internal');
    }
    const page = pageOf(this.visibleJoinRequests(callerId), query);
    return page === null ? refusal(422, 'validation_failed') : json(200, page);
  }

  private visibleJoinRequests(tutorId: string): TutorJoinRequest[] {
    const now = this.serverNow();
    return (this.joinRequests.get(tutorId) ?? [])
      .filter((request) => Date.parse(request.expires_at) > now)
      .sort(
        (left, right) => Date.parse(left.created_at) - Date.parse(right.created_at) || left.id.localeCompare(right.id),
      );
  }

  private openAssignmentsOf(tutorId: string, cardId: string): number {
    return this.openAssignmentCounts.get(cardId) ?? this.submissions.openCount(tutorId, cardId);
  }

  private clearFeedbackNew(studentId: string, submissionId: string): void {
    const items = this.assignments.get(studentId);
    if (items !== undefined) {
      this.assignments.set(
        studentId,
        items.map((item) => (item.submission_id === submissionId ? { ...item, feedback_new: false } : item)),
      );
    }
  }

  private settingsOf(tutorId: string): TutorSettings {
    return this.tutorSettings.get(tutorId) ?? defaultTutorSettings;
  }

  private codeOf(caller: Me): TutorCode {
    return this.tutorCodes.get(caller.id) ?? { code: defaultTutorCode, active: caller.state === 'active' };
  }

  private async answerTutorSettings(request: Request, caller: Me): Promise<Response> {
    if (request.method === 'GET') {
      return json(200, this.settingsOf(caller.id));
    }
    if (request.method !== 'PATCH') {
      return refusal(405, 'method_not_allowed');
    }
    const body = await request.text();
    this.writtenSettings.push(body);
    this.settingsWriteReceived();
    await this.settingsWriteGate;
    const refused = settingsPatchRefusal(body) ?? this.settingsWriteRefusals.shift();
    if (refused !== undefined) {
      return refused;
    }
    const settings: TutorSettings = { ...this.settingsOf(caller.id), onboarding_dismissed: true };
    this.tutorSettings.set(caller.id, settings);
    return json(200, settings);
  }

  private async readHome(caller: Me, address: URL): Promise<Response> {
    this.homeReadCount += 1;
    this.homeReadQueries.push(address.search);
    this.homeReadReceived();
    await this.homeReadGate;
    if (this.failingHomeReads > 0) {
      this.failingHomeReads -= 1;
      return refusal(503, 'service_unavailable');
    }
    if (caller.role === 'tutor') {
      return address.searchParams.has('tutor')
        ? fieldRefusal('query.tutor', 'not_allowed')
        : json(200, this.tutorHome(caller));
    }
    const current = (this.tutors.get(caller.id) ?? []).filter(isCurrentTutor);
    const filter = homeFilter(current, address.searchParams.getAll('tutor'));
    return filter === null ? refusal(404, 'not_found') : json(200, this.studentHome(caller, current, filter));
  }

  private tutorHome(caller: Me): TutorHome {
    const now = this.serverNow();
    const unconfirmed = caller.state === 'unverified';
    const dayStart = localDayStart(now, caller.timezone, 0);
    const calendar = byStart(this.calendarItems.get(caller.id) ?? []);
    const today = calendar.filter(startsWithin(dayStart, localDayStart(now, caller.timezone, 1)));
    const queue = [...(this.queueItems.get(caller.id) ?? [])].sort(queueOrder);
    const deadlines = (this.issueItems.get(caller.id) ?? [])
      .filter((issue) => issue.status === 'active' && issue.deadline_at !== null)
      .filter((issue) => {
        const deadline = Date.parse(issue.deadline_at ?? '');
        return deadline > now && deadline <= now + homeDeadlineWindowMilliseconds;
      })
      .sort(
        (left, right) =>
          Date.parse(left.deadline_at ?? '') - Date.parse(right.deadline_at ?? '') || left.id.localeCompare(right.id),
      );
    const requests = this.visibleJoinRequests(caller.id);
    return {
      today_lessons: today.slice(0, homeListLimit),
      today_lessons_count: today.length,
      needs_checking: unconfirmed
        ? null
        : { count: Math.min(queue.length, homeCountCap), items: queue.slice(0, homePanelLimit) },
      upcoming_deadlines: unconfirmed
        ? null
        : {
            count: Math.min(deadlines.length, homeCountCap),
            items: deadlines.slice(0, homePanelLimit).map(issueListItem),
          },
      unread_channels: this.chat.homeUnreadChannels(caller),
      join_requests: unconfirmed
        ? null
        : {
            count: Math.min(requests.length, homeCountCap),
            items: requests.slice(0, homePanelLimit),
            next_cursor: requests.length > homePanelLimit ? `${cursorPrefix}${String(homePanelLimit)}` : null,
          },
      tutor_code: this.codeOf(caller),
      checklist: {
        onboarding_dismissed: this.settingsOf(caller.id).onboarding_dismissed,
        steps: {
          profile: (caller.bio ?? '') !== '' || (caller.subjects ?? []).length > 0,
          student: unconfirmed ? null : [...this.cards.values()].some((found) => found.tutorId === caller.id),
          lesson: calendar.some(startsWithin(dayStart, localDayStart(now, caller.timezone, homeLookaheadDays))),
          assignment: (this.templateCounts.get(caller.id) ?? 0) > 0,
        },
      },
    };
  }

  private studentHome(caller: Me, current: readonly CurrentTutor[], filter: HomeFilter): StudentHome {
    const now = this.serverNow();
    const dayStart = localDayStart(now, caller.timezone, 0);
    const lookaheadEnd = localDayStart(now, caller.timezone, homeLookaheadDays);
    const lessons = byStart(this.calendarItems.get(caller.id) ?? []).filter(
      (item) =>
        item.status !== 'cancelled' &&
        Date.parse(item.ends_at) > dayStart &&
        Date.parse(item.starts_at) < lookaheadEnd &&
        this.lessonInFilter(item, filter),
    );
    const upcoming = lessons.filter((item) => Date.parse(item.ends_at) > now);
    const nextSevenDays = upcoming.filter((item) => Date.parse(item.starts_at) < now + homeWeekMilliseconds);
    const work = this.assignments.get(caller.id) ?? [];
    const inFilter = (item: StudentAssignmentItem): boolean =>
      filter.relationshipId === null || item.tutor.relationship_id === filter.relationshipId;
    const todo = work.filter((item) => todoStatuses.has(item.status)).sort(todoOrder);
    const checked = work.filter((item) => item.status === 'checked').sort(checkedOrder);
    return {
      next_lesson: upcoming[0] ?? null,
      today_lessons_count: lessons.filter(startsWithin(dayStart, localDayStart(now, caller.timezone, 1))).length,
      todo: { count: Math.min(todo.length, homeCountCap), items: todo.filter(inFilter).slice(0, homePanelLimit) },
      recently_checked: checked.filter(inFilter).slice(0, homePanelLimit),
      next_seven_days: nextSevenDays.slice(0, homeListLimit),
      next_seven_days_count: nextSevenDays.length,
      tutors: current.slice(0, homeListLimit).map(homeTutor),
      tutors_count: current.length,
    };
  }

  private lessonInFilter(item: CalendarItem, filter: HomeFilter): boolean {
    const relationshipId = this.lessonRelationships.get(item);
    return filter.relationshipId === null || relationshipId === undefined || relationshipId === filter.relationshipId;
  }

  private async answerLesson(method: string, route: string, token: string): Promise<Response> {
    const refused = this.refusedToken(token);
    if (refused !== null) {
      return refused;
    }
    const [id = '', action] = route.split('/');
    const lesson = this.lessons.get(id);
    const accountId = this.sessionOf(token)?.accountId ?? '';
    if (lesson?.members.includes(accountId) !== true) {
      return refusal(404, 'not_found');
    }
    if (method === 'GET' && action === undefined) {
      this.lessonDetailReads.set(id, this.lessonReads(id) + 1);
      return json(200, lesson.detail);
    }
    if (method !== 'POST' || (action !== 'join' && action !== 'end' && action !== 'restart-room')) {
      return refusal(405, 'method_not_allowed');
    }
    this.lessonWriteReceived();
    await this.lessonWriteGate;
    const planned = this.refusedLessonWrite;
    this.refusedLessonWrite = null;
    if (planned !== null) {
      return planned.retryAfterSeconds === null
        ? refusal(planned.status, planned.code)
        : refusalRetryingAfter(planned.status, planned.code, planned.retryAfterSeconds);
    }
    return action === 'end' ? this.endLesson(id) : this.openFakeRoom(id);
  }

  private endLesson(id: string): Response {
    this.changeLesson(id, { status: 'ended', room_open: false, took_place: true });
    return json(200, this.lessons.get(id)?.detail);
  }

  private openFakeRoom(id: string): Response {
    this.changeLesson(id, { status: 'live', room_open: true, took_place: true });
    this.roomGeneration += 1;
    const lesson = this.lessons.get(id);
    const url = this.nextRoomAddress ?? `https://room.example/${id}/${String(this.roomGeneration)}`;
    this.nextRoomAddress = null;
    return json(200, {
      url,
      embed_allowed: (lesson?.detail.embed_allowed ?? false) && !this.embeddingRefused.has(id),
    });
  }

  private async readCalendar(token: string, query: URLSearchParams): Promise<Response> {
    const refused = this.refusedToken(token);
    this.calendarReadReceived();
    await this.calendarReadGate;
    if (refused !== null) {
      return refused;
    }
    if (this.failingCalendarReads > 0) {
      this.failingCalendarReads -= 1;
      return refusal(503, 'service_unavailable');
    }
    const accountId = this.sessionOf(token)?.accountId ?? '';
    const caller = [...this.accounts.values()].find((account) => account.me.id === accountId);
    if (caller?.me.role === 'tutor' && query.has('tutor')) {
      return fieldRefusal('query.tutor', 'not_allowed');
    }
    const from = Date.parse(query.get('from') ?? '');
    const to = Date.parse(query.get('to') ?? '');
    if (Number.isNaN(from) || Number.isNaN(to)) {
      return refusal(422, 'validation_failed');
    }
    if (to <= from) {
      return fieldRefusal('query.to', 'invalid');
    }
    if (to - from > calendarMaxRangeMilliseconds) {
      return fieldRefusal('query.to', 'too_long');
    }
    const items = (this.calendarItems.get(accountId) ?? [])
      .filter((item) => Date.parse(item.ends_at) > from && Date.parse(item.starts_at) < to)
      .sort((left, right) => Date.parse(left.starts_at) - Date.parse(right.starts_at));
    return json(200, { items, truncated: false });
  }

  private readTemplate(caller: Me, id: string): Response {
    if (caller.role !== 'tutor') {
      return refusal(404, 'not_found');
    }
    if (this.failingTemplateReads > 0) {
      this.failingTemplateReads -= 1;
      return refusal(503, 'service_unavailable');
    }
    const template = this.templates.get(caller.id)?.get(id);
    return template === undefined ? refusal(404, 'not_found') : json(200, template);
  }

  private async uploadToLibrary(request: Request, caller: Me, upload: UploadTransfer | undefined): Promise<Response> {
    request.signal.throwIfAborted();
    if (caller.role !== 'tutor') {
      return refusal(404, 'not_found');
    }
    if (caller.state !== 'active') {
      return refusal(403, 'email_not_verified');
    }
    const body = upload?.body ?? (await request.formData());
    const file = body.get('file');
    if (file === null || typeof file === 'string') {
      return fieldRefusal('body.file', 'required');
    }
    const refused = this.libraryUploadRefusals.shift();
    if (refused !== undefined) {
      return refused;
    }
    upload?.progress(file.size, file.size);
    this.libraryUploads += 1;
    return json(201, {
      id: `00000000-0000-4000-9000-${String(this.libraryUploads).padStart(12, '0')}`,
      filename: file.name,
      media_type: file.type,
      size_bytes: file.size,
    });
  }

  private readNotifications(token: string, query: URLSearchParams): Response {
    const refused = this.refusedToken(token);
    if (refused !== null) {
      return refused;
    }
    if (this.notificationReadsFailing) {
      return refusal(503, 'service_unavailable');
    }
    const rows = this.notifications.get(this.sessionOf(token)?.accountId ?? '') ?? [];
    const page = pageOf(rows, query);
    if (page === null) {
      return refusal(422, 'validation_failed');
    }
    const unread = rows.filter((row) => row.read_at === null).length;
    return json(200, { ...page, unread_count: Math.min(unread, unreadCountCap) });
  }

  private async answerPreferences(request: Request, token: string): Promise<Response> {
    const refused = this.refusedToken(token);
    if (refused !== null) {
      return refused;
    }
    const accountId = this.sessionOf(token)?.accountId ?? '';
    const caller = [...this.accounts.values()].find((account) => account.me.id === accountId);
    if (caller === undefined) {
      return refusal(401, 'unauthenticated');
    }
    const groups = emailGroupsOf[caller.me.role];
    if (request.method === 'PUT') {
      if (this.failingPreferenceSaves > 0) {
        this.failingPreferenceSaves -= 1;
        return refusal(503, 'service_unavailable');
      }
      const requested = requestedPreferences(await request.json());
      if (requested === null || requested.some((preference) => !groups.includes(preference.group))) {
        return refusal(422, 'validation_failed');
      }
      for (const preference of requested) {
        this.setEmailGroup(accountId, preference.group, preference.enabled);
      }
    } else if (request.method !== 'GET') {
      return refusal(405, 'method_not_allowed');
    }
    const disabled = this.disabledGroups.get(accountId);
    return json(200, { groups: groups.map((group) => ({ group, enabled: !(disabled?.has(group) ?? false) })) });
  }

  private answerUnsubscribe(request: Request, path: string): Response {
    const grant = this.unsubscribeGrants.get(new URL(request.url).searchParams.get('token') ?? '');
    const owner = [...this.accounts.values()].find((account) => account.me.id === grant?.accountId);
    if (grant === undefined || owner === undefined || owner.blocked) {
      return refusal(422, 'token_invalid');
    }
    if (path === unsubscribePreviewPath && request.method === 'GET') {
      return json(200, { group: grant.group });
    }
    if (path === unsubscribePath && request.method === 'POST') {
      if (this.failingUnsubscribes > 0) {
        this.failingUnsubscribes -= 1;
        return refusal(503, 'service_unavailable');
      }
      this.setEmailGroup(grant.accountId, grant.group, false);
      return new Response(null, { status: 204 });
    }
    return refusal(405, 'method_not_allowed');
  }

  private setEmailGroup(accountId: string, group: EmailGroup, enabled: boolean): void {
    const disabled = this.disabledGroups.get(accountId) ?? new Set<EmailGroup>();
    if (enabled) {
      disabled.delete(group);
    } else {
      disabled.add(group);
    }
    this.disabledGroups.set(accountId, disabled);
  }

  private async writeAccount(token: string, request: Request): Promise<Response> {
    const accountId = this.sessionOf(token)?.accountId ?? null;
    const body: unknown = await request.clone().json();
    const language = typeof body === 'object' && body !== null && 'language' in body ? body.language : null;
    this.writtenAccounts.push({ accountId, language: typeof language === 'string' ? language : null });
    this.accountWriteReceived();
    await this.accountWriteGate;
    const answer = await this.readAccount(token, request.signal);
    this.answeredAccountWrites += 1;
    return answer;
  }

  private async readAccount(token: string, signal: AbortSignal): Promise<Response> {
    const answer = this.failingAccountReads > 0 ? this.failAccountRead() : this.answerMe(token);
    this.accountReadReceived();
    await this.accountReadGate;
    return this.accountBodies === null ? answer : this.accountBodies.response(answer, signal);
  }

  private failAccountRead(): Response {
    this.failingAccountReads -= 1;
    return refusal(503, 'service_unavailable');
  }

  private answerMe(token: string): Response {
    const refused = this.refusedToken(token);
    if (refused !== null) {
      return refused;
    }
    const session = this.sessionOf(token);
    const account = [...this.accounts.values()].find((candidate) => candidate.me.id === session?.accountId);
    if (account?.blocked === true) return refusal(403, 'account_blocked');
    return account === undefined ? refusal(401, 'unauthenticated') : json(200, account.me);
  }

  private refusedToken(token: string): Response | null {
    const access = this.tokens.get(token);
    const session = this.sessionOf(token);
    if (access === undefined || session === undefined || session.revoked) {
      return refusal(401, 'unauthenticated');
    }
    if (this.nowSeconds - access.issuedAt >= accessTokenSeconds) {
      return refusal(401, 'token_expired');
    }
    return null;
  }

  private sessionOf(token: string): FakeSession | undefined {
    const access = this.tokens.get(token);
    return access === undefined ? undefined : this.sessions.get(access.sessionId);
  }

  private issue(sessionId: number): string {
    this.issued += 1;
    const token = `fake-access-${String(this.issued)}`;
    this.tokens.set(token, { sessionId, issuedAt: this.nowSeconds });
    return token;
  }
}

export interface PausedAnswers {
  readonly resume: () => void;
  readonly answered: Promise<void>;
}

export interface FakeAccountWrite {
  readonly accountId: string | null;
  readonly language: string | null;
}

export interface PausedReads {
  readonly resume: () => void;
  readonly received: Promise<void>;
}

export function memoryLocks(): LockManagerLike {
  const queues = new Map<string, Promise<void>>();
  return {
    request: <Result>(name: string, callback: () => Promise<Result>): Promise<Result> => {
      const previous = queues.get(name) ?? Promise.resolve();
      const run = previous.then(callback);
      queues.set(
        name,
        run.then(
          () => undefined,
          () => undefined,
        ),
      );
      return run;
    },
  };
}

export function unsharedLocks(): LockManagerLike {
  return { request: <Result>(_name: string, callback: () => Promise<Result>): Promise<Result> => callback() };
}

export interface ManualScheduler extends Scheduler {
  readonly runAll: () => void;
  readonly nextSchedule: () => Promise<number>;
  readonly pass: (milliseconds: number) => void;
  readonly pendingDelays: () => readonly number[];
  readonly suspend: (milliseconds: number) => void;
}

export function manualScheduler(): ManualScheduler {
  const tasks = new Map<() => void, number>();
  const waiting: ((milliseconds: number) => void)[] = [];
  let elapsed = 0;
  let suspended = 0;

  function earliestDueBy(limit: number): (() => void) | null {
    let earliest: { readonly task: () => void; readonly due: number } | null = null;
    for (const [task, due] of tasks) {
      if (due <= limit && (earliest === null || due < earliest.due)) {
        earliest = { task, due };
      }
    }
    if (earliest === null) {
      return null;
    }
    elapsed = earliest.due;
    tasks.delete(earliest.task);
    return earliest.task;
  }

  return {
    wallClock: () => elapsed + suspended,
    suspend: (milliseconds): void => {
      suspended += milliseconds;
    },
    pass: (milliseconds): void => {
      const target = elapsed + milliseconds;
      for (let task = earliestDueBy(target); task !== null; task = earliestDueBy(target)) {
        task();
      }
      elapsed = target;
    },
    pendingDelays: () => [...tasks.values()].map((due) => due - elapsed).sort((left, right) => left - right),
    schedule: (milliseconds, task) => {
      const scheduled = (): void => {
        task();
      };
      tasks.set(scheduled, elapsed + milliseconds);
      for (const notify of waiting.splice(0)) {
        notify(milliseconds);
      }
      return (): void => {
        tasks.delete(scheduled);
      };
    },
    nextSchedule: () =>
      new Promise<number>((resolve) => {
        waiting.push(resolve);
      }),
    runAll: (): void => {
      const due = [...tasks.keys()];
      tasks.clear();
      for (const task of due) {
        task();
      }
    },
  };
}
