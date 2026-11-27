import type { BrowserContext, Page, Route } from '@playwright/test';

import { type TestAccount, pageSettled, seedAccount, signInThroughTheApi } from './accounts.ts';
import type { ObservedBrowser } from './browser-diagnostics.ts';
import type { Deployment } from './deployment.ts';
import {
  type Diagnostics,
  type PageDiagnostics,
  type TestControl,
  diagnosedTest as test,
  expect,
  seedVerifiedUser,
} from './fixtures.ts';
import {
  accountId,
  closeSockets,
  expectRefetchedAfterTheSocketIsReady,
  firstPageSocketClosed,
  trafficAfterReady,
  waitForSockets,
} from './realtime.ts';
import { localClock, localDate, readLesson, scheduleLessonsWithIds } from './schedule.ts';
import { expectDangerConfirmation } from './dangerActions.ts';
import { accessToken, callApi, createCard, acceptSignedIn, createGroup } from './tutoring.ts';

const moscow = 'Europe/Moscow';
const lessonTitle = 'Разговорная практика';
const startsInMilliseconds = 5 * 60_000;
const listenerReconnectCeilingMilliseconds = 2_000;
const roomRefreshMilliseconds = 1_000;
const tutorName = 'Мария Тьютор';
const studentName = 'Анна Ученица';
const waitingText = 'Комната откроется, когда подключится преподаватель';
const inAnotherTab = 'Урок открыт в другой вкладке';
const unavailableText = 'Видеосвязь недоступна, попробуйте ещё раз';

interface LessonSeed {
  readonly tutor: TestAccount;
  readonly student: TestAccount;
  readonly cardId: string;
  readonly lessonId: string;
}

async function seedLesson(deployment: Deployment, control: TestControl): Promise<LessonSeed> {
  const tutor = await seedVerifiedUser(deployment, tutorName);
  const student = await seedAccount(deployment, studentName, 'student', 'ru');
  const card = await createCard(deployment, tutor, 'Анна', student.email);
  await acceptSignedIn(deployment, student, card.link);
  const start = (await control.now()) + startsInMilliseconds;
  const [lesson] = await scheduleLessonsWithIds(deployment, tutor, {
    relationship_id: card.id,
    title: lessonTitle,
    date: localDate(start, moscow),
    start: localClock(start, moscow),
    duration_minutes: 60,
  });
  if (lesson === undefined) {
    throw new Error('no lesson was scheduled');
  }
  return { tutor, student, cardId: card.id, lessonId: lesson.id };
}

async function secondContext(browser: ObservedBrowser, deployment: Deployment): Promise<BrowserContext> {
  return browser.newContext({
    baseURL: deployment.origin,
    ignoreHTTPSErrors: true,
    locale: 'ru-RU',
    timezoneId: moscow,
    viewport: { width: 1280, height: 800 },
  });
}

async function followSharedClock(context: BrowserContext, control: TestControl): Promise<void> {
  await context.clock.install({ time: await control.now() });
}

async function openLesson(
  page: Page,
  account: TestAccount,
  lessonId: string,
  control: TestControl,
): Promise<ReturnType<typeof trafficAfterReady>> {
  await followSharedClock(page.context(), control);
  await signInThroughTheApi(page, account);
  const traffic = trafficAfterReady(page, [`/api/v1/lessons/${lessonId}`]);
  await page.goto(`/lessons/${lessonId}`);
  await expect(page.getByRole('heading', { level: 1, name: lessonTitle })).toBeVisible();
  await expectRefetchedAfterTheSocketIsReady(traffic);
  return traffic;
}

async function openHome(page: Page, account: TestAccount, control: TestControl): Promise<void> {
  await followSharedClock(page.context(), control);
  await signInThroughTheApi(page, account);
  await page.goto('/home');
  await expect(nextLessonOnHome(page).getByRole('link')).toContainText(lessonTitle);
}

function nextLessonOnHome(page: Page): ReturnType<Page['getByRole']> {
  return page.getByRole('region', { name: 'Следующий урок', exact: true });
}

function editLessonButton(page: Page): ReturnType<Page['getByRole']> {
  return page.getByRole('button', { name: 'Изменить урок' });
}

function roomButton(page: Page, name: string): ReturnType<Page['getByRole']> {
  return page.getByRole('region', { name: 'Видеокомната' }).getByRole('button', { name, exact: true });
}

function lessonActions(page: Page): ReturnType<Page['getByRole']> {
  return page.getByRole('button', { name: 'Действия с уроком', exact: true });
}

async function chooseLiveAction(page: Page, name: string): Promise<void> {
  await lessonActions(page).click();
  await page.locator('main [aria-expanded="true"] + div').getByRole('button', { name, exact: true }).click();
}

async function joinInNewTab(page: Page, button: string, diagnostics: Diagnostics): Promise<Page> {
  const opened = page.context().waitForEvent('page');
  await roomButton(page, button).click();
  const room = await opened;
  await diagnostics.watch(room);
  await expect(room.locator('#room')).toHaveAttribute('data-state', 'entered');
  return room;
}

async function holdLessonWrite(
  page: Page,
  action: string,
): Promise<{ held: Promise<Route>; release: () => Promise<void> }> {
  let received: (route: Route) => void = () => undefined;
  const held = new Promise<Route>((resolve) => {
    received = resolve;
  });
  await page.route(`**/api/v1/lessons/*/${action}`, (route) => {
    received(route);
  });
  return {
    held,
    release: async (): Promise<void> => {
      await (await held).continue();
      await page.unroute(`**/api/v1/lessons/*/${action}`);
    },
  };
}

async function joinThroughTheApi(deployment: Deployment, seed: LessonSeed): Promise<void> {
  const joined = await callApi(
    deployment,
    await accessToken(deployment, seed.tutor),
    'POST',
    `/lessons/${seed.lessonId}/join`,
  );
  if (joined.status !== 200) {
    throw new Error('the tutor could not join through the API');
  }
}

async function setFakeAvailability(deployment: Deployment, available: boolean): Promise<void> {
  const response = await fetch(`${deployment.apiUrl}/__test__/fake-provider/availability`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ available }),
  });
  expect(response.ok).toBe(true);
}

async function setEmbedOverride(deployment: Deployment, allowed: boolean): Promise<void> {
  const response = await fetch(`${deployment.apiUrl}/__test__/settings/embed`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ allowed }),
  });
  expect(response.ok).toBe(true);
}

async function heldJoinTab(
  page: Page,
  button: string,
  diagnostics: Diagnostics,
): Promise<{ readonly blank: Page; readonly release: () => Promise<void> }> {
  const write = await holdLessonWrite(page, 'join');
  const opened = page.context().waitForEvent('page');
  await roomButton(page, button).click();
  const blank = await opened;
  await diagnostics.watch(blank);
  await write.held;
  return { blank, release: write.release };
}

async function signOutThroughTheMenu(page: Page): Promise<void> {
  await page.getByRole('button', { name: /Меню аккаунта/ }).click();
  await page.getByRole('button', { name: 'Выйти' }).click();
}

async function studentPageOf(
  observedBrowser: ObservedBrowser,
  deployment: Deployment,
  diagnostics: Diagnostics,
): Promise<Page> {
  const context = await secondContext(observedBrowser, deployment);
  const page = await context.newPage();
  await diagnostics.watch(page);
  return page;
}

test.describe('the lesson room through a new tab', () => {
  test.use({ locale: 'ru-RU', timezoneId: moscow, viewport: { width: 1280, height: 800 } });

  test.afterEach(async ({ deployment }) => {
    await setFakeAvailability(deployment, true);
  });

  test('lesson room › before the tutor joins the student waits with no join button', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await seedLesson(deployment, control);

    await openLesson(page, seed.student, seed.lessonId, control);

    await expect(page.getByText(waitingText)).toBeVisible();
    await expect(roomButton(page, 'Подключиться')).toHaveCount(0);
  });

  test('lesson room › the tutor enters as moderator in a tab without an opener and focus lands on Open again', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.tutor, seed.lessonId, control);

    const tutorRoom = await joinInNewTab(page, 'Подключиться к уроку', diagnostics);

    await expect(tutorRoom.locator('#participant-name')).toHaveText(tutorName);
    await expect(tutorRoom.locator('#participant-role')).toHaveText('moderator');
    expect(await tutorRoom.evaluate(() => window.opener === null)).toBe(true);
    await expect(page.getByText(inAnotherTab)).toBeVisible();
    await expect(roomButton(page, 'Открыть снова')).toBeFocused();
    await expect(page.getByRole('status').filter({ hasText: 'Идёт сейчас' })).toBeVisible();
  });

  test('lesson room › the tutor joining reaches the student without a reload', async ({
    page,
    observedBrowser,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    const studentPage = await studentPageOf(observedBrowser, deployment, diagnostics);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    await openLesson(studentPage, seed.student, seed.lessonId, control);

    await joinInNewTab(page, 'Подключиться к уроку', diagnostics);

    await expect(roomButton(studentPage, 'Подключиться')).toBeVisible();
    await expect(studentPage.getByText(waitingText)).toHaveCount(0);
  });

  test('lesson room › the student enters as viewer with their own name only', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    await joinThroughTheApi(deployment, seed);
    await openLesson(page, seed.student, seed.lessonId, control);

    const studentRoom = await joinInNewTab(page, 'Подключиться', diagnostics);

    await expect(studentRoom.locator('#participant-name')).toHaveText(studentName);
    await expect(studentRoom.locator('#participant-role')).toHaveText('viewer');
    await expect(studentRoom.locator('body')).not.toContainText(tutorName);
  });

  test('lesson room › a slow join shows a focused busy button beside one blank tab', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.tutor, seed.lessonId, control);

    const { blank, release } = await heldJoinTab(page, 'Подключиться к уроку', diagnostics);

    const busy = page.getByRole('button', { name: 'Подключение…' });
    await expect(busy).toHaveAttribute('aria-busy', 'true');
    await expect(busy).toBeFocused();
    expect(blank.url()).toBe('about:blank');
    expect(page.context().pages()).toHaveLength(2);
    await release();
  });

  test('lesson room › a join in flight while the start passes and Edit leaves enters its blank tab', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    await expect(editLessonButton(page)).toBeVisible();
    const { blank, release } = await heldJoinTab(page, 'Подключиться к уроку', diagnostics);

    await control.advance(startsInMilliseconds / 1000 + 60);
    await page.clock.fastForward(startsInMilliseconds + 60_000);
    await editLessonButton(page).waitFor({ state: 'detached' });
    await release();

    await expect(blank.locator('#room')).toHaveAttribute('data-state', 'entered');
    expect(blank.isClosed()).toBe(false);
    expect(page.context().pages()).toHaveLength(2);
    await expect(editLessonButton(page)).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Действия с уроком' })).toHaveCount(1);
    await expect(roomButton(page, 'Открыть снова')).toBeFocused();
  });

  test('lesson room › pressing the busy button sends no second join and the answer enters the room', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    let joinRequests = 0;
    page.on('request', (request) => {
      if (request.url().endsWith('/join')) joinRequests += 1;
    });
    const { blank, release } = await heldJoinTab(page, 'Подключиться к уроку', diagnostics);

    await page.keyboard.press('Enter');
    await release();

    await expect(blank.locator('#room')).toHaveAttribute('data-state', 'entered');
    await expect(roomButton(page, 'Открыть снова')).toBeFocused();
    expect(joinRequests).toBe(1);
    expect(page.context().pages()).toHaveLength(2);
  });

  test('lesson room › a refused join closes the tab, explains it and keeps focus on the button', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    await setFakeAvailability(deployment, false);
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/lessons/${seed.lessonId}/join`, status: 503 });
    const opened = page.context().waitForEvent('page');

    await roomButton(page, 'Подключиться к уроку').click();
    const blank = await opened;
    await diagnostics.watch(blank);

    await expect(page.getByRole('alert')).toHaveText(unavailableText);
    await expect.poll(() => blank.isClosed()).toBe(true);
    await expect(roomButton(page, 'Подключиться к уроку')).toBeFocused();
  });

  test('lesson room › once the provider returns a join enters and clears the refusal', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    await setFakeAvailability(deployment, false);
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/lessons/${seed.lessonId}/join`, status: 503 });
    const opened = page.context().waitForEvent('page');
    await roomButton(page, 'Подключиться к уроку').click();
    await diagnostics.watch(await opened);
    await page.getByRole('alert').waitFor();
    await setFakeAvailability(deployment, true);
    await control.advance(121);

    const room = await joinInNewTab(page, 'Подключиться к уроку', diagnostics);

    await expect(room.locator('#participant-name')).toHaveText(tutorName);
    await expect(page.getByRole('alert')).toHaveCount(0);
  });

  test('lesson room › Open again asks for a new authorized link', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    const first = await joinInNewTab(page, 'Подключиться к уроку', diagnostics);

    const second = await joinInNewTab(page, 'Открыть снова', diagnostics);

    expect(second.url()).not.toBe(first.url());
    await expect(second.locator('#participant-name')).toHaveText(tutorName);
  });

  test('lesson room › signing out during a join closes the blank tab and the late answer reaches nobody', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/lessons/${seed.lessonId}/join`, status: 401 });
    const { blank, release } = await heldJoinTab(page, 'Подключиться к уроку', diagnostics);
    const closed = blank.waitForEvent('close');
    const refused = page.waitForResponse((response) => response.url().endsWith('/join'));

    await signOutThroughTheMenu(page);
    await closed;
    await release();
    await refused;
    await diagnostics.finishHttpPhase();

    await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
    expect(page.context().pages()).toEqual([page]);
    await expect(page.locator('body')).not.toContainText(lessonTitle);
  });

  test('lesson room › moving to another page during a join closes the blank tab', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    const { blank, release } = await heldJoinTab(page, 'Подключиться к уроку', diagnostics);
    const closed = blank.waitForEvent('close');
    const answered = page.waitForResponse((response) => response.url().endsWith('/join'));

    await page.getByRole('main').getByRole('link', { name: 'К расписанию' }).click();
    await closed;
    await release();
    await answered;

    expect(page.context().pages()).toEqual([page]);
    await expect(page).toHaveURL(/\/schedule/);
  });

  test('lesson room › the link stays out of browser storage, the console, the address and request bodies of the lesson page', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    const consoleTexts: string[] = [];
    page.on('console', (message) => {
      consoleTexts.push(message.text());
    });
    const requests: string[] = [];
    page.on('request', (request) => {
      requests.push(`${request.url()} ${request.postData() ?? ''}`);
    });

    const room = await joinInNewTab(page, 'Подключиться к уроку', diagnostics);
    const stored = await page.evaluate(() =>
      JSON.stringify([Object.entries(window.localStorage), Object.entries(window.sessionStorage)]),
    );

    const capability = new URL(room.url()).hash.slice(1);
    expect(capability).not.toBe('');
    expect(stored).not.toContain(capability);
    expect(consoleTexts.join('\n')).not.toContain(capability);
    expect(requests.join('\n')).not.toContain(capability);
    expect(page.url()).not.toContain(capability);
  });

  test('lesson room › signed out, the lesson address asks to sign in and shows nothing private', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.student, seed.lessonId, control);
    await signOutThroughTheMenu(page);
    await page.getByRole('heading', { level: 1, name: 'Вход' }).waitFor();
    await pageSettled(page);

    await page.goto(`/lessons/${seed.lessonId}`);

    await expect(page.getByRole('heading', { level: 1, name: 'Вход' })).toBeVisible();
    await expect(page.locator('body')).not.toContainText(lessonTitle);
    await expect(page.locator('body')).not.toContainText(studentName);
  });

  test('lesson room › after a reconnect the student sees the room the tutor opened meanwhile', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await seedLesson(deployment, control);
    const socketClosed = firstPageSocketClosed(page);
    await openLesson(page, seed.student, seed.lessonId, control);
    const student = await accountId(deployment, seed.student);
    const traffic = trafficAfterReady(page, [`/api/v1/lessons/${seed.lessonId}`]);

    await closeSockets(deployment, student, 1012);
    await joinThroughTheApi(deployment, seed);
    await socketClosed;
    await page.clock.fastForward(listenerReconnectCeilingMilliseconds);
    await expectRefetchedAfterTheSocketIsReady(traffic);

    await expect(roomButton(page, 'Подключиться')).toBeVisible();
  });

  test('lesson room › an open join is the same for a tutor returning to a live lesson', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await seedLesson(deployment, control);
    await joinThroughTheApi(deployment, seed);

    await openLesson(page, seed.tutor, seed.lessonId, control);

    await expect(roomButton(page, 'Подключиться снова')).toBeVisible();
  });
});

test.describe('ending the lesson and restarting the room', () => {
  test.use({ locale: 'ru-RU', timezoneId: moscow, viewport: { width: 1280, height: 800 } });

  test.afterEach(async ({ deployment }) => {
    await setFakeAvailability(deployment, true);
  });

  async function liveLesson(page: Page, deployment: Deployment, control: TestControl): Promise<LessonSeed> {
    const seed = await seedLesson(deployment, control);
    await joinThroughTheApi(deployment, seed);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    return seed;
  }

  for (const name of ['Завершить урок', 'Перезапустить комнату']) {
    test(`lesson room › the dialog of «${name}» opens on Cancel`, async ({ page, deployment, control }) => {
      await liveLesson(page, deployment, control);

      await chooseLiveAction(page, name);

      await expectDangerConfirmation(page.getByRole('dialog', { name: `${name}?` }), name, 'Отмена');
    });

    test(`lesson room › Escape closes the dialog of «${name}» onto Lesson actions`, async ({
      page,
      deployment,
      control,
    }) => {
      await liveLesson(page, deployment, control);
      await chooseLiveAction(page, name);

      await page.keyboard.press('Escape');

      await expect(page.getByRole('dialog')).toHaveCount(0);
      await expect(lessonActions(page)).toBeFocused();
    });

    test(`lesson room › dismissing the dialog of «${name}» returns focus to Lesson actions`, async ({
      page,
      deployment,
      control,
    }) => {
      await liveLesson(page, deployment, control);
      await chooseLiveAction(page, name);

      await page
        .getByRole('dialog', { name: `${name}?` })
        .getByRole('button', { name: 'Отмена' })
        .click();

      await expect(page.getByRole('dialog')).toHaveCount(0);
      await expect(lessonActions(page)).toBeFocused();
    });
  }

  test('lesson room › End and Restart are the dangerous items of Lesson actions while the lesson is live and are no longer below the room', async ({
    page,
    deployment,
    control,
  }) => {
    await liveLesson(page, deployment, control);

    await lessonActions(page).click();

    const items = page.locator('main [aria-expanded="true"] + div').locator(':scope > button');
    await expect(items).toHaveText(['Перезапустить комнату', 'Завершить урок']);
    await expect(items.nth(0)).toHaveClass(/text-danger/);
    await expect(items.nth(1)).toHaveClass(/text-danger/);
    await expect(items.locator('svg')).toHaveCount(2);
    await expect(roomButton(page, 'Завершить урок')).toHaveCount(0);
    await expect(roomButton(page, 'Перезапустить комнату')).toHaveCount(0);
  });

  test('lesson room › a join refusal left in the room disappears once Restart enters the room', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await liveLesson(page, deployment, control);
    await setFakeAvailability(deployment, false);
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/lessons/${seed.lessonId}/join`, status: 503 });
    const refused = page.context().waitForEvent('page');
    await roomButton(page, 'Подключиться снова').click();
    await diagnostics.watch(await refused);
    await expect(page.getByRole('region', { name: 'Видеокомната' }).getByRole('alert')).toHaveText(unavailableText);
    await setFakeAvailability(deployment, true);
    await chooseLiveAction(page, 'Перезапустить комнату');
    const opened = page.context().waitForEvent('page');

    await page
      .getByRole('dialog', { name: 'Перезапустить комнату?' })
      .getByRole('button', { name: 'Перезапустить комнату', exact: true })
      .click();
    await diagnostics.watch(await opened);

    await expect(page.getByText(inAnotherTab)).toBeVisible();
    await expect(page.getByRole('region', { name: 'Видеокомната' }).getByRole('alert')).toHaveCount(0);
  });

  test('lesson room › while a join is pending Lesson actions is unavailable and opens no dialog', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    await liveLesson(page, deployment, control);
    const { release } = await heldJoinTab(page, 'Подключиться снова', diagnostics);

    await lessonActions(page).press('Enter');

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(lessonActions(page)).toHaveAttribute('aria-disabled', 'true');
    await expect(lessonActions(page)).toHaveAttribute('aria-expanded', 'false');
    await release();
  });

  test('lesson room › confirming End closes onto the focused notice and removes the room', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await liveLesson(page, deployment, control);
    await chooseLiveAction(page, 'Завершить урок');

    await page.getByRole('dialog').getByRole('button', { name: 'Завершить урок' }).click();

    await expect(page.getByRole('status').filter({ hasText: 'Урок завершён' })).toBeFocused();
    await expect(page.getByRole('status').filter({ hasText: /^Завершён$/ })).toBeVisible();
    await expect(page.getByRole('region', { name: 'Видеокомната' })).toHaveCount(0);
    expect((await readLesson(deployment, seed.tutor, seed.lessonId)).status).toBe('ended');
  });

  test('lesson room › ending reaches the student without a reload and moves focus from the gone button', async ({
    page,
    observedBrowser,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await liveLesson(page, deployment, control);
    const studentPage = await studentPageOf(observedBrowser, deployment, diagnostics);
    await openLesson(studentPage, seed.student, seed.lessonId, control);
    await roomButton(studentPage, 'Подключиться').focus();
    await chooseLiveAction(page, 'Завершить урок');

    await page.getByRole('dialog').getByRole('button', { name: 'Завершить урок' }).click();

    await expect(studentPage.getByRole('status').filter({ hasText: /^Завершён$/ })).toBeVisible();
    await expect(roomButton(studentPage, 'Подключиться')).toHaveCount(0);
    await expect(studentPage.getByRole('heading', { level: 1 })).toBeFocused();
  });

  test('lesson room › a restart opens a fresh tab, ends the old room and returns focus to Lesson actions', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    await liveLesson(page, deployment, control);
    const oldRoom = await joinInNewTab(page, 'Подключиться снова', diagnostics);
    await chooseLiveAction(page, 'Перезапустить комнату');
    const restart = lessonActions(page);
    const dialog = page.getByRole('dialog', { name: 'Перезапустить комнату?' });
    const opened = page.context().waitForEvent('page');
    const restarted = page.waitForResponse(
      (response) =>
        response.request().method() === 'POST' && new URL(response.url()).pathname.endsWith('/restart-room'),
    );

    await dialog.getByRole('button', { name: 'Перезапустить комнату' }).click();
    const newRoom = await opened;
    await diagnostics.watch(newRoom);
    await restarted;
    await page.clock.runFor(roomRefreshMilliseconds);

    await expect(newRoom.locator('#room')).toHaveAttribute('data-state', 'entered');
    await expect(oldRoom.locator('#room')).toHaveAttribute('data-state', 'ended');
    await expect(dialog).toHaveCount(0);
    await expect(restart).toBeFocused();
    await expect(page.getByText(inAnotherTab)).toBeVisible();
  });

  test('lesson room › a refused restart stays in its dialog and closes the tab', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await liveLesson(page, deployment, control);
    await chooseLiveAction(page, 'Перезапустить комнату');
    await setFakeAvailability(deployment, false);
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/lessons/${seed.lessonId}/restart-room`, status: 503 });
    const dialog = page.getByRole('dialog', { name: 'Перезапустить комнату?' });
    const opened = page.context().waitForEvent('page');

    await dialog.getByRole('button', { name: 'Перезапустить комнату' }).click();
    const blank = await opened;
    await diagnostics.watch(blank);

    await expect(dialog.getByRole('alert')).toHaveText(unavailableText);
    await expect.poll(() => blank.isClosed()).toBe(true);
  });

  test('lesson room › a lesson ended elsewhere closes an open dialog onto the changed notice', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await liveLesson(page, deployment, control);
    await chooseLiveAction(page, 'Перезапустить комнату');

    const ended = await callApi(
      deployment,
      await accessToken(deployment, seed.tutor),
      'POST',
      `/lessons/${seed.lessonId}/end`,
    );

    expect(ended.status).toBe(200);
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: /^Этот урок тем временем изменился/ })).toBeFocused();
  });

  interface GroupRoom {
    readonly lessonId: string;
    readonly annaPage: Page;
    readonly annaRoom: Page;
    readonly annaDiagnostics: PageDiagnostics;
  }

  async function studentRemovedFromGroupRoom(
    page: Page,
    observedBrowser: ObservedBrowser,
    deployment: Deployment,
    control: TestControl,
    diagnostics: Diagnostics,
  ): Promise<GroupRoom> {
    const tutor = await seedVerifiedUser(deployment, tutorName);
    const anna = await seedAccount(deployment, studentName, 'student', 'ru');
    const card = await createCard(deployment, tutor, 'Анна', anna.email);
    await acceptSignedIn(deployment, anna, card.link);
    const group = await createGroup(deployment, tutor, 'B1', [card.id]);
    const start = (await control.now()) + startsInMilliseconds;
    const [lesson] = await scheduleLessonsWithIds(deployment, tutor, {
      group_id: group,
      title: lessonTitle,
      date: localDate(start, moscow),
      start: localClock(start, moscow),
      duration_minutes: 60,
    });
    if (lesson === undefined) {
      throw new Error('no lesson was scheduled');
    }
    const annaPage = await studentPageOf(observedBrowser, deployment, diagnostics);
    const annaDiagnostics = await diagnostics.watch(annaPage);
    await openLesson(page, tutor, lesson.id, control);
    await joinInNewTab(page, 'Подключиться к уроку', diagnostics);
    await openLesson(annaPage, anna, lesson.id, control);
    const annaRoom = await joinInNewTab(annaPage, 'Подключиться', diagnostics);
    const removed = await callApi(deployment, await accessToken(deployment, tutor), 'PATCH', `/groups/${group}`, {
      remove: [card.id],
    });
    if (removed.status !== 200) {
      throw new Error('the student was not removed from the group');
    }
    return { lessonId: lesson.id, annaPage, annaRoom, annaDiagnostics };
  }

  test('lesson room › a student removed from the group is refused a new link and focus reaches the heading', async ({
    page,
    observedBrowser,
    deployment,
    control,
    diagnostics,
  }) => {
    const room = await studentRemovedFromGroupRoom(page, observedBrowser, deployment, control, diagnostics);
    room.annaDiagnostics.expectFailure({ method: 'POST', path: `/api/v1/lessons/${room.lessonId}/join`, status: 404 });
    const again = room.annaPage.context().waitForEvent('page');

    await roomButton(room.annaPage, 'Открыть снова').click();
    const refusedTab = await again;
    await diagnostics.watch(refusedTab);

    await expect(room.annaPage.getByRole('alert')).toHaveText('Вы не можете подключиться к этому уроку');
    await expect.poll(() => refusedTab.isClosed()).toBe(true);
    await expect(room.annaPage.getByRole('heading', { level: 1 })).toBeFocused();
    await expect(room.annaRoom.locator('#room')).toHaveAttribute('data-state', 'entered');
  });
});

test.describe('the embedded room under the test override', () => {
  test.use({ locale: 'ru-RU', timezoneId: moscow, viewport: { width: 1280, height: 800 } });

  test.beforeEach(async ({ deployment, browserName, diagnostics }) => {
    if (browserName === 'firefox') {
      diagnostics.expectConsoleWarning('Skipping unsupported feature name “autoplay”');
    }
    await setEmbedOverride(deployment, true);
  });

  test.afterEach(async ({ deployment }) => {
    await setEmbedOverride(deployment, false);
  });

  function frameOf(page: Page): ReturnType<Page['locator']> {
    return page.locator(`iframe[title="${lessonTitle}"]`);
  }

  async function nextRoomDocument(page: Page): Promise<void> {
    const frame = await page.waitForEvent('framenavigated', (navigated) =>
      navigated.url().includes('/__test__/fake-room'),
    );
    await frame.waitForLoadState('load');
  }

  async function embeddedRoom(page: Page, deployment: Deployment, control: TestControl): Promise<LessonSeed> {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    const loaded = nextRoomDocument(page);
    await roomButton(page, 'Подключиться к уроку').click();
    await loaded;
    await expect(page.frameLocator(`iframe[title="${lessonTitle}"]`).locator('#room')).toHaveAttribute(
      'data-state',
      'entered',
    );
    return seed;
  }

  async function leaveTheFrame(page: Page): Promise<void> {
    const room = page.frameLocator(`iframe[title="${lessonTitle}"]`);
    await room.getByRole('button', { name: 'Leave' }).click();
    await expect(room.locator('#room')).toHaveAttribute('data-state', 'left');
  }

  test('lesson room › the frame loads one room document and keeps focus while the lesson goes live and Edit leaves', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    await expect(editLessonButton(page)).toBeVisible();
    let roomDocuments = 0;
    page.on('framenavigated', (navigated) => {
      if (navigated.url().includes('/__test__/fake-room')) roomDocuments += 1;
    });
    const loaded = nextRoomDocument(page);

    await roomButton(page, 'Подключиться к уроку').click();
    await loaded;
    await page.getByRole('status').filter({ hasText: 'Идёт сейчас' }).waitFor();
    await editLessonButton(page).waitFor({ state: 'detached' });
    await page.frameLocator(`iframe[title="${lessonTitle}"]`).locator('#room[data-state="entered"]').waitFor();
    const lessonActionsShown = await page.getByRole('button', { name: 'Действия с уроком' }).count();
    const frameFocused = await frameOf(page).evaluate((frame) => frame === document.activeElement);
    const frames = await frameOf(page).count();
    const documentsWhileLive = roomDocuments;
    await leaveTheFrame(page);

    expect(lessonActionsShown).toBe(1);
    expect(frameFocused).toBe(true);
    expect(frames).toBe(1);
    expect(documentsWhileLive).toBe(1);
  });

  test('lesson room › the frame is titled, permitted, focused and stays beside a new-tab button', async ({
    page,
    deployment,
    control,
  }) => {
    await embeddedRoom(page, deployment, control);

    await expect(frameOf(page)).toHaveAttribute('allow', 'camera; microphone; display-capture; fullscreen; autoplay');
    await expect(frameOf(page)).toHaveAttribute('referrerpolicy', 'no-referrer');
    await expect(frameOf(page)).toBeFocused();
    await expect(roomButton(page, 'Открыть в новой вкладке')).toBeVisible();
    await expect(page.frameLocator(`iframe[title="${lessonTitle}"]`).locator('#participant-name')).toHaveText(
      tutorName,
    );
    expect(page.context().pages()).toHaveLength(1);
  });

  async function inFocusMode(page: Page, deployment: Deployment, control: TestControl): Promise<void> {
    await embeddedRoom(page, deployment, control);
    await roomButton(page, 'Режим фокуса').click();
    await expect(roomButton(page, 'Выйти из режима фокуса')).toBeFocused();
  }

  test('lesson room › focus mode fills the viewport height without reloading the room', async ({
    page,
    deployment,
    control,
  }) => {
    await embeddedRoom(page, deployment, control);
    let frameDocuments = 0;
    page.on('framenavigated', () => {
      frameDocuments += 1;
    });

    await roomButton(page, 'Режим фокуса').click();

    const area = page.locator('[data-part="room"]');
    await expect(roomButton(page, 'Выйти из режима фокуса')).toBeFocused();
    await expect.poll(async () => (await area.boundingBox())?.height).toBe(800);
    await expect.poll(async () => Math.abs((await area.boundingBox())?.y ?? 1)).toBeLessThan(1);
    expect(frameDocuments).toBe(0);
  });

  test('lesson room › Exit focus leaves focus mode and keeps focus on its button', async ({
    page,
    deployment,
    control,
  }) => {
    await inFocusMode(page, deployment, control);

    await roomButton(page, 'Выйти из режима фокуса').click();

    await expect(roomButton(page, 'Режим фокуса')).toBeFocused();
    await expect.poll(async () => (await page.locator('[data-part="room"]').boundingBox())?.height).toBeLessThan(800);
  });

  test('lesson room › Escape leaves focus mode and keeps focus on its button', async ({
    page,
    deployment,
    control,
  }) => {
    await inFocusMode(page, deployment, control);

    await page.keyboard.press('Escape');

    await expect(roomButton(page, 'Режим фокуса')).toBeFocused();
    await expect.poll(async () => (await page.locator('[data-part="room"]').boundingBox())?.height).toBeLessThan(800);
  });

  test('lesson room › a frame unloaded for 15 seconds highlights the new-tab button until it loads', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    const received: Route[] = [];
    await page.route('**/__test__/fake-room**', (route) => {
      received.push(route);
    });
    await roomButton(page, 'Подключиться к уроку').click();
    await expect(frameOf(page)).toBeVisible();
    await page.clock.fastForward(15_000);
    await roomButton(page, 'Возникли проблемы? Откройте в новой вкладке').waitFor();
    await expect.poll(() => received.length).toBeGreaterThan(0);

    await Promise.all(received.map((route) => route.continue()));
    await page.unroute('**/__test__/fake-room**');

    await expect(roomButton(page, 'Открыть в новой вкладке')).toBeVisible();
  });

  test('lesson room › switching to a new tab removes the frame first and focus stays on the room button', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    await embeddedRoom(page, deployment, control);
    await leaveTheFrame(page);
    const write = await holdLessonWrite(page, 'join');
    const opened = page.context().waitForEvent('page');

    await roomButton(page, 'Открыть в новой вкладке').click();
    const room = await opened;
    await diagnostics.watch(room);
    await write.held;

    await expect(frameOf(page)).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Подключение…' })).toBeFocused();
    await write.release();
  });

  test('lesson room › the answer after switching enters the tab and focus lands on Open again', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    await embeddedRoom(page, deployment, control);
    await leaveTheFrame(page);

    const room = await joinInNewTab(page, 'Открыть в новой вкладке', diagnostics);

    await expect(room.locator('#participant-name')).toHaveText(tutorName);
    await expect(frameOf(page)).toHaveCount(0);
    await expect(roomButton(page, 'Открыть снова')).toBeFocused();
  });

  test('lesson room › a restart replaces the frame and returns focus to Lesson actions', async ({
    page,
    deployment,
    control,
  }) => {
    await embeddedRoom(page, deployment, control);
    const oldAddress = await frameOf(page).getAttribute('src');
    await leaveTheFrame(page);
    await chooseLiveAction(page, 'Перезапустить комнату');
    const loaded = nextRoomDocument(page);

    await page.getByRole('dialog').getByRole('button', { name: 'Перезапустить комнату' }).click();
    await loaded;

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect.poll(() => frameOf(page).getAttribute('src')).not.toBe(oldAddress);
    await expect(page.frameLocator(`iframe[title="${lessonTitle}"]`).locator('#room')).toHaveAttribute(
      'data-state',
      'entered',
    );
    await expect(lessonActions(page)).toBeFocused();
  });

  test('lesson room › ending removes the frame and focus lands on the notice', async ({
    page,
    deployment,
    control,
  }) => {
    await embeddedRoom(page, deployment, control);
    await leaveTheFrame(page);
    await chooseLiveAction(page, 'Завершить урок');

    await page.getByRole('dialog').getByRole('button', { name: 'Завершить урок' }).click();

    await expect(frameOf(page)).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: 'Урок завершён' })).toBeFocused();
  });

  test('lesson room › leaving the page removes the frame', async ({ page, deployment, control }) => {
    await embeddedRoom(page, deployment, control);
    await leaveTheFrame(page);

    await page.getByRole('main').getByRole('link', { name: 'К расписанию' }).click();

    await expect(page).toHaveURL(/\/schedule/);
    await expect(frameOf(page)).toHaveCount(0);
  });
});

test.describe('the room on a phone under the test override', () => {
  test.use({ locale: 'ru-RU', timezoneId: moscow, viewport: { width: 360, height: 780 } });

  test.beforeEach(async ({ deployment }) => {
    await setEmbedOverride(deployment, true);
  });

  test.afterEach(async ({ deployment }) => {
    await setEmbedOverride(deployment, false);
  });

  test('lesson room › at 360 px the header actions sit below the title, Join fills the width, takes the new-tab path without an opener and focus lands on Open again', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    const joinWidth = await roomButton(page, 'Подключиться к уроку').evaluate(
      (button) => button.getBoundingClientRect().width,
    );
    const titleBottom = await page
      .getByRole('heading', { level: 1, name: lessonTitle })
      .evaluate((heading) => heading.getBoundingClientRect().bottom);
    const editTop = await editLessonButton(page).evaluate((button) => button.getBoundingClientRect().top);
    const iconSizes = await page
      .getByRole('main')
      .locator('svg[data-outline-icon]')
      .evaluateAll((icons) => icons.map((icon) => icon.getBoundingClientRect().width));

    const tutorRoom = await joinInNewTab(page, 'Подключиться к уроку', diagnostics);

    expect(joinWidth).toBeGreaterThanOrEqual(290);
    expect(editTop).toBeGreaterThanOrEqual(titleBottom);
    expect(iconSizes.length).toBeGreaterThan(0);
    expect(iconSizes.every((size) => size === 20)).toBe(true);
    await expect(tutorRoom.locator('#participant-role')).toHaveText('moderator');
    expect(await tutorRoom.evaluate(() => window.opener === null)).toBe(true);
    await expect(page.locator('iframe')).toHaveCount(0);
    await expect(roomButton(page, 'Режим фокуса')).toHaveCount(0);
    await expect(roomButton(page, 'Открыть снова')).toBeFocused();
  });
});

test.describe('pilot scenario 1 in the lesson room', () => {
  test.use({ locale: 'ru-RU', timezoneId: moscow, viewport: { width: 1280, height: 800 } });

  test('pilot-1 › steps 8–11 Five minutes before the start Anna waits on Home, Maria joins as moderator and Anna joins from Home as viewer without a reload', async ({
    page,
    observedBrowser,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    const annaContext = await secondContext(observedBrowser, deployment);
    const annaPage = await annaContext.newPage();
    await diagnostics.watch(annaPage);
    await openHome(annaPage, seed.student, control);
    await waitForSockets(deployment, await accountId(deployment, seed.student), 1);

    await expect(nextLessonOnHome(annaPage).getByText(waitingText)).toBeVisible();
    await expect(nextLessonOnHome(annaPage).getByRole('button', { name: 'Подключиться', exact: true })).toHaveCount(0);

    await openLesson(page, seed.tutor, seed.lessonId, control);
    const mariaRoom = await joinInNewTab(page, 'Подключиться к уроку', diagnostics);
    await expect(mariaRoom.locator('#participant-role')).toHaveText('moderator');
    await expect(mariaRoom.locator('#participant-name')).toHaveText(tutorName);

    const annaJoin = nextLessonOnHome(annaPage).getByRole('button', { name: 'Подключиться', exact: true });
    await expect(annaJoin).toBeVisible();
    const annaOpened = annaPage.context().waitForEvent('page');
    await annaJoin.click();
    const annaRoom = await annaOpened;
    await diagnostics.watch(annaRoom);
    await expect(annaRoom.locator('#room')).toHaveAttribute('data-state', 'entered');
    await expect(annaRoom.locator('#participant-role')).toHaveText('viewer');
    await expect(annaRoom.locator('#participant-name')).toHaveText(studentName);
    await expect(annaPage).toHaveURL('/home');

    await diagnostics.closeContext(annaContext);
  });

  test('pilot-1 › step 13 Maria ends the lesson, both see Ended, and no Recordings block appears', async ({
    page,
    observedBrowser,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await seedLesson(deployment, control);
    const annaContext = await secondContext(observedBrowser, deployment);
    const annaPage = await annaContext.newPage();
    await diagnostics.watch(annaPage);
    await openLesson(page, seed.tutor, seed.lessonId, control);
    await joinInNewTab(page, 'Подключиться к уроку', diagnostics);
    await openLesson(annaPage, seed.student, seed.lessonId, control);

    await chooseLiveAction(page, 'Завершить урок');
    await page.getByRole('dialog').getByRole('button', { name: 'Завершить урок' }).click();

    for (const shown of [page, annaPage]) {
      await expect(shown.getByRole('status').filter({ hasText: /^Завершён$/ })).toBeVisible();
      await expect(shown.getByRole('main').getByText(/запис/i)).toHaveCount(0);
    }

    await diagnostics.closeContext(annaContext);
  });
});
