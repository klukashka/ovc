import { createContext, useContext, useSyncExternalStore } from 'react';

import { type ApiError, apiErrorFrom } from './client.ts';
import { type components, meLanguageValues, meRoleValues, meStateValues, meThemeValues } from './schema.ts';
import type { AuthTransport, UploadTransfer } from './transport.ts';

export type Me = components['schemas']['Me'];

type AccessToken = components['schemas']['AccessToken'];

export interface LockManagerLike {
  request<Result>(name: string, callback: () => Promise<Result>): Promise<Result>;
}

export interface Scheduler {
  schedule(milliseconds: number, task: () => void): () => void;
  wallClock(): number;
}

export type SignedOutReason = 'anonymous' | 'signedOut' | 'blocked' | 'accountDeleted' | 'sessionEnded';

export type RefreshResult =
  | { readonly kind: 'renewed' }
  | { readonly kind: 'ended'; readonly reason: SignedOutReason }
  | { readonly kind: 'superseded' };

export type SessionState =
  | { readonly kind: 'unknown' }
  | { readonly kind: 'unavailable' }
  | { readonly kind: 'signedOut'; readonly reason: SignedOutReason }
  | { readonly kind: 'signedIn'; readonly me: Me };

export type EndingSend = (request: Request) => Promise<Response>;

export type EndingReason = 'signedOut' | 'accountDeleted';

export type SignInResult =
  | { readonly kind: 'signedIn' }
  | { readonly kind: 'refused'; readonly error: ApiError; readonly retryAfterSeconds: number | null };

export interface SessionDependencies {
  readonly origin: string;
  readonly transport: AuthTransport;
  readonly locks: LockManagerLike;
  readonly scheduler: Scheduler;
}

const refreshLock = 'ovc-refresh';
const refreshShare = 0.8;
const refreshTimeoutMilliseconds = 15_000;
const readsBeforeEndingMilliseconds = 15_000;
const proactiveRetryMilliseconds = 30_000;
const apiPrefix = '/api/';
const superseded: RefreshResult = { kind: 'superseded' };
const renewed: RefreshResult = { kind: 'renewed' };

function oneOf<Value extends string>(values: readonly Value[], candidate: unknown): candidate is Value {
  return values.some((value) => value === candidate);
}

function isAccessToken(value: unknown): value is AccessToken {
  return (
    typeof value === 'object' &&
    value !== null &&
    'access_token' in value &&
    typeof value.access_token === 'string' &&
    'expires_in' in value &&
    typeof value.expires_in === 'number' &&
    value.expires_in > 0
  );
}

function isMe(value: unknown): value is Me {
  return (
    typeof value === 'object' &&
    value !== null &&
    'id' in value &&
    typeof value.id === 'string' &&
    'name' in value &&
    typeof value.name === 'string' &&
    'email' in value &&
    typeof value.email === 'string' &&
    'email_verified' in value &&
    typeof value.email_verified === 'boolean' &&
    'timezone' in value &&
    typeof value.timezone === 'string' &&
    'avatar_id' in value &&
    (value.avatar_id === null || typeof value.avatar_id === 'string') &&
    'reclaim_pending' in value &&
    typeof value.reclaim_pending === 'boolean' &&
    'role' in value &&
    oneOf(meRoleValues, value.role) &&
    'state' in value &&
    oneOf(meStateValues, value.state) &&
    'language' in value &&
    oneOf(meLanguageValues, value.language) &&
    'theme' in value &&
    oneOf(meThemeValues, value.theme)
  );
}

async function errorOf(response: Response): Promise<ApiError> {
  const body: unknown = await response.json().then(
    (parsed: unknown) => parsed,
    () => undefined,
  );
  return apiErrorFrom(response.status, body);
}

function retryAfter(response: Response): number | null {
  const header = response.headers.get('Retry-After');
  if (header === null || !/^[0-9]+$/.test(header)) {
    return null;
  }
  return Number(header);
}

export const ownedResponseBodyLimit = 64 * 1024;

function responseBodyAborted(signal: AbortSignal): boolean {
  return signal.aborted;
}

function declaresBoundedBody(response: Response): boolean {
  if (response.headers.has('Content-Encoding') || response.headers.has('Transfer-Encoding')) return false;
  const length = response.headers.get('Content-Length');
  return length !== null && /^\d+$/.test(length) && Number(length) <= ownedResponseBodyLimit;
}

async function drainDeclaredBody(
  response: Response,
  signal: AbortSignal,
  owner: 'refresh_refusal' | 'account',
): Promise<void> {
  let rejectAbort = (): void => undefined;
  const aborted = new Promise<never>((_resolve, reject) => {
    rejectAbort = (): void => {
      reject(new Error(`${owner}_body_aborted`));
    };
  });
  signal.addEventListener('abort', rejectAbort, { once: true });
  let failure: 'overflow' | 'aborted' | 'read_failed' | null = null;
  try {
    const body = await Promise.race([response.arrayBuffer(), aborted]);
    if (body.byteLength > ownedResponseBodyLimit) failure = 'overflow';
  } catch {
    failure = responseBodyAborted(signal) ? 'aborted' : 'read_failed';
  } finally {
    signal.removeEventListener('abort', rejectAbort);
  }
  if (failure !== null) throw new AggregateError([new Error(`${owner}_body_${failure}`)], `${owner}_body_failed`);
}

export async function drainOwnedResponse(
  response: Response,
  signal: AbortSignal,
  owner: 'refresh_refusal' | 'account',
): Promise<void> {
  if (responseBodyAborted(signal)) throw new Error(`${owner}_body_aborted`);
  if (response.body === null) return;
  if (declaresBoundedBody(response)) {
    await drainDeclaredBody(response, signal, owner);
    return;
  }
  const reader = response.body.getReader();
  let rejectAbort = (): void => undefined;
  const aborted = new Promise<never>((_resolve, reject) => {
    rejectAbort = (): void => {
      reject(new Error(`${owner}_body_aborted`));
    };
  });
  signal.addEventListener('abort', rejectAbort, { once: true });
  let size = 0;
  let complete = false;
  let failure: 'overflow' | 'aborted' | 'read_failed' | null = null;
  let cancellationFailed = false;
  try {
    while (!complete) {
      if (responseBodyAborted(signal)) throw new Error(`${owner}_body_aborted`);
      const part = await Promise.race([reader.read(), aborted]);
      if (responseBodyAborted(signal)) throw new Error(`${owner}_body_aborted`);
      if (part.done) {
        complete = true;
        continue;
      }
      if (part.value.byteLength > ownedResponseBodyLimit - size) {
        failure = 'overflow';
        throw new Error(`${owner}_body_overflow`);
      }
      size += part.value.byteLength;
    }
  } catch {
    failure ??= responseBodyAborted(signal) ? 'aborted' : 'read_failed';
    try {
      await reader.cancel();
    } catch {
      cancellationFailed = true;
    }
  } finally {
    signal.removeEventListener('abort', rejectAbort);
    reader.releaseLock();
  }
  if (failure !== null) {
    const errors = [new Error(`${owner}_body_${failure}`)];
    if (cancellationFailed) errors.push(new Error(`${owner}_body_cancellation_failed`));
    throw new AggregateError(errors, `${owner}_body_failed`);
  }
}

export class Session {
  private readonly deps: SessionDependencies;
  private current: SessionState = { kind: 'unknown' };
  private token: string | null = null;
  private tokenExpiresAt = 0;
  private epoch = 0;
  private pendingRefresh: Promise<RefreshResult> | null = null;
  private readonly endingActions = new Set<Promise<unknown>>();
  private readonly requestsInFlight = new Set<Promise<unknown>>();
  private cancelScheduled: (() => void) | null = null;
  private readonly listeners = new Set<() => void>();
  private readonly tokenListeners = new Set<(token: string, epoch: number) => void>();

  constructor(dependencies: SessionDependencies) {
    this.deps = dependencies;
  }

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return (): void => {
      this.listeners.delete(listener);
    };
  };

  readonly snapshot = (): SessionState => this.current;

  readonly currentEpoch = (): number => this.epoch;

  readonly subscribeTokens = (listener: (token: string, epoch: number) => void): (() => void) => {
    this.tokenListeners.add(listener);
    return (): void => {
      this.tokenListeners.delete(listener);
    };
  };

  readonly whileEnding = async <Result>(
    work: (send: EndingSend) => Promise<Result>,
    endsWith: (result: Result) => EndingReason | null = () => null,
  ): Promise<Result> => {
    const running = this.settleRequestsInFlight()
      .then(() => work(this.sendWhileEnding))
      .then((result) => {
        const reason = endsWith(result);
        if (reason !== null) this.end({ kind: 'signedOut', reason });
        return result;
      });
    this.endingActions.add(running);
    try {
      return await running;
    } finally {
      this.endingActions.delete(running);
    }
  };

  readonly refreshAfterSocketRefusal = async (): Promise<RefreshResult> =>
    (await this.survivesOwnEnding()) ? this.refresh() : superseded;

  readonly freshToken = async (minimumValidityMilliseconds: number, epoch: number): Promise<string | null> => {
    if (epoch !== this.epoch) {
      return null;
    }
    if (this.token !== null && this.tokenExpiresAt - this.deps.scheduler.wallClock() > minimumValidityMilliseconds) {
      return this.token;
    }
    const result = await this.refresh();
    return result.kind === 'renewed' && epoch === this.epoch ? this.token : null;
  };

  readonly authorizedFetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    return this.authorizedRequest(new Request(input, init));
  };

  readonly authorizedUpload = (request: Request, upload: UploadTransfer): Promise<Response> => {
    if (!this.isOwnApi(request.url)) throw new Error('uploads require the own API');
    const headers = new Headers(request.headers);
    headers.delete('Content-Type');
    return this.authorizedRequest(new Request(request, { headers }), upload);
  };

  private readonly sendWhileEnding = (request: Request): Promise<Response> =>
    this.authorizedRequest(request, undefined, 'ending');

  private async survivesOwnEnding(): Promise<boolean> {
    const epoch = this.epoch;
    while (this.endingActions.size > 0) {
      await Promise.allSettled([...this.endingActions]);
    }
    return epoch === this.epoch;
  }

  private async authorizedRequest(
    request: Request,
    upload?: UploadTransfer,
    owner: 'ending' | 'session' = 'session',
  ): Promise<Response> {
    request.signal.throwIfAborted();
    if (owner === 'session' && this.endingActions.size > 0 && !(await this.survivesOwnEnding())) {
      throw new DOMException('the session ended before the request was sent', 'AbortError');
    }
    request.signal.throwIfAborted();
    const exchange = this.exchange(request, upload);
    if (owner === 'ending' || request.method !== 'GET') return exchange;
    this.requestsInFlight.add(exchange);
    try {
      return await exchange;
    } finally {
      this.requestsInFlight.delete(exchange);
    }
  }

  private settleRequestsInFlight(): Promise<void> {
    if (this.requestsInFlight.size === 0) return Promise.resolve();
    return new Promise((resolve) => {
      const cancelDeadline = this.deps.scheduler.schedule(readsBeforeEndingMilliseconds, resolve);
      void this.everyRequestInFlightSettled().then(() => {
        cancelDeadline();
        resolve();
      });
    });
  }

  private async everyRequestInFlightSettled(): Promise<void> {
    while (this.requestsInFlight.size > 0) {
      await Promise.allSettled([...this.requestsInFlight]);
    }
  }

  private async exchange(request: Request, upload: UploadTransfer | undefined): Promise<Response> {
    const retry = request.clone();
    const sentToken = this.token;
    const epoch = this.epoch;
    const response = await this.deps.transport.send(this.withToken(request), upload);
    if (epoch !== this.epoch) {
      return response;
    }
    if (response.status === 401 && (await errorOf(response.clone())).code === 'token_expired') {
      if (epoch !== this.epoch) {
        return response;
      }
      const replaced = this.token !== null && this.token !== sentToken;
      if ((replaced || (await this.refresh()).kind === 'renewed') && epoch === this.epoch) {
        retry.signal.throwIfAborted();
        const retried = await this.deps.transport.send(this.withToken(retry), upload);
        return epoch === this.epoch ? this.watch(retried, epoch) : retried;
      }
    }
    return epoch === this.epoch ? this.watch(response, epoch) : response;
  }

  readonly start = async (): Promise<void> => {
    this.set({ kind: 'unknown' });
    const epoch = this.epoch;
    try {
      if ((await this.refresh()).kind === 'renewed') {
        await this.loadMe();
      }
    } catch {
      if (epoch === this.epoch) {
        this.end({ kind: 'unavailable' });
      }
    }
  };

  readonly refresh = async (): Promise<RefreshResult> => {
    if (this.pendingRefresh !== null) {
      return this.pendingRefresh;
    }
    const epoch = this.epoch;
    const pending = this.deps.locks
      .request(refreshLock, () => (epoch === this.epoch ? this.refreshNow(epoch) : Promise.resolve(superseded)))
      .finally(() => {
        if (this.pendingRefresh === pending) {
          this.pendingRefresh = null;
        }
      });
    this.pendingRefresh = pending;
    return pending;
  };

  readonly signIn = async (email: string, password: string): Promise<SignInResult> => {
    const epoch = this.epoch;
    const response = await this.deps.transport.login(email, password);
    if (!response.ok) {
      return { kind: 'refused', error: await errorOf(response), retryAfterSeconds: retryAfter(response) };
    }
    const token: unknown = await response.json();
    if (epoch !== this.epoch) {
      throw new Error('the session changed while signing in');
    }
    await this.installAccount(token, epoch);
    if (this.current.kind !== 'signedIn') {
      throw new Error('the session ended while the account was loading');
    }
    return { kind: 'signedIn' };
  };

  readonly adopt = async (token: unknown): Promise<void> => {
    this.epoch += 1;
    this.pendingRefresh = null;
    await this.installAccount(token, this.epoch);
  };

  readonly updateAccount = (value: unknown): void => {
    if (!isMe(value)) {
      throw new Error('the server answered an account of an unexpected shape');
    }
    if (this.current.kind !== 'signedIn' || this.current.me.id !== value.id) {
      throw new Error('the updated account is not the signed-in account');
    }
    this.set({ kind: 'signedIn', me: value });
  };

  readonly endRevoked = (reason: 'signedOut' | 'accountDeleted'): void => {
    this.end({ kind: 'signedOut', reason });
  };

  readonly reloadAccount = async (): Promise<void> => {
    await this.loadMe();
  };

  readonly signOut = async (): Promise<void> => {
    const response = await this.whileEnding(
      (send) => send(new Request(`${this.deps.origin}/api/v1/auth/logout`, { method: 'POST' })),
      (answer) => (answer.ok || answer.status === 401 ? 'signedOut' : null),
    );
    if (!response.ok && response.status !== 401) {
      throw await errorOf(response);
    }
  };

  private async installAccount(token: unknown, epoch: number): Promise<void> {
    try {
      this.acceptToken(token);
      await this.loadMe();
      if (epoch === this.epoch && this.current.kind !== 'signedIn') {
        this.end(this.current);
      }
    } catch (error) {
      if (epoch === this.epoch) {
        this.end({ kind: 'unavailable' });
      }
      throw error;
    }
  }

  private withToken(request: Request): Request {
    if (this.token === null || !this.isOwnApi(request.url)) {
      return request;
    }
    const headers = new Headers(request.headers);
    headers.set('Authorization', `Bearer ${this.token}`);
    return new Request(request, { headers });
  }

  private isOwnApi(address: string): boolean {
    const url = new URL(address);
    return url.origin === this.deps.origin && url.pathname.startsWith(apiPrefix);
  }

  private async watch(response: Response, epoch: number): Promise<Response> {
    if (
      response.status === 403 &&
      (await errorOf(response.clone())).code === 'account_blocked' &&
      epoch === this.epoch
    ) {
      this.end({ kind: 'signedOut', reason: 'blocked' });
    } else if (response.status === 401 && this.current.kind === 'signedIn') {
      this.end({ kind: 'signedOut', reason: 'sessionEnded' });
    }
    return response;
  }

  private async refreshNow(epoch: number): Promise<RefreshResult> {
    const signal = AbortSignal.timeout(refreshTimeoutMilliseconds);
    const response = await this.deps.transport.refresh(signal);
    if (epoch !== this.epoch) {
      if (response.status === 401 || response.status === 204)
        await drainOwnedResponse(response, signal, 'refresh_refusal');
      return superseded;
    }
    if (response.status === 200) {
      const token: unknown = await response.json();
      if (epoch !== this.epoch) {
        return superseded;
      }
      if (!isAccessToken(token)) {
        throw new Error('the server answered an access token of an unexpected shape');
      }
      const owner = await this.ownerOf(token, epoch);
      if (owner === 'superseded') {
        return superseded;
      }
      if (owner === 'another') {
        return this.endWith('anonymous');
      }
      this.acceptToken(token);
      return renewed;
    }
    if (response.status === 204 || response.status === 401) {
      const ended = this.endWith(this.current.kind === 'signedIn' ? 'sessionEnded' : 'anonymous');
      const endedEpoch = this.epoch;
      await drainOwnedResponse(response, signal, 'refresh_refusal');
      return endedEpoch === this.epoch ? ended : superseded;
    }
    const error = await errorOf(response);
    if (epoch !== this.epoch) {
      return superseded;
    }
    if (error.code === 'account_blocked') {
      return this.endWith('blocked');
    }
    throw error;
  }

  private endWith(reason: SignedOutReason): RefreshResult {
    this.end({ kind: 'signedOut', reason });
    return { kind: 'ended', reason };
  }

  private async ownerOf(token: AccessToken, epoch: number): Promise<'shown' | 'another' | 'superseded'> {
    if (this.current.kind !== 'signedIn') {
      return 'shown';
    }
    const shown = this.current.me.id;
    const signal = AbortSignal.timeout(refreshTimeoutMilliseconds);
    const response = await this.deps.transport.me(token.access_token, signal);
    if (epoch !== this.epoch) {
      await drainOwnedResponse(response, signal, 'account');
      return 'superseded';
    }
    if (!response.ok) {
      const error = await errorOf(response);
      if (epoch !== this.epoch) {
        return 'superseded';
      }
      throw error;
    }
    const me: unknown = await response.json();
    if (epoch !== this.epoch) {
      return 'superseded';
    }
    if (!isMe(me)) {
      throw new Error('the server answered an account of an unexpected shape');
    }
    return me.id === shown ? 'shown' : 'another';
  }

  private acceptToken(value: unknown): void {
    if (!isAccessToken(value)) {
      throw new Error('the server answered an access token of an unexpected shape');
    }
    this.token = value.access_token;
    this.tokenExpiresAt = this.deps.scheduler.wallClock() + value.expires_in * 1000;
    for (const listener of this.tokenListeners) {
      listener(value.access_token, this.epoch);
    }
    this.scheduleRefresh(value.expires_in * 1000 * refreshShare, value.expires_in * 1000 * (1 - refreshShare));
  }

  private scheduleRefresh(delay: number, remaining: number): void {
    this.cancelScheduled?.();
    const epoch = this.epoch;
    this.cancelScheduled = this.deps.scheduler.schedule(delay, () => {
      this.refresh().catch(() => {
        if (epoch === this.epoch && remaining > proactiveRetryMilliseconds) {
          this.scheduleRefresh(proactiveRetryMilliseconds, remaining - proactiveRetryMilliseconds);
        }
      });
    });
  }

  private async loadMe(): Promise<void> {
    const epoch = this.epoch;
    const signal = AbortSignal.timeout(refreshTimeoutMilliseconds);
    const response = await this.authorizedFetch(`${this.deps.origin}/api/v1/users/me`, { signal });
    if (epoch !== this.epoch) {
      await drainOwnedResponse(response, signal, 'account');
      return;
    }
    if (response.status === 401 || response.status === 403) {
      if (this.current.kind !== 'signedOut') {
        this.end({ kind: 'signedOut', reason: 'anonymous' });
      }
      await drainOwnedResponse(response, signal, 'account');
      return;
    }
    if (!response.ok) {
      throw await errorOf(response);
    }
    const me: unknown = await response.json();
    if (epoch !== this.epoch) {
      return;
    }
    if (!isMe(me)) {
      throw new Error('the server answered an account of an unexpected shape');
    }
    this.set({ kind: 'signedIn', me });
  }

  private end(state: SessionState): void {
    this.epoch += 1;
    this.token = null;
    this.tokenExpiresAt = 0;
    this.pendingRefresh = null;
    this.cancelScheduled?.();
    this.cancelScheduled = null;
    this.set(state);
  }

  private set(state: SessionState): void {
    this.current = state;
    for (const listener of this.listeners) {
      listener();
    }
  }
}

export const browserScheduler: Scheduler = {
  schedule: (milliseconds, task) => {
    const handle = window.setTimeout(task, milliseconds);
    return (): void => {
      window.clearTimeout(handle);
    };
  },
  wallClock: () => Date.now(),
};

export function browserLocks(locks: LockManager | undefined): LockManagerLike {
  if (locks === undefined) {
    throw new Error('The Web Locks API is required to keep sessions consistent across tabs');
  }
  return {
    request: <Result>(name: string, callback: () => Promise<Result>): Promise<Result> => locks.request(name, callback),
  };
}

export const SessionContext = createContext<Session | null>(null);

export function useSession(): Session {
  const session = useContext(SessionContext);
  if (session === null) {
    throw new Error('SessionContext is not provided');
  }
  return session;
}

export function useSessionState(): SessionState {
  const session = useSession();
  return useSyncExternalStore(session.subscribe, session.snapshot);
}

export function useSignedInAccount(): Me {
  const state = useSessionState();
  if (state.kind !== 'signedIn') {
    throw new Error('a signed-in page rendered without a signed-in account');
  }
  return state.me;
}
