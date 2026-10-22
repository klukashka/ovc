import { createContext, useContext, useSyncExternalStore } from 'react';

import { type ApiError, apiErrorFrom } from './client.ts';
import { type components, meLanguageValues, meRoleValues, meStateValues, meThemeValues } from './schema.ts';
import type { AuthTransport } from './transport.ts';

export type Me = components['schemas']['Me'];

type AccessToken = components['schemas']['AccessToken'];

export interface LockManagerLike {
  request<Result>(name: string, callback: () => Promise<Result>): Promise<Result>;
}

export interface Scheduler {
  schedule(milliseconds: number, task: () => void): () => void;
}

export type SignedOutReason = 'anonymous' | 'signedOut' | 'blocked' | 'accountDeleted';

export type SessionState =
  | { readonly kind: 'unknown' }
  | { readonly kind: 'unavailable' }
  | { readonly kind: 'signedOut'; readonly reason: SignedOutReason }
  | { readonly kind: 'signedIn'; readonly me: Me };

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
const proactiveRetryMilliseconds = 30_000;
const apiPrefix = '/api/';

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

export class Session {
  private readonly deps: SessionDependencies;
  private current: SessionState = { kind: 'unknown' };
  private token: string | null = null;
  private epoch = 0;
  private pendingRefresh: Promise<boolean> | null = null;
  private cancelScheduled: (() => void) | null = null;
  private readonly listeners = new Set<() => void>();

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

  readonly authorizedFetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const request = new Request(input, init);
    const retry = request.clone();
    const sentToken = this.token;
    const epoch = this.epoch;
    const response = await this.deps.transport.send(this.withToken(request));
    if (epoch !== this.epoch) {
      return response;
    }
    if (response.status === 401 && (await errorOf(response.clone())).code === 'token_expired') {
      if (epoch !== this.epoch) {
        return response;
      }
      const renewed = this.token !== null && this.token !== sentToken ? true : await this.refresh();
      if (renewed && epoch === this.epoch) {
        const retried = await this.deps.transport.send(this.withToken(retry));
        return epoch === this.epoch ? this.watch(retried, epoch) : retried;
      }
    }
    return epoch === this.epoch ? this.watch(response, epoch) : response;
  };

  readonly start = async (): Promise<void> => {
    this.set({ kind: 'unknown' });
    try {
      if (await this.refresh()) {
        await this.loadMe();
      }
    } catch {
      this.end({ kind: 'unavailable' });
    }
  };

  readonly refresh = async (): Promise<boolean> => {
    if (this.pendingRefresh !== null) {
      return this.pendingRefresh;
    }
    const epoch = this.epoch;
    const pending = this.deps.locks
      .request(refreshLock, () => this.refreshNow(epoch))
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
    const response = await this.authorizedFetch(`${this.deps.origin}/api/v1/auth/logout`, { method: 'POST' });
    if (!response.ok && response.status !== 401) {
      throw await errorOf(response);
    }
    this.end({ kind: 'signedOut', reason: 'signedOut' });
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
      this.end({ kind: 'signedOut', reason: 'anonymous' });
    }
    return response;
  }

  private async refreshNow(epoch: number): Promise<boolean> {
    const response = await this.deps.transport.refresh(AbortSignal.timeout(refreshTimeoutMilliseconds));
    if (epoch !== this.epoch) {
      return false;
    }
    if (response.status === 200) {
      const token: unknown = await response.json();
      if (epoch !== this.epoch) {
        return false;
      }
      if (!isAccessToken(token)) {
        throw new Error('the server answered an access token of an unexpected shape');
      }
      if (!(await this.belongsToShownAccount(token, epoch)) || epoch !== this.epoch) {
        return false;
      }
      this.acceptToken(token);
      return true;
    }
    if (response.status === 204 || response.status === 401) {
      this.end({ kind: 'signedOut', reason: 'anonymous' });
      return false;
    }
    const error = await errorOf(response);
    if (epoch !== this.epoch) {
      return false;
    }
    if (error.code === 'account_blocked') {
      this.end({ kind: 'signedOut', reason: 'blocked' });
      return false;
    }
    throw error;
  }

  private async belongsToShownAccount(token: AccessToken, epoch: number): Promise<boolean> {
    if (this.current.kind !== 'signedIn') {
      return true;
    }
    const shown = this.current.me.id;
    const response = await this.deps.transport.me(token.access_token, AbortSignal.timeout(refreshTimeoutMilliseconds));
    if (epoch !== this.epoch) {
      return false;
    }
    if (!response.ok) {
      const error = await errorOf(response);
      if (epoch !== this.epoch) {
        return false;
      }
      throw error;
    }
    const me: unknown = await response.json();
    if (epoch !== this.epoch) {
      return false;
    }
    if (!isMe(me)) {
      throw new Error('the server answered an account of an unexpected shape');
    }
    if (me.id !== shown) {
      this.end({ kind: 'signedOut', reason: 'anonymous' });
      return false;
    }
    return true;
  }

  private acceptToken(value: unknown): void {
    if (!isAccessToken(value)) {
      throw new Error('the server answered an access token of an unexpected shape');
    }
    this.token = value.access_token;
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
    const response = await this.authorizedFetch(`${this.deps.origin}/api/v1/users/me`);
    if (epoch !== this.epoch) {
      return;
    }
    if (response.status === 401 || response.status === 403) {
      if (this.current.kind !== 'signedOut') {
        this.end({ kind: 'signedOut', reason: 'anonymous' });
      }
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
