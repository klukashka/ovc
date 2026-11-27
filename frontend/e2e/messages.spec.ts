import type { BrowserContext, Locator, Page, Request } from '@playwright/test';

import type { ObservedBrowser } from './browser-diagnostics.ts';

import { type TestAccount, newTestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { type Diagnostics, diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { channelOf, channelUnread, postMessage } from './messages.ts';
import { backgroundOf, transparent } from './dangerActions.ts';
import { expectRefetchedAfterTheSocketIsReady, trafficAfterReady } from './realtime.ts';
import { accessToken, callApi, createCard, createGroup, seedActiveStudent, signUpOnInvitation } from './tutoring.ts';

async function holdListUntilChannelDenial(page: Page, channel: string): Promise<() => Promise<boolean>> {
  let reached = (): void => {
    throw new Error('list barrier is not initialized');
  };
  let release = (): void => {
    throw new Error('list release is not initialized');
  };
  let complete = (): void => {
    throw new Error('list completion is not initialized');
  };
  let fail: (error: unknown) => void = (): void => {
    throw new Error('list failure is not initialized');
  };
  const started = new Promise<void>((resolve) => {
    reached = resolve;
  });
  const resumed = new Promise<void>((resolve) => {
    release = resolve;
  });
  const completed = new Promise<void>((resolve, reject) => {
    complete = resolve;
    fail = reject;
  });
  let held: Request | null = null;
  let aborted = false;
  const finished = (request: Request): void => {
    if (request === held) complete();
  };
  const failed = (request: Request): void => {
    if (request === held) {
      aborted = true;
      complete();
    }
  };
  page.on('requestfinished', finished);
  page.on('requestfailed', failed);
  await page.route(/\/api\/v1\/channels\?/, async (route) => {
    held ??= route.request();
    reached();
    await resumed;
    try {
      await route.continue();
    } catch (error) {
      fail(error);
      throw error;
    }
  });
  await page.route(`**/api/v1/channels/${channel}`, async (route) => {
    await started;
    await route.continue();
  });
  return async (): Promise<boolean> => {
    release();
    await completed;
    page.off('requestfinished', finished);
    page.off('requestfailed', failed);
    await page.unroute(/\/api\/v1\/channels\?/);
    await page.unroute(`**/api/v1/channels/${channel}`);
    return aborted;
  };
}

async function studentContext(browser: ObservedBrowser, deployment: Deployment): Promise<BrowserContext> {
  return browser.newContext({
    baseURL: deployment.origin,
    ignoreHTTPSErrors: true,
    locale: 'en-GB',
    timezoneId: 'Europe/Moscow',
    viewport: { width: 360, height: 780 },
  });
}

function messages(page: Page): Locator {
  return page.getByRole('list', { name: 'Messages', exact: true }).getByRole('listitem');
}

function conversationQueryPaths(channel: string): readonly string[] {
  return [`/api/v1/channels/${channel}`, `/api/v1/channels/${channel}/messages`];
}

async function openSettledConversation(
  page: Page,
  channel: string,
  layout: 'with the chat list' | 'conversation only',
): Promise<void> {
  const paths = conversationQueryPaths(channel);
  const traffic = trafficAfterReady(page, layout === 'with the chat list' ? [...paths, '/api/v1/channels'] : paths);
  await traffic.initialize();
  await page.goto(`/messages/${channel}`);
  await expectRefetchedAfterTheSocketIsReady(traffic);
}

async function settledComposer(page: Page): Promise<void> {
  await page.waitForFunction(() => document.querySelector('textarea')?.readOnly === false);
}

async function send(page: Page, body: string): Promise<void> {
  await page.getByRole('textbox', { name: 'Message', exact: true }).fill(body);
  const saved = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' && /\/channels\/[^/]+\/messages$/.test(new URL(response.url()).pathname),
  );
  await page.getByRole('button', { name: 'Send', exact: true }).click();
  const response = await saved;
  if (response.status() !== 201) {
    throw new Error(`sending a message answered ${String(response.status())}`);
  }
  await settledComposer(page);
}

test.describe('text messages', () => {
  test.use({ locale: 'en-GB', timezoneId: 'Europe/Moscow', viewport: { width: 1280, height: 800 } });

  test('messages › a tutor and a student exchange messages live, with unread state, header links and focus, also at 360 px with Back to the list', async ({
    page,
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
    const context = await studentContext(browser, deployment);
    const studentPage = await context.newPage();
    await diagnostics.watch(studentPage);
    await signInThroughTheApi(page, tutor);
    await signInThroughTheApi(studentPage, student);
    await page.goto('/today');
    const studentTraffic = trafficAfterReady(studentPage, ['/api/v1/channels']);
    await studentTraffic.initialize();
    await studentPage.goto('/messages');

    await expectRefetchedAfterTheSocketIsReady(studentTraffic);
    await studentPage.getByRole('region', { name: 'Chats', exact: true }).getByRole('link').click();
    const studentHeading = studentPage.getByRole('heading', { level: 2, name: 'Pavel', exact: true });
    await studentHeading.waitFor();
    const studentOpeningFocus = await studentHeading.evaluate((element) => element === document.activeElement);
    const listHidden = await studentPage.getByRole('searchbox', { name: 'Search chats' }).isHidden();
    await send(studentPage, 'A question before our lesson');
    const studentFocusAfterSend = await studentPage
      .getByRole('textbox', { name: 'Message', exact: true })
      .evaluate((element) => element === document.activeElement);
    const unreadLink = page.getByRole('link', { name: 'Messages, 1 unread chats', exact: true });
    await unreadLink.waitFor();
    const unreadName = await unreadLink.getAttribute('aria-label');
    await unreadLink.click();
    await page.getByRole('region', { name: 'Chats', exact: true }).getByRole('link').click();
    await messages(page).filter({ hasText: 'A question before our lesson' }).waitFor();
    const openingFocus = await page
      .getByRole('heading', { level: 2, name: 'Anna', exact: true })
      .evaluate((element) => element === document.activeElement);
    await send(page, 'Let us discuss it together');
    await messages(studentPage).filter({ hasText: 'Let us discuss it together' }).waitFor();
    const frameInConversation = await studentPage.locator('[data-frame-only]').count();
    const back = studentPage.getByRole('link', { name: 'Back to the list', exact: true });
    const backShown = await back.isVisible();
    await back.focus();
    await studentPage.setViewportSize({ width: 1024, height: 780 });
    await studentPage.getByRole('region', { name: 'Chats', exact: true }).waitFor();
    const focusAfterWidening = await studentHeading.evaluate((element) => element === document.activeElement);
    await studentPage.setViewportSize({ width: 360, height: 780 });
    await back.click();
    await studentPage.waitForURL((url) => url.pathname === '/messages');

    expect(listHidden).toBe(true);
    expect(studentFocusAfterSend).toBe(true);
    expect(unreadName).toBe('Messages, 1 unread chats');
    expect(openingFocus).toBe(true);
    await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toBeFocused();
    await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toHaveValue('');
    expect(frameInConversation).toBe(0);
    expect(backShown).toBe(true);
    expect(studentOpeningFocus).toBe(true);
    expect(focusAfterWidening).toBe(true);
    await expect(studentPage.getByRole('heading', { level: 1, name: 'Messages', exact: true })).toBeFocused();
    await expect(studentPage.getByRole('navigation', { name: 'Main navigation', exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Student card', exact: true })).toHaveAttribute(
      'href',
      `/students/${cardId}`,
    );
    await expect.poll(() => channelUnread(deployment, student, channel)).toBe(0);
    await expect(page.getByRole('link', { name: 'Messages', exact: true })).toBeVisible();
    await diagnostics.closeContext(context);
  });

  test('messages › editing and deleting own messages, and the tutor deleting a member’s message', async ({
    page,
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
    const context = await studentContext(browser, deployment);
    const studentPage = await context.newPage();
    await diagnostics.watch(studentPage);
    await signInThroughTheApi(page, tutor);
    await signInThroughTheApi(studentPage, student);
    await openSettledConversation(page, channel, 'with the chat list');
    await openSettledConversation(studentPage, channel, 'conversation only');
    await send(studentPage, 'Please correct this');
    await send(page, 'First version');
    const own = messages(page).filter({ hasText: 'First version' });
    const member = messages(page).filter({ hasText: 'Please correct this' });

    await own.getByRole('button', { name: 'Message actions', exact: true }).click();
    await page.getByRole('button', { name: 'Edit', exact: true }).click();
    const editFocus = await page
      .getByRole('textbox', { name: 'Message', exact: true })
      .evaluate((element) => element === document.activeElement);
    await page.getByRole('textbox', { name: 'Message', exact: true }).fill('Corrected version');
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    const corrected = messages(page).filter({ hasText: 'Corrected version' });
    await corrected.waitFor();
    await settledComposer(page);
    const savedFocus = await corrected.evaluate((element) => element === document.activeElement);
    const ownId = await corrected.getAttribute('data-row');
    await corrected.getByRole('button', { name: 'Message actions', exact: true }).click();
    await page.getByRole('button', { name: 'Delete', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Delete this message?' })
      .getByRole('button', { name: 'Delete message', exact: true })
      .click();
    await page
      .locator(`[data-row="${ownId ?? ''}"]`)
      .getByText('Message deleted', { exact: true })
      .waitFor();
    await settledComposer(page);
    const deletedFocus = await page
      .locator(`[data-row="${ownId ?? ''}"]`)
      .evaluate((element) => element === document.activeElement);
    const memberId = await member.getAttribute('data-row');
    await member.getByRole('button', { name: 'Message actions', exact: true }).click();
    await page.getByRole('button', { name: 'Delete', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Delete this message?' })
      .getByRole('button', { name: 'Delete message', exact: true })
      .click();

    expect(editFocus).toBe(true);
    expect(savedFocus).toBe(true);
    expect(deletedFocus).toBe(true);
    await expect(page.locator(`[data-row="${memberId ?? ''}"]`)).toBeFocused();
    await expect(messages(page).filter({ hasText: 'Message deleted' })).toHaveCount(2);
    await expect(messages(studentPage).filter({ hasText: 'Message deleted' })).toHaveCount(2);
    await diagnostics.closeContext(context);
  });

  test('messages › Delete is the last item of the message menu after a separator and asks first, opening on Cancel', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
    await signInThroughTheApi(page, tutor);
    await openSettledConversation(page, channel, 'with the chat list');
    await send(page, 'Something to delete');
    const own = messages(page).filter({ hasText: 'Something to delete' });
    await own.getByRole('button', { name: 'Message actions', exact: true }).click();
    const menu = own.locator('[aria-expanded="true"] + div');
    const lastItem = menu.locator(':scope > button:last-child');
    await expect(lastItem).toHaveText('Delete');
    await expect(lastItem).toHaveClass(/text-danger/);
    await expect(lastItem.locator('svg')).toHaveCount(1);
    await expect(menu.locator(':scope > hr')).toHaveCount(1);

    await lastItem.click();

    const dialog = page.getByRole('dialog', { name: 'Delete this message?' });
    await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeFocused();
    await expect(dialog.getByRole('button')).toHaveText(['Delete message', 'Cancel']);
    expect(await backgroundOf(dialog.getByRole('button', { name: 'Delete message', exact: true }))).not.toBe(
      transparent,
    );
    await expect(dialog).toContainText('its attachments are removed. It cannot be undone.');
  });

  test('messages › Cancel and Escape close the deletion dialog onto the message menu and keep the message', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
    await signInThroughTheApi(page, tutor);
    await openSettledConversation(page, channel, 'with the chat list');
    await send(page, 'Something to keep');
    const own = messages(page).filter({ hasText: 'Something to keep' });
    const opener = own.getByRole('button', { name: 'Message actions', exact: true });
    const dialog = page.getByRole('dialog', { name: 'Delete this message?' });

    await opener.click();
    await page.getByRole('button', { name: 'Delete', exact: true }).click();
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(opener).toBeFocused();
    await opener.click();
    await page.getByRole('button', { name: 'Delete', exact: true }).click();
    await page.keyboard.press('Escape');

    await expect(dialog).toBeHidden();
    await expect(opener).toBeFocused();
    await expect(own).not.toContainText('Message deleted');
  });

  test('messages › a message deleted elsewhere while its deletion dialog is open closes it onto the placeholder without a request', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
    await signInThroughTheApi(page, tutor);
    await openSettledConversation(page, channel, 'with the chat list');
    await send(page, 'Deleted in another tab');
    const own = messages(page).filter({ hasText: 'Deleted in another tab' });
    const ownId = (await own.getAttribute('data-row')) ?? '';
    let deletions = 0;
    page.on('request', (sent) => {
      if (sent.method() === 'DELETE' && new URL(sent.url()).pathname === `/api/v1/messages/${ownId}`) {
        deletions += 1;
      }
    });
    await own.getByRole('button', { name: 'Message actions', exact: true }).click();
    await page.getByRole('button', { name: 'Delete', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Delete this message?' });
    await expect(dialog).toBeVisible();

    await callApi(deployment, await accessToken(deployment, tutor), 'DELETE', `/messages/${ownId}`);

    await expect(dialog).toBeHidden();
    await expect(page.locator(`[data-row="${ownId}"]`)).toContainText('Message deleted');
    await expect(page.locator(`[data-row="${ownId}"]`)).toBeFocused();
    expect(deletions).toBe(0);
  });

  test('messages › a stale edit explicitly keeps local text before a separate save', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
    const otherClient = await accessToken(deployment, tutor);
    await signInThroughTheApi(page, tutor);
    await page.goto(`/messages/${channel}`);
    await send(page, 'Original version');
    const row = messages(page).filter({ hasText: 'Original version' });
    const id = await row.getAttribute('data-row');
    if (id === null) throw new Error('The message identifier is missing');
    await row.getByRole('button', { name: 'Message actions', exact: true }).click();
    await page.getByRole('button', { name: 'Edit', exact: true }).click();
    const composer = page.getByRole('textbox', { name: 'Message', exact: true });
    await composer.fill('Retained local text');
    const concurrent = await callApi(deployment, otherClient, 'PATCH', `/messages/${id}`, {
      body: 'Text saved by another client',
      version: 1,
    });
    if (concurrent.status !== 200) throw new Error('The concurrent edit failed');
    diagnostics.expectFailure({ method: 'PATCH', path: `/api/v1/messages/${id}`, status: 409 });
    let patches = 0;
    const countPatch = (request: Request): void => {
      if (request.method() === 'PATCH' && new URL(request.url()).pathname === `/api/v1/messages/${id}`) patches += 1;
    };
    page.on('request', countPatch);

    await page.getByRole('button', { name: 'Save', exact: true }).click();
    const keep = page.getByRole('button', { name: 'Keep my text', exact: true });
    await keep.waitFor();
    const retained = await composer.inputValue();
    const currentText = await page.getByRole('region', { name: 'Current saved text' }).textContent();
    await keep.click();
    const choiceFocus = await composer.evaluate((element) => element === document.activeElement);
    const patchesBeforeSave = patches;
    const beforeSave = await callApi(deployment, otherClient, 'GET', `/channels/${channel}/messages`);
    await page.getByRole('button', { name: 'Save', exact: true }).click();
    await messages(page).filter({ hasText: 'Retained local text' }).waitFor();
    await settledComposer(page);
    const afterSave = await callApi(deployment, otherClient, 'GET', `/channels/${channel}/messages`);
    page.off('request', countPatch);

    expect(retained).toBe('Retained local text');
    expect(currentText).toContain('Text saved by another client');
    expect(choiceFocus).toBe(true);
    expect(patchesBeforeSave).toBe(1);
    expect(patches).toBe(2);
    expect(beforeSave.status).toBe(200);
    expect(beforeSave.body).toMatchObject({ items: [{ id, version: 2, body: 'Text saved by another client' }] });
    expect(afterSave.status).toBe(200);
    expect(afterSave.body).toMatchObject({ items: [{ id, version: 3, body: 'Retained local text' }] });
    await expect(page.locator(`[data-row="${id}"]`)).toBeFocused();
    await expect(composer).toHaveValue('');
  });

  test('messages › a channel outside the caller’s chats shows Not found for another tutor and a removed member', async ({
    page,
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const other = await seedVerifiedUser(deployment, 'Olga', 'en');
    const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    const group = await createGroup(deployment, tutor, 'Private group', [cardId]);
    const channel = await channelOf(deployment, tutor, `/groups/${group}`);
    const removed = await callApi(deployment, await accessToken(deployment, tutor), 'PATCH', `/groups/${group}`, {
      remove: [cardId],
    });
    if (removed.status !== 200) {
      throw new Error(`removing the group member answered ${String(removed.status)}`);
    }
    const context = await studentContext(browser, deployment);
    const studentPage = await context.newPage();
    const watched = await diagnostics.watch(studentPage);
    watched.expectFailure({ method: 'GET', path: `/api/v1/channels/${channel}`, status: 404 });
    diagnostics.expectFailure({ method: 'GET', path: `/api/v1/channels/${channel}`, status: 404 });
    await signInThroughTheApi(page, other);
    await signInThroughTheApi(studentPage, student);
    const releaseList = await holdListUntilChannelDenial(page, channel);

    await page.goto(`/messages/${channel}`);
    await studentPage.goto(`/messages/${channel}`);

    await expect(page.getByRole('main').getByText('Not found', { exact: true })).toBeVisible();
    await expect(studentPage.getByRole('main').getByText('Not found', { exact: true })).toBeVisible();
    await expect(page.getByRole('main').getByText('Not found', { exact: true })).toBeFocused();
    await expect(studentPage.getByRole('main').getByText('Not found', { exact: true })).toBeFocused();
    await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toHaveCount(0);
    await expect(studentPage.getByRole('textbox', { name: 'Message', exact: true })).toHaveCount(0);
    await expect(page.getByRole('main')).not.toContainText('Private group');
    await expect(studentPage.getByRole('main')).not.toContainText('Private group');
    if (await releaseList()) diagnostics.expectAbortedRequest(/^\/api\/v1\/channels$/);
    await expect(page.getByRole('main').getByText('Not found', { exact: true })).toBeFocused();
    await expect(studentPage.getByRole('main').getByText('Not found', { exact: true })).toBeFocused();
    await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toHaveCount(0);
    await expect(studentPage.getByRole('textbox', { name: 'Message', exact: true })).toHaveCount(0);
    await expect(page.getByRole('main')).not.toContainText('Private group');
    await expect(studentPage.getByRole('main')).not.toContainText('Private group');
    await diagnostics.closeContext(context);
  });

  test('messages › an unconfirmed provisional student reaches only the direct chat, which the tutor sees as unconfirmed', async ({
    page,
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const student = newTestAccount('Unconfirmed learner', 'student');
    const card = await createCard(deployment, tutor, 'Anna', student.email);
    await signUpOnInvitation(deployment, card.link, student, 'en');
    const channel = await channelOf(deployment, tutor, `/students/${card.id}`);
    await createGroup(deployment, tutor, 'Other group', []);
    const context = await studentContext(browser, deployment);
    const studentPage = await context.newPage();
    await diagnostics.watch(studentPage);
    await signInThroughTheApi(page, tutor);
    await signInThroughTheApi(studentPage, student);
    await page.goto(`/messages/${channel}`);
    const studentTraffic = trafficAfterReady(studentPage, ['/api/v1/channels']);
    await studentTraffic.initialize();
    await studentPage.goto('/messages');
    await expectRefetchedAfterTheSocketIsReady(studentTraffic);

    const listed = studentPage.getByRole('region', { name: 'Chats', exact: true }).getByRole('link');
    await listed.first().waitFor();
    const listCount = await listed.count();
    await listed.first().click();
    await send(studentPage, 'I am waiting for confirmation');

    expect(listCount).toBe(1);
    await expect(studentPage.getByRole('heading', { level: 2, name: 'Pavel', exact: true })).toBeVisible();
    await expect(page.getByRole('main').getByText('Email not confirmed', { exact: true })).toHaveCount(2);
    await expect(messages(page).filter({ hasText: 'I am waiting for confirmation' })).toContainText('Anna');
    await expect(page.getByRole('main')).not.toContainText('Unconfirmed learner');
    await expect(studentPage.getByRole('textbox', { name: 'Message', exact: true })).toBeFocused();
    await diagnostics.closeContext(context);
  });
});

const paginationTest = test.extend<
  { diagnostics: Diagnostics },
  { paginationActors: { readonly tutor: TestAccount; readonly student: TestAccount } }
>({
  paginationActors: [
    async ({ deployment }, use): Promise<void> => {
      const tutor = await seedVerifiedUser(deployment, 'Pagination tutor', 'en');
      const member = await seedActiveStudent(deployment, tutor, 'Pagination student', 'en');
      for (let index = 0; index < 60; index += 1) {
        await createGroup(deployment, tutor, `Pagination group ${String(index)}`, [member.cardId]);
      }
      await use({ tutor, student: member.student });
    },
    { scope: 'worker' },
  ],
});

paginationTest.describe('channel pagination', () => {
  paginationTest.describe.configure({ mode: 'default' });
  paginationTest.use({ locale: 'en-GB', timezoneId: 'Europe/Moscow' });
  for (const role of ['tutor', 'student'] as const) {
    for (const profile of [
      { language: 'en', theme: 'light', width: 1280, list: 'Chats', more: 'Show more' },
      { language: 'ru', theme: 'dark', width: 360, list: 'Чаты', more: 'Показать ещё' },
    ] as const) {
      for (const final of [false, true]) {
        paginationTest(
          `${role} ${profile.language} ${profile.theme} ${String(profile.width)}px focuses its first new channel on the ${final ? 'final' : 'intermediate'} page`,
          async ({ page, deployment, paginationActors }) => {
            const account = paginationActors[role];
            const saved = await callApi(deployment, await accessToken(deployment, account), 'PATCH', '/users/me', {
              language: profile.language,
              theme: profile.theme,
            });
            if (saved.status !== 200) throw new Error('pagination preferences were not saved');
            await page.setViewportSize({ width: profile.width, height: 800 });
            await signInThroughTheApi(page, account);
            const traffic = trafficAfterReady(page, ['/api/v1/channels']);
            await traffic.initialize();
            await page.goto('/messages');
            await expectRefetchedAfterTheSocketIsReady(traffic);
            const links = page.getByRole('region', { name: profile.list, exact: true }).getByRole('link');
            const more = page.getByRole('button', { name: profile.more, exact: true });
            if (final) {
              await more.click();
              await links.nth(30).waitFor();
            }

            await more.focus();
            await more.press('Enter');

            await expect(links.nth(final ? 60 : 30)).toBeFocused();
            await expect(more).toHaveCount(final ? 0 : 1);
          },
        );
      }
    }
  }
});

async function longHistory(
  deployment: Deployment,
): Promise<{ readonly tutor: TestAccount; readonly studentToken: string; readonly channel: string }> {
  const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
  const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
  const senders = [await accessToken(deployment, student), await accessToken(deployment, tutor)];
  for (let index = 1; index <= 12; index += 1) {
    const lines = Array.from({ length: 5 }, (_, line) => `Line ${String(line + 1)} of a long message`).join('\n');
    const sent = await callApi(deployment, senders[index % 2] ?? '', 'POST', `/channels/${channel}/messages`, {
      body: `${lines}\nHistory message ${String(index)}`,
      attachment_ids: [],
    });
    expect(sent.status).toBe(201);
  }
  return { tutor, studentToken: senders[0] ?? '', channel };
}

const historyTest = test.extend<
  { diagnostics: Diagnostics },
  { sharedHistory: { readonly tutor: TestAccount; readonly channel: string } }
>({
  sharedHistory: [
    async ({ deployment }, use): Promise<void> => {
      const { tutor, channel } = await longHistory(deployment);
      await use({ tutor, channel });
    },
    { scope: 'worker' },
  ],
});

historyTest.describe('Messages on a computer, with one long history shared by the tests below', () => {
  historyTest.describe.configure({ mode: 'default' });
  historyTest.use({ locale: 'en-GB', timezoneId: 'Europe/Moscow', viewport: { width: 1440, height: 900 } });

  historyTest(
    "at 1440 × 900 the composer's bottom edge stays inside the window with a long history, which shows its latest message with the heading focused",
    async ({ page, sharedHistory }) => {
      const { tutor, channel } = sharedHistory;
      await signInThroughTheApi(page, tutor);

      await page.goto(`/messages/${channel}`);
      await messages(page)
        .getByText(/History message 12$/)
        .waitFor();

      const composerBox = await page.getByRole('textbox', { name: 'Message', exact: true }).boundingBox();
      const pageScrolls = await page.evaluate(
        () => document.documentElement.scrollHeight > document.documentElement.clientHeight,
      );
      expect(composerBox).not.toBeNull();
      expect((composerBox?.y ?? 0) + (composerBox?.height ?? 0)).toBeLessThanOrEqual(900);
      expect(pageScrolls).toBe(false);
      await expect(messages(page).getByText(/History message 12$/)).toBeInViewport();
      await expect(page.getByRole('heading', { level: 2, name: 'Anna', exact: true })).toBeFocused();
    },
  );

  historyTest(
    'a history shown at its bottom stays at its bottom when the window gets shorter',
    async ({ page, sharedHistory }) => {
      const { tutor, channel } = sharedHistory;
      await signInThroughTheApi(page, tutor);
      await page.goto(`/messages/${channel}`);
      await expect(messages(page).getByText(/History message 12$/)).toBeInViewport();

      await page.setViewportSize({ width: 1440, height: 600 });

      await expect(messages(page).getByText(/History message 12$/)).toBeInViewport();
    },
  );
});

test.describe('Messages on a computer', () => {
  test.use({ locale: 'en-GB', timezoneId: 'Europe/Moscow', viewport: { width: 1440, height: 900 } });

  function isChannelRead(request: Request): boolean {
    return request.method() === 'POST' && /\/channels\/[^/]+\/read$/.test(new URL(request.url()).pathname);
  }

  function channelReads(page: Page): { readonly count: number } {
    const reads = { count: 0 };
    page.on('request', (request) => {
      if (isChannelRead(request)) reads.count += 1;
    });
    return reads;
  }

  async function scrollHistory(page: Page, lastRowBelowPaneEdge: number): Promise<void> {
    await page.getByRole('list', { name: 'Messages', exact: true }).evaluate((list, below) => {
      let pane = list.parentElement;
      while (pane !== null && getComputedStyle(pane).overflowY !== 'auto') pane = pane.parentElement;
      if (pane === null) throw new Error('the history has no scroll pane');
      pane.scrollTop = pane.scrollHeight;
      if (below > 0) {
        pane.scrollTop -= pane.getBoundingClientRect().bottom + below - list.getBoundingClientRect().bottom;
      }
      pane.dispatchEvent(new Event('scroll'));
    }, lastRowBelowPaneEdge);
  }

  test('a message that arrives below the history pane edge stays unread until it is scrolled into view', async ({
    page,
    deployment,
    control,
  }) => {
    const { tutor, studentToken, channel } = await longHistory(deployment);
    await page.clock.install({ time: await control.now() });
    const reads = channelReads(page);
    await signInThroughTheApi(page, tutor);
    await page.goto(`/messages/${channel}`);
    await expect(messages(page).getByText(/History message 12$/)).toBeInViewport();
    await page.clock.runFor(2000);
    await expect.poll(() => reads.count).toBeGreaterThan(0);
    await scrollHistory(page, 40);
    const readsBeforeArrival = reads.count;

    const sent = await callApi(deployment, studentToken, 'POST', `/channels/${channel}/messages`, {
      body: 'Arrived out of view',
      attachment_ids: [],
    });
    await messages(page).getByText('Arrived out of view').waitFor({ state: 'attached' });
    await page.clock.runFor(2000);
    const readsBeforeScrolling = reads.count;
    const unreadBeforeScrolling = await channelUnread(deployment, tutor, channel);
    await scrollHistory(page, 0);
    await page.clock.runFor(2000);

    expect(sent.status).toBe(201);
    expect(readsBeforeScrolling).toBe(readsBeforeArrival);
    expect(unreadBeforeScrolling).toBe(1);
    await expect.poll(() => channelUnread(deployment, tutor, channel)).toBe(0);
  });

  test('the composer grows with its text up to eight lines', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
    await signInThroughTheApi(page, tutor);
    await page.goto(`/messages/${channel}`);
    const field = page.getByRole('textbox', { name: 'Message', exact: true });
    await settledComposer(page);
    const oneLine = await field.evaluate((element) => (element instanceof HTMLTextAreaElement ? element.rows : -1));

    await field.fill(['one', 'two', 'three', 'four'].join('\n'));
    const fourLines = await field.evaluate((element) => (element instanceof HTMLTextAreaElement ? element.rows : -1));
    await field.fill(Array.from({ length: 12 }, (_, index) => `line ${String(index)}`).join('\n'));
    const manyLines = await field.evaluate((element) => (element instanceof HTMLTextAreaElement ? element.rows : -1));

    expect(oneLine).toBe(1);
    expect(fourLines).toBe(4);
    expect(manyLines).toBe(8);
  });

  test('Send is the primary action and attach is a named icon button', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
    await signInThroughTheApi(page, tutor);

    await page.goto(`/messages/${channel}`);
    await settledComposer(page);

    const attach = page.getByRole('button', { name: 'Attach files', exact: true });
    await expect(attach).toBeVisible();
    await expect(attach).toHaveText('');
    await expect(page.getByRole('button', { name: 'Send', exact: true })).toHaveClass(/bg-accent/);
  });
});

test.describe('the messenger layout on a computer', () => {
  test.use({ locale: 'en-GB', timezoneId: 'Europe/Moscow', viewport: { width: 1280, height: 800 } });

  async function bubbleColours(page: Page): Promise<{ readonly own: string[]; readonly others: string[] }> {
    return page.getByRole('list', { name: 'Messages', exact: true }).evaluate((list) => {
      const probe = document.createElement('span');
      probe.style.background = 'var(--color-mine)';
      document.body.append(probe);
      const mine = getComputedStyle(probe).backgroundColor;
      probe.remove();
      const colours = (own: boolean): string[] =>
        Array.from(list.querySelectorAll<HTMLElement>(own ? 'li[data-own]' : 'li:not([data-own])')).map((row) => {
          const bubble = row.querySelector('[data-bubble]');
          return bubble === null ? 'none' : getComputedStyle(bubble).backgroundColor === mine ? 'mine' : 'other';
        });
      return { own: colours(true), others: colours(false) };
    });
  }

  async function scrollHistoryTo(page: Page, end: 'top' | 'bottom'): Promise<void> {
    await page.getByRole('list', { name: 'Messages', exact: true }).evaluate((list, target) => {
      let pane = list.parentElement;
      while (pane !== null && getComputedStyle(pane).overflowY !== 'auto') pane = pane.parentElement;
      if (pane === null) throw new Error('the history has no scroll pane');
      pane.scrollTop = target === 'top' ? 0 : pane.scrollHeight;
      pane.dispatchEvent(new Event('scroll'));
    }, end);
  }

  test('bubbles mark own messages, every message offers its actions by Tab, Enter sends, and Jump to latest counts arrivals', async ({
    page,
    deployment,
    browserName,
  }) => {
    const { tutor, studentToken, channel } = await longHistory(deployment);
    await signInThroughTheApi(page, tutor);
    await openSettledConversation(page, channel, 'with the chat list');
    await expect(messages(page).getByText(/History message 12$/)).toBeInViewport();
    const composer = page.getByRole('textbox', { name: 'Message', exact: true });
    await settledComposer(page);

    const colours = await bubbleColours(page);
    const rows = await messages(page).count();
    const reachedByTab: boolean[] = [];
    for (let index = 0; index < rows; index += 1) {
      const row = messages(page).nth(index);
      await row.focus();
      await page.keyboard.press(browserName === 'webkit' ? 'Alt+Tab' : 'Tab');
      reachedByTab.push(
        await row.getByRole('button', { name: 'Message actions', exact: true }).evaluate((button) => {
          return button === document.activeElement;
        }),
      );
    }
    await composer.fill('Sent with Enter');
    const sent = page.waitForResponse(
      (response) =>
        response.request().method() === 'POST' && /\/channels\/[^/]+\/messages$/.test(new URL(response.url()).pathname),
    );
    await composer.press('Enter');
    const sentStatus = (await sent).status();
    await messages(page).filter({ hasText: 'Sent with Enter' }).waitFor();
    await settledComposer(page);
    const sendsBeforeShiftEnter = await messages(page).count();
    await composer.press('a');
    await composer.press('Shift+Enter');
    await composer.press('b');
    const twoLines = await composer.inputValue();
    await composer.fill('');
    await scrollHistoryTo(page, 'top');
    const arrivals: number[] = [];
    for (const body of ['First while away', 'Second while away']) {
      const arrived = await callApi(deployment, studentToken, 'POST', `/channels/${channel}/messages`, {
        body,
        attachment_ids: [],
      });
      arrivals.push(arrived.status);
    }
    const jump = page.getByRole('button', { name: 'Jump to latest, 2 new', exact: true });
    await jump.waitFor();
    const badge = await jump.textContent();
    await jump.click();
    const focusAfterJump = await composer.evaluate((element) => element === document.activeElement);
    const latestAfterJump = await messages(page)
      .getByText('Second while away')
      .evaluate((element) => {
        const box = element.getBoundingClientRect();
        return box.top >= 0 && box.bottom <= window.innerHeight;
      });
    await scrollHistoryTo(page, 'top');
    const third = await callApi(deployment, studentToken, 'POST', `/channels/${channel}/messages`, {
      body: 'Third while away',
      attachment_ids: [],
    });
    arrivals.push(third.status);
    const oneNew = page.getByRole('button', { name: 'Jump to latest, 1 new', exact: true });
    await oneNew.focus();
    await scrollHistoryTo(page, 'bottom');

    expect(sentStatus).toBe(201);
    expect(arrivals).toEqual([201, 201, 201]);
    expect(latestAfterJump).toBe(true);
    expect(focusAfterJump).toBe(true);
    expect(badge).toBe('2');
    expect(colours.own).toEqual(Array.from({ length: 6 }, () => 'mine'));
    expect(colours.others).toEqual(Array.from({ length: 6 }, () => 'other'));
    expect(reachedByTab).toEqual(Array.from({ length: rows }, () => true));
    expect(twoLines).toBe('a\nb');
    expect(await messages(page).count()).toBe(sendsBeforeShiftEnter + 3);
    await expect(messages(page).getByText('Third while away')).toBeInViewport();
    await expect(composer).toBeFocused();
    await expect(page.getByRole('button', { name: /^Jump to latest/ })).toHaveCount(0);
  });
});

test.describe('replies and the pinned message', () => {
  test.use({ locale: 'en-GB', timezoneId: 'Europe/Moscow', viewport: { width: 1280, height: 800 } });

  test('a student replies to an earlier message, the tutor jumps to it from the quote, and a pin reaches the student live until its message is deleted', async ({
    page,
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
    const tutorToken = await accessToken(deployment, tutor);
    for (const body of ['The question to answer', 'Another note', 'Something to pin']) {
      const seeded = await callApi(deployment, tutorToken, 'POST', `/channels/${channel}/messages`, {
        body,
        attachment_ids: [],
      });
      expect(seeded.status).toBe(201);
    }
    const context = await studentContext(browser, deployment);
    const studentPage = await context.newPage();
    await diagnostics.watch(studentPage);
    await signInThroughTheApi(page, tutor);
    await signInThroughTheApi(studentPage, student);
    await openSettledConversation(studentPage, channel, 'conversation only');
    await openSettledConversation(page, channel, 'with the chat list');
    await settledComposer(studentPage);

    const pinned = messages(page).filter({ hasText: 'Something to pin' });
    await pinned.getByRole('button', { name: 'Message actions', exact: true }).click();
    await page.getByRole('button', { name: 'Pin', exact: true }).click();
    const studentBar = studentPage.getByRole('region', { name: 'Pinned message', exact: true });
    await studentBar.waitFor();
    const barText = await studentBar.textContent();
    const studentUnpins = await studentBar.getByRole('button', { name: 'Unpin', exact: true }).count();
    await pinned.getByRole('button', { name: 'Message actions', exact: true }).click();
    await page.getByRole('button', { name: 'Delete', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Delete this message?' })
      .getByRole('button', { name: 'Delete message', exact: true })
      .click();
    await studentBar.waitFor({ state: 'detached' });
    const question = messages(studentPage).filter({ hasText: 'The question to answer' });
    await question.focus();
    await question.getByRole('button', { name: 'Message actions', exact: true }).click();
    await studentPage.getByRole('button', { name: 'Reply', exact: true }).click();
    await send(studentPage, 'Here is my answer');
    const reply = messages(page).filter({ hasText: 'Here is my answer' });
    await reply.waitFor();
    await reply.getByRole('button', { name: /The question to answer/ }).click();

    expect(barText).toContain('Something to pin');
    expect(studentUnpins).toBe(0);
    await expect(
      messages(page).filter({ hasText: 'The question to answer' }).filter({ hasNotText: 'Here is my answer' }),
    ).toBeFocused();
    await diagnostics.closeContext(context);
  });
});

async function openQuotedConversationAsTutor(page: Page, deployment: Deployment): Promise<QuotedConversation> {
  const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
  const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
  const question = await postMessage(deployment, student, channel, 'A question from the student', null);
  const answer = await postMessage(deployment, tutor, channel, 'An answer from the tutor', question);
  await postMessage(deployment, student, channel, 'A thank-you from the student', answer);
  await signInThroughTheApi(page, tutor);
  return { channel };
}

interface QuotedConversation {
  readonly channel: string;
}

function ownAnswer(page: Page): Locator {
  return messages(page)
    .filter({ hasText: 'An answer from the tutor' })
    .filter({ hasNotText: 'A thank-you from the student' });
}

function othersThanks(page: Page): Locator {
  return messages(page).filter({ hasText: 'A thank-you from the student' });
}

test.describe('secondary text inside the bubbles', () => {
  test.use({
    locale: 'en-GB',
    timezoneId: 'Europe/Moscow',
    viewport: { width: 1280, height: 800 },
    colorScheme: 'light',
  });

  test("the time and quoted snippet of one's own message use the primary text colour, and another's keep the muted colour", async ({
    page,
    deployment,
  }) => {
    const { channel } = await openQuotedConversationAsTutor(page, deployment);

    await openSettledConversation(page, channel, 'with the chat list');

    await expect(ownAnswer(page).locator('time')).toHaveCSS('color', 'rgb(36, 34, 29)');
    await expect(ownAnswer(page).getByText('A question from the student')).toHaveCSS('color', 'rgb(36, 34, 29)');
    await expect(othersThanks(page).locator('time')).toHaveCSS('color', 'rgb(106, 100, 88)');
    await expect(othersThanks(page).getByText('An answer from the tutor')).toHaveCSS('color', 'rgb(106, 100, 88)');
  });
});

test.describe('secondary text inside the bubbles with the dark colour scheme', () => {
  test.use({
    locale: 'en-GB',
    timezoneId: 'Europe/Moscow',
    viewport: { width: 1280, height: 800 },
    colorScheme: 'dark',
  });

  test("the time and quoted snippet of one's own message keep the muted colour", async ({ page, deployment }) => {
    const { channel } = await openQuotedConversationAsTutor(page, deployment);

    await openSettledConversation(page, channel, 'with the chat list');

    await expect(ownAnswer(page).locator('time')).toHaveCSS('color', 'rgb(173, 165, 147)');
    await expect(ownAnswer(page).getByText('A question from the student')).toHaveCSS('color', 'rgb(173, 165, 147)');
  });
});

interface PinnedChatWorld {
  readonly tutorToken: string;
  readonly student: TestAccount;
  readonly channel: string;
  readonly message: string;
}

const chatsBeyondTheFirstPage = 31;

async function studentWithTwoListPages(deployment: Deployment): Promise<PinnedChatWorld> {
  const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
  const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
  const tutorToken = await accessToken(deployment, tutor);
  const seeded = await callApi(deployment, tutorToken, 'POST', `/channels/${channel}/messages`, {
    body: 'Something to pin',
    attachment_ids: [],
  });
  expect(seeded.status).toBe(201);
  const message = (seeded.body as { readonly id?: unknown }).id;
  if (typeof message !== 'string') throw new Error('the seeded message has no identifier');
  for (let index = 1; index <= chatsBeyondTheFirstPage; index += 1) {
    await createGroup(deployment, tutor, `Group ${String(index)}`, [cardId]);
  }
  return { tutorToken, student, channel, message };
}

async function openBothListPages(page: Page, world: PinnedChatWorld): Promise<Locator> {
  await signInThroughTheApi(page, world.student);
  await openSettledConversation(page, world.channel, 'with the chat list');
  const list = page.getByRole('region', { name: 'Chats', exact: true });
  const rows = list.getByRole('link');
  await expect(rows).toHaveCount(30);
  await list.getByRole('button', { name: 'Show more', exact: true }).click();
  await expect(rows).toHaveCount(chatsBeyondTheFirstPage + 1);
  return rows;
}

test.describe('pin changes and the chat list', () => {
  test.use({ locale: 'en-GB', timezoneId: 'Europe/Moscow', viewport: { width: 1280, height: 800 } });

  test('a pin by the tutor shows the student the pinned bar and keeps the student’s second list page', async ({
    page,
    deployment,
  }) => {
    const world = await studentWithTwoListPages(deployment);
    const rows = await openBothListPages(page, world);
    const bar = page.getByRole('region', { name: 'Pinned message', exact: true });

    const pinned = await callApi(deployment, world.tutorToken, 'PUT', `/channels/${world.channel}/pin`, {
      message_id: world.message,
    });

    expect(pinned.status).toBe(204);
    await expect(bar).toContainText('Something to pin');
    await expect(rows).toHaveCount(chatsBeyondTheFirstPage + 1);
  });

  test('an unpin by the tutor removes the student’s pinned bar and keeps the student’s second list page', async ({
    page,
    deployment,
  }) => {
    const world = await studentWithTwoListPages(deployment);
    const pinned = await callApi(deployment, world.tutorToken, 'PUT', `/channels/${world.channel}/pin`, {
      message_id: world.message,
    });
    expect(pinned.status).toBe(204);
    const rows = await openBothListPages(page, world);
    const bar = page.getByRole('region', { name: 'Pinned message', exact: true });
    await bar.waitFor();

    const unpinned = await callApi(deployment, world.tutorToken, 'DELETE', `/channels/${world.channel}/pin`);

    expect(unpinned.status).toBe(204);
    await expect(bar).toHaveCount(0);
    await expect(rows).toHaveCount(chatsBeyondTheFirstPage + 1);
  });
});

test.describe('read ticks in a direct chat', () => {
  test.use({ locale: 'en-GB', timezoneId: 'Europe/Moscow', viewport: { width: 1280, height: 800 } });

  test('a tutor sees one tick on a sent message and two once the student opens the chat, without a reload', async ({
    page,
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
    const context = await studentContext(browser, deployment);
    const studentPage = await context.newPage();
    await diagnostics.watch(studentPage);
    await signInThroughTheApi(page, tutor);
    await signInThroughTheApi(studentPage, student);
    await openSettledConversation(page, channel, 'with the chat list');
    await send(page, 'Did you read this?');
    const sent = messages(page).filter({ hasText: 'Did you read this?' });
    await expect(sent.getByText('Sent', { exact: true })).toBeAttached();

    await openSettledConversation(studentPage, channel, 'conversation only');

    await expect(sent.getByText('Read', { exact: true })).toBeAttached();
    await expect(sent.getByText('Sent', { exact: true })).toHaveCount(0);
    await diagnostics.closeContext(context);
  });
});

test.describe('search in a conversation', () => {
  test.use({ locale: 'en-GB', timezoneId: 'Europe/Moscow', viewport: { width: 1280, height: 800 } });

  test('a tutor searches a Russian chat on Enter, opens the match focused in the history, and the address holds no term', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
    const tutorToken = await accessToken(deployment, tutor);
    for (const body of ['Домашнее задание на среду', 'Обычное сообщение', 'Вопрос про тест']) {
      const seeded = await callApi(deployment, tutorToken, 'POST', `/channels/${channel}/messages`, {
        body,
        attachment_ids: [],
      });
      expect(seeded.status).toBe(201);
    }
    await signInThroughTheApi(page, tutor);
    await openSettledConversation(page, channel, 'with the chat list');
    await page.getByRole('button', { name: 'Search in this chat', exact: true }).click();
    const field = page.getByRole('searchbox', { name: 'Search messages', exact: true });
    await expect(field).toBeFocused();

    await field.fill('ДОМАШНЕЕ');
    await field.press('Enter');
    const results = page.getByRole('list', { name: 'Search results', exact: true });
    await expect(results.getByRole('button')).toHaveCount(1);
    await results.getByRole('button').click();

    await expect(messages(page).filter({ hasText: 'Домашнее задание на среду' })).toBeFocused();
    expect(page.url()).not.toContain(encodeURIComponent('ДОМАШНЕЕ'));
    expect(page.url().toLowerCase()).not.toContain('домашнее');
  });
});

test.describe('reactions in a group chat', () => {
  test.use({ locale: 'en-GB', timezoneId: 'Europe/Moscow', viewport: { width: 1280, height: 800 } });

  test('a student reacts from the menu, the tutor sees who reacted live, and the other student sees only the count', async ({
    page,
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const first = await seedActiveStudent(deployment, tutor, 'Anna');
    const second = await seedActiveStudent(deployment, tutor, 'Boris');
    const group = await createGroup(deployment, tutor, 'Reading group', [first.cardId, second.cardId]);
    const channel = await channelOf(deployment, tutor, `/groups/${group}`);
    const seeded = await callApi(
      deployment,
      await accessToken(deployment, tutor),
      'POST',
      `/channels/${channel}/messages`,
      {
        body: 'Who finished the chapter?',
        attachment_ids: [],
      },
    );
    expect(seeded.status).toBe(201);
    const firstContext = await studentContext(browser, deployment);
    const firstPage = await firstContext.newPage();
    await diagnostics.watch(firstPage);
    const secondContext = await studentContext(browser, deployment);
    const secondPage = await secondContext.newPage();
    await diagnostics.watch(secondPage);
    await signInThroughTheApi(page, tutor);
    await signInThroughTheApi(firstPage, first.student);
    await signInThroughTheApi(secondPage, second.student);
    await openSettledConversation(page, channel, 'with the chat list');
    await openSettledConversation(firstPage, channel, 'conversation only');
    await openSettledConversation(secondPage, channel, 'conversation only');

    const asked = messages(firstPage).filter({ hasText: 'Who finished the chapter?' });
    await asked.getByRole('button', { name: 'Message actions', exact: true }).click();
    await firstPage.getByRole('button', { name: 'React', exact: true }).click();
    await firstPage.getByRole('button', { name: 'Thumbs up', exact: true }).click();

    const tutorChip = messages(page)
      .filter({ hasText: 'Who finished the chapter?' })
      .getByRole('listitem')
      .filter({ hasText: '1' });
    await expect(tutorChip).toContainText('Anna');
    const secondChip = messages(secondPage)
      .filter({ hasText: 'Who finished the chapter?' })
      .getByRole('list', { name: 'Reactions' });
    await expect(secondChip).toContainText('1');
    await expect(secondChip).not.toContainText('Anna');
    await diagnostics.closeContext(firstContext);
    await diagnostics.closeContext(secondContext);
  });
});
