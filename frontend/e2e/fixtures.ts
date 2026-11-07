import { type BrowserContext, type Locator, type Page, type Request, expect, test as base } from '@playwright/test';

import {
  type TestAccount,
  confirmFromAnotherBrowser,
  confirmationToken,
  newTestAccount,
  registerTutor,
  seedAccount,
  signInThroughTheApi,
} from './accounts.ts';
import {
  type ObservedBrowser,
  installPolicyObserver,
  observeBrowserContext,
  observedBrowser,
} from './browser-diagnostics.ts';
import { type Deployment, readE2ESettings, startDeployment } from './deployment.ts';
import { type SocketSlotHolder, socketSlotHolder } from './realtime.ts';
import { type FailedRequest, unexpectedFailures } from './request-failures.ts';
import { createGroup, seedActiveStudent, seedActiveStudentsThroughCode, seedJoinRequests } from './tutoring.ts';

export interface SentMail {
  readonly subject: string;
  readonly text: string;
  readonly links: readonly string[];
}

export interface TestControl {
  readonly mail: (to: string) => Promise<SentMail[]>;
  readonly tick: () => Promise<void>;
  readonly advance: (seconds: number) => Promise<void>;
  readonly now: () => Promise<number>;
}

async function post(apiUrl: string, path: string, body: unknown): Promise<unknown> {
  const response = await fetch(`${apiUrl}${path}`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    throw new Error(`${path} answered ${String(response.status)}`);
  }
  return response.json();
}

function isSentMail(value: unknown): value is SentMail {
  return (
    typeof value === 'object' &&
    value !== null &&
    'subject' in value &&
    typeof value.subject === 'string' &&
    'text' in value &&
    typeof value.text === 'string' &&
    'links' in value &&
    Array.isArray(value.links) &&
    value.links.every((link) => typeof link === 'string')
  );
}

export function testControl(deployment: Deployment): TestControl {
  return {
    mail: async (to: string): Promise<SentMail[]> => {
      const messages = await post(deployment.apiUrl, '/__test__/mail', { to });
      if (!Array.isArray(messages) || !messages.every(isSentMail)) {
        throw new Error('the mail sink answered an unexpected shape');
      }
      return messages;
    },
    tick: async (): Promise<void> => {
      await post(deployment.apiUrl, '/__test__/tick', {});
    },
    advance: async (seconds: number): Promise<void> => {
      await post(deployment.apiUrl, '/__test__/clock', { advance_seconds: seconds });
    },
    now: async (): Promise<number> => {
      const answer = await post(deployment.apiUrl, '/__test__/clock', { advance_seconds: 1 });
      if (typeof answer !== 'object' || answer === null || !('now' in answer) || typeof answer.now !== 'string') {
        throw new Error('the clock route answered an unexpected shape');
      }
      return Date.parse(answer.now);
    },
  };
}

export async function seedVerifiedUser(
  deployment: Deployment,
  name: string,
  language: 'ru' | 'en' = 'ru',
): Promise<TestAccount> {
  return seedAccount(deployment, name, 'tutor', language);
}

export async function signUpAndConfirmTutor(
  deployment: Deployment,
  name: string,
  language: 'ru' | 'en' = 'ru',
): Promise<TestAccount> {
  const control = testControl(deployment);
  const account = newTestAccount(name);
  await registerTutor(deployment, account, language);
  await control.tick();
  await confirmFromAnotherBrowser(deployment, account, confirmationToken(await control.mail(account.email)));
  return account;
}

export interface MariaAndAnna {
  readonly maria: TestAccount;
  readonly anna: TestAccount;
  readonly cardId: string;
}

export interface PavelAndAGroup {
  readonly pavel: TestAccount;
  readonly groupId: string;
}

export interface StudentsOfPavel {
  readonly pavel: TestAccount;
  readonly cardIds: readonly string[];
}

export interface MariaWithJoinRequests {
  readonly maria: TestAccount;
}

interface SharedActors {
  readonly tutor: TestAccount;
  readonly mariaAndAnna: MariaAndAnna;
  readonly pavelAndAGroup: PavelAndAGroup;
  readonly studentsOfPavel: StudentsOfPavel;
  readonly mariaWithJoinRequests: MariaWithJoinRequests;
}

export interface MeasurementWorkerOptions {
  readonly measurementDatabaseMode: 'on' | 'off' | undefined;
}

export const test = base.extend<
  {
    control: TestControl;
    signedInPage: Page;
    socketSlots: SocketSlotHolder;
    observedBrowser: ObservedBrowser;
    diagnosticRegistry: DiagnosticRegistry;
  },
  { deployment: Deployment } & SharedActors & MeasurementWorkerOptions
>({
  diagnosticRegistry: [
    async ({ browserName }, use): Promise<void> => {
      const registry = diagnosticRegistry(browserName);
      await use(registry);
      await registry.verify();
    },
    { auto: true },
  ],
  context: async ({ context, diagnosticRegistry }, use): Promise<void> => {
    await diagnosticRegistry.watch(context);
    await use(context);
  },
  observedBrowser: async ({ browser, diagnosticRegistry }, use): Promise<void> => {
    await use(observedBrowser(browser, (context) => diagnosticRegistry.watch(context)));
  },
  measurementDatabaseMode: [undefined, { scope: 'worker', option: true }],
  deployment: [
    async ({ browserName, measurementDatabaseMode }, use, workerInfo): Promise<void> => {
      const deployment = await startDeployment(
        readE2ESettings(process.env),
        workerInfo.parallelIndex,
        `${browserName}-worker${String(workerInfo.workerIndex)}`,
        measurementDatabaseMode === undefined ? undefined : { synchronousCommit: measurementDatabaseMode },
      );
      try {
        await use(deployment);
      } finally {
        await deployment.stop();
      }
    },
    { scope: 'worker', timeout: 150_000 },
  ],
  tutor: [
    async ({ deployment }, use): Promise<void> => {
      await use(await seedVerifiedUser(deployment, 'Тестовый репетитор'));
    },
    { scope: 'worker' },
  ],
  mariaAndAnna: [
    async ({ deployment }, use): Promise<void> => {
      const maria = await seedVerifiedUser(deployment, 'Мария');
      const { student: anna, cardId } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
      await use({ maria, anna, cardId });
    },
    { scope: 'worker' },
  ],
  pavelAndAGroup: [
    async ({ deployment }, use): Promise<void> => {
      const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
      await use({ pavel, groupId: await createGroup(deployment, pavel, 'Evening algebra', []) });
    },
    { scope: 'worker' },
  ],
  studentsOfPavel: [
    async ({ deployment }, use): Promise<void> => {
      const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
      const cardIds = await seedActiveStudentsThroughCode(deployment, pavel, 31);
      await use({ pavel, cardIds });
    },
    { scope: 'worker' },
  ],
  mariaWithJoinRequests: [
    async ({ deployment }, use): Promise<void> => {
      const maria = await seedVerifiedUser(deployment, 'Мария');
      await seedJoinRequests(deployment, maria, 31);
      await use({ maria });
    },
    { scope: 'worker' },
  ],
  baseURL: async ({ deployment }, use): Promise<void> => {
    await use(deployment.origin);
  },
  control: async ({ deployment }, use): Promise<void> => {
    await use(testControl(deployment));
  },
  socketSlots: async ({ deployment, page }, use): Promise<void> => {
    const holder = socketSlotHolder(deployment, page);
    try {
      await use(holder);
      await holder.expectPageSocketsRefused();
    } finally {
      holder.release();
    }
  },
  signedInPage: async ({ page, tutor }, use): Promise<void> => {
    await signInThroughTheApi(page, tutor);
    await use(page);
  },
});

export interface ExpectedFailure {
  readonly method: 'GET' | 'POST' | 'PATCH' | 'DELETE';
  readonly path: string;
  readonly status: number;
}

export interface PageDiagnostics {
  readonly expectFailure: (failure: ExpectedFailure) => void;
  readonly expectAbortedRequest: (path: RegExp) => void;
  readonly finishHttpPhase: () => Promise<void>;
  readonly beginLanguageFailure: (language: 'en' | 'ru') => void;
  readonly finishLanguageFailure: () => Promise<void>;
  readonly expectConsoleError: (text: string) => void;
  readonly expectConsoleWarning: (fragment: string) => void;
}

export interface Diagnostics extends PageDiagnostics {
  readonly watch: (page: Page) => Promise<PageDiagnostics>;
  readonly closeContext: (context: BrowserContext) => Promise<void>;
}

interface WatchedPage extends PageDiagnostics {
  readonly page: Page;
  readonly markClose: () => void;
  readonly verify: () => void;
}

function describeFailure(failure: ExpectedFailure): string {
  return `${failure.method} ${failure.path} ${String(failure.status)}`;
}

interface DiagnosticRegistry {
  readonly watch: (context: BrowserContext) => Promise<void>;
  readonly console: (page: Page) => ReturnType<typeof observeBrowserContext>;
  readonly verify: () => Promise<void>;
}

function diagnosticRegistry(engine: string): DiagnosticRegistry {
  const watched = new Map<BrowserContext, ReturnType<typeof observeBrowserContext>>();
  return {
    watch: async (context): Promise<void> => {
      if (watched.has(context)) return;
      watched.set(context, observeBrowserContext(context, engine, 'journey'));
      await installPolicyObserver(context);
    },
    console: (page): ReturnType<typeof observeBrowserContext> => {
      const observer = watched.get(page.context());
      if (observer === undefined) throw new Error('browser context was not observed before page creation');
      return observer;
    },
    verify: async (): Promise<void> => {
      for (const observer of watched.values()) await observer.finish();
    },
  };
}

function pathOf(address: string): string {
  return URL.canParse(address) ? new URL(address).pathname : address;
}

interface FailedAt {
  readonly failed: FailedRequest;
  readonly path: string;
}

function unexpectedAborts(failures: readonly FailedAt[], abortedPaths: readonly RegExp[]): FailedRequest[] {
  return failures
    .filter((failure) => !abortedPaths.some((path) => path.test(failure.path)))
    .map((failure) => failure.failed);
}

function unseenAborts(failures: readonly FailedAt[], abortedPaths: readonly RegExp[]): string[] {
  return abortedPaths
    .filter((path) => !failures.some((failure) => path.test(failure.path)))
    .map(() => 'expected_aborted_request_missing');
}

function watchPage(page: Page, deployment: Deployment, registry: DiagnosticRegistry): WatchedPage {
  const consoleExpectation = registry.console(page).expect(page);
  const failures: string[] = [];
  const expected: string[] = [];
  const abortedPaths = consoleExpectation.abortedPaths;
  let step = 0;
  const navigations: number[] = [];
  const requests: number[] = [];
  const closes: number[] = [];
  const startOf = new Map<Request, number>();
  const answeredRequests = new Set<Request>();
  page.on('request', (request) => {
    step += 1;
    if (request.isNavigationRequest() && request.frame() === page.mainFrame()) {
      navigations.push(step);
    } else {
      requests.push(step);
    }
    startOf.set(request, step);
  });
  const failedRequests: FailedAt[] = [];
  page.on('requestfailed', (request) => {
    step += 1;
    failedRequests.push({
      failed: {
        description: request.method(),
        reason: request.failure()?.errorText ?? '',
        answered: answeredRequests.has(request),
        started: startOf.get(request) ?? Number.POSITIVE_INFINITY,
        failed: step,
      },
      path: pathOf(request.url()),
    });
  });
  page.on('response', (response) => {
    answeredRequests.add(response.request());
    const url = new URL(response.url());
    if (response.status() >= 400 && url.origin === deployment.origin) {
      failures.push(`${response.request().method()} ${url.pathname}${url.search} ${String(response.status())}`);
    }
  });
  return {
    page,
    markClose: (): void => {
      step += 1;
      closes.push(step);
    },
    expectFailure: (failure): void => {
      registry.console(page).expectHttp(page, {
        method: failure.method,
        address: new URL(failure.path, deployment.origin).href,
        status: failure.status,
      });
      expected.push(describeFailure(failure));
    },
    finishHttpPhase: (): Promise<void> => registry.console(page).finishHttp(page),
    beginLanguageFailure: (language): void => {
      registry.console(page).beginLanguageFailure(page, deployment.origin, language);
    },
    finishLanguageFailure: (): Promise<void> => registry.console(page).finishLanguageFailure(page),
    expectAbortedRequest: (path): void => {
      abortedPaths.push(path);
    },
    expectConsoleError: (text): void => {
      consoleExpectation.expected.push(text);
    },
    expectConsoleWarning: (fragment): void => {
      consoleExpectation.expectedWarnings.set(fragment, 0);
    },
    verify: (): void => {
      expect([
        ...unexpectedFailures(unexpectedAborts(failedRequests, abortedPaths), { navigations, requests, closes }).map(
          () => 'request_failure',
        ),
        ...unseenAborts(failedRequests, abortedPaths),
      ]).toEqual([]);
      expect(
        failures.length === expected.length &&
          [...failures].sort().every((entry, index) => entry === [...expected].sort()[index]),
      ).toBe(true);
    },
  };
}

export const diagnosedTest = test.extend<{ diagnostics: Diagnostics }>({
  diagnostics: [
    async ({ page, deployment, diagnosticRegistry }, use): Promise<void> => {
      const main = watchPage(page, deployment, diagnosticRegistry);
      const others: WatchedPage[] = [];
      const pages = new Map<Page, WatchedPage>([[page, main]]);
      await use({
        expectFailure: main.expectFailure,
        finishHttpPhase: main.finishHttpPhase,
        beginLanguageFailure: main.beginLanguageFailure,
        finishLanguageFailure: main.finishLanguageFailure,
        expectAbortedRequest: main.expectAbortedRequest,
        expectConsoleError: main.expectConsoleError,
        expectConsoleWarning: main.expectConsoleWarning,
        watch: (other): Promise<PageDiagnostics> => {
          const existing = pages.get(other);
          if (existing !== undefined) return Promise.resolve(existing);
          const watched = watchPage(other, deployment, diagnosticRegistry);
          others.push(watched);
          pages.set(other, watched);
          return Promise.resolve(watched);
        },
        closeContext: async (context): Promise<void> => {
          for (const watched of [main, ...others]) {
            if (watched.page.context() === context) {
              watched.markClose();
            }
          }
          await context.close();
        },
      });
      main.verify();
      for (const watched of others) {
        watched.verify();
      }
    },
    { auto: true },
  ],
});

export async function expectRowsNamed(rows: Locator, names: readonly string[]): Promise<void> {
  await expect(rows).toHaveCount(names.length);
  for (const name of names) {
    await expect(rows.filter({ hasText: name })).toHaveCount(1);
  }
}

export { expect } from '@playwright/test';
export { testNetAddress } from './accounts.ts';
