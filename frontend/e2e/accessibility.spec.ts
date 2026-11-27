import type { Page } from '@playwright/test';

import { type TestAccount, newTestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { trafficAfterReady, expectRefetchedAfterTheSocketIsReady } from './realtime.ts';
import { channelOf, postMessage } from './messages.ts';
import { accessToken, callApi, createGroup, seedActiveStudent } from './tutoring.ts';

test.use({ locale: 'ru-RU' });

const heldRequests: (() => Promise<void>)[] = [];

test.afterEach(async () => {
  for (const releaseHeld of heldRequests.splice(0)) {
    await releaseHeld();
  }
});

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
  await traffic.initialize();
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
  await page.getByRole('main').getByRole('button', { name: 'Сменить пароль' }).click();
  const password = page.getByRole('form', { name: 'Сменить пароль' });
  const change = password.getByRole('button', { name: 'Сменить пароль' });
  await password.getByLabel('Текущий пароль').fill(account.password);
  await password.getByLabel('Новый пароль').fill(`${account.password} changed`);
  await change.focus();
  let release: () => void = () => undefined;
  const resumed = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route('**/api/v1/auth/password/change', async (route) => {
    await resumed;
    await route.continue();
  });
  heldRequests.push(async () => {
    const changed = page.waitForResponse('**/api/v1/auth/password/change');
    release();
    await changed;
    await page.unroute('**/api/v1/auth/password/change');
  });

  await page.keyboard.press('Enter');

  await expect(change).toHaveAttribute('aria-busy', 'true');
  await expect(change).toBeFocused();
});

test('a changed password closes its dialog onto the notice', async ({ page, deployment }) => {
  const account = await seedVerifiedUser(deployment, 'Уведомление о смене пароля');
  await signedInAt(page, account, '/settings/account');
  await page.getByRole('main').getByRole('button', { name: 'Сменить пароль' }).click();
  const password = page.getByRole('form', { name: 'Сменить пароль' });
  await password.getByLabel('Текущий пароль').fill(account.password);
  await password.getByLabel('Новый пароль').fill(`${account.password} changed`);

  await password.getByLabel('Новый пароль').press('Enter');

  await expect(page.getByRole('main').getByText('Пароль изменён.')).toBeFocused();
  await expect(password).toHaveCount(0);
});

test('the resend button keeps focus while it waits', async ({ page }) => {
  const account = newTestAccount('Фокус на повторной отправке');
  await page.clock.install();
  await page.goto('/signup/tutor');
  await page.getByLabel('Имя').fill(account.name);
  await page.getByLabel('Email').fill(account.email);
  await page.getByLabel('Пароль', { exact: true }).fill(account.password);
  await page.getByLabel(/^Я даю согласие на обработку моих персональных данных/).check();
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

for (const theme of ['light', 'dark'] as const) {
  test(`the chat filters are a named group of pressed-state buttons in the ${theme} theme`, async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Репетитор фильтров', 'ru');
    const member = await seedActiveStudent(deployment, tutor, 'Ученик фильтров', 'ru');
    await createGroup(deployment, tutor, 'Группа фильтров', [member.cardId]);
    const saved = await callApi(deployment, await accessToken(deployment, tutor), 'PATCH', '/users/me', { theme });
    if (saved.status !== 200) throw new Error('chat filter theme was not saved');
    await signInThroughTheApi(page, tutor);
    const ready = trafficAfterReady(page, ['/api/v1/channels']);
    await ready.initialize();
    await page.goto('/messages');
    await expectRefetchedAfterTheSocketIsReady(ready);
    const chats = page.getByRole('region', { name: 'Чаты', exact: true }).getByRole('link');
    await chats.first().waitFor();
    const filters = page.getByRole('group', { name: 'Фильтр чатов', exact: true });
    const groups = filters.getByRole('button', { name: 'Группы', exact: true });

    await groups.click();

    await page.waitForURL('/messages?filter=group');
    await expect(chats).toHaveCount(1);
    await expect(chats.first()).toContainText('Группа фильтров');
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
    await expect(filters.getByRole('button')).toHaveText(['Все', 'Ученики', 'Группы', 'Непрочитанные']);
    expect(
      await filters
        .getByRole('button')
        .evaluateAll((buttons) => buttons.map((button) => button.getAttribute('aria-pressed'))),
    ).toEqual(['false', 'false', 'true', 'false']);
    await expect(groups).toBeFocused();
  });
}

async function openConversationWithOneMessage(
  page: Page,
  deployment: Deployment,
  theme: 'light' | 'dark',
): Promise<void> {
  const tutor = await seedVerifiedUser(deployment, 'Репетитор', 'ru');
  const { cardId } = await seedActiveStudent(deployment, tutor, 'Ученик', 'ru');
  const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
  await postMessage(deployment, tutor, channel, 'Сообщение для проверки', null);
  const saved = await callApi(deployment, await accessToken(deployment, tutor), 'PATCH', '/users/me', { theme });
  if (saved.status !== 200) throw new Error('theme was not saved');
  await signedInAt(page, tutor, `/messages/${channel}`);
  await page.getByText('Сообщение для проверки', { exact: true }).waitFor();
}

for (const theme of ['light', 'dark'] as const) {
  test(`the message menu's reaction row is a named group of six named pressed-state buttons before Reply in the ${theme} theme`, async ({
    page,
    deployment,
  }) => {
    await openConversationWithOneMessage(page, deployment, theme);

    await page.getByRole('button', { name: 'Действия с сообщением', exact: true }).click();

    const row = page.getByRole('group', { name: 'Реакции', exact: true });
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
    expect(
      await row
        .getByRole('button')
        .evaluateAll((buttons) =>
          buttons.map((button) => [button.getAttribute('aria-label'), button.getAttribute('aria-pressed')]),
        ),
    ).toEqual([
      ['Нравится', 'false'],
      ['Сердце', 'false'],
      ['Смешно', 'false'],
      ['Удивление', 'false'],
      ['Грустно', 'false'],
      ['Готово', 'false'],
    ]);
    await expect(page.getByRole('button', { name: 'Ответить', exact: true })).toBeFocused();
  });

  test(`the emoji picker is a named group of named buttons with one tab stop in the ${theme} theme`, async ({
    page,
    deployment,
  }) => {
    await openConversationWithOneMessage(page, deployment, theme);

    await page.getByRole('button', { name: 'Эмодзи', exact: true }).click();

    const picker = page.getByRole('group', { name: 'Эмодзи', exact: true });
    const choices = picker.getByRole('button');
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
    await expect(choices).toHaveCount(40);
    await expect(choices.first()).toHaveAccessibleName('Улыбающееся лицо');
    await expect(choices.first()).toBeFocused();
    expect(
      await choices.evaluateAll(
        (buttons) => buttons.filter((button) => button.getAttribute('tabindex') !== '-1').length,
      ),
    ).toBe(1);
  });
}
