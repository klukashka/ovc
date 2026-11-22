import { randomBytes } from 'node:crypto';

import { expect, type Page } from '@playwright/test';

import type { Deployment } from './deployment.ts';

export type SeedingDeployment = Pick<Deployment, 'apiUrl' | 'origin'>;

export interface TestAccount {
  readonly name: string;
  readonly email: string;
  readonly password: string;
  readonly seededOn: SeedingDeployment | null;
}

export interface SeededSession {
  readonly accessToken: string;
  readonly refreshCookie: string;
  readonly lifetimeSeconds: number;
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
  return { name, email: `${role}-${suffix}@example.org`, password: `probe horse battery ${suffix}`, seededOn: null };
}

const refreshCookieName = '__Host-ovc_refresh';

function stringField(value: unknown, name: string): string {
  if (typeof value !== 'object' || value === null || !(name in value)) {
    throw new Error(`the test control answer has no ${name}`);
  }
  const found: unknown = (value as Record<string, unknown>)[name];
  if (typeof found !== 'string') {
    throw new Error(`${name} is not a string`);
  }
  return found;
}

async function postToTestControl(
  deployment: SeedingDeployment,
  path: string,
  body: unknown,
): Promise<{ readonly body: unknown; readonly session: SeededSession }> {
  const response = await fetch(`${deployment.apiUrl}${path}`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (response.status !== 200) {
    throw new Error(`${path} answered ${String(response.status)}`);
  }
  const answer: unknown = await response.json();
  const header = response.headers.getSetCookie().find((candidate) => candidate.startsWith(`${refreshCookieName}=`));
  const cookie = header?.split(';')[0];
  const lifetime = header === undefined ? undefined : /; Max-Age=([1-9][0-9]*)/.exec(header)?.[1];
  if (cookie === undefined || lifetime === undefined) {
    throw new Error(`${path} set no refresh cookie`);
  }
  return {
    body: answer,
    session: {
      accessToken: stringField(answer, 'access_token'),
      refreshCookie: cookie.slice(refreshCookieName.length + 1),
      lifetimeSeconds: Number(lifetime),
    },
  };
}

export async function seedAccount(
  deployment: SeedingDeployment,
  name: string,
  role: 'tutor' | 'student',
  language: 'ru' | 'en',
): Promise<TestAccount> {
  const { email } = newTestAccount(name, role);
  const seeded = await postToTestControl(deployment, '/__test__/accounts', {
    role,
    name,
    email,
    language,
    timezone: 'Europe/Moscow',
  });
  return {
    name,
    email,
    password: stringField(seeded.body, 'password'),
    seededOn: { apiUrl: deployment.apiUrl, origin: deployment.origin },
  };
}

export async function seededSession(deployment: SeedingDeployment, account: TestAccount): Promise<SeededSession> {
  const answer = await postToTestControl(deployment, '/__test__/session', { email: account.email });
  return answer.session;
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

export async function pageLoaded(page: Page): Promise<void> {
  await expect(page.locator('main [aria-busy="true"]')).toHaveCount(0);
}

export async function pageSettled(page: Page): Promise<void> {
  await page.locator('main article h1').waitFor();
  await pageLoaded(page);
  await page.evaluate(async () => {
    await document.fonts.ready;
  });
}

export async function signInThroughTheApi(page: Page, account: TestAccount): Promise<void> {
  if (account.seededOn !== null) {
    const session = await seededSession(account.seededOn, account);
    await page.context().addCookies([
      {
        name: refreshCookieName,
        value: session.refreshCookie,
        url: account.seededOn.origin,
        expires: Math.floor(Date.now() / 1000) + session.lifetimeSeconds,
        httpOnly: true,
        secure: true,
        sameSite: 'Strict',
      },
    ]);
    return;
  }
  await page.goto('/favicon.svg');
  const status = await page.evaluate(async ({ email, password }) => {
    const response = await fetch('/api/v1/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password }),
    });
    await response.arrayBuffer();
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
