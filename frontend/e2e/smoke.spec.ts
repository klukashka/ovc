import { type Locator, type Page } from '@playwright/test';

import { signInThroughTheApi } from './accounts.ts';
import { diagnosedTest as test, expect } from './fixtures.ts';

import { contentSecurityPolicy } from '../security-headers.ts';

async function recordThemeAtFirstBody(page: Page): Promise<void> {
  await page.addInitScript(() => {
    const observer = new MutationObserver(() => {
      if (document.querySelector('body') !== null) {
        window.ovcFirstBodyTheme = document.documentElement.dataset.theme ?? 'unset';
        observer.disconnect();
      }
    });
    observer.observe(document, { childList: true, subtree: true });
  });
}

test.use({ locale: 'ru-RU' });

async function chooseEnglish(page: Page): Promise<void> {
  await page.goto('/');
  await page.getByRole('button', { name: 'English' }).click();
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');
}

async function openOverlay(page: Page): Promise<void> {
  await page.goto('/today');
  await page.getByRole('button', { name: 'Показать боковую панель' }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
}

test('the public frame follows the browser language', async ({ page }) => {
  await page.goto('/');

  await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('lang', 'ru');
  await expect(page.getByRole('button', { name: 'Русский' })).toHaveAttribute('aria-pressed', 'true');
});

test('the chosen language is remembered across reloads', async ({ page }) => {
  await chooseEnglish(page);

  await page.reload();

  await expect(page.getByRole('heading', { level: 1, name: 'Sign in' })).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');
  await expect(page.getByRole('button', { name: 'English' })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('navigation').getByRole('link', { name: 'Sign up' })).toBeVisible();
});

test('pages and proxied API responses carry the security headers', async ({ page, request }) => {
  const pageResponse = await page.goto('/today');
  const apiResponse = await request.get('/api/v1/health/live');

  const pageHeaders = pageResponse?.headers() ?? {};
  expect(pageHeaders['referrer-policy']).toBe('no-referrer');
  expect(pageHeaders['x-content-type-options']).toBe('nosniff');
  expect(pageHeaders['content-security-policy']).toBe(contentSecurityPolicy);
  expect(apiResponse.status()).toBe(200);
  expect(apiResponse.headers()['referrer-policy']).toBe('no-referrer');
  expect(apiResponse.headers()['x-content-type-options']).toBe('nosniff');
});

test('the preview server has no build manifest to serve', async ({ request, deployment }) => {
  const addresses = ['/.vite/manifest.json', '//.vite/manifest.json', '//%2Evite/manifest.json'];

  const bodies = await Promise.all(
    addresses.map(async (address) => (await request.get(`${deployment.origin}${address}`)).text()),
  );

  expect(bodies.filter((body) => body.includes('isEntry'))).toEqual([]);
});

test('the tutor frame shows the tutor sections and server readiness', async ({ signedInPage: page }) => {
  await page.goto('/today');

  const navigation = page.getByRole('navigation', { name: 'Основная навигация' });
  for (const label of ['Сегодня', 'Расписание', 'Ученики', 'Группы', 'Задания', 'Сообщения', 'Настройки']) {
    await expect(navigation.getByRole('link', { name: label })).toBeVisible();
  }
  await expect(navigation.getByRole('button', { name: /^Уведомления/ })).toBeVisible();
  await expect(navigation.getByRole('link', { name: 'Сегодня' })).toHaveAttribute('aria-current', 'page');
  await expect(page.getByRole('region', { name: 'Состояние сервера' }).getByRole('status')).toHaveText('Сервер готов');
  await expect(page).toHaveTitle('Сегодня · OVC');
});

test('hiding the sidebar moves focus to the show button and is remembered', async ({ signedInPage: page }) => {
  await page.goto('/today');

  await page.getByRole('button', { name: 'Скрыть боковую панель' }).click();
  const focusedAfterHiding = await page
    .waitForFunction(() => document.activeElement?.getAttribute('aria-label') === 'Показать боковую панель', null, {
      timeout: 5_000,
    })
    .then(
      () => true,
      () => false,
    );
  await page.reload();

  expect(focusedAfterHiding).toBe(true);
  await expect(page.getByRole('navigation', { name: 'Основная навигация' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Показать боковую панель' })).toBeVisible();
});

test('showing the sidebar again is remembered', async ({ signedInPage: page }) => {
  await page.goto('/today');
  await page.getByRole('button', { name: 'Скрыть боковую панель' }).click();

  await page.getByRole('button', { name: 'Показать боковую панель' }).click();
  await page.reload();

  await expect(page.getByRole('navigation', { name: 'Основная навигация' })).toBeVisible();
});

test.describe('below 1024 px', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('the top bar shows the page title, the bell and touch-sized targets', async ({ signedInPage: page }) => {
    await page.goto('/today');

    const showButton = page.getByRole('button', { name: 'Показать боковую панель' });
    const bell = page.getByRole('banner').getByRole('button', { name: /^Уведомления/ });
    await expect(page.getByRole('banner').getByText('Сегодня')).toBeVisible();
    for (const target of [showButton, bell]) {
      const box = await target.boundingBox();
      expect(box?.width).toBeGreaterThanOrEqual(44);
      expect(box?.height).toBeGreaterThanOrEqual(44);
    }
  });

  test('opening the overlay focuses the current item', async ({ signedInPage: page }) => {
    await page.goto('/today');

    await page.getByRole('button', { name: 'Показать боковую панель' }).click();

    await expect(page.getByRole('dialog').getByRole('link', { name: 'Сегодня' })).toBeFocused();
  });

  test('Tab and Shift+Tab stay inside the overlay', async ({ signedInPage: page }) => {
    await openOverlay(page);
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('link', { name: 'Настройки' }).focus();

    await page.keyboard.press('Tab');
    const focusedAfterTab = await dialog
      .getByRole('button', { name: /Меню аккаунта/ })
      .evaluate((element) => element === document.activeElement);
    await page.keyboard.press('Shift+Tab');

    expect(focusedAfterTab).toBe(true);
    await expect(dialog.getByRole('link', { name: 'Настройки' })).toBeFocused();
  });

  test('Escape closes the overlay and returns focus to the show button', async ({ signedInPage: page }) => {
    await openOverlay(page);

    await page.keyboard.press('Escape');

    await expect(page.getByRole('dialog')).toBeHidden();
    await expect(page.getByRole('button', { name: 'Показать боковую панель' })).toBeFocused();
  });

  test('a click on the dimmed page closes the overlay and returns focus', async ({ signedInPage: page }) => {
    await openOverlay(page);

    await page.mouse.click(380, 400);

    await expect(page.getByRole('dialog')).toBeHidden();
    await expect(page.getByRole('button', { name: 'Показать боковую панель' })).toBeFocused();
  });

  test('Escape in the account menu closes only the menu', async ({ signedInPage: page }) => {
    await openOverlay(page);
    await page
      .getByRole('dialog')
      .getByRole('button', { name: /Меню аккаунта/ })
      .click();

    await page.keyboard.press('Escape');

    await expect(page.getByRole('dialog')).toBeVisible();
    await expect(page.getByRole('dialog').getByRole('button', { name: 'Выйти' })).toHaveCount(0);
    await expect(page.getByRole('dialog').getByRole('button', { name: /Меню аккаунта/ })).toBeFocused();
  });

  test('choosing an item closes the overlay, navigates and focuses the heading', async ({ signedInPage: page }) => {
    await openOverlay(page);

    await page.getByRole('dialog').getByRole('link', { name: 'Расписание' }).click();

    await expect(page.getByRole('dialog')).toBeHidden();
    await expect(page).toHaveURL(/\/schedule$/);
    await expect(page.getByRole('heading', { level: 1, name: 'Расписание' })).toBeFocused();
  });
});

test.describe('with the dark colour scheme', () => {
  test.use({ colorScheme: 'dark' });

  test('the theme is applied before the body is parsed', async ({ page }) => {
    await recordThemeAtFirstBody(page);

    await page.goto('/today');

    const themeAtFirstBody = await page.evaluate(() => String(window.ovcFirstBodyTheme));
    expect(themeAtFirstBody).toBe('dark');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(28, 27, 24)');
  });
});

async function boxSizes(locators: Locator[]): Promise<{ width: number; height: number }[]> {
  const sizes: { width: number; height: number }[] = [];
  for (const locator of locators) {
    const box = await locator.boundingBox();
    sizes.push({ width: box?.width ?? 0, height: box?.height ?? 0 });
  }
  return sizes;
}

test('every page, asset and proxied API response carries the security headers', async ({ page, request }) => {
  await page.goto('/');
  const script = await page.locator('script[type="module"]').getAttribute('src');
  expect(script).toMatch(/^\/assets\/.+\.js$/);
  const paths = [
    '/students/1',
    '/no/such/page',
    '/theme-init.js',
    '/favicon.svg',
    String(script),
    '/api/v1/no-such-route',
  ];

  const responses = await Promise.all(paths.map((path) => request.get(path)));

  for (const [index, response] of responses.entries()) {
    const headers = response.headers();
    expect(headers['referrer-policy'], paths[index]).toBe('no-referrer');
    expect(headers['x-content-type-options'], paths[index]).toBe('nosniff');
  }
  expect(responses.at(-1)?.status()).toBe(404);
});

test('showing the sidebar moves focus to the hide button', async ({ signedInPage: page }) => {
  await page.goto('/today');
  await page.getByRole('button', { name: 'Скрыть боковую панель' }).click();

  await page.getByRole('button', { name: 'Показать боковую панель' }).click();

  await expect(page.getByRole('button', { name: 'Скрыть боковую панель' })).toBeFocused();
});

test('the current sidebar item uses the primary text colour', async ({ signedInPage: page }) => {
  await page.goto('/today');

  const current = page.getByRole('navigation').getByRole('link', { name: 'Сегодня' });

  await expect(current).toHaveCSS('color', 'rgb(38, 36, 31)');
  await expect(page.getByRole('navigation').getByRole('link', { name: 'Расписание' })).toHaveCSS(
    'color',
    'rgb(107, 101, 90)',
  );
});

test('controls on a computer follow the token sizes', async ({ page, tutor }) => {
  await page.goto('/');
  const languageButtons = await boxSizes([page.getByRole('button', { name: 'Русский' })]);
  const logo = await boxSizes([page.getByRole('link', { name: 'OVC' })]);
  await signInThroughTheApi(page, tutor);
  await page.goto('/today');

  const sidebarItems = await boxSizes([page.getByRole('navigation').getByRole('link', { name: 'Сегодня' })]);

  expect(sidebarItems[0]?.height).toBe(32);
  expect(languageButtons[0]?.height).toBeGreaterThanOrEqual(40);
  expect(logo[0]?.height).toBeGreaterThanOrEqual(44);
});

test('the account menu closes when focus leaves it', async ({ signedInPage: page }) => {
  await page.goto('/today');
  await page.getByRole('button', { name: /Меню аккаунта/ }).click();
  await page.getByRole('button', { name: 'Выйти' }).waitFor();

  await page.getByRole('button', { name: 'Выйти' }).focus();
  await page.keyboard.press('Tab');

  await expect(page.getByRole('button', { name: 'Выйти' })).toHaveCount(0);
});

test.describe('at 900 px', () => {
  test.use({ viewport: { width: 900, height: 800 } });

  test('every frame target is at least 44 px', async ({ page, tutor }) => {
    await page.goto('/');
    const publicTargets = await boxSizes([
      page.getByRole('button', { name: 'Русский' }),
      page.getByRole('navigation').getByRole('link', { name: 'Войти' }),
    ]);
    await signInThroughTheApi(page, tutor);
    await page.goto('/today');
    await page.getByRole('button', { name: 'Показать боковую панель' }).click();

    const overlayTargets = await boxSizes([
      page.getByRole('dialog').getByRole('link', { name: 'Сегодня' }),
      page.getByRole('dialog').getByRole('button', { name: /Меню аккаунта/ }),
    ]);

    for (const size of [...publicTargets, ...overlayTargets]) {
      expect(size.height).toBeGreaterThanOrEqual(44);
    }
  });

  test('the overlay is named and does not reopen after the window widens and narrows', async ({
    signedInPage: page,
  }) => {
    await page.goto('/today');
    await page.getByRole('button', { name: 'Показать боковую панель' }).click();
    await page.getByRole('dialog', { name: 'Боковая панель' }).waitFor();

    await page.setViewportSize({ width: 1280, height: 800 });
    await page.getByRole('button', { name: 'Скрыть боковую панель' }).waitFor();
    await page.setViewportSize({ width: 900, height: 800 });

    await expect(page.getByRole('button', { name: 'Показать боковую панель' })).toBeVisible();
    await expect(page.getByRole('dialog')).toBeHidden();
  });
});
