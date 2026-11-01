import { execFileSync } from 'node:child_process';
import { cpus, freemem, totalmem } from 'node:os';
import { performance } from 'node:perf_hooks';

import { type Browser, type BrowserType, chromium, firefox, webkit } from '@playwright/test';

import { seedAccount, signInThroughTheApi } from '../e2e/accounts.ts';
import { finishRun, prepareRun, readE2ESettings, startDeployment, takeRunLock } from '../e2e/deployment.ts';

import { measurementCount, measurementSmoke } from './slice6-measurement-support.ts';

interface Options {
  readonly engine: 'chromium' | 'firefox' | 'webkit';
  readonly surface: 'blank' | 'messages';
  readonly commit: 'on' | 'off';
}

function options(args: readonly string[]): Options {
  const [engine, surface, commit] = args;
  if (
    args.length !== 3 ||
    (engine !== 'chromium' && engine !== 'firefox' && engine !== 'webkit') ||
    (surface !== 'blank' && surface !== 'messages') ||
    (commit !== 'on' && commit !== 'off')
  ) {
    throw new Error('usage: node scripts/measure-slice6-contexts.ts chromium|firefox|webkit blank|messages on|off');
  }
  return { engine, surface, commit };
}

function emit(record: Readonly<Record<string, string | number | boolean>>): void {
  process.stdout.write(`${JSON.stringify(record)}\n`);
}

function machine(sample: number, stage: string): void {
  emit({
    event: 'measurement.machine',
    sample,
    stage,
    cores: cpus().length,
    freeBytes: freemem(),
    totalBytes: totalmem(),
  });
  process.stdout.write(execFileSync('ps', ['-axo', 'pid,ppid,pcpu,rss,comm'], { encoding: 'utf8' }));
}

async function measureContexts(browser: Browser, configuration: Options, sample: number): Promise<void> {
  const deployment = await startDeployment(readE2ESettings(process.env), 0, `measurement-${String(sample)}`, {
    synchronousCommit: configuration.commit,
  });
  try {
    const account = await seedAccount(deployment, 'Measurement tutor', 'tutor', 'en');
    for (let cycle = 0; cycle < measurementCount(110, 3); cycle += 1) {
      const started = performance.now();
      const context = await browser.newContext({ ignoreHTTPSErrors: true, baseURL: deployment.origin });
      const contextMs = performance.now() - started;
      let openedSockets = 0;
      let closedSockets = 0;
      let failedRequests = 0;
      let consoleErrors = 0;
      let pageErrors = 0;
      try {
        const beforePage = performance.now();
        const page = await context.newPage();
        const pageMs = performance.now() - beforePage;
        page.on('websocket', (socket) => {
          openedSockets += 1;
          socket.on('close', () => {
            closedSockets += 1;
          });
        });
        page.on('requestfailed', () => {
          failedRequests += 1;
        });
        page.on('pageerror', () => {
          pageErrors += 1;
        });
        page.on('console', (message) => {
          if (message.type() === 'error') {
            consoleErrors += 1;
          }
        });
        const beforeSignIn = performance.now();
        if (configuration.surface === 'messages') {
          await signInThroughTheApi(page, account);
        }
        const signInMs = performance.now() - beforeSignIn;
        const beforeNavigation = performance.now();
        await page.goto(configuration.surface === 'blank' ? 'about:blank' : '/messages');
        if (configuration.surface === 'messages') {
          await page.getByRole('searchbox', { name: 'Search chats' }).waitFor();
        }
        const navigationMs = performance.now() - beforeNavigation;
        const domNodes = await page.locator('*').count();
        const beforeClose = performance.now();
        await context.close();
        const closeMs = performance.now() - beforeClose;
        emit({
          event: 'measurement.context',
          sample,
          cycle,
          phase: cycle < measurementCount(10, 1) ? 'warmup' : 'sample',
          contextMs,
          pageMs,
          signInMs,
          navigationMs,
          closeMs,
          domNodes,
          openedSockets,
          closedSockets,
          failedRequests,
          consoleErrors,
          pageErrors,
          remainingContexts: browser.contexts().length,
          remainingPages: context.pages().length,
          engine: configuration.engine,
          surface: configuration.surface,
          synchronousCommit: configuration.commit,
        });
        if (
          browser.contexts().length !== 0 ||
          context.pages().length !== 0 ||
          pageErrors !== 0 ||
          consoleErrors !== 0
        ) {
          throw new Error('context cleanup or page diagnostics failed');
        }
      } finally {
        if (browser.contexts().includes(context)) {
          await context.close();
        }
      }
    }
  } finally {
    await deployment.stop();
  }
}

async function run(): Promise<void> {
  const configuration = options(process.argv.slice(2).filter((argument) => argument !== '--smoke'));
  const engines: Readonly<Record<Options['engine'], BrowserType>> = { chromium, firefox, webkit };
  takeRunLock(process.env);
  try {
    prepareRun();
    emit({
      event: 'measurement.protocol',
      mode: measurementSmoke ? 'smoke_not_performance_evidence' : 'full',
      seed: 'slice6-context-1',
      samples: measurementCount(3, 1),
      warmupCycles: measurementCount(10, 1),
      cycles: measurementCount(100, 2),
      localReferenceCertification: false,
      heapRetention: 'unverified',
      nativeBrowserMemory: 'unverified',
      timerRetention: 'unverified',
      listenerRetention: 'unverified',
      engine: configuration.engine,
      surface: configuration.surface,
      synchronousCommit: configuration.commit,
    });
    for (let sample = 1; sample <= measurementCount(3, 1); sample += 1) {
      machine(sample, 'before');
      const browser = await engines[configuration.engine].launch();
      try {
        emit({ event: 'measurement.browser', sample, version: browser.version() });
        await measureContexts(browser, configuration, sample);
      } finally {
        await browser.close();
        machine(sample, 'after');
      }
    }
  } finally {
    finishRun();
  }
}

try {
  await run();
} catch {
  emit({ event: 'measurement.failed', kind: 'run_failed' });
  process.exitCode = 1;
}
