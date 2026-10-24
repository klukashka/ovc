import { type Page, type Request, expect, test as base } from '@playwright/test';

import {
  type TestAccount,
  confirmFromAnotherBrowser,
  confirmationToken,
  newTestAccount,
  registerTutor,
  signInThroughTheApi,
} from './accounts.ts';
import { type Deployment, readE2ESettings, startDeployment } from './deployment.ts';

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

function testControl(deployment: Deployment): TestControl {
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
  const control = testControl(deployment);
  const account = newTestAccount(name);
  await registerTutor(deployment, account, language);
  await control.tick();
  await confirmFromAnotherBrowser(deployment, account, confirmationToken(await control.mail(account.email)));
  return account;
}

export const test = base.extend<
  { control: TestControl; signedInPage: Page },
  { deployment: Deployment; tutor: TestAccount }
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
  baseURL: async ({ deployment }, use): Promise<void> => {
    await use(deployment.origin);
  },
  control: async ({ deployment }, use): Promise<void> => {
    await use(testControl(deployment));
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

export interface Diagnostics {
  readonly expectFailure: (failure: ExpectedFailure) => void;
}

function describeFailure(failure: ExpectedFailure): string {
  return `${failure.method} ${failure.path} ${String(failure.status)}`;
}

const cancelledRequest = /abort|cancel/i;
const failedResourceLog = /^Failed to load resource: the server responded with a status of \d{3}/;

async function reportPolicyViolations(page: Page): Promise<void> {
  await page.addInitScript(() => {
    document.addEventListener('securitypolicyviolation', (event) => {
      console.error(`csp-violation ${event.violatedDirective} ${event.blockedURI}`);
    });
  });
}

export const diagnosedTest = test.extend<{ diagnostics: Diagnostics }>({
  diagnostics: [
    async ({ page, deployment }, use): Promise<void> => {
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
      let documents = 0;
      const documentOf = new Map<Request, number>();
      page.on('request', (request) => {
        if (request.isNavigationRequest() && request.frame() === page.mainFrame()) {
          documents += 1;
        }
        documentOf.set(request, documents);
      });
      const failureChecks: Promise<void>[] = [];
      page.on('requestfailed', (request) => {
        failureChecks.push(
          request.response().then((response) => {
            const reason = request.failure()?.errorText ?? '';
            const leftBehind = documents > (documentOf.get(request) ?? documents);
            if (response === null && !(cancelledRequest.test(reason) && leftBehind)) {
              problems.push(`request failed: ${request.method()} ${request.url()} ${reason}`);
            }
          }),
        );
      });
      page.on('response', (response) => {
        const url = new URL(response.url());
        if (response.status() >= 400 && url.origin === deployment.origin) {
          failures.push(`${response.request().method()} ${url.pathname}${url.search} ${String(response.status())}`);
        }
      });
      await use({
        expectFailure: (failure) => {
          expected.push(describeFailure(failure));
        },
      });
      await Promise.all(failureChecks);
      expect(problems).toEqual([]);
      expect([...failures].sort()).toEqual([...expected].sort());
    },
    { auto: true },
  ],
});

export { expect } from '@playwright/test';
export { testNetAddress } from './accounts.ts';
