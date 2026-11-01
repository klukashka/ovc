import type { Page } from '@playwright/test';

import { type TestAccount, newTestAccount, signInThroughTheApi } from './accounts.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { trafficAfterReady, expectRefetchedAfterTheSocketIsReady } from './realtime.ts';

test.use({ locale: 'ru-RU' });

const onePixelPng = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64',
);

async function signedInAt(page: Page, account: TestAccount, address: string): Promise<void> {
  await signInThroughTheApi(page, account);
  await page.goto(address);
}

function signUpLanguage(page: Page): ReturnType<Page['locator']> {
  return page.locator('select[name="language"]');
}

test('after sign-up the result heading takes focus', async ({ page }) => {
  const account = newTestAccount('Фокус после регистрации');
  await page.goto('/signup/tutor');
  await page.getByLabel('Имя').fill(account.name);
  await page.getByLabel('Email').fill(account.email);
  await page.getByLabel('Пароль', { exact: true }).fill(account.password);
  await page.getByLabel(/Я соглашаюсь на обработку персональных данных/).check();

  await page.getByRole('button', { name: 'Зарегистрироваться' }).click();

  await expect(page.getByRole('heading', { level: 1, name: 'Проверьте почту' })).toBeFocused();
});

test('after asking for a reset link the notice takes focus', async ({ page }) => {
  await page.goto('/forgot-password');
  await page.getByLabel('Email').fill('nobody@example.test');

  await page.getByRole('button', { name: 'Отправить ссылку' }).click();

  await expect(page.getByRole('status')).toBeFocused();
});

test('the skip link is the first stop and moves focus to the main content', async ({ page, browserName }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();

  await page.keyboard.press(browserName === 'webkit' ? 'Alt+Tab' : 'Tab');
  const skipLink = page.getByRole('link', { name: 'Перейти к содержимому' });
  const firstStop = await skipLink.evaluate((element) => element === document.activeElement);
  await page.keyboard.press('Enter');

  expect(firstStop).toBe(true);
  await expect(page.locator('main')).toBeFocused();
});

test('signing out from an unknown address returns to sign-in', async ({ page, tutor }) => {
  const traffic = trafficAfterReady(page, ['/api/v1/notifications', '/api/v1/channels/unread']);
  await signedInAt(page, tutor, '/no/such/page');
  await expect(page.getByRole('heading', { level: 1, name: 'Страница не найдена' })).toBeVisible();
  await expectRefetchedAfterTheSocketIsReady(traffic);

  await page.getByRole('button', { name: /Меню аккаунта/ }).click();
  await page.getByRole('button', { name: 'Выйти' }).click();

  await expect(page).toHaveURL('/');
  await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
});

test('the sign-up language follows the interface while the person has not chosen one', async ({ page }) => {
  await page.goto('/signup/tutor');
  await expect(signUpLanguage(page)).toHaveValue('ru');

  await page.getByRole('button', { name: 'English' }).click();

  await expect(signUpLanguage(page)).toHaveValue('en');
});

test('the sign-up language stops following the interface once the person chooses one', async ({ page }) => {
  await page.goto('/signup/tutor');
  await page.getByRole('button', { name: 'English' }).click();
  await expect(signUpLanguage(page)).toHaveValue('en');
  await signUpLanguage(page).selectOption('ru');

  await page.getByRole('button', { name: 'Русский' }).click();
  await page.getByRole('button', { name: 'English' }).click();

  await expect(signUpLanguage(page)).toHaveValue('ru');
});

test('the language switch of a signed-in person is saved to the account', async ({ page, deployment }) => {
  const account = await seedVerifiedUser(deployment, 'Язык в аккаунте');
  await signedInAt(page, account, '/privacy');
  const saved = page.waitForResponse(
    (response) => new URL(response.url()).pathname === '/api/v1/users/me' && response.request().method() === 'PATCH',
  );

  await page.getByRole('button', { name: 'English' }).click();
  const answer = await saved;
  await page.reload();

  expect(answer.status()).toBe(200);
  await expect(page.getByRole('button', { name: 'English' })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');
});

test('removing the photo moves focus to its notice', async ({ page, deployment }) => {
  const account = await seedVerifiedUser(deployment, 'Фокус после удаления фото');
  await signedInAt(page, account, '/settings/profile');
  await page
    .getByLabel('Загрузить фото')
    .setInputFiles({ name: 'photo.png', mimeType: 'image/png', buffer: onePixelPng });
  await expect(page.getByRole('img', { name: 'Фото: Фокус после удаления фото' })).toBeVisible();

  await page.getByRole('button', { name: 'Удалить фото' }).click();

  await expect(page.getByText('Фото не загружено.')).toBeVisible();
  await expect(page.getByRole('status').filter({ hasText: 'Фото обновлено.' })).toBeFocused();
});

test('a tutor sees the subjects normalised after saving them', async ({ page, deployment }) => {
  const account = await seedVerifiedUser(deployment, 'Предметы без мусора');
  await signedInAt(page, account, '/settings/profile');
  const details = page.getByRole('form', { name: 'Имя и описание' });
  const subjects = details.getByLabel('Предметы');
  await subjects.fill(' Английский ,, Немецкий ');

  await subjects.press('Enter');

  await expect(details.getByRole('status')).toHaveText('Сохранено.');
  await expect(subjects).toHaveValue('Английский, Немецкий');
  await expect(subjects).toBeFocused();
});

test('an avatar saved while another field has focus leaves the focus there', async ({ page, deployment }) => {
  const account = await seedVerifiedUser(deployment, 'Фокус остаётся в поле');
  await signedInAt(page, account, '/settings/profile');
  const name = page.getByRole('form', { name: 'Имя и описание' }).getByLabel('Имя');
  await name.focus();

  await page
    .getByLabel('Загрузить фото')
    .setInputFiles({ name: 'photo.png', mimeType: 'image/png', buffer: onePixelPng });

  await expect(page.getByRole('img', { name: 'Фото: Фокус остаётся в поле' })).toBeVisible();
  await expect(page.getByText('Фото обновлено.')).toBeVisible();
  await expect(name).toBeFocused();
});

test('Escape on the deletion dialog returns focus to the button that opened it', async ({ page, tutor }) => {
  await signedInAt(page, tutor, '/settings/account');
  const opener = page.getByRole('button', { name: 'Удалить аккаунт' });
  await opener.click();
  await expect(page.getByRole('dialog', { name: 'Удалить аккаунт?' })).toBeVisible();

  await page.keyboard.press('Escape');

  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(opener).toBeFocused();
});

test('the deletion dialog opens on its password field', async ({ page, tutor }) => {
  await signedInAt(page, tutor, '/settings/account');

  await page.getByRole('button', { name: 'Удалить аккаунт' }).click();

  const dialog = page.getByRole('dialog', { name: 'Удалить аккаунт?' });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByLabel('Текущий пароль')).toBeFocused();
});

test('a busy preferences save keeps focus on its button', async ({ page, deployment }) => {
  const account = await seedVerifiedUser(deployment, 'Фокус на кнопке сохранения');
  await signedInAt(page, account, '/settings/account');
  const preferences = page.getByRole('form', { name: 'Язык, часовой пояс и тема' });
  const save = preferences.getByRole('button', { name: 'Сохранить' });
  await save.focus();

  await page.keyboard.press('Enter');

  await expect(preferences.getByRole('status')).toHaveText('Сохранено.');
  await expect(save).toBeFocused();
});

test('a busy password change keeps focus on its button', async ({ page, deployment }) => {
  const account = await seedVerifiedUser(deployment, 'Фокус на кнопке смены пароля');
  await signedInAt(page, account, '/settings/account');
  const password = page.getByRole('form', { name: 'Сменить пароль' });
  const change = password.getByRole('button', { name: 'Сменить пароль' });
  await password.getByLabel('Текущий пароль').fill(account.password);
  await password.getByLabel('Новый пароль').fill(`${account.password} changed`);
  await change.focus();

  await page.keyboard.press('Enter');

  await expect(password.getByRole('status')).toHaveText('Пароль изменён.');
  await expect(change).toBeFocused();
  await expect(password.getByLabel('Текущий пароль')).toHaveValue('');
});

test('the resend button keeps focus while it waits', async ({ page }) => {
  const account = newTestAccount('Фокус на повторной отправке');
  await page.clock.install();
  await page.goto('/signup/tutor');
  await page.getByLabel('Имя').fill(account.name);
  await page.getByLabel('Email').fill(account.email);
  await page.getByLabel('Пароль', { exact: true }).fill(account.password);
  await page.getByLabel(/Я соглашаюсь на обработку персональных данных/).check();
  await page.getByRole('button', { name: 'Зарегистрироваться' }).click();
  const resend = page.getByRole('button', { name: 'Отправить ещё раз' });
  await expect(resend).toBeDisabled();
  await page.clock.runFor('01:01');
  await expect(resend).toBeEnabled();

  await resend.focus();
  await page.keyboard.press('Enter');

  await expect(page.getByRole('status')).toHaveText('Письмо отправлено ещё раз.');
  await expect(resend).toBeDisabled();
  await expect(resend).toBeFocused();
});
