import type { Browser, BrowserContext, Locator, Page } from '@playwright/test';

import { type TestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { accessToken, callApi, createCard, invitationToken, seedActiveStudent, tutorCode } from './tutoring.ts';

test.use({ locale: 'ru-RU' });

const onePixelPng = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64',
);

async function freshContext(browser: Browser, deployment: Deployment): Promise<BrowserContext> {
  return browser.newContext({ baseURL: deployment.origin, ignoreHTTPSErrors: true, locale: 'ru-RU' });
}

async function signedInAt(page: Page, account: TestAccount, address: string): Promise<void> {
  await signInThroughTheApi(page, account);
  await page.goto(address);
  await expect(page.getByRole('navigation', { name: 'Основная навигация' })).toBeVisible();
}

async function apiSignIn(page: Page, account: TestAccount, password: string): Promise<number> {
  return page.evaluate(
    async ({ email, secret }) =>
      (
        await fetch('/api/v1/auth/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ email, password: secret }),
        })
      ).status,
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

async function openDeletionDialog(page: Page): Promise<void> {
  await page.getByRole('main').getByRole('button', { name: 'Удалить аккаунт' }).click();
  await expect(deletionDialog(page)).toBeVisible();
}

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
  test('FR-1.3 role mismatch', async ({ page, browser, deployment, control }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student: anna } = await seedActiveStudent(deployment, control, maria, 'Анна', 'ru');
    const pavelToken = await accessToken(deployment, pavel);
    const card = await createCard(deployment, maria, 'Pavel', pavel.email);

    const codeEntry = await callApi(deployment, pavelToken, 'POST', '/join-requests', {
      code: await tutorCode(deployment, maria),
    });
    const acceptance = await callApi(
      deployment,
      pavelToken,
      'POST',
      `/invitations/${encodeURIComponent(invitationToken(card.link))}/accept`,
      {},
    );
    const tutorList = await callApi(deployment, pavelToken, 'GET', '/tutors');
    const requestList = await callApi(deployment, await accessToken(deployment, anna), 'GET', '/join-requests');

    expect(codeEntry.status).toBe(409);
    expect(codeEntry.body).toMatchObject({ error: { code: 'role_mismatch' } });
    expect(acceptance.status).toBe(409);
    expect(acceptance.body).toMatchObject({ error: { code: 'role_mismatch' } });
    expect(tutorList.status).toBe(404);
    expect(requestList.status).toBe(404);
    await signInThroughTheApi(page, pavel);
    await page.goto('/tutors');
    await expect(page.getByRole('heading', { level: 1, name: 'Page not found' })).toBeVisible();
    await expect(page.getByRole('form', { name: 'Tutor code' })).toHaveCount(0);
    const annaContext = await freshContext(browser, deployment);
    const annaPage = await annaContext.newPage();
    await signedInAt(annaPage, anna, '/today');
    await expect(annaPage.getByRole('heading', { level: 1, name: 'Страница не найдена' })).toBeVisible();
    await annaPage.goto('/students');
    await expect(annaPage.getByRole('heading', { level: 1, name: 'Страница не найдена' })).toBeVisible();
    await expect(annaPage.getByRole('region', { name: 'Заявки на занятия' })).toHaveCount(0);
    await annaContext.close();
  });

  test('signing out everywhere ends the sessions of other browsers', async ({ page, browser, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor with two browsers');
    const elsewhere = await freshContext(browser, deployment);
    const otherPage = await elsewhere.newPage();
    await signedInAt(otherPage, account, '/today');
    await signedInAt(page, account, '/settings/account');

    await page.getByRole('main').getByRole('button', { name: 'Выйти на всех устройствах' }).click();

    await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
    await otherPage.reload();
    await expect(otherPage.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
    await elsewhere.close();
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

    await passwordForm(page).getByLabel('Текущий пароль').fill(account.password);
    await passwordForm(page).getByLabel('Новый пароль').fill(newPassword);
    await passwordForm(page).getByRole('button', { name: 'Сменить пароль' }).click();

    await expect(passwordForm(page).getByRole('status')).toHaveText('Пароль изменён.');
    await page.reload();
    await expect(page.getByRole('heading', { level: 1, name: 'Аккаунт' })).toBeVisible();
    expect(await apiSignIn(page, account, account.password)).toBe(401);
    expect(await apiSignIn(page, account, newPassword)).toBe(200);
  });

  test('a wrong current password is shown under the field', async ({ page, deployment, diagnostics }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/password/change', status: 422 });
    const account = await seedVerifiedUser(deployment, 'Tutor mistyping a password');
    await signedInAt(page, account, '/settings/account');

    await passwordForm(page).getByLabel('Текущий пароль').fill(`${account.password} wrong`);
    await passwordForm(page).getByLabel('Новый пароль').fill(`${account.password} changed`);
    await passwordForm(page).getByRole('button', { name: 'Сменить пароль' }).click();

    await expect(passwordForm(page).getByRole('alert')).toHaveText('Проверьте выделенные поля.');
    await expect(passwordForm(page).getByLabel('Текущий пароль')).toHaveAttribute('aria-invalid', 'true');
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
    for (let attempt = 0; attempt < 5; attempt += 1) {
      diagnostics.expectFailure({ method: 'DELETE', path: '/api/v1/users/me', status: 422 });
    }
    diagnostics.expectFailure({ method: 'DELETE', path: '/api/v1/users/me', status: 429 });
    const account = await seedVerifiedUser(deployment, 'Tutor guessing');
    await signedInAt(page, account, '/settings/account');
    await openDeletionDialog(page);
    for (let attempt = 0; attempt < 5; attempt += 1) {
      await confirmDeletion(page, `${account.password} wrong ${String(attempt)}`);
      await expect(deletionDialog(page).getByLabel('Текущий пароль')).toHaveAttribute('aria-invalid', 'true');
    }

    await confirmDeletion(page, `${account.password} wrong again`);

    await expect(deletionDialog(page).getByRole('alert')).toHaveText(
      /^Слишком много попыток, попробуйте через \d+ (минуту|минуты|минут)\.$/,
    );
  });

  test('the deletion dialog names the consequences for a tutor and for a student', async ({
    page,
    browser,
    deployment,
    control,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const { student: anna } = await seedActiveStudent(deployment, control, maria, 'Анна', 'ru');
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

  test('a tutor saves subjects', async ({ page, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Tutor of two languages');
    await signedInAt(page, account, '/settings/profile');
    const details = page.getByRole('form', { name: 'Имя и описание' });
    await details.getByLabel('Предметы').fill('Английский, Немецкий');

    await details.getByRole('button', { name: 'Сохранить' }).click();

    await expect(details.getByRole('status')).toHaveText('Сохранено.');
    await page.reload();
    await expect(page.getByLabel('Предметы')).toHaveValue('Английский, Немецкий');
  });

  test('an avatar is uploaded, shown and removed', async ({ page, deployment }) => {
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
