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

async function signedInTab(shared: Browser): Promise<Session> {
  const session = tab(shared);
  await session.signIn(first.email, first.password);
  return session;
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
