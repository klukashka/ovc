import { describe, expect, test } from 'vitest';

import {
  FakeAuthServer,
  AccountResponseBodies,
  SignalAwareRefreshBodies,
  type ManualScheduler,
  manualScheduler,
  memoryLocks,
  unsharedLocks,
} from './fakeTransport.ts';
import { type LockManagerLike, Session, drainOwnedResponse, ownedResponseBodyLimit } from './session.ts';
import { FollowingQueryClient, accountCacheOwnership } from '../../app/queryClient.ts';

const origin = 'https://ovc.example';
const first = { id: 'account-a', name: 'A tutor', email: 'a@example.org', password: 'a long enough password' };
const second = { id: 'account-b', name: 'B tutor', email: 'b@example.org', password: 'another long password' };

interface Browser {
  readonly server: FakeAuthServer;
  readonly locks: LockManagerLike;
}

function browser(locks: LockManagerLike = memoryLocks()): Browser {
  const server = new FakeAuthServer();
  server.addAccount(first);
  server.addAccount(second);
  return { server, locks };
}

function tab(shared: Browser, scheduler: ManualScheduler = manualScheduler()): Session {
  return new Session({ origin, transport: shared.server.transport(), locks: shared.locks, scheduler });
}

function readAccount(session: Session): Promise<Response> {
  return session.authorizedFetch(`${origin}/api/v1/users/me`);
}

async function signedInTab(shared: Browser, scheduler: ManualScheduler = manualScheduler()): Promise<Session> {
  const session = tab(shared, scheduler);
  await session.signIn(first.email, first.password);
  return session;
}

async function tokenFor(shared: Browser, account: typeof second): Promise<unknown> {
  const response = await shared.server.transport().login(account.email, account.password);
  return response.json();
}

function signedInAs(session: Session): string | null {
  const state = session.snapshot();
  return state.kind === 'signedIn' ? state.me.id : null;
}

async function tokensSentByNextRequest(shared: Browser, session: Session): Promise<readonly string[]> {
  const mark = shared.server.sentTokens.length;
  await readAccount(session);
  return sentSince(shared, mark);
}

function sentSince(shared: Browser, mark: number): readonly string[] {
  return shared.server.sentTokens.slice(mark);
}

describe('the session refresh', () => {
  test('concurrent requests with an expired token share one refresh', async () => {
    const shared = browser();
    const session = await signedInTab(shared);
    shared.server.advance(16 * 60);

    const answers = await Promise.all([readAccount(session), readAccount(session), readAccount(session)]);

    expect(answers.map((answer) => answer.status)).toEqual([200, 200, 200]);
    expect(shared.server.refreshes).toBe(1);
  });

  test('an unexpired revoked session ends after account refusal without a refresh', async () => {
    const shared = browser();
    const session = await signedInTab(shared);
    const refreshes = shared.server.refreshes;
    shared.server.revokeSessionsOf(first.email);

    await session.reloadAccount();

    expect(session.snapshot().kind).toBe('signedOut');
    expect(shared.server.refreshes).toBe(refreshes);
  });

  test('two tabs refreshing at once with the shared lock keep the session', async () => {
    const shared = browser();
    await signedInTab(shared);
    const tabs = [tab(shared), tab(shared)];

    await Promise.all(tabs.map((each) => each.start()));

    expect(tabs.map((each) => each.snapshot().kind)).toEqual(['signedIn', 'signedIn']);
    expect(shared.server.reuseDetected).toBe(false);
  });

  test('two tabs refreshing at once without a shared lock lose the session to reuse detection', async () => {
    const shared = browser(unsharedLocks());
    await signedInTab(shared);
    const tabs = [tab(shared), tab(shared)];

    await Promise.all(tabs.map((each) => each.start()));

    expect(shared.server.reuseDetected).toBe(true);
    expect(tabs.map((each) => each.snapshot().kind)).toContain('signedOut');
  });

  test('a failed account check fails the request without sending the unchecked token', async () => {
    const shared = browser();
    const session = await signedInTab(shared);
    shared.server.advance(16 * 60);
    shared.server.failNextAccountCheck();
    const mark = shared.server.sentTokens.length;

    const request = readAccount(session);

    await expect(request).rejects.toMatchObject({ status: 503, code: 'service_unavailable' });
    expect(sentSince(shared, mark)).toEqual(['fake-access-1']);
    expect(session.snapshot().kind).toBe('signedIn');
  });

  test('after a failed account check the next request still starts from the old token', async () => {
    const shared = browser();
    const session = await signedInTab(shared);
    shared.server.advance(16 * 60);
    shared.server.failNextAccountCheck();
    await readAccount(session).catch((error: unknown) => error);
    const mark = shared.server.sentTokens.length;

    const answer = await readAccount(session);

    expect(answer.status).toBe(200);
    expect(sentSince(shared, mark)).toEqual(['fake-access-1', 'fake-access-3']);
    expect(shared.server.refreshes).toBe(2);
  });

  test('a refresh that returns another account ends the tab instead of acting as that account', async () => {
    const shared = browser();
    const session = await signedInTab(shared);
    await tab(shared).signIn(second.email, second.password);
    shared.server.advance(16 * 60);
    const mark = shared.server.sentTokens.length;

    const answer = await readAccount(session);

    expect(answer.status).toBe(401);
    expect(session.snapshot()).toEqual({ kind: 'signedOut', reason: 'anonymous' });
    expect(sentSince(shared, mark).map((token) => shared.server.issuedBy(token))).toEqual([first.id]);
  });

  test('a refresh answering with a token after sign-out brings no token back', async () => {
    const shared = browser();
    const session = await signedInTab(shared);
    const paused = shared.server.pauseRefreshes();
    const late = session.refresh();
    await paused.answered;
    await session.signOut();
    paused.resume();
    await late;
    const mark = shared.server.sentTokens.length;

    await readAccount(session);

    expect(shared.server.refreshesGranted).toBe(1);
    expect(sentSince(shared, mark)).toEqual(['']);
    expect(session.snapshot()).toEqual({ kind: 'signedOut', reason: 'signedOut' });
  });
});

describe('the session after it changes hands', () => {
  test('an expired answer that arrives after an adoption leaves the adopted session signed in', async () => {
    const shared = browser();
    const session = await signedInTab(shared);
    shared.server.advance(16 * 60);
    const paused = shared.server.pauseRefreshes();
    const late = readAccount(session);
    await paused.answered;
    await session.adopt(await tokenFor(shared, second));

    paused.resume();
    await late;

    expect(signedInAs(session)).toBe(second.id);
  });

  test('an account answer that arrives after sign-out does not sign the tab back in', async () => {
    const shared = browser();
    const session = await signedInTab(shared);
    const paused = shared.server.pauseAccountReads();
    const reload = session.reloadAccount();
    await paused.answered;
    await session.signOut();

    paused.resume();
    await reload;

    expect(session.snapshot()).toEqual({ kind: 'signedOut', reason: 'signedOut' });
  });

  test('a stale refusal of the account that arrives after an adoption leaves the adopted session signed in', async () => {
    const shared = browser();
    const session = await signedInTab(shared);
    shared.server.block(first.email);
    const paused = shared.server.pauseAccountReads();
    const reload = session.reloadAccount();
    await paused.answered;
    const adoption = session.adopt(await tokenFor(shared, second));

    paused.resume();
    await Promise.all([reload, adoption]);

    expect(signedInAs(session)).toBe(second.id);
  });

  test('an adoption whose session was already revoked stays signed out and keeps no token', async () => {
    const shared = browser();
    const session = tab(shared);
    await session.start();
    const token = await tokenFor(shared, second);
    shared.server.block(second.email);

    await session.adopt(token);

    expect(session.snapshot()).toEqual({ kind: 'signedOut', reason: 'anonymous' });
    expect(await tokensSentByNextRequest(shared, session)).toEqual(['']);
  });

  test('an adoption whose account cannot be read shows unavailable without a token, and a restart signs in', async () => {
    const shared = browser();
    const session = tab(shared);
    await session.start();
    shared.server.failNextAccountRead();

    const adoption = session.adopt(await tokenFor(shared, second));

    await expect(adoption).rejects.toMatchObject({ status: 503, code: 'service_unavailable' });
    expect(session.snapshot()).toEqual({ kind: 'unavailable' });
    expect(await tokensSentByNextRequest(shared, session)).toEqual(['']);
    await session.start();
    expect(signedInAs(session)).toBe(second.id);
  });
});

describe('the session when the server fails', () => {
  test('a sign-in whose account cannot be read shows unavailable without a token, and a restart signs in', async () => {
    const shared = browser();
    const session = tab(shared);
    shared.server.failNextAccountRead();

    const signIn = session.signIn(first.email, first.password);

    await expect(signIn).rejects.toMatchObject({ status: 503, code: 'service_unavailable' });
    expect(session.snapshot()).toEqual({ kind: 'unavailable' });
    expect(await tokensSentByNextRequest(shared, session)).toEqual(['']);
    await session.start();
    expect(signedInAs(session)).toBe(first.id);
  });

  test('a start-up the server cannot answer shows unavailable, and starting again signs in', async () => {
    const shared = browser();
    await signedInTab(shared);
    const session = tab(shared);
    shared.server.failNextRefresh();

    await session.start();
    const failed = session.snapshot();
    await session.start();

    expect(failed).toEqual({ kind: 'unavailable' });
    expect(signedInAs(session)).toBe(first.id);
  });

  test('a failed proactive refresh is retried after 30 seconds and the retry renews the token', async () => {
    const shared = browser();
    const scheduler = manualScheduler();
    const session = await signedInTab(shared, scheduler);
    shared.server.failNextRefresh();
    const retryScheduled = scheduler.nextSchedule();
    scheduler.runAll();
    const retryDelay = await retryScheduled;
    const renewed = scheduler.nextSchedule();

    scheduler.runAll();
    await renewed;

    expect(retryDelay).toBe(30_000);
    expect(shared.server.refreshes).toBe(2);
    expect(shared.server.refreshesGranted).toBe(1);
    expect(signedInAs(session)).toBe(first.id);
  });

  test('a token whose lifetime passed in wall-clock time while timers were suspended is renewed before use', async () => {
    const shared = browser();
    const scheduler = manualScheduler();
    const session = await signedInTab(shared, scheduler);
    const [tokenBeforeSleep] = await tokensSentByNextRequest(shared, session);
    shared.server.advance(16 * 60);
    scheduler.suspend(16 * 60 * 1000);

    const token = await session.freshToken(30_000, session.currentEpoch());

    expect(shared.server.refreshes).toBe(1);
    expect(token).not.toBeNull();
    expect(token).not.toBe(tokenBeforeSleep);
    expect(scheduler.pendingDelays()).toEqual([720_000]);
    expect(await tokensSentByNextRequest(shared, session)).toEqual([token]);
  });

  test('a signed-in tab whose account is blocked ends on its next request', async () => {
    const shared = browser();
    const session = await signedInTab(shared);
    shared.server.block(first.email);

    const answer = await readAccount(session);

    expect(answer.status).toBe(401);
    expect(session.snapshot()).toEqual({ kind: 'signedOut', reason: 'sessionEnded' });
  });

  test('signing in to a blocked account is refused as account_blocked', async () => {
    const shared = browser();
    shared.server.block(first.email);
    const session = tab(shared);

    const result = await session.signIn(first.email, first.password);

    expect(result).toMatchObject({ kind: 'refused', error: { status: 403, code: 'account_blocked' } });
    expect(session.snapshot().kind).not.toBe('signedIn');
  });
});

describe('the session bearer', () => {
  test.each([
    ['another origin', 'https://elsewhere.example/api/v1/users/me'],
    ['a non-API path of the OVC origin', `${origin}/assets/logo.svg`],
  ])('is not attached to %s', async (_name, address) => {
    const shared = browser();
    const session = await signedInTab(shared);
    const mark = shared.server.sentTokens.length;

    await session.authorizedFetch(address);

    expect(sentSince(shared, mark)).toEqual(['']);
  });
});

describe('session', () => {
  test('refresh reports blocked or sessionEnded', async () => {
    const revoked = browser();
    const revokedTab = await signedInTab(revoked);
    revoked.server.revokeSessionsOf(first.email);
    const blocked = browser();
    const blockedTab = await signedInTab(blocked);
    blocked.server.blockWithoutRevoking(first.email);

    const results = await Promise.all([revokedTab.refresh(), blockedTab.refresh()]);

    expect(results).toEqual([
      { kind: 'ended', reason: 'sessionEnded' },
      { kind: 'ended', reason: 'blocked' },
    ]);
    expect([revokedTab.snapshot(), blockedTab.snapshot()]).toEqual([
      { kind: 'signedOut', reason: 'sessionEnded' },
      { kind: 'signedOut', reason: 'blocked' },
    ]);
  });

  test('a socket refusal while the tab ends its own session waits for it, and refreshes only if it failed', async () => {
    const ended = browser();
    const endedTab = await signedInTab(ended);
    const refreshesBeforeEnding = ended.server.refreshes;
    const failed = browser();
    const failedTab = await signedInTab(failed);
    const refreshesBeforeFailure = failed.server.refreshes;
    const ending = endedTab.whileEnding(() => endedTab.signOut());
    const failing = failedTab.whileEnding(() => Promise.reject(new Error('the sign-out answer was lost')));

    const results = await Promise.all([
      endedTab.refreshAfterSocketRefusal(),
      failedTab.refreshAfterSocketRefusal(),
      ending,
      failing.catch(() => undefined),
    ]);

    expect(results.slice(0, 2)).toEqual([{ kind: 'superseded' }, { kind: 'renewed' }]);
    expect(ended.server.refreshes).toBe(refreshesBeforeEnding);
    expect(failed.server.refreshes).toBe(refreshesBeforeFailure + 1);
  });

  test('start keeps a session that a revocation replaced meanwhile', async () => {
    const shared = browser();
    await signedInTab(shared);
    const session = tab(shared);
    const paused = shared.server.pauseRefreshes();
    shared.server.loseNextRefresh();
    const starting = session.start();
    await paused.answered;
    session.endRevoked('signedOut');

    paused.resume();
    await starting;

    expect(session.snapshot()).toEqual({ kind: 'signedOut', reason: 'signedOut' });
  });
});

describe('authenticated upload lifetime', () => {
  test('cancelling while token refresh is pending prevents the upload replay', async () => {
    const shared = browser();
    const session = await signedInTab(shared);
    shared.server.advance(16 * 60);
    const paused = shared.server.pauseRefreshes();
    const controller = new AbortController();
    const body = new FormData();
    body.append('file', new File(['text'], 'notes.txt', { type: 'text/plain' }));
    const before = shared.server.sentTokens.length;
    const upload = session.authorizedUpload(
      new Request(`${origin}/api/v1/uploads?context=chat&scope=channel:one`, {
        method: 'POST',
        signal: controller.signal,
      }),
      { body, progress: (): void => undefined },
    );
    const settled = upload.catch((error: unknown): unknown => error);
    await paused.answered;

    controller.abort();
    paused.resume();
    const outcome = await settled;

    expect(outcome).toBeInstanceOf(DOMException);
    expect(shared.server.sentTokens).toHaveLength(before + 1);
    expect(signedInAs(session)).toBe(first.id);
  });

  test('an upload never sends credentials or a file to a foreign origin', async () => {
    const shared = browser();
    const session = await signedInTab(shared);
    const before = shared.server.sentTokens.length;
    const body = new FormData();
    body.append('file', new File(['text'], 'notes.txt'));

    const attempt = (): Promise<Response> =>
      session.authorizedUpload(new Request('https://outside.example/api/v1/uploads', { method: 'POST' }), {
        body,
        progress: (): void => undefined,
      });

    expect(attempt).toThrow('uploads require the own API');
    expect(shared.server.sentTokens).toHaveLength(before);
  });
});

async function heldRefusal(): Promise<{
  readonly shared: Browser;
  readonly session: Session;
  readonly body: SignalAwareRefreshBodies;
}> {
  const body = new SignalAwareRefreshBodies({ hold: true });
  const server = new FakeAuthServer({ refreshBodies: body });
  server.addAccount(first);
  server.addAccount(second);
  const shared = { server, locks: memoryLocks() };
  const session = await signedInTab(shared);
  server.revokeSessionsOf(first.email);
  return { shared, session, body };
}

function waitSignedOut(session: Session): Promise<void> {
  return new Promise((resolve) => {
    const check = (): void => {
      if (session.snapshot().kind === 'signedOut') {
        unsubscribe();
        resolve();
      }
    };
    const unsubscribe = session.subscribe(check);
    check();
  });
}

test.each(['release', 'fail'] as const)(
  'refusal immediately ends ownership and its %s cannot restore it',
  async (outcome) => {
    const { shared, session, body } = await heldRefusal();

    const pending = session.refresh();
    const result = pending.then(
      (value) => ({ kind: 'resolved' as const, value }),
      () => ({ kind: 'rejected' as const }),
    );
    await body.headers;
    await waitSignedOut(session);
    await body.reading;
    const whileHeld = session.snapshot();
    if (outcome === 'release') body.release();
    else body.fail();
    const settled = await result;

    expect(whileHeld).toEqual({ kind: 'signedOut', reason: 'sessionEnded' });
    expect(settled.kind).toBe(outcome === 'release' ? 'resolved' : 'rejected');
    expect(session.snapshot()).toEqual(whileHeld);
    expect(shared.server.refreshes).toBe(1);
    expect(body.snapshot()).toMatchObject({ active: 0, readerLocked: false });
  },
);

test.each(['release', 'fail'] as const)(
  'queued old-epoch refresh cannot send after another account is adopted and old body %s',
  async (outcome) => {
    const { shared, session, body } = await heldRefusal();
    const firstOperation = session.refresh();
    const firstResult = firstOperation.then(
      (value) => ({ kind: 'resolved' as const, value }),
      () => ({ kind: 'rejected' as const }),
    );
    await body.headers;
    await waitSignedOut(session);
    await body.reading;

    const queued = session.refresh();
    await session.adopt(await tokenFor(shared, second));
    const before = shared.server.refreshes;
    if (outcome === 'release') body.release();
    else body.fail();
    const ended = await firstResult;
    const queuedResult = await queued;
    const owner = await readAccount(session);

    expect(before).toBe(1);
    expect(shared.server.refreshes).toBe(before);
    expect(queuedResult.kind).toBe('superseded');
    expect(ended.kind).toBe(outcome === 'release' ? 'resolved' : 'rejected');
    if (ended.kind === 'resolved') expect(ended.value.kind).toBe('superseded');
    expect(signedInAs(session)).toBe(second.id);
    expect(await owner.json()).toMatchObject({ id: second.id });
    expect(body.snapshot()).toMatchObject({ active: 0, readerLocked: false });
  },
);

test.each([64 * 1024, 64 * 1024 + 1])('refresh refusal stream enforces its exact %s-byte boundary', async (bytes) => {
  const body = new SignalAwareRefreshBodies({ bytes });
  const shared = { server: new FakeAuthServer({ refreshBodies: body }), locks: memoryLocks() };
  shared.server.addAccount(first);
  const session = await signedInTab(shared);
  shared.server.revokeSessionsOf(first.email);

  const settled = await session.refresh().then(
    (value) => ({ kind: 'resolved' as const, value }),
    () => ({ kind: 'rejected' as const }),
  );

  expect(settled.kind).toBe(bytes === 64 * 1024 ? 'resolved' : 'rejected');
  expect(session.snapshot()).toEqual({ kind: 'signedOut', reason: 'sessionEnded' });
  expect(body.snapshot()).toMatchObject({ active: 0, readerLocked: false, cancelled: bytes === 64 * 1024 ? 0 : 1 });
});

test('empty refresh 204 retains anonymous semantics without a body reader', async () => {
  const body = new SignalAwareRefreshBodies();
  const shared = { server: new FakeAuthServer({ refreshBodies: body }), locks: memoryLocks() };
  const session = tab(shared);

  const result = await session.refresh();

  expect(result).toEqual({ kind: 'ended', reason: 'anonymous' });
  expect(session.snapshot()).toEqual({ kind: 'signedOut', reason: 'anonymous' });
  expect(body.snapshot()).toMatchObject({ active: 0, readerLocked: false });
});

test.each(['release', 'fail'] as const)(
  'a protected request cannot replay after an old refusal body %s and account replacement',
  async (outcome) => {
    const body = new SignalAwareRefreshBodies({ hold: true });
    const shared = { server: new FakeAuthServer({ refreshBodies: body }), locks: memoryLocks() };
    shared.server.addAccount(first);
    shared.server.addAccount(second);
    const session = await signedInTab(shared);
    shared.server.advance(16 * 60);
    const accountRead = shared.server.pauseAccountReads();
    const mark = shared.server.sentTokens.length;

    const pending = readAccount(session).then(
      (response) => ({ kind: 'resolved' as const, response }),
      () => ({ kind: 'rejected' as const }),
    );
    await accountRead.answered;
    shared.server.revokeSessionsOf(first.email);
    accountRead.resume();
    await body.headers;
    await waitSignedOut(session);
    await body.reading;
    const oldRequestsBeforeAdopt = shared.server.sentTokens.length - mark;
    await session.adopt(await tokenFor(shared, second));
    const beforeCompletion = shared.server.sentTokens.length;
    if (outcome === 'release') body.release();
    else body.fail();
    const result = await pending;
    const replayCount = shared.server.sentTokens.length - beforeCompletion;
    const next = await readAccount(session);

    expect(oldRequestsBeforeAdopt).toBe(1);
    expect(replayCount).toBe(0);
    expect(result.kind).toBe(outcome === 'release' ? 'resolved' : 'rejected');
    if (result.kind === 'resolved') expect(result.response.status).toBe(401);
    expect(await next.json()).toMatchObject({ id: second.id });
    expect(signedInAs(session)).toBe(second.id);
    expect(shared.server.refreshes).toBe(1);
    expect(body.snapshot()).toMatchObject({ active: 0, readerLocked: false });
  },
);

async function accountBodyFixture(): Promise<{
  readonly shared: Browser;
  readonly session: Session;
  readonly bodies: AccountResponseBodies;
}> {
  const bodies = new AccountResponseBodies();
  const shared = { server: new FakeAuthServer({ accountBodies: bodies }), locks: memoryLocks() };
  shared.server.addAccount(first);
  shared.server.addAccount(second);
  return { shared, session: await signedInTab(shared), bodies };
}

test.each([
  ['loadMe', 200],
  ['loadMe', 401],
  ['ownerOf', 200],
  ['ownerOf', 401],
] as const)('owned account lifecycle: stale %s %s completes the original body', async (owner, status) => {
  const { shared, session, bodies } = await accountBodyFixture();
  const body = new SignalAwareRefreshBodies({ hold: true, holdHeaders: true });
  bodies.arm(body);
  const refreshHold = owner === 'ownerOf' && status === 401 ? shared.server.pauseRefreshes() : null;
  if (owner === 'loadMe' && status === 401) shared.server.advance(16 * 60);
  const pending = owner === 'loadMe' ? session.reloadAccount() : session.refresh();
  if (refreshHold !== null) {
    await refreshHold.answered;
    shared.server.revokeSessionsOf(first.email);
    refreshHold.resume();
  }
  await body.headers;

  session.endRevoked('signedOut');
  await session.adopt(await tokenFor(shared, second));
  body.releaseHeaders();
  body.release();
  await pending;
  const lifecycle = body.snapshot();
  await body.dispose();

  expect(lifecycle).toMatchObject({ consumed: true, active: 0, completed: 1, readerLocked: false });
  expect(signedInAs(session)).toBe(second.id);
  expect(shared.server.refreshes).toBe(owner === 'ownerOf' ? 1 : 0);
});

test.each([401, 403] as const)(
  'owned account lifecycle: current loadMe %s completes the original body',
  async (status) => {
    const { shared, session, bodies } = await accountBodyFixture();
    const body = new SignalAwareRefreshBodies();
    const cache = new FollowingQueryClient();
    const ownership = accountCacheOwnership(cache);
    const initial = session.snapshot();
    if (initial.kind !== 'signedIn') throw new Error('expected signed-in fixture');
    ownership(initial.me);
    cache.setQueryData(['private'], 'old');
    let disposedAtInvalidation = true;
    const unsubscribe = session.subscribe(() => {
      const state = session.snapshot();
      ownership(state.kind === 'signedIn' ? state.me : null);
      if (state.kind === 'signedOut') disposedAtInvalidation = body.snapshot().consumed;
    });
    bodies.arm(body);
    if (status === 401) shared.server.revokeSessionsOf(first.email);
    else shared.server.blockWithoutRevoking(first.email);

    await session.reloadAccount();
    const retained = cache.getQueryData(['private']);
    unsubscribe();
    cache.clear();

    expect(disposedAtInvalidation).toBe(false);
    expect(retained).toBeUndefined();
    expect(body.snapshot()).toMatchObject({ consumed: true, active: 0, completed: 1, readerLocked: false });
    expect(session.snapshot().kind).toBe('signedOut');
    expect(shared.server.refreshes).toBe(0);
  },
);

function settledOperation(
  operation: Promise<unknown>,
): Promise<{ readonly failed: boolean; readonly error?: unknown }> {
  return operation.then(
    () => ({ failed: false }),
    (error: unknown) => ({ failed: true, error }),
  );
}

test.each(['loadMe', 'ownerOf'] as const)(
  'owned account lifecycle: %s preserves replacement cache through bounded cleanup',
  async (owner) => {
    for (const outcome of ['limit', 'overflow', 'readFailure', 'cancelFailure'] as const) {
      const { shared, session, bodies } = await accountBodyFixture();
      const body = new SignalAwareRefreshBodies({
        hold: true,
        holdHeaders: true,
        bytes: outcome === 'overflow' || outcome === 'cancelFailure' ? 64 * 1024 + 1 : 64 * 1024,
        cancellationFails: outcome === 'cancelFailure',
      });
      const cache = new FollowingQueryClient();
      const ownership = accountCacheOwnership(cache);
      const synchronize = (): void => {
        const state = session.snapshot();
        ownership(state.kind === 'signedIn' ? state.me : null);
      };
      synchronize();
      const unsubscribe = session.subscribe(synchronize);
      cache.setQueryData(['private'], 'old');
      bodies.arm(body);
      const pending = settledOperation(owner === 'loadMe' ? session.reloadAccount() : session.refresh());
      await body.headers;

      session.endRevoked('signedOut');
      const clearedBeforeDisposal = cache.getQueryData(['private']);
      await session.adopt(await tokenFor(shared, second));
      cache.setQueryData(['private'], 'replacement');
      const requestsBeforeDisposal = shared.server.sentTokens.length;
      body.releaseHeaders();
      await body.reading;
      const whileReading = body.snapshot();
      if (outcome === 'readFailure') body.fail();
      else body.release();
      const result = await pending;
      const retained = cache.getQueryData(['private']);
      unsubscribe();
      cache.clear();

      expect(clearedBeforeDisposal).toBeUndefined();
      expect(whileReading).toMatchObject({ active: 1, readerLocked: true, consumed: true });
      expect(result.failed).toBe(outcome !== 'limit');
      expect(body.snapshot()).toMatchObject({ active: 0, readerLocked: false });
      expect(retained).toBe('replacement');
      expect(signedInAs(session)).toBe(second.id);
      expect(shared.server.sentTokens).toHaveLength(requestsBeforeDisposal);
      expect(shared.server.refreshes).toBe(owner === 'ownerOf' ? 1 : 0);
      if (outcome === 'cancelFailure') {
        expect(result.error).toBeInstanceOf(AggregateError);
        if (!(result.error instanceof AggregateError)) throw new Error('expected aggregate cleanup failure');
        expect(result.error.errors.map((error: unknown) => (error instanceof Error ? error.message : ''))).toEqual([
          'account_body_overflow',
          'account_body_cancellation_failed',
        ]);
        expect(body.snapshot().cancelled).toBe(1);
      }
      if (outcome === 'readFailure') expect(body.snapshot().readFailures).toBe(1);
      if (outcome === 'limit') expect(body.snapshot().completed).toBe(1);
    }
  },
);

test('owned account lifecycle: abort settles the owned reader and retains failure evidence', async () => {
  const body = new SignalAwareRefreshBodies({ hold: true });
  const controller = new AbortController();
  const response = await body.response(new Response('account data'), controller.signal);
  const pending = settledOperation(drainOwnedResponse(response, controller.signal, 'account'));
  await body.reading;

  controller.abort();
  const result = await pending;

  expect(result.failed).toBe(true);
  expect(result.error).toBeInstanceOf(AggregateError);
  expect(body.snapshot()).toMatchObject({ active: 0, aborted: 1, readerLocked: false, consumed: true });
});

test('owned account lifecycle: a declared body within the limit is read whole without a released reader', async () => {
  const response = new Response('account data', { headers: { 'Content-Length': '12' } });

  await drainOwnedResponse(response, new AbortController().signal, 'account');

  expect(response.bodyUsed).toBe(true);
  expect(response.body?.locked).toBe(true);
});

test('owned account lifecycle: abort settles a declared body and retains failure evidence', async () => {
  const body = new SignalAwareRefreshBodies({ hold: true, declaresLength: true });
  const controller = new AbortController();
  const response = await body.response(new Response('account data'), controller.signal);
  const pending = settledOperation(drainOwnedResponse(response, controller.signal, 'account'));
  await body.reading;

  controller.abort();
  const result = await pending;

  expect(result.error).toBeInstanceOf(AggregateError);
  if (!(result.error instanceof AggregateError)) throw new Error('expected aggregate abort failure');
  expect(result.error.errors.map((error: unknown) => (error instanceof Error ? error.message : ''))).toEqual([
    'account_body_aborted',
  ]);
  expect(body.snapshot()).toMatchObject({ active: 0, aborted: 1, readerLocked: true });
});

test('owned account lifecycle: abort settles a declared drain under a signal not bound to its body', async () => {
  const response = new Response(new ReadableStream<Uint8Array>(), { headers: { 'Content-Length': '12' } });
  const controller = new AbortController();
  const pending = settledOperation(drainOwnedResponse(response, controller.signal, 'account'));

  controller.abort();
  const result = await pending;

  expect(result.error).toBeInstanceOf(AggregateError);
  if (!(result.error instanceof AggregateError)) throw new Error('expected aggregate abort failure');
  expect(result.error.errors.map((error: unknown) => (error instanceof Error ? error.message : ''))).toEqual([
    'account_body_aborted',
  ]);
});

test.each([
  ['over the limit', { 'Content-Length': String(ownedResponseBodyLimit + 1) }],
  ['malformed', { 'Content-Length': '12, 12' }],
  ['non-decimal', { 'Content-Length': '1e3' }],
  ['absent', {}],
  ['compressed', { 'Content-Length': '12', 'Content-Encoding': 'gzip' }],
  ['chunked', { 'Content-Length': '12', 'Transfer-Encoding': 'chunked' }],
])('owned account lifecycle: a %s declaration keeps the streamed limit', async (_name, headers) => {
  const oversized = new Uint8Array(ownedResponseBodyLimit + 1);
  const response = new Response(oversized, { headers });

  const result = await settledOperation(drainOwnedResponse(response, new AbortController().signal, 'account'));

  expect(result.failed).toBe(true);
  expect(result.error).toBeInstanceOf(AggregateError);
  if (!(result.error instanceof AggregateError)) throw new Error('expected aggregate overflow failure');
  expect(result.error.errors.map((error: unknown) => (error instanceof Error ? error.message : ''))).toEqual([
    'account_body_overflow',
  ]);
  expect(response.body?.locked).toBe(false);
});

test('owned account lifecycle: a declared body larger than the limit fails after the read', async () => {
  const response = new Response(new Uint8Array(ownedResponseBodyLimit + 1), { headers: { 'Content-Length': '12' } });

  const result = await settledOperation(drainOwnedResponse(response, new AbortController().signal, 'account'));

  expect(result.error).toBeInstanceOf(AggregateError);
  if (!(result.error instanceof AggregateError)) throw new Error('expected aggregate overflow failure');
  expect(result.error.errors.map((error: unknown) => (error instanceof Error ? error.message : ''))).toEqual([
    'account_body_overflow',
  ]);
  expect(response.body?.locked).toBe(true);
});

test('owned account lifecycle: a declared body of exactly the limit completes', async () => {
  const response = new Response(new Uint8Array(ownedResponseBodyLimit), {
    headers: { 'Content-Length': String(ownedResponseBodyLimit) },
  });

  const result = await settledOperation(drainOwnedResponse(response, new AbortController().signal, 'account'));

  expect(result.failed).toBe(false);
  expect(response.body?.locked).toBe(true);
});

test('owned account lifecycle: a failed read of a declared body is a read failure', async () => {
  const failing = new ReadableStream<Uint8Array>({
    pull: (controller): void => {
      controller.error(new Error('read failed'));
    },
  });
  const response = new Response(failing, { headers: { 'Content-Length': '12' } });

  const result = await settledOperation(drainOwnedResponse(response, new AbortController().signal, 'account'));

  expect(result.error).toBeInstanceOf(AggregateError);
  if (!(result.error instanceof AggregateError)) throw new Error('expected aggregate read failure');
  expect(result.error.errors.map((error: unknown) => (error instanceof Error ? error.message : ''))).toEqual([
    'account_body_read_failed',
  ]);
});

test('owned account lifecycle: a replacement account survives the drain of a declared body', async () => {
  const { shared, session, bodies } = await accountBodyFixture();
  const body = new SignalAwareRefreshBodies({ hold: true, holdHeaders: true, declaresLength: true });
  bodies.arm(body);
  const pending = settledOperation(session.reloadAccount());
  await body.headers;

  session.endRevoked('signedOut');
  await session.adopt(await tokenFor(shared, second));
  body.releaseHeaders();
  await body.reading;
  const whileReading = signedInAs(session);
  body.release();
  await pending;

  expect(whileReading).toBe(second.id);
  expect(signedInAs(session)).toBe(second.id);
  expect(body.snapshot()).toMatchObject({ consumed: true, active: 0, completed: 1, readerLocked: true });
});

test('owned account lifecycle: an arbitrary API caller still owns its response', async () => {
  const { shared, session, bodies } = await accountBodyFixture();
  const body = new SignalAwareRefreshBodies({ hold: true, holdHeaders: true });
  bodies.arm(body);
  const pending = readAccount(session);
  await body.headers;

  session.endRevoked('signedOut');
  await session.adopt(await tokenFor(shared, second));
  body.releaseHeaders();
  const response = await pending;
  const beforeCallerRead = body.snapshot();
  body.release();
  const account: unknown = await response.json();

  expect(beforeCallerRead).toMatchObject({ consumed: false, active: 1, completed: 0 });
  expect(account).toMatchObject({ id: first.id });
  expect(signedInAs(session)).toBe(second.id);
  expect(body.snapshot()).toMatchObject({ active: 0, completed: 1 });
});
