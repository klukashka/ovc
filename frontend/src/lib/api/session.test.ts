import { describe, expect, test } from 'vitest';

import { FakeAuthServer, type ManualScheduler, manualScheduler, memoryLocks, unsharedLocks } from './fakeTransport.ts';
import { type LockManagerLike, Session } from './session.ts';

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

  test('a signed-in tab whose account is blocked ends on its next request', async () => {
    const shared = browser();
    const session = await signedInTab(shared);
    shared.server.block(first.email);

    const answer = await readAccount(session);

    expect(answer.status).toBe(401);
    expect(session.snapshot()).toEqual({ kind: 'signedOut', reason: 'anonymous' });
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
