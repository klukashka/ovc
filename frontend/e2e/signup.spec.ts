import type { Browser, BrowserContext, Page } from '@playwright/test';

import {
  type SentMailLinks,
  type TestAccount,
  newTestAccount,
  registerTutor,
  signInThroughTheApi,
} from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { expect, seedVerifiedUser, test } from './fixtures.ts';
import { recordLazyChunkAddresses } from './observers.ts';

test.use({ locale: 'ru-RU' });

function emailedLink(messages: readonly SentMailLinks[], path: string): string {
  const link = messages
    .flatMap((message) => message.links)
    .filter((candidate) => new URL(candidate).pathname === path)
    .at(-1);
  if (link === undefined) {
    throw new Error(`no emailed link to ${path}`);
  }
  return link;
}

async function freshContext(browser: Browser, deployment: Deployment): Promise<BrowserContext> {
  return browser.newContext({ baseURL: deployment.origin, ignoreHTTPSErrors: true, locale: 'ru-RU' });
}

async function signUpThroughTheForm(page: Page, account: TestAccount): Promise<void> {
  await page.goto('/signup');
  await page.getByRole('link', { name: /Я преподаю/ }).click();
  await page.getByLabel('Имя').fill(account.name);
  await page.getByLabel('Email').fill(account.email);
  await page.getByLabel('Пароль', { exact: true }).fill(account.password);
  await page.getByLabel(/Я соглашаюсь на обработку персональных данных/).check();
  await page.getByRole('button', { name: 'Зарегистрироваться' }).click();
}

async function expectSignedInToToday(page: Page): Promise<void> {
  await expect(page).toHaveURL('/today');
  await expect(page.getByRole('heading', { level: 1, name: 'Сегодня' })).toBeVisible();
}

test('pilot-1 › steps 1–3 sign up, confirm in a fresh browser with a new password, then save the bio', async ({
  page,
  browser,
  deployment,
  control,
}) => {
  const maria = newTestAccount('Мария');
  await signUpThroughTheForm(page, maria);
  await expect(page.getByRole('heading', { level: 1, name: 'Проверьте почту' })).toBeVisible();
  await expect(page.getByText(maria.email)).toBeVisible();
  await control.tick();
  const link = emailedLink(await control.mail(maria.email), '/verify-email');
  const fresh = await freshContext(browser, deployment);
  const confirming = await fresh.newPage();

  await confirming.goto(link);
  await expect(confirming).toHaveURL('/verify-email');
  await confirming.getByLabel('Новый пароль').fill(maria.password);
  await confirming.getByRole('button', { name: 'Подтвердить и войти' }).click();

  await expectSignedInToToday(confirming);
  await expect(confirming.getByRole('region', { name: 'Подтверждение email' })).toHaveCount(0);
  await confirming.goto('/settings/profile');
  await confirming.getByLabel('О себе').fill('Английский для взрослых, разговорная практика.');
  await confirming.getByRole('button', { name: 'Сохранить' }).click();
  await expect(confirming.getByRole('status')).toHaveText('Сохранено.');
  await confirming.reload();
  await expect(confirming.getByLabel('О себе')).toHaveValue('Английский для взрослых, разговорная практика.');
  await fresh.close();
});

test('pilot-3 › step 1 seeded tutors are confirmed and sign in to Today', async ({ page, deployment }) => {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const anotherTutor = await seedVerifiedUser(deployment, 'Ольга');

  await signInThroughTheApi(page, maria);
  await page.goto('/today');
  await expectSignedInToToday(page);
  await signInThroughTheApi(page, anotherTutor);
  await page.goto('/today');

  await expectSignedInToToday(page);
  await expect(page.getByRole('region', { name: 'Подтверждение email' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Ольга, Меню аккаунта' })).toBeVisible();
});

test('the confirmation link opened in a signed-in browser asks for no password and ends other sessions', async ({
  page,
  browser,
  deployment,
  control,
}) => {
  const account = newTestAccount('Unverified tutor');
  await registerTutor(deployment, account, 'ru');
  await control.tick();
  const link = emailedLink(await control.mail(account.email), '/verify-email');
  await signInThroughTheApi(page, account);
  const elsewhere = await freshContext(browser, deployment);
  const otherPage = await elsewhere.newPage();
  await signInThroughTheApi(otherPage, account);
  await otherPage.goto('/today');
  await expect(otherPage.getByRole('region', { name: 'Подтверждение email' })).toBeVisible();
  const tab = await page.context().newPage();
  await tab.goto(link);
  await expectSignedInToToday(tab);
  await expect(tab.getByLabel('Новый пароль')).toHaveCount(0);
  await expect(tab.getByRole('region', { name: 'Подтверждение email' })).toHaveCount(0);

  await otherPage.reload();

  await expect(otherPage.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
  await elsewhere.close();
});

test('the resend button waits 60 seconds after sign-up', async ({ page }) => {
  await page.clock.install();
  await signUpThroughTheForm(page, newTestAccount('Patient tutor'));
  await expect(page.getByRole('button', { name: 'Отправить ещё раз' })).toBeDisabled();
  await expect(page.getByText(/^Можно отправить через \d+ с\.$/)).toBeVisible();

  await page.clock.runFor('01:01');
  await page.getByRole('button', { name: 'Отправить ещё раз' }).click();

  await expect(page.getByRole('status')).toHaveText('Письмо отправлено ещё раз.');
});

test('a resend starts the 60-second countdown again and it counts down each second', async ({ page }) => {
  await page.clock.install();
  await signUpThroughTheForm(page, newTestAccount('Repeating tutor'));
  await expect(page.getByText(/^Можно отправить через \d+ с\.$/)).toBeVisible();
  await page.clock.runFor('01:01');
  await page.getByRole('button', { name: 'Отправить ещё раз' }).click();
  await expect(page.getByRole('status')).toHaveText('Письмо отправлено ещё раз.');
  const restarted = page.getByText('Можно отправить через 60 с.');
  await expect(restarted).toBeVisible();

  await page.clock.runFor(1000);

  await expect(page.getByText('Можно отправить через 59 с.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Отправить ещё раз' })).toBeDisabled();
});

test('a signed-in unverified tutor resends from the confirmation page and is told when to try again', async ({
  page,
  deployment,
}) => {
  const account = newTestAccount('Hasty tutor');
  await registerTutor(deployment, account, 'ru');
  await signInThroughTheApi(page, account);
  await page.goto('/today');
  await page
    .getByRole('region', { name: 'Подтверждение email' })
    .getByRole('link', { name: 'Подтвердить email' })
    .click();

  await page.getByRole('button', { name: 'Отправить ещё раз' }).click();

  await expect(page.getByRole('heading', { level: 1, name: 'Проверьте почту' })).toBeVisible();
  await expect(page.getByText(account.email)).toBeVisible();
  await expect(page.getByRole('alert')).toHaveText(/^Слишком много попыток, попробуйте через 1 минуту\.$/);
});

test('an invalid confirmation link is refused without asking for a password', async ({ page }) => {
  await page.goto('/verify-email?token=not-a-real-token');

  await expect(page.getByRole('alert')).toHaveText('Ссылка недействительна или устарела.');
  await expect(page).toHaveURL('/verify-email');
  await expect(page.getByLabel('Новый пароль')).toHaveCount(0);
});

test('reloading a token page after the token was removed asks to open the link again', async ({ page }) => {
  await page.goto('/verify-email?token=not-a-real-token');
  await expect(page).toHaveURL('/verify-email');

  await page.reload();

  await expect(page.getByText('Откройте ссылку из письма ещё раз.')).toBeVisible();
});

test('a forgotten password is reset through the emailed link and the new password signs in', async ({
  page,
  deployment,
  control,
}) => {
  const account = await seedVerifiedUser(deployment, 'Forgetful tutor');
  await control.advance(61);
  await page.goto('/forgot-password');
  await page.getByLabel('Email').fill(account.email);
  await page.getByRole('button', { name: 'Отправить ссылку' }).click();
  await expect(page.getByRole('status')).toContainText(account.email);
  await control.tick();
  const link = emailedLink(await control.mail(account.email), '/reset-password');
  const newPassword = `${account.password} renewed`;
  await page.goto(link);
  await expect(page).toHaveURL('/reset-password');
  await page.getByLabel('Новый пароль').fill(newPassword);
  await page.getByRole('button', { name: 'Сохранить пароль' }).click();
  await expect(page.getByRole('status')).toHaveText('Пароль изменён. Войдите с новым паролем.');
  await expect(page.getByRole('status')).toBeFocused();

  await page.getByRole('main').getByRole('link', { name: 'Войти', exact: true }).click();
  await page.getByLabel('Email').fill(account.email);
  await page.getByLabel('Пароль', { exact: true }).fill(newPassword);
  await page.getByRole('button', { name: 'Войти', exact: true }).click();

  await expectSignedInToToday(page);
});

test('a student sign-up with a code that is not valid creates nothing and says so under the code', async ({
  page,
  control,
}) => {
  const student = newTestAccount('Ученик без кода', 'student');
  await page.goto('/signup');
  await page.getByRole('link', { name: /Я учусь/ }).click();
  await expect(page).toHaveURL('/signup/student');
  await expect(page.getByText('Получили ссылку от репетитора? Откройте её вместо этого.')).toBeVisible();
  await page.getByLabel('Код репетитора').fill('2222-2222');
  await page.getByLabel('Имя').fill(student.name);
  await page.getByLabel('Email').fill(student.email);
  await page.getByLabel('Пароль', { exact: true }).fill(student.password);
  await page.getByLabel(/Я соглашаюсь на обработку персональных данных/).check();

  await page.getByRole('button', { name: 'Зарегистрироваться' }).click();

  await expect(page.getByRole('alert')).toHaveText('Проверьте выделенные поля.');
  await expect(page.getByRole('alert')).toBeFocused();
  await expect(page.getByText('Этот код больше не действует — попросите у репетитора новый.')).toBeVisible();
  await expect(page.getByLabel('Код репетитора')).toHaveAttribute('aria-invalid', 'true');
  await control.tick();
  expect(await control.mail(student.email)).toEqual([]);
});

test('a signed-in tutor opening sign-up is sent to Today', async ({ signedInPage: page }) => {
  await page.goto('/signup/tutor');

  await expectSignedInToToday(page);
});

test('confirming another account in a signed-in browser warns, signs out first and leaves no tab acting as the old account', async ({
  page,
  deployment,
  control,
}) => {
  const first = await seedVerifiedUser(deployment, 'Первый репетитор');
  const second = newTestAccount('Второй репетитор');
  await registerTutor(deployment, second, 'ru');
  await control.tick();
  const link = emailedLink(await control.mail(second.email), '/verify-email');
  await page.context().clock.install();
  await signInThroughTheApi(page, first);
  await page.goto('/today');
  const otherTab = await page.context().newPage();
  await otherTab.goto('/today');
  await expect(otherTab.getByRole('button', { name: 'Первый репетитор, Меню аккаунта' })).toBeVisible();
  await page.goto(link);
  await expect(page.getByRole('status')).toContainText(first.email);

  await page.getByLabel('Новый пароль').fill(second.password);
  await page.getByRole('button', { name: 'Подтвердить и войти' }).click();
  await expectSignedInToToday(page);
  await page.clock.runFor('12:00');

  await expect(page.getByRole('button', { name: 'Второй репетитор, Меню аккаунта' })).toBeVisible();
  await expect(otherTab.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
});

test('a confirmation link with an over-long token leaves the address before any page chunk loads and is refused', async ({
  page,
  deployment,
}) => {
  const chunkAddresses = await recordLazyChunkAddresses(page, deployment.origin);

  await page.goto(`/verify-email?token=${'a'.repeat(65)}`);

  await expect(page.getByRole('alert')).toHaveText('Ссылка недействительна или устарела.');
  await expect(page.getByLabel('Новый пароль')).toHaveCount(0);
  expect(chunkAddresses).not.toEqual([]);
  expect(chunkAddresses.filter((address) => address.includes('token='))).toEqual([]);
});

test('a signed-in account can sign out from the public frame', async ({ page, deployment }) => {
  const account = newTestAccount('Leaving tutor');
  await registerTutor(deployment, account, 'ru');
  await signInThroughTheApi(page, account);
  await page.goto('/verify-email');

  await page.getByRole('banner').getByRole('button', { name: 'Выйти' }).click();

  await expect(page.getByRole('navigation').getByRole('link', { name: 'Войти' })).toBeVisible();
  await expect(page.getByRole('banner').getByRole('button', { name: 'Выйти' })).toHaveCount(0);
});

test('resetting the password in a browser signed in to that account ends its session there', async ({
  page,
  deployment,
  control,
}) => {
  const account = await seedVerifiedUser(deployment, 'Resetting tutor');
  await control.advance(61);
  await signInThroughTheApi(page, account);
  await page.goto('/forgot-password');
  await page.getByLabel('Email').fill(account.email);
  await page.getByRole('button', { name: 'Отправить ссылку' }).click();
  await expect(page.getByRole('status')).toContainText(account.email);
  await control.tick();
  await page.goto(emailedLink(await control.mail(account.email), '/reset-password'));

  await page.getByLabel('Новый пароль').fill(`${account.password} renewed`);
  await page.getByRole('button', { name: 'Сохранить пароль' }).click();

  await expect(page.getByRole('status')).toHaveText('Пароль изменён. Войдите с новым паролем.');
  await expect(page.getByRole('navigation').getByRole('link', { name: 'Войти' })).toBeVisible();
});

test('a problem the server finds is shown under the field it names', async ({ page }) => {
  const account = newTestAccount('   ');

  await signUpThroughTheForm(page, account);

  await expect(page.getByRole('alert')).toHaveText('Проверьте выделенные поля.');
  await expect(page.getByLabel('Имя')).toHaveAttribute('aria-invalid', 'true');
  await expect(page.getByText('Проверьте значение.')).toBeVisible();
});

test('a reset link with an over-long token leaves the address before any page chunk loads and is refused on sending', async ({
  page,
  deployment,
}) => {
  const chunkAddresses = await recordLazyChunkAddresses(page, deployment.origin);
  await page.goto(`/reset-password?token=${'a'.repeat(65)}`);
  await page.getByLabel('Новый пароль').fill('a sufficiently long password');

  await page.getByRole('button', { name: 'Сохранить пароль' }).click();

  await expect(page.getByRole('alert')).toHaveText('Ссылка недействительна или устарела.');
  await expect(page.getByRole('link', { name: 'Запросить новую ссылку' })).toBeVisible();
  expect(chunkAddresses).not.toEqual([]);
  expect(chunkAddresses.filter((address) => address.includes('token='))).toEqual([]);
});
