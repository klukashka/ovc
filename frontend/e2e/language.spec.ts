import type { Page } from '@playwright/test';

import { signInThroughTheApi } from './accounts.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';

import { languageOfChunkFile } from '../scripts/bundle-budget.ts';
import type { Language } from '../src/lib/i18n/languages.ts';

test.use({ locale: 'ru-RU' });

const interfaceLanguage: Language = 'ru';
const otherLanguage: Language = 'en';

type ChunkTreatment = 'deliver' | 'hold' | 'abort';

interface LanguageChunkRoute {
  readonly requested: readonly Language[];
  readonly treatedArrived: Promise<void>;
  readonly release: () => void;
}

function languageOfAddress(address: URL): Language | null {
  return languageOfChunkFile(address.pathname.slice(1));
}

async function routeLanguageChunks(
  page: Page,
  treated: Language | null = null,
  treatment: ChunkTreatment = 'deliver',
): Promise<LanguageChunkRoute> {
  const requested: Language[] = [];
  let markArrived = (): void => undefined;
  let release = (): void => undefined;
  const treatedArrived = new Promise<void>((resolve) => {
    markArrived = resolve;
  });
  const released = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(
    (address) => languageOfAddress(address) !== null,
    async (route) => {
      const language = languageOfAddress(new URL(route.request().url()));
      if (language !== null) {
        requested.push(language);
      }
      if (language !== treated || treatment === 'deliver') {
        await route.continue();
        return;
      }
      markArrived();
      if (treatment === 'abort') {
        await route.abort();
        return;
      }
      await released;
      await route.continue();
    },
  );
  return { requested, treatedArrived, release };
}

interface ShownTexts {
  readonly heading: string | null;
  readonly language: string | null;
}

async function shownTexts(page: Page): Promise<ShownTexts> {
  return {
    heading: await page.getByRole('heading', { level: 1 }).textContent(),
    language: await page.locator('html').getAttribute('lang'),
  };
}

async function storedLanguage(page: Page): Promise<string | null> {
  return page.evaluate(() => globalThis.localStorage.getItem('ovc.language'));
}

test.describe('language', () => {
  test('switching the language loads the other texts before they show', async ({ page }) => {
    const languageChunks = await routeLanguageChunks(page, otherLanguage, 'hold');
    await page.goto('/');
    await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
    const chunksOfTheFirstLoad = [...languageChunks.requested];

    await page.getByRole('button', { name: 'English' }).click();
    await languageChunks.treatedArrived;
    const shownWhileLoading = await shownTexts(page);
    const russianPressedWhileLoading = await page.getByRole('button', { name: 'Русский' }).getAttribute('aria-pressed');
    languageChunks.release();

    expect(chunksOfTheFirstLoad).toEqual([interfaceLanguage]);
    expect(shownWhileLoading).toEqual({ heading: 'Вход', language: interfaceLanguage });
    expect(russianPressedWhileLoading).toBe('true');
    await expect(page.getByRole('heading', { level: 1, name: 'Sign in' })).toBeVisible();
    await expect(page.locator('html')).toHaveAttribute('lang', otherLanguage);
    await expect(page.getByRole('button', { name: 'English' })).toHaveAttribute('aria-pressed', 'true');
  });

  test('a language that fails to load leaves the texts, the stored choice and the account as they were', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    diagnostics.expectAbortedRequest(new RegExp(`^/assets/language-${otherLanguage}-`));
    diagnostics.expectConsoleError('language_load_failed TypeError');
    const account = await seedVerifiedUser(deployment, 'Язык не загрузился');
    const languageChunks = await routeLanguageChunks(page, otherLanguage, 'abort');
    await signInThroughTheApi(page, account);
    await page.goto('/privacy');
    await expect(page.getByRole('button', { name: 'Русский' })).toHaveAttribute('aria-pressed', 'true');
    const shownBefore = await shownTexts(page);
    const storedBefore = await storedLanguage(page);
    const accountSaves: string[] = [];
    page.on('request', (request) => {
      if (request.method() === 'PATCH' && new URL(request.url()).pathname === '/api/v1/users/me') {
        accountSaves.push(request.url());
      }
    });
    const failureLogged = page.waitForEvent('console', (message) => message.text().startsWith('language_load_failed'));

    diagnostics.beginLanguageFailure(otherLanguage);
    await page.getByRole('button', { name: 'English' }).click();
    await languageChunks.treatedArrived;
    await failureLogged;
    await diagnostics.finishLanguageFailure();

    expect(await shownTexts(page)).toEqual(shownBefore);
    expect(shownBefore.language).toBe(interfaceLanguage);
    expect(await storedLanguage(page)).toBe(storedBefore);
    expect(storedBefore).toBe(interfaceLanguage);
    expect(accountSaves).toEqual([]);
    await expect(page.getByRole('button', { name: 'Русский' })).toHaveAttribute('aria-pressed', 'true');
  });
});

test.describe('settings', () => {
  test('a saved language survives a reload', async ({ page, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Сохранённый язык');
    const languageChunks = await routeLanguageChunks(page);
    await signInThroughTheApi(page, account);
    await page.goto('/settings/account');
    const preferences = page.getByRole('form', { name: 'Язык, часовой пояс и тема' });
    await preferences.getByLabel('Язык', { exact: true }).selectOption(otherLanguage);
    await preferences.getByRole('button', { name: 'Сохранить' }).click();
    await expect(page.getByRole('form', { name: 'Language, timezone and theme' })).toBeVisible();
    const chunksBeforeTheReload = languageChunks.requested.length;

    await page.reload();

    await expect(page.getByRole('form', { name: 'Language, timezone and theme' })).toBeVisible();
    await expect(page.locator('html')).toHaveAttribute('lang', otherLanguage);
    await expect(
      page.getByRole('form', { name: 'Language, timezone and theme' }).getByLabel('Language', { exact: true }),
    ).toHaveValue(otherLanguage);
    expect(languageChunks.requested.slice(chunksBeforeTheReload)).toEqual([otherLanguage]);
  });
});
