import { ownFailurePosition } from '../e2e/diagnostic-expectations.ts';
import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';

import {
  chromium,
  expect,
  firefox,
  webkit,
  type BrowserType,
  type Browser,
  type BrowserContext,
  type Page,
  type Request,
  type Route,
  type WebSocket,
} from '@playwright/test';

import { seedAccount, signInThroughTheApi, type TestAccount } from '../e2e/accounts.ts';
import { installPolicyObserver, observeBrowserContext } from '../e2e/browser-diagnostics.ts';
import {
  finishRun,
  prepareRun,
  readE2ESettings,
  startDeployment,
  takeRunLock,
  type Deployment,
} from '../e2e/deployment.ts';
import { todayQueryPaths, trafficAfterReady, type TrafficAfterReady } from '../e2e/realtime.ts';
import {
  unexpectedFailures,
  cancellationAfterNavigationAction,
  type FailedRequest,
  type PageActivity,
} from '../e2e/request-failures.ts';
import { measurementScope, recordMeasurement } from './slice6-measurement-support.ts';

const notificationsPath = '/api/v1/notifications';
const calendarPath = '/api/v1/calendar';
const pageEvents = [
  'request',
  'response',
  'requestfinished',
  'requestfailed',
  'framenavigated',
  'frameattached',
  'websocket',
  'close',
] as const;
const socketEvents = ['framereceived', 'close'] as const;

type Control = 'hard_document' | 'spa_current_pending' | 'observer_disposal' | 'lifecycle';
type Stage =
  | 'old_document_navigation'
  | 'arm_main_navigation'
  | 'begin_main_navigation'
  | 'release_old_before_commit'
  | 'release_main_navigation'
  | 'current_document_ready'
  | 'arm_current_query'
  | 'spa_navigation'
  | 'held_current_poll'
  | 'release_current_query'
  | 'observer_attach'
  | 'observer_navigation'
  | 'observer_dispose'
  | 'deployment_start'
  | 'browser_start'
  | 'context_start'
  | 'seed_account'
  | 'sign_in'
  | 'context_close'
  | 'diagnostics_finish'
  | 'deployment_stop'
  | 'deployment_stopped'
  | 'browser_close'
  | 'browser_closed';

const currentStages = new Map<string, Stage>();

function stage(engine: string, control: Control, current: Stage): void {
  currentStages.set(engine, current);
  recordMeasurement({ event: 'browser.navigation_readiness_control_stage', engine, control, stage: current });
}

type FailureKind =
  | 'aggregate'
  | 'assertion'
  | 'timeout'
  | 'native_abort'
  | 'already_handled'
  | 'hold_join'
  | 'goto'
  | 'context_close'
  | 'diagnostics'
  | 'unknown';

function failureKind(error: unknown): FailureKind {
  if (error instanceof AggregateError) return 'aggregate';
  if (!(error instanceof Error)) return 'unknown';
  if (error.name === 'AssertionError') return 'assertion';
  if (error.name === 'TimeoutError') return 'timeout';
  if (error.message.includes('ERR_ABORTED')) return 'native_abort';
  if (error.message.includes('Route is already handled')) return 'already_handled';
  if (error.message === 'navigation_control_hold_join_failed') return 'hold_join';
  if (error.message === 'navigation_control_goto_failed') return 'goto';
  if (error.message === 'navigation_control_context_close_failed') return 'context_close';
  if (error.message === 'navigation_control_diagnostics_failed') return 'diagnostics';
  return 'unknown';
}

function recordFailureDetails(error: unknown): void {
  const pending: unknown[] = [error];
  const seen = new Set<object>();
  let examined = 0;
  while (pending.length > 0 && examined < 32) {
    const current = pending.shift();
    if (current !== null && typeof current === 'object') {
      if (seen.has(current)) continue;
      seen.add(current);
    }
    recordMeasurement({
      event: 'browser.navigation_readiness_failure_detail',
      kind: failureKind(current),
      ...ownFailurePosition(current, new URL(import.meta.url).pathname),
      index: examined,
    });
    examined += 1;
    if (current instanceof AggregateError) {
      const nested: readonly unknown[] = current.errors;
      for (const problem of nested.slice(0, 32 - examined)) if (pending.length < 32) pending.push(problem);
    }
  }
}

function observeFailureProvenance(context: BrowserContext, engine: string, origin: string): () => void {
  const assets = readdirSync(new URL('../dist/assets/', import.meta.url))
    .filter((name) => name.endsWith('.js'))
    .sort()
    .map((name) => new URL(`/assets/${name}`, origin).href);
  const socketAddress = new URL('/api/v1/ws', origin);
  socketAddress.protocol = socketAddress.protocol === 'https:' ? 'wss:' : 'ws:';
  let sequence = 0;
  const record = (fields: Readonly<Record<string, string | number | boolean>>): void => {
    sequence += 1;
    if (sequence <= 128)
      recordMeasurement({
        event: 'browser.navigation_readiness_failure_provenance',
        engine,
        stage: currentStages.get(engine) ?? 'unknown',
        sequence,
        ...fields,
      });
  };
  const nativeError = (error: import('@playwright/test').WebError): void => {
    const value = error.error();
    const text = `${value.name}: ${value.message}`;
    const standardNames = [
      'Error',
      'TypeError',
      'ReferenceError',
      'SyntaxError',
      'AbortError',
      'NetworkError',
      'InvalidStateError',
      'DOMException',
    ];
    const name = standardNames.includes(value.name)
      ? value.name
      : value.name === ''
        ? 'empty'
        : value.name.startsWith('WebSocket connection')
          ? 'websocket_connection'
          : value.name.startsWith('Unhandled Promise Rejection')
            ? 'unhandled_promise_rejection'
            : 'unknown';
    const location = error.location();
    const asset = assets.indexOf(location.url);
    const parsed = URL.canParse(location.url) ? new URL(location.url) : null;
    record({
      producer: 'native_weberror',
      name,
      cancellation: /Load request cancelled|cancelled|The operation was aborted|AbortError/.test(text),
      socketClosure:
        /closed before the connection is established|Socket is not connected|WebSocket is closed|WebSocket connection/.test(
          text,
        ),
      networkFailure: /Load failed|NetworkError|Failed to fetch|network connection was lost/.test(text),
      moduleFailure: /Importing a module script failed|dynamically imported module/.test(text),
      illegalInvocation: text.includes('Illegal invocation'),
      ownSocketAddress: text.includes(socketAddress.href),
      location:
        asset >= 0
          ? 'own_script'
          : parsed?.origin === origin
            ? 'own_other'
            : parsed !== null
              ? 'other_origin'
              : location.url === ''
                ? 'absent'
                : 'malformed',
      asset,
      line: asset >= 0 ? location.line : -1,
      column: asset >= 0 ? location.column : -1,
      stackHasFrames: /\n\s*at /.test(value.stack ?? ''),
      pagePresent: error.page() !== null,
    });
    const stack = value.stack ?? '';
    for (const [index, address] of assets.entries()) {
      const offset = stack.indexOf(`${address}:`);
      if (offset < 0) continue;
      const coordinates = /^(\d+):(\d+)/.exec(stack.slice(offset + address.length + 1));
      if (coordinates === null) continue;
      record({
        producer: 'own_script_stack',
        asset: index,
        line: Number(coordinates[1]),
        column: Number(coordinates[2]),
      });
    }
  };
  const requestFrame = (request: Request): void => {
    try {
      request.frame().page();
    } catch (error: unknown) {
      const kind =
        error instanceof Error && error.message.startsWith('Frame for this navigation request is not available')
          ? 'navigation_frame_unavailable'
          : error instanceof Error && error.message === 'Service Worker requests do not have an associated frame.'
            ? 'service_worker_no_frame'
            : 'unknown';
      record({ producer: 'request_frame_lookup', kind, navigation: request.isNavigationRequest() });
    }
  };
  const requested = (request: Request): void => {
    requestFrame(request);
    if (request.isNavigationRequest() && request.url() === new URL('/today', origin).href)
      record({ producer: 'today_navigation_request' });
  };
  const failed = (request: Request): void => {
    requestFrame(request);
    const address = new URL(request.url());
    if (address.origin === origin && address.pathname === notificationsPath)
      record({ producer: 'notifications_request_failed' });
  };
  const socketClosers = new Map<WebSocket, () => void>();
  const opened = (socket: WebSocket): void => {
    if (socket.url() !== socketAddress.href) return;
    record({ producer: 'own_socket_opened' });
    const closed = (): void => {
      record({ producer: 'own_socket_closed' });
    };
    socketClosers.set(socket, closed);
    socket.on('close', closed);
  };
  const pages = new Set<Page>();
  const pageOpened = (page: Page): void => {
    pages.add(page);
    page.on('websocket', opened);
  };
  context.on('page', pageOpened);
  for (const page of context.pages()) pageOpened(page);
  context.on('weberror', nativeError);
  context.on('request', requested);
  context.on('requestfailed', failed);
  return (): void => {
    context.off('page', pageOpened);
    for (const page of pages) page.off('websocket', opened);
    for (const [socket, closed] of socketClosers) socket.off('close', closed);
    context.off('weberror', nativeError);
    context.off('request', requested);
    context.off('requestfailed', failed);
    if (sequence > 128) throw new Error('navigation_control_provenance_overflow');
  };
}

interface PublicListenerSource {
  readonly listenerCount: (event: string) => number;
}

function hasPublicListenerCount(value: unknown): value is PublicListenerSource {
  return (
    typeof value === 'object' && value !== null && 'listenerCount' in value && typeof value.listenerCount === 'function'
  );
}

function listeners(value: unknown, events: readonly string[]): number[] {
  if (!hasPublicListenerCount(value)) throw new Error('navigation_control_public_listener_api_unavailable');
  return events.map((event) => {
    const count: unknown = value.listenerCount(event);
    if (typeof count !== 'number' || !Number.isSafeInteger(count) || count < 0)
      throw new Error('navigation_control_public_listener_count_invalid');
    return count;
  });
}

interface RequestHold {
  readonly request: () => Request | null;
  readonly terminal: () => boolean;
  readonly released: () => boolean;
  readonly retireForNavigation: () => void;
  readonly nativeRetired: () => boolean;
  readonly releaseAndJoin: () => Promise<void>;
}

async function holdOneGet(
  page: Page,
  origin: string,
  path: string,
  mainNavigation = false,
  observe?: (request: Request, phase: 'entered' | 'selected') => void,
): Promise<RequestHold> {
  let observed: Request | null = null;
  let terminal = false;
  let released = false;
  let failures = 0;
  let retiring = false;
  let nativeRetired = false;
  let releaseBarrier = (): void => undefined;
  const operations: Promise<void>[] = [];
  const barrier = new Promise<void>((resolve) => {
    releaseBarrier = resolve;
  });
  const matches = (url: URL): boolean => url.origin === origin && url.pathname === path;
  const handler = (route: Route): Promise<void> => {
    const operation = (async (): Promise<void> => {
      const candidate = route.request();
      observe?.(candidate, 'entered');
      const selected =
        observed === null &&
        candidate.method() === 'GET' &&
        (!mainNavigation || (candidate.isNavigationRequest() && candidate.frame() === page.mainFrame()));
      if (selected) {
        observed = candidate;
        observe?.(candidate, 'selected');
        await barrier;
      }
      if (selected && terminal) {
        await route.fallback();
        if (!retiring || candidate.failure() === null)
          throw new Error('navigation_control_unexpected_terminal_request');
        nativeRetired = true;
        return;
      }
      await route.continue();
    })();
    const joined = operation.then(
      () => undefined,
      (error: unknown) => {
        failures += 1;
        recordMeasurement({
          event: 'browser.navigation_readiness_hold_operation_failed',
          mainNavigation,
          kind: failureKind(error),
        });
      },
    );
    operations.push(joined);
    return joined;
  };
  const completed = (request: Request): void => {
    if (request === observed) terminal = true;
  };
  page.on('requestfinished', completed);
  page.on('requestfailed', completed);
  await page.route(matches, handler);
  return {
    request: () => observed,
    terminal: () => terminal,
    released: () => released,
    retireForNavigation: (): void => {
      retiring = true;
    },
    nativeRetired: () => nativeRetired,
    releaseAndJoin: async (): Promise<void> => {
      released = true;
      releaseBarrier();
      try {
        for (let before = 0; before < operations.length;) {
          const current = operations.slice(before);
          before = operations.length;
          await Promise.all(current);
        }
      } finally {
        page.off('requestfinished', completed);
        page.off('requestfailed', completed);
      }
      if (failures !== 0)
        throw new AggregateError(
          Array.from({ length: failures }, () => new Error('navigation_control_hold_operation_failed')),
          'navigation_control_hold_join_failed',
        );
    },
  };
}

async function releaseRouteHolds(page: Page, holds: readonly RequestHold[], engine: string): Promise<void> {
  const released = await Promise.allSettled(holds.map((hold) => hold.releaseAndJoin()));
  const problems = released
    .filter((outcome) => outcome.status === 'rejected')
    .map(() => new Error('navigation_control_route_release_failed'));
  try {
    await page.unrouteAll({ behavior: 'wait' });
  } catch {
    problems.push(new Error('navigation_control_route_removal_failed'));
  }
  const joined = await Promise.allSettled(holds.map((hold) => hold.releaseAndJoin()));
  for (const outcome of joined)
    if (outcome.status === 'rejected') problems.push(new Error('navigation_control_late_route_operation_failed'));
  if (problems.length !== 0) throw new AggregateError(problems, 'navigation_control_route_cleanup_failed');
  recordMeasurement({
    event: 'browser.navigation_readiness_control',
    engine,
    control: 'route_cleanup',
    handlersJoinedBeforeRemoval: true,
    holds: holds.length,
  });
}

function nativeActivity(page: Page): {
  readonly failureFor: (request: Request | null) => FailedRequest | undefined;
  readonly terminalFor: (request: Request | null) => 'pending' | 'finished' | 'failed';
  readonly navigationAction: () => number;
  readonly activity: () => PageActivity;
  readonly verify: () => void;
  readonly dispose: () => void;
} {
  let step = 0;
  let overflow = false;
  const navigations: number[] = [];
  const requests: number[] = [];
  const starts = new WeakMap<Request, number>();
  const answered = new WeakSet<Request>();
  const finished = new WeakSet<Request>();
  const failed = new Map<Request, FailedRequest>();
  const advance = (): boolean => {
    step += 1;
    if (step <= 1024) return true;
    overflow = true;
    return false;
  };
  const started = (request: Request): void => {
    if (!advance()) return;
    starts.set(request, step);
    if (request.isNavigationRequest() && request.frame() === page.mainFrame()) navigations.push(step);
    else requests.push(step);
  };
  const response = (value: import('@playwright/test').Response): void => {
    answered.add(value.request());
  };
  const failure = (request: Request): void => {
    if (!advance()) return;
    failed.set(request, {
      method: request.method(),
      description: request.method(),
      reason: request.failure()?.errorText ?? '',
      answered: answered.has(request),
      started: starts.get(request) ?? Number.POSITIVE_INFINITY,
      failed: step,
    });
  };
  const complete = (request: Request): void => {
    finished.add(request);
  };
  page.on('request', started);
  page.on('requestfinished', complete);
  page.on('response', response);
  page.on('requestfailed', failure);
  const activity = (): PageActivity => ({ documents: navigations, routeChanges: [], requests, closes: [] });
  return {
    failureFor: (request) => (request === null ? undefined : failed.get(request)),
    terminalFor: (request) =>
      request === null ? 'pending' : failed.has(request) ? 'failed' : finished.has(request) ? 'finished' : 'pending',
    navigationAction: (): number => {
      if (!advance()) throw new Error('navigation_activity_overflow');
      return step;
    },
    activity,
    verify: (): void => {
      assert.equal(overflow, false);
      assert.equal(unexpectedFailures([...failed.values()], activity()).length, 0);
    },
    dispose: (): void => {
      page.off('request', started);
      page.off('requestfinished', complete);
      page.off('response', response);
      page.off('requestfailed', failure);
      failed.clear();
    },
  };
}

async function ready(traffic: TrafficAfterReady): Promise<void> {
  await expect.poll(() => traffic.coherent()).toBe(true);
}

async function hardNavigationAndSpa(page: Page, origin: string, engine: string): Promise<void> {
  await measurementScope(async (cleanup) => {
    const activity = nativeActivity(page);
    cleanup((): Promise<void> => {
      activity.dispose();
      return Promise.resolve();
    });
    const holds: RequestHold[] = [];
    cleanup(() => releaseRouteHolds(page, holds, engine));
    const traffic = trafficAfterReady(page, todayQueryPaths);
    await traffic.initialize();
    cleanup(() => traffic.dispose());
    const old = await holdOneGet(page, origin, notificationsPath);
    holds.push(old);
    cleanup(() => old.releaseAndJoin());

    stage(engine, 'hard_document', 'old_document_navigation');
    await traffic.prepareNavigation();
    await page.goto('/today');
    await expect.poll(() => old.request() !== null).toBe(true);
    await expect.poll(() => traffic.readyReceived()).toBe(true);
    assert.equal(traffic.snapshot().pending > 0, true);
    const oldGeneration = traffic.snapshot().generation;
    assert.equal(old.released(), false);

    stage(engine, 'hard_document', 'arm_main_navigation');
    const navigation = await holdOneGet(page, origin, '/today', true);
    holds.push(navigation);
    let navigationOutcome: Promise<boolean> | null = null;
    cleanup(async () => {
      const results = await Promise.allSettled([old.releaseAndJoin(), navigation.releaseAndJoin()]);
      const completed = navigationOutcome === null || (await navigationOutcome);
      const failures = results
        .filter((result) => result.status === 'rejected')
        .map(() => new Error('navigation_control_main_navigation_hold_failed'));
      if (!completed) failures.push(new Error('navigation_control_goto_failed'));
      if (failures.length !== 0)
        throw new AggregateError(failures, 'navigation_control_main_navigation_cleanup_failed');
    });
    stage(engine, 'hard_document', 'begin_main_navigation');
    old.retireForNavigation();
    const navigationAction = activity.navigationAction();
    await traffic.prepareNavigation();
    navigationOutcome = page.goto('/today').then(
      () => true,
      (error: unknown) => {
        recordMeasurement({
          event: 'browser.navigation_readiness_navigation_failed',
          engine,
          kind: failureKind(error),
          phase: traffic.snapshot().phase,
          pending: traffic.snapshot().pending,
        });
        return false;
      },
    );
    await expect.poll(() => navigation.request() !== null || old.terminal()).toBe(true);
    if (navigation.request() === null) {
      assert.notEqual(activity.failureFor(old.request()), undefined);
      await old.releaseAndJoin();
    }
    await expect.poll(() => navigation.request() !== null).toBe(true);
    const oldHeldAtObservedMainRequest = !old.released();
    assert.equal(traffic.snapshot().generation, oldGeneration + 1);
    assert.equal(traffic.snapshot().phase, 'navigating');
    assert.equal(traffic.readyReceived(), false);
    assert.equal(traffic.snapshot().pending, 0);
    assert.equal(navigation.released(), false);
    const oldTerminalBeforeRelease = old.terminal();
    stage(engine, 'hard_document', 'release_old_before_commit');
    await old.releaseAndJoin();
    if (old.nativeRetired()) {
      const failure = activity.failureFor(old.request());
      assert.notEqual(failure, undefined);
      assert.equal(failure !== undefined && cancellationAfterNavigationAction(failure, navigationAction), true);
    }
    assert.equal(traffic.snapshot().phase, 'navigating');
    assert.equal(navigation.released(), false);
    stage(engine, 'hard_document', 'release_main_navigation');
    await navigation.releaseAndJoin();
    assert.equal(await navigationOutcome, true);
    stage(engine, 'hard_document', 'current_document_ready');
    await ready(traffic);
    assert.equal(traffic.snapshot().generation, oldGeneration + 1);
    recordMeasurement({
      event: 'browser.navigation_readiness_control',
      control: 'hard_document',
      engine,
      oldHeldWhenGotoStarted: true,
      oldHeldAtObservedMainRequest,
      oldRetiredBeforeCommit: true,
      oldNativeCancellation: old.nativeRetired(),
      oldTerminalBeforeRelease,
      currentReady: true,
      currentPending: traffic.snapshot().pending,
    });

    stage(engine, 'spa_current_pending', 'arm_current_query');
    const current = await holdOneGet(page, origin, calendarPath);
    holds.push(current);
    cleanup(() => current.releaseAndJoin());
    const generation = traffic.snapshot().generation;
    const document = await page.evaluateHandle(() => window.document);
    cleanup(() => document.dispose());

    stage(engine, 'spa_current_pending', 'spa_navigation');
    await page.getByRole('link', { name: 'Schedule', exact: true }).click();
    await expect.poll(() => current.request() !== null).toBe(true);
    await expect.poll(() => traffic.snapshot().phase).toBe('committed');
    assert.equal(await page.evaluate((previous) => previous === window.document, document), true);
    assert.equal(traffic.snapshot().generation, generation);
    assert.equal(traffic.readyReceived(), true);
    await expect.poll(() => traffic.unsettled()).toEqual(['pending_api_request']);
    assert.equal(traffic.snapshot().pending, 1);
    assert.equal(current.terminal(), false);
    stage(engine, 'spa_current_pending', 'held_current_poll');
    const negative = { rejected: false };
    try {
      await expect.poll(() => traffic.unsettled()).toEqual([]);
    } catch (error) {
      const stillPending =
        traffic.snapshot().generation === generation &&
        traffic.snapshot().phase === 'committed' &&
        traffic.snapshot().pending === 1 &&
        traffic.readyReceived() &&
        !current.released() &&
        !current.terminal() &&
        traffic.unsettled().length === 1 &&
        traffic.unsettled()[0] === 'pending_api_request';
      if (
        !(error instanceof Error) ||
        !error.message.includes('Timeout 5000ms exceeded while waiting on the predicate') ||
        !stillPending
      )
        throw new Error('navigation_control_current_hold_failed_for_unrelated_reason');
      negative.rejected = true;
    }
    assert.equal(negative.rejected, true);
    stage(engine, 'spa_current_pending', 'release_current_query');
    await current.releaseAndJoin();
    await ready(traffic);
    assert.equal(traffic.snapshot().generation, generation);
    recordMeasurement({
      event: 'browser.navigation_readiness_control',
      control: 'spa_current_pending',
      engine,
      sameDocument: true,
      sameGeneration: true,
      pendingWasNotWaived: true,
      originalPollRejected: negative.rejected,
      currentReady: true,
      currentPending: traffic.snapshot().pending,
    });
    activity.verify();
  });
}

async function repeatedObserverDisposal(page: Page, engine: string): Promise<void> {
  for (let cycle = 0; cycle < 3; cycle += 1) {
    stage(engine, 'observer_disposal', 'observer_attach');
    const baseline = listeners(page, pageEvents);
    const sockets: { readonly socket: WebSocket; readonly baseline: readonly number[] }[] = [];
    const listenerObservation = { failed: false };
    const opened = (socket: WebSocket): void => {
      if (sockets.length >= 4) {
        listenerObservation.failed = true;
        return;
      }
      try {
        sockets.push({ socket, baseline: listeners(socket, socketEvents) });
      } catch {
        listenerObservation.failed = true;
      }
    };
    page.on('websocket', opened);
    const traffic = trafficAfterReady(page, todayQueryPaths);
    await traffic.initialize();
    const failures: Error[] = [];
    try {
      stage(engine, 'observer_disposal', 'observer_navigation');
      await traffic.prepareNavigation();
      await page.goto('/today');
      await ready(traffic);
      assert.equal(listenerObservation.failed, false);
      const live = sockets.filter(({ socket }) => !socket.isClosed());
      assert.equal(live.length > 0, true);
      for (const watched of live) {
        const attached = listeners(watched.socket, socketEvents);
        assert.deepEqual(
          attached,
          watched.baseline.map((count) => count + 1),
        );
      }
    } catch {
      failures.push(new Error('navigation_control_observer_assertion_failed'));
    }
    stage(engine, 'observer_disposal', 'observer_dispose');
    const disposed = await Promise.allSettled([traffic.dispose(), traffic.dispose()]);
    page.off('websocket', opened);
    for (const result of disposed)
      if (result.status === 'rejected') failures.push(new Error('navigation_control_observer_disposal_failed'));
    if (failures.length !== 0) throw new AggregateError(failures, 'navigation_control_observer_cleanup_failed');
    assert.deepEqual(listeners(page, pageEvents), baseline);
    for (const watched of sockets) {
      const after = listeners(watched.socket, socketEvents);
      assert.equal(
        after.every((count, index) => count <= (watched.baseline[index] ?? -1)),
        true,
      );
    }
    recordMeasurement({
      event: 'browser.navigation_readiness_control',
      control: 'observer_disposal',
      engine,
      cycle,
      pageListenersRestored: true,
      socketListenersNotGrowing: true,
      socketsObserved: sockets.length,
    });
  }
}

type EntryModulePhase = 'page' | 'context' | 'entered' | 'selected';
type EntryModuleCounts = Record<EntryModulePhase | 'inexact' | 'unknown' | 'commits' | 'parsed', number>;
interface CaptureEvidence {
  waits: number;
  handles: number;
  owner: number;
  captured: number;
}

function observeEntryModule(
  page: Page,
  origin: string,
  path: string,
): {
  observe: (request: Request, phase: EntryModulePhase) => void;
  detach: () => void;
  snapshot: () => EntryModuleCounts & { overflow: boolean };
} {
  const counts = { page: 0, context: 0, entered: 0, selected: 0, inexact: 0, unknown: 0, commits: 0, parsed: 0 };
  let overflow = false;
  const increment = (key: keyof typeof counts): void => {
    if (counts[key] === 128) overflow = true;
    else counts[key] += 1;
  };
  const observe = (request: Request, phase: 'page' | 'context' | 'entered' | 'selected'): void => {
    const url = new URL(request.url());
    if (url.origin !== origin || url.pathname !== path) return;
    increment(phase);
    try {
      if (request.method() !== 'GET' || url.search !== '' || url.hash !== '' || request.frame() !== page.mainFrame())
        increment('inexact');
    } catch {
      increment('unknown');
    }
  };
  const requested = (request: Request): void => {
    observe(request, 'page');
  };
  const contextRequested = (request: Request): void => {
    observe(request, 'context');
  };
  const committed = (frame: import('@playwright/test').Frame): void => {
    if (frame === page.mainFrame()) increment('commits');
  };
  const parsed = (): void => {
    increment('parsed');
  };
  const detach = (): void => {
    page.off('request', requested);
    page.context().off('request', contextRequested);
    page.off('framenavigated', committed);
    page.off('domcontentloaded', parsed);
  };
  try {
    page.on('request', requested);
    page.context().on('request', contextRequested);
    page.on('framenavigated', committed);
    page.on('domcontentloaded', parsed);
  } catch (error: unknown) {
    detach();
    throw error;
  }
  return { observe, detach, snapshot: (): EntryModuleCounts & { overflow: boolean } => ({ ...counts, overflow }) };
}

async function parsedDocumentCapture(
  browser: Browser,
  deployment: Deployment,
  account: TestAccount,
  engine: string,
): Promise<void> {
  assert.notEqual(account.seededOn, null);
  assert.equal(account.seededOn?.origin, deployment.origin);
  assert.equal(account.seededOn.apiUrl, deployment.apiUrl);
  const origin = deployment.origin;
  const html = readFileSync(new URL('../dist/index.html', import.meta.url), 'utf8');
  const entry = /<script\b[^>]*type="module"[^>]*src="(\/assets\/[^"?#]+\.js)"/.exec(html)?.[1];
  if (entry === undefined) throw new Error('navigation_control_entry_module_missing');
  for (const mode of ['released', 'disposed', 'superseded'] as const) {
    await measurementScope(async (cleanup) => {
      const context = await browser.newContext({ ignoreHTTPSErrors: true, baseURL: origin });
      let diagnostics: ReturnType<typeof observeBrowserContext> | null = null;
      let stopProvenance: (() => void) | null = null;
      cleanup(async () => {
        const failures: unknown[] = [];
        try {
          await context.close();
        } catch (error: unknown) {
          failures.push(error);
        }
        try {
          if (diagnostics !== null) await diagnostics.finish();
        } catch (error: unknown) {
          for (const record of diagnostics?.records() ?? []) {
            recordMeasurement({
              event: 'browser.navigation_readiness_diagnostic',
              engine,
              mode,
              category: record.category,
              route: record.route,
              count: record.count,
            });
          }
          failures.push(error);
        } finally {
          stopProvenance?.();
        }
        if (failures.length > 0) throw new AggregateError(failures, 'navigation_capture_context_cleanup_failed');
      });
      diagnostics = observeBrowserContext(context, engine, 'navigation_readiness_control');
      stopProvenance = observeFailureProvenance(context, engine, origin);
      await installPolicyObserver(context);
      const page = await context.newPage();
      await signInThroughTheApi(page, account);
      const observation = observeEntryModule(page, origin, entry);
      let captureState = (): { waits: number; handles: number; owner: number; captured: number } => ({
        waits: 0,
        handles: 0,
        owner: 0,
        captured: -1,
      });
      try {
        const activity = nativeActivity(page);
        cleanup(() => {
          activity.dispose();
          return Promise.resolve();
        });
        let parsed = 0;
        const parsedCount = (): number => parsed;
        const onParsed = (): void => {
          parsed += 1;
        };
        page.on('domcontentloaded', onParsed);
        cleanup(() => {
          page.off('domcontentloaded', onParsed);
          return Promise.resolve();
        });
        const hold = await holdOneGet(page, origin, entry, false, observation.observe);
        cleanup(() => releaseRouteHolds(page, [hold], engine));
        const traffic = trafficAfterReady(page, todayQueryPaths);
        captureState = (): CaptureEvidence => {
          const { waits, handles, owner, captured } = traffic.capture();
          return { waits, handles, owner, captured: captured ?? -1 };
        };
        cleanup(() => traffic.dispose());
        await traffic.initialize();
        await traffic.prepareNavigation();
        await page.goto('/today', { waitUntil: 'commit' });
        await expect.poll(() => hold.request() !== null).toBe(true);
        await expect.poll(() => traffic.capture().waits > 0 || traffic.capture().handles > 0).toBe(true);
        const heldOwner = traffic.capture().owner;
        recordMeasurement({
          event: 'browser.capture_assertion_stage',
          engine,
          mode,
          stage: 'held_capture',
          waits: traffic.capture().waits,
          handles: traffic.capture().handles,
          parsed,
        });
        assert.equal(parsed, 0);
        assert.equal(hold.released(), false);
        assert.equal(traffic.capture().captured, null);
        assert.equal(traffic.capture().waits, 1);
        assert.equal(traffic.capture().handles, 0);
        assert.equal(traffic.readyReceived(), false);
        if (mode === 'disposed') {
          await traffic.dispose();
          recordMeasurement({
            event: 'browser.capture_assertion_stage',
            engine,
            mode,
            stage: 'disposed_capture',
            waits: traffic.capture().waits,
            handles: traffic.capture().handles,
            parsed,
          });
          assert.equal(traffic.capture().waits, 0);
          assert.equal(traffic.capture().handles, 0);
          assert.equal(parsed, 0);
          assert.equal(hold.released(), false);
          await hold.releaseAndJoin();
          await expect.poll(() => parsed > 0).toBe(true);
          assert.equal(traffic.capture().publications.includes(heldOwner), false);
          assert.equal(traffic.readyReceived(), false);
        } else if (mode === 'superseded') {
          const retiring = traffic.prepareNavigation().then(
            () => true,
            () => false,
          );
          cleanup(async () => {
            assert.equal(await retiring, true);
          });
          await hold.releaseAndJoin();
          await expect.poll(() => activity.terminalFor(hold.request())).toBe('finished');
          assert.equal(await retiring, true);
          recordMeasurement({
            event: 'browser.capture_assertion_stage',
            engine,
            mode,
            stage: 'retired_before_navigation',
            waits: traffic.capture().waits,
            handles: traffic.capture().handles,
            parsed,
          });
          assert.equal(traffic.capture().publications.includes(heldOwner), false);
          assert.equal(traffic.capture().captured, null);
          assert.equal(traffic.capture().waits, 0);
          assert.equal(traffic.capture().handles, 0);
          assert.equal(traffic.coherent(), false);
          await page.goto('/today');
          await ready(traffic);
          recordMeasurement({
            event: 'browser.capture_assertion_stage',
            engine,
            mode,
            stage: 'new_capture',
            waits: traffic.capture().waits,
            handles: traffic.capture().handles,
            parsed,
          });
          assert.equal(traffic.capture().owner > heldOwner, true);
          assert.equal(traffic.capture().captured, traffic.capture().owner);
          assert.equal(traffic.capture().publications.includes(heldOwner), false);
          assert.equal(parsedCount() > 0, true);
          assert.equal(traffic.coherent(), true);
        } else {
          await hold.releaseAndJoin();
          await ready(traffic);
          assert.equal(parsed, 1);
          assert.equal(traffic.capture().captured, heldOwner);
          assert.equal(traffic.capture().waits, 0);
          assert.equal(traffic.capture().handles, 1);
        }
        activity.verify();
        recordMeasurement({
          event: 'browser.navigation_readiness_control',
          control: 'parsed_document',
          engine,
          mode,
          parsingBlockedBeforeRelease: true,
          ownershipPreserved: true,
        });
      } catch (error: unknown) {
        recordMeasurement({
          event: 'browser.entry_module_observation',
          engine,
          mode,
          ...observation.snapshot(),
          ...captureState(),
        });
        throw error;
      } finally {
        observation.detach();
      }
    });
  }
}

async function rejectChildFrame(page: Page, engine: string): Promise<void> {
  const traffic = trafficAfterReady(page, todayQueryPaths);
  await traffic.initialize();
  try {
    await page.evaluate(() => {
      const child = document.createElement('iframe');
      child.name = 'readiness-owned-control';
      document.body.append(child);
    });
    await expect.poll(() => page.frames().length).toBe(2);
    assert.equal(traffic.snapshot().phase, 'failed');
    assert.equal(traffic.coherent(), false);
  } finally {
    try {
      await page.evaluate(() => {
        document.querySelector('iframe[name="readiness-owned-control"]')?.remove();
      });
    } finally {
      await traffic.dispose();
    }
  }
  assert.equal(page.frames().length, 1);
  recordMeasurement({ event: 'browser.navigation_readiness_control', control: 'child_frame_refused', engine });
}

async function checkEngine(engine: BrowserType): Promise<void> {
  await measurementScope(async (cleanup) => {
    stage(engine.name(), 'lifecycle', 'deployment_start');
    const deployment = await startDeployment(readE2ESettings(process.env), 0, 'navigation-readiness-control');
    cleanup(async () => {
      stage(engine.name(), 'lifecycle', 'deployment_stop');
      await deployment.stop();
      stage(engine.name(), 'lifecycle', 'deployment_stopped');
    });
    stage(engine.name(), 'lifecycle', 'browser_start');
    const browser = await engine.launch();
    cleanup(async () => {
      stage(engine.name(), 'lifecycle', 'browser_close');
      await browser.close();
      stage(engine.name(), 'lifecycle', 'browser_closed');
    });
    stage(engine.name(), 'lifecycle', 'context_start');
    const context = await browser.newContext({ ignoreHTTPSErrors: true, baseURL: deployment.origin });
    const diagnostics = observeBrowserContext(context, engine.name(), 'navigation_readiness_control');
    const stopProvenance = observeFailureProvenance(context, engine.name(), deployment.origin);
    cleanup(async () => {
      const failures: Error[] = [];
      stage(engine.name(), 'lifecycle', 'context_close');
      try {
        await context.close();
      } catch {
        failures.push(new Error('navigation_control_context_close_failed'));
      }
      stage(engine.name(), 'lifecycle', 'diagnostics_finish');
      try {
        await diagnostics.finish();
      } catch {
        for (const record of diagnostics.records()) {
          recordMeasurement({
            event: 'browser.navigation_readiness_diagnostic',
            engine: engine.name(),
            category: record.category,
            route: record.route,
            count: record.count,
          });
        }
        failures.push(new Error('navigation_control_diagnostics_failed'));
      }
      stopProvenance();
      if (failures.length !== 0) throw new AggregateError(failures, 'navigation_control_context_cleanup_failed');
    });
    await installPolicyObserver(context);
    stage(engine.name(), 'lifecycle', 'seed_account');
    const tutor = await seedAccount(deployment, 'Navigation control tutor', 'tutor', 'en');
    const page = await context.newPage();
    stage(engine.name(), 'lifecycle', 'sign_in');
    await signInThroughTheApi(page, tutor);

    await hardNavigationAndSpa(page, deployment.origin, engine.name());
    await repeatedObserverDisposal(page, engine.name());
    await parsedDocumentCapture(browser, deployment, tutor, engine.name());
    await rejectChildFrame(page, engine.name());
  });
}

async function run(): Promise<void> {
  const arguments_ = process.argv.slice(2);
  const diagnosticWebkit = arguments_.length === 1 && arguments_[0] === '--diagnostic-webkit';
  if (arguments_.length !== 0 && !diagnosticWebkit) throw new Error('navigation_control_arguments_invalid');
  recordMeasurement({
    event: 'browser.navigation_readiness_run',
    mode: diagnosticWebkit ? 'diagnostic_webkit' : 'all_engines',
  });
  takeRunLock(process.env);
  try {
    prepareRun();
    for (const engine of diagnosticWebkit ? [webkit] : [chromium, firefox, webkit]) await checkEngine(engine);
  } finally {
    finishRun();
  }
}

try {
  await run();
} catch (error: unknown) {
  recordFailureDetails(error);
  recordMeasurement({ event: 'browser.navigation_readiness_control_failed', kind: 'verification_failed' });
  process.exitCode = 1;
}
