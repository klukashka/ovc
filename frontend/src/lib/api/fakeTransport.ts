import type { components } from './schema.ts';
import type { LockManagerLike, Me, Scheduler } from './session.ts';
import type { AuthTransport } from './transport.ts';

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

interface FakeAccessToken {
  readonly sessionId: number;
  readonly issuedAt: number;
}

interface RefreshCookie {
  readonly sessionId: number;
  readonly generation: number;
}

type StudentCard = components['schemas']['StudentCard'];

const cardPathPrefix = '/api/v1/students/';
const invitationPathPrefix = '/api/v1/invitations/';
const fakeTokenPrefix = 'fake-';

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
}

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function refusal(status: number, code: string): Response {
  return json(status, { error: { code, message: code, request_id: 'fake-request' } });
}

async function nextTurn(): Promise<void> {
  await Promise.resolve();
}

export class FakeAuthServer {
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
  private accountReadGate: Promise<void> | null = null;
  private accountReadReceived: () => void = () => undefined;
  private refreshCount = 0;
  private reuse = false;
  private readonly sent: string[] = [];
  private readonly cards = new Map<string, FakeCard>();
  private failingInvitationReads = 0;

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

  addAccount(seed: FakeAccountSeed): void {
    this.accounts.set(seed.email, {
      password: seed.password,
      blocked: false,
      me: {
        id: seed.id,
        name: seed.name,
        email: seed.email,
        email_verified: true,
        timezone: 'Europe/Moscow',
        avatar_id: null,
        reclaim_pending: false,
        role: seed.role ?? 'tutor',
        state: 'active',
        language: 'ru',
        theme: 'system',
      },
    });
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

  failNextAccountCheck(): void {
    this.failingAccountChecks += 1;
  }

  failNextAccountRead(): void {
    this.failingAccountReads += 1;
  }

  failNextInvitationRead(): void {
    this.failingInvitationReads += 1;
  }

  failNextRefresh(): void {
    this.failingRefreshes += 1;
  }

  block(email: string): void {
    const account = this.accounts.get(email);
    if (account === undefined) {
      throw new Error('no fake account has this email');
    }
    account.blocked = true;
    for (const session of this.sessions.values()) {
      if (session.accountId === account.me.id) {
        session.revoked = true;
      }
    }
  }

  issuedBy(token: string): string | undefined {
    const access = this.tokens.get(token);
    const session = access === undefined ? undefined : this.sessions.get(access.sessionId);
    return session?.accountId;
  }

  transport(): AuthTransport {
    return {
      login: (email, password) => this.login(email, password),
      refresh: () => this.refresh(),
      me: (token) => this.checkAccount(token),
      send: (request) => this.send(request),
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

  private async refresh(): Promise<Response> {
    const presented = this.storedRefresh;
    await nextTurn();
    const answer = this.failingRefreshes > 0 ? this.failRefresh() : this.rotate(presented);
    this.refreshReceived();
    await this.refreshGate;
    return answer;
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
    session.generation += 1;
    this.grantedRefreshes += 1;
    this.storedRefresh = { sessionId: session.id, generation: session.generation };
    return json(200, { access_token: this.issue(session.id), expires_in: accessTokenSeconds });
  }

  private async checkAccount(token: string): Promise<Response> {
    await nextTurn();
    if (this.failingAccountChecks > 0) {
      this.failingAccountChecks -= 1;
      return refusal(503, 'service_unavailable');
    }
    return this.answerMe(token);
  }

  private async send(request: Request): Promise<Response> {
    await nextTurn();
    const token = request.headers.get('Authorization')?.replace(/^Bearer /, '') ?? '';
    this.sent.push(token);
    const path = new URL(request.url).pathname;
    if (path === '/api/v1/users/me') {
      return this.readAccount(token);
    }
    if (path === '/api/v1/students' || path.startsWith(cardPathPrefix)) {
      return this.answerCards(request, token, path);
    }
    if (request.method === 'GET' && path.startsWith(invitationPathPrefix)) {
      return this.answerInvitation(decodeURIComponent(path.slice(invitationPathPrefix.length)));
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
    return json(200, found.card);
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

  private async readAccount(token: string): Promise<Response> {
    const answer = this.failingAccountReads > 0 ? this.failAccountRead() : this.answerMe(token);
    this.accountReadReceived();
    await this.accountReadGate;
    return answer;
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
}

export function manualScheduler(): ManualScheduler {
  const tasks = new Set<() => void>();
  const waiting: ((milliseconds: number) => void)[] = [];
  return {
    schedule: (milliseconds, task) => {
      tasks.add(task);
      for (const notify of waiting.splice(0)) {
        notify(milliseconds);
      }
      return (): void => {
        tasks.delete(task);
      };
    },
    nextSchedule: () =>
      new Promise<number>((resolve) => {
        waiting.push(resolve);
      }),
    runAll: (): void => {
      const due = [...tasks];
      tasks.clear();
      for (const task of due) {
        task();
      }
    },
  };
}
