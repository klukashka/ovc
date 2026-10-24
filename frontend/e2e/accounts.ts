import { randomBytes } from 'node:crypto';

import type { Page } from '@playwright/test';

import type { Deployment } from './deployment.ts';

export interface TestAccount {
  readonly name: string;
  readonly email: string;
  readonly password: string;
}

export interface SentMailLinks {
  readonly links: readonly string[];
}

export type MailReader = (to: string) => Promise<readonly SentMailLinks[]>;

export function testNetAddress(): string {
  return `198.51.100.${String(1 + Math.floor(Math.random() * 254))}`;
}

export function newTestAccount(name: string, role: 'tutor' | 'student' = 'tutor'): TestAccount {
  const suffix = randomBytes(6).toString('hex');
  return { name, email: `${role}-${suffix}@example.org`, password: `probe horse battery ${suffix}` };
}

async function postFromAnotherBrowser(deployment: Deployment, path: string, body: unknown): Promise<Response> {
  return fetch(`${deployment.apiUrl}${path}`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Origin: deployment.origin, 'X-Forwarded-For': testNetAddress() },
    body: JSON.stringify(body),
  });
}

export async function registerTutor(
  deployment: Deployment,
  account: TestAccount,
  language: 'ru' | 'en',
): Promise<void> {
  const response = await postFromAnotherBrowser(deployment, '/api/v1/auth/register', {
    role: 'tutor',
    name: account.name,
    email: account.email,
    password: account.password,
    language,
    timezone: 'Europe/Moscow',
    consent: true,
  });
  if (response.status !== 202) {
    throw new Error(`sign-up answered ${String(response.status)}`);
  }
}

export function confirmationToken(messages: readonly SentMailLinks[]): string {
  const link = messages.flatMap((message) => message.links).find((candidate) => candidate.includes('token='));
  const token = link === undefined ? null : new URL(link).searchParams.get('token');
  if (token === null) {
    throw new Error('no confirmation link was sent');
  }
  return token;
}

export async function confirmFromAnotherBrowser(
  deployment: Deployment,
  account: TestAccount,
  token: string,
): Promise<void> {
  const response = await postFromAnotherBrowser(deployment, '/api/v1/auth/verify-email', {
    token,
    new_password: account.password,
  });
  if (response.status !== 200) {
    throw new Error(`confirmation answered ${String(response.status)}`);
  }
}

export async function signInThroughTheApi(page: Page, account: TestAccount): Promise<void> {
  await page.goto('/');
  const status = await page.evaluate(async ({ email, password }) => {
    const response = await fetch('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password }),
    });
    return response.status;
  }, account);
  if (status !== 200) {
    throw new Error(`sign-in answered ${String(status)}`);
  }
}

export function emailedLinkTo(messages: readonly SentMailLinks[], path: string): string {
  const link = messages
    .flatMap((message) => message.links)
    .filter((candidate) => new URL(candidate).pathname === path || new URL(candidate).pathname.startsWith(`${path}/`))
    .at(-1);
  if (link === undefined) {
    throw new Error(`no emailed link to ${path}`);
  }
  return link;
}
