import { type Locator, type Page, type Request, expect, test as base } from '@playwright/test';

import {
  type TestAccount,
  confirmFromAnotherBrowser,
  confirmationToken,
  newTestAccount,
  registerTutor,
  seedAccount,
  signInThroughTheApi,
} from './accounts.ts';
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

export const test = base.extend<
  { control: TestControl; signedInPage: Page; socketSlots: SocketSlotHolder },
  { deployment: Deployment } & SharedActors
>({
  deployment: [
    async ({ browserName }, use, workerInfo): Promise<void> => {
      const deployment = await startDeployment(
        readE2ESettings(process.env),
        workerInfo.parallelIndex,
        `${browserName}-worker${String(workerInfo.workerIndex)}`,
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
}

export interface Diagnostics extends PageDiagnostics {
  readonly watch: (page: Page) => Promise<PageDiagnostics>;
}

interface WatchedPage extends PageDiagnostics {
  readonly verify: () => Promise<void>;
}

function describeFailure(failure: ExpectedFailure): string {
  return `${failure.method} ${failure.path} ${String(failure.status)}`;
}

const failedResourceLog = /^Failed to load resource: the server responded with a status of \d{3}/;

async function reportPolicyViolations(page: Page): Promise<void> {
  await page.addInitScript(() => {
    document.addEventListener('securitypolicyviolation', (event) => {
      console.error(`csp-violation ${event.violatedDirective} ${event.blockedURI}`);
    });
  });
}

async function watchPage(page: Page, deployment: Deployment): Promise<WatchedPage> {
  await reportPolicyViolations(page);
  const problems: string[] = [];
  const failures: string[] = [];
  const expected: string[] = [];
  page.on('console', (message) => {
    if (message.type() === 'error' && !failedResourceLog.test(message.text())) {
      problems.push(`console: ${message.text()}`);
    }
  });
  page.on('pageerror', (error) => {
    problems.push(`page error: ${error.message}`);
  });
  let step = 0;
  const navigations: number[] = [];
  const requests: number[] = [];
  const startOf = new Map<Request, number>();
  page.on('request', (request) => {
    step += 1;
    if (request.isNavigationRequest() && request.frame() === page.mainFrame()) {
      navigations.push(step);
    } else {
      requests.push(step);
    }
    startOf.set(request, step);
  });
  const failureChecks: Promise<FailedRequest>[] = [];
  page.on('requestfailed', (request) => {
    step += 1;
    const started = startOf.get(request) ?? Number.POSITIVE_INFINITY;
    const failed = step;
    failureChecks.push(
      request.response().then((response) => ({
        description: `${request.method()} ${request.url()}`,
        reason: request.failure()?.errorText ?? '',
        answered: response !== null,
        started,
        failed,
      })),
    );
  });
  page.on('response', (response) => {
    const url = new URL(response.url());
    if (response.status() >= 400 && url.origin === deployment.origin) {
      failures.push(`${response.request().method()} ${url.pathname}${url.search} ${String(response.status())}`);
    }
  });
  return {
    expectFailure: (failure): void => {
      expected.push(describeFailure(failure));
    },
    verify: async (): Promise<void> => {
      const failedRequests = await Promise.all(failureChecks);
      expect([...problems, ...unexpectedFailures(failedRequests, { navigations, requests })]).toEqual([]);
      expect([...failures].sort()).toEqual([...expected].sort());
    },
  };
}

export const diagnosedTest = test.extend<{ diagnostics: Diagnostics }>({
  diagnostics: [
    async ({ page, deployment }, use): Promise<void> => {
      const main = await watchPage(page, deployment);
      const others: WatchedPage[] = [];
      await use({
        expectFailure: main.expectFailure,
        watch: async (other): Promise<PageDiagnostics> => {
          const watched = await watchPage(other, deployment);
          others.push(watched);
          return watched;
        },
      });
      await main.verify();
      for (const watched of others) {
        await watched.verify();
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
