import { QueryClient, QueryObserver } from '@tanstack/react-query';
import { describe, expect, test } from 'vitest';

import { latestNotificationsKey } from '../features/notifications/api.ts';
import { FakeAuthServer, type ManualScheduler, manualScheduler, memoryLocks } from '../lib/api/fakeTransport.ts';
import { Session } from '../lib/api/session.ts';
import { eventually } from '../lib/eventually.ts';
import { FakeGateway } from '../lib/realtime/fakeGateway.ts';
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
  wireRealtime({
    session,
    queryClient,
    sockets: gateway.sockets,
    scheduler: clock,
    page: { events, loaded: () => loaded },
    onFailure: (error) => {
      failures.push(error);
    },
  });
  return { server, gateway, session, queryClient, events, clock, failures };
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
