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
import { accessToken, createGroup } from '../e2e/tutoring.ts';
import {
  measurementCount,
  measurementScope,
  measurementIdentifier,
  measurementSmoke,
  recordMeasurement,
  untilMeasurementCondition,
} from './slice6-measurement-support.ts';

const shutdownWindowControl = process.argv.includes('--shutdown-window-control');

type UploadOutcome = { readonly status: number } | { readonly failed: true };

interface ActiveUpload {
  readonly cancel: () => void;
  readonly outcome: Promise<UploadOutcome>;
}

async function begin(
  deployment: Deployment,
  apiUrl: string,
  token: string,
  channel: string,
  cleanup: (release: () => Promise<void>) => void,
): Promise<ActiveUpload> {
  const query = deployment.measurementQuery;
  if (query === undefined) throw new Error('upload process observation unavailable');
  const abort = new AbortController();
  const stream = new ReadableStream<Uint8Array>({
    start(controller): void {
      controller.enqueue(
        new TextEncoder().encode(
          `--measurement-boundary\r\nContent-Disposition: form-data; name="file"; filename="measurement.txt"\r\nContent-Type: text/plain\r\n\r\n${'x'.repeat(1024)}`,
        ),
      );
    },
  });
  const options: RequestInit & { readonly duplex: 'half' } = {
    method: 'POST',
    headers: {
      Origin: deployment.origin,
      Authorization: `Bearer ${token}`,
      'Content-Type': 'multipart/form-data; boundary=measurement-boundary',
    },
    body: stream,
    signal: abort.signal,
    duplex: 'half',
  };
  const outcome: Promise<UploadOutcome> = fetch(
    `${apiUrl}/api/v1/uploads?context=chat&scope=channel:${channel}`,
    options,
  )
    .then(
      async (response): Promise<UploadOutcome> => {
        await response.arrayBuffer();
        return { status: response.status };
      },
      (): UploadOutcome => ({ status: 0 }),
    )
    .catch((): UploadOutcome => ({ failed: true }));
  cleanup(async () => {
    abort.abort();
    await uploadOutcome(outcome);
  });
  await untilMeasurementCondition(
    () => query("SELECT count(*) FROM upload_intents WHERE phase='streaming'") === '1',
    10_000,
  );
  return {
    cancel: (): void => {
      abort.abort();
    },
    outcome,
  };
}

async function uploadOutcome(outcome: Promise<UploadOutcome>): Promise<number> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    const result = await Promise.race([
      outcome,
      new Promise<never>((_, reject) => {
        timer = setTimeout(() => {
          reject(new Error('upload outcome cleanup deadline exceeded'));
        }, 10_000);
      }),
    ]);
    if ('failed' in result) throw new Error('upload response observation failed');
    return result.status;
  } finally {
    clearTimeout(timer);
  }
}

async function advanceAndDrain(deployment: Deployment): Promise<void> {
  const advance = await fetch(`${deployment.apiUrl}/__test__/clock`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{"advance_seconds":1300}',
    signal: AbortSignal.timeout(10_000),
  });
  if (!advance.ok) throw new Error('upload process recovery clock failed');
  await advance.arrayBuffer();
  const tick = await fetch(`${deployment.apiUrl}/__test__/tick`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
    signal: AbortSignal.timeout(120_000),
  });
  if (!tick.ok) throw new Error('upload process recovery worker failed');
  await tick.arrayBuffer();
}

async function sample(index: number): Promise<void> {
  let stage = 'setup';
  await measurementScope(async (cleanup) => {
    const deployment = await startDeployment(
      readE2ESettings(process.env),
      0,
      `${shutdownWindowControl ? 'upload-process-control' : 'upload-process'}-${String(index)}`,
      {
        synchronousCommit: 'on',
      },
    );
    cleanup(() => deployment.stop());
    const account = await seedAccount(deployment, 'Measurement tutor', 'tutor', 'en');
    const group = await createGroup(deployment, account, 'Upload process measurement', []);
    const channel = measurementIdentifier(await channelOf(deployment, account, `/groups/${group}`));
    const query = deployment.measurementQuery;
    const startPeer = deployment.startMeasurementPeer;
    if (query === undefined || startPeer === undefined) throw new Error('upload process fixture unavailable');
    for (let cycle = 0; cycle < measurementCount(100, 1); cycle += 1) {
      for (const mode of shutdownWindowControl ? (['shutdown'] as const) : (['cancel', 'shutdown'] as const)) {
        await measurementScope(async (uploadCleanup) => {
          const peer = await startPeer();
          const token = await accessToken(deployment, account);
          stage = 'admit_streaming_upload';
          const upload = await begin(deployment, peer.apiUrl, token, channel, uploadCleanup);
          const before = performance.now();
          try {
            if (mode === 'cancel') upload.cancel();
            if (mode === 'shutdown') {
              stage = 'shutdown_peer';
              await peer.stop(shutdownWindowControl ? undefined : { shutdownWindow: 'upload_drain' });
            }
          } finally {
            upload.cancel();
          }
          stage = 'upload_outcome';
          const status = await uploadOutcome(upload.outcome);
          if (mode === 'cancel') await peer.stop();
          if (status >= 200 && status < 300) throw new Error('interrupted upload reported success');
          stage = 'worker_recovery';
          await advanceAndDrain(deployment);
          stage = 'durable_quiescence';
          await untilMeasurementCondition(() => query('SELECT count(*) FROM upload_intents') === '0', 30_000);
          const capacity = Number(query('SELECT used FROM upload_intent_capacity'));
          const receipts = Number(query('SELECT count(*) FROM upload_cleanup_receipts'));
          const uploading = Number(query("SELECT count(*) FROM attachments WHERE state='uploading'"));
          recordMeasurement({
            event: 'measurement.upload_process_cycle',
            sample: index,
            cycle,
            mode,
            status,
            durationMs: performance.now() - before,
            capacity,
            receipts,
            uploading,
            orphanObjects: 'unverified_in_process_probe',
            incompleteMultipart: 'unverified_in_process_probe',
          });
          if (capacity !== 0 || receipts !== 0 || uploading !== 0)
            throw new Error('interrupted upload durable resources did not drain');
        });
      }
    }
  }).catch((error: unknown) => {
    recordMeasurement({
      event: 'measurement.upload_process_stage_failed',
      sample: index,
      stage,
      kind: 'operation_or_cleanup_failed',
    });
    throw error;
  });
}

async function run(): Promise<void> {
  if (shutdownWindowControl && !measurementSmoke) throw new Error('shutdown control requires smoke mode');
  takeRunLock(process.env);
  try {
    prepareRun();
    recordMeasurement({
      event: 'measurement.protocol',
      mode: measurementSmoke ? 'smoke_not_performance_evidence' : 'full',
      seed: 'slice6-upload-process-1',
      samples: measurementCount(3, 1),
      cyclesPerMode: measurementCount(100, 1),
      modes: shutdownWindowControl ? 'shutdown_negative_control' : 'cancel,shutdown',
      shutdownWindowMilliseconds: shutdownWindowControl ? 20_000 : 45_000,
      recoveryClockAdvanceSeconds: 1300,
    });
    for (let index = 1; index <= measurementCount(3, 1); index += 1) await sample(index);
  } finally {
    finishRun();
  }
}

try {
  await run();
} catch {
  recordMeasurement({ event: 'measurement.failed', kind: 'upload_process_run_failed' });
  process.exitCode = 1;
}
