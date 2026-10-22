import { type Page, test as base } from '@playwright/test';

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

export async function seedVerifiedUser(deployment: Deployment, name: string): Promise<TestAccount> {
  const control = testControl(deployment);
  const account = newTestAccount(name);
  await registerTutor(deployment, account, 'ru');
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

export { expect } from '@playwright/test';
export { testNetAddress } from './accounts.ts';
