import { performance } from 'node:perf_hooks';

import type { Page, Request } from '@playwright/test';

import { readinessDeadlineMilliseconds } from '../e2e/deployment.ts';
import { parseServerFrame } from '../src/lib/realtime/frames.ts';
import {
  DevelopmentInitialReads,
  DevelopmentNavigationReads,
  type DevelopmentReadTarget,
} from './development-initial-reads.ts';
import { recordMeasurement, untilMeasurementCondition } from './slice6-measurement-support.ts';

const cancellationOutcomes = [
  'net::ERR_ABORTED',
  'NS_BINDING_ABORTED',
  'cancelled',
  'Load request cancelled',
  'Provisiolal navigation canceled.',
];
export const developmentReadinessMilliseconds = readinessDeadlineMilliseconds;

export function observeDevelopmentReads(
  page: Page,
  origin: string,
): {
  readonly declareDiagnosticDestination: (channel: string) => void;
  readonly diagnosticReport: (engine: string, stage: string) => void;
  readonly armNavigation: (source: string, destination: string) => void;
  readonly sealNavigation: () => void;
  readonly navigationDeadlineExpired: () => boolean;
  readonly navigationExpected: () => number;
  readonly arm: (account: string, channel: string) => void;
  readonly waitInitial: () => Promise<void>;
  readonly sealInitial: () => void;
  readonly waitSettled: () => Promise<void>;
  readonly mark: () => number;
  readonly initialDeadlineExpired: () => boolean;
  readonly waitChannel: (channel: string, after: number) => Promise<void>;
  readonly report: (engine: string, stage: string) => void;
  readonly verify: () => void;
  readonly finish: () => Promise<void>;
  readonly failureRecords: () => readonly { readonly kind: string; readonly count: number }[];
  readonly expectedFailures: () => number;
  readonly rawFailures: () => number;
  readonly unexpectedFailures: () => number;
} {
  const pending = new Set<Request>();
  const eventFailures = new Map<string, number>();
  const observations = new Set<Promise<void>>();
  const observe = (kind: string, action: () => void): void => {
    try {
      action();
    } catch {
      eventFailures.set(kind, Math.min(1000, (eventFailures.get(kind) ?? 0) + 1));
    }
  };
  const identities = new Map<Request, number>();
  const responses = new Map<Request, number>();
  const completed: { readonly target: string; readonly sequence: number }[] = [];
  let state: DevelopmentInitialReads | null = null;
  let navigation: DevelopmentNavigationReads | null = null;
  let navigationDestination = '';
  let navigationDocument = '';
  const navigationRequests = new Set<Request>();
  const invalidate = (kind: import('./development-initial-reads.ts').OwnershipFailure): void => {
    state?.ownershipChanged(kind);
    navigation?.ownershipChanged(kind);
  };
  let account = '';
  let generation = 0;
  let sequence = 0;
  let initialNavigation = false;
  let initialDocument = '';
  let deadline = 0;
  let failuresBeforeArm = 0;
  let missingQueryFunctions = 0;
  let ownerChecks = 0;
  let ownerMismatch = false;
  let ownerValidated = false;
  let targets = new Map<string, DevelopmentReadTarget>();
  const targetAddress = (address: string): string => {
    const url = new URL(address);
    const entries = [...url.searchParams.entries()].sort(
      ([firstKey, firstValue], [secondKey, secondValue]) =>
        firstKey.localeCompare(secondKey) || firstValue.localeCompare(secondValue),
    );
    return JSON.stringify([url.origin, url.pathname, entries]);
  };
  const isApiRequest = (request: Request): boolean => {
    const url = new URL(request.url());
    return url.origin === origin && url.pathname.startsWith('/api/') && request.resourceType() !== 'websocket';
  };
  const pendingApi = (): number => [...pending].filter(isApiRequest).length;
  const target = (request: Request): DevelopmentReadTarget =>
    request.method() === 'GET' ? (targets.get(targetAddress(request.url())) ?? 'other') : 'other';
  let diagnosticDestination = '';
  const diagnosticRequests = new Map<
    Request,
    { category: string; scope: string; ordinal: number; generation: number; accountMatch: boolean; methodGet: boolean }
  >();
  const diagnosticOrdinals = new Map<string, number>();
  const diagnosticRecords: Readonly<Record<string, string | number | boolean>>[] = [];
  const diagnosticEnd = (request: Request, cancelled: boolean, bodyComplete: boolean): void => {
    const entry = diagnosticRequests.get(request);
    if (entry === undefined) return;
    if (diagnosticRecords.length >= 2000) throw new Error('development_diagnostic_record_limit');
    const status = responses.get(request) ?? 0;
    diagnosticRecords.push({
      category: entry.category,
      scope: entry.scope,
      ordinal: entry.ordinal,
      cancellation: cancelled,
      documentMatch: entry.generation === generation && generation === 1,
      accountMatch: entry.accountMatch && ownerValidated && !ownerMismatch,
      methodGet: entry.methodGet,
      statusClass:
        status === 0
          ? 'none'
          : status < 200
            ? 'informational'
            : status < 300
              ? 'success'
              : status < 400
                ? 'redirect'
                : status < 500
                  ? 'client_error'
                  : 'server_error',
      completeBodyCount: bodyComplete ? 1 : 0,
    });
  };
  page.on('request', (request) => {
    observe('request_observation_failed', () => {
      pending.add(request);
      if (diagnosticDestination !== '') {
        if (diagnosticRequests.size >= 2000) throw new Error('development_diagnostic_request_limit');
        const address = targetAddress(request.url());
        const initial = target(request);
        const detail = targetAddress(`${origin}/api/v1/channels/${diagnosticDestination}`);
        const history = targetAddress(`${origin}/api/v1/channels/${diagnosticDestination}/messages`);
        const destination = request.method() === 'GET' && (address === detail || address === history);
        const category = destination
          ? address === detail
            ? 'current_channel'
            : 'history'
          : initial === 'other'
            ? isApiRequest(request)
              ? 'other_api'
              : 'non_api'
            : initial;
        const scope = destination ? 'destination' : initial === 'other' ? 'other' : 'initial';
        const key = `${scope}:${category}`;
        const ordinal = (diagnosticOrdinals.get(key) ?? 0) + 1;
        diagnosticOrdinals.set(key, ordinal);
        diagnosticRequests.set(request, {
          category,
          scope,
          ordinal,
          generation,
          accountMatch: ownerValidated && !ownerMismatch,
          methodGet: request.method() === 'GET',
        });
      }
      if (request.isNavigationRequest() && request.frame() === page.mainFrame() && state !== null) {
        if (initialNavigation && request.url() === initialDocument) initialNavigation = false;
        else {
          generation += 1;
          invalidate('document_generation_changed');
        }
      }
      if (state === null) return;
      const url = new URL(request.url());
      if (!isApiRequest(request)) return;
      sequence += 1;
      identities.set(request, sequence);
      if (navigation !== null && request.method() === 'GET' && targetAddress(request.url()) === navigationDestination) {
        navigationRequests.add(request);
        navigation.begin({ id: sequence, target: 'current_channel', account, generation }, performance.now());
      } else state.begin({ id: sequence, target: target(request), account, generation }, performance.now());
      if (
        request.method() === 'POST' &&
        ['/api/v1/auth/login', '/api/v1/auth/logout', '/api/v1/auth/logout-all'].includes(url.pathname)
      )
        invalidate('account_boundary_changed');
    });
  });
  page.on('framenavigated', (frame) => {
    observe('navigation_observation_failed', () => {
      if (state !== null && generation === 1 && frame === page.mainFrame()) {
        if (navigation !== null) navigation.navigation(frame.url() === navigationDocument);
        if (frame.url() !== initialDocument) state.ownershipChanged('spa_scope_changed');
      }
    });
  });
  page.on('response', (response) => {
    observe('response_observation_failed', () => {
      responses.set(response.request(), response.status());
      const url = new URL(response.url());
      if (state !== null && response.status() >= 400) invalidate('scope_boundary_changed');
      if (state !== null && url.origin === origin && url.pathname === '/api/v1/users/me' && response.status() === 200) {
        ownerChecks += 1;
        const observation = response
          .json()
          .then((body: unknown) => {
            if (typeof body !== 'object' || body === null || !('id' in body) || body.id !== account) {
              ownerMismatch = true;
              invalidate('account_identity_mismatch');
            } else {
              ownerValidated = true;
            }
          })
          .catch(() => {
            ownerMismatch = true;
            invalidate('account_identity_mismatch');
          })
          .finally(() => {
            ownerChecks -= 1;
            observations.delete(observation);
          });
        observations.add(observation);
      }
    });
  });
  page.on('requestfinished', (request) => {
    observe('requestfinished_observation_failed', () => {
      pending.delete(request);
      diagnosticEnd(request, false, true);
      const id = identities.get(request);
      if (id === undefined || state === null) return;
      const status = responses.get(request) ?? 0;
      if (navigationRequests.has(request)) navigation?.completed(id, status, performance.now());
      else state.completed(id, status, performance.now());
      if (status === 200) completed.push({ target: targetAddress(request.url()), sequence: id });
    });
  });
  page.on('requestfailed', (request) => {
    observe('requestfailed_observation_failed', () => {
      pending.delete(request);
      diagnosticEnd(request, cancellationOutcomes.includes(request.failure()?.errorText ?? ''), false);
      const id = identities.get(request);
      if (id === undefined || state === null) {
        failuresBeforeArm += 1;
        return;
      }
      const cancellation = cancellationOutcomes.includes(request.failure()?.errorText ?? '');
      if (navigationRequests.has(request)) navigation?.failed(id, cancellation, performance.now());
      else state.failed(id, cancellation, performance.now());
    });
  });
  page.on('console', (message) => {
    observe('console_observation_failed', () => {
      if (
        message.type() === 'error' &&
        message.text().includes('No queryFn was passed as an option, and no default queryFn was found.')
      )
        missingQueryFunctions += 1;
    });
  });
  page.on('websocket', (socket) => {
    socket.on('framereceived', ({ payload }) => {
      observe('websocket_observation_failed', () => {
        if (typeof payload !== 'string') return;
        const frame = parseServerFrame(payload);
        if (frame?.type === 'ready') state?.ready(performance.now());
        if (frame?.type === 'entity.changed' && frame.kind === 'channel') invalidate('scope_boundary_changed');
      });
    });
  });
  const observationFailureCount = (): number => [...eventFailures.values()].reduce((sum, count) => sum + count, 0);
  const requiredState = (): DevelopmentInitialReads => {
    if (state === null) throw new Error('development_initial_reads_not_armed');
    return state;
  };
  const verify = (): void => {
    requiredState().verify(performance.now());
    navigation?.verify(performance.now());
    if (eventFailures.size !== 0 || failuresBeforeArm !== 0 || !ownerValidated || ownerMismatch || ownerChecks !== 0)
      throw new Error('development_read_ownership_or_network_failed');
  };
  return {
    armNavigation: (source, destination): void => {
      verify();
      if (
        navigation !== null ||
        !requiredState().summary(performance.now()).initialSettlementComplete ||
        generation !== 1 ||
        page.url() !== `${origin}/messages/${source}` ||
        page.url() !== initialDocument ||
        pendingApi() !== 0 ||
        ownerChecks !== 0 ||
        observations.size !== 0
      )
        throw new Error('development_navigation_arm_invalid');
      navigationDestination = targetAddress(`${origin}/api/v1/channels/${destination}`);
      navigationDocument = `${origin}/messages/${destination}`;
      navigation = new DevelopmentNavigationReads(
        account,
        generation,
        performance.now() + developmentReadinessMilliseconds,
      );
    },
    sealNavigation: (): void => {
      verify();
      if (
        navigation === null ||
        page.url() !== navigationDocument ||
        pendingApi() !== 0 ||
        ownerChecks !== 0 ||
        observations.size !== 0
      )
        throw new Error('development_navigation_seal_invalid');
      navigation.seal(performance.now());
    },
    navigationDeadlineExpired: (): boolean => navigation?.expired(performance.now()) ?? false,
    navigationExpected: (): number => navigation?.expected() ?? 0,
    declareDiagnosticDestination: (channel): void => {
      if (diagnosticDestination !== '') throw new Error('development_diagnostic_destination_repeated');
      diagnosticDestination = channel;
    },
    diagnosticReport: (engine, stage): void => {
      for (const entry of diagnosticRecords)
        recordMeasurement({ event: 'browser.development_request_lifecycle', engine, stage, ...entry });
    },
    arm: (ownedAccount, channel): void => {
      if (state !== null) throw new Error('development_initial_reads_already_armed');
      account = ownedAccount;
      generation = 1;
      initialDocument = `${origin}/messages/${channel}`;
      initialNavigation = true;
      targets = new Map([
        [targetAddress(`${origin}/api/v1/channels/unread`), 'unread'],
        [targetAddress(`${origin}/api/v1/channels?q=`), 'list'],
        [targetAddress(`${origin}/api/v1/channels/${channel}`), 'current_channel'],
        [targetAddress(`${origin}/api/v1/channels/${channel}/messages`), 'history'],
      ]);
      deadline = performance.now() + developmentReadinessMilliseconds;
      state = new DevelopmentInitialReads(account, generation, deadline);
    },
    waitInitial: async (): Promise<void> => {
      await untilMeasurementCondition(
        () => {
          if (requiredState().problemCount() !== 0) verify();
          return (
            requiredState().initialSettled(performance.now()) &&
            pendingApi() === 0 &&
            ownerChecks === 0 &&
            observations.size === 0
          );
        },
        Math.max(0, deadline - performance.now()),
      );
      verify();
    },
    sealInitial: (): void => {
      verify();
      if (
        !requiredState().initialSettled(performance.now()) ||
        pendingApi() !== 0 ||
        ownerChecks !== 0 ||
        observations.size !== 0
      )
        throw new Error('development_initial_assembly_not_settled');
      requiredState().sealInitial();
    },
    waitSettled: async (): Promise<void> => {
      await untilMeasurementCondition(
        () =>
          pendingApi() === 0 &&
          [...pending].every((request) => ['websocket', 'eventsource'].includes(request.resourceType())) &&
          ownerChecks === 0 &&
          observations.size === 0,
        developmentReadinessMilliseconds,
      );
      verify();
    },
    mark: (): number => sequence,
    initialDeadlineExpired: (): boolean => requiredState().summary(performance.now()).deadlineExpired,
    waitChannel: async (channel, after): Promise<void> => {
      const addresses = [`${origin}/api/v1/channels/${channel}`, `${origin}/api/v1/channels/${channel}/messages`].map(
        targetAddress,
      );
      await untilMeasurementCondition(
        () =>
          addresses.every((address) =>
            completed.some((request) => request.target === address && request.sequence > after),
          ) &&
          pendingApi() === 0 &&
          ownerChecks === 0,
        developmentReadinessMilliseconds,
      );
      verify();
    },
    report: (engine, stage): void => {
      for (const entry of [
        ...requiredState().failureRecords(),
        ...(navigation?.failures() ?? []),
        ...[...eventFailures].map(([kind, count]) => ({ kind, count })),
      ])
        recordMeasurement({ event: 'browser.development_read_failure', engine, stage, ...entry });
      if (!ownerValidated || ownerMismatch)
        recordMeasurement({
          event: 'browser.development_read_failure',
          engine,
          stage,
          kind: ownerMismatch ? 'account_owner_mismatch' : 'account_owner_unverified',
          count: 1,
        });
      for (const entry of requiredState().snapshot())
        recordMeasurement({ event: 'browser.development_initial_target', engine, stage, ...entry });
      const pendingResources = new Map<string, number>();
      for (const request of pending) {
        const type = request.resourceType();
        const category = isApiRequest(request)
          ? 'api'
          : type === 'websocket' || type === 'eventsource'
            ? 'transport'
            : type === 'script' || type === 'stylesheet' || type === 'font' || type === 'image'
              ? 'asset'
              : 'other';
        pendingResources.set(category, (pendingResources.get(category) ?? 0) + 1);
      }
      const pendingRoutes = new Map<
        string,
        { readonly route: string; readonly stateBound: boolean; readonly responseSeen: boolean; count: number }
      >();
      for (const request of pending) {
        if (!isApiRequest(request)) continue;
        const path = new URL(request.url()).pathname;
        const targetClass = target(request);
        const route =
          path === '/api/v1/auth/login'
            ? 'auth_login'
            : path === '/api/v1/auth/refresh'
              ? 'auth_refresh'
              : path === '/api/v1/users/me'
                ? 'account_me'
                : targetClass !== 'other'
                  ? targetClass
                  : 'other_api';
        const stateBound = identities.has(request);
        const responseSeen = responses.has(request);
        const key = `${route}:${String(stateBound)}:${String(responseSeen)}`;
        const existing = pendingRoutes.get(key);
        if (existing === undefined) pendingRoutes.set(key, { route, stateBound, responseSeen, count: 1 });
        else existing.count += 1;
      }
      for (const entry of pendingRoutes.values())
        recordMeasurement({ event: 'browser.development_pending_api', engine, stage, ...entry });
      for (const [category, count] of pendingResources)
        recordMeasurement({ event: 'browser.development_pending_resource', engine, stage, category, count });
      recordMeasurement({
        event: 'browser.development_read_state',
        engine,
        stage,
        ...requiredState().summary(performance.now()),
        pendingApi: pendingApi(),
        pendingNonApi: pending.size - pendingApi(),
        pendingOwnerObservations: ownerChecks,
        pendingAsyncObservations: observations.size,
      });
      recordMeasurement({
        event: 'browser.development_read_diagnostics',
        engine,
        stage,
        rawRequestFailures: failuresBeforeArm + requiredState().failedRequests() + (navigation?.raw() ?? 0),
        expectedInitialCancellations: requiredState().expectedCancellations(),
        expectedNavigationCancellations: navigation?.expected() ?? 0,
        unexpectedFailures:
          observationFailureCount() +
          failuresBeforeArm +
          requiredState().problemCount() +
          (navigation?.unexpected() ?? 0),
        missingQueryFunctions,
        documentGeneration: generation,
        ownerVerified: ownerValidated && !ownerMismatch && ownerChecks === 0,
      });
    },
    verify,
    failureRecords: (): readonly { readonly kind: string; readonly count: number }[] => [
      ...requiredState().failureRecords(),
      ...(navigation?.failures() ?? []),
    ],
    finish: async (): Promise<void> => {
      await Promise.all([...observations]);
    },
    expectedFailures: (): number => requiredState().expectedCancellations(),
    rawFailures: (): number => failuresBeforeArm + requiredState().failedRequests() + (navigation?.raw() ?? 0),
    unexpectedFailures: (): number =>
      observationFailureCount() + failuresBeforeArm + requiredState().problemCount() + (navigation?.unexpected() ?? 0),
  };
}
