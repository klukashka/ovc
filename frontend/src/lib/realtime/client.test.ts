import { QueryClient, QueryObserver } from '@tanstack/react-query';
import { describe, expect, test } from 'vitest';

import { liveRefetch } from '../../app/liveRefetch.ts';
import { NotificationArrivals } from '../../app/notificationArrivals.ts';
import { studentsKey } from '../../features/students/api.ts';
import { FakeAuthServer, type ManualScheduler, manualScheduler, memoryLocks } from '../api/fakeTransport.ts';
import { Session } from '../api/session.ts';
import { eventually } from '../eventually.ts';
import { type RealtimeEvent, RealtimeClient, connectWhileSignedIn } from './client.ts';
import { closeCodes } from './frames.ts';
import { FakeGateway } from './fakeGateway.ts';

const origin = 'https://ovc.example';
const tutor = { id: 'tutor-a', name: 'A tutor', email: 'a@example.org', password: 'a long enough password' };
const other = { id: 'tutor-b', name: 'B tutor', email: 'b@example.org', password: 'another long password' };

interface Rig {
  readonly server: FakeAuthServer;
  readonly gateway: FakeGateway;
  readonly session: Session;
  readonly sessionScheduler: ManualScheduler;
  readonly clock: ManualScheduler;
  readonly failures: Error[];
  readonly disconnect: () => void;
}

async function connectedRig(onEvent: (event: RealtimeEvent) => void = () => undefined): Promise<Rig> {
  const server = new FakeAuthServer();
  server.addAccount(tutor);
  server.addAccount(other);
  const gateway = new FakeGateway(server);
  const sessionScheduler = manualScheduler();
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: sessionScheduler,
  });
  await session.signIn(tutor.email, tutor.password);
  const clock = manualScheduler();
  const failures: Error[] = [];
  const disconnect = connectWhileSignedIn(
    session,
    (epoch): RealtimeClient =>
      new RealtimeClient({
        sockets: gateway.sockets,
        tokens: session,
        epoch,
        scheduler: clock,
        random: (): number => 0.5,
        onEvent,
        onFailure: (error): void => {
          failures.push(error);
        },
        connectedBefore: false,
      }),
  );
  await eventually(() => {
    expect(gateway.readySockets(tutor.id)).toBe(1);
  });
  return { server, gateway, session, sessionScheduler, clock, failures, disconnect };
}

async function passEveryReconnectDelay(clock: ManualScheduler, attempts: number): Promise<readonly number[]> {
  const delays: number[] = [];
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    await eventually(() => {
      expect(clock.pendingDelays()).toHaveLength(1);
    });
    const [delay = 0] = clock.pendingDelays();
    delays.push(delay);
    clock.pass(delay);
  }
  return delays;
}

async function refreshAndReconnect(rig: Rig): Promise<void> {
  const refreshesBefore = rig.server.refreshes;
  rig.gateway.closeAccount(tutor.id, closeCodes.unauthenticated);
  await eventually(() => {
    expect(rig.server.refreshes).toBe(refreshesBefore + 1);
  });
  await passEveryReconnectDelay(rig.clock, 1);
}

async function closeAfterEveryReady(rig: Rig, closes: number): Promise<readonly number[]> {
  const delays: number[] = [];
  for (let close = 0; close < closes; close += 1) {
    const opened = rig.gateway.socketsOpened;
    rig.gateway.closeAccount(tutor.id, closeCodes.reconnect);
    delays.push(...(await passEveryReconnectDelay(rig.clock, 1)));
    await eventually(() => {
      expect(rig.gateway.socketsOpened).toBe(opened + 1);
      expect(rig.gateway.readySockets(tutor.id)).toBe(1);
    });
  }
  return delays;
}

async function afterNextPong(rig: Rig): Promise<void> {
  await eventually(() => {
    expect(rig.clock.pendingDelays()).toEqual([30_000]);
  });
}

async function tokenOf(server: FakeAuthServer, account: typeof other): Promise<unknown> {
  const response = await server.transport().login(account.email, account.password);
  return response.json();
}

describe('realtime client', () => {
  test('accepts a lesson entity frame without dropping the socket', async () => {
    const events: RealtimeEvent[] = [];
    const rig = await connectedRig((event) => {
      events.push(event);
    });
    const opened = rig.gateway.socketsOpened;

    rig.gateway.changed(tutor.id, 'lesson', 'a-lesson');

    await eventually(() => {
      expect(events).toContainEqual({ kind: 'entity', entity: 'lesson', id: 'a-lesson' });
    });
    expect(rig.failures).toEqual([]);
    expect(rig.gateway.socketsOpened).toBe(opened);
    expect(rig.gateway.readySockets(tutor.id)).toBe(1);
    rig.disconnect();
  });

  test("refreshes once on 4401 and reconnects, or signs out with the refresh's reason", async () => {
    const renewable = await connectedRig();
    const revoked = await connectedRig();
    revoked.server.revokeSessionsOf(tutor.email);

    revoked.gateway.recheck();
    await refreshAndReconnect(renewable);

    await eventually(() => {
      expect(renewable.gateway.readySockets(tutor.id)).toBe(1);
      expect(revoked.session.snapshot().kind).toBe('signedOut');
    });
    expect(renewable.gateway.socketsOpened).toBe(2);
    expect(revoked.session.snapshot()).toEqual({ kind: 'signedOut', reason: 'sessionEnded' });
    expect(revoked.clock.pendingDelays()).toEqual([]);
    expect(revoked.gateway.socketsOpened).toBe(1);
    expect([...renewable.failures, ...revoked.failures]).toEqual([]);
  });

  test('sends each new access token in-band', async () => {
    const rig = await connectedRig();

    rig.sessionScheduler.runAll();

    await eventually(() => {
      expect(rig.gateway.tokensFrom(tutor.id)).toHaveLength(2);
    });
    const [first, renewed] = rig.gateway.tokensFrom(tutor.id);
    expect(renewed).not.toBe(first);
    expect(rig.server.liveSessionOf(renewed ?? '')).not.toBeNull();
    expect(rig.gateway.socketsOpened).toBe(1);
    expect(rig.failures).toEqual([]);
  });

  test('waits at least the listener reconnect delay after 1012, backs off with jitter and refetches after ready', async () => {
    const queryClient = new QueryClient();
    let fetches = 0;
    const observer = new QueryObserver(queryClient, {
      queryKey: studentsKey,
      queryFn: (): Promise<number> => {
        fetches += 1;
        return Promise.resolve(fetches);
      },
    });
    const unsubscribe = observer.subscribe(() => undefined);
    const rig = await connectedRig(liveRefetch(queryClient, new NotificationArrivals()));
    await eventually(() => {
      expect(fetches).toBe(2);
    });
    rig.gateway.refuseConnections(6);

    rig.gateway.closeAccount(tutor.id, closeCodes.reconnect);
    const delays = await passEveryReconnectDelay(rig.clock, 7);

    await eventually(() => {
      expect(fetches).toBe(3);
    });
    expect(delays).toEqual([1_500, 1_000, 2_000, 4_000, 8_000, 15_000, 15_000]);
    expect(rig.gateway.readySockets(tutor.id)).toBe(1);
    expect(rig.failures).toEqual([]);
    unsubscribe();
  });

  test('marks only the readiness after the first connection as a reconnect', async () => {
    const events: RealtimeEvent[] = [];
    const rig = await connectedRig((event) => {
      events.push(event);
    });

    await closeAfterEveryReady(rig, 1);

    expect(events).toEqual([
      { kind: 'connected', reconnect: false },
      { kind: 'connected', reconnect: true },
    ]);
  });

  test('marks a first readiness after refused attempts as a reconnect', async () => {
    const events: RealtimeEvent[] = [];
    const server = new FakeAuthServer();
    server.addAccount(tutor);
    const gateway = new FakeGateway(server);
    const session = new Session({
      origin,
      transport: server.transport(),
      locks: memoryLocks(),
      scheduler: manualScheduler(),
    });
    await session.signIn(tutor.email, tutor.password);
    gateway.refuseConnections(1);
    const clock = manualScheduler();

    const disconnect = connectWhileSignedIn(
      session,
      (epoch): RealtimeClient =>
        new RealtimeClient({
          sockets: gateway.sockets,
          tokens: session,
          epoch,
          scheduler: clock,
          random: (): number => 0.5,
          onEvent: (event): void => {
            events.push(event);
          },
          onFailure: (): void => undefined,
          connectedBefore: false,
        }),
    );
    await passEveryReconnectDelay(clock, 1);
    await eventually(() => {
      expect(gateway.readySockets(tutor.id)).toBe(1);
    });

    expect(events).toEqual([{ kind: 'connected', reconnect: true }]);
    disconnect();
  });

  test('the backoff starts over only after a pong', async () => {
    const rig = await connectedRig();

    const beforePong = await closeAfterEveryReady(rig, 2);
    rig.clock.pass(30_000);
    await eventually(() => {
      expect(rig.clock.pendingDelays()).toEqual([10_000, 30_000]);
    });
    await afterNextPong(rig);
    const afterPong = await closeAfterEveryReady(rig, 1);

    expect(beforePong).toEqual([1_500, 2_000]);
    expect(afterPong).toEqual([1_500]);
    expect(rig.failures).toEqual([]);
  });

  test('waits up to 60 seconds after 4429', async () => {
    const rig = await connectedRig();
    rig.gateway.refuseConnections(6, closeCodes.tooMany);

    rig.gateway.closeAccount(tutor.id, closeCodes.tooMany);
    const delays = await passEveryReconnectDelay(rig.clock, 7);

    await eventually(() => {
      expect(rig.gateway.readySockets(tutor.id)).toBe(1);
    });
    expect(delays).toEqual([500, 1_000, 2_000, 4_000, 8_000, 16_000, 30_000]);
    expect(rig.failures).toEqual([]);
  });

  test('reconnects when no pong arrives', async () => {
    const rig = await connectedRig();
    rig.gateway.stopAnsweringPings(tutor.id);

    rig.clock.pass(30_000);
    rig.clock.pass(10_000);
    rig.clock.pass(500);

    await eventually(() => {
      expect(rig.gateway.readySockets(tutor.id)).toBe(1);
      expect(rig.gateway.socketsOpened).toBe(2);
    });
    expect(rig.failures).toEqual([]);
  });

  test('closes when the session epoch changes', async () => {
    const rig = await connectedRig();

    await rig.session.adopt(await tokenOf(rig.server, other));

    await eventually(() => {
      expect(rig.gateway.readySockets(other.id)).toBe(1);
    });
    expect(rig.gateway.readySockets(tutor.id)).toBe(0);
    expect(rig.gateway.openSockets).toBe(1);
    expect(rig.gateway.tokensOfferedOnSocketsOf(tutor.id).map((token) => rig.server.issuedBy(token))).toEqual([
      tutor.id,
    ]);
    expect(rig.failures).toEqual([]);
  });
});

test('chat frames emit refetch hints without making their payload authoritative', async () => {
  const events: RealtimeEvent[] = [];
  const rig = await connectedRig((event) => {
    events.push(event);
  });

  rig.gateway.messageExchanged(tutor.id, 'channel-a', 'message-a', 5);
  rig.gateway.channelRead(tutor.id, 'channel-a');

  await eventually(() => {
    expect(events.filter((event) => event.kind !== 'connected')).toEqual([
      { kind: 'message', change: 'new', channelId: 'channel-a', messageId: 'message-a' },
      { kind: 'message', change: 'updated', channelId: 'channel-a', messageId: 'message-a' },
      { kind: 'message', change: 'deleted', channelId: 'channel-a', messageId: 'message-a' },
      { kind: 'read', channelId: 'channel-a' },
    ]);
  });
  rig.disconnect();
});
