import type { BrowserContext, Locator, Page, Request, Response } from '@playwright/test';

import { type TestAccount, newTestAccount, pageLoaded, registerTutor, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import {
  chooseFromPageMenu,
  diagnosedTest as test,
  expect,
  openPageMenu,
  pageMenuButton,
  seedVerifiedUser,
  signUpAndConfirmTutor,
  tableRows,
} from './fixtures.ts';
import {
  apiRequestsInFlight,
  busyControls,
  headingsAtPopstate,
  recordBusyControls,
  recordHeadingsAtPopstate,
} from './observers.ts';
import { createPastSimpleTemplate } from './assignment.ts';
import { issueThroughTheApi } from './homework.ts';
import { expectRefetchedAfterTheSocketIsReady, trafficAfterReady } from './realtime.ts';
import { addDays, cancelThisLesson, localDate, scheduleLessons, scheduleLessonsWithIds } from './schedule.ts';
import {
  accessToken,
  callApi,
  createCard,
  createGroup,
  invitationIsUsable,
  removeStudent,
  seedActiveStudent,
  seedInvitedCards,
  signUpOnInvitation,
} from './tutoring.ts';

interface HeldReads {
  readonly held: Promise<void>;
  readonly sent: () => number;
  readonly release: () => Promise<Response>;
}

function isCardIssueRead(address: URL, cardId: string): boolean {
  return address.pathname === '/api/v1/issues' && address.searchParams.get('student') === cardId;
}

async function holdCardIssueReads(page: Page, cardId: string): Promise<HeldReads> {
  let resume: () => void = () => undefined;
  const resumed = new Promise<void>((resolve) => {
    resume = resolve;
  });
  let reach: () => void = () => undefined;
  const held = new Promise<void>((resolve) => {
    reach = resolve;
  });
  const sent: Request[] = [];
  page.on('request', (request) => {
    if (isCardIssueRead(new URL(request.url()), cardId)) {
      sent.push(request);
    }
  });
  const matches = (address: URL): boolean => isCardIssueRead(address, cardId);
  await page.route(matches, async (route) => {
    reach();
    await resumed;
    await route.continue();
  });
  return {
    held,
    sent: () => sent.length,
    release: async (): Promise<Response> => {
      const answered = page.waitForResponse((response) => isCardIssueRead(new URL(response.url()), cardId));
      resume();
      const response = await answered;
      await page.unroute(matches);
      return response;
    },
  };
}

function isCardRead(request: Request, cardId: string): boolean {
  return request.method() === 'GET' && new URL(request.url()).pathname === `/api/v1/students/${cardId}`;
}

async function holdCardRead(page: Page, cardId: string): Promise<HeldReads> {
  let resume: () => void = () => undefined;
  const resumed = new Promise<void>((resolve) => {
    resume = resolve;
  });
  let reach: () => void = () => undefined;
  const held = new Promise<void>((resolve) => {
    reach = resolve;
  });
  let sent = 0;
  page.on('request', (request) => {
    if (isCardRead(request, cardId)) {
      sent += 1;
    }
  });
  const address = `**/api/v1/students/${cardId}`;
  await page.route(address, async (route) => {
    if (isCardRead(route.request(), cardId)) {
      reach();
      await resumed;
    }
    await route.continue();
  });
  return {
    held,
    sent: () => sent,
    release: async (): Promise<Response> => {
      const answered = page.waitForResponse((response) => isCardRead(response.request(), cardId));
      resume();
      const response = await answered;
      await page.unroute(address);
      return response;
    },
  };
}

async function chooseTabUntilCurrent(tab: Locator): Promise<void> {
  await tab.click();
  await expect(tab).toHaveAttribute('aria-current', 'page');
}

const knownPreferenceKeys = ['ovc.language', 'ovc.sidebar', 'ovc.theme'];

async function secondPageCursor(deployment: Deployment, tutor: TestAccount, cardId: string): Promise<string> {
  const firstPage = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'GET',
    `/students/${cardId}/lessons`,
  );
  const body = firstPage.body;
  const cursor = typeof body === 'object' && body !== null && 'next_cursor' in body ? body.next_cursor : null;
  if (typeof cursor !== 'string') {
    throw new Error('the card has a single page of lessons');
  }
  return cursor;
}

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

function recordCardWrites(page: Page, cardId: string): string[] {
  const writes: string[] = [];
  page.on('request', (request) => {
    if (request.method() === 'PATCH' && new URL(request.url()).pathname === `/api/v1/students/${cardId}`) {
      writes.push(request.method());
    }
  });
  return writes;
}

async function storedNotes(deployment: Deployment, tutor: TestAccount, cardId: string): Promise<unknown> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'GET', `/students/${cardId}`);
  return typeof answer.body === 'object' && answer.body !== null && 'notes' in answer.body ? answer.body.notes : null;
}

async function expectNothingStoredContaining(page: Page, text: string): Promise<void> {
  const stored = await page.evaluate(() => ({
    local: Object.keys(globalThis.localStorage).sort(),
    localValues: Object.values(globalThis.localStorage).join(' '),
    session: globalThis.sessionStorage.length,
    historyState: JSON.stringify(globalThis.history.state),
    address: globalThis.location.href,
  }));
  expect(stored.local.every((key) => knownPreferenceKeys.includes(key))).toBe(true);
  expect(stored.localValues).not.toContain(text);
  expect(stored.session).toBe(0);
  expect(stored.historyState).not.toContain(text);
  expect(decodeURIComponent(stored.address)).not.toContain(text);
}

test.describe('in Russian', () => {
  test.use({ locale: 'ru-RU', colorScheme: 'light' });

  test('pilot-1 › step 4 Maria creates a card for Anna and sends the invitation by email', async ({
    page,
    deployment,
    control,
  }) => {
    const maria = await signUpAndConfirmTutor(deployment, 'Мария');
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
    await expect(page.getByRole('button', { name: 'Добавить ученика' })).toHaveCount(1);
    await expect(page.locator('main header').getByRole('button', { name: 'Добавить ученика' })).toBeFocused();
    await page.getByRole('link', { name: 'Приглашённые' }).click();
    await expect(page).toHaveURL('/students?state=invited');
    const row = tableRows(page, 'Приглашённые').filter({ hasText: 'Анна' });
    await expect(row).toContainText(anna.email);
    await expect(row).toContainText('Приглашение отправлено');
    const emailed = invitationLinks((await control.mail(anna.email)).flatMap((message) => message.links));
    expect(emailed).toEqual([link]);
    await row.getByRole('link', { name: 'Анна' }).click();
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
  await chooseFromPageMenu(page, 'Card actions', 'Withdraw invitation');
  await page
    .getByRole('dialog', { name: 'Withdraw the invitation?' })
    .getByRole('button', { name: 'Withdraw invitation', exact: true })
    .click();
}

function listLoaded(page: Page, state: 'active' | 'invited'): Promise<unknown> {
  return page.waitForResponse(
    (response) => new URL(response.url()).search === `?state=${state}` && response.status() === 200,
  );
}

test.describe('in English with the dark theme', () => {
  test.use({ locale: 'en-GB', colorScheme: 'dark' });

  test('student card › while its texts load the card keeps its heading, which keeps focus once they arrive', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await seedActiveStudent(deployment, pavel, 'Boris');
    let release: () => void = () => undefined;
    const released = new Promise<void>((resolve) => {
      release = resolve;
    });
    const held = page.waitForRequest(/\/assets\/cardTexts\.(en|ru)-[^/]+\.js$/);
    await page.route(/\/assets\/cardTexts\.(en|ru)-[^/]+\.js$/, async (route) => {
      await released;
      await route.continue();
    });
    await signInThroughTheApi(page, pavel);
    await page.goto('/students');
    await page.getByRole('link', { name: 'Boris', exact: true }).click();
    await held;
    await page.waitForFunction(
      () => document.activeElement instanceof HTMLHeadingElement && document.activeElement.matches('main h1'),
    );
    const sectionsWhileLoading = await page.getByRole('navigation', { name: 'Card sections' }).count();

    release();

    expect(sectionsWhileLoading).toBe(0);
    await expect(page.getByRole('navigation', { name: 'Card sections' })).toBeVisible();
    await expect(page.getByRole('heading', { level: 1, name: 'Boris' })).toBeFocused();
  });

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
    const mainColumn = page.locator('[data-column="main"]');
    await page.getByRole('button', { name: 'Send by email' }).click();
    await expect(mainColumn.getByRole('status')).toHaveText('Sent.');
    await expect(mainColumn.getByRole('status')).toBeFocused();
    await control.tick();

    await page.getByRole('button', { name: 'Create a new link' }).click();

    await expect(mainColumn.getByRole('status')).toHaveText('A new link was created. The old one no longer works.');
    await expect(mainColumn.getByRole('status')).toBeFocused();
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
    const opener = pageMenuButton(page, 'Card actions');
    await chooseFromPageMenu(page, 'Card actions', 'Withdraw invitation');
    const confirm = page.getByRole('dialog', { name: 'Withdraw the invitation?' });
    await expect(confirm).toContainText('The link stops working and this card is removed.');
    await page.keyboard.press('Escape');
    await expect(opener).toBeFocused();
    await chooseFromPageMenu(page, 'Card actions', 'Withdraw invitation');
    await expect(confirm.getByRole('button', { name: 'Cancel' })).toBeFocused();

    await confirm.getByRole('button', { name: 'Withdraw invitation', exact: true }).click();

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

    await pageLoaded(page);
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

  test('students › the card of an active student opens from its row without invitation controls and autosaves its notes', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student, cardId } = await seedActiveStudent(deployment, pavel, 'Boris');
    const inFlight = apiRequestsInFlight(page);
    await signInThroughTheApi(page, pavel);
    await page.goto('/students');
    const row = tableRows(page, 'Active').filter({ hasText: 'Boris' });
    await expect(row).toContainText(student.email);
    await expect(row).not.toContainText('Invitation sent');
    await row.getByRole('link', { name: 'Boris' }).click();
    await expect(page).toHaveURL(`/students/${cardId}`);
    await expect(page.getByRole('heading', { level: 1, name: 'Boris' })).toBeFocused();
    await expect(page.getByRole('heading', { level: 2, name: 'Invitation link' })).toHaveCount(0);
    const actions = await openPageMenu(page, 'Card actions');
    await expect(actions.getByRole('button', { name: 'End working together' })).toBeVisible();
    await expect(actions.getByRole('button', { name: 'Withdraw invitation' })).toHaveCount(0);
    await page.keyboard.press('Escape');
    const writes = recordCardWrites(page, cardId);
    const notes = page.getByRole('form', { name: 'Notes' });
    const cardRefetched = page.waitForResponse(
      (response) =>
        response.request().method() === 'GET' && new URL(response.url()).pathname === `/api/v1/students/${cardId}`,
    );
    await notes.getByLabel('Notes').fill('Prefers evening lessons.');
    await expect(notes.getByRole('status')).toHaveText('Saved.');
    await expect(notes.getByLabel('Notes')).toBeFocused();
    await expect(notes.getByRole('button')).toHaveCount(0);
    await cardRefetched;
    expect(writes).toEqual(['PATCH']);
    expect(await storedNotes(deployment, pavel, cardId)).toBe('Prefers evening lessons.');
    await expect.poll(inFlight).toBe(0);

    await page.reload();

    await expect(page.getByRole('form', { name: 'Notes' }).getByLabel('Notes')).toHaveValue('Prefers evening lessons.');
  });

  test('student card › at 1440 × 900 the notes of an active card are in view without scrolling', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, pavel, 'Boris');
    await signInThroughTheApi(page, pavel);
    await page.setViewportSize({ width: 1440, height: 900 });

    await page.goto(`/students/${cardId}`);
    const notes = page.getByRole('form', { name: 'Notes' }).getByLabel('Notes');
    await notes.waitFor();

    await expect(notes).toBeInViewport({ ratio: 1 });
    expect(await page.evaluate(() => globalThis.scrollY)).toBe(0);
    await expect(page.getByRole('complementary', { name: 'Notes' })).toContainText('Only you see this.');
  });

  test('student card › notes typed and reloaded before the save keep nothing in storage or the address', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, pavel, 'Boris');
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${cardId}`);
    const draft = 'Draft that must not be stored';
    await page.getByRole('form', { name: 'Notes' }).getByLabel('Notes').fill(draft);

    await expectNothingStoredContaining(page, draft);
    await pageLoaded(page);
    await page.reload();
    await page.getByRole('form', { name: 'Notes' }).getByLabel('Notes').waitFor();

    await expectNothingStoredContaining(page, draft);
  });

  test('student card › notes the server refuses keep the text and say they were not saved, and Try again keeps focus in the notes', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, pavel, 'Boris');
    diagnostics.expectFailure({ method: 'PATCH', path: `/api/v1/students/${cardId}`, status: 422 });
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${cardId}`);
    const form = page.getByRole('form', { name: 'Notes' });
    const notes = form.getByLabel('Notes');
    const refused = 'Bell \u0007 character';
    await notes.focus();
    await notes.evaluate((field, value) => {
      if (!(field instanceof HTMLTextAreaElement)) throw new Error('the notes field is not a text area');
      field.setRangeText(value, 0, field.value.length, 'end');
      field.dispatchEvent(new Event('input', { bubbles: true }));
    }, refused);

    await form.getByRole('status').filter({ hasText: 'Notes were not saved.' }).waitFor();
    await diagnostics.finishHttpPhase();
    diagnostics.expectFailure({ method: 'PATCH', path: `/api/v1/students/${cardId}`, status: 422 });
    const failures = page.waitForResponse(
      (response) => response.request().method() === 'PATCH' && response.status() === 422,
    );

    await form.getByRole('button', { name: 'Try again' }).focus();
    await page.keyboard.press('Enter');
    await failures;

    await expect(notes).toBeFocused();
    await expect(notes).toHaveValue(refused);
    await expect(form.getByRole('status')).toHaveText('Notes were not saved.');
    await expect(form.getByRole('button', { name: 'Try again' })).toBeVisible();
  });

  test('student card › notes corrected after a refusal are saved', async ({ page, deployment, diagnostics }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, pavel, 'Boris');
    diagnostics.expectFailure({ method: 'PATCH', path: `/api/v1/students/${cardId}`, status: 422 });
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${cardId}`);
    const form = page.getByRole('form', { name: 'Notes' });
    const notes = form.getByLabel('Notes');
    const refused = 'Bell \u0007 character';
    await notes.focus();
    await notes.evaluate((field, value) => {
      if (!(field instanceof HTMLTextAreaElement)) throw new Error('the notes field is not a text area');
      field.setRangeText(value, 0, field.value.length, 'end');
      field.dispatchEvent(new Event('input', { bubbles: true }));
    }, refused);
    await form.getByRole('status').filter({ hasText: 'Notes were not saved.' }).waitFor();

    await notes.fill('Prefers mornings.');
    await expect(form.getByRole('status')).toHaveText('Saved.');

    await expect(form.getByRole('button', { name: 'Try again' })).toHaveCount(0);
    expect(await storedNotes(deployment, pavel, cardId)).toBe('Prefers mornings.');
  });

  test('student card › an invited card edits its email in Name and email…, which replaces the link, and an active card offers no email field', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const invited = await createCard(deployment, pavel, 'Boris', newTestAccount('Boris', 'student').email);
    const { cardId: activeCard } = await seedActiveStudent(deployment, pavel, 'Vera');
    const newEmail = newTestAccount('Boris', 'student').email;
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${activeCard}`);
    await chooseFromPageMenu(page, 'Card actions', 'Name and email…');
    const activeDialog = page.getByRole('dialog', { name: 'Name and email' });
    await activeDialog.getByLabel('Name', { exact: true }).waitFor();
    const activeEmailFields = await activeDialog.getByLabel('Email (optional)').count();
    await page.keyboard.press('Escape');
    await pageLoaded(page);
    await page.goto(`/students/${invited.id}`);
    await chooseFromPageMenu(page, 'Card actions', 'Name and email…');
    const dialog = page.getByRole('dialog', { name: 'Name and email' });
    const email = dialog.getByLabel('Email (optional)');

    await email.fill(newEmail);
    await dialog.getByRole('button', { name: 'Save' }).click();
    await dialog.waitFor({ state: 'hidden' });

    expect(activeEmailFields).toBe(0);
    await expect(page.getByRole('status').filter({ hasText: 'Saved.' })).toBeFocused();
    await expect(page.getByRole('main')).toContainText(newEmail);
    expect(await shownLink(page.getByRole('main'))).not.toBe(invited.link);
  });

  test('student card › Name and email… edits the name in a dialog that opens on its first field and closes onto Saved', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const card = await createCard(deployment, pavel, 'Boris', newTestAccount('Boris', 'student').email);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${card.id}`);
    await chooseFromPageMenu(page, 'Card actions', 'Name and email…');
    const dialog = page.getByRole('dialog', { name: 'Name and email' });
    await expect(dialog.getByLabel('Name', { exact: true })).toBeFocused();
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
    await expect(pageMenuButton(page, 'Card actions')).toBeFocused();
    await chooseFromPageMenu(page, 'Card actions', 'Name and email…');
    await dialog.getByLabel('Name', { exact: true }).fill('Boris Petrov');

    await dialog.getByRole('button', { name: 'Save' }).click();

    await expect(dialog).toBeHidden();
    await expect(page.getByRole('status').filter({ hasText: 'Saved.' })).toBeFocused();
    await expect(page.getByRole('heading', { level: 1, name: 'Boris Petrov' })).toBeVisible();
    expect(await shownLink(page.getByRole('main'))).toBe(card.link);
  });

  for (const { chosen, from } of [
    { chosen: 'Lessons', from: '' },
    { chosen: 'Overview', from: '?tab=lessons' },
    { chosen: 'Homework', from: '' },
  ] as const) {
    test(`student card › choosing ${chosen} keeps focus on it`, async ({ page, deployment }) => {
      const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
      const { cardId } = await seedActiveStudent(deployment, pavel, 'Boris');
      await signInThroughTheApi(page, pavel);
      await page.goto(`/students/${cardId}${from}`);
      const tab = page.getByRole('navigation', { name: 'Card sections' }).getByRole('link', { name: chosen });
      await tab.focus();

      await page.keyboard.press('Enter');

      await expect(tab).toHaveAttribute('aria-current', 'page');
      await expect(tab).toBeFocused();
    });
  }

  for (const { chosen, shown } of [
    { chosen: 'Lessons', shown: 'No lessons yet.' },
    { chosen: 'Homework', shown: 'No assignments yet.' },
  ] as const) {
    test(`student card › choosing ${chosen} while Overview's homework is loading lets that read finish and sends it once`, async ({
      page,
      deployment,
    }) => {
      const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
      const { cardId } = await seedActiveStudent(deployment, pavel, 'Boris');
      await signInThroughTheApi(page, pavel);
      const traffic = trafficAfterReady(page, [`/api/v1/students/${cardId}`]);
      await traffic.initialize();
      await page.goto(`/students/${cardId}?tab=lessons`);
      await expectRefetchedAfterTheSocketIsReady(traffic);
      const reads = await holdCardIssueReads(page, cardId);
      const sections = page.getByRole('navigation', { name: 'Card sections' });
      await sections.getByRole('link', { name: 'Overview' }).click();
      await reads.held;
      const tab = sections.getByRole('link', { name: chosen });

      await chooseTabUntilCurrent(tab);
      const answer = await reads.release();

      expect(answer.status()).toBe(200);
      expect(reads.sent()).toBe(1);
      await expect(page.getByRole('main').getByText(shown)).toBeVisible();
    });
  }

  test('student card › Lessons lists the history and Schedule a lesson preselects the card', async ({
    page,
    deployment,
    control,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, pavel, 'Boris');
    const tomorrow = addDays(localDate(await control.now(), 'Europe/Moscow'), 1);
    const oneOff = { relationship_id: cardId, title: 'Lesson', date: tomorrow, duration_minutes: 60 };
    await scheduleLessons(deployment, pavel, { ...oneOff, start: '10:00' });
    const [later] = await scheduleLessonsWithIds(deployment, pavel, { ...oneOff, start: '12:00' });
    if (later === undefined) {
      throw new Error('no lesson was scheduled');
    }
    await cancelThisLesson(deployment, pavel, later.id);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${cardId}?tab=lessons`);
    const opener = page.getByRole('button', { name: 'Schedule a lesson' });
    await opener.click();
    await expect(page.getByRole('heading', { level: 1, name: 'New lesson' })).toBeFocused();
    const dialog = page.getByRole('main');
    await dialog.getByLabel('Date', { exact: true }).fill(addDays(tomorrow, 1));
    await dialog.getByLabel('Start time').fill('09:00');

    await dialog.getByRole('button', { name: 'Create lesson', exact: true }).click();

    await expect(page.getByRole('heading', { level: 1, name: 'New lesson' })).toBeHidden();
    await expect(opener).toBeFocused();
    const history = page.getByRole('list', { name: 'Lessons', exact: true }).getByRole('listitem');
    await expect(history).toHaveText([
      /09:00–10:00.*Lesson.*Boris$/,
      /12:00–13:00.*Lesson.*Boris.*Cancelled/,
      /10:00–11:00.*Lesson.*Boris$/,
    ]);
  });

  test('student card › Show more in Lessons moves focus to the first new lesson', async ({
    page,
    deployment,
    control,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, pavel, 'Boris');
    const tomorrow = addDays(localDate(await control.now(), 'Europe/Moscow'), 1);
    await scheduleLessons(deployment, pavel, {
      relationship_id: cardId,
      title: 'Lesson',
      date: tomorrow,
      start: '10:00',
      duration_minutes: 60,
      weekdays: [1, 2, 3, 4, 5, 6, 7],
      count: 31,
    });
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${cardId}?tab=lessons`);

    await page.getByRole('button', { name: 'Show more' }).click();

    const history = page.getByRole('list', { name: 'Lessons', exact: true }).getByRole('listitem');
    await expect(history).toHaveCount(31);
    await expect(history.nth(30).getByRole('link')).toBeFocused();
  });

  test('student card › Show more for a card removed meanwhile shows that the student is no longer on the list', async ({
    page,
    deployment,
    control,
    diagnostics,
    socketSlots,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris', 'student');
    const card = await createCard(deployment, pavel, 'Boris', boris.email);
    await signUpOnInvitation(deployment, card.link, boris, 'en');
    const tomorrow = addDays(localDate(await control.now(), 'Europe/Moscow'), 1);
    await scheduleLessons(deployment, pavel, {
      relationship_id: card.id,
      title: 'Lesson',
      date: tomorrow,
      start: '10:00',
      duration_minutes: 60,
      weekdays: [1, 2, 3, 4, 5, 6, 7],
      count: 31,
    });
    const cursor = await secondPageCursor(deployment, pavel, card.id);
    diagnostics.expectFailure({
      method: 'GET',
      path: `/api/v1/students/${card.id}/lessons?cursor=${encodeURIComponent(cursor)}`,
      status: 404,
    });
    diagnostics.expectFailure({ method: 'GET', path: `/api/v1/students/${card.id}`, status: 404 });
    await socketSlots.keepPageSocketsOff(pavel);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${card.id}?tab=lessons`);
    const showMore = page.getByRole('button', { name: 'Show more' });
    await showMore.waitFor();
    await removeStudent(deployment, pavel, card.id);

    await showMore.press('Enter');

    await expect(page.getByRole('status').filter({ hasText: 'This student is no longer on your list.' })).toBeFocused();
    await expect(page.getByRole('main').getByRole('link', { name: 'All students' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Try again' })).toHaveCount(0);
    await expect(page.getByRole('list', { name: 'Lessons', exact: true })).toHaveCount(0);
  });

  test('student card › the only next lesson cancelled elsewhere while focused moves focus to the page heading', async ({
    page,
    deployment,
    control,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, pavel, 'Boris');
    const tomorrow = addDays(localDate(await control.now(), 'Europe/Moscow'), 1);
    const [lesson] = await scheduleLessonsWithIds(deployment, pavel, {
      relationship_id: cardId,
      title: 'Lesson',
      date: tomorrow,
      start: '10:00',
      duration_minutes: 60,
    });
    if (lesson === undefined) {
      throw new Error('no lesson was scheduled');
    }
    await signInThroughTheApi(page, pavel);
    const traffic = trafficAfterReady(page, [`/api/v1/students/${cardId}`]);
    await traffic.initialize();
    await page.goto(`/students/${cardId}`);
    await expectRefetchedAfterTheSocketIsReady(traffic);
    await page.getByRole('list', { name: 'Next lessons', exact: true }).getByRole('link').focus();

    await cancelThisLesson(deployment, pavel, lesson.id);

    await expect(page.getByRole('main').getByText('No lessons scheduled.')).toBeVisible();
    await expect(page.getByRole('heading', { level: 1, name: 'Boris' })).toBeFocused();
  });

  test('students › a row shows the student’s next lesson', async ({ page, deployment, control }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, pavel, 'Boris');
    const tomorrow = addDays(localDate(await control.now(), 'Europe/Moscow'), 1);
    await scheduleLessons(deployment, pavel, {
      relationship_id: cardId,
      title: 'Lesson',
      date: tomorrow,
      start: '10:00',
      duration_minutes: 60,
    });
    await signInThroughTheApi(page, pavel);

    await page.goto('/students');

    await expect(tableRows(page, 'Active').filter({ hasText: 'Boris' })).toContainText(/Next lesson.*10:00–11:00/);
  });

  test('students › a new student with the email of an active student is refused with a link to that card', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/students', status: 409 });
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student, cardId } = await seedActiveStudent(deployment, pavel, 'Boris');
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
    const rows = tableRows(page, 'Invited');
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
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await seedActiveStudent(deployment, pavel, 'Vera Active');
    await signInThroughTheApi(page, pavel);
    await page.goto('/students?state=invited');
    await page.getByRole('link', { name: 'Active', exact: true }).click();
    await expect(tableRows(page, 'Active').getByRole('link', { name: /Vera Active/ })).toBeVisible();
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

  test('students › Back while the card is still loading commits the student list at the address change', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, pavel, 'Boris');
    await signInThroughTheApi(page, pavel);
    await page.goto('/students');
    await expect(page.getByRole('heading', { level: 1, name: 'Students' })).toBeVisible();
    const cardRead = await holdCardRead(page, cardId);
    await tableRows(page, 'Active').getByRole('link', { name: 'Boris' }).click();
    await expect(page).toHaveURL(`/students/${cardId}`);
    await cardRead.held;
    await recordHeadingsAtPopstate(page);

    await page.goBack();
    const cardAnswer = await cardRead.release();

    await expect(page).toHaveURL('/students');
    await expect(page.getByRole('heading', { level: 1, name: 'Students' })).toBeVisible();
    expect(await headingsAtPopstate(page)).toEqual(['Students']);
    expect(cardAnswer.status()).toBe(200);
    expect(cardRead.sent()).toBe(1);
  });

  test.describe('Pavel with one empty group, shared by the tests below', () => {
    test.describe.configure({ mode: 'default' });

    test('students › Enter in the search field keeps focus on the field and an empty search keeps Add student', async ({
      page,
      pavelAndAGroup: { pavel },
    }) => {
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

    test('students › the Search button keeps focus after a group is chosen', async ({
      page,
      pavelAndAGroup: { pavel, groupId },
    }) => {
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
      pavelAndAGroup: { pavel, groupId },
    }) => {
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
      diagnostics,
      pavelAndAGroup: { pavel },
    }) => {
      diagnostics.expectFailure({
        method: 'GET',
        path: '/api/v1/students?state=active&group=not-a-group',
        status: 422,
      });
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
      diagnostics,
      pavelAndAGroup: { pavel },
    }) => {
      const longSearch = 'a'.repeat(255);
      diagnostics.expectFailure({
        method: 'GET',
        path: `/api/v1/students?state=active&q=${longSearch}`,
        status: 422,
      });
      await signInThroughTheApi(page, pavel);

      await page.goto(`/students?q=${longSearch}`);

      await expect(page.getByText('This search cannot be used. A search holds at most 254 characters.')).toBeVisible();
      await expect(page.getByRole('link', { name: 'Show the list without the search' })).toHaveAttribute(
        'href',
        '/students?state=active',
      );
      await expect(page.getByRole('button', { name: 'Try again' })).toHaveCount(0);
    });
  });

  test('students › a card that changed meanwhile shows its current name and keeps the notes typed before it', async ({
    socketSlots,
    page,
    deployment,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris', 'student');
    const card = await createCard(deployment, pavel, 'Boris', boris.email);
    diagnostics.expectFailure({ method: 'DELETE', path: `/api/v1/students/${card.id}`, status: 409 });
    await socketSlots.keepPageSocketsOff(pavel);
    let releaseNotes = (): void => undefined;
    const notesHeld = new Promise<void>((resolve) => {
      releaseNotes = resolve;
    });
    await page.route(`**/api/v1/students/${card.id}`, async (route) => {
      if (route.request().method() === 'PATCH') {
        await notesHeld;
      }
      await route.fallback();
    });
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${card.id}`);
    const notes = page.getByRole('form', { name: 'Notes' }).getByLabel('Notes');
    await notes.fill('Typed before: prefers evenings.');
    const renamed = await callApi(deployment, await accessToken(deployment, pavel), 'PATCH', `/students/${card.id}`, {
      display_name: 'Boris Petrov',
    });
    expect(renamed.status).toBe(200);
    await signUpOnInvitation(deployment, card.link, boris, 'en');
    await chooseFromPageMenu(page, 'Card actions', 'Withdraw invitation');

    await page
      .getByRole('dialog', { name: 'Withdraw the invitation?' })
      .getByRole('button', { name: 'Withdraw invitation', exact: true })
      .click();
    const changedNotice = page
      .getByRole('status')
      .filter({ hasText: 'This card changed in the meantime. Its current state is shown below.' });
    await changedNotice.waitFor();
    const noticeFocused = await page
      .waitForFunction(
        (text) => document.activeElement?.textContent.includes(text) === true,
        'This card changed in the meantime.',
      )
      .then(
        () => true,
        () => false,
      );
    const notesWhileHeld = await notes.inputValue();
    const notesSaved = page.waitForResponse(
      (response) => response.request().method() === 'PATCH' && response.url().endsWith(`/students/${card.id}`),
    );
    releaseNotes();
    const savedStatus = (await notesSaved).status();

    expect(noticeFocused).toBe(true);
    expect(notesWhileHeld).toBe('Typed before: prefers evenings.');
    expect(savedStatus).toBe(200);
    expect(await storedNotes(deployment, pavel, card.id)).toBe('Typed before: prefers evenings.');
    await expect(page.getByRole('heading', { level: 1, name: 'Boris Petrov' })).toBeVisible();
    await expect(notes).toHaveValue('Typed before: prefers evenings.');
    await chooseFromPageMenu(page, 'Card actions', 'Name and email…');
    await expect(page.getByRole('form', { name: 'Name and email' }).getByLabel('Name', { exact: true })).toHaveValue(
      'Boris Petrov',
    );
  });
});

async function layoutBox(
  locator: Locator,
): Promise<{ readonly y: number; readonly height: number; readonly width: number }> {
  const box = await locator.boundingBox();
  if (box === null) {
    throw new Error('the element has no layout box');
  }
  return box;
}

test.describe('the open assignments column', () => {
  test.use({ locale: 'en-GB', viewport: { width: 1440, height: 900 }, colorScheme: 'light' });

  test('shows each student’s open work in a column, and under the name with the groups on a phone below stacked filters', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Tutor', 'en');
    const { cardId } = await seedActiveStudent(deployment, tutor, 'Vera Student', 'en');
    await seedActiveStudent(deployment, tutor, 'Oleg Student', 'en');
    await createGroup(deployment, tutor, 'Evening group', [cardId]);
    const template = await createPastSimpleTemplate(deployment, tutor);
    await issueThroughTheApi(deployment, tutor, template, { relationship_id: cardId });
    await signInThroughTheApi(page, tutor);
    await page.goto('/students');
    const vera = page.getByRole('row', { name: /Vera Student/ });
    await vera.waitFor();

    const wide = {
      header: await page.getByRole('columnheader', { name: 'Open assignments' }).isVisible(),
      vera: (await vera.getByRole('cell').last().innerText()).trim(),
      oleg: (
        await page
          .getByRole('row', { name: /Oleg Student/ })
          .getByRole('cell')
          .last()
          .innerText()
      ).trim(),
    };
    await page.setViewportSize({ width: 360, height: 780 });

    expect(wide).toEqual({ header: true, vera: '1', oleg: '0' });
    await expect(vera.getByText('Open assignments: 1')).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Open assignments' })).toBeHidden();
    await expect(page.getByRole('columnheader', { name: 'Groups' })).toBeHidden();
    await expect(vera.getByRole('rowheader').getByText('Evening group')).toBeVisible();
    expect((await vera.getByRole('link', { name: 'Vera Student' }).boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(
      44,
    );
    await expect(page.getByRole('table', { name: 'Active', exact: true }).getByRole('columnheader')).toHaveText([
      'Name',
      'State',
    ]);
    const search = await layoutBox(page.getByRole('searchbox', { name: 'Name or email' }));
    const group = await layoutBox(page.getByRole('combobox', { name: 'Group' }));
    const submit = await layoutBox(page.getByRole('search').getByRole('button', { name: 'Search' }));
    expect(group.y).toBeGreaterThanOrEqual(search.y + search.height);
    expect(submit.y).toBeGreaterThanOrEqual(group.y + group.height);
    expect(Math.abs(submit.width - search.width)).toBeLessThanOrEqual(1);
  });
});
