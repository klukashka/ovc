import type { BrowserContext, Locator, Page } from '@playwright/test';

import { newTestAccount, registerTutor, signInThroughTheApi } from './accounts.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { invitationIsUsable, seedActiveStudent, seedInvitedCards } from './tutoring.ts';

const knownPreferenceKeys = ['ovc.language', 'ovc.sidebar', 'ovc.theme'];

function invitationLinks(links: readonly string[]): string[] {
  return links.filter((link) => new URL(link).pathname.startsWith('/invite/'));
}

async function shownLink(region: Locator): Promise<string> {
  const link = await region.getByText(/\/invite\//).textContent();
  if (link === null) {
    throw new Error('no invitation link is shown');
  }
  return link.trim();
}

async function allowClipboard(context: BrowserContext, browserName: string): Promise<boolean> {
  if (browserName !== 'chromium') {
    return false;
  }
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  return true;
}

async function clipboardText(page: Page): Promise<string> {
  return page.evaluate(() => navigator.clipboard.readText());
}

function recordAddresses(page: Page): string[] {
  const addresses: string[] = [];
  page.on('framenavigated', (frame) => {
    if (frame === page.mainFrame()) {
      addresses.push(frame.url());
    }
  });
  return addresses;
}

async function expectNoLinkStored(page: Page): Promise<void> {
  const stored = await page.evaluate(() => ({
    local: Object.keys(globalThis.localStorage).sort(),
    localValues: Object.values(globalThis.localStorage).join(' '),
    session: globalThis.sessionStorage.length,
    historyState: JSON.stringify(globalThis.history.state),
  }));
  expect(stored.local.every((key) => knownPreferenceKeys.includes(key))).toBe(true);
  expect(stored.localValues).not.toContain('/invite/');
  expect(stored.session).toBe(0);
  expect(stored.historyState).not.toContain('/invite/');
}

test.describe('in Russian', () => {
  test.use({ locale: 'ru-RU', colorScheme: 'light' });

  test('pilot-1 › step 4 Maria creates a card for Anna and sends the invitation by email', async ({
    page,
    deployment,
    control,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const anna = newTestAccount('Анна', 'student');
    await signInThroughTheApi(page, maria);
    await page.goto('/students');
    await expect(page.getByRole('heading', { level: 2, name: 'Пригласите первого ученика' })).toBeVisible();

    await page.getByRole('button', { name: 'Добавить ученика' }).click();
    const form = page.getByRole('dialog', { name: 'Новый ученик' });
    await expect(form.getByLabel('Имя')).toBeFocused();
    await form.getByLabel('Имя').fill('Анна');
    await form.getByLabel('Email (необязательно)').fill(anna.email);
    await form.getByRole('button', { name: 'Создать приглашение' }).click();
    const ready = page.getByRole('dialog', { name: 'Приглашение готово' });
    await expect(ready.getByRole('heading', { name: 'Приглашение готово' })).toBeFocused();
    const link = await shownLink(ready);
    await ready.getByRole('button', { name: 'Отправить по email' }).click();
    await expect(ready.getByRole('status')).toHaveText('Отправлено.');
    await expect(ready.getByRole('status')).toBeFocused();
    await ready.getByRole('button', { name: 'Готово' }).click();
    await control.tick();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Добавить ученика' })).toBeFocused();
    await page.getByRole('link', { name: 'Приглашённые' }).click();
    await expect(page).toHaveURL('/students?state=invited');
    const row = page.getByRole('link', { name: /Анна/ });
    await expect(row).toContainText(anna.email);
    await expect(row).toContainText('Приглашение отправлено');
    const emailed = invitationLinks((await control.mail(anna.email)).flatMap((message) => message.links));
    expect(emailed).toEqual([link]);
    await row.click();
    await expect(page.getByRole('heading', { level: 1, name: 'Анна' })).toBeFocused();
    expect(await shownLink(page.getByRole('main'))).toBe(link);
  });

  test('unverified tutor › invite and regenerate show the confirmation prompt', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'GET', path: '/api/v1/students?state=active', status: 403 });
    diagnostics.expectFailure({ method: 'GET', path: '/api/v1/tutor/blocked-requesters', status: 403 });
    const tutor = newTestAccount('Неподтверждённый репетитор');
    await registerTutor(deployment, tutor, 'ru');
    await signInThroughTheApi(page, tutor);

    await page.goto('/students');

    await expect(page.getByText('Подтвердите email, чтобы приглашать учеников.')).toBeVisible();
    await expect(page.getByRole('main').getByRole('link', { name: 'Подтвердить email' })).toHaveAttribute(
      'href',
      '/verify-email',
    );
    await expect(page.getByRole('button', { name: 'Добавить ученика' })).toHaveCount(0);
    await page.goto('/settings/teaching');
    await expect(page.getByText(/^[2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4}$/)).toBeVisible();
    await expect(page.getByText('Код начнёт работать, когда вы подтвердите email.')).toBeVisible();
    await expect(page.getByText('Подтвердите email, чтобы код заработал и его можно было обновить.')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Обновить код' })).toHaveCount(0);
    await expect(
      page
        .getByRole('region', { name: 'Заблокированные ученики' })
        .getByText('Список заблокированных учеников доступен после подтверждения email.'),
    ).toBeVisible();
  });
});

test.describe('in English with the dark theme', () => {
  test.use({ locale: 'en-GB', colorScheme: 'dark' });

  test('students › add, copy, send, regenerate, withdraw', async ({
    page,
    browserName,
    deployment,
    control,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris', 'student');
    const clipboardReadable = await allowClipboard(page.context(), browserName);
    const addresses = recordAddresses(page);
    await signInThroughTheApi(page, pavel);
    await page.goto('/students');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');

    await page.getByRole('button', { name: 'Add student' }).click();
    const form = page.getByRole('dialog', { name: 'New student' });
    await form.getByLabel('Name').fill('Boris');
    await form.getByLabel('Email (optional)').fill(boris.email);
    await form.getByRole('button', { name: 'Create invitation' }).click();
    const ready = page.getByRole('dialog', { name: 'The invitation is ready' });
    await expect(ready.getByRole('heading', { name: 'The invitation is ready' })).toBeFocused();
    const created = await shownLink(ready);
    await ready.getByRole('button', { name: 'Copy link' }).click();
    await expect(ready.getByRole('status')).toHaveText('Link copied.');
    await expect(ready.getByRole('status')).toBeFocused();
    if (clipboardReadable) {
      expect(await clipboardText(page)).toBe(created);
    }
    await ready.getByRole('link', { name: 'Open the card' }).click();
    await expect(page.getByRole('heading', { level: 1, name: 'Boris' })).toBeFocused();
    await expect(page.getByRole('main').getByText('Invitation sent')).toBeVisible();
    await expect(page.getByText(/^Valid until \d{1,2} [A-Z][a-z]{2}, \d{2}:\d{2}$/)).toBeVisible();
    expect(await shownLink(page.getByRole('main'))).toBe(created);

    await page.getByRole('button', { name: 'Send by email' }).click();
    await expect(page.getByRole('status')).toHaveText('Sent.');
    await expect(page.getByRole('status')).toBeFocused();
    await control.tick();
    await page.getByRole('button', { name: 'Create a new link' }).click();
    await expect(page.getByRole('status')).toHaveText('A new link was created. The old one no longer works.');
    await expect(page.getByRole('status')).toBeFocused();
    const regenerated = await shownLink(page.getByRole('main'));

    expect(regenerated).not.toBe(created);
    expect(await invitationIsUsable(deployment, created)).toBe(false);
    expect(await invitationIsUsable(deployment, regenerated)).toBe(true);
    const emailed = invitationLinks((await control.mail(boris.email)).flatMap((message) => message.links));
    expect(emailed).toEqual([created]);

    const cardAddress = page.url();
    const opener = page.getByRole('button', { name: 'Withdraw invitation' });
    await opener.click();
    const confirm = page.getByRole('dialog', { name: 'Withdraw the invitation?' });
    await expect(confirm).toContainText('The link stops working and this card is removed.');
    await page.keyboard.press('Escape');
    await expect(opener).toBeFocused();
    await opener.click();
    await expect(confirm.getByRole('button', { name: 'Cancel' })).toBeFocused();
    await confirm.getByRole('button', { name: 'Withdraw', exact: true }).click();

    await expect(page).toHaveURL('/students?state=invited');
    await expect(page.getByRole('status').filter({ hasText: 'The invitation was withdrawn.' })).toBeVisible();
    await expect(page.getByText('No open invitations.')).toBeVisible();
    expect(await invitationIsUsable(deployment, regenerated)).toBe(false);
    await expectNoLinkStored(page);
    const activeLoaded = page.waitForResponse(
      (response) => new URL(response.url()).search === '?state=active' && response.status() === 200,
    );
    await page.getByRole('link', { name: 'Active', exact: true }).click();
    await activeLoaded;
    await expect(page.getByText(/^[2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4}$/)).toBeVisible();
    await expect(page.getByText('The invitation was withdrawn.')).toHaveCount(0);
    const invitedAgain = page.waitForResponse(
      (response) => new URL(response.url()).search === '?state=invited' && response.status() === 200,
    );
    await page.goBack();
    await invitedAgain;
    await expect(page.getByText('No open invitations.')).toBeVisible();
    await page.reload();
    await expect(page.getByText('No open invitations.')).toBeVisible();
    await expect(page.getByText('The invitation was withdrawn.')).toHaveCount(0);
    diagnostics.expectFailure({
      method: 'GET',
      path: new URL(cardAddress).pathname.replace('/students/', '/api/v1/students/'),
      status: 404,
    });
    await page.goto(cardAddress);
    await expect(page.getByRole('heading', { level: 1, name: 'Page not found' })).toBeVisible();
    expect(addresses.filter((address) => address.includes('/invite/'))).toEqual([]);
  });

  test('students › the card of an active student keeps notes and refuses a duplicate email', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/students', status: 409 });
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student, cardId } = await seedActiveStudent(deployment, control, pavel, 'Boris');
    await signInThroughTheApi(page, pavel);
    await page.goto('/students');
    const row = page.getByRole('link', { name: /Boris/ });
    await expect(row).toContainText(student.email);
    await expect(row).not.toContainText('Invitation sent');

    await row.click();
    await expect(page).toHaveURL(`/students/${cardId}`);
    await expect(page.getByRole('heading', { level: 1, name: 'Boris' })).toBeFocused();
    await expect(page.getByRole('heading', { level: 2, name: 'Invitation link' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Withdraw invitation' })).toHaveCount(0);
    const notes = page.getByRole('form', { name: 'Notes' });
    await notes.getByLabel('Notes').fill('Prefers evening lessons.');
    await notes.getByRole('button', { name: 'Save' }).click();
    await expect(notes.getByRole('status')).toHaveText('Saved.');
    await expect(notes.getByRole('status')).toBeFocused();
    await page.reload();
    await expect(page.getByRole('form', { name: 'Notes' }).getByLabel('Notes')).toHaveValue('Prefers evening lessons.');

    await page.goto('/students');
    await page.getByRole('button', { name: 'Add student' }).click();
    const form = page.getByRole('dialog', { name: 'New student' });
    await form.getByLabel('Name').fill('Boris again');
    await form.getByLabel('Email (optional)').fill(student.email);
    await form.getByRole('button', { name: 'Create invitation' }).click();

    await expect(form.getByRole('alert')).toHaveText('A student with this email already exists.');
    await form.getByRole('link', { name: 'Open the card of this student' }).click();
    await expect(page).toHaveURL(`/students/${cardId}`);
  });

  test('students › tabs and search live in the address and pages load on demand', async ({
    page,
    deployment,
    control,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await seedActiveStudent(deployment, control, pavel, 'Vera Active');
    await seedInvitedCards(deployment, pavel, 31);
    await signInThroughTheApi(page, pavel);
    await page.goto('/students?state=invited');
    await expect(page.getByRole('link', { name: 'Invited', exact: true })).toHaveAttribute('aria-current', 'page');
    const rows = page.getByRole('list', { name: 'Invited' }).getByRole('listitem');
    await expect(rows).toHaveCount(30);
    await page.getByRole('button', { name: 'Show more' }).click();
    await expect(rows).toHaveCount(31);
    await expect(rows.nth(30).getByRole('link')).toBeFocused();
    await expect(page.getByRole('button', { name: 'Show more' })).toHaveCount(0);
    const names = await rows.getByText(/^Invited \d{2}$/).allTextContents();
    expect(new Set(names)).toEqual(
      new Set(Array.from({ length: 31 }, (_, index) => `Invited ${String(index + 1).padStart(2, '0')}`)),
    );

    await page.getByRole('link', { name: 'Active', exact: true }).click();
    await expect(page.getByRole('list', { name: 'Active' }).getByRole('link', { name: /Vera Active/ })).toBeVisible();
    await page.getByLabel('Name or email').fill('vera');
    await page.getByRole('button', { name: 'Search', exact: true }).click();

    await expect(page).toHaveURL('/students?state=active&q=vera');
    await expect(page.getByRole('link', { name: 'Clear search' })).toBeVisible();
    await expect(page.getByRole('link', { name: /Vera Active/ })).toBeVisible();
    await page.getByLabel('Name or email').fill('nobody');
    await page.getByRole('button', { name: 'Search', exact: true }).click();
    await expect(page.getByText('Nobody matches the search.')).toBeVisible();
    await page.goBack();
    await expect(page.getByRole('link', { name: /Vera Active/ })).toBeVisible();
    await expect(page.getByLabel('Name or email')).toHaveValue('vera');
  });
});
