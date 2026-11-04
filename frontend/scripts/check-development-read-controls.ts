import assert from 'node:assert/strict';

import { chromium, type ConsoleMessage, type Page, type Response, type Route } from '@playwright/test';

import { type TestAccount, seedAccount } from '../e2e/accounts.ts';
import { installPolicyObserver, observeBrowserContext } from '../e2e/browser-diagnostics.ts';
import {
  finishRun,
  prepareRun,
  readE2ESettings,
  startDeployment,
  takeRunLock,
  type Deployment,
} from '../e2e/deployment.ts';
import { channelOf } from '../e2e/messages.ts';
import { accountId } from '../e2e/realtime.ts';
import { accessToken, callApi, createGroup, seedActiveStudent } from '../e2e/tutoring.ts';
import { continuationErrorKind, type ContinuationErrorKind } from './development-continuation.ts';
import { developmentReadinessMilliseconds, observeDevelopmentReads } from './development-read-observer.ts';
import { measurementScope, recordMeasurement, untilMeasurementCondition } from './slice6-measurement-support.ts';

type ControlWindow = Window & {
  ovcDevelopmentReadControl?: { readonly abort: () => void; readonly done: Promise<number | string> };
};
type NavigationControl =
  | 'navigation_positive'
  | 'navigation_wrong_destination'
  | 'navigation_wrong_query'
  | 'navigation_repeat'
  | 'navigation_extra_navigation'
  | 'navigation_missing_second'
  | 'navigation_failed_second'
  | 'navigation_late_second';
type ControlCase =
  | 'outside_window'
  | 'repeated'
  | 'missing_replacement'
  | 'failed_replacement'
  | 'late_replacement'
  | 'http_error'
  | 'different_query'
  | 'different_target'
  | 'document_replacement'
  | 'account_replacement';

type ControlStage =
  | 'browser_start'
  | 'context_start'
  | 'policy_observer'
  | 'page_start'
  | 'native_login'
  | 'owner_setup'
  | 'owner_lookup'
  | 'access_token'
  | 'owner_request'
  | 'owner_settlement'
  | 'outside_window_completion'
  | 'hold_request'
  | 'start_native_fetch'
  | 'wait_request_held'
  | 'abort_native_fetch'
  | 'continue_held_request'
  | 'observe_request_failure'
  | 'replacement_response'
  | 'replacement_deadline'
  | 'document_reload'
  | 'account_login'
  | 'account_identity'
  | 'scope_mutation'
  | 'http_response'
  | 'http_response_completion'
  | 'http_console_expectation'
  | 'initial_navigation'
  | 'switch_link'
  | 'spa_switch'
  | 'history_completion'
  | 'expected_rejection'
  | 'expected_kind'
  | 'console_verification'
  | 'complete'
  | 'cleanup';
interface ControlProgress {
  stage: ControlStage;
  cancellationAttempt: number;
  armed: boolean;
  bodyComplete: boolean;
  reads: ReturnType<typeof observeDevelopmentReads> | null;
  observer: ReturnType<typeof observeBrowserContext> | null;
}

function controlProgress(): ControlProgress {
  return {
    stage: 'browser_start',
    cancellationAttempt: 0,
    armed: false,
    bodyComplete: false,
    reads: null,
    observer: null,
  };
}

function reportProgress(
  control: ControlCase | 'ordinary_spa_switch',
  progress: ControlProgress,
  completed: boolean,
  phase: 'before_cleanup' | 'after_cleanup',
): void {
  const reads = progress.armed ? progress.reads : null;
  recordMeasurement({
    event: 'browser.development_negative_control_stage',
    phase,
    control,
    stage: progress.bodyComplete && !completed ? 'cleanup' : progress.stage,
    completed,
    bodyComplete: progress.bodyComplete,
    armed: progress.armed,
    cancellationAttempt: progress.cancellationAttempt,
    rawRequestFailures: reads?.rawFailures() ?? 0,
    expectedInitialCancellations: reads?.expectedFailures() ?? 0,
    unexpectedFailures: reads?.unexpectedFailures() ?? 0,
    consoleWarnings: progress.observer?.warnings() ?? 0,
    consoleErrors: progress.observer?.errors() ?? 0,
  });
  reads?.report('chromium', control);
}

async function login(page: Page, account: TestAccount, bootstrapDocument = true): Promise<void> {
  if (bootstrapDocument) {
    const bootstrap = await page.goto('/api/v1/health/live');
    assert.equal(bootstrap?.status(), 200);
  }
  const status = await page.evaluate(
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
  assert.equal(status, 200);
}

async function get(page: Page, url: string, token: string): Promise<number> {
  return page.evaluate(
    async ({ url, token }) => {
      const response = await fetch(url, { headers: { Authorization: `Bearer ${token}` } });
      await response.arrayBuffer();
      return response.status;
    },
    { url, token },
  );
}

async function expectResource404(
  page: Page,
  url: string,
  token: string,
  observer: ReturnType<typeof observeBrowserContext>,
  progress: ControlProgress,
  control: 'failed_replacement' | 'http_error' | 'navigation_failed_second',
  trigger?: () => Promise<number>,
): Promise<void> {
  let httpExpectationVerified = false;
  const remainingExpected = (): number => observer.expect(page).expected.length;
  assert.equal(remainingExpected(), 0);
  const observed: {
    response: Response | null;
    responses: number;
    expected: string | null;
    exactErrors: number;
    emptyPhraseErrors: number;
    notFoundPhraseErrors: number;
    otherErrors: number;
    statusText: 'unobserved' | 'empty' | 'not_found' | 'other';
    complete: boolean;
  } = {
    response: null,
    responses: 0,
    expected: null,
    exactErrors: 0,
    emptyPhraseErrors: 0,
    notFoundPhraseErrors: 0,
    otherErrors: 0,
    statusText: 'unobserved',
    complete: false,
  };
  const prefix = 'Failed to load resource: the server responded with a status of 404';
  const onResponse = (response: Response): void => {
    if (response.request().method() !== 'GET' || response.url() !== url || response.status() !== 404) return;
    observed.responses += 1;
    if (observed.responses !== 1) return;
    const phrase = response.statusText();
    observed.statusText = phrase === '' ? 'empty' : phrase === 'Not Found' ? 'not_found' : 'other';
    observed.expected = `${prefix} (${phrase})`;
    observed.response = response;
  };
  const onConsole = (message: ConsoleMessage): void => {
    if (message.type() !== 'error') return;
    const text = message.text();
    if (text === observed.expected) observed.exactErrors += 1;
    if (text === `${prefix} ()`) observed.emptyPhraseErrors += 1;
    else if (text === `${prefix} (Not Found)`) observed.notFoundPhraseErrors += 1;
    else observed.otherErrors += 1;
  };
  page.on('response', onResponse);
  page.on('console', onConsole);
  try {
    progress.stage = 'http_response';
    assert.equal(await (trigger === undefined ? get(page, url, token) : trigger()), 404);
    progress.stage = 'http_response_completion';
    assert.equal(observed.responses, 1);
    if (observed.response === null) throw new Error('control_expected_404_response_missing');
    assert.equal((await observed.response.finished()) === null, true);
    observed.complete = true;
    progress.stage = 'http_console_expectation';
    await untilMeasurementCondition(
      () => observed.exactErrors > 0 || observer.errors() > 0 || observer.warnings() > 0,
      developmentReadinessMilliseconds,
    );
    assert.equal(observed.exactErrors, 1);
    assert.equal(remainingExpected(), 0);
    assert.equal(observer.errors(), 0);
    assert.equal(observer.warnings(), 0);
    await observer.finishHttp(page);
    assert.equal(observer.errors(), 0);
    assert.equal(observer.warnings(), 0);
    httpExpectationVerified = true;
  } finally {
    page.off('response', onResponse);
    page.off('console', onConsole);
    recordMeasurement({
      event: 'browser.development_404_console',
      control,
      statusTextVariant: observed.statusText,
      responses: observed.responses,
      responseComplete: observed.complete,
      expectedErrorConsumed: httpExpectationVerified && observed.exactErrors === 1 && remainingExpected() === 0,
      exactErrors: observed.exactErrors,
      emptyPhraseErrors: observed.emptyPhraseErrors,
      notFoundPhraseErrors: observed.notFoundPhraseErrors,
      otherErrors: observed.otherErrors,
    });
  }
}

async function hold(
  page: Page,
  url: string,
): Promise<{
  readonly count: () => number;
  readonly proveNativeAbort: () => void;
  readonly cancelledRequests: () => number;
  readonly release: () => Promise<void>;
}> {
  let resume = (): void => {
    throw new Error('control_barrier_not_initialized');
  };
  const barrier = new Promise<void>((resolve) => {
    resume = resolve;
  });
  let count = 0;
  let nativeAbortProved = false;
  const ownedRequests: import('@playwright/test').Request[] = [];
  const running = new Set<Promise<void>>();
  const failures: ContinuationErrorKind[] = [];
  const handler = (route: Route): Promise<void> => {
    count += 1;
    const request = route.request();
    const owned = request.method() === 'GET' && request.url() === url;
    if (owned) ownedRequests.push(request);
    const report = (outcome: 'continued' | 'rejected', kind: ContinuationErrorKind | 'none'): void => {
      recordMeasurement({
        event: 'browser.development_continuation',
        outcome,
        kind,
        ownedRequest: owned,
        nativeAbortProved,
        explicitCancellation: request.failure()?.errorText === 'net::ERR_ABORTED',
        pageOpen: !page.isClosed(),
        contextOpen: page.context().pages().includes(page),
        browserConnected: page.context().browser()?.isConnected() === true,
      });
    };
    const operation = (async (): Promise<void> => {
      await barrier;
      try {
        await route.continue();
        report('continued', 'none');
        if (!owned) failures.push('unknown');
      } catch (error) {
        const kind = continuationErrorKind(error);
        failures.push(kind);
        report('rejected', kind);
      }
    })();
    const completion = operation
      .catch(() => {
        failures.push('unknown');
      })
      .finally(() => {
        running.delete(completion);
      });
    running.add(completion);
    return completion;
  };
  await page.route(url, handler);
  let released: Promise<void> | null = null;
  return {
    count: (): number => count,
    proveNativeAbort: (): void => {
      nativeAbortProved = true;
    },
    cancelledRequests: (): number =>
      ownedRequests.filter((request) => request.failure()?.errorText === 'net::ERR_ABORTED').length,
    release: (): Promise<void> => {
      released ??= (async (): Promise<void> => {
        resume();
        try {
          await Promise.all([...running]);
        } finally {
          try {
            await page.unroute(url, handler);
          } finally {
            await Promise.all([...running]);
          }
        }
        if (failures.length !== 0) throw new Error('control_request_continuation_failed');
      })();
      return released;
    },
  };
}

async function cancel(
  page: Page,
  url: string,
  token: string,
  rawFailures: () => number,
  progress: ControlProgress,
): Promise<'cancelled_request_continued'> {
  progress.cancellationAttempt += 1;
  progress.stage = 'hold_request';
  const held = await hold(page, url);
  const before = rawFailures();
  try {
    progress.stage = 'start_native_fetch';
    await page.evaluate(
      ({ url, token }) => {
        const controller = new AbortController();
        const done = fetch(url, { headers: { Authorization: `Bearer ${token}` }, signal: controller.signal })
          .then(async (response) => {
            await response.arrayBuffer();
            return response.status;
          })
          .catch((error: unknown) => (error instanceof Error ? error.name : 'unknown_error'));
        const target = window as ControlWindow;
        target.ovcDevelopmentReadControl = {
          abort: (): void => {
            controller.abort();
          },
          done,
        };
      },
      { url, token },
    );
    progress.stage = 'wait_request_held';
    await untilMeasurementCondition(() => held.count() > 0, developmentReadinessMilliseconds);
    progress.stage = 'abort_native_fetch';
    const result = await page.evaluate(async () => {
      const control = (window as ControlWindow).ovcDevelopmentReadControl;
      if (control === undefined) throw new Error('control_request_missing');
      control.abort();
      return control.done;
    });
    assert.equal(result, 'AbortError');
    held.proveNativeAbort();
    progress.stage = 'continue_held_request';
    await held.release();
    progress.stage = 'observe_request_failure';
    await untilMeasurementCondition(
      () => rawFailures() > before && held.cancelledRequests() === 1,
      developmentReadinessMilliseconds,
    );
    assert.equal(held.count(), 1);
    assert.equal(held.cancelledRequests(), 1);
    assert.equal(page.isClosed(), false);
    assert.equal(page.context().pages().includes(page), true);
    assert.equal(page.context().browser()?.isConnected(), true);
    const outcome = 'cancelled_request_continued';
    recordMeasurement({
      event: 'browser.development_cancelled_continuation',
      outcome,
      nativeAbortProved: true,
      explicitCancellation: true,
      ownedRequest: true,
      pageOpen: true,
      contextOpen: true,
      browserConnected: true,
    });
    return outcome;
  } finally {
    await held.release();
  }
}

interface Actors {
  readonly tutor: TestAccount;
  readonly student: TestAccount;
  readonly card: string;
  readonly group: string;
  readonly secondGroup: string;
  readonly channel: string;
  readonly secondChannel: string;
}

async function negativeCase(deployment: Deployment, actors: Actors, name: ControlCase): Promise<void> {
  const progress = controlProgress();
  let completed = false;
  try {
    await measurementScope(async (cleanup) => {
      try {
        const browser = await chromium.launch();
        cleanup(() => browser.close());
        progress.stage = 'context_start';
        const context = await browser.newContext({ ignoreHTTPSErrors: true, baseURL: deployment.origin });
        progress.stage = 'policy_observer';
        const observer = observeBrowserContext(context, 'chromium', 'initial_read_negative_control');
        cleanup(async (): Promise<void> => {
          await context.close();
          await observer.finish();
        });
        progress.observer = observer;
        await installPolicyObserver(context);
        progress.stage = 'page_start';
        const page = await context.newPage();
        const reads = observeDevelopmentReads(page, deployment.origin);
        progress.reads = reads;
        progress.stage = 'native_login';
        await login(page, actors.student);
        progress.stage = 'owner_lookup';
        const originalAccount = await accountId(deployment, actors.student);
        progress.stage = 'owner_setup';
        reads.arm(originalAccount, actors.channel);
        progress.armed = true;
        progress.stage = 'access_token';
        const token = await accessToken(deployment, actors.student);
        progress.stage = 'owner_request';
        assert.equal(await get(page, '/api/v1/users/me', token), 200);
        progress.stage = 'owner_settlement';
        await reads.waitSettled();
        const current = `${deployment.origin}/api/v1/channels/${actors.channel}`;
        const list = `${deployment.origin}/api/v1/channels?q=`;
        try {
          if (name === 'outside_window') {
            progress.stage = 'outside_window_completion';
            await page.goto(`/messages/${actors.channel}`);
            await page.getByRole('textbox', { name: 'Message', exact: true }).waitFor();
            const history = page.getByRole('list', { name: 'Messages', exact: true });
            await history.waitFor();
            await history.getByText('Synthetic control history', { exact: true }).waitFor();
            await reads.waitInitial();
            reads.sealInitial();
          }
          if (name !== 'http_error') {
            const target =
              name === 'different_query'
                ? `${list}unexpected`
                : name === 'different_target'
                  ? `${deployment.origin}/api/v1/groups`
                  : current;
            assert.equal(await cancel(page, target, token, reads.rawFailures, progress), 'cancelled_request_continued');
          }
          if (
            [
              'repeated',
              'missing_replacement',
              'failed_replacement',
              'late_replacement',
              'document_replacement',
              'account_replacement',
            ].includes(name)
          ) {
            assert.equal(reads.expectedFailures(), 1);
            assert.equal(reads.unexpectedFailures(), 0);
          }
          if (name === 'repeated') {
            assert.equal(
              await cancel(page, current, token, reads.rawFailures, progress),
              'cancelled_request_continued',
            );
            progress.stage = 'replacement_response';
            assert.equal(await get(page, current, token), 200);
          }
          if (name === 'late_replacement') {
            progress.stage = 'hold_request';
            const held = await hold(page, current);
            cleanup(() => held.release());
            const pending = get(page, current, token).then(
              (status) => ({ kind: 'response', status }) as const,
              () => ({ kind: 'failed' }) as const,
            );
            cleanup(async (): Promise<void> => {
              await held.release();
              const completion = await pending;
              if (completion.kind !== 'response') throw new Error('control_late_replacement_fetch_failed');
            });
            await untilMeasurementCondition(() => held.count() === 1, developmentReadinessMilliseconds);
            progress.stage = 'replacement_deadline';
            await untilMeasurementCondition(reads.initialDeadlineExpired, developmentReadinessMilliseconds);
            await held.release();
            const completion = await pending;
            if (completion.kind !== 'response') throw new Error('control_late_replacement_fetch_failed');
            assert.equal(completion.status, 200);
            recordMeasurement({
              event: 'browser.development_late_replacement',
              deadlineExpired: true,
              responseComplete: true,
              status: completion.status,
            });
          }
          if (name === 'document_replacement') {
            progress.stage = 'document_reload';
            await page.reload();
            progress.stage = 'replacement_response';
            assert.equal(await get(page, current, token), 200);
          }
          if (name === 'account_replacement') {
            progress.stage = 'account_login';
            await login(page, actors.tutor, false);
            progress.stage = 'replacement_response';
            assert.equal(await get(page, current, token), 200);
            progress.stage = 'account_identity';
            const replacementId = await accountId(deployment, actors.tutor);
            const replacement = await page.evaluate(async (expectedId) => {
              const refreshed = await fetch('/api/v1/auth/refresh', { method: 'POST' });
              const token: unknown = await refreshed.json();
              if (
                typeof token !== 'object' ||
                token === null ||
                !('access_token' in token) ||
                typeof token.access_token !== 'string'
              )
                throw new Error('control_replacement_token_invalid');
              const me = await fetch('/api/v1/users/me', {
                headers: { Authorization: `Bearer ${token.access_token}` },
              });
              const body: unknown = await me.json();
              return {
                refreshStatus: refreshed.status,
                meStatus: me.status,
                matches: typeof body === 'object' && body !== null && 'id' in body && body.id === expectedId,
              };
            }, replacementId);
            assert.equal(replacement.refreshStatus, 200);
            assert.equal(replacement.meStatus, 200);
            assert.equal(replacement.matches, true);
            recordMeasurement({
              event: 'browser.development_account_replacement',
              refreshStatus: replacement.refreshStatus,
              meStatus: replacement.meStatus,
              replacementAccountVerified: replacement.matches,
            });
          }
          if (name === 'failed_replacement' || name === 'http_error') {
            progress.stage = 'scope_mutation';
            const tutorToken = await accessToken(deployment, actors.tutor);
            cleanup(async (): Promise<void> => {
              const restored = await callApi(deployment, tutorToken, 'PATCH', `/groups/${actors.group}`, {
                add: [actors.card],
              });
              assert.equal(restored.status, 200);
            });
            const removed = await callApi(deployment, tutorToken, 'PATCH', `/groups/${actors.group}`, {
              remove: [actors.card],
            });
            assert.equal(removed.status, 200);
            progress.stage = 'http_response';
            observer.expectHttp(page, { method: 'GET', address: current, status: 404 });
            await expectResource404(page, current, token, observer, progress, name);
            if (name === 'failed_replacement') {
              const restored = await callApi(deployment, tutorToken, 'PATCH', `/groups/${actors.group}`, {
                add: [actors.card],
              });
              assert.equal(restored.status, 200);
              assert.equal(await get(page, current, token), 200);
            }
          }
          await reads.finish();
          progress.stage = 'expected_rejection';
          await assert.rejects(async (): Promise<void> => {
            await reads.waitSettled();
          });
          progress.stage = 'expected_kind';
          const intended: Readonly<Record<ControlCase, string>> = {
            outside_window: 'cancellation_after_initial_seal',
            repeated: 'repeated_initial_cancellation',
            missing_replacement: 'unresolved_initial_replacement',
            failed_replacement: 'http_failure',
            late_replacement: 'replacement_completed_after_deadline',
            http_error: 'http_failure',
            different_query: 'unexpected_target_cancellation',
            different_target: 'unexpected_target_cancellation',
            document_replacement: 'document_generation_changed',
            account_replacement: 'account_boundary_changed',
          };
          assert.equal(
            reads.failureRecords().some((failure) => failure.kind === intended[name] && failure.count > 0),
            true,
          );
          if (name === 'account_replacement')
            assert.equal(
              reads.failureRecords().some((failure) => failure.kind === 'unresolved_initial_replacement'),
              false,
            );
          progress.stage = 'console_verification';
          observer.verify();
          assert.equal(observer.warnings(), 0);
          recordMeasurement({
            event: 'browser.development_negative_control',
            control: name,
            failureKind: intended[name],
            rejected: true,
            consoleWarnings: observer.warnings(),
            consoleErrors: observer.errors(),
          });
        } finally {
          await reads.finish();
          reads.report('chromium', name);
        }
        progress.bodyComplete = true;
        progress.stage = 'complete';
      } finally {
        reportProgress(name, progress, progress.bodyComplete, 'before_cleanup');
      }
    });
    completed = true;
  } finally {
    if (progress.reads !== null) await progress.reads.finish();
    reportProgress(name, progress, completed, 'after_cleanup');
  }
}

async function spaControl(deployment: Deployment, actors: Actors): Promise<void> {
  const progress = controlProgress();
  let completed = false;
  try {
    await measurementScope(async (cleanup) => {
      try {
        const browser = await chromium.launch();
        cleanup(() => browser.close());
        progress.stage = 'context_start';
        const context = await browser.newContext({ ignoreHTTPSErrors: true, baseURL: deployment.origin });
        progress.stage = 'policy_observer';
        const observer = observeBrowserContext(context, 'chromium', 'spa_cancellation_control');
        cleanup(async (): Promise<void> => {
          await context.close();
          await observer.finish();
        });
        progress.observer = observer;
        await installPolicyObserver(context);
        progress.stage = 'page_start';
        const page = await context.newPage();
        const reads = observeDevelopmentReads(page, deployment.origin);
        progress.reads = reads;
        progress.stage = 'native_login';
        await login(page, actors.tutor);
        const held = await hold(page, `${deployment.origin}/api/v1/channels/${actors.channel}`);
        cleanup(() => held.release());
        progress.stage = 'owner_lookup';
        const originalAccount = await accountId(deployment, actors.tutor);
        progress.stage = 'owner_setup';
        reads.arm(originalAccount, actors.channel);
        try {
          progress.armed = true;
          progress.stage = 'initial_navigation';
          await page.goto(`/messages/${actors.channel}`);
          progress.stage = 'switch_link';
          const second = page.locator(`a[href="/messages/${actors.secondChannel}"]`).first();
          await second.waitFor();
          await untilMeasurementCondition(() => held.count() > 0, developmentReadinessMilliseconds);
          const before = reads.rawFailures();
          progress.stage = 'spa_switch';
          await second.click();
          await untilMeasurementCondition(() => reads.rawFailures() > before, developmentReadinessMilliseconds);
          await held.release();
          progress.stage = 'history_completion';
          await page.getByRole('textbox', { name: 'Message', exact: true }).waitFor();
          await page
            .getByRole('list', { name: 'Messages', exact: true })
            .getByText('Synthetic control history', { exact: true })
            .waitFor();
          progress.stage = 'expected_rejection';
          await assert.rejects(async (): Promise<void> => {
            await reads.waitSettled();
          });
          assert.equal(
            reads.failureRecords().some((failure) => failure.kind === 'spa_scope_changed' && failure.count > 0),
            true,
          );
          progress.stage = 'console_verification';
          observer.verify();
          recordMeasurement({
            event: 'browser.development_negative_control',
            failureKind: 'spa_scope_changed',
            control: 'ordinary_spa_switch',
            rejected: true,
            consoleWarnings: observer.warnings(),
            consoleErrors: observer.errors(),
          });
        } finally {
          await reads.finish();
          reads.report('chromium', 'ordinary_spa_switch');
        }
        progress.bodyComplete = true;
        progress.stage = 'complete';
      } finally {
        reportProgress('ordinary_spa_switch', progress, progress.bodyComplete, 'before_cleanup');
      }
    });
    completed = true;
  } finally {
    if (progress.reads !== null) await progress.reads.finish();
    reportProgress('ordinary_spa_switch', progress, completed, 'after_cleanup');
  }
}

async function navigationControl(deployment: Deployment, actors: Actors, name: NavigationControl): Promise<void> {
  await measurementScope(async (cleanup) => {
    const browser = await chromium.launch();
    cleanup(() => browser.close());
    const context = await browser.newContext({ ignoreHTTPSErrors: true, baseURL: deployment.origin });
    const observer = observeBrowserContext(context, 'chromium', 'navigation_control');
    cleanup(async (): Promise<void> => {
      await context.close();
      await observer.finish();
    });
    await installPolicyObserver(context);
    const page = await context.newPage();
    const reads = observeDevelopmentReads(page, deployment.origin);
    await login(page, actors.student);
    reads.arm(await accountId(deployment, actors.student), actors.channel);
    await page.goto(`/messages/${actors.channel}`);
    await page.getByRole('textbox', { name: 'Message', exact: true }).waitFor();
    await page
      .getByRole('list', { name: 'Messages', exact: true })
      .getByText('Synthetic control history', { exact: true })
      .waitFor();
    await reads.waitInitial();
    reads.sealInitial();
    const before = reads.mark();
    reads.armNavigation(
      actors.channel,
      name === 'navigation_wrong_destination' ? actors.channel : actors.secondChannel,
    );
    const destination = `${deployment.origin}/api/v1/channels/${actors.secondChannel}`;
    const token = await accessToken(deployment, actors.student);
    let release = (): void => undefined;
    let heldSecond = false;
    let routeCount = 0;
    const operations = new Set<Promise<void>>();
    const continuationFailures: string[] = [];
    const barrier = new Promise<void>((resolve) => {
      release = resolve;
    });
    const holding = ['navigation_missing_second', 'navigation_failed_second', 'navigation_late_second'].includes(name);
    if (holding) {
      const handler = (route: Route): Promise<void> => {
        routeCount += 1;
        const ordinal = routeCount;
        const operation = (async (): Promise<void> => {
          if (ordinal === 2) {
            heldSecond = true;
            await barrier;
          }
          await route.continue();
        })();
        const tracked = operation
          .catch(() => {
            continuationFailures.push('continuation_failed');
          })
          .finally(() => {
            operations.delete(tracked);
          });
        operations.add(tracked);
        return tracked;
      };
      await page.route(destination, handler);
      cleanup(async (): Promise<void> => {
        release();
        await Promise.all([...operations]);
        await page.unroute(destination, handler);
        if (continuationFailures.length !== 0) throw new Error('navigation_control_continuation_failed');
      });
    }
    try {
      if (name === 'navigation_failed_second')
        observer.expectHttp(page, { method: 'GET', address: destination, status: 404 });
      await page.locator(`a[href="/messages/${actors.secondChannel}"]`).first().click();
      if (holding) {
        await untilMeasurementCondition(
          () => heldSecond && reads.navigationExpected() === 1,
          developmentReadinessMilliseconds,
        );
        assert.equal(reads.unexpectedFailures(), 0);
        if (name === 'navigation_missing_second') {
          assert.throws((): void => {
            reads.verify();
          });
          assert.equal(
            reads.failureRecords().some((entry) => entry.kind === 'unresolved_initial_replacement'),
            true,
          );
          release();
        } else if (name === 'navigation_late_second') {
          await untilMeasurementCondition(() => reads.navigationDeadlineExpired(), developmentReadinessMilliseconds);
          release();
        } else {
          const tutorToken = await accessToken(deployment, actors.tutor);
          cleanup(async (): Promise<void> => {
            const restored = await callApi(deployment, tutorToken, 'PATCH', `/groups/${actors.secondGroup}`, {
              add: [actors.card],
            });
            assert.equal(restored.status, 200);
          });
          const removed = await callApi(deployment, tutorToken, 'PATCH', `/groups/${actors.secondGroup}`, {
            remove: [actors.card],
          });
          assert.equal(removed.status, 200);
          await expectResource404(
            page,
            destination,
            token,
            observer,
            controlProgress(),
            'navigation_failed_second',
            async (): Promise<number> => {
              const responsePromise = page.waitForResponse(
                (response) => response.url() === destination && response.status() === 404,
              );
              release();
              const response = await responsePromise;
              assert.equal((await response.finished()) === null, true);
              return response.status();
            },
          );
          const restored = await callApi(deployment, tutorToken, 'PATCH', `/groups/${actors.secondGroup}`, {
            add: [actors.card],
          });
          assert.equal(restored.status, 200);
          assert.equal(await get(page, destination, token), 200);
          assert.equal(
            reads
              .failureRecords()
              .some((entry) => entry.kind === 'unresolved_initial_replacement' && entry.count === 1),
            true,
          );
        }
      }
      if (name !== 'navigation_failed_second') {
        await page.getByRole('textbox', { name: 'Message', exact: true }).waitFor();
        await page
          .getByRole('list', { name: 'Messages', exact: true })
          .getByText('Synthetic control history', { exact: true })
          .waitFor();
      }
      if (name === 'navigation_repeat' || name === 'navigation_extra_navigation')
        await reads.waitChannel(actors.secondChannel, before);
      if (name === 'navigation_positive' || name === 'navigation_missing_second') {
        await reads.waitChannel(actors.secondChannel, before);
        reads.sealNavigation();
        reads.verify();
        assert.equal(reads.navigationExpected(), 1);
        assert.equal(reads.rawFailures(), reads.expectedFailures() + 1);
        assert.equal(reads.unexpectedFailures(), 0);
      } else {
        if (name === 'navigation_wrong_query' || name === 'navigation_repeat')
          assert.equal(
            await cancel(
              page,
              name === 'navigation_wrong_query' ? `${destination}?unexpected=1` : destination,
              token,
              reads.rawFailures,
              controlProgress(),
            ),
            'cancelled_request_continued',
          );
        if (name === 'navigation_extra_navigation')
          await page.locator(`a[href="/messages/${actors.channel}"]`).first().click();
        await reads.finish();
        assert.throws((): void => {
          reads.verify();
        });
        const expected =
          name === 'navigation_wrong_destination'
            ? 'spa_scope_changed'
            : name === 'navigation_extra_navigation'
              ? 'navigation_boundary_invalidated'
              : name === 'navigation_wrong_query'
                ? 'unexpected_target_cancellation'
                : name === 'navigation_repeat'
                  ? 'repeated_initial_cancellation'
                  : name === 'navigation_late_second'
                    ? 'replacement_completed_after_deadline'
                    : 'http_failure';
        assert.equal(
          reads.failureRecords().some((entry) => entry.kind === expected),
          true,
        );
      }
      observer.verify();
      recordMeasurement({
        event: 'browser.development_navigation_control',
        control: name,
        passed: true,
        rawRequestFailures: reads.rawFailures(),
        expectedInitialCancellations: reads.expectedFailures(),
        expectedNavigationCancellations: reads.navigationExpected(),
      });
    } finally {
      release();
      await reads.finish();
      reads.report('chromium', name);
    }
  });
}

function selectedControls(
  args: readonly string[],
): ReadonlySet<ControlCase | 'ordinary_spa_switch' | NavigationControl> {
  const available = [
    'outside_window',
    'repeated',
    'missing_replacement',
    'failed_replacement',
    'late_replacement',
    'http_error',
    'different_query',
    'different_target',
    'document_replacement',
    'account_replacement',
    'ordinary_spa_switch',
    'navigation_positive',
    'navigation_wrong_destination',
    'navigation_wrong_query',
    'navigation_repeat',
    'navigation_extra_navigation',
    'navigation_missing_second',
    'navigation_failed_second',
    'navigation_late_second',
  ] as const;
  if (args.length === 0) return new Set(available);
  const selector = args[0];
  if (args.length !== 1 || !selector?.startsWith('--control=')) throw new Error('development_control_selector_invalid');
  const requested = selector.slice('--control='.length);
  const selected = available.find((name) => name === requested);
  if (selected === undefined) throw new Error('development_control_selector_invalid');
  return new Set([selected]);
}

async function run(): Promise<void> {
  const selected = selectedControls(process.argv.slice(2));
  takeRunLock(process.env);
  try {
    prepareRun('measurement-dev-tls');
    await measurementScope(async (cleanup) => {
      const deployment = await startDeployment(
        readE2ESettings(process.env),
        0,
        'development-read-controls',
        undefined,
        'measurement-dev-tls',
      );
      cleanup(() => deployment.stop());
      const tutor = await seedAccount(deployment, 'Synthetic control tutor', 'tutor', 'en');
      const member = await seedActiveStudent(deployment, tutor, 'Synthetic control student', 'en');
      const groups = await Promise.all(
        ['First control group', 'Second control group'].map((name) =>
          createGroup(deployment, tutor, name, [member.cardId]),
        ),
      );
      const channels = await Promise.all(groups.map((group) => channelOf(deployment, tutor, `/groups/${group}`)));
      const group = groups[0];
      const channel = channels[0];
      const secondChannel = channels[1];
      if (group === undefined || channel === undefined || secondChannel === undefined)
        throw new Error('control_seed_missing');
      const token = await accessToken(deployment, tutor);
      for (const id of channels)
        assert.equal(
          (
            await callApi(deployment, token, 'POST', `/channels/${id}/messages`, {
              body: 'Synthetic control history',
              attachment_ids: [],
            })
          ).status,
          201,
        );
      const secondGroup = groups[1];
      if (secondGroup === undefined) throw new Error('control_second_group_missing');
      const actors: Actors = {
        tutor,
        student: member.student,
        card: member.cardId,
        group,
        secondGroup,
        channel,
        secondChannel,
      };
      let failed = 0;
      for (const name of [
        'outside_window',
        'repeated',
        'missing_replacement',
        'failed_replacement',
        'late_replacement',
        'http_error',
        'different_query',
        'different_target',
        'document_replacement',
        'account_replacement',
      ] as const) {
        if (!selected.has(name)) continue;
        try {
          await negativeCase(deployment, actors, name);
        } catch {
          failed += 1;
          recordMeasurement({
            event: 'browser.development_negative_control_failed',
            control: name,
            kind: 'control_failed',
          });
        }
      }
      if (selected.has('ordinary_spa_switch')) {
        try {
          await spaControl(deployment, actors);
        } catch {
          failed += 1;
          recordMeasurement({
            event: 'browser.development_negative_control_failed',
            control: 'ordinary_spa_switch',
            kind: 'control_failed',
          });
        }
      }
      for (const name of [
        'navigation_positive',
        'navigation_wrong_destination',
        'navigation_wrong_query',
        'navigation_repeat',
        'navigation_extra_navigation',
        'navigation_missing_second',
        'navigation_failed_second',
        'navigation_late_second',
      ] as const) {
        if (!selected.has(name)) continue;
        try {
          await navigationControl(deployment, actors, name);
        } catch {
          failed += 1;
          recordMeasurement({
            event: 'browser.development_negative_control_failed',
            control: name,
            kind: 'control_failed',
          });
        }
      }
      assert.equal(failed, 0);
    });
  } finally {
    finishRun();
  }
}

try {
  await run();
} catch {
  recordMeasurement({ event: 'browser.development_read_controls_failed', kind: 'verification_failed' });
  process.exitCode = 1;
}
