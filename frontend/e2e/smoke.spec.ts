import { randomBytes } from 'node:crypto';
import { once } from 'node:events';
import { readFile, rm } from 'node:fs/promises';
import { type Server, createServer } from 'node:http';

import { type Locator, type Page } from '@playwright/test';

import { signInThroughTheApi } from './accounts.ts';
import { diagnosedTest as test, diagnosticRegistry, expect, seedVerifiedUser, watchPage } from './fixtures.ts';
import { expectSettledAfterTheSocketIsReady, todayQueryPaths, trafficAfterReady } from './realtime.ts';
import { seedActiveStudent } from './tutoring.ts';

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

async function closeNatively(dialog: Locator): Promise<void> {
  await dialog.evaluate((element) => {
    if (element instanceof HTMLDialogElement) {
      element.close();
    }
  });
}

async function openOverlay(page: Page): Promise<void> {
  await openSettledToday(page);
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

test('the tutor frame shows the tutor sections without a server status block', async ({ signedInPage: page }) => {
  const readinessRequests = recordRequestsTo(page, '/api/v1/health/ready');

  await openSettledToday(page);

  const navigation = page.getByRole('navigation', { name: 'Основная навигация' });
  for (const label of ['Сегодня', 'Расписание', 'Ученики', 'Группы', 'Задания', 'Сообщения', 'Настройки']) {
    await expect(navigation.getByRole('link', { name: label })).toBeVisible();
  }
  await expect(navigation.getByRole('button', { name: /^Уведомления/ })).toBeVisible();
  await expect(navigation.getByRole('link', { name: 'Сегодня' })).toHaveAttribute('aria-current', 'page');
  await expect(page.getByRole('region', { name: 'Состояние сервера' })).toHaveCount(0);
  expect(readinessRequests).toEqual([]);
  await expect(page).toHaveTitle('Сегодня · OVC');
});

function recordRequestsTo(page: Page, path: string): readonly string[] {
  const requested: string[] = [];
  page.on('request', (request) => {
    if (new URL(request.url()).pathname === path) {
      requested.push(request.url());
    }
  });
  return requested;
}

async function openSettledToday(page: Page): Promise<void> {
  const traffic = trafficAfterReady(page, todayQueryPaths);
  await traffic.initialize();
  await page.goto('/today');
  await expectSettledAfterTheSocketIsReady(page, traffic);
}

test('hiding the sidebar moves focus to the show button and is remembered', async ({ signedInPage: page }) => {
  await openSettledToday(page);

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
  await openSettledToday(page);
  await page.getByRole('button', { name: 'Скрыть боковую панель' }).click();

  await page.getByRole('button', { name: 'Показать боковую панель' }).click();
  await page.reload();

  await expect(page.getByRole('navigation', { name: 'Основная навигация' })).toBeVisible();
});

test.describe('from 768 to 1023 px', () => {
  test.use({ viewport: { width: 800, height: 1000 } });

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

  test('a close by the browser itself, without a cancel event, returns focus to the show button', async ({
    signedInPage: page,
  }) => {
    await openOverlay(page);

    await closeNatively(page.getByRole('dialog'));

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

const phoneMarkup = '<img src=x onerror="document.title=1"><b>Bold</b>';

function bottomBar(page: Page): Locator {
  return page.getByRole('navigation', { name: 'Основная навигация' });
}

async function endsAboveTheBar(page: Page, field: Locator): Promise<boolean> {
  const fieldBox = await field.boundingBox();
  const barBox = await bottomBar(page).boundingBox();
  return fieldBox !== null && barBox !== null && fieldBox.y + fieldBox.height <= barBox.y + 1;
}

function moreSheet(page: Page): Locator {
  return page.getByRole('dialog', { name: 'Ещё' });
}

async function openSheetOnToday(page: Page): Promise<void> {
  await openSettledToday(page);
  await bottomBar(page).getByRole('button', { name: 'Ещё' }).click();
  await expect(moreSheet(page)).toBeVisible();
}

test.describe('below 768 px', () => {
  test.use({ viewport: { width: 360, height: 780 } });

  test('a bottom bar item opens its section, marks it current and focuses the heading', async ({
    signedInPage: page,
  }) => {
    await openSettledToday(page);
    const schedule = bottomBar(page).getByRole('link', { name: 'Расписание' });

    await schedule.click();

    await expect(page).toHaveURL(/\/schedule$/);
    await expect(page.getByRole('heading', { level: 1, name: 'Расписание' })).toBeFocused();
    await expect(schedule).toHaveAttribute('aria-current', 'page');
    await expect(bottomBar(page).getByRole('link', { name: 'Сегодня' })).not.toHaveAttribute('aria-current');
  });

  test('the frame’s icons are 20 px and its targets at least 44 × 44 px', async ({ signedInPage: page }) => {
    await openSheetOnToday(page);

    const sizes = await page.evaluate(() => {
      const frame = [
        ...document.querySelectorAll('header, nav[aria-label="Основная навигация"], dialog[aria-label="Ещё"]'),
      ].filter((element) => element.closest('main') === null);
      const box = (element: Element): { readonly width: number; readonly height: number } => {
        const rect = element.getBoundingClientRect();
        return { width: Math.round(rect.width), height: Math.round(rect.height) };
      };
      return {
        icons: frame.flatMap((part) => [...part.querySelectorAll('svg')].map(box)),
        targets: frame.flatMap((part) => [...part.querySelectorAll('a, button')].map(box)),
      };
    });

    expect(sizes.icons.length).toBeGreaterThanOrEqual(10);
    expect(new Set(sizes.icons.map((icon) => `${String(icon.width)}×${String(icon.height)}`))).toEqual(
      new Set(['20×20']),
    );
    expect(sizes.targets.length).toBeGreaterThanOrEqual(11);
    for (const target of sizes.targets) {
      expect(target.width).toBeGreaterThanOrEqual(44);
      expect(target.height).toBeGreaterThanOrEqual(44);
    }
  });

  test('More opens the sheet on its first section row', async ({ signedInPage: page }) => {
    await openSettledToday(page);

    await bottomBar(page).getByRole('button', { name: 'Ещё' }).click();

    await expect(moreSheet(page).getByRole('link', { name: 'Ученики' })).toBeFocused();
  });

  test('on a page the sheet lists, More opens the sheet on that page’s row', async ({ signedInPage: page }) => {
    await page.goto('/groups');
    await page.getByRole('heading', { level: 1, name: 'Группы' }).waitFor();

    await bottomBar(page).getByRole('button', { name: 'Ещё' }).click();

    const groups = moreSheet(page).getByRole('link', { name: 'Группы' });
    await expect(groups).toBeFocused();
    await expect(groups).toHaveAttribute('aria-current', 'page');
  });

  test('Shift+Tab and Tab stay inside the sheet', async ({ signedInPage: page }) => {
    await openSheetOnToday(page);

    await page.keyboard.press('Shift+Tab');
    const closeFocusedAfterShiftTab = await moreSheet(page)
      .getByRole('button', { name: 'Закрыть' })
      .evaluate((element) => element === document.activeElement);
    await page.keyboard.press('Tab');

    expect(closeFocusedAfterShiftTab).toBe(true);
    await expect(moreSheet(page).getByRole('link', { name: 'Ученики' })).toBeFocused();
  });

  test('Escape closes the sheet and returns focus to More', async ({ signedInPage: page }) => {
    await openSheetOnToday(page);

    await page.keyboard.press('Escape');

    await expect(moreSheet(page)).toBeHidden();
    await expect(bottomBar(page).getByRole('button', { name: 'Ещё' })).toBeFocused();
  });

  test('a close by the browser itself, without a cancel event, returns focus to More', async ({
    signedInPage: page,
  }) => {
    await openSheetOnToday(page);

    await closeNatively(moreSheet(page));

    await expect(moreSheet(page)).toBeHidden();
    await expect(bottomBar(page).getByRole('button', { name: 'Ещё' })).toBeFocused();
  });

  test('Close closes the sheet and returns focus to More', async ({ signedInPage: page }) => {
    await openSheetOnToday(page);

    await moreSheet(page).getByRole('button', { name: 'Закрыть' }).click();

    await expect(moreSheet(page)).toBeHidden();
    await expect(bottomBar(page).getByRole('button', { name: 'Ещё' })).toBeFocused();
  });

  test('a tap on the dimmed page closes the sheet and returns focus to More', async ({ signedInPage: page }) => {
    await openSheetOnToday(page);

    await page.mouse.click(180, 20);

    await expect(moreSheet(page)).toBeHidden();
    await expect(bottomBar(page).getByRole('button', { name: 'Ещё' })).toBeFocused();
  });

  test('a field that Tab focuses behind the bottom bar is scrolled above it', async ({ signedInPage: page }) => {
    await page.goto('/settings/profile');
    const subjects = page.getByLabel('Предметы', { exact: true });
    const about = page.getByLabel('О себе', { exact: true });
    await subjects.focus();
    const aboveBeforeTab = await endsAboveTheBar(page, about);

    await page.keyboard.press('Tab');

    expect(aboveBeforeTab).toBe(false);
    await expect(about).toBeFocused();
    await expect.poll(async () => endsAboveTheBar(page, about)).toBe(true);
  });

  test('a row of the sheet closes it, opens its page and focuses the heading', async ({ signedInPage: page }) => {
    await openSheetOnToday(page);

    await moreSheet(page).getByRole('link', { name: 'Группы' }).click();

    await expect(moreSheet(page)).toBeHidden();
    await expect(page).toHaveURL(/\/groups$/);
    await expect(page.getByRole('heading', { level: 1, name: 'Группы' })).toBeFocused();
  });

  test('widening the window past 768 px closes the sheet and moves focus from it to the page heading', async ({
    signedInPage: page,
  }) => {
    await openSheetOnToday(page);

    await page.setViewportSize({ width: 800, height: 1000 });

    await expect(moreSheet(page)).toHaveCount(0);
    await expect(page.getByRole('heading', { level: 1, name: 'Сегодня' })).toBeFocused();
    await expect(page.getByRole('button', { name: 'Показать боковую панель' })).toBeVisible();
  });

  test('narrowing the window below 768 px moves focus from Show sidebar to the page heading', async ({
    signedInPage: page,
  }) => {
    await page.setViewportSize({ width: 800, height: 1000 });
    await openSettledToday(page);
    await page.getByRole('button', { name: 'Показать боковую панель' }).focus();

    await page.setViewportSize({ width: 360, height: 780 });

    await expect(page.getByRole('heading', { level: 1, name: 'Сегодня' })).toBeFocused();
    await expect(bottomBar(page).getByRole('button', { name: 'Ещё' })).toBeVisible();
  });

  test('narrowing the window from 1440 to 360 px moves focus from a sidebar item to the page heading', async ({
    signedInPage: page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await openSettledToday(page);
    await page
      .getByRole('navigation', { name: 'Основная навигация' })
      .getByRole('link', { name: 'Расписание' })
      .focus();

    await page.setViewportSize({ width: 360, height: 780 });

    await expect(bottomBar(page).getByRole('button', { name: 'Ещё' })).toBeVisible();
    await expect(page.getByRole('heading', { level: 1, name: 'Сегодня' })).toBeFocused();
  });

  test('crossing 768 px keeps focus and a typed value inside the page', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Репетитор с учеником');
    await seedActiveStudent(deployment, tutor, 'Вера Ученица');
    await signInThroughTheApi(page, tutor);
    await page.goto('/students');
    const search = page.getByRole('main').getByRole('searchbox');
    await search.fill('Вера');

    await page.setViewportSize({ width: 800, height: 1000 });

    await expect(page.getByRole('button', { name: 'Показать боковую панель' })).toBeVisible();
    await expect(search).toBeFocused();
    await expect(search).toHaveValue('Вера');
  });

  test('crossing 768 px with the notifications panel open closes it and moves focus to the page heading', async ({
    signedInPage: page,
  }) => {
    await openSettledToday(page);
    await page
      .getByRole('banner')
      .getByRole('button', { name: /^Уведомления/ })
      .click();
    await expect(page.getByRole('region', { name: 'Уведомления' }).getByRole('heading')).toBeFocused();

    await page.setViewportSize({ width: 800, height: 1000 });

    await expect(page.getByRole('region', { name: 'Уведомления' })).toHaveCount(0);
    await expect(page.getByRole('heading', { level: 1, name: 'Сегодня' })).toBeFocused();
  });

  test('the sheet shows an account name with markup as text', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, phoneMarkup);
    await signInThroughTheApi(page, tutor);

    await openSheetOnToday(page);

    await expect(moreSheet(page)).toContainText(phoneMarkup);
    await expect(moreSheet(page).locator('img, b')).toHaveCount(0);
    await expect(page).not.toHaveTitle('1');
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

interface HoldingServer {
  readonly origin: string;
  readonly reached: Promise<void>;
  readonly close: () => Promise<void>;
}

async function closeServer(server: Server): Promise<void> {
  server.closeAllConnections();
  await new Promise<void>((resolve, reject) => {
    server.close((error) => {
      if (error === undefined) resolve();
      else reject(error);
    });
  });
}

function htmlPage(body: string): string {
  return `<!DOCTYPE html><html><head><link rel="icon" href="data:,"></head><body>${body}</body></html>`;
}

async function startHoldingServer(heldAddress: string): Promise<HoldingServer> {
  const heldPath = new URL(heldAddress, 'http://127.0.0.1').pathname;
  let markReached: () => void = () => undefined;
  const reached = new Promise<void>((resolve) => {
    markReached = resolve;
  });
  const server = createServer((request, response) => {
    const path = new URL(request.url ?? '/', 'http://127.0.0.1').pathname;
    if (path === heldPath) {
      markReached();
      return;
    }
    response.setHeader('Content-Type', 'text/html; charset=utf-8');
    response.end(
      path === '/frame'
        ? htmlPage(`<script>fetch(${JSON.stringify(heldAddress)}).catch(() => undefined)</script>`)
        : htmlPage('<iframe src="/frame"></iframe>'),
    );
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const address = server.address();
  if (address === null || typeof address === 'string') throw new Error('the holding server has no port');
  return {
    origin: `http://127.0.0.1:${String(address.port)}`,
    reached,
    close: () => closeServer(server),
  };
}

async function removeFrame(page: Page): Promise<void> {
  await page.evaluate(() => {
    document.querySelector('iframe')?.remove();
  });
}

async function thrownBy(action: () => unknown): Promise<string> {
  try {
    await action();
  } catch (error) {
    return error instanceof Error ? error.message : 'not an error';
  }
  return 'nothing thrown';
}

test('the diagnostics fixture records an unexpected request failure in the redacted details', async ({
  browser,
  browserName,
}, testInfo) => {
  const invitationToken = randomBytes(32).toString('base64url');
  const queryToken = randomBytes(32).toString('base64url');
  const server = await startHoldingServer(
    `/api/v1/invitations/${invitationToken}?token=${queryToken}#token=${queryToken}`,
  );
  const context = await browser.newContext();
  const registry = diagnosticRegistry(browserName);
  await registry.watch(context);
  const page = await context.newPage();
  const watched = watchPage(page, server.origin, registry);
  const failed = page.waitForEvent('requestfailed');
  await page.goto(`${server.origin}/diagnostics`);
  await server.reached;
  let pageFailure: string;
  let registryFailure: string;
  let attached: string;

  try {
    await removeFrame(page);
    await failed;
    watched.recordUnexpected();
    pageFailure = await thrownBy(() => {
      watched.verify();
    });
    registryFailure = await thrownBy(() => registry.verify(testInfo));
    const attachment = testInfo.attachments.find((entry) => entry.name === 'browser-diagnostics');
    attached = attachment?.path === undefined ? 'no attachment' : await readFile(attachment.path, 'utf8');
  } finally {
    await context.close();
    await server.close();
  }

  expect(pageFailure).toContain('request_failure');
  expect(pageFailure).not.toContain('/api/v1/invitations');
  expect(registryFailure).not.toBe('nothing thrown');
  expect(JSON.parse(registryFailure)).toEqual({
    event: 'browser.diagnostics_failed',
    records: [
      {
        category: 'request_failure',
        classification: 'unknown',
        engine: browserName,
        stage: 'journey',
        route: 'unknown',
        count: 1,
      },
    ],
    missingExpectedErrors: 0,
  });
  expect(JSON.parse(attached)).toEqual({
    details: [
      {
        category: 'request_failure',
        engine: browserName,
        stage: 'journey',
        route: 'unknown',
        page: `${server.origin}/diagnostics`,
        method: 'GET',
        path: '/api/v1/invitations/:redacted',
        failure: 'cancellation',
      },
    ],
    omitted: 0,
  });
  for (const evidence of [pageFailure, registryFailure, attached]) {
    expect(evidence).not.toContain(invitationToken);
    expect(evidence).not.toContain(queryToken);
  }
  await rm(testInfo.outputPath(), { recursive: true, force: true });
});
