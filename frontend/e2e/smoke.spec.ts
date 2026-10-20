import { type Page, expect, test as base } from '@playwright/test';

import { contentSecurityPolicy } from '../security-headers.ts';

const firstBodyThemeKey = 'ovcFirstBodyTheme';

function collectConsoleErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on('console', (message) => {
    if (message.type() === 'error') {
      errors.push(message.text());
    }
  });
  page.on('pageerror', (error) => {
    errors.push(error.message);
  });
  return errors;
}

async function reportPolicyViolations(page: Page): Promise<void> {
  await page.addInitScript(() => {
    document.addEventListener('securitypolicyviolation', (event) => {
      console.error(`csp-violation ${event.violatedDirective} ${event.blockedURI}`);
    });
  });
}

async function recordThemeAtFirstBody(page: Page, key: string): Promise<void> {
  await page.addInitScript((storageKey) => {
    const observer = new MutationObserver(() => {
      if (document.querySelector('body') !== null) {
        Reflect.set(globalThis, storageKey, document.documentElement.dataset.theme ?? 'unset');
        observer.disconnect();
      }
    });
    observer.observe(document, { childList: true, subtree: true });
  }, key);
}

const test = base.extend<{ consoleErrors: string[] }>({
  consoleErrors: [
    async ({ page }, use): Promise<void> => {
      await reportPolicyViolations(page);
      const errors = collectConsoleErrors(page);
      await use(errors);
      expect(errors).toEqual([]);
    },
    { auto: true },
  ],
});

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

test('the preview server answers no cross-origin requests', async ({ request }) => {
  const preflight = await request.fetch('/api/v1/health/live', {
    method: 'OPTIONS',
    headers: { Origin: 'http://localhost:9', 'Access-Control-Request-Method': 'GET' },
  });
  const simple = await request.get('/api/v1/health/live', { headers: { Origin: 'http://localhost:9' } });

  expect(preflight.headers()['access-control-allow-origin']).toBeUndefined();
  expect(simple.headers()['access-control-allow-origin']).toBeUndefined();
});

test('the tutor frame shows the tutor sections and server readiness', async ({ page }) => {
  await page.goto('/today');

  const navigation = page.getByRole('navigation', { name: 'Основная навигация' });
  for (const label of [
    'Сегодня',
    'Расписание',
    'Ученики',
    'Группы',
    'Задания',
    'Сообщения',
    'Уведомления',
    'Настройки',
  ]) {
    await expect(navigation.getByRole('link', { name: label })).toBeVisible();
  }
  await expect(navigation.getByRole('link', { name: 'Сегодня' })).toHaveAttribute('aria-current', 'page');
  await expect(page.getByRole('status')).toHaveText('Сервер готов');
  await expect(page).toHaveTitle('Сегодня · OVC');
});

test('the student frame shows the student sections in English', async ({ page }) => {
  await chooseEnglish(page);

  await page.goto('/home');

  const navigation = page.getByRole('navigation', { name: 'Main navigation' });
  for (const label of ['Home', 'Schedule', 'Homework', 'Messages', 'My tutors', 'Notifications', 'Settings']) {
    await expect(navigation.getByRole('link', { name: label, exact: true })).toBeVisible();
  }
  await expect(navigation.getByRole('link', { name: 'Students' })).toHaveCount(0);
  await expect(page.getByRole('status')).toHaveText('Server ready');
});

test('a shared page opened from the student sidebar keeps the student frame and focuses its heading', async ({
  page,
}) => {
  await page.goto('/home');

  await page.getByRole('navigation').getByRole('link', { name: 'Расписание' }).click();

  await expect(page.getByRole('heading', { level: 1, name: 'Расписание' })).toBeFocused();
  await expect(page.getByRole('navigation').getByRole('link', { name: 'Мои репетиторы' })).toBeVisible();
  await expect(page.getByRole('navigation').getByRole('link', { name: 'Ученики' })).toHaveCount(0);
});

test('hiding the sidebar moves focus to the show button and is remembered', async ({ page }) => {
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

test('showing the sidebar again is remembered', async ({ page }) => {
  await page.goto('/today');
  await page.getByRole('button', { name: 'Скрыть боковую панель' }).click();

  await page.getByRole('button', { name: 'Показать боковую панель' }).click();
  await page.reload();

  await expect(page.getByRole('navigation', { name: 'Основная навигация' })).toBeVisible();
});

test.describe('below 1024 px', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test('the top bar shows the page title, the bell and touch-sized targets', async ({ page }) => {
    await page.goto('/today');

    const showButton = page.getByRole('button', { name: 'Показать боковую панель' });
    const bell = page.getByRole('banner').getByRole('link', { name: 'Уведомления' });
    await expect(page.getByRole('banner').getByText('Сегодня')).toBeVisible();
    for (const target of [showButton, bell]) {
      const box = await target.boundingBox();
      expect(box?.width).toBeGreaterThanOrEqual(44);
      expect(box?.height).toBeGreaterThanOrEqual(44);
    }
  });

  test('the bell keeps the student frame', async ({ page }) => {
    await page.goto('/home');

    await page.getByRole('banner').getByRole('link', { name: 'Уведомления' }).click();
    await page.getByRole('button', { name: 'Показать боковую панель' }).click();

    await expect(page.getByRole('heading', { level: 1, name: 'Уведомления' })).toBeVisible();
    await expect(page.getByRole('dialog').getByRole('link', { name: 'Мои репетиторы' })).toBeVisible();
    await expect(page.getByRole('dialog').getByRole('link', { name: 'Ученики' })).toHaveCount(0);
  });

  test('opening the overlay focuses the current item', async ({ page }) => {
    await page.goto('/today');

    await page.getByRole('button', { name: 'Показать боковую панель' }).click();

    await expect(page.getByRole('dialog').getByRole('link', { name: 'Сегодня' })).toBeFocused();
  });

  test('Tab and Shift+Tab stay inside the overlay', async ({ page }) => {
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

  test('Escape closes the overlay and returns focus to the show button', async ({ page }) => {
    await openOverlay(page);

    await page.keyboard.press('Escape');

    await expect(page.getByRole('dialog')).toBeHidden();
    await expect(page.getByRole('button', { name: 'Показать боковую панель' })).toBeFocused();
  });

  test('a click on the dimmed page closes the overlay and returns focus', async ({ page }) => {
    await openOverlay(page);

    await page.mouse.click(380, 400);

    await expect(page.getByRole('dialog')).toBeHidden();
    await expect(page.getByRole('button', { name: 'Показать боковую панель' })).toBeFocused();
  });

  test('Escape in the account menu closes only the menu', async ({ page }) => {
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

  test('choosing an item closes the overlay, navigates and focuses the heading', async ({ page }) => {
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
    await recordThemeAtFirstBody(page, firstBodyThemeKey);

    await page.goto('/today');

    const themeAtFirstBody = await page.evaluate((key) => String(Reflect.get(globalThis, key)), firstBodyThemeKey);
    expect(themeAtFirstBody).toBe('dark');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(28, 27, 24)');
  });
});
