import type { Page, Response } from '@playwright/test';

import { newTestAccount, pageSettled, registerTutor, signInThroughTheApi } from './accounts.ts';
import { diagnosedTest as test, expect, seedVerifiedUser, type PageDiagnostics } from './fixtures.ts';
import {
  accessTokenExpiredWithinSocketGrace,
  expectRefetchedAfterTheSocketIsReady,
  expectReadyForNavigation,
  todayQueryPaths,
  trafficAfterReady,
} from './realtime.ts';

test.use({ locale: 'ru-RU' });

const knownPreferenceKeys = ['ovc.language', 'ovc.sidebar', 'ovc.theme'];

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

interface HeldPageModule {
  readonly requested: Promise<void>;
  readonly release: () => void;
}

async function holdThePageModule(page: Page, chunk: string): Promise<HeldPageModule> {
  let release: () => void = () => undefined;
  const resumed = new Promise<void>((resolve) => {
    release = resolve;
  });
  let markRequested: () => void = () => undefined;
  const requested = new Promise<void>((resolve) => {
    markRequested = resolve;
  });
  await page.route(`**/assets/${chunk}-*.js`, async (route) => {
    markRequested();
    await resumed;
    await route.continue();
  });
  return { requested, release };
}

test('a blocked account is signed out and told it is blocked when it signs in again', async ({
  page,
  deployment,
  diagnostics,
}) => {
  const blocked = await seedVerifiedUser(deployment, 'Заблокированный репетитор');
  await page.goto('/');
  const ready = trafficAfterReady(page, todayQueryPaths);
  await ready.initialize();
  await signInThroughTheForm(page, blocked.email, blocked.password);
  await expect(page).toHaveURL('/today');
  await expectReadyForNavigation(page, ready);
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/refresh', status: 401 });
  deployment.administer(['users', 'block', blocked.email]);
  await expect(page.getByRole('status')).toHaveText('Вы вышли из аккаунта на этом устройстве');
  await expect(page).toHaveURL('/');
  await diagnostics.finishHttpPhase();
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/login', status: 403 });

  await signInThroughTheForm(page, blocked.email, blocked.password);

  await expect(page.getByText('Этот аккаунт заблокирован.')).toBeVisible();
  await expect(page).toHaveURL('/');
  await diagnostics.finishHttpPhase();
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
  await pageSettled(page);

  await page.reload();

  await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
  expect(new URL(signedInUrl).pathname + new URL(signedInUrl).search).toBe('/schedule?week=3');
  for (const storage of [signedInStorage, await browserStorage(page)]) {
    expect(knownPreferenceKeys).toEqual(expect.arrayContaining(storage.local));
    expect(storage.session).toBe(0);
    expect(storage.cookie).toBe('');
  }
});

test('a page counts as settled only once its page module has loaded, not while its loading heading shows', async ({
  page,
}) => {
  const signInModule = await holdThePageModule(page, 'SignInPage');
  await page.goto('/');
  await signInModule.requested;
  await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
  const order: string[] = [];
  const settled = pageSettled(page).then(() => {
    order.push('settled');
  });
  await page.evaluate(async () => {
    await document.fonts.ready;
  });
  await expect(page.locator('main [aria-busy="true"]')).toHaveCount(1);

  order.push('released');
  signInModule.release();
  await settled;

  expect(order).toEqual(['released', 'settled']);
  await expect(page.getByLabel('Email')).toBeVisible();
});

test('a next address on another origin is ignored after sign-in', async ({ page, tutor }) => {
  await page.goto('/?next=%2F%2Fevil.example%2Ftoday');

  await signInThroughTheForm(page, tutor.email, tutor.password);

  await expect(page).toHaveURL('/today');
  await expect(page.getByRole('heading', { level: 1, name: 'Сегодня' })).toBeVisible();
});

async function refusalOf(page: Page, diagnostics: PageDiagnostics, email: string, password: string): Promise<string> {
  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/login', status: 401 });
  await signInThroughTheForm(page, email, password);
  const alert = page.getByRole('alert');
  await expect(alert).toBeFocused();
  await diagnostics.finishHttpPhase();
  return (await alert.textContent()) ?? '';
}

test('a wrong password and an unknown email show the same message', async ({ page, tutor, diagnostics }) => {
  await page.goto('/');

  const wrongPassword = await refusalOf(page, diagnostics, tutor.email, 'not the right password');
  const unknownEmail = await refusalOf(page, diagnostics, newTestAccount('Nobody').email, tutor.password);

  expect([wrongPassword, unknownEmail]).toEqual(['Неверный email или пароль.', 'Неверный email или пароль.']);
  await expect(page).toHaveURL('/');
});

test('the sixth failed sign-in within 15 minutes says when to try again', async ({ page, diagnostics }) => {
  const stranger = newTestAccount('Stranger');
  await page.goto('/');
  for (let attempt = 0; attempt < 5; attempt += 1) {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/login', status: 401 });
    await signInThroughTheForm(page, stranger.email, stranger.password);
    await diagnostics.finishHttpPhase();
  }

  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/login', status: 429 });
  await signInThroughTheForm(page, stranger.email, stranger.password);

  await expect(page.getByRole('alert')).toHaveText(
    /^Слишком много попыток, попробуйте через \d+ (минуту|минуты|минут)\.$/,
  );
  await diagnostics.finishHttpPhase();
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

test.describe('from 768 to 1023 px', () => {
  test.use({ viewport: { width: 800, height: 1000 } });

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
  const firstReady = trafficAfterReady(page, todayQueryPaths);
  await firstReady.initialize();
  const secondReady = trafficAfterReady(second, todayQueryPaths);
  await secondReady.initialize();
  await Promise.all([page.goto('/today'), second.goto('/today')]);
  await Promise.all([expectSignedIn(page), expectSignedIn(second)]);
  await Promise.all([expectReadyForNavigation(page, firstReady), expectReadyForNavigation(second, secondReady)]);
  const firstResponses = authResponses(page);
  const secondResponses = authResponses(second);

  const firstReloadReady = trafficAfterReady(page, todayQueryPaths);

  await firstReloadReady.initialize();
  const secondReloadReady = trafficAfterReady(second, todayQueryPaths);
  await secondReloadReady.initialize();
  await Promise.all([page.reload(), second.reload()]);

  await Promise.all([expectSignedIn(page), expectSignedIn(second)]);
  await Promise.all([
    expectReadyForNavigation(page, firstReloadReady),
    expectReadyForNavigation(second, secondReloadReady),
  ]);
  const third = await page.context().newPage();
  const thirdReady = trafficAfterReady(third, todayQueryPaths);
  await thirdReady.initialize();
  await third.goto('/today');
  await expectSignedIn(third);
  await expectReadyForNavigation(third, thirdReady);
  const refreshes = [...summary(firstResponses), ...summary(secondResponses)];
  expect(refreshes.length).toBeGreaterThanOrEqual(2);
  expect(new Set(refreshes)).toEqual(new Set(['/api/v1/auth/refresh 200']));
});

test('an expired access token is refreshed once and the request is retried', async ({
  page,
  tutor,
  control,
  diagnostics,
}) => {
  await signInThroughTheApi(page, tutor);
  const traffic = trafficAfterReady(page, todayQueryPaths);
  await traffic.initialize();
  await page.goto('/today');
  await expect(page.getByRole('navigation').getByRole('link', { name: 'Сегодня' })).toBeVisible();
  await expectTodayLoaded(page);
  await expectRefetchedAfterTheSocketIsReady(traffic);
  const responses = authResponses(page);
  await control.advance(accessTokenExpiredWithinSocketGrace);

  diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/logout', status: 401 });
  await signOutThroughTheMenu(page);

  await expect(page).toHaveURL('/');
  expect(summary(responses)).toEqual([
    '/api/v1/auth/logout 401',
    '/api/v1/auth/refresh 200',
    '/api/v1/auth/logout 204',
  ]);
  await diagnostics.finishHttpPhase();
});

test('the access token is refreshed before it expires', async ({ page, tutor }) => {
  await page.clock.install();
  const traffic = trafficAfterReady(page, todayQueryPaths);
  await traffic.initialize();
  await signInThroughTheApi(page, tutor);
  await page.goto('/today');
  await expect(page.getByRole('navigation').getByRole('link', { name: 'Сегодня' })).toBeVisible();
  await expectReadyForNavigation(page, traffic);
  const proactive = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/auth/refresh');
  await page.clock.fastForward('12:00');

  const refreshed = await proactive;
  expect(refreshed.status()).toBe(200);
  expect(await refreshed.finished()).toBeNull();
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

test.describe('below 768 px', () => {
  test.use({ viewport: { width: 360, height: 780 } });

  test('signing out from the More sheet returns to sign-in and the old session is refused', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Телефонный репетитор');
    await signInThroughTheApi(page, tutor);
    const bearers = new Set<string>();
    page.on('request', (request) => {
      const bearer = request.headers().authorization;
      if (bearer !== undefined && new URL(request.url()).pathname === '/api/v1/users/me') {
        bearers.add(bearer);
      }
    });
    const ready = trafficAfterReady(page, todayQueryPaths);
    await ready.initialize();
    await page.goto('/today');
    await expectReadyForNavigation(page, ready);
    await page.getByRole('navigation', { name: 'Основная навигация' }).getByRole('button', { name: 'Ещё' }).click();

    await page.getByRole('dialog', { name: 'Ещё' }).getByRole('button', { name: 'Выйти' }).click();

    await expect(page).toHaveURL('/');
    await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
    expect(bearers.size).toBeGreaterThanOrEqual(1);
    for (const bearer of bearers) {
      const answer = await page.request.get('/api/v1/users/me', { headers: { Authorization: bearer } });
      expect(answer.status()).toBe(401);
    }
  });
});
