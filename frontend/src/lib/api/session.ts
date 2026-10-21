import { createContext, useContext, useSyncExternalStore } from 'react';

import { type ApiError, apiErrorFrom } from './client.ts';
import { type components, meLanguageValues, meRoleValues, meStateValues, meThemeValues } from './schema.ts';

export type Me = components['schemas']['Me'];

type AccessToken = components['schemas']['AccessToken'];

export interface LockManagerLike {
  request<Result>(name: string, callback: () => Promise<Result>): Promise<Result>;
}

export interface Scheduler {
  schedule(milliseconds: number, task: () => void): () => void;
}

export type SignedOutReason = 'anonymous' | 'signedOut' | 'blocked';

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
  readonly fetch: typeof fetch;
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
    const response = await this.deps.fetch(this.withToken(request));
    if (epoch !== this.epoch) {
      return response;
    }
    if (response.status === 401 && (await errorOf(response.clone())).code === 'token_expired') {
      const renewed = this.token !== null && this.token !== sentToken ? true : await this.refresh();
      if (renewed && epoch === this.epoch) {
        const retried = await this.deps.fetch(this.withToken(retry));
        return epoch === this.epoch ? this.watch(retried) : retried;
      }
    }
    return this.watch(response);
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
    const response = await this.deps.fetch(`${this.deps.origin}/api/v1/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password }),
    });
    if (!response.ok) {
      return { kind: 'refused', error: await errorOf(response), retryAfterSeconds: retryAfter(response) };
    }
    const token: unknown = await response.json();
    if (epoch !== this.epoch) {
      throw new Error('the session changed while signing in');
    }
    this.acceptToken(token);
    await this.loadMe();
    if (this.current.kind !== 'signedIn') {
      throw new Error('the session ended while the account was loading');
    }
    return { kind: 'signedIn' };
  };

  readonly signOut = async (): Promise<void> => {
    const response = await this.authorizedFetch(`${this.deps.origin}/api/v1/auth/logout`, { method: 'POST' });
    if (!response.ok && response.status !== 401) {
      throw await errorOf(response);
    }
    this.end({ kind: 'signedOut', reason: 'signedOut' });
  };

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

  private async watch(response: Response): Promise<Response> {
    if (response.status === 403 && (await errorOf(response.clone())).code === 'account_blocked') {
      this.end({ kind: 'signedOut', reason: 'blocked' });
    } else if (response.status === 401 && this.current.kind === 'signedIn') {
      this.end({ kind: 'signedOut', reason: 'anonymous' });
    }
    return response;
  }

  private async refreshNow(epoch: number): Promise<boolean> {
    const response = await this.deps.fetch(`${this.deps.origin}/api/v1/auth/refresh`, {
      method: 'POST',
      signal: AbortSignal.timeout(refreshTimeoutMilliseconds),
    });
    if (epoch !== this.epoch) {
      return false;
    }
    if (response.status === 200) {
      const token: unknown = await response.json();
      if (epoch !== this.epoch) {
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
    if (error.code === 'account_blocked') {
      this.end({ kind: 'signedOut', reason: 'blocked' });
      return false;
    }
    throw error;
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
    const response = await this.authorizedFetch(`${this.deps.origin}/api/v1/users/me`);
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
