import type { Browser, BrowserContext, Locator, Page, Request } from '@playwright/test';

import { newTestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { channelOf, channelUnread } from './messages.ts';
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

async function studentContext(browser: Browser, deployment: Deployment): Promise<BrowserContext> {
  return browser.newContext({
    baseURL: deployment.origin,
    ignoreHTTPSErrors: true,
    locale: 'en-GB',
    timezoneId: 'Europe/Moscow',
    viewport: { width: 375, height: 812 },
  });
}

function messages(page: Page): Locator {
  return page.getByRole('list', { name: 'Messages', exact: true }).getByRole('listitem');
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

  test('messages › a tutor and a student exchange messages live, with unread state, header links and focus, also at 375 px', async ({
    page,
    browser,
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
    await studentPage.goto('/messages');

    await studentPage.getByRole('region', { name: 'Chats', exact: true }).getByRole('link').click();
    await studentPage.getByRole('heading', { level: 2, name: 'Pavel', exact: true }).waitFor();
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

    expect(listHidden).toBe(true);
    expect(studentFocusAfterSend).toBe(true);
    expect(unreadName).toBe('Messages, 1 unread chats');
    expect(openingFocus).toBe(true);
    await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toBeFocused();
    await expect(page.getByRole('textbox', { name: 'Message', exact: true })).toHaveValue('');
    await expect(studentPage.getByRole('link', { name: 'Back to the list', exact: true })).toBeVisible();
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
    browser,
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
    await page.goto(`/messages/${channel}`);
    await studentPage.goto(`/messages/${channel}`);
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

    expect(editFocus).toBe(true);
    expect(savedFocus).toBe(true);
    expect(deletedFocus).toBe(true);
    await expect(page.locator(`[data-row="${memberId ?? ''}"]`)).toBeFocused();
    await expect(messages(page).filter({ hasText: 'Message deleted' })).toHaveCount(2);
    await expect(messages(studentPage).filter({ hasText: 'Message deleted' })).toHaveCount(2);
    await diagnostics.closeContext(context);
  });

  test('messages › a channel outside the caller’s chats shows Not found for another tutor and a removed member', async ({
    page,
    browser,
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
    browser,
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
    await studentPage.goto('/messages');

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
