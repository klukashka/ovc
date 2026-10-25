import type { BrowserContext, Locator, Page } from '@playwright/test';

import { newTestAccount, registerTutor, signInThroughTheApi } from './accounts.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { busyControls, recordBusyControls } from './observers.ts';
import {
  accessToken,
  callApi,
  createCard,
  createGroup,
  invitationIsUsable,
  seedActiveStudent,
  seedInvitedCards,
  signUpOnInvitation,
} from './tutoring.ts';

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

  test('unverified tutor › Students shows the confirmation prompt instead of Add student', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'GET', path: '/api/v1/students?state=active', status: 403 });
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
  });

  test('unverified tutor › teaching settings show the code with the confirmation prompt instead of regenerate', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'GET', path: '/api/v1/tutor/blocked-requesters', status: 403 });
    const tutor = newTestAccount('Неподтверждённый репетитор');
    await registerTutor(deployment, tutor, 'ru');
    await signInThroughTheApi(page, tutor);

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

async function createThroughTheDialog(page: Page, email: string): Promise<Locator> {
  await page.getByRole('button', { name: 'Add student' }).click();
  const form = page.getByRole('dialog', { name: 'New student' });
  await form.getByLabel('Name').fill('Boris');
  await form.getByLabel('Email (optional)').fill(email);
  await form.getByRole('button', { name: 'Create invitation' }).click();
  const ready = page.getByRole('dialog', { name: 'The invitation is ready' });
  await expect(ready.getByRole('heading', { name: 'The invitation is ready' })).toBeFocused();
  return ready;
}

async function withdrawThroughTheDialog(page: Page): Promise<void> {
  await page.getByRole('button', { name: 'Withdraw invitation' }).click();
  await page
    .getByRole('dialog', { name: 'Withdraw the invitation?' })
    .getByRole('button', { name: 'Withdraw', exact: true })
    .click();
}

function listLoaded(page: Page, state: 'active' | 'invited'): Promise<unknown> {
  return page.waitForResponse(
    (response) => new URL(response.url()).search === `?state=${state}` && response.status() === 200,
  );
}

test.describe('in English with the dark theme', () => {
  test.use({ locale: 'en-GB', colorScheme: 'dark' });

  test('students › creating an invitation focuses the ready dialog and Copy link copies its link', async ({
    page,
    browserName,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris', 'student');
    const clipboardReadable = await allowClipboard(page.context(), browserName);
    const addresses = recordAddresses(page);
    await recordBusyControls(page);
    await signInThroughTheApi(page, pavel);
    await page.goto('/students');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    const ready = await createThroughTheDialog(page, boris.email);
    const created = await shownLink(ready);

    await ready.getByRole('button', { name: 'Copy link' }).click();

    await expect(ready.getByRole('status')).toHaveText('Link copied.');
    await expect(ready.getByRole('status')).toBeFocused();
    if (clipboardReadable) {
      expect(await clipboardText(page)).toBe(created);
    }
    expect(await busyControls(page)).toEqual([['Create invitation']]);
    expect(addresses.filter((address) => address.includes('/invite/'))).toEqual([]);
  });

  test('students › Open the card from the ready invitation shows the invitation it created', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris', 'student');
    const addresses = recordAddresses(page);
    await recordBusyControls(page);
    await signInThroughTheApi(page, pavel);
    await page.goto('/students');
    const ready = await createThroughTheDialog(page, boris.email);
    const created = await shownLink(ready);

    await ready.getByRole('link', { name: 'Open the card' }).click();

    await expect(page.getByRole('heading', { level: 1, name: 'Boris' })).toBeFocused();
    await expect(page.getByRole('main').getByText('Invitation sent')).toBeVisible();
    await expect(page.getByText(/^Valid until \d{1,2} [A-Z][a-z]{2}, \d{2}:\d{2}$/)).toBeVisible();
    expect(await shownLink(page.getByRole('main'))).toBe(created);
    expect(await busyControls(page)).toEqual([['Create invitation']]);
    expect(addresses.filter((address) => address.includes('/invite/'))).toEqual([]);
  });

  test('students › after Send by email, Create a new link replaces the emailed link and only the new one works', async ({
    page,
    deployment,
    control,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris', 'student');
    const card = await createCard(deployment, pavel, 'Boris', boris.email);
    const addresses = recordAddresses(page);
    await recordBusyControls(page);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${card.id}`);
    expect(await shownLink(page.getByRole('main'))).toBe(card.link);
    await page.getByRole('button', { name: 'Send by email' }).click();
    await expect(page.getByRole('status')).toHaveText('Sent.');
    await expect(page.getByRole('status')).toBeFocused();
    await control.tick();

    await page.getByRole('button', { name: 'Create a new link' }).click();

    await expect(page.getByRole('status')).toHaveText('A new link was created. The old one no longer works.');
    await expect(page.getByRole('status')).toBeFocused();
    const regenerated = await shownLink(page.getByRole('main'));
    expect(await busyControls(page)).toEqual([['Send by email'], ['Create a new link']]);
    expect(regenerated).not.toBe(card.link);
    expect(await invitationIsUsable(deployment, card.link)).toBe(false);
    expect(await invitationIsUsable(deployment, regenerated)).toBe(true);
    const emailed = invitationLinks((await control.mail(boris.email)).flatMap((message) => message.links));
    expect(emailed).toEqual([card.link]);
    expect(addresses.filter((address) => address.includes('/invite/'))).toEqual([]);
  });

  test('students › withdrawing after Escape and a second opening removes the card and keeps no link', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const card = await createCard(deployment, pavel, 'Boris', newTestAccount('Boris', 'student').email);
    const addresses = recordAddresses(page);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${card.id}`);
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
    await expect(page.getByRole('button', { name: 'Add student' })).toBeVisible();
    expect(await invitationIsUsable(deployment, card.link)).toBe(false);
    await expectNoLinkStored(page);
    expect(addresses.filter((address) => address.includes('/invite/'))).toEqual([]);
  });

  test('students › after a withdrawal, Active, Back and a reload drop the notice and show no open invitations', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const card = await createCard(deployment, pavel, 'Boris', newTestAccount('Boris', 'student').email);
    const addresses = recordAddresses(page);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${card.id}`);
    await withdrawThroughTheDialog(page);
    await expect(page.getByRole('status').filter({ hasText: 'The invitation was withdrawn.' })).toBeVisible();
    const activeLoaded = listLoaded(page, 'active');
    await page.getByRole('link', { name: 'Active', exact: true }).click();
    await activeLoaded;
    await expect(page.getByText(/^[2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4}$/)).toBeVisible();
    await expect(page.getByText('The invitation was withdrawn.')).toHaveCount(0);
    const invitedAgain = listLoaded(page, 'invited');
    await page.goBack();
    await invitedAgain;
    await expect(page.getByText('No open invitations.')).toBeVisible();

    await page.reload();

    await expect(page.getByText('No open invitations.')).toBeVisible();
    await expect(page.getByText('The invitation was withdrawn.')).toHaveCount(0);
    expect(addresses.filter((address) => address.includes('/invite/'))).toEqual([]);
  });

  test('students › the address of a withdrawn card is not found', async ({ page, deployment, diagnostics }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const card = await createCard(deployment, pavel, 'Boris', null);
    expect(
      (await callApi(deployment, await accessToken(deployment, pavel), 'DELETE', `/students/${card.id}`)).status,
    ).toBe(204);
    diagnostics.expectFailure({ method: 'GET', path: `/api/v1/students/${card.id}`, status: 404 });
    const addresses = recordAddresses(page);
    await signInThroughTheApi(page, pavel);

    await page.goto(`/students/${card.id}`);

    await expect(page.getByRole('heading', { level: 1, name: 'Page not found' })).toBeVisible();
    expect(addresses.filter((address) => address.includes('/invite/'))).toEqual([]);
  });

  test('students › the card of an active student opens from its row without invitation controls and keeps saved notes', async ({
    page,
    deployment,
    control,
  }) => {
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
  });

  test('students › a new student with the email of an active student is refused with a link to that card', async ({
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
    await page.getByRole('button', { name: 'Add student' }).click();
    const form = page.getByRole('dialog', { name: 'New student' });
    await form.getByLabel('Name').fill('Boris again');
    await form.getByLabel('Email (optional)').fill(student.email);
    await form.getByRole('button', { name: 'Create invitation' }).click();
    await expect(form.getByRole('alert')).toHaveText('A student with this email already exists.');

    await form.getByRole('link', { name: 'Open the card of this student' }).click();

    await expect(page).toHaveURL(`/students/${cardId}`);
  });

  test('students › the Invited tab lives in the address and Show more loads the remaining invitations', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
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
  });

  test('students › the search lives in the address and Back restores the earlier search', async ({
    page,
    deployment,
    control,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await seedActiveStudent(deployment, control, pavel, 'Vera Active');
    await signInThroughTheApi(page, pavel);
    await page.goto('/students?state=invited');
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

  test('students › Enter in the search field keeps focus on the field and an empty search keeps Add student', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await signInThroughTheApi(page, pavel);
    await page.goto('/students');
    const field = page.getByLabel('Name or email');
    await field.fill('nobody');

    await field.press('Enter');

    await expect(page).toHaveURL('/students?state=active&q=nobody');
    await expect(page.getByText('Nobody matches the search.')).toBeVisible();
    await expect(field).toBeFocused();
    await expect(field).toHaveValue('nobody');
    await expect(page.getByRole('button', { name: 'Add student' })).toBeVisible();
  });

  test('students › the Search button keeps focus after a group is chosen', async ({ page, deployment }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', []);
    await signInThroughTheApi(page, pavel);
    await page.goto('/students');
    const search = page.getByRole('search', { name: 'Search students' });
    await search.getByLabel('Group', { exact: true }).selectOption({ label: 'Evening algebra' });

    await search.getByRole('button', { name: 'Search', exact: true }).press('Enter');

    await expect(page).toHaveURL(`/students?state=active&group=${groupId}`);
    await expect(search.getByRole('button', { name: 'Search', exact: true })).toBeFocused();
    await expect(search.getByLabel('Group', { exact: true })).toHaveValue(groupId);
    await expect(search.getByRole('link', { name: 'Clear search' })).toBeVisible();
  });

  test('students › Clear search empties the filters and moves focus to the search field', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', []);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students?state=active&group=${groupId}&q=vera`);
    const search = page.getByRole('search', { name: 'Search students' });
    await expect(search.getByLabel('Name or email')).toHaveValue('vera');

    await search.getByRole('link', { name: 'Clear search' }).click();

    await expect(page).toHaveURL('/students?state=active');
    await expect(search.getByLabel('Name or email')).toBeFocused();
    await expect(search.getByLabel('Name or email')).toHaveValue('');
    await expect(search.getByLabel('Group', { exact: true })).toHaveValue('');
    await expect(search.getByRole('link', { name: 'Clear search' })).toHaveCount(0);
  });

  test('students › a malformed group in the address is not available and links to all groups', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'GET', path: '/api/v1/students?state=active&group=not-a-group', status: 422 });
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await signInThroughTheApi(page, pavel);

    await page.goto('/students?group=not-a-group');

    await expect(page.getByText('This group is not available.')).toBeVisible();
    await expect(page.getByRole('main').getByRole('link', { name: 'All groups' })).toHaveAttribute(
      'href',
      '/students?state=active',
    );
    await expect(page.getByRole('button', { name: 'Try again' })).toHaveCount(0);
  });

  test('students › an over-long search in the address is explained and links to the list without it', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const longSearch = 'a'.repeat(255);
    diagnostics.expectFailure({ method: 'GET', path: `/api/v1/students?state=active&q=${longSearch}`, status: 422 });
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await signInThroughTheApi(page, pavel);

    await page.goto(`/students?q=${longSearch}`);

    await expect(page.getByText('This search cannot be used. A search holds at most 254 characters.')).toBeVisible();
    await expect(page.getByRole('link', { name: 'Show the list without the search' })).toHaveAttribute(
      'href',
      '/students?state=active',
    );
    await expect(page.getByRole('button', { name: 'Try again' })).toHaveCount(0);
  });

  test('students › a card that changed meanwhile shows its current name and keeps the unsaved notes', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris', 'student');
    const card = await createCard(deployment, pavel, 'Boris', boris.email);
    diagnostics.expectFailure({ method: 'DELETE', path: `/api/v1/students/${card.id}`, status: 409 });
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${card.id}`);
    const notes = page.getByRole('form', { name: 'Notes' }).getByLabel('Notes');
    await notes.fill('Unsaved: prefers evenings.');
    const renamed = await callApi(deployment, await accessToken(deployment, pavel), 'PATCH', `/students/${card.id}`, {
      display_name: 'Boris Petrov',
    });
    expect(renamed.status).toBe(200);
    await signUpOnInvitation(deployment, card.link, boris, 'en');
    await page.getByRole('button', { name: 'Withdraw invitation' }).click();

    await page
      .getByRole('dialog', { name: 'Withdraw the invitation?' })
      .getByRole('button', { name: 'Withdraw', exact: true })
      .click();

    await expect(
      page
        .getByRole('status')
        .filter({ hasText: 'This card changed in the meantime. Its current state is shown below.' }),
    ).toBeFocused();
    await expect(page.getByRole('heading', { level: 1, name: 'Boris Petrov' })).toBeVisible();
    await expect(page.getByRole('form', { name: 'Name and email' }).getByLabel('Name', { exact: true })).toHaveValue(
      'Boris Petrov',
    );
    await expect(notes).toHaveValue('Unsaved: prefers evenings.');
  });
});
