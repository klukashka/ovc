import assert from 'node:assert/strict';

import { chromium, firefox, webkit } from '@playwright/test';

import { seedAccount } from '../e2e/accounts.ts';
import { installPolicyObserver, observeBrowserContext } from '../e2e/browser-diagnostics.ts';
import {
  finishRun,
  prepareRun,
  readE2ESettings,
  startDeployment,
  takeRunLock,
  readinessDeadlineMilliseconds,
} from '../e2e/deployment.ts';
import { measurementScope, recordMeasurement } from './slice6-measurement-support.ts';

interface CanaryScheduler {
  readonly schedule: (milliseconds: number, task: () => void) => () => void;
  readonly wallClock: () => number;
}
interface CanarySession {
  readonly signIn: (email: string, password: string) => Promise<{ readonly kind: 'signedIn' | 'refused' }>;
  readonly signOut: () => Promise<void>;
}
interface CanarySessionModule {
  readonly Session: new (dependencies: {
    readonly origin: string;
    readonly transport: import('../src/lib/api/transport.ts').AuthTransport;
    readonly locks: { request<Result>(name: string, callback: () => Promise<Result>): Promise<Result> };
    readonly scheduler: CanaryScheduler;
  }) => CanarySession;
  readonly browserLocks: (locks: LockManager | undefined) => {
    request<Result>(name: string, callback: () => Promise<Result>): Promise<Result>;
  };
  readonly browserScheduler: CanaryScheduler;
}
interface CanaryQueryClient {
  readonly clear: () => void;
  readonly suspendFollowUps: () => void;
  readonly resumeFollowUps: () => void;
}
interface CanaryConnection {
  readonly follow: (session: CanarySession) => () => void;
}
interface CanaryRealtimeModule {
  readonly reportRealtimeError: (error: Error) => void;
  readonly wireRealtime: (options: {
    readonly session: CanarySession;
    readonly connection: CanaryConnection;
    readonly queryClient: CanaryQueryClient;
    readonly scheduler: CanaryScheduler;
    readonly sockets: import('../src/lib/realtime/socket.ts').SocketFactory;
    readonly lifecycle: { readonly dispose: () => void };
    readonly onFailure: (error: Error) => void;
  }) => void;
}

async function run(): Promise<void> {
  takeRunLock(process.env);
  try {
    prepareRun('measurement-dev-tls');
    await measurementScope(async (cleanup) => {
      const deployment = await startDeployment(
        readE2ESettings(process.env),
        0,
        'realtime-reporting',
        undefined,
        'measurement-dev-tls',
      );
      cleanup(() => deployment.stop());
      const account = await seedAccount(deployment, 'Synthetic reporter tutor', 'tutor', 'en');
      const aliasOnly = process.argv.includes('--callback-alias');
      for (const engine of aliasOnly ? [firefox] : [chromium, firefox, webkit]) {
        for (const alias of aliasOnly ? [true] : engine === firefox ? [false, true] : [false]) {
          await measurementScope(async (release) => {
            const browser = await engine.launch();
            release(() => browser.close());
            const context = await browser.newContext({ ignoreHTTPSErrors: true, baseURL: deployment.origin });
            release(() => context.close());
            const observer = observeBrowserContext(context, engine.name(), 'realtime_error_reporting');
            await installPolicyObserver(context);
            const page = await context.newPage();
            assert.equal((await page.goto('/api/v1/health/live'))?.status(), 200);
            const result = await page.evaluate(
              async ({ origin, email, password, alias, deadline }) => {
                const realtime = (await import(`${origin}/src/app/realtime.ts`)) as unknown as CanaryRealtimeModule;
                const sessions = (await import(`${origin}/src/lib/api/session.ts`)) as unknown as CanarySessionModule;
                const transports = (await import(
                  `${origin}/src/lib/api/transport.ts`
                )) as unknown as typeof import('../src/lib/api/transport.ts');
                const sockets = (await import(
                  `${origin}/src/lib/realtime/socket.ts`
                )) as unknown as typeof import('../src/lib/realtime/socket.ts');
                const frames = (await import(
                  `${origin}/src/lib/realtime/frames.ts`
                )) as unknown as typeof import('../src/lib/realtime/frames.ts');
                const queries = (await import(`${origin}/src/app/queryClient.ts`)) as unknown as {
                  readonly FollowingQueryClient: new () => CanaryQueryClient;
                };
                const lifecycles = (await import(`${origin}/src/app/documentLifecycle.ts`)) as unknown as {
                  readonly DocumentLifecycle: new (
                    page: { readonly events: EventTarget; readonly loaded: () => boolean },
                    scheduler: CanaryScheduler,
                    queries: CanaryQueryClient,
                  ) => { readonly dispose: () => void };
                };
                const connections = (await import(`${origin}/src/app/connectionState.ts`)) as unknown as {
                  readonly ConnectionState: new (scheduler: CanaryScheduler) => CanaryConnection;
                };
                const session = new sessions.Session({
                  origin,
                  transport: transports.httpAuthTransport(origin, window.fetch.bind(window)),
                  locks: sessions.browserLocks(navigator.locks),
                  scheduler: sessions.browserScheduler,
                });
                if ((await session.signIn(email, password)).kind !== 'signedIn')
                  throw new Error('realtime_reporting_sign_in_refused');
                const queryClient = new queries.FollowingQueryClient();
                const lifecycle = new lifecycles.DocumentLifecycle(
                  { events: window, loaded: (): boolean => true },
                  sessions.browserScheduler,
                  queryClient,
                );
                const connection = new connections.ConnectionState(sessions.browserScheduler);
                const unfollow = connection.follow(session);
                let socket: import('../src/lib/realtime/socket.ts').RealtimeSocket | null = null;
                let markReady = (): void => undefined;
                const ready = new Promise<void>((resolve) => {
                  markReady = resolve;
                });
                const factory = sockets.browserSocketFactory(origin);
                let errorEvents = 0;
                let originalError = false;
                let receiverTypeError = false;
                let markReported = (): void => undefined;
                const reported = new Promise<void>((resolve) => {
                  markReported = resolve;
                });
                const onError = (event: ErrorEvent): void => {
                  errorEvents += 1;
                  originalError ||=
                    event.error instanceof Error &&
                    event.error.message === 'the realtime server refused a frame with bad_request';
                  receiverTypeError ||=
                    event.error instanceof TypeError &&
                    /reportError.*Window|Window.*reportError/.test(event.error.message);
                  markReported();
                };
                window.addEventListener('error', onError);
                const bounded = async (operation: Promise<void>): Promise<void> => {
                  let timer = 0;
                  const expired = new Promise<never>((_resolve, reject) => {
                    timer = window.setTimeout(() => {
                      reject(new Error('realtime_reporting_deadline'));
                    }, deadline);
                  });
                  try {
                    await Promise.race([operation, expired]);
                  } finally {
                    window.clearTimeout(timer);
                  }
                };
                try {
                  realtime.wireRealtime({
                    session,
                    connection,
                    queryClient,
                    scheduler: sessions.browserScheduler,
                    sockets: (events) => {
                      const owned = factory({
                        ...events,
                        onText: (text): void => {
                          events.onText(text);
                          if (frames.parseServerFrame(text)?.type === 'ready') markReady();
                        },
                      });
                      socket = owned;
                      return owned;
                    },
                    lifecycle,
                    onFailure: alias ? reportError : realtime.reportRealtimeError,
                  });
                  await bounded(ready);
                  const owned = socket as import('../src/lib/realtime/socket.ts').RealtimeSocket | null;
                  if (owned === null) throw new Error('realtime_reporting_socket_missing');
                  owned.send(JSON.stringify({ type: 'diagnostic_unknown_frame' }));
                  await bounded(reported);
                  return { errorEvents, originalError, receiverTypeError };
                } finally {
                  lifecycle.dispose();
                  await session.signOut();
                  unfollow();
                  queryClient.clear();
                  window.removeEventListener('error', onError);
                }
              },
              {
                origin: deployment.origin,
                email: account.email,
                password: account.password,
                alias,
                deadline: readinessDeadlineMilliseconds,
              },
            );
            await context.close();
            await assert.rejects(async (): Promise<void> => {
              await observer.finish();
            });
            assert.equal(result.errorEvents, 1);
            const pageErrors = observer
              .records()
              .filter((entry) => entry.category === 'page_error')
              .reduce((count, entry) => count + entry.count, 0);
            assert.equal(pageErrors, 1);
            assert.equal(observer.errors(), 1);
            assert.equal(observer.warnings(), 0);
            const reportingContractPassed = result.originalError && !result.receiverTypeError;
            recordMeasurement({
              event: 'browser.realtime_error_reporting',
              engine: engine.name(),
              callbackAlias: alias,
              reportingContractPassed,
              originalErrorReported: result.originalError,
              receiverTypeError: result.receiverTypeError,
              errorEvents: result.errorEvents,
              collectorRejected: true,
            });
            if (alias && !aliasOnly) {
              assert.equal(result.originalError, false);
              assert.equal(result.receiverTypeError, true);
            } else assert.equal(reportingContractPassed, true);
          });
        }
      }
    });
  } finally {
    finishRun();
  }
}

try {
  await run();
} catch {
  recordMeasurement({ event: 'browser.realtime_error_reporting_failed', kind: 'verification_failed' });
  process.exitCode = 1;
}
