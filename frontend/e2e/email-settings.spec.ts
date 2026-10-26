import type { Page } from '@playwright/test';

import { type TestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { recordLazyChunkAddresses } from './observers.ts';
import { accessToken, callApi } from './tutoring.ts';
import { mintUnsubscribeToken } from './unsubscribe.ts';

async function enabledGroups(deployment: Deployment, account: TestAccount): Promise<string> {
  const answer = await callApi(
    deployment,
    await accessToken(deployment, account),
    'GET',
    '/users/me/email-preferences',
  );
  expect(answer.status).toBe(200);
  return JSON.stringify(answer.body);
}

const tutorGroupsOn =
  '{"groups":[{"group":"lesson_reminders","enabled":true},{"group":"submissions","enabled":true},{"group":"message_digest","enabled":true}]}';
const submissionsOff =
  '{"groups":[{"group":"lesson_reminders","enabled":true},{"group":"submissions","enabled":false},{"group":"message_digest","enabled":true}]}';

function recordAddresses(page: Page): string[] {
  const addresses: string[] = [];
  page.on('framenavigated', (frame) => {
    if (frame === page.mainFrame()) {
      addresses.push(frame.url());
    }
  });
  return addresses;
}

async function expectTokenNowhere(page: Page, token: string): Promise<void> {
  const stored = await page.evaluate(() => ({
    address: globalThis.location.href,
    historyState: JSON.stringify(globalThis.history.state),
    local: Object.values(globalThis.localStorage).join(' '),
    session: Object.values(globalThis.sessionStorage).join(' '),
    cookies: document.cookie,
  }));
  expect(stored.address).not.toContain(token);
  expect(stored.historyState).not.toContain(token);
  expect(stored.local).not.toContain(token);
  expect(stored.session).not.toContain(token);
  expect(stored.cookies).not.toContain(token);
}

test("settings › email switches show the role's groups and save", async ({ page, deployment }) => {
  const account = await seedVerifiedUser(deployment, 'Настройки писем');
  await signInThroughTheApi(page, account);
  await page.goto('/settings/notifications');
  const switches = page.getByRole('switch');
  const submissions = page.getByRole('switch', { name: 'Сданные работы' });
  await submissions.waitFor();

  await submissions.focus();
  await page.keyboard.press('Space');

  await expect(switches).toHaveCount(3);
  await expect(switches.nth(0)).toHaveAccessibleName('Напоминания об уроках');
  await expect(switches.nth(1)).toHaveAccessibleName('Сданные работы');
  await expect(switches.nth(2)).toHaveAccessibleName('Непрочитанные сообщения');
  await expect(page.getByText('Обязательные письма отключить нельзя', { exact: false })).toBeVisible();
  await expect(submissions).toHaveAttribute('aria-checked', 'false');
  await expect(submissions).toHaveAttribute('aria-busy', 'false');
  await expect(switches.nth(0)).toHaveAttribute('aria-checked', 'true');
  await expect(submissions).toBeFocused();
  expect(await enabledGroups(deployment, account)).toBe(submissionsOff);
});

test('unsubscribe › the page names the group and changes nothing until confirmed', async ({ page, deployment }) => {
  const account = await seedVerifiedUser(deployment, 'Отписка');
  const token = await mintUnsubscribeToken(deployment, account, 'submissions');
  const confirm = page.getByRole('button', { name: 'Turn off these emails' });
  const result = page.getByRole('status').filter({ hasText: 'These emails are turned off' });

  await page.goto(`/unsubscribe?token=${token}`);
  await confirm.waitFor();
  const groupLineBeforeConfirming = await page.getByText('Emails: Submitted work').count();
  const preferencesBeforeConfirming = await enabledGroups(deployment, account);
  await confirm.click();
  await result.waitFor();

  expect(groupLineBeforeConfirming).toBe(1);
  expect(preferencesBeforeConfirming).toBe(tutorGroupsOn);
  await expect(result).toBeFocused();
  await expect(confirm).toHaveCount(0);
  await expect(page.getByText('Emails: Submitted work')).toBeVisible();
  await expect(page.getByRole('link', { name: 'All email settings' })).toHaveAttribute(
    'href',
    '/settings/notifications',
  );
  expect(await enabledGroups(deployment, account)).toBe(submissionsOff);
});

test('unsubscribe › the token leaves the address and never appears in next', async ({ page, deployment }) => {
  const account = await seedVerifiedUser(deployment, 'Адрес отписки');
  const token = await mintUnsubscribeToken(deployment, account, 'submissions');
  const addresses = recordAddresses(page);
  const chunkAddresses = await recordLazyChunkAddresses(page, deployment.origin);
  const foreignRequests: string[] = [];
  page.on('request', (request) => {
    if (new URL(request.url()).origin !== deployment.origin) {
      foreignRequests.push(request.url());
    }
  });

  const opened = await page.goto(`/unsubscribe?token=${token}`);
  await page.getByRole('button', { name: 'Turn off these emails' }).click();
  await page.getByRole('status').filter({ hasText: 'These emails are turned off' }).waitFor();
  await page.getByRole('link', { name: 'All email settings' }).click();
  await page.getByLabel('Email').fill(account.email);
  await page.getByLabel('Password', { exact: true }).fill(account.password);
  await page.getByRole('button', { name: 'Sign in' }).click();

  await expect(page).toHaveURL('/settings/notifications');
  await expect(page.getByRole('switch', { name: 'Сданные работы' })).toHaveAttribute('aria-checked', 'false');
  expect(opened?.headers()['referrer-policy']).toBe('no-referrer');
  await expectTokenNowhere(page, token);
  const afterRemoval = addresses.slice(addresses.findIndex((address) => !address.includes(token)));
  expect(afterRemoval.map((address) => new URL(address).pathname + new URL(address).search)).toContain('/unsubscribe');
  expect(afterRemoval.filter((address) => address.includes(token))).toEqual([]);
  expect(addresses.map((address) => new URL(address).searchParams.get('next'))).toContain('/settings/notifications');
  expect(addresses.filter((address) => (new URL(address).searchParams.get('next') ?? '').includes(token))).toEqual([]);
  expect(chunkAddresses).not.toEqual([]);
  expect(chunkAddresses.filter((address) => address.includes(token))).toEqual([]);
  expect(foreignRequests).toEqual([]);
});

test('unsubscribe › an invalid link shows one message', async ({ page, context, diagnostics }) => {
  diagnostics.expectFailure({ method: 'GET', path: '/api/v1/unsubscribe/preview?token=v1.forged.link', status: 422 });
  const withoutToken = await context.newPage();

  await page.goto('/unsubscribe?token=v1.forged.link');
  await withoutToken.goto('/unsubscribe');

  for (const shown of [page, withoutToken]) {
    const alert = shown.getByRole('alert');
    await expect(alert).toHaveText('This link is no longer valid');
    await expect(alert).toBeFocused();
    await expect(shown.getByRole('button', { name: 'Turn off these emails' })).toHaveCount(0);
    await expect(shown.getByRole('link', { name: 'All email settings' })).toHaveAttribute(
      'href',
      '/settings/notifications',
    );
  }
});
