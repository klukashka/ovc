import { QueryClient, QueryObserver } from '@tanstack/react-query';
import { describe, expect, test } from 'vitest';

import { createApiClient } from '../lib/api/client.ts';
import { markRead } from '../features/messages/api.ts';
import { scheduleChatReads } from '../features/messages/refetch.ts';
import { unreadChannelsKey } from '../features/messages/keys.ts';
import { latestNotificationsKey } from '../features/notifications/api.ts';
import { FakeAuthServer, type ManualScheduler, manualScheduler, memoryLocks } from '../lib/api/fakeTransport.ts';
import { Session } from '../lib/api/session.ts';
import { eventually } from '../lib/eventually.ts';
import { closeCodes } from '../lib/realtime/frames.ts';
import { FakeGateway } from '../lib/realtime/fakeGateway.ts';
import { ConnectionState } from './connectionState.ts';
import { resumeAfterBeforeUnloadMilliseconds, wireRealtime } from './realtime.ts';

const origin = 'https://ovc.example';
const tutor = { id: 'tutor-a', name: 'A tutor', email: 'a@example.org', password: 'a long enough password' };
const other = { id: 'tutor-b', name: 'B tutor', email: 'b@example.org', password: 'another long password' };

interface WiredPage {
  readonly server: FakeAuthServer;
  readonly gateway: FakeGateway;
  readonly session: Session;
  readonly queryClient: QueryClient;
  readonly events: EventTarget;
  readonly clock: ManualScheduler;
  readonly failures: readonly Error[];
  readonly connection: ConnectionState;
  readonly statusClock: ManualScheduler;
}

async function wiredPage(loaded: boolean): Promise<WiredPage> {
  const server = new FakeAuthServer();
  server.addAccount(tutor);
  server.addAccount(other);
  const gateway = new FakeGateway(server);
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(tutor.email, tutor.password);
  const queryClient = new QueryClient();
  const events = new EventTarget();
  const clock = manualScheduler();
  const failures: Error[] = [];
  const statusClock = manualScheduler();
  const connection = new ConnectionState(statusClock);
  connection.follow(session);
  wireRealtime({
    session,
    connection,
    queryClient,
    sockets: gateway.sockets,
    scheduler: clock,
    page: { events, loaded: () => loaded },
    onFailure: (error) => {
      failures.push(error);
    },
  });
  return { server, gateway, session, queryClient, events, clock, failures, connection, statusClock };
}

async function connectedPage(): Promise<WiredPage> {
  const page = await wiredPage(true);
  await eventually(() => {
    expect(page.gateway.readySockets(tutor.id)).toBe(1);
  });
  return page;
}

async function pageWithOneFailedRead(page: WiredPage): Promise<{ readonly reads: () => number }> {
  let reads = 0;
  const observer = new QueryObserver(page.queryClient, {
    queryKey: latestNotificationsKey,
    queryFn: (): Promise<number> => {
      reads += 1;
      return reads === 1 ? Promise.reject(new Error('the first read fails')) : Promise.resolve(reads);
    },
    retry: false,
  });
  observer.subscribe(() => undefined);
  await eventually(() => {
    expect(observer.getCurrentResult().status).toBe('error');
  });
  return { reads: () => reads };
}

function restoredFromTheBackForwardCache(): Event {
  return Object.assign(new Event('pageshow'), { persisted: true });
}

async function tokenOf(server: FakeAuthServer, account: typeof other): Promise<unknown> {
  const response = await server.transport().login(account.email, account.password);
  return response.json();
}

describe('the realtime wiring of a page', () => {
  test('a beforeunload closes the socket and schedules one resume', async () => {
    const { gateway, events, clock } = await connectedPage();

    events.dispatchEvent(new Event('beforeunload'));

    expect(gateway.openSockets).toBe(0);
    expect(clock.pendingDelays()).toEqual([resumeAfterBeforeUnloadMilliseconds]);
  });

  test('the page still shown after the grace delay connects again as a reconnect', async () => {
    const page = await connectedPage();
    const failedRead = await pageWithOneFailedRead(page);
    page.events.dispatchEvent(new Event('beforeunload'));

    page.clock.pass(resumeAfterBeforeUnloadMilliseconds);

    await eventually(() => {
      expect(page.gateway.readySockets(tutor.id)).toBe(1);
      expect(failedRead.reads()).toBe(2);
    });
    expect(page.gateway.socketsOpened).toBe(2);
    expect(page.failures).toEqual([]);
  });

  test('two beforeunload events leave one resume and one socket', async () => {
    const { gateway, events, clock } = await connectedPage();
    events.dispatchEvent(new Event('beforeunload'));
    events.dispatchEvent(new Event('beforeunload'));
    const pendingAfterBoth = clock.pendingDelays();

    clock.pass(resumeAfterBeforeUnloadMilliseconds);

    expect(pendingAfterBoth).toEqual([resumeAfterBeforeUnloadMilliseconds]);
    await eventually(() => {
      expect(gateway.readySockets(tutor.id)).toBe(1);
    });
    expect(gateway.openSockets).toBe(1);
    expect(gateway.socketsOpened).toBe(2);
  });

  test('a sign-out before the resume opens no socket', async () => {
    const { gateway, session, events, clock } = await connectedPage();
    events.dispatchEvent(new Event('beforeunload'));
    await session.signOut();

    clock.pass(resumeAfterBeforeUnloadMilliseconds);

    expect(gateway.openSockets).toBe(0);
    expect(gateway.socketsOpened).toBe(1);
    expect(clock.pendingDelays()).toEqual([]);
  });

  test('a session that ended before the resume opens no socket', async () => {
    const { gateway, session, events, clock } = await connectedPage();
    events.dispatchEvent(new Event('beforeunload'));
    session.endRevoked('accountDeleted');

    clock.pass(resumeAfterBeforeUnloadMilliseconds);

    expect(gateway.openSockets).toBe(0);
    expect(gateway.socketsOpened).toBe(1);
    expect(clock.pendingDelays()).toEqual([]);
  });

  test('an account change before the resume connects only the new account', async () => {
    const { server, gateway, session, events, clock } = await connectedPage();
    events.dispatchEvent(new Event('beforeunload'));
    await session.adopt(await tokenOf(server, other));

    clock.pass(resumeAfterBeforeUnloadMilliseconds);

    await eventually(() => {
      expect(gateway.readySockets(other.id)).toBe(1);
    });
    expect(gateway.readySockets(tutor.id)).toBe(0);
    expect(gateway.openSockets).toBe(1);
    expect(gateway.tokensOfferedOnSocketsOf(other.id).map((token) => server.issuedBy(token))).toEqual([other.id]);
  });

  test('a beforeunload before the load event connects once, at the resume', async () => {
    const { gateway, events, clock } = await wiredPage(false);
    events.dispatchEvent(new Event('beforeunload'));
    events.dispatchEvent(new Event('load'));

    clock.pass(resumeAfterBeforeUnloadMilliseconds);

    await eventually(() => {
      expect(gateway.readySockets(tutor.id)).toBe(1);
    });
    expect(gateway.socketsOpened).toBe(1);
  });

  test('a pagehide after a beforeunload keeps the socket closed and leaves nothing scheduled', async () => {
    const { gateway, events, clock, failures } = await connectedPage();
    events.dispatchEvent(new Event('beforeunload'));

    events.dispatchEvent(new Event('pagehide'));

    expect(gateway.openSockets).toBe(0);
    expect(clock.pendingDelays()).toEqual([]);
    expect(gateway.socketsOpened).toBe(1);
    expect(failures).toEqual([]);
  });

  test('a pagehide alone closes the socket', async () => {
    const { gateway, events, clock } = await connectedPage();

    events.dispatchEvent(new Event('pagehide'));

    expect(gateway.openSockets).toBe(0);
    expect(clock.pendingDelays()).toEqual([]);
  });

  test('a restore from the back-forward cache connects again as a reconnect', async () => {
    const page = await connectedPage();
    const failedRead = await pageWithOneFailedRead(page);
    page.events.dispatchEvent(new Event('pagehide'));

    page.events.dispatchEvent(restoredFromTheBackForwardCache());

    await eventually(() => {
      expect(page.gateway.readySockets(tutor.id)).toBe(1);
      expect(failedRead.reads()).toBe(2);
    });
    expect(page.gateway.socketsOpened).toBe(2);
  });

  test('a pageshow that is not a restore leaves the socket as it is', async () => {
    const { gateway, events } = await connectedPage();

    events.dispatchEvent(new Event('pageshow'));

    expect(gateway.openSockets).toBe(1);
    expect(gateway.socketsOpened).toBe(1);
  });
});

describe('wired offline state', () => {
  test('includes the load wait and clears only after the gateway is ready', async () => {
    const page = await wiredPage(false);

    page.statusClock.pass(5000);
    const waiting = page.connection.snapshot();
    page.events.dispatchEvent(new Event('load'));
    await eventually(() => {
      expect(page.gateway.readySockets(tutor.id)).toBe(1);
    });

    expect(waiting).toBe('offline');
    expect(page.connection.snapshot()).toBe('recovered');
    expect(page.statusClock.pendingDelays()).toEqual([]);
  });

  test('a close starts the grace and refused reconnects do not reset it', async () => {
    const page = await connectedPage();
    page.gateway.refuseConnections(2);

    page.gateway.closeAccount(tutor.id, closeCodes.reconnect);
    await eventually(() => {
      expect(page.statusClock.pendingDelays()).toEqual([5000]);
    });
    page.statusClock.pass(4000);
    page.clock.pass(2000);
    await eventually(() => {
      expect(page.gateway.socketsOpened).toBe(2);
    });
    page.statusClock.pass(1000);

    expect(page.connection.snapshot()).toBe('offline');
    expect(page.statusClock.pendingDelays()).toEqual([]);
  });

  test('pagehide cancels the grace and BFCache restore reconnects', async () => {
    const page = await connectedPage();
    page.gateway.closeAccount(tutor.id, closeCodes.reconnect);

    page.events.dispatchEvent(new Event('pagehide'));
    const pendingWhileHidden = page.statusClock.pendingDelays();
    page.events.dispatchEvent(restoredFromTheBackForwardCache());
    await eventually(() => {
      expect(page.gateway.readySockets(tutor.id)).toBe(1);
    });

    expect(pendingWhileHidden).toEqual([]);
    expect(page.connection.snapshot()).toBe('waiting');
    expect(page.statusClock.pendingDelays()).toEqual([]);
  });

  test('sign-out clears visible offline state and leaves no status timer', async () => {
    const page = await wiredPage(false);
    page.statusClock.pass(5000);

    await page.session.signOut();
    page.statusClock.pass(10000);

    expect(page.connection.snapshot()).toBe('waiting');
    expect(page.statusClock.pendingDelays()).toEqual([]);
  });
});

test.each(['pagehide', 'signout'] as const)('pending chat invalidation is cancelled on %s', async (ending) => {
  const page = await connectedPage();
  const key = ['channels-unread'] as const;
  page.queryClient.setQueryData(key, 0);
  page.gateway.channelRead(tutor.id, 'channel-a');
  await eventually(() => {
    expect(page.clock.pendingDelays()).toContain(1000);
  });

  if (ending === 'pagehide') {
    page.events.dispatchEvent(new Event('pagehide'));
  } else {
    await page.session.signOut();
  }
  page.clock.pass(1000);

  expect(page.queryClient.getQueryState(key)?.isInvalidated).toBe(false);
  expect(page.clock.pendingDelays()).not.toContain(1000);
});

test('a read reply after pagehide cannot restart refetch work and pageshow restores it', async () => {
  const page = await connectedPage();
  page.server.chat.addChannel({
    channel: {
      id: 'chat-a',
      kind: 'direct',
      label: 'A student',
      label_kind: null,
      colour: null,
      tutor: null,
      last_message: null,
      unread_count: 0,
      writable: true,
      online: null,
      unconfirmed: false,
      user_id: null,
    },
    members: [{ accountId: tutor.id }],
  });
  const paused = page.server.chat.pauseNextReadResponse();
  const client = createApiClient(origin, page.session.authorizedFetch);
  const read = markRead(client, 'chat-a', 0, new AbortController().signal).then(() => {
    scheduleChatReads(page.queryClient, page.clock);
  });
  await paused.answered;
  page.queryClient.setQueryData(unreadChannelsKey, 0);

  page.events.dispatchEvent(new Event('pagehide'));
  paused.resume();
  await read;
  page.clock.pass(1000);
  const invalidatedWhileHidden = page.queryClient.getQueryState(unreadChannelsKey)?.isInvalidated;
  const pendingWhileHidden = page.clock.pendingDelays();
  page.events.dispatchEvent(restoredFromTheBackForwardCache());
  await eventually(() => {
    expect(page.gateway.readySockets(tutor.id)).toBe(1);
  });
  scheduleChatReads(page.queryClient, page.clock);

  expect(invalidatedWhileHidden).toBe(false);
  expect(pendingWhileHidden).not.toContain(1000);
  expect(page.clock.pendingDelays()).toContain(1000);
  page.events.dispatchEvent(new Event('pagehide'));
});
