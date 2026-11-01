import { performance } from 'node:perf_hooks';

import { type BrowserType, chromium, errors, firefox, webkit } from '@playwright/test';

import { seedAccount, signInThroughTheApi } from '../e2e/accounts.ts';
import { finishRun, prepareRun, readE2ESettings, startDeployment, takeRunLock } from '../e2e/deployment.ts';
import { channelOf } from '../e2e/messages.ts';
import { accountId } from '../e2e/realtime.ts';
import { createGroup, seedActiveStudent } from '../e2e/tutoring.ts';
import { seedMeasurementPictures } from './slice6-browser-seed.ts';
import {
  measurementScope,
  measurementCount,
  measurementSmoke,
  browserProcessMemory,
  measurementIdentifier,
  recordMeasurement,
  untilMeasurementCondition,
} from './slice6-measurement-support.ts';

async function sample(engine: BrowserType, sampleNumber: number): Promise<void> {
  await measurementScope(async (cleanup) => {
    const deployment = await startDeployment(readE2ESettings(process.env), 0, `browser-${String(sampleNumber)}`, {
      synchronousCommit: 'on',
    });
    cleanup(() => deployment.stop());
    const browserServer = await engine.launchServer({ host: '127.0.0.1' });
    cleanup(() => browserServer.close());
    const browserPid = browserServer.process().pid;
    if (browserPid === undefined) throw new Error('owned browser PID unavailable');
    const browser = await engine.connect(browserServer.wsEndpoint());
    cleanup(() => browser.close());
    const tutor = await seedAccount(deployment, 'Measurement tutor', 'tutor', 'en');
    const student = await seedActiveStudent(deployment, tutor, 'Measurement student', 'en');
    const group = await createGroup(deployment, tutor, 'History measurement', [student.cardId]);
    const pictureMembers = [student];
    for (let index = 0; index < 3; index += 1) {
      pictureMembers.push(await seedActiveStudent(deployment, tutor, 'Measurement student', 'en'));
    }
    const otherGroup = await createGroup(
      deployment,
      tutor,
      'Picture measurement',
      pictureMembers.map((member) => member.cardId),
    );
    const channel = measurementIdentifier(await channelOf(deployment, tutor, `/groups/${group}`));
    const otherChannel = measurementIdentifier(await channelOf(deployment, tutor, `/groups/${otherGroup}`));
    const sender = measurementIdentifier(await accountId(deployment, student.student));
    if (deployment.measurementQuery === undefined) {
      throw new Error('measurement seed connection unavailable');
    }
    deployment.measurementQuery(`INSERT INTO messages (id, channel_id, seq, sender_id, body, version, created_at, updated_at)
SELECT md5('browser-history-' || seq)::uuid, '${channel}'::uuid, seq, '${sender}'::uuid, 'Synthetic history ' || seq, 1, now(), now()
FROM generate_series(1,${String(measurementCount(1000, 100))}) AS series(seq);
UPDATE channels SET last_seq=${String(measurementCount(1000, 100))}, last_activity_at=now() WHERE id='${channel}'::uuid;`);
    await seedMeasurementPictures(
      deployment,
      pictureMembers.map((member) => member.student),
      otherChannel,
    );
    const context = await browser.newContext({ ignoreHTTPSErrors: true, baseURL: deployment.origin });
    cleanup(() => context.close());
    const page = await context.newPage();
    await page.addInitScript(() => {
      const target = window as Window & { slice6LayoutShift?: number | null };
      target.slice6LayoutShift = PerformanceObserver.supportedEntryTypes.includes('layout-shift') ? 0 : null;
      if (target.slice6LayoutShift !== null) {
        const observer = new PerformanceObserver((list) => {
          for (const entry of list.getEntries()) {
            if (
              'value' in entry &&
              typeof entry.value === 'number' &&
              'hadRecentInput' in entry &&
              entry.hadRecentInput === false &&
              typeof target.slice6LayoutShift === 'number'
            )
              target.slice6LayoutShift += entry.value;
          }
        });
        observer.observe({ type: 'layout-shift', buffered: true });
      }
    });
    let activeDownloads = 0;
    let peakDownloads = 0;
    let diagnostics = 0;
    let requests = 0;
    let completedRequests = 0;
    let downloadedBytes = 0;
    let noStoreFailures = 0;
    let peakRssKiB = 0;
    let processSamples = 0;
    const sampling = { failed: false };
    const memoryTimer = setInterval(() => {
      try {
        const memory = browserProcessMemory(browserPid);
        if (memory.processes === 0) throw new Error('browser process unavailable');
        peakRssKiB = Math.max(peakRssKiB, memory.rssKiB);
        processSamples += 1;
      } catch {
        sampling.failed = true;
      }
    }, 100);
    const networkSamples: Promise<void>[] = [];
    page.on('response', (response) => {
      if (new URL(response.url()).pathname.includes('/api/v1/files/')) {
        networkSamples.push(
          (async (): Promise<void> => {
            const cache = await response.headerValue('cache-control');
            if (!cache?.includes('no-store')) noStoreFailures += 1;
          })().catch(() => {
            diagnostics += 1;
          }),
        );
      }
    });
    page.on('request', (request) => {
      requests += 1;
      if (new URL(request.url()).pathname.includes('/api/v1/files/')) {
        activeDownloads += 1;
        peakDownloads = Math.max(peakDownloads, activeDownloads);
      }
    });
    const complete = (address: string): void => {
      completedRequests += 1;
      if (new URL(address).pathname.includes('/api/v1/files/')) {
        activeDownloads -= 1;
      }
    };
    page.on('requestfinished', (request) => {
      complete(request.url());
      if (new URL(request.url()).pathname.includes('/api/v1/files/'))
        networkSamples.push(
          request.sizes().then(
            (sizes) => {
              downloadedBytes += sizes.responseBodySize;
            },
            () => {
              diagnostics += 1;
            },
          ),
        );
    });
    page.on('requestfailed', (request) => {
      complete(request.url());
    });
    page.on('pageerror', () => {
      diagnostics += 1;
    });
    page.on('console', (message) => {
      if (message.type() === 'error') diagnostics += 1;
    });
    let stage = 'sign_in';
    try {
      await signInThroughTheApi(page, tutor);
      stage = 'initial_render';
      const baselineMemory = browserProcessMemory(browserPid);
      const initialStart = performance.now();
      await page.goto(`/messages/${channel}`);
      await page.getByRole('textbox', { name: 'Message', exact: true }).waitFor();
      recordMeasurement({
        event: 'measurement.initial_render',
        engine: engine.name(),
        sample: sampleNumber,
        durationMs: performance.now() - initialStart,
      });
      stage = 'history_pages';
      for (let index = 0; index < measurementCount(19, 1); index += 1) {
        await page.getByRole('button', { name: 'Show older', exact: true }).click();
        await page
          .getByRole('list', { name: 'Messages', exact: true })
          .getByRole('listitem')
          .nth(50 * (index + 2) - 1)
          .waitFor();
      }
      const rows = await page.getByRole('list', { name: 'Messages', exact: true }).getByRole('listitem').count();
      const milestones = await page.evaluate(() => {
        const navigation = window.performance.getEntriesByType('navigation')[0];
        const paint = window.performance.getEntriesByName('first-contentful-paint')[0];
        return {
          domContentLoadedMs:
            navigation instanceof PerformanceNavigationTiming ? navigation.domContentLoadedEventEnd : -1,
          loadMs: navigation instanceof PerformanceNavigationTiming ? navigation.loadEventEnd : -1,
          firstContentfulPaintMs: paint?.startTime ?? -1,
        };
      });
      recordMeasurement({
        event: 'measurement.loading_milestones',
        sample: sampleNumber,
        engine: engine.name(),
        ...milestones,
      });
      stage = 'history_input';
      const inputStart = performance.now();
      await page.getByRole('textbox', { name: 'Message', exact: true }).fill('Synthetic input latency sample');
      await page.evaluate(
        () =>
          new Promise<void>((resolve) =>
            requestAnimationFrame(() => {
              resolve();
            }),
          ),
      );
      recordMeasurement({
        event: 'measurement.history_render',
        engine: engine.name(),
        sample: sampleNumber,
        rows,
        inputMs: performance.now() - inputStart,
        domNodes: await page.locator('*').count(),
      });
      stage = 'channel_switches';
      for (let cycle = 0; cycle < measurementCount(100, 2); cycle += 1) {
        const next = cycle % 2 === 0 ? otherChannel : channel;
        const started = performance.now();
        await page.locator(`a[href="/messages/${next}"]`).click();
        await page.getByRole('textbox', { name: 'Message', exact: true }).waitFor();
        recordMeasurement({
          event: 'measurement.channel_switch',
          engine: engine.name(),
          sample: sampleNumber,
          cycle,
          durationMs: performance.now() - started,
          domNodes: await page.locator('*').count(),
          activeDownloads,
          peakDownloads,
        });
      }
      stage = 'pictures';
      await page.locator(`a[href="/messages/${otherChannel}"]`).click();
      const pictureStart = performance.now();
      for (let index = 0; index < measurementCount(3, 0); index += 1) {
        await page.getByRole('button', { name: 'Show older', exact: true }).click();
        await page
          .getByRole('list', { name: 'Messages', exact: true })
          .getByRole('listitem')
          .nth(50 * (index + 2) - 1)
          .waitFor();
      }
      const pictures = page.getByRole('img', { name: 'measurement.png', exact: true });
      for (let index = 0; index < measurementCount(200, 8); index += 1) {
        await pictures.nth(index).scrollIntoViewIfNeeded();
        await pictures.nth(index).evaluate(async (image) => {
          if (!(image instanceof HTMLImageElement)) throw new Error('picture element missing');
          await image.decode();
        });
      }
      recordMeasurement({
        event: 'measurement.pictures',
        sample: sampleNumber,
        engine: engine.name(),
        pictures: await pictures.count(),
        displayMs: performance.now() - pictureStart,
        activeDownloads,
        peakDownloads,
      });
      stage = 'concurrent_uploads';
      const buffer = Buffer.alloc(measurementCount(25_000_000, 4096), 120);
      const uploadStart = performance.now();
      await page.locator('input[type="file"]').setInputFiles(
        Array.from({ length: 4 }, (_, index) => ({
          name: `measurement-${String(index)}.txt`,
          mimeType: 'text/plain',
          buffer,
        })),
      );
      await page
        .getByRole('list', { name: 'Attachments', exact: true })
        .getByText('Uploaded', { exact: true })
        .nth(3)
        .waitFor();
      recordMeasurement({
        event: 'measurement.browser_uploads',
        engine: engine.name(),
        sample: sampleNumber,
        concurrency: 4,
        bytesEach: buffer.byteLength,
        durationMs: performance.now() - uploadStart,
        nativeMemory: 'process_rss_recorded',
      });
      for (let index = 0; index < 4; index += 1) {
        stage = `remove_upload_${String(index + 1)}`;
        await page.getByRole('button', { name: 'Remove file', exact: true }).first().click();
      }
      stage = 'settings_navigation';
      await page.getByRole('link', { name: 'Settings', exact: true }).click();
      stage = 'profile_heading';
      await page.getByRole('heading', { name: 'Profile', level: 1, exact: true }).waitFor();
      if (engine.name() === 'chromium') {
        stage = 'chromium_cdp_session';
        const session = await context.newCDPSession(page);
        try {
          stage = 'chromium_dom_counters';
          const counters: unknown = await session.send('Memory.getDOMCounters');
          if (
            typeof counters === 'object' &&
            counters !== null &&
            'documents' in counters &&
            typeof counters.documents === 'number' &&
            'nodes' in counters &&
            typeof counters.nodes === 'number' &&
            'jsEventListeners' in counters &&
            typeof counters.jsEventListeners === 'number'
          ) {
            recordMeasurement({
              event: 'measurement.chromium_dom',
              sample: sampleNumber,
              documents: counters.documents,
              nodes: counters.nodes,
              listeners: counters.jsEventListeners,
            });
          } else {
            throw new Error('Chromium resource counters unavailable');
          }
        } finally {
          await session.detach();
        }
      }
      stage = 'network_quiescence';
      await untilMeasurementCondition(() => requests === completedRequests && activeDownloads === 0, 10_000);
      await Promise.all(networkSamples);
      const afterMemory = browserProcessMemory(browserPid);
      const layoutShift = await page.evaluate(
        () => (window as Window & { slice6LayoutShift?: number | null }).slice6LayoutShift,
      );
      recordMeasurement({
        event: 'measurement.layout_shift',
        sample: sampleNumber,
        engine: engine.name(),
        value: layoutShift ?? -1,
        status: layoutShift === null ? 'unverified_engine_unsupported' : 'observed',
      });
      recordMeasurement({
        event: 'measurement.browser_native_memory',
        sample: sampleNumber,
        engine: engine.name(),
        peakRssKiB,
        baselineRssKiB: baselineMemory.rssKiB,
        postCleanupRssKiB: afterMemory.rssKiB,
        processes: afterMemory.processes,
        processSamples,
        downloadedBytes,
        noStoreFailures,
        samplingFailure: sampling.failed,
      });
      recordMeasurement({
        event: 'measurement.browser_cleanup',
        sample: sampleNumber,
        engine: engine.name(),
        requests,
        completedRequests,
        activeDownloads,
        diagnostics,
        heapRetainers: 'unverified',
        timerRetainers: 'unverified',
        boundedQueryCaches: 'unverified',
        nativeMemory: 'process_rss_recorded',
        expectedPictures: measurementCount(200, 8),
      });
      stage = 'browser_assertions';
      if (
        rows !== measurementCount(1000, 100) ||
        diagnostics !== 0 ||
        activeDownloads !== 0 ||
        sampling.failed ||
        noStoreFailures !== 0
      ) {
        throw new Error('measurement browser assertions failed');
      }
    } catch (error) {
      recordMeasurement({
        event: 'measurement.browser_stage_failed',
        engine: engine.name(),
        sample: sampleNumber,
        stage,
        kind: error instanceof errors.TimeoutError ? 'timeout' : 'operation_failed',
      });
      throw error;
    } finally {
      clearInterval(memoryTimer);
    }
  });
}

async function run(): Promise<void> {
  const engines: readonly BrowserType[] = [chromium, firefox, webkit];
  takeRunLock(process.env);
  try {
    prepareRun();
    recordMeasurement({
      event: 'measurement.protocol',
      mode: measurementSmoke ? 'smoke_not_performance_evidence' : 'full',
      seed: 'slice6-browser-1',
      samples: measurementCount(3, 1),
      historyRows: measurementCount(1000, 100),
      channelSwitches: measurementCount(100, 2),
      pictures: measurementCount(200, 8),
      warmup: 'initial_navigation',
      referenceCertification: false,
    });
    for (const engine of engines) {
      for (let index = 1; index <= measurementCount(3, 1); index += 1) {
        await sample(engine, index);
      }
    }
  } finally {
    finishRun();
  }
}

try {
  await run();
} catch {
  recordMeasurement({ event: 'measurement.failed', kind: 'browser_run_failed' });
  process.exitCode = 1;
}
