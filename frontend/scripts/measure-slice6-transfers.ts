import { execFileSync } from 'node:child_process';
import { performance } from 'node:perf_hooks';

import { seedAccount } from '../e2e/accounts.ts';
import {
  type Deployment,
  finishRun,
  prepareRun,
  readE2ESettings,
  startDeployment,
  takeRunLock,
} from '../e2e/deployment.ts';
import { channelOf } from '../e2e/messages.ts';
import { accessToken, callApi, createGroup, seedActiveStudent } from '../e2e/tutoring.ts';
import {
  measurementCount,
  measurementSmoke,
  recordMeasurement,
  requiredMeasurementString,
} from './slice6-measurement-support.ts';

interface UploadActor {
  readonly token: string;
  readonly channel: string;
}

function processes(deployment: Deployment, sample: number, phase: string, bytes: number): void {
  for (const pid of deployment.measurementProcessIds?.() ?? []) {
    const values = execFileSync('ps', ['-p', String(pid), '-o', 'pid=,pcpu=,rss='], { encoding: 'utf8' })
      .trim()
      .split(/\s+/);
    if (values.length !== 3 || values.some((value) => !Number.isFinite(Number(value)))) {
      throw new Error('measurement process counters unavailable');
    }
    recordMeasurement({
      event: 'measurement.process',
      sample,
      phase,
      bytes,
      pid,
      cpuPercent: Number(values[1]),
      rssKiB: Number(values[2]),
    });
  }
}

async function upload(deployment: Deployment, actor: UploadActor, bytes: number): Promise<string> {
  const body = new FormData();
  body.append('file', new File([new Uint8Array(bytes).fill(120)], 'measurement.txt', { type: 'text/plain' }));
  const response = await fetch(`${deployment.apiUrl}/api/v1/uploads?context=chat&scope=channel:${actor.channel}`, {
    method: 'POST',
    headers: { Origin: deployment.origin, Authorization: `Bearer ${actor.token}` },
    body,
    signal: AbortSignal.timeout(120_000),
  });
  const answer: unknown = await response.json();
  if (response.status !== 201) {
    recordMeasurement({ event: 'measurement.upload_refused', status: response.status, bytes });
    throw new Error('measurement upload failed');
  }
  return requiredMeasurementString(answer, 'id');
}

async function download(address: string, expectedBytes: number): Promise<void> {
  const response = await fetch(address, { signal: AbortSignal.timeout(120_000) });
  if (response.status !== 200 || response.body === null) {
    recordMeasurement({ event: 'measurement.download_refused', status: response.status });
    throw new Error('measurement download failed');
  }
  let bytes = 0;
  for await (const chunk of response.body) {
    bytes += chunk.byteLength;
  }
  if (bytes !== expectedBytes) {
    throw new Error('measurement download byte count differs');
  }
}

async function workload(
  deployment: Deployment,
  actors: readonly UploadActor[],
  sample: number,
  bytes: number,
): Promise<void> {
  processes(deployment, sample, 'before', bytes);
  let phase = 'uploads';
  let sampleFailure: unknown;
  const monitor = setInterval(() => {
    try {
      processes(deployment, sample, phase, bytes);
    } catch (error) {
      sampleFailure = error;
    }
  }, 100);
  const messages: { readonly actor: UploadActor; readonly id: string; readonly address: string }[] = [];
  try {
    const started = performance.now();
    const uploaded = await Promise.allSettled(
      actors.flatMap((actor) =>
        Array.from({ length: 4 }, async () => ({ actor, id: await upload(deployment, actor, bytes) })),
      ),
    );
    recordMeasurement({
      event: 'measurement.upload_batch',
      sample,
      bytes,
      concurrency: 32,
      durationMs: performance.now() - started,
      successes: uploaded.filter((result) => result.status === 'fulfilled').length,
      failures: uploaded.filter((result) => result.status === 'rejected').length,
    });
    for (const result of uploaded) {
      if (result.status !== 'fulfilled') {
        throw new Error('measurement upload batch failed; retain this sample');
      }
      const { actor, id } = result.value;
      const sent = await callApi(deployment, actor.token, 'POST', `/channels/${actor.channel}/messages`, {
        body: 'Synthetic attachment',
        attachment_ids: [id],
      });
      if (sent.status !== 201) {
        throw new Error('measurement attachment binding failed');
      }
      const message = requiredMeasurementString(sent.body, 'id');
      const addressResponse = await callApi(deployment, actor.token, 'GET', `/attachments/${id}`);
      if (addressResponse.status !== 200) {
        throw new Error('measurement download authorization failed');
      }
      const address = new URL(requiredMeasurementString(addressResponse.body, 'url'), deployment.apiUrl);
      if (address.origin !== deployment.apiUrl && address.origin !== deployment.origin) {
        throw new Error('measurement download address is not local');
      }
      address.protocol = 'http:';
      address.host = new URL(deployment.apiUrl).host;
      messages.push({ actor, id: message, address: address.toString() });
    }
    phase = 'downloads';
    const downloadStart = performance.now();
    const downloaded = await Promise.allSettled(
      messages.flatMap((message) => [download(message.address, bytes), download(message.address, bytes)]),
    );
    recordMeasurement({
      event: 'measurement.download_batch',
      sample,
      bytes,
      concurrency: 64,
      durationMs: performance.now() - downloadStart,
      successes: downloaded.filter((result) => result.status === 'fulfilled').length,
      failures: downloaded.filter((result) => result.status === 'rejected').length,
    });
    if (downloaded.some((result) => result.status === 'rejected')) {
      throw new Error('measurement download batch failed; retain this sample');
    }
    phase = 'cleanup';
    for (const message of messages) {
      const response = await callApi(deployment, message.actor.token, 'DELETE', `/messages/${message.id}`);
      if (response.status !== 204) {
        throw new Error('measurement message cleanup failed');
      }
    }
    const tick = await fetch(`${deployment.apiUrl}/__test__/tick`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
      signal: AbortSignal.timeout(120_000),
    });
    if (!tick.ok) {
      throw new Error('measurement cleanup worker failed');
    }
    await tick.arrayBuffer();
    processes(deployment, sample, 'requests_complete_and_worker_ticked', bytes);
    recordMeasurement({
      event: 'measurement.transfer_limits',
      sample,
      bytes,
      perUserConcurrency: 4,
      instanceConcurrency: 32,
      retainedObjects: 'unverified',
      incompleteMultipart: 'unverified',
      heap: 'unverified',
      descriptors: 'unverified',
      poolCheckouts: 'unverified',
      goroutines: 'unverified',
      cancellation: 'unverified',
      shutdownInFlight: 'unverified',
    });
    if (sampleFailure !== undefined) {
      throw new Error('measurement process sampling failed');
    }
  } finally {
    clearInterval(monitor);
  }
}

async function sample(index: number): Promise<void> {
  const deployment = await startDeployment(readE2ESettings(process.env), 0, `transfers-${String(index)}`, {
    synchronousCommit: 'on',
  });
  try {
    const actors: UploadActor[] = [];
    for (let member = 0; member < 8; member += 1) {
      const tutor = await seedAccount(deployment, 'Measurement tutor', 'tutor', 'en');
      const student = await seedActiveStudent(deployment, tutor, 'Measurement student', 'en');
      const group = await createGroup(deployment, tutor, 'Transfer measurements', [student.cardId]);
      const channel = await channelOf(deployment, tutor, `/groups/${group}`);
      actors.push({ token: await accessToken(deployment, student.student), channel });
    }
    let expectedDailyBytes = 0;
    for (const bytes of [
      measurementCount(1_000_000, 1024),
      measurementCount(5_000_000, 2048),
      measurementCount(25_000_000, 4096),
    ]) {
      await workload(deployment, actors, index, bytes);
      expectedDailyBytes += 32 * bytes;
      if (deployment.measurementQuery === undefined) throw new Error('daily accounting observation unavailable');
      const actual = Number(
        deployment.measurementQuery('SELECT coalesce(sum(completed_bytes), 0) FROM daily_chat_upload_bytes'),
      );
      if (actual !== expectedDailyBytes) throw new Error('transfer cleanup lost durable daily accounting');
      recordMeasurement({
        event: 'measurement.transfer_daily_accounting',
        sample: index,
        completedBytes: actual,
        expectedDailyBytes,
        pairs: 8,
        retention: 'expected_after_object_deletion',
      });
    }
  } finally {
    await deployment.stop();
  }
}

async function run(): Promise<void> {
  takeRunLock(process.env);
  try {
    prepareRun();
    recordMeasurement({
      event: 'measurement.protocol',
      mode: measurementSmoke ? 'smoke_not_performance_evidence' : 'full',
      seed: 'slice6-transfers-1',
      samples: measurementCount(3, 1),
      warmup: 'none_finite_batches',
      uploadConcurrency: 32,
      downloadConcurrency: 64,
      localReferenceCertification: false,
    });
    for (let index = 1; index <= measurementCount(3, 1); index += 1) {
      await sample(index);
    }
  } finally {
    finishRun();
  }
}

try {
  await run();
} catch {
  recordMeasurement({ event: 'measurement.failed', kind: 'transfer_run_failed' });
  process.exitCode = 1;
}
