import type { Browser, BrowserContext, Page, Request, Response } from '@playwright/test';

import { type TestAccount, newTestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser, type SentMail, type TestControl } from './fixtures.ts';
import { channelOf, channelUnread } from './messages.ts';
import { accountId, expectRefetchedAfterTheSocketIsReady, trafficAfterReady, waitForSockets } from './realtime.ts';
import { accessToken, callApi, createCard, createGroup, seedActiveStudent, signUpOnInvitation } from './tutoring.ts';

const picture = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64',
);
const documentBytes = Buffer.from(
  '%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n',
);

interface GroupActors {
  readonly tutor: TestAccount;
  readonly anna: TestAccount;
  readonly boris: TestAccount;
  readonly channel: string;
}

async function groupActors(deployment: Deployment): Promise<GroupActors> {
  const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
  const anna = await seedActiveStudent(deployment, tutor, 'Anna');
  const boris = await seedActiveStudent(deployment, tutor, 'Boris');
  const group = await createGroup(deployment, tutor, 'B1 evening', [anna.cardId, boris.cardId]);
  return {
    tutor,
    anna: anna.student,
    boris: boris.student,
    channel: await channelOf(deployment, tutor, `/groups/${group}`),
  };
}

async function contextFor(browser: Browser, deployment: Deployment): Promise<BrowserContext> {
  return browser.newContext({
    baseURL: deployment.origin,
    ignoreHTTPSErrors: true,
    locale: 'en-GB',
    reducedMotion: 'reduce',
  });
}

async function openChat(page: Page, account: TestAccount, channel: string): Promise<void> {
  await signInThroughTheApi(page, account);
  const traffic = trafficAfterReady(page, [`/api/v1/channels/${channel}`]);
  await page.goto(`/messages/${channel}`);
  await page.getByRole('textbox', { name: 'Message', exact: true }).waitFor();
  await expectRefetchedAfterTheSocketIsReady(traffic);
}

async function send(page: Page, body: string): Promise<void> {
  await page.getByRole('textbox', { name: 'Message', exact: true }).fill(body);
  const saved = page.waitForResponse(async (response) => {
    if (
      response.request().method() !== 'POST' ||
      !/\/channels\/[^/]+\/messages$/.test(new URL(response.url()).pathname)
    )
      return false;
    if (response.status() !== 401) return true;
    const answer: unknown = await response.json();
    return !(
      typeof answer === 'object' &&
      answer !== null &&
      'error' in answer &&
      typeof answer.error === 'object' &&
      answer.error !== null &&
      'code' in answer.error &&
      answer.error.code === 'token_expired'
    );
  });
  await page.getByRole('button', { name: 'Send', exact: true }).click();
  const response = await saved;
  if (response.status() !== 201) throw new Error(`sending a message answered ${String(response.status())}`);
  await page.waitForFunction(() => document.querySelector('textarea')?.readOnly === false);
}

function settledReadRefresh(page: Page, channel: string): () => Promise<void> {
  const pending = new Set<Request>();
  const afterRead = new Set<Request>();
  let sent = false;
  let read = false;
  let refreshed = false;
  const channelPath = `/api/v1/channels/${channel}`;
  const started = (request: Request): void => {
    const path = new URL(request.url()).pathname;
    if (path === '/api/v1/channels' || path.startsWith(channelPath)) {
      pending.add(request);
      if (read && request.method() === 'GET') afterRead.add(request);
    }
  };
  const answered = (response: Response): void => {
    if (
      response.request().method() === 'POST' &&
      new URL(response.url()).pathname === `${channelPath}/messages` &&
      response.status() === 201
    )
      sent = true;
  };
  const finished = (request: Request): void => {
    pending.delete(request);
    const path = new URL(request.url()).pathname;
    if (sent && request.method() === 'POST' && path === `${channelPath}/read`) {
      read = true;
      refreshed = false;
      afterRead.clear();
    }
    if (path === '/api/v1/channels' && afterRead.has(request)) refreshed = true;
  };
  page.on('request', started);
  page.on('response', answered);
  page.on('requestfinished', finished);
  return async (): Promise<void> => {
    await expect.poll(() => read && refreshed && pending.size === 0).toBe(true);
    page.off('request', started);
    page.off('response', answered);
    page.off('requestfinished', finished);
  };
}

async function digestMail(control: TestControl, email: string): Promise<SentMail[]> {
  return (await control.mail(email)).filter((mail) => mail.text.includes('B1 evening'));
}

async function offlineMessage(page: Page, deployment: Deployment, actors: GroupActors): Promise<void> {
  await waitForSockets(deployment, await accountId(deployment, actors.anna), 0);
  await openChat(page, actors.boris, actors.channel);
  await send(page, 'A group message while Anna is offline');
}

test.use({ locale: 'en-GB', viewport: { width: 1280, height: 800 }, reducedMotion: 'reduce' });

test('pilot-3 › step 6 Anna sends a picture and Maria sees its preview, unread badge and online presence', async ({
  page,
  browser,
  deployment,
  diagnostics,
}) => {
  const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
  const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
  const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
  const context = await contextFor(browser, deployment);
  const anna = await context.newPage();
  await diagnostics.watch(anna);
  await signInThroughTheApi(page, tutor);
  await page.goto('/today');
  await openChat(anna, student, channel);

  await anna
    .locator('input[type=file]')
    .setInputFiles({ name: 'lesson-picture.png', mimeType: 'image/png', buffer: picture });
  await expect(anna.getByRole('list', { name: 'Attachments', exact: true })).toContainText('Uploaded');
  await send(anna, 'My lesson picture');
  await expect(anna.getByRole('textbox', { name: 'Message', exact: true })).toBeFocused();
  await expect(page.getByRole('link', { name: 'Messages, 1 unread chats', exact: true })).toBeVisible();
  await page.goto(`/messages/${channel}`);

  const preview = page.getByRole('img', { name: 'lesson-picture.png', exact: true });
  await expect(preview).toBeVisible();
  await expect
    .poll(() =>
      preview.evaluate((image) => image instanceof HTMLImageElement && image.complete && image.naturalWidth > 0),
    )
    .toBe(true);
  await expect(preview).toHaveAttribute('width', '1');
  await expect(preview).toHaveAttribute('height', '1');
  await expect(page.getByRole('main').getByText('online', { exact: true })).toHaveCount(2);
  await diagnostics.closeContext(context);
});

test('pilot-3 › step 7 tutor sees card-name typing and Anna sees anonymous group typing followed by the live message', async ({
  page,
  browser,
  deployment,
  diagnostics,
}) => {
  const actors = await groupActors(deployment);
  const annaContext = await contextFor(browser, deployment);
  const borisContext = await contextFor(browser, deployment);
  const anna = await annaContext.newPage();
  const boris = await borisContext.newPage();
  await diagnostics.watch(anna);
  await diagnostics.watch(boris);
  await openChat(page, actors.tutor, actors.channel);
  await openChat(anna, actors.anna, actors.channel);
  await openChat(boris, actors.boris, actors.channel);
  await waitForSockets(deployment, await accountId(deployment, actors.boris), 1);

  await boris.getByRole('textbox', { name: 'Message', exact: true }).fill('Group discussion');
  await expect(page.getByRole('status').filter({ hasText: 'Boris is typing' })).toBeVisible();
  await expect(anna.getByRole('status').filter({ hasText: 'Someone is typing' })).toBeVisible();
  await expect(anna.getByRole('list', { name: 'Members', exact: true })).toHaveCount(0);
  await expect(anna.getByRole('main')).not.toContainText('Boris');
  await send(boris, 'Group discussion');

  await expect(page.getByRole('list', { name: 'Messages', exact: true })).toContainText('Group discussion');
  await expect(anna.getByRole('list', { name: 'Messages', exact: true })).toContainText('Group discussion');
  await expect(boris.getByRole('textbox', { name: 'Message', exact: true })).toBeFocused();
  await diagnostics.closeContext(annaContext);
  await diagnostics.closeContext(borisContext);
});

test('pilot-3 › steps 8–11 Anna closes her tabs and receives one digest for one unread group message without a duplicate', async ({
  page,
  browser,
  deployment,
  diagnostics,
  control,
}) => {
  const actors = await groupActors(deployment);
  const context = await contextFor(browser, deployment);
  const anna = await context.newPage();
  await diagnostics.watch(anna);
  await openChat(anna, actors.anna, actors.channel);
  await waitForSockets(deployment, await accountId(deployment, actors.anna), 1);

  await diagnostics.closeContext(context);
  await waitForSockets(deployment, await accountId(deployment, actors.anna), 0);
  await control.advance(1);
  await offlineMessage(page, deployment, actors);
  await control.advance(11 * 60);
  await control.tick();
  const first = await digestMail(control, actors.anna.email);
  expect(first).toHaveLength(1);
  expect(first[0]?.text).toContain('B1 evening');
  expect(await channelUnread(deployment, actors.anna, actors.channel)).toBe(1);
  await control.advance(31 * 60);
  await control.tick();

  expect(await digestMail(control, actors.anna.email)).toHaveLength(1);
});

test('pilot-3 › steps 14–15 Anna unsubscribes through the digest and receives no new digest while offline', async ({
  page,
  browser,
  deployment,
  diagnostics,
  control,
}) => {
  const actors = await groupActors(deployment);
  const settled = settledReadRefresh(page, actors.channel);
  await offlineMessage(page, deployment, actors);
  await control.advance(11 * 60);
  await control.tick();
  const initial = await digestMail(control, actors.anna.email);
  const link = initial.flatMap((mail) => mail.links).find((address) => new URL(address).pathname === '/unsubscribe');
  if (link === undefined) throw new Error('the message digest has no unsubscribe address');
  const context = await contextFor(browser, deployment);
  const unsubscribe = await context.newPage();
  await diagnostics.watch(unsubscribe);

  await unsubscribe.goto(link);
  await unsubscribe.getByRole('button', { name: 'Turn off these emails', exact: true }).click();
  await expect(unsubscribe.getByRole('status')).toContainText('These emails are turned off');
  await expect(unsubscribe.getByRole('status')).toBeFocused();
  await diagnostics.closeContext(context);
  await waitForSockets(deployment, await accountId(deployment, actors.anna), 0);
  await settled();
  await page.getByRole('button', { name: 'Boris, Account menu', exact: true }).click();
  await page.getByRole('button', { name: 'Sign out', exact: true }).click();
  await page.getByRole('button', { name: 'Sign in', exact: true }).waitFor();
  await control.advance(31 * 60);
  await openChat(page, actors.boris, actors.channel);
  await send(page, 'Another group message after unsubscribe');
  await control.advance(11 * 60);
  await control.tick();

  expect(initial).toHaveLength(1);
  expect(await digestMail(control, actors.anna.email)).toHaveLength(1);
  expect(await channelUnread(deployment, actors.anna, actors.channel)).toBe(2);
});

test('attachments › type and quota refusals take focus and an unconfirmed student cannot attach', async ({
  page,
  browser,
  deployment,
  diagnostics,
}) => {
  const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
  const student = newTestAccount('Unconfirmed learner', 'student');
  const card = await createCard(deployment, tutor, 'Anna', student.email);
  await signUpOnInvitation(deployment, card.link, student, 'en');
  const channel = await channelOf(deployment, tutor, `/students/${card.id}`);
  await openChat(page, tutor, channel);
  diagnostics.expectFailure({
    method: 'POST',
    path: `/api/v1/uploads?context=chat&scope=channel%3A${channel}`,
    status: 415,
  });
  const context = await contextFor(browser, deployment);
  const anna = await context.newPage();
  await diagnostics.watch(anna);
  await openChat(anna, student, channel);

  await page
    .locator('input[type=file]')
    .setInputFiles({ name: 'invalid.png', mimeType: 'image/png', buffer: Buffer.from('not a picture') });

  await page.getByRole('alert').waitFor();
  const unsupportedText = await page.getByRole('alert').textContent();
  const unsupportedFocused = await page.getByRole('alert').evaluate((element) => element === document.activeElement);
  await page.getByRole('button', { name: 'Remove file', exact: true }).click();
  deployment.administer(['tutors', 'set-quota', tutor.email, '--files', '1', '--recordings', '1']);
  diagnostics.expectFailure({
    method: 'POST',
    path: `/api/v1/uploads?context=chat&scope=channel%3A${channel}`,
    status: 507,
  });
  await page
    .locator('input[type=file]')
    .setInputFiles({ name: 'over-quota.pdf', mimeType: 'application/pdf', buffer: documentBytes });

  expect(unsupportedText).toContain('not supported');
  expect(unsupportedFocused).toBe(true);
  await expect(page.getByRole('alert')).toBeFocused();
  await expect(page.getByRole('alert')).toHaveText('The file storage is full.');
  await expect(anna.getByText('Confirm your email to send files', { exact: true })).toBeVisible();
  await expect(anna.locator('input[type=file]')).toHaveCount(0);
  await diagnostics.closeContext(context);
});

test('attachments › dropped documents and pasted pictures upload and preserve the downloaded filename', async ({
  page,
  deployment,
}) => {
  const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
  const { cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
  const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
  await openChat(page, tutor, channel);

  await page.getByRole('textbox', { name: 'Message', exact: true }).evaluate(
    (element, bytes) => {
      const transfer = new DataTransfer();
      transfer.items.add(new File([new Uint8Array(bytes)], 'lesson notes.pdf', { type: 'application/pdf' }));
      element.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: transfer }));
    },
    [...documentBytes],
  );
  await page.getByRole('textbox', { name: 'Message', exact: true }).evaluate(
    (element, bytes) => {
      const transfer = new DataTransfer();
      const file = new File([new Uint8Array(bytes)], 'pasted-picture.png', { type: 'image/png' });
      transfer.items.add(file);
      const event = new ClipboardEvent('paste', { bubbles: true, cancelable: true, clipboardData: transfer });
      if (event.clipboardData?.files.length === 0) event.clipboardData.items.add(file);
      if (event.clipboardData?.files.length !== 1) throw new Error('the native paste event contains no picture');
      element.dispatchEvent(event);
    },
    [...picture],
  );
  await page
    .getByRole('list', { name: 'Attachments', exact: true })
    .getByText('Uploaded', { exact: true })
    .nth(1)
    .waitFor();
  await send(page, '');
  const downloaded = page.waitForEvent('download');
  await page.getByRole('link', { name: 'lesson notes.pdf', exact: true }).click();
  const download = await downloaded;

  expect(download.suggestedFilename()).toBe('lesson notes.pdf');
  expect(await download.failure()).toBeNull();
  await expect(page.getByRole('img', { name: 'pasted-picture.png', exact: true })).toBeVisible();
  await expect(page.getByRole('list', { name: 'Attachments', exact: true }).getByRole('listitem')).toHaveCount(0);
});

test('teaching settings › storage meter reports the server usage after a chat upload', async ({ page, deployment }) => {
  const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
  const { cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
  const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
  await openChat(page, tutor, channel);

  await page
    .locator('input[type=file]')
    .setInputFiles({ name: 'usage.pdf', mimeType: 'application/pdf', buffer: documentBytes });
  await page.getByRole('list', { name: 'Attachments', exact: true }).getByText('Uploaded', { exact: true }).waitFor();
  await send(page, 'Storage accounting');
  await page.goto('/settings/teaching');
  const response = await callApi(deployment, await accessToken(deployment, tutor), 'GET', '/tutor/settings');
  const usage = response.body;
  if (
    response.status !== 200 ||
    typeof usage !== 'object' ||
    usage === null ||
    !('file_usage_bytes' in usage) ||
    typeof usage.file_usage_bytes !== 'number' ||
    !('file_quota_bytes' in usage) ||
    typeof usage.file_quota_bytes !== 'number'
  )
    throw new Error('the storage usage response has an unexpected shape');

  await expect(page.getByRole('heading', { name: 'Storage usage', exact: true })).toBeVisible();
  await expect(page.getByRole('meter')).toHaveAttribute('value', String(usage.file_usage_bytes));
  await expect(page.getByRole('meter')).toHaveAttribute('max', String(usage.file_quota_bytes));
  expect(usage.file_usage_bytes).toBeGreaterThan(0);
});
