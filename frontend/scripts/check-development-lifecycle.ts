import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { createConnection, createServer, type Server } from 'node:net';

import { chromium } from '@playwright/test';

import { installPolicyObserver, observeBrowserContext } from '../e2e/browser-diagnostics.ts';
import {
  type DevelopmentLifecycleObserver,
  type Deployment,
  finishRun,
  prepareRun,
  readE2ESettings,
  startDeployment,
  takeRunLock,
} from '../e2e/deployment.ts';
import { measurementScope, recordMeasurement, untilMeasurementCondition } from './slice6-measurement-support.ts';

function reservation(port: number): Promise<Server> {
  return new Promise((resolve, reject) => {
    const server = createServer((socket) => {
      socket.destroy();
    });
    server.once('error', reject);
    server.listen(port, '127.0.0.1', () => {
      resolve(server);
    });
  });
}

function release(server: Server): Promise<void> {
  return new Promise((resolve, reject) => {
    server.close((error) => {
      if (error === undefined) resolve();
      else reject(error);
    });
  });
}

function portIsOpen(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const socket = createConnection({ host: '127.0.0.1', port });
    socket.once('connect', () => {
      socket.destroy();
      resolve(true);
    });
    socket.once('error', () => {
      resolve(false);
    });
  });
}

function processIsAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    if (typeof error === 'object' && error !== null && 'code' in error && error.code === 'ESRCH') return false;
    throw error;
  }
}

function resources(): { readonly observer: DevelopmentLifecycleObserver; readonly verify: () => Promise<void> } {
  const children: number[] = [];
  const directories: string[] = [];
  return {
    observer: {
      processStarted: (pid): void => {
        children.push(pid);
      },
      temporaryTlsCreated: (directory): void => {
        directories.push(directory);
      },
    },
    verify: async (): Promise<void> => {
      assert.equal(directories.length, 1);
      assert.ok(children.length >= 3);
      await untilMeasurementCondition(() => children.every((pid) => !processIsAlive(pid)), 20_000);
      assert.equal(
        directories.every((directory) => !existsSync(directory)),
        true,
      );
      assert.equal(await portIsOpen(4300), false);
      assert.equal(await portIsOpen(18800), false);
    },
  };
}

function startupLeaves(error: unknown): readonly string[] {
  if (error instanceof AggregateError) {
    const nested: readonly unknown[] = error.errors;
    return nested.flatMap(startupLeaves);
  }
  if (error instanceof Error) return [error.message];
  throw new Error('unexpected_startup_failure_shape');
}

async function startupFailure(): Promise<void> {
  const tracked = resources();
  let held: Server | undefined;
  let rejected: unknown;
  let unexpectedDeployment: Deployment | undefined;
  try {
    unexpectedDeployment = await startDeployment(
      readE2ESettings(process.env),
      0,
      'development-start-failure',
      undefined,
      'measurement-dev-tls',
      {
        ...tracked.observer,
        beforeFrontendStart: async (port): Promise<void> => {
          held = await reservation(port);
        },
      },
    );
  } catch (error) {
    rejected = error;
  } finally {
    if (held !== undefined) await release(held);
    if (unexpectedDeployment !== undefined) await unexpectedDeployment.stop();
  }
  const leaves = startupLeaves(rejected);
  assert.ok(leaves.includes('development_server_exited_before_https_readiness'));
  assert.equal(
    leaves.every((message) =>
      [
        'development_server_exited_before_https_readiness',
        'development stopped unexpectedly with exit code 1',
        'development stopped with exit code 1 after SIGTERM',
      ].includes(message),
    ),
    true,
  );
  await tracked.verify();
  recordMeasurement({
    event: 'browser.development_startup_failure',
    realBindFailure: true,
    ownedProcessesExited: true,
    temporaryTlsRemoved: true,
    portsReleased: true,
  });
}

async function navigationFailure(): Promise<void> {
  const tracked = resources();
  await measurementScope(async (cleanup) => {
    const deployment = await startDeployment(
      readE2ESettings(process.env),
      0,
      'development-navigation-failure',
      undefined,
      'measurement-dev-tls',
      tracked.observer,
    );
    cleanup(() => deployment.stop());
    const browser = await chromium.launch();
    cleanup(() => browser.close());
    const context = await browser.newContext({ ignoreHTTPSErrors: true, baseURL: deployment.origin });
    cleanup(() => context.close());
    const observer = observeBrowserContext(context, 'chromium', 'navigation_failure_control');
    await installPolicyObserver(context);
    const page = await context.newPage();
    const held = await reservation(0);
    const address = held.address();
    if (address === null || typeof address === 'string') throw new Error('navigation_failure_address_unavailable');
    await release(held);
    const destination = `http://127.0.0.1:${String(address.port)}/`;
    let failures = 0;
    context.on('requestfailed', (request) => {
      if (request.url() === destination && request.isNavigationRequest()) failures += 1;
    });
    let rejected = false;
    try {
      await page.goto(destination);
    } catch {
      rejected = true;
    }
    assert.equal(rejected, true);
    assert.equal(failures, 1);
    assert.equal(observer.warnings(), 0);
    assert.equal(observer.errors(), 0);
    observer.verify();
  });
  await tracked.verify();
  recordMeasurement({
    event: 'browser.development_navigation_failure',
    realNavigationFailure: true,
    ownedProcessesExited: true,
    temporaryTlsRemoved: true,
    portsReleased: true,
  });
}

async function run(): Promise<void> {
  takeRunLock(process.env);
  try {
    prepareRun('measurement-dev-tls');
    await startupFailure();
    await navigationFailure();
  } finally {
    finishRun();
  }
}

try {
  await run();
} catch {
  recordMeasurement({ event: 'browser.development_lifecycle_failed', kind: 'verification_failed' });
  process.exitCode = 1;
}
