import { FakeChatServer } from './fakeChat.ts';

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

const cardPathPrefix = '/api/v1/students/';
const calendarPath = '/api/v1/calendar';
const lessonPathPrefix = '/api/v1/lessons/';
const joinRequestsPath = '/api/v1/join-requests';
const notificationsPath = '/api/v1/notifications';
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
}

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function refusal(status: number, code: string): Response {
  return json(status, { error: { code, message: code, request_id: 'fake-request' } });
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
    path.startsWith('/api/v1/groups');
  if ((tutorOnly && caller.role !== 'tutor') || (path === '/api/v1/tutors' && caller.role !== 'student')) {
    return refusal(404, 'not_found');
  }
  const unverifiedOK =
    path === '/api/v1/users/me' ||
    path === '/api/v1/auth/logout' ||
    path === emailPreferencesPath ||
    (method === 'GET' && (path === notificationsPath || path === calendarPath || path === '/api/v1/tutors')) ||
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
  private readonly tutors = new Map<string, components['schemas']['MyTutor'][]>();
  private readonly accounts = new Map<string, FakeAccount>();
  private readonly sessions = new Map<number, FakeSession>();
  private readonly tokens = new Map<string, FakeAccessToken>();
  private storedRefresh: RefreshCookie | null = null;
  private nowSeconds = 0;
  private issued = 0;
  private refreshGate: Promise<void> | null = null;
  private refreshReceived: () => void = () => undefined;
  private grantedRefreshes = 0;
  private failingAccountChecks = 0;
  private failingAccountReads = 0;
  private failingRefreshes = 0;
  private lostRefreshes = 0;
  private accountReadGate: Promise<void> | null = null;
  private accountReadReceived: () => void = () => undefined;
  private joinRequestReadGate: Promise<void> | null = null;
  private joinRequestReadReceived: () => void = () => undefined;
  private failingJoinRequestReads = 0;
  private notificationReadsFailing = false;
  private failingCardReads = 0;
  private refreshCount = 0;
  private reuse = false;
  private readonly sent: string[] = [];
  private readonly addresses: URL[] = [];
  private readonly calendarItems = new Map<string, CalendarItem[]>();
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

  addCalendarItem(email: string, item: CalendarItem): void {
    const accountId = this.account(email).me.id;
    this.calendarItems.set(accountId, [...(this.calendarItems.get(accountId) ?? []), item]);
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

  refuseNextLessonWrite(status: number, code: string): void {
    this.refusedLessonWrite = { status, code };
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

  failNextCardRead(): void {
    this.failingCardReads += 1;
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
    if (path === '/api/v1/uploads' && request.method === 'POST' && caller !== undefined) {
      request.signal.throwIfAborted();
      const body = upload?.body ?? (await request.formData());
      const file = body.get('file');
      if (upload !== undefined && file !== null && typeof file !== 'string') upload.progress(file.size, file.size);
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
    if (path === '/api/v1/tutors' && request.method === 'GET') {
      const invalid = this.refusedToken(token);
      if (invalid !== null) {
        return invalid;
      }
      return json(200, { tutors: this.tutors.get(accountId ?? '') ?? [] });
    }
    if (path === '/api/v1/students' || path.startsWith(cardPathPrefix)) {
      return this.answerCards(request, token, path);
    }
    if (request.method === 'GET' && path.startsWith(invitationPathPrefix)) {
      return this.answerInvitation(decodeURIComponent(path.slice(invitationPathPrefix.length)));
    }
    if (request.method === 'GET' && path === joinRequestsPath) {
      return this.readJoinRequests(token);
    }
    if (request.method === 'GET' && path === notificationsPath) {
      return this.readNotifications(token);
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
    const session = this.sessionOf(token);
    if (session !== undefined) {
      session.revoked = true;
    }
    this.storedRefresh = null;
    return new Response(null, { status: 204 });
  }

  private async answerCards(request: Request, token: string, path: string): Promise<Response> {
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
    const found = this.cards.get(path.slice(cardPathPrefix.length));
    if (request.method !== 'GET' || found?.tutorId !== tutorId) {
      return refusal(404, 'not_found');
    }
    if (this.failingCardReads > 0) {
      this.failingCardReads -= 1;
      return refusal(503, 'service_unavailable');
    }
    return json(200, { ...found.card, next_lessons: [] });
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

  private async readJoinRequests(token: string): Promise<Response> {
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
    return json(200, { items: [], next_cursor: null });
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
      return refusal(planned.status, planned.code);
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

  private readNotifications(token: string): Response {
    const refused = this.refusedToken(token);
    if (refused !== null) {
      return refused;
    }
    if (this.notificationReadsFailing) {
      return refusal(503, 'service_unavailable');
    }
    return json(200, { items: [], next_cursor: null, unread_count: 0 });
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
