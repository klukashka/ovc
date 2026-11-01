import { performance } from 'node:perf_hooks';

import { seedAccount, seededSession } from '../e2e/accounts.ts';
import {
  type Deployment,
  finishRun,
  prepareRun,
  readE2ESettings,
  startDeployment,
  takeRunLock,
} from '../e2e/deployment.ts';
import { accountId, presence } from '../e2e/realtime.ts';
import { authFrame, parseServerFrame, pingFrame } from '../src/lib/realtime/frames.ts';
import type { RealtimeSocket } from '../src/lib/realtime/socket.ts';
import {
  measurementCount,
  measurementScope,
  measurementSocketFactory,
  measurementSmoke,
  measurementIdentifier,
  recordMeasurement,
  requiredMeasurementString,
  untilMeasurementCondition,
} from './slice6-measurement-support.ts';

interface Socket {
  readonly connection: RealtimeSocket;
  readonly state: { ready: boolean; closed: boolean; invalid: boolean; pongs: number };
}

async function open(
  deployment: Deployment,
  token: string,
  cleanup: (release: () => Promise<void>) => void,
): Promise<Socket> {
  const state = { ready: false, closed: false, invalid: false, pongs: 0 };
  const connection = measurementSocketFactory(deployment)({
    onOpen: (): void => {
      connection.send(authFrame(token));
    },
    onText: (text): void => {
      const frame = parseServerFrame(text);
      if (frame?.type === 'ready') state.ready = true;
      if (frame?.type === 'pong') state.pongs += 1;
    },
    onBinary: (): void => {
      connection.close(1000);
      state.invalid = true;
    },
    onClose: (): void => {
      state.closed = true;
    },
  });
  cleanup(async () => {
    connection.close(1000);
    await untilMeasurementCondition(() => state.closed, 10_000);
  });
  await untilMeasurementCondition(() => state.ready || state.closed, 10_000);
  if (!state.ready) throw new Error('presence measurement connection refused');
  return { connection, state };
}

async function advance(deployment: Deployment, seconds: number): Promise<string> {
  const response = await fetch(`${deployment.apiUrl}/__test__/clock`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ advance_seconds: seconds }),
    signal: AbortSignal.timeout(10_000),
  });
  if (!response.ok) throw new Error('presence measurement clock failed');
  const body: unknown = await response.json();
  const now = requiredMeasurementString(body, 'now');
  if (!Number.isFinite(Date.parse(now))) throw new Error('presence clock returned an invalid instant');
  return new Date(now).toISOString();
}

async function tick(deployment: Deployment, deadline: number): Promise<void> {
  const response = await fetch(`${deployment.apiUrl}/__test__/tick`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
    signal: AbortSignal.timeout(Math.max(1, Math.floor(deadline - performance.now()))),
  });
  await response.arrayBuffer();
  if (!response.ok) throw new Error('presence measurement worker tick failed');
}

async function sample(index: number): Promise<void> {
  let stage = 'setup';
  await measurementScope(async (cleanup) => {
    const deployment = await startDeployment(readE2ESettings(process.env), 0, `presence-${String(index)}`, {
      synchronousCommit: 'on',
    });
    cleanup(() => deployment.stop());
    const connections: Socket[] = [];
    const account = await seedAccount(deployment, 'Measurement tutor', 'tutor', 'en');
    const user = measurementIdentifier(await accountId(deployment, account));
    const session = await seededSession(deployment, account);
    let token = session.accessToken;
    let refreshCookie = session.refreshCookie;
    const query = deployment.measurementQuery;
    if (query === undefined || deployment.startMeasurementPeer === undefined)
      throw new Error('presence measurement fixture missing');
    const primary = await open(deployment, token, cleanup);
    connections.push(primary);
    const heartbeat = setInterval(() => {
      for (const socket of connections) if (!socket.state.closed) socket.connection.send(pingFrame('measurement'));
    }, 20_000);
    cleanup(() => {
      clearInterval(heartbeat);
      return Promise.resolve();
    });
    for (let cycle = 0; cycle < measurementCount(100, 2); cycle += 1) {
      const refreshed = await fetch(`${deployment.apiUrl}/api/v1/auth/refresh`, {
        method: 'POST',
        headers: { Origin: deployment.origin, Cookie: `__Host-ovc_refresh=${refreshCookie}` },
        signal: AbortSignal.timeout(10_000),
      });
      if (!refreshed.ok) throw new Error('presence measurement session refresh failed');
      const refreshBody: unknown = await refreshed.json();
      token = requiredMeasurementString(refreshBody, 'access_token');
      const cookie = refreshed.headers
        .getSetCookie()
        .find((value) => value.startsWith('__Host-ovc_refresh='))
        ?.split(';')[0];
      if (cookie === undefined) throw new Error('presence refresh cookie missing');
      refreshCookie = cookie.slice('__Host-ovc_refresh='.length);
      const beforePong = primary.state.pongs;
      primary.connection.send(authFrame(token));
      primary.connection.send(pingFrame('reauthenticated'));
      await untilMeasurementCondition(() => primary.state.pongs > beforePong || primary.state.closed, 10_000);
      if (primary.state.closed) throw new Error('primary presence socket expired');
      const peer = await deployment.startMeasurementPeer();
      const secondary = await open({ ...deployment, apiUrl: peer.apiUrl }, token, cleanup);
      connections.push(secondary);
      if (query(`SELECT count(*) FROM user_presence_instances WHERE user_id='${user}'::uuid`) !== '2')
        throw new Error('independent processes did not own two leases');
      const before = performance.now();
      await peer.stop();
      await untilMeasurementCondition(() => secondary.state.closed, 10_000);
      const beforeExpiry = Number(query(`SELECT count(*) FROM user_presence_instances WHERE user_id='${user}'::uuid`));
      let now = await advance(deployment, 91);
      query(
        `INSERT INTO user_presence_instances (user_id, instance_id, online_until, last_seen_at) VALUES ('${user}'::uuid, md5('measurement-expired-${String(cycle)}')::uuid, '${now}'::timestamptz, '${now}'::timestamptz)`,
      );
      now = await advance(deployment, 61);
      recordMeasurement({
        event: 'measurement.presence_expiry_seed',
        sample: index,
        cycle,
        source: 'synthetic_expired_lease_after_clean_peer_shutdown',
      });
      stage = 'expire_worker_tick';
      const expiryDeadline = performance.now() + 30_000;
      await tick(deployment, expiryDeadline);
      if (performance.now() >= expiryDeadline) throw new Error('presence expiry deadline exceeded');
      stage = 'expiry_quiescence';
      await untilMeasurementCondition(
        () =>
          query(
            `SELECT count(*) FROM user_presence_instances WHERE user_id='${user}'::uuid AND online_until <= '${now}'::timestamptz`,
          ) === '0',
        Math.max(1, expiryDeadline - performance.now()),
      );
      const leases = Number(query(`SELECT count(*) FROM user_presence_instances WHERE user_id='${user}'::uuid`));
      const online =
        query(`SELECT online_until > '${now}'::timestamptz FROM user_presence WHERE user_id='${user}'::uuid`) === 't';
      const local = await presence(deployment, user);
      recordMeasurement({
        event: 'measurement.presence_process',
        sample: index,
        cycle,
        mode: 'graceful_shutdown',
        durationMs: performance.now() - before,
        beforeExpiry,
        leases,
        online,
        primarySockets: local.sockets,
      });
      if (leases !== 1 || !online || local.sockets !== 1)
        throw new Error('peer loss removed surviving account presence');
      connections.splice(connections.indexOf(secondary), 1);
    }
    if (deployment.holdMeasurementPresence === undefined) throw new Error('presence row lock fixture missing');
    stage = 'release_backlog_lock';
    const release = await deployment.holdMeasurementPresence(user);
    const releaseStarted = performance.now();
    try {
      stage = 'release_socket_close';
      primary.connection.close(1000);
      await untilMeasurementCondition(() => primary.state.closed, 10_000);
      stage = 'release_backlog_observation';
      await untilMeasurementCondition(async () => {
        const response = await fetch(`${deployment.apiUrl}/__test__/presence`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ user }),
          signal: AbortSignal.timeout(2000),
        });
        await response.arrayBuffer();
        if (response.status !== 200 && response.status !== 503) throw new Error('release backlog observation failed');
        return response.status === 503;
      }, 10_000);
      recordMeasurement({
        event: 'measurement.presence_release_backlog',
        sample: index,
        pending: true,
        durationMs: performance.now() - releaseStarted,
      });
    } finally {
      await release();
    }
    stage = 'release_retry_clock_flush';
    await advance(deployment, 1);
    await untilMeasurementCondition(
      () => query(`SELECT count(*) FROM user_presence_instances WHERE user_id='${user}'::uuid`) === '0',
      10_000,
    );
    if (connections.some((socket) => socket.state.invalid))
      throw new Error('presence measurement received binary frame');
    recordMeasurement({
      event: 'measurement.presence_quiescent',
      sample: index,
      leases: 0,
      releaseBacklog: 'drained',
      durationMs: performance.now() - releaseStarted,
    });
  }).catch(() => {
    recordMeasurement({
      event: 'measurement.presence_stage_failed',
      sample: index,
      stage,
      kind: 'condition_or_fixture_failed',
    });
    throw new Error('presence measurement sample failed');
  });
}

async function run(): Promise<void> {
  takeRunLock(process.env);
  try {
    prepareRun();
    recordMeasurement({
      event: 'measurement.protocol',
      mode: measurementSmoke ? 'smoke_not_performance_evidence' : 'full',
      seed: 'slice6-presence-1',
      samples: measurementCount(3, 1),
      cycles: measurementCount(100, 2),
      apiProcesses: 2,
      clockAdvanceSecondsPerCycle: 152,
      warmup: 'first_connection',
      referenceCertification: false,
    });
    for (let index = 1; index <= measurementCount(3, 1); index += 1) await sample(index);
  } finally {
    finishRun();
  }
}

try {
  await run();
} catch {
  recordMeasurement({ event: 'measurement.failed', kind: 'presence_run_failed' });
  process.exitCode = 1;
}
