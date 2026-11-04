import assert from 'node:assert/strict';

import { chromium, firefox, webkit } from '@playwright/test';

import { seedAccount } from '../e2e/accounts.ts';
import { installPolicyObserver, observeBrowserContext } from '../e2e/browser-diagnostics.ts';
import { finishRun, prepareRun, readE2ESettings, startDeployment, takeRunLock } from '../e2e/deployment.ts';
import { accountId } from '../e2e/realtime.ts';
import { channelOf } from '../e2e/messages.ts';
import { accessToken, callApi, createGroup, seedActiveStudent } from '../e2e/tutoring.ts';
import { observeDevelopmentReads } from './development-read-observer.ts';
import { measurementScope, recordMeasurement } from './slice6-measurement-support.ts';

async function run(): Promise<void> {
  takeRunLock(process.env);
  try {
    prepareRun('measurement-dev-tls');
    await measurementScope(async (cleanup) => {
      const deployment = await startDeployment(
        readE2ESettings(process.env),
        0,
        'development-warning',
        undefined,
        'measurement-dev-tls',
      );
      cleanup(() => deployment.stop());
      assert.equal(new URL(deployment.origin).hostname, '127.0.0.1');
      assert.equal(new URL(deployment.origin).protocol, 'https:');
      const tutor = await seedAccount(deployment, 'Synthetic tutor', 'tutor', 'en');
      const student = await seedActiveStudent(deployment, tutor, 'Synthetic student', 'en');
      const groups = await Promise.all(
        ['First synthetic group', 'Second synthetic group'].map((name) =>
          createGroup(deployment, tutor, name, [student.cardId]),
        ),
      );
      const channels = await Promise.all(groups.map((group) => channelOf(deployment, tutor, `/groups/${group}`)));
      const seedToken = await accessToken(deployment, tutor);
      for (const channel of channels) {
        const seeded = await callApi(deployment, seedToken, 'POST', `/channels/${channel}/messages`, {
          body: 'Synthetic development history',
          attachment_ids: [],
        });
        assert.equal(seeded.status, 201);
      }
      const initialControl = process.argv.includes('--initial-control');
      const diagnostic = process.argv.includes('--diagnostic-navigation');
      if (diagnostic && initialControl) throw new Error('development_diagnostic_mode_conflict');
      let diagnosticFailures = 0;
      for (const engine of initialControl ? [chromium] : [chromium, firefox, webkit]) {
        await measurementScope(async (release) => {
          const browser = await engine.launch();
          release(() => browser.close());
          for (const account of initialControl ? [tutor] : [tutor, student.student]) {
            try {
              await measurementScope(async (close) => {
                const context = await browser.newContext({ ignoreHTTPSErrors: true, baseURL: deployment.origin });
                close(() => context.close());
                const ownedAccount = await accountId(deployment, account);
                const observer = observeBrowserContext(context, engine.name(), 'development_smoke');
                await installPolicyObserver(context);
                let httpFailures = 0;
                let successes = 0;
                context.on('response', (response) => {
                  if (response.status() >= 400) httpFailures += 1;
                  if (response.status() === 200 && new URL(response.url()).pathname.startsWith('/api/v1/'))
                    successes += 1;
                });
                const page = await context.newPage();
                const reads = observeDevelopmentReads(page, deployment.origin);
                let stage = 'bootstrap';
                let armed = false;
                try {
                  const bootstrap = await page.goto('/api/v1/health/live');
                  assert.equal(bootstrap?.status(), 200);
                  stage = 'native_login';
                  const loginRequest = page.waitForRequest(
                    (request) =>
                      request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/auth/login',
                  );
                  const signedIn = await page.evaluate(
                    async ({ email, password }) => {
                      const response = await fetch('/api/v1/auth/login', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ email, password }),
                      });
                      await response.arrayBuffer();
                      return response.status;
                    },
                    { email: account.email, password: account.password },
                  );
                  assert.equal(signedIn, 200);
                  assert.equal((await (await loginRequest).headerValue('origin')) === deployment.origin, true);
                  const firstChannel = channels[0];
                  const secondChannel = channels[1];
                  if (firstChannel === undefined || secondChannel === undefined)
                    throw new Error('development_seed_channel_missing');
                  reads.arm(ownedAccount, firstChannel);
                  armed = true;
                  stage = 'initial_history';
                  await page.goto(`/messages/${firstChannel}`);
                  await page.getByRole('textbox', { name: 'Message', exact: true }).waitFor();
                  const history = page.getByRole('list', { name: 'Messages', exact: true });
                  await history.waitFor();
                  await history.getByText('Synthetic development history', { exact: true }).waitFor();
                  const cookie = (await context.cookies()).find((entry) => entry.name.startsWith('__Host-'));
                  assert.equal(cookie?.secure, true);
                  assert.equal(cookie.httpOnly, true);
                  stage = 'initial_authoritative_settlement';
                  await reads.waitInitial();
                  reads.sealInitial();
                  const refreshed = await page.evaluate(async () => {
                    const response = await fetch('/api/v1/auth/refresh', { method: 'POST' });
                    await response.arrayBuffer();
                    return response.status;
                  });
                  assert.equal(refreshed, 200);
                  await reads.waitSettled();
                  reads.report(engine.name(), stage);
                  if (!initialControl) {
                    stage = 'channel_switch';
                    const beforeSwitch = reads.mark();
                    if (diagnostic) reads.declareDiagnosticDestination(secondChannel);
                    else reads.armNavigation(firstChannel, secondChannel);
                    await page.locator(`a[href="/messages/${secondChannel}"]`).first().click();
                    await page.getByRole('textbox', { name: 'Message', exact: true }).waitFor();
                    await history.waitFor();
                    await history.getByText('Synthetic development history', { exact: true }).waitFor();
                    if (diagnostic) {
                      try {
                        await reads.waitChannel(secondChannel, beforeSwitch);
                      } catch {
                        diagnosticFailures += 1;
                        recordMeasurement({
                          event: 'browser.development_diagnostic_assertion_failed',
                          engine: engine.name(),
                          stage,
                          kind: 'original_read_assertion_failed',
                        });
                      }
                    } else {
                      await reads.waitChannel(secondChannel, beforeSwitch);
                      reads.sealNavigation();
                    }
                    stage = 'settings_navigation';
                    await page.getByRole('button', { name: `${account.name}, Account menu`, exact: true }).click();
                    await page.getByRole('link', { name: 'Settings', exact: true }).last().click();
                    await page.getByRole('heading', { level: 1, name: 'Profile', exact: true }).waitFor();
                    assert.equal(new URL(page.url()).pathname === '/settings/profile', true);
                    try {
                      await reads.waitSettled();
                    } catch (error: unknown) {
                      if (!diagnostic) throw error;
                      diagnosticFailures += 1;
                      recordMeasurement({
                        event: 'browser.development_diagnostic_assertion_failed',
                        engine: engine.name(),
                        stage,
                        kind: 'original_settlement_assertion_failed',
                      });
                    }
                  }
                  await context.close();
                  await reads.finish();
                  for (const assertion of [
                    (): void => {
                      reads.verify();
                    },
                    (): void => {
                      observer.verify();
                    },
                    (): void => {
                      assert.equal(httpFailures, 0);
                    },
                    (): void => {
                      assert.equal(reads.unexpectedFailures(), 0);
                    },
                    (): void => {
                      assert.ok(successes > 0);
                    },
                  ]) {
                    try {
                      assertion();
                    } catch (error: unknown) {
                      if (!diagnostic) throw error;
                      diagnosticFailures += 1;
                      recordMeasurement({
                        event: 'browser.development_diagnostic_assertion_failed',
                        engine: engine.name(),
                        stage,
                        kind: 'original_final_assertion_failed',
                      });
                    }
                  }
                  recordMeasurement({
                    event: diagnostic
                      ? 'browser.development_diagnostic_journey_completed'
                      : initialControl
                        ? 'browser.development_initial_control'
                        : 'browser.development_smoke',
                    engine: engine.name(),
                    role: account === tutor ? 'tutor' : 'student',
                    consoleWarnings: observer.warnings(),
                    consoleErrors: observer.errors(),
                    httpFailures,
                    rawRequestFailures: reads.rawFailures(),
                    expectedInitialCancellations: reads.expectedFailures(),
                    expectedNavigationCancellations: reads.navigationExpected(),
                    unexpectedRequestFailures: reads.unexpectedFailures(),
                    secureCookie: true,
                  });
                } finally {
                  await reads.finish();
                  if (armed) reads.report(engine.name(), stage);
                  if (diagnostic) reads.diagnosticReport(engine.name(), stage);
                  for (const diagnostic of observer.records())
                    recordMeasurement({ event: 'browser.development_diagnostic', ...diagnostic });
                }
              });
            } catch (error: unknown) {
              if (!diagnostic) throw error;
              diagnosticFailures += 1;
              recordMeasurement({
                event: 'browser.development_diagnostic_journey_failed',
                engine: engine.name(),
                role: account === tutor ? 'tutor' : 'student',
                kind: 'original_assertion_or_cleanup_failed',
                dependentStepsBlocked: true,
              });
            }
          }
        });
      }
      if (diagnostic) {
        recordMeasurement({
          event: 'browser.development_navigation_diagnostic',
          kind: 'diagnostic_not_acceptance',
          failures: diagnosticFailures,
        });
        if (diagnosticFailures !== 0) throw new Error('development_diagnostic_original_assertions_failed');
      }
    });
  } finally {
    finishRun();
  }
}

try {
  await run();
} catch {
  recordMeasurement({ event: 'browser.development_smoke_failed', kind: 'verification_failed' });
  process.exitCode = 1;
}
