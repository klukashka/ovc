import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { performance } from 'node:perf_hooks';

import { chromium, expect, type Page, type Request } from '@playwright/test';

import { seedAccount, signInThroughTheApi } from '../e2e/accounts.ts';
import {
  type Deployment,
  finishRun,
  prepareRun,
  readE2ESettings,
  startDeployment,
  takeRunLock,
} from '../e2e/deployment.ts';
import { channelOf } from '../e2e/messages.ts';
import { accountId, closeSockets } from '../e2e/realtime.ts';
import { accessToken, callApi, createGroup, seedActiveStudent } from '../e2e/tutoring.ts';
import { parseServerFrame } from '../src/lib/realtime/frames.ts';
import {
  measurementIdentifier,
  measurementScope,
  recordMeasurement,
  requiredMeasurementString,
  untilMeasurementCondition,
} from './slice6-measurement-support.ts';

type RequestClass =
  'history_full' | 'history_catchup' | 'history_identified' | 'channel_detail' | 'channel_list' | 'unread' | 'other';
interface Observation {
  readonly request: Request;
  readonly kind: RequestClass;
  readonly method: string;
  completed: boolean;
  succeeded: boolean;
}

interface HistoryObservation {
  readonly state: {
    ready: number;
    closed: number;
    newFrames: number;
    updateFrames: number;
    deleteFrames: number;
    diagnostics: number;
    pending: number;
    lastActivity: number;
  };
  readonly observations: Observation[];
  readonly settle: () => Promise<void>;
}

function observe(page: Page, channel: string): HistoryObservation {
  const observations: Observation[] = [];
  const state = {
    ready: 0,
    closed: 0,
    newFrames: 0,
    updateFrames: 0,
    deleteFrames: 0,
    diagnostics: 0,
    pending: 0,
    lastActivity: performance.now(),
  };
  const classify = (request: Request): RequestClass => {
    const url = new URL(request.url());
    if (url.pathname === `/api/v1/channels/${channel}/messages`) {
      if (!url.searchParams.has('after_seq')) return 'history_full';
      return url.searchParams.get('limit') === '1' ? 'history_identified' : 'history_catchup';
    }
    if (url.pathname === `/api/v1/channels/${channel}`) return 'channel_detail';
    if (url.pathname === '/api/v1/channels') return 'channel_list';
    if (url.pathname.includes('unread')) return 'unread';
    return 'other';
  };
  page.on('request', (request) => {
    state.pending += 1;
    state.lastActivity = performance.now();
    observations.push({
      request,
      kind: classify(request),
      method: request.method(),
      completed: false,
      succeeded: false,
    });
  });
  const finish = (request: Request, succeeded: boolean): void => {
    const found = observations.find((entry) => entry.request === request);
    if (found === undefined) {
      state.diagnostics += 1;
      return;
    }
    found.completed = true;
    found.succeeded = succeeded;
    state.pending -= 1;
    state.lastActivity = performance.now();
  };
  page.on('requestfinished', (request) => {
    finish(request, true);
  });
  page.on('requestfailed', (request) => {
    finish(request, false);
  });
  page.on('response', (response) => {
    if (response.status() >= 400) state.diagnostics += 1;
  });
  page.on('pageerror', () => {
    state.diagnostics += 1;
  });
  page.on('console', (message) => {
    if (message.type() === 'error') state.diagnostics += 1;
  });
  page.on('websocket', (socket) => {
    socket.on('close', () => {
      state.closed += 1;
    });
    socket.on('framereceived', ({ payload }) => {
      if (typeof payload !== 'string') return;
      const frame = parseServerFrame(payload);
      if (frame?.type === 'ready') state.ready += 1;
      if (frame !== null && 'channelId' in frame && frame.channelId === channel) {
        if (frame.type === 'message.new') state.newFrames += 1;
        if (frame.type === 'message.updated') state.updateFrames += 1;
        if (frame.type === 'message.deleted') state.deleteFrames += 1;
      }
    });
  });
  return {
    state,
    observations,
    settle: async (): Promise<void> => {
      await untilMeasurementCondition(
        () => state.ready > 0 && state.pending === 0 && performance.now() - state.lastActivity >= 1600,
        20_000,
      );
    },
  };
}

async function mutation(
  deployment: Deployment,
  token: string,
  method: 'POST' | 'PATCH' | 'DELETE',
  path: string,
  body?: unknown,
): Promise<unknown> {
  const response = await callApi(deployment, token, method, path, body);
  if (response.status !== (method === 'POST' ? 201 : method === 'DELETE' ? 204 : 200))
    throw new Error('history probe mutation failed');
  return response.body;
}

async function sample(rows: number): Promise<void> {
  let stage = 'deployment_start';
  const observation: { current: HistoryObservation | null } = { current: null };
  await measurementScope(async (cleanup) => {
    const deployment = await startDeployment(readE2ESettings(process.env), 0, `history-refresh-${String(rows)}`, {
      synchronousCommit: 'on',
    });
    cleanup(() => deployment.stop());
    stage = 'fixture_seed';
    const tutor = await seedAccount(deployment, 'Measurement tutor', 'tutor', 'en');
    const member = await seedActiveStudent(deployment, tutor, 'Measurement student', 'en');
    const group = await createGroup(deployment, tutor, 'History refresh measurement', [member.cardId]);
    const channel = measurementIdentifier(await channelOf(deployment, tutor, `/groups/${group}`));
    const tutorUser = measurementIdentifier(await accountId(deployment, tutor));
    const sender = measurementIdentifier(await accountId(deployment, member.student));
    const token = await accessToken(deployment, member.student);
    const query = deployment.measurementQuery;
    if (query === undefined) throw new Error('history probe seed fixture unavailable');
    query(`INSERT INTO messages (id, channel_id, seq, sender_id, body, version, created_at, updated_at)
SELECT md5('history-refresh-' || seq)::uuid, '${channel}'::uuid, seq, '${sender}'::uuid, 'Synthetic history ' || seq, 1, (SELECT instant FROM test_clock WHERE id=1), (SELECT instant FROM test_clock WHERE id=1)
FROM generate_series(1,${String(rows)}) AS series(seq);
UPDATE channels SET last_seq=${String(rows)}, last_activity_at=(SELECT instant FROM test_clock WHERE id=1) WHERE id='${channel}'::uuid;`);
    stage = 'browser_start';
    const browser = await chromium.launch();
    cleanup(() => browser.close());
    const context = await browser.newContext({ ignoreHTTPSErrors: true, baseURL: deployment.origin });
    cleanup(() => context.close());
    const page = await context.newPage();
    const traffic = observe(page, channel);
    observation.current = traffic;
    stage = 'initial_history_ready';
    await signInThroughTheApi(page, tutor);
    await page.goto(`/messages/${channel}`);
    await page.getByRole('textbox', { name: 'Message', exact: true }).waitFor();
    const list = page.getByRole('list', { name: 'Messages', exact: true });
    for (let loaded = 50; loaded < rows; loaded += 50) {
      await page.getByRole('button', { name: 'Show older', exact: true }).click();
      await expect(list.getByRole('listitem')).toHaveCount(Math.min(rows, loaded + 50));
    }
    await traffic.settle();
    await expect(list.getByRole('listitem')).toHaveCount(rows);
    const idAt = async (seq: number): Promise<string> => {
      const id = await list.locator(`[data-seq="${String(seq)}"]`).getAttribute('data-row');
      if (id === null) throw new Error('history probe loaded row missing');
      return measurementIdentifier(id);
    };
    const oldEdit = await idAt(1);
    const oldDelete = await idAt(2);
    const reconnectEdit = await idAt(3);
    const reconnectDelete = await idAt(4);
    const runCase = async (
      name: string,
      action: () => Promise<void>,
      rendered: () => Promise<void>,
      expected: RequestClass | null,
      expectedFrames: number,
      expectedAfter?: number,
    ): Promise<void> => {
      stage = name;
      await traffic.settle();
      const start = traffic.observations.length;
      const frameStart = traffic.state.newFrames + traffic.state.updateFrames + traffic.state.deleteFrames;
      const began = performance.now();
      try {
        await action();
        await rendered();
      } catch {
        const partial = traffic.observations
          .slice(start)
          .filter((entry) => entry.method === 'GET' && entry.kind.startsWith('history_'));
        recordMeasurement({
          event: 'measurement.history_case_failed',
          rows,
          case: name,
          historyGets: partial.length,
          completed: partial.filter((entry) => entry.completed && entry.succeeded).length,
          kind: 'action_or_render_assertion_failed',
        });
        throw new Error('history probe action or render assertion failed');
      }
      const renderedMs = performance.now() - began;
      await traffic.settle();
      const requests = traffic.observations.slice(start);
      const history = requests.filter((entry) => entry.method === 'GET' && entry.kind.startsWith('history_'));
      const frames = traffic.state.newFrames + traffic.state.updateFrames + traffic.state.deleteFrames - frameStart;
      for (const kind of [
        'history_full',
        'history_catchup',
        'history_identified',
        'channel_detail',
        'channel_list',
        'unread',
        'other',
      ] as const) {
        for (const method of ['GET', 'POST', 'PATCH', 'DELETE']) {
          const classified = requests.filter((entry) => entry.kind === kind && entry.method === method);
          if (classified.length > 0)
            recordMeasurement({
              event: 'measurement.history_request_class',
              rows,
              case: name,
              method,
              queryKeyClass: kind,
              requests: classified.length,
              completed: classified.filter((entry) => entry.completed && entry.succeeded).length,
            });
        }
      }
      recordMeasurement({
        event: 'measurement.history_refresh',
        rows,
        case: name,
        historyGets: history.length,
        authoritativeCompleted: history.filter((entry) => entry.completed && entry.succeeded).length,
        messageFrames: frames,
        renderedMs,
        waveObservation: 'post_frame_production_request_batch',
        internalWaveCounter: 'unverified',
        warnings: 'pending_change_3',
        nfrLatencyPass: false,
      });
      if (expected !== null && (history.length !== 1 || history[0]?.kind !== expected))
        throw new Error('history probe bounded request assertion failed');
      if (
        expectedAfter !== undefined &&
        new URL(history[0]?.request.url() ?? 'https://invalid.local').searchParams.get('after_seq') !==
          String(expectedAfter)
      )
        throw new Error('history probe sequence assertion failed');
      if (name === 'inflight_full_read_new_followup' && history.length <= Math.ceil((rows + 2) / 50))
        throw new Error('history probe mandatory fresh followup missing');
      if (name === 'reconnect_old_changes' && history.length < Math.ceil((rows + 1) / 50))
        throw new Error('history probe full reconciliation missing');
      if (frames < expectedFrames || history.some((entry) => !entry.completed || !entry.succeeded))
        throw new Error('history probe completion assertion failed');
    };
    await runCase(
      'live_new',
      async () => {
        await mutation(deployment, token, 'POST', `/channels/${channel}/messages`, {
          body: 'Synthetic remote new',
          attachment_ids: [],
        });
      },
      async () => {
        await expect(list.getByRole('listitem')).toHaveCount(rows + 1);
        await expect(list).toContainText('Synthetic remote new');
      },
      'history_catchup',
      1,
      rows,
    );
    await runCase(
      'live_old_edit',
      async () => {
        await mutation(deployment, token, 'PATCH', `/messages/${oldEdit}`, {
          body: 'Synthetic old edited',
          version: 1,
        });
      },
      async () => {
        await expect(list.locator(`[data-row="${oldEdit}"]`)).toContainText('Synthetic old edited');
      },
      'history_identified',
      1,
      0,
    );
    await runCase(
      'live_old_delete',
      async () => {
        await mutation(deployment, token, 'DELETE', `/messages/${oldDelete}`);
      },
      async () => {
        await expect(list.locator(`[data-row="${oldDelete}"]`)).toContainText('Message deleted');
      },
      'history_identified',
      1,
      1,
    );
    await traffic.settle();
    stage = 'reconnect_disconnect';
    const closed = traffic.state.closed;
    await context.setOffline(true);
    if ((await closeSockets(deployment, tutorUser, 1012)) !== 1)
      throw new Error('history probe expected one server socket to close');
    await untilMeasurementCondition(() => traffic.state.closed > closed, 10_000);
    stage = 'reconnect_missed_changes';
    await mutation(deployment, token, 'PATCH', `/messages/${reconnectEdit}`, {
      body: 'Synthetic reconnect edited',
      version: 1,
    });
    await mutation(deployment, token, 'DELETE', `/messages/${reconnectDelete}`);
    const ready = traffic.state.ready;
    await runCase(
      'reconnect_old_changes',
      async () => {
        await context.setOffline(false);
        await untilMeasurementCondition(() => traffic.state.ready > ready, 20_000);
      },
      async () => {
        await expect(list.locator(`[data-row="${reconnectEdit}"]`)).toContainText('Synthetic reconnect edited');
        await expect(list.locator(`[data-row="${reconnectDelete}"]`)).toContainText('Message deleted');
        await expect(list.getByRole('listitem')).toHaveCount(rows + 1);
      },
      null,
      0,
    );
    let release: (() => void) | undefined;
    let held = false;
    const barrier = new Promise<void>((resolve) => {
      release = resolve;
    });
    const historyPattern = `**/api/v1/channels/${channel}/messages*`;
    await page.route(historyPattern, async (route) => {
      if (!held && !new URL(route.request().url()).searchParams.has('after_seq')) {
        held = true;
        await barrier;
      }
      await route.continue();
    });
    cleanup(async () => {
      release?.();
      await page.unroute(historyPattern);
    });
    await runCase(
      'inflight_full_read_new_followup',
      async () => {
        stage = 'inflight_disconnect';
        const closes = traffic.state.closed;
        await context.setOffline(true);
        if ((await closeSockets(deployment, tutorUser, 1012)) !== 1)
          throw new Error('history probe expected one server socket to close');
        await untilMeasurementCondition(() => traffic.state.closed > closes, 10_000);
        stage = 'inflight_full_read_ready';
        const readies = traffic.state.ready;
        await context.setOffline(false);
        await untilMeasurementCondition(() => held && traffic.state.ready > readies, 20_000);
        stage = 'inflight_new_message';
        const frames = traffic.state.newFrames;
        const detailBefore = traffic.observations.filter((entry) => entry.kind === 'channel_detail').length;
        try {
          const answer = await mutation(deployment, token, 'POST', `/channels/${channel}/messages`, {
            body: 'Synthetic overlapping remote',
            attachment_ids: [],
          });
          requiredMeasurementString(answer, 'id');
          await untilMeasurementCondition(
            () =>
              traffic.state.newFrames > frames &&
              traffic.observations.filter((entry) => entry.kind === 'channel_detail').length > detailBefore,
            10_000,
          );
        } finally {
          release?.();
        }
      },
      async () => {
        await expect(list).toContainText('Synthetic overlapping remote');
        await expect(list.getByRole('listitem')).toHaveCount(rows + 2);
      },
      null,
      1,
    );
    await page.unroute(historyPattern);
    const lastFull = traffic.observations.filter((entry) => entry.kind === 'history_full');
    recordMeasurement({
      event: 'measurement.history_followup_coverage',
      rows,
      fullReadsTotal: lastFull.length,
      mandatoryFollowup: 'reported_in_case_request_classes',
    });
    if (traffic.state.diagnostics !== 0) throw new Error('history probe unexpected browser failure');
    stage = 'owned_cleanup';
  }).catch(() => {
    recordMeasurement({
      event: 'measurement.history_sample_failed',
      rows,
      stage,
      readyFrames: observation.current?.state.ready ?? 0,
      closedSockets: observation.current?.state.closed ?? 0,
      diagnostics: observation.current?.state.diagnostics ?? 0,
      kind: 'history_sample_failed',
    });
    throw new Error('history probe sample failed');
  });
}

async function run(): Promise<void> {
  if (!process.argv.includes('--acceptance'))
    throw new Error('history probe requires explicit bounded acceptance opt-in');
  takeRunLock(process.env);
  try {
    prepareRun();
    recordMeasurement({
      event: 'measurement.protocol',
      mode: 'bounded_history_acceptance',
      samples: 1,
      engine: 'chromium',
      datasets: '50,1000',
      sourceSha256: createHash('sha256')
        .update(readFileSync(new URL(import.meta.url)))
        .digest('hex'),
      fullPerformanceMatrix: false,
      referenceCertification: false,
      warnings: 'pending_change_3',
    });
    const selected = process.argv.find((argument) => argument.startsWith('--dataset='));
    if (selected !== undefined && selected !== '--dataset=50' && selected !== '--dataset=1000')
      throw new Error('history probe dataset invalid');
    for (const rows of selected === undefined ? [50, 1000] : [Number(selected.slice('--dataset='.length))])
      await sample(rows);
  } finally {
    finishRun();
  }
}

try {
  await run();
} catch {
  recordMeasurement({ event: 'measurement.failed', kind: 'history_refresh_probe_failed' });
  process.exitCode = 1;
}
