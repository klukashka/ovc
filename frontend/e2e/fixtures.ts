import { test as base } from '@playwright/test';

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

export function testNetAddress(): string {
  return `198.51.100.${String(1 + Math.floor(Math.random() * 254))}`;
}

export const test = base.extend<{ control: TestControl }, { deployment: Deployment }>({
  deployment: [
    async ({ browserName }, use, workerInfo): Promise<void> => {
      const deployment = await startDeployment(readE2ESettings(process.env), workerInfo.parallelIndex, browserName);
      try {
        await use(deployment);
      } finally {
        await deployment.stop();
      }
    },
    { scope: 'worker', timeout: 150_000 },
  ],
  baseURL: async ({ deployment }, use): Promise<void> => {
    await use(deployment.origin);
  },
  control: async ({ deployment }, use): Promise<void> => {
    await use(testControl(deployment));
  },
});

export { expect } from '@playwright/test';
