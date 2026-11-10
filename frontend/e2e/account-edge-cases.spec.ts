import type { BrowserContext, Locator, Page } from '@playwright/test';

import type { ObservedBrowser } from './browser-diagnostics.ts';

import { type TestAccount, pageSettled, signInThroughTheApi } from './accounts.ts';
import {
  accountId,
  trafficAfterReady,
  expectRefetchedAfterTheSocketIsReady,
  expectReadyForNavigation,
  todayQueryPaths,
  waitForSockets,
} from './realtime.ts';
import { accessToken, callApi } from './tutoring.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';

test.use({ locale: 'ru-RU' });

const onePixelPng = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64',
);

async function freshContext(browser: ObservedBrowser, deployment: Deployment): Promise<BrowserContext> {
  return browser.newContext({ baseURL: deployment.origin, ignoreHTTPSErrors: true, locale: 'ru-RU' });
}

async function signedInAt(page: Page, account: TestAccount, address: string): Promise<void> {
  await signInThroughTheApi(page, account);
  await page.goto(address);
  await expect(page.getByRole('navigation', { name: 'Основная навигация' })).toBeVisible();
}

async function apiSignIn(page: Page, account: TestAccount, password: string): Promise<number> {
  return page.evaluate(
    async ({ email, secret }) => {
      const response = await fetch('/api/v1/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password: secret }),
      });
      await response.arrayBuffer();
      return response.status;
    },
    { email: account.email, secret: password },
  );
}

function preferencesForm(page: Page): Locator {
  return page.getByRole('form', { name: 'Язык, часовой пояс и тема' });
}

async function savePreferences(page: Page): Promise<void> {
  await preferencesForm(page).getByRole('button', { name: 'Сохранить' }).click();
  await expect(preferencesForm(page).getByRole('status')).toHaveText('Сохранено.');
}

function passwordForm(page: Page): Locator {
  return page.getByRole('form', { name: 'Сменить пароль' });
}

function deletionDialog(page: Page): Locator {
  return page.getByRole('dialog', { name: 'Удалить аккаунт?' });
}

function accountActions(page: Page): Locator {
  return page.getByRole('main').getByRole('button', { name: 'Действия с аккаунтом' });
}

async function chooseAccountAction(page: Page, action: string): Promise<void> {
  await accountActions(page).click();
  await page.getByRole('main').getByRole('button', { name: action, exact: true }).click();
}

async function openDeletionDialog(page: Page): Promise<void> {
  await chooseAccountAction(page, 'Удалить аккаунт');
  await expect(deletionDialog(page)).toBeVisible();
}

function signOutDialog(page: Page): Locator {
  return page.getByRole('dialog', { name: 'Выйти на всех устройствах?' });
}

async function openSignOutDialog(page: Page): Promise<void> {
  await chooseAccountAction(page, 'Выйти на всех устройствах');
  await expect(signOutDialog(page)).toBeVisible();
}

function passwordOpener(page: Page): Locator {
  return page.getByRole('main').getByRole('button', { name: 'Сменить пароль' });
}

async function openPasswordDialog(page: Page): Promise<void> {
  await passwordOpener(page).click();
  await expect(passwordForm(page)).toBeVisible();
}

async function storedState(
  page: Page,
): Promise<{ readonly local: string[]; readonly session: number; readonly history: string }> {
  return page.evaluate(() => ({
    local: Object.keys(globalThis.localStorage).sort(),
    session: globalThis.sessionStorage.length,
    history: JSON.stringify(globalThis.history.state ?? null),
  }));
}

const knownPreferenceKeys = ['ovc.language', 'ovc.sidebar', 'ovc.theme'];

async function confirmDeletion(page: Page, password: string): Promise<void> {
  await deletionDialog(page).getByLabel('Текущий пароль').fill(password);
  await deletionDialog(page).getByRole('button', { name: 'Удалить навсегда' }).click();
}

async function countSchemeChanges(page: Page): Promise<void> {
  await page.evaluate(() => {
    window.ovcSchemeChanges = 0;
    globalThis.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
      window.ovcSchemeChanges = Number(window.ovcSchemeChanges) + 1;
    });
  });
}

async function switchScheme(page: Page, colorScheme: 'light' | 'dark', changesSoFar: number): Promise<void> {
  await page.emulateMedia({ colorScheme });
  await expect.poll(() => page.evaluate(() => Number(window.ovcSchemeChanges))).toBe(changesSoFar + 1);
}

test.describe('account edge cases', () => {
  const heldRequests: (() => Promise<void>)[] = [];

  test.afterEach(async () => {
    for (const releaseHeld of heldRequests.splice(0)) {
      await releaseHeld();
    }
  });

  test('FR-1.3 › a tutor opening My tutors sees that the page is not found and no code form', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await signInThroughTheApi(page, pavel);

    await page.goto('/tutors');

    await expect(page.getByRole('heading', { level: 1, name: 'Page not found' })).toBeVisible();
    await expect(page.getByRole('form', { name: 'Tutor code' })).toHaveCount(0);
  });

  test('signing out everywhere ends the sessions of other browsers', async ({
    page,
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor with two browsers');
    const elsewhere = await freshContext(browser, deployment);
    const otherPage = await elsewhere.newPage();
    const otherDiagnostics = await diagnostics.watch(otherPage);
    await signInThroughTheApi(otherPage, account);
    const otherReady = trafficAfterReady(otherPage, todayQueryPaths);
    await otherReady.initialize();
    await otherPage.goto('/today');
    await expect(otherPage.getByRole('navigation', { name: 'Основная навигация' })).toBeVisible();
    await expectReadyForNavigation(otherPage, otherReady);
    const traffic = trafficAfterReady(page, ['/api/v1/channels/unread']);
    await traffic.initialize();
    await signedInAt(page, account, '/settings/account');
    await expectRefetchedAfterTheSocketIsReady(traffic);
    otherDiagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/refresh', status: 401 });
    await openSignOutDialog(page);
    await signOutDialog(page).getByRole('button', { name: 'Выйти на всех устройствах' }).click();
    await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();

    await expect(otherPage.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
    await otherDiagnostics.finishHttpPhase();
    await pageSettled(otherPage);
    await otherPage.reload();

    await expect(otherPage.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
    await diagnostics.closeContext(elsewhere);
  });

  test('changing the password keeps this browser signed in and only the new password signs in', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/login', status: 401 });
    const account = await seedVerifiedUser(deployment, 'Tutor changing a password');
    const newPassword = `${account.password} changed`;
    await signedInAt(page, account, '/settings/account');
    await openPasswordDialog(page);
    await passwordForm(page).getByLabel('Текущий пароль').fill(account.password);
    await passwordForm(page).getByLabel('Новый пароль').fill(newPassword);
    await passwordForm(page).getByRole('button', { name: 'Сменить пароль' }).click();
    await expect(page.getByRole('main').getByText('Пароль изменён.')).toBeFocused();
    await expect(passwordForm(page)).toHaveCount(0);
    await expect(page).toHaveURL('/settings/account');
    const stored = await storedState(page);
    expect(stored.history).not.toContain(account.password);
    expect(stored.history).not.toContain(newPassword);
    expect(knownPreferenceKeys).toEqual(expect.arrayContaining(stored.local));
    expect(stored.session).toBe(0);

    await page.reload();

    await expect(page.getByRole('heading', { level: 1, name: 'Аккаунт' })).toBeVisible();
    expect(await apiSignIn(page, account, account.password)).toBe(401);
    await diagnostics.finishHttpPhase();
    expect(await apiSignIn(page, account, newPassword)).toBe(200);
  });

  test('a wrong current password is shown under the field', async ({ page, deployment, diagnostics }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/password/change', status: 422 });
    const account = await seedVerifiedUser(deployment, 'Tutor mistyping a password');
    await signedInAt(page, account, '/settings/account');
    await openPasswordDialog(page);

    await passwordForm(page).getByLabel('Текущий пароль').fill(`${account.password} wrong`);
    await passwordForm(page).getByLabel('Новый пароль').fill(`${account.password} changed`);
    await passwordForm(page).getByRole('button', { name: 'Сменить пароль' }).click();

    await expect(passwordForm(page).getByRole('alert')).toHaveText('Проверьте выделенные поля.');
    await expect(passwordForm(page).getByLabel('Текущий пароль')).toHaveAttribute('aria-invalid', 'true');
  });

  for (const closing of ['Escape', 'Отмена'] as const) {
    test(`the password dialog leaves no password field behind when closed with ${closing}`, async ({
      page,
      deployment,
    }) => {
      const account = await seedVerifiedUser(deployment, 'Tutor closing the password dialog');
      await signedInAt(page, account, '/settings/account');
      await openPasswordDialog(page);
      await passwordForm(page).getByLabel('Текущий пароль').fill(account.password);
      await passwordForm(page).getByLabel('Новый пароль').fill(`${account.password} changed`);

      if (closing === 'Escape') {
        await page.keyboard.press('Escape');
      } else {
        await passwordForm(page).getByRole('button', { name: 'Отмена' }).click();
      }

      await expect(page.locator('input[type="password"]')).toHaveCount(0);
      await expect(passwordOpener(page)).toBeFocused();
      await expect(page).toHaveURL('/settings/account');
      const stored = await storedState(page);
      expect(stored.history).not.toContain(account.password);
      expect(stored.history).not.toContain(`${account.password} changed`);
      expect(knownPreferenceKeys).toEqual(expect.arrayContaining(stored.local));
      expect(stored.session).toBe(0);
    });

    test(`the delete dialog leaves no password field behind when closed with ${closing}`, async ({
      page,
      deployment,
    }) => {
      const account = await seedVerifiedUser(deployment, 'Tutor closing the delete dialog');
      await signedInAt(page, account, '/settings/account');
      await openDeletionDialog(page);
      await deletionDialog(page).getByLabel('Текущий пароль').fill(account.password);

      if (closing === 'Escape') {
        await page.keyboard.press('Escape');
      } else {
        await deletionDialog(page).getByRole('button', { name: 'Отмена' }).click();
      }

      await expect(page.locator('input[type="password"]')).toHaveCount(0);
      await expect(accountActions(page)).toBeFocused();
      await expect(page).toHaveURL('/settings/account');
      const stored = await storedState(page);
      expect(stored.history).not.toContain(account.password);
      expect(knownPreferenceKeys).toEqual(expect.arrayContaining(stored.local));
      expect(stored.session).toBe(0);
    });
  }

  test('the password dialog reopens empty on its first field', async ({ page, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor reopening the password dialog');
    await signedInAt(page, account, '/settings/account');
    await openPasswordDialog(page);
    await passwordForm(page).getByLabel('Текущий пароль').fill(account.password);
    await passwordForm(page).getByLabel('Новый пароль').fill(`${account.password} changed`);
    await page.keyboard.press('Escape');

    await openPasswordDialog(page);

    await expect(passwordForm(page).getByLabel('Текущий пароль')).toBeFocused();
    await expect(passwordForm(page).getByLabel('Текущий пароль')).toHaveValue('');
    await expect(passwordForm(page).getByLabel('Новый пароль')).toHaveValue('');
  });

  test('the password dialog does not outlive the session', async ({ page, deployment, diagnostics }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor signed out while typing');
    const user = await accountId(deployment, account);
    await signedInAt(page, account, '/settings/account');
    await waitForSockets(deployment, user, 1);
    await openPasswordDialog(page);
    await passwordForm(page).getByLabel('Текущий пароль').fill(account.password);
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/refresh', status: 401 });

    const signedOut = await callApi(deployment, await accessToken(deployment, account), 'POST', '/auth/logout-all');

    expect(signedOut.status).toBe(204);
    await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.locator('input[type="password"]')).toHaveCount(1);
    await expect(page.locator('input[type="password"]')).toHaveValue('');
  });

  test('sign out everywhere opens on Cancel', async ({ page, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor about to sign out everywhere');
    await signedInAt(page, account, '/settings/account');

    await openSignOutDialog(page);

    await expect(signOutDialog(page).getByRole('button', { name: 'Отмена' })).toBeFocused();
    await expect(signOutDialog(page)).toContainText('Все сессии, включая эту, будут завершены.');
  });

  test('while sign out everywhere runs, its dialog stays open and Account actions is unavailable', async ({
    page,
    deployment,
  }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor waiting for sign out');
    await signedInAt(page, account, '/settings/account');
    await openSignOutDialog(page);
    let release: () => void = () => undefined;
    const resumed = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route('**/api/v1/auth/logout-all', async (route) => {
      await resumed;
      await route.continue();
    });
    heldRequests.push(async () => {
      const signedOut = page.waitForResponse('**/api/v1/auth/logout-all');
      release();
      await signedOut;
      await page.unroute('**/api/v1/auth/logout-all');
    });

    await signOutDialog(page).getByRole('button', { name: 'Выйти на всех устройствах' }).click();
    await page.keyboard.press('Escape');

    await expect(signOutDialog(page)).toBeVisible();
    await expect(signOutDialog(page).getByRole('button', { name: 'Выйти на всех устройствах' })).toHaveAttribute(
      'aria-busy',
      'true',
    );
    await expect(signOutDialog(page).getByRole('button', { name: 'Отмена' })).toHaveAttribute('aria-disabled', 'true');
    await expect(accountActions(page)).toHaveAttribute('aria-disabled', 'true');
  });

  test('Cancel on sign out everywhere revokes nothing', async ({ page, observedBrowser: browser, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor changing their mind about signing out');
    const elsewhere = await freshContext(browser, deployment);
    const otherPage = await elsewhere.newPage();
    await signedInAt(otherPage, account, '/settings/account');
    await signedInAt(page, account, '/settings/account');
    const signOutRequests: string[] = [];
    page.on('request', (request) => {
      if (request.method() === 'POST' && request.url().endsWith('/api/v1/auth/logout-all')) {
        signOutRequests.push(request.url());
      }
    });
    await openSignOutDialog(page);

    await signOutDialog(page).getByRole('button', { name: 'Отмена' }).click();

    await expect(signOutDialog(page)).toHaveCount(0);
    await expect(accountActions(page)).toBeFocused();
    await otherPage.reload();
    await expect(otherPage.getByRole('heading', { level: 1, name: 'Аккаунт' })).toBeVisible();
    expect(signOutRequests).toEqual([]);
    await elsewhere.close();
  });

  test('the delete dialog opens on its empty password field', async ({ page, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor looking at deletion');
    await signedInAt(page, account, '/settings/account');

    await openDeletionDialog(page);

    await expect(deletionDialog(page).getByLabel('Текущий пароль')).toBeFocused();
    await expect(deletionDialog(page).getByLabel('Текущий пароль')).toHaveValue('');
  });

  test('deleting the account with a wrong password is refused under the field', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'DELETE', path: '/api/v1/users/me', status: 422 });
    const account = await seedVerifiedUser(deployment, 'Tutor hesitating');
    await signedInAt(page, account, '/settings/account');
    await openDeletionDialog(page);

    await confirmDeletion(page, `${account.password} wrong`);

    await expect(deletionDialog(page).getByRole('alert')).toHaveText('Проверьте выделенные поля.');
    await expect(deletionDialog(page).getByLabel('Текущий пароль')).toHaveAttribute('aria-invalid', 'true');
  });

  test('reopening the deletion dialog starts with an empty password', async ({ page, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor changing their mind');
    await signedInAt(page, account, '/settings/account');
    await openDeletionDialog(page);
    await deletionDialog(page).getByLabel('Текущий пароль').fill(account.password);
    await deletionDialog(page).getByRole('button', { name: 'Отмена' }).click();

    await openDeletionDialog(page);

    await expect(deletionDialog(page).getByLabel('Текущий пароль')).toHaveValue('');
  });

  test('the sixth wrong deletion password says when to try again', async ({ page, deployment, diagnostics }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor guessing');
    await signedInAt(page, account, '/settings/account');
    await openDeletionDialog(page);
    for (let attempt = 0; attempt < 5; attempt += 1) {
      diagnostics.expectFailure({ method: 'DELETE', path: '/api/v1/users/me', status: 422 });
      await confirmDeletion(page, `${account.password} wrong ${String(attempt)}`);
      await expect(deletionDialog(page).getByLabel('Текущий пароль')).toHaveAttribute('aria-invalid', 'true');
      await diagnostics.finishHttpPhase();
    }

    diagnostics.expectFailure({ method: 'DELETE', path: '/api/v1/users/me', status: 429 });
    await confirmDeletion(page, `${account.password} wrong again`);

    await expect(deletionDialog(page).getByRole('alert')).toHaveText(
      /^Слишком много попыток, попробуйте через \d+ (минуту|минуты|минут)\.$/,
    );
    await diagnostics.finishHttpPhase();
  });

  test('deleting the account signs out and the account no longer signs in', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/login', status: 401 });
    const account = await seedVerifiedUser(deployment, 'Tutor leaving for good');
    await signedInAt(page, account, '/settings/account');
    await openDeletionDialog(page);

    await confirmDeletion(page, account.password);

    await expect(page).toHaveURL('/');
    await expect(page.getByRole('status')).toHaveText('Аккаунт удалён.');
    expect(await apiSignIn(page, account, account.password)).toBe(401);
    await diagnostics.finishHttpPhase();
  });

  test('email language follows the account', async ({ page, deployment, control }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor switching to English');
    await control.advance(61);
    await signedInAt(page, account, '/settings/account');
    await preferencesForm(page).getByLabel('Язык', { exact: true }).selectOption('en');
    await preferencesForm(page).getByRole('button', { name: 'Сохранить' }).click();
    await expect(page.locator('html')).toHaveAttribute('lang', 'en');
    await page.goto('/forgot-password');
    await page.getByLabel('Email').fill(account.email);
    await page.getByRole('button', { name: 'Send link' }).click();
    await expect(page.getByRole('status')).toContainText(account.email);

    await control.tick();

    const subjects = (await control.mail(account.email)).map((message) => message.subject);
    expect(subjects).toContain('Reset your OVC password');
  });

  test('the timezone is saved', async ({ page, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor moving to Berlin');
    await signedInAt(page, account, '/settings/account');
    await preferencesForm(page).getByLabel('Часовой пояс').selectOption('Europe/Berlin');

    await savePreferences(page);

    await page.reload();
    await expect(preferencesForm(page).getByLabel('Часовой пояс')).toHaveValue('Europe/Berlin');
  });

  test('the system theme follows the colour scheme', async ({ page, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor at dusk');
    await page.emulateMedia({ colorScheme: 'light' });
    await signedInAt(page, account, '/settings/account');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    await countSchemeChanges(page);

    await switchScheme(page, 'dark', 0);

    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  });

  test('a fixed theme ignores the colour scheme', async ({ page, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor at night');
    await page.emulateMedia({ colorScheme: 'light' });
    await signedInAt(page, account, '/settings/account');
    await preferencesForm(page).getByLabel('Тема').selectOption('light');
    await savePreferences(page);
    await countSchemeChanges(page);

    await switchScheme(page, 'dark', 0);

    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    await expect(page.locator('html')).toHaveAttribute('data-theme-preference', 'light');
  });

  test('the subjects a tutor saved are shown after a reload', async ({ page, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor of two languages');
    await signedInAt(page, account, '/settings/profile');
    const details = page.getByRole('form', { name: 'Имя и описание' });
    await details.getByLabel('Предметы').fill('Английский, Немецкий');
    await details.getByRole('button', { name: 'Сохранить' }).click();
    await expect(details.getByRole('status')).toHaveText('Сохранено.');

    await page.reload();

    await expect(page.getByLabel('Предметы')).toHaveValue('Английский, Немецкий');
  });

  test('an uploaded avatar shown from its avatar address is removed', async ({ page, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor with a photo');
    await signedInAt(page, account, '/settings/profile');
    const photo = page.getByRole('img', { name: 'Фото: Tutor with a photo' });
    await page
      .getByLabel('Загрузить фото')
      .setInputFiles({ name: 'photo.png', mimeType: 'image/png', buffer: onePixelPng });
    await expect(photo).toHaveAttribute('src', /^\/api\/v1\/avatars\/[0-9a-f-]{36}$/);

    await page.getByRole('button', { name: 'Удалить фото' }).click();

    await expect(page.getByText('Фото не загружено.')).toBeVisible();
    await expect(photo).toHaveCount(0);
  });

  test('a file that is not an image is refused as an avatar', async ({ page, deployment, diagnostics }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/uploads?context=avatar', status: 415 });
    const account = await seedVerifiedUser(deployment, 'Tutor with a text file');
    await signedInAt(page, account, '/settings/profile');

    await page
      .getByLabel('Загрузить фото')
      .setInputFiles({ name: 'notes.png', mimeType: 'image/png', buffer: Buffer.from('not an image') });

    await expect(page.getByRole('alert')).toHaveText('Такой тип файла не поддерживается.');
    await expect(page.getByText('Фото не загружено.')).toBeVisible();
  });

  test('stored keys are known preferences', async ({ page, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor checking storage');
    await signedInAt(page, account, '/settings/account');
    await preferencesForm(page).getByLabel('Тема').selectOption('dark');
    await savePreferences(page);
    await page.getByRole('button', { name: 'Скрыть боковую панель' }).click();

    const stored = await page.evaluate(() => ({
      local: Object.keys(globalThis.localStorage).sort(),
      session: globalThis.sessionStorage.length,
      cookie: globalThis.document.cookie,
    }));

    expect(['ovc.language', 'ovc.sidebar', 'ovc.theme']).toEqual(expect.arrayContaining(stored.local));
    expect(stored.local).toContain('ovc.theme');
    expect(stored.session).toBe(0);
    expect(stored.cookie).toBe('');
  });
});

test.describe('Maria and her student Anna, shared by the tests below', () => {
  test.describe.configure({ mode: 'default' });

  test('FR-1.3 › a student opening Today sees that the page is not found inside the signed-in frame', async ({
    page,
    mariaAndAnna: { anna },
  }) => {
    await signInThroughTheApi(page, anna);

    await page.goto('/today');

    await expect(page.getByRole('navigation', { name: 'Основная навигация' })).toBeVisible();
    await expect(page.getByRole('heading', { level: 1, name: 'Страница не найдена' })).toBeVisible();
  });

  test('FR-1.3 › a student opening Students sees that the page is not found and no join requests', async ({
    page,
    mariaAndAnna: { anna },
  }) => {
    await signInThroughTheApi(page, anna);

    await page.goto('/students');

    await expect(page.getByRole('heading', { level: 1, name: 'Страница не найдена' })).toBeVisible();
    await expect(page.getByRole('region', { name: 'Заявки на занятия' })).toHaveCount(0);
  });

  test('the deletion dialog names the consequences for a tutor and for a student', async ({
    page,
    observedBrowser: browser,
    deployment,
    mariaAndAnna: { maria, anna },
  }) => {
    const annaContext = await freshContext(browser, deployment);
    const annaPage = await annaContext.newPage();
    await signedInAt(page, maria, '/settings/account');
    await signedInAt(annaPage, anna, '/settings/account');

    await openDeletionDialog(page);
    await openDeletionDialog(annaPage);

    await expect(deletionDialog(page)).toContainText(
      'Занятия со всеми учениками завершатся, и ученики получат письмо; ссылки-приглашения и код репетитора перестанут работать, заявки к вам истекут, а группы уйдут в архив.',
    );
    await expect(deletionDialog(annaPage)).toContainText(
      'Занятия со всеми репетиторами завершатся, вы выйдете из их групп, а ваши запросы репетиторам истекут.',
    );
    await annaContext.close();
  });
});
