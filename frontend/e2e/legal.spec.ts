import type { Locator, Page } from '@playwright/test';

import { newTestAccount, signInThroughTheApi } from './accounts.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { createCard } from './tutoring.ts';

const legalWording = /политик|соглашени|согласи|персональн|оферт|privacy|agreement|consent|personal data|offer/iu;

const russianDocuments = [
  { path: '/privacy', title: 'Политика конфиденциальности', firstSection: '1. Общие положения.' },
  { path: '/consent', title: 'Согласие на обработку персональных данных', firstSection: '1. Кто даёт согласие.' },
  { path: '/terms', title: 'Пользовательское соглашение', firstSection: '1. Стороны и акцепт.' },
] as const;

async function textOutsideTheLegalLines(form: Locator): Promise<string> {
  return form.evaluate((element) => {
    const copy = element.cloneNode(true);
    if (!(copy instanceof HTMLElement)) {
      return '';
    }
    for (const allowed of copy.querySelectorAll('label:has(input[type="checkbox"]), [data-acceptance-line]')) {
      allowed.remove();
    }
    return copy.textContent;
  });
}

async function linkTargets(form: Locator): Promise<string[]> {
  return form.locator('a').evaluateAll((links) => links.map((link) => link.getAttribute('href') ?? ''));
}

interface LinkAttributes {
  readonly href: string | null;
  readonly target: string | null;
  readonly rel: string | null;
}

async function attributesOf(link: Locator): Promise<LinkAttributes> {
  return {
    href: await link.getAttribute('href'),
    target: await link.getAttribute('target'),
    rel: await link.getAttribute('rel'),
  };
}

async function legalLinks(form: Locator): Promise<readonly LinkAttributes[]> {
  return [
    await attributesOf(form.getByRole('link', { name: /^обработку моих персональных данных/ })),
    await attributesOf(form.getByRole('link', { name: /^пользовательское соглашение/ })),
  ];
}

const expectedLegalLinks: readonly LinkAttributes[] = [
  { href: '/consent', target: '_blank', rel: 'noopener noreferrer' },
  { href: '/terms', target: '_blank', rel: 'noopener noreferrer' },
];

async function openInvitationForm(page: Page, link: string, choice: string): Promise<Locator> {
  await page.goto(link);
  await page.getByRole('button', { name: choice }).click();
  return page.getByRole('form', { name: choice });
}

test.describe('legal documents in Russian', () => {
  test.use({ locale: 'ru-RU' });

  for (const document of russianDocuments) {
    test(`legal › ${document.path} shows its title, version and draft text signed out`, async ({ page }) => {
      await page.goto(document.path);

      await expect(page.getByRole('heading', { level: 1, name: document.title })).toBeVisible();
      await expect(page.getByText(/^Версия \d{4}-\d{2}-\d{2}\.\d+, /)).toBeVisible();
      await expect(page.getByRole('main').getByText(document.firstSection, { exact: true })).toBeVisible();
      await expect(page.getByRole('main').getByText(/^Черновик\.$/)).toBeVisible();
    });
  }

  test('legal › every document opens signed in', async ({ page, deployment }) => {
    const account = await seedVerifiedUser(deployment, 'Читатель документов');
    await signInThroughTheApi(page, account);
    const shown: string[] = [];

    for (const document of russianDocuments) {
      await page.goto(document.path);
      await page.getByRole('main').getByText(document.firstSection, { exact: true }).waitFor();
      shown.push((await page.getByRole('heading', { level: 1 }).textContent()) ?? '');
    }

    expect(shown).toEqual(russianDocuments.map((document) => document.title));
  });

  test('legal › a document switches to English without a navigation', async ({ page }) => {
    await page.goto('/terms');
    await page.getByRole('heading', { level: 1, name: 'Пользовательское соглашение' }).waitFor();
    const navigations: string[] = [];
    page.on('framenavigated', (frame) => {
      if (frame === page.mainFrame()) {
        navigations.push(frame.url());
      }
    });

    await page.getByRole('button', { name: 'English' }).click();

    await expect(page.getByRole('heading', { level: 1, name: 'User agreement' })).toBeVisible();
    await expect(page.getByRole('main').getByText('1. Parties and acceptance.', { exact: true })).toBeVisible();
    await expect(page.getByRole('main').getByText(/The Russian text is the binding version\./)).toBeVisible();
    expect(navigations).toEqual([]);
  });

  test('legal › the sign-up forms link the consent text and the user agreement and hold no other legal wording', async ({
    page,
  }) => {
    const shown: {
      readonly lineBelowButton: boolean;
      readonly links: readonly LinkAttributes[];
      readonly targets: string[];
      readonly outside: string;
      readonly line: string;
    }[] = [];

    for (const path of ['/signup/tutor', '/signup/student']) {
      await page.goto(path);
      const form = page.locator('form');
      const buttonBox = await form.getByRole('button', { name: 'Зарегистрироваться' }).boundingBox();
      const lineBox = await form.locator('[data-acceptance-line]').boundingBox();
      shown.push({
        lineBelowButton: (lineBox?.y ?? 0) > (buttonBox?.y ?? 0),
        links: await legalLinks(form),
        targets: await linkTargets(form),
        outside: await textOutsideTheLegalLines(form),
        line: (await form.locator('[data-acceptance-line]').textContent()) ?? '',
      });
    }

    expect(shown.map((form) => form.lineBelowButton)).toEqual([true, true]);
    expect(shown.map((form) => form.links)).toEqual([expectedLegalLinks, expectedLegalLinks]);
    expect(shown.map((form) => form.targets)).toEqual([
      ['/consent', '/terms', '/'],
      ['/consent', '/terms', '/'],
    ]);
    expect(shown.every((form) => !legalWording.test(form.outside))).toBe(true);
    expect(
      shown.every((form) => form.line.startsWith('Создавая аккаунт, вы принимаете пользовательское соглашение')),
    ).toBe(true);
  });

  test('legal › a sign-up without the consent is refused and the acceptance line stands under the button', async ({
    page,
  }) => {
    await page.goto('/signup/tutor');
    const form = page.locator('form');
    const account = newTestAccount('Без согласия', 'tutor');
    await form.getByLabel('Имя', { exact: true }).fill(account.name);
    await form.getByLabel('Email', { exact: true }).fill(account.email);
    await form.getByLabel('Пароль', { exact: true }).fill(account.password);
    const registrations: string[] = [];
    page.on('request', (request) => {
      if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/auth/register') {
        registrations.push(request.url());
      }
    });

    await form.getByRole('button', { name: 'Зарегистрироваться' }).click();

    const consentMissing = await form
      .getByLabel(/^Я даю согласие на обработку моих персональных данных/)
      .evaluate((input) => input instanceof HTMLInputElement && input.validity.valueMissing);
    const buttonBox = await form.getByRole('button', { name: 'Зарегистрироваться' }).boundingBox();
    const lineBox = await form.locator('[data-acceptance-line]').boundingBox();
    await expect(form.getByLabel(/^Я даю согласие на обработку моих персональных данных/)).toBeFocused();
    await expect(page).toHaveURL(/\/signup\/tutor$/);
    expect(consentMissing).toBe(true);
    expect(registrations).toEqual([]);
    expect((lineBox?.y ?? 0) > (buttonBox?.y ?? 0)).toBe(true);
  });

  test('legal › the invitation form links the documents without the token and holds no other legal wording', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Мария');
    const card = await createCard(deployment, tutor, 'Анна', null);
    const token = new URL(card.link).pathname.split('/').at(-1) ?? '';

    const form = await openInvitationForm(page, card.link, 'Создать аккаунт');

    const buttonBox = await form.getByRole('button', { name: 'Создать аккаунт и присоединиться' }).boundingBox();
    const lineBox = await form.locator('[data-acceptance-line]').boundingBox();
    expect((lineBox?.y ?? 0) > (buttonBox?.y ?? 0)).toBe(true);
    expect(await legalLinks(form)).toEqual(expectedLegalLinks);
    const targets = await linkTargets(form);
    expect(targets).toEqual(['/consent', '/terms']);
    expect(targets.some((target) => target.includes(token) || target.includes('?') || target.includes('#'))).toBe(
      false,
    );
    expect(await textOutsideTheLegalLines(form)).not.toMatch(legalWording);
  });
});

test.describe('legal documents in English', () => {
  test.use({ locale: 'en-GB' });

  test('legal › the privacy policy shows the English translation with its version', async ({ page }) => {
    await page.goto('/privacy');

    await expect(page.getByRole('heading', { level: 1, name: 'Privacy policy' })).toBeVisible();
    await expect(page.getByText(/^Version \d{4}-\d{2}-\d{2}\.\d+, /)).toBeVisible();
    await expect(page.getByRole('main').getByText('1. General provisions.', { exact: true })).toBeVisible();
  });
});

test('legal › the preview server never answers with a legal text source', async ({ page }) => {
  const addresses = [
    '/src/legal/ru/privacy.md',
    '/src/legal/ru/privacy.md?raw',
    '/src/legal/RU/PRIVACY.MD',
    '/src/legal/ru/privacy%2Emd',
  ];

  const bodies = await Promise.all(addresses.map(async (address) => (await page.request.get(address)).text()));

  expect(bodies.every((body) => !/^version: /m.test(body) && !/^status: /m.test(body))).toBe(true);
});
