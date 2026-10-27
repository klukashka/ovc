import type { Page, Response } from '@playwright/test';

import { newTestAccount, registerTutor, signInThroughTheApi } from './accounts.ts';
import { expect, seedVerifiedUser, test } from './fixtures.ts';
import { expectRefetchedAfterTheSocketIsReady, trafficAfterReady } from './realtime.ts';

test.use({ locale: 'ru-RU' });

const knownPreferenceKeys = ['ovc.language', 'ovc.sidebar', 'ovc.theme'];
const accessTokenExpiredWithinSocketGrace = 15 * 60 + 10;

async function signInThroughTheForm(page: Page, email: string, password: string): Promise<void> {
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Пароль', { exact: true }).fill(password);
  const answered = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/auth/login');
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await answered;
}

async function signOutThroughTheMenu(page: Page): Promise<void> {
  await page.getByRole('button', { name: /Меню аккаунта/ }).click();
  await page.getByRole('button', { name: 'Выйти' }).click();
}

function authResponses(page: Page): Response[] {
  const seen: Response[] = [];
  page.on('response', (response) => {
    if (new URL(response.url()).pathname.startsWith('/api/v1/auth/')) {
      seen.push(response);
    }
  });
  return seen;
}

function summary(responses: readonly Response[]): string[] {
  return responses.map((response) => `${new URL(response.url()).pathname} ${String(response.status())}`);
}

async function browserStorage(page: Page): Promise<{ local: string[]; session: number; cookie: string }> {
  return page.evaluate(() => ({
    local: Object.keys(globalThis.localStorage).sort(),
    session: globalThis.sessionStorage.length,
    cookie: globalThis.document.cookie,
  }));
}

test('a blocked account is signed out and told it is blocked when it signs in again', async ({ page, deployment }) => {
  const blocked = await seedVerifiedUser(deployment, 'Заблокированный репетитор');
  await page.goto('/');
  await signInThroughTheForm(page, blocked.email, blocked.password);
  await expect(page).toHaveURL('/today');
  deployment.administer(['users', 'block', blocked.email]);
  await expect(page.getByRole('status')).toHaveText('Вы вышли из аккаунта на этом устройстве');
  await expect(page).toHaveURL('/');

  await signInThroughTheForm(page, blocked.email, blocked.password);

  await expect(page.getByText('Этот аккаунт заблокирован.')).toBeVisible();
  await expect(page).toHaveURL('/');
});

test('pilot-4 › step 8 anonymous redirect', async ({ page, request }) => {
  await page.goto('/schedule?week=3');

  const me = await request.get('/api/v1/users/me');

  await expect(page).toHaveURL('/?next=%2Fschedule%3Fweek%3D3');
  await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
  expect(me.status()).toBe(401);
  expect(await me.json()).toMatchObject({ error: { code: 'unauthenticated' } });
});

test('signing in follows next, signing out returns to sign-in that a reload keeps, and only known preferences are stored', async ({
  page,
  tutor,
}) => {
  await page.goto('/schedule?week=3');
  await signInThroughTheForm(page, tutor.email, tutor.password);
  await expect(page.getByRole('heading', { level: 1, name: 'Расписание' })).toBeVisible();
  const signedInUrl = page.url();
  const signedInStorage = await browserStorage(page);
  await signOutThroughTheMenu(page);
  await expect(page).toHaveURL('/');
  await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();

  await page.reload();

  await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
  expect(new URL(signedInUrl).pathname + new URL(signedInUrl).search).toBe('/schedule?week=3');
  for (const storage of [signedInStorage, await browserStorage(page)]) {
    expect(knownPreferenceKeys).toEqual(expect.arrayContaining(storage.local));
    expect(storage.session).toBe(0);
    expect(storage.cookie).toBe('');
  }
});

test('a next address on another origin is ignored after sign-in', async ({ page, tutor }) => {
  await page.goto('/?next=%2F%2Fevil.example%2Ftoday');

  await signInThroughTheForm(page, tutor.email, tutor.password);

  await expect(page).toHaveURL('/today');
  await expect(page.getByRole('heading', { level: 1, name: 'Сегодня' })).toBeVisible();
});

async function refusalOf(page: Page, email: string, password: string): Promise<string> {
  await signInThroughTheForm(page, email, password);
  const alert = page.getByRole('alert');
  await expect(alert).toBeFocused();
  return (await alert.textContent()) ?? '';
}

test('a wrong password and an unknown email show the same message', async ({ page, tutor }) => {
  await page.goto('/');

  const wrongPassword = await refusalOf(page, tutor.email, 'not the right password');
  const unknownEmail = await refusalOf(page, newTestAccount('Nobody').email, tutor.password);

  expect([wrongPassword, unknownEmail]).toEqual(['Неверный email или пароль.', 'Неверный email или пароль.']);
  await expect(page).toHaveURL('/');
});

test('the sixth failed sign-in within 15 minutes says when to try again', async ({ page }) => {
  const stranger = newTestAccount('Stranger');
  await page.goto('/');
  for (let attempt = 0; attempt < 5; attempt += 1) {
    await signInThroughTheForm(page, stranger.email, stranger.password);
  }

  await signInThroughTheForm(page, stranger.email, stranger.password);

  await expect(page.getByRole('alert')).toHaveText(
    /^Слишком много попыток, попробуйте через \d+ (минуту|минуты|минут)\.$/,
  );
});

test('a signed-in tutor opening sign-in is sent to Today', async ({ signedInPage: page }) => {
  await page.goto('/');

  await expect(page).toHaveURL('/today');
  await expect(page.getByRole('heading', { level: 1, name: 'Сегодня' })).toBeVisible();
});

test('a signed-in tutor sees Not found in their own frame at a student address', async ({ signedInPage: page }) => {
  await page.goto('/home');

  await expect(page.getByRole('heading', { level: 1, name: 'Страница не найдена' })).toBeVisible();
  await expect(page.getByRole('navigation').getByRole('link', { name: 'Ученики' })).toBeVisible();
  await expect(page).toHaveTitle('Страница не найдена · OVC');
});

test.describe('below 1024 px', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('the top bar names Not found, not the student page, at a student address', async ({ signedInPage: page }) => {
    await page.goto('/home');

    await expect(page.getByRole('heading', { level: 1, name: 'Страница не найдена' })).toBeVisible();
    await expect(page.getByRole('banner').getByText('Страница не найдена')).toBeVisible();
    await expect(page.getByRole('banner').getByText('Главная')).toHaveCount(0);
  });
});

async function expectTodayLoaded(page: Page): Promise<void> {
  const requests = page.getByRole('region', { name: 'Заявки на занятия' });
  await expect(requests.getByText('Новых заявок нет.')).toBeVisible();
  await expect(requests.getByText(/^[2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4}$/)).toBeVisible();
}

async function expectSignedIn(page: Page): Promise<void> {
  await expect(page.getByRole('navigation').getByRole('link', { name: 'Сегодня' })).toBeVisible();
}

test('two tabs refreshing at once keep the session', async ({ signedInPage: page }) => {
  const second = await page.context().newPage();
  await Promise.all([page.goto('/today'), second.goto('/today')]);
  await Promise.all([expectSignedIn(page), expectSignedIn(second)]);
  const firstResponses = authResponses(page);
  const secondResponses = authResponses(second);

  await Promise.all([page.reload(), second.reload()]);

  await Promise.all([expectSignedIn(page), expectSignedIn(second)]);
  const third = await page.context().newPage();
  await third.goto('/today');
  await expectSignedIn(third);
  const refreshes = [...summary(firstResponses), ...summary(secondResponses)];
  expect(refreshes.length).toBeGreaterThanOrEqual(2);
  expect(new Set(refreshes)).toEqual(new Set(['/api/v1/auth/refresh 200']));
});

const todayQueryPaths = ['/api/v1/notifications', '/api/v1/tutor/code', '/api/v1/join-requests'];

test('an expired access token is refreshed once and the request is retried', async ({ page, tutor, control }) => {
  await signInThroughTheApi(page, tutor);
  const traffic = trafficAfterReady(page, todayQueryPaths);
  await page.goto('/today');
  await expect(page.getByRole('navigation').getByRole('link', { name: 'Сегодня' })).toBeVisible();
  await expectTodayLoaded(page);
  await expectRefetchedAfterTheSocketIsReady(traffic);
  const responses = authResponses(page);
  await control.advance(accessTokenExpiredWithinSocketGrace);

  await signOutThroughTheMenu(page);

  await expect(page).toHaveURL('/');
  expect(summary(responses)).toEqual([
    '/api/v1/auth/logout 401',
    '/api/v1/auth/refresh 200',
    '/api/v1/auth/logout 204',
  ]);
});

test('the access token is refreshed before it expires', async ({ page, tutor }) => {
  await page.clock.install();
  await signInThroughTheApi(page, tutor);
  await page.goto('/today');
  await expect(page.getByRole('navigation').getByRole('link', { name: 'Сегодня' })).toBeVisible();
  const proactive = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/auth/refresh');

  await page.clock.runFor('12:00');

  expect((await proactive).status()).toBe(200);
});

test('an unverified tutor signs in to the account language and sees the confirmation banner', async ({
  page,
  deployment,
}) => {
  const account = newTestAccount('A tutor who reads English');
  await registerTutor(deployment, account, 'en');
  await page.goto('/');

  await signInThroughTheForm(page, account.email, account.password);

  const banner = page.getByRole('region', { name: 'Email confirmation' });
  await expect(page).toHaveURL('/today');
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');
  await expect(banner).toContainText(account.email);
  await expect(banner.getByRole('link', { name: 'Confirm email' })).toHaveAttribute('href', '/verify-email');
  expect(await page.evaluate(() => globalThis.localStorage.getItem('ovc.language'))).toBe('en');
});
