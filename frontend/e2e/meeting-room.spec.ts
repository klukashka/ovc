import type { Locator, Page, Request } from '@playwright/test';

import { type TestAccount } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { type TestControl, diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { localClock, localDate, scheduleLessonsWithIds } from './schedule.ts';
import { type ApiAnswer, accessToken, callApi, seedActiveStudent } from './tutoring.ts';

const moscow = 'Europe/Moscow';
const lessonTitle = 'Private lesson title sentinel';
const neutralRoomName = 'OVC lesson';
const tutorName = 'Мария Тьютор';
const studentName = 'Анна Ученица';
const hostileName = '<img src=x onerror="window.hostile=1">';
const startsInMilliseconds = 300_000;
const refreshMilliseconds = 1_000;
const roomPath = '/__test__/fake-room';

interface LessonParties {
  readonly tutor: TestAccount;
  readonly student: TestAccount;
  readonly lesson: string;
}

async function lessonFor(
  deployment: Deployment,
  control: TestControl,
  names: { readonly tutor: string; readonly student: string },
): Promise<LessonParties> {
  const tutor = await seedVerifiedUser(deployment, names.tutor, 'en');
  const { student, cardId } = await seedActiveStudent(deployment, tutor, names.student);
  const start = (await control.now()) + startsInMilliseconds;
  const [lesson] = await scheduleLessonsWithIds(deployment, tutor, {
    relationship_id: cardId,
    title: lessonTitle,
    date: localDate(start, moscow),
    start: localClock(start, moscow),
    duration_minutes: 60,
  });
  if (lesson === undefined) {
    throw new Error('no lesson was scheduled');
  }
  return { tutor, student, lesson: lesson.id };
}

function textField(body: unknown, name: string): string {
  if (typeof body !== 'object' || body === null || !(name in body)) {
    throw new Error(`the answer has no ${name}`);
  }
  const value = (body as Record<string, unknown>)[name];
  if (typeof value !== 'string') {
    throw new Error(`the field ${name} is not text`);
  }
  return value;
}

async function lessonAction(
  deployment: Deployment,
  account: TestAccount,
  lesson: string,
  action: 'join' | 'end' | 'restart-room',
): Promise<ApiAnswer> {
  return callApi(deployment, await accessToken(deployment, account), 'POST', `/lessons/${lesson}/${action}`);
}

async function joinLink(
  deployment: Deployment,
  account: TestAccount,
  lesson: string,
  action: 'join' | 'restart-room' = 'join',
): Promise<string> {
  const answer = await lessonAction(deployment, account, lesson, action);
  expect(answer.status).toBe(200);
  return textField(answer.body, 'url');
}

async function testControlPost(deployment: Deployment, path: string, body: unknown): Promise<Response> {
  return fetch(`${deployment.apiUrl}${path}`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

function roomState(page: Page): Locator {
  return page.locator('#room');
}

async function expectRoomState(page: Page, state: string): Promise<void> {
  await expect(roomState(page)).toHaveAttribute('data-state', state);
}

async function roomReaches(page: Page, state: string): Promise<void> {
  await page.locator(`#room[data-state="${state}"]`).waitFor();
}

async function expectInRoom(page: Page, name: string, role: string): Promise<void> {
  await expectRoomState(page, 'entered');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText(neutralRoomName);
  await expect(page.locator('#participant-name')).toHaveText(name);
  await expect(page.locator('#participant-role')).toHaveText(role);
}

async function leaveRoom(page: Page): Promise<void> {
  await page.getByRole('button', { name: 'Leave' }).click();
  await expectRoomState(page, 'left');
}

function roomActions(page: Page): string[] {
  const actions: string[] = [];
  page.on('request', (request: Request) => {
    const { pathname } = new URL(request.url());
    if (pathname.startsWith(`${roomPath}/`) && request.method() === 'POST') {
      actions.push(pathname.slice(roomPath.length + 1));
    }
  });
  return actions;
}

async function pausedRoom(
  page: Page,
  deployment: Deployment,
  control: TestControl,
): Promise<{ readonly actions: string[]; readonly link: string }> {
  const { tutor, lesson } = await lessonFor(deployment, control, { tutor: tutorName, student: studentName });
  const link = await joinLink(deployment, tutor, lesson);
  const actions = roomActions(page);
  const clockStart = Date.UTC(2030, 0, 1);
  await page.clock.install({ time: clockStart });
  await page.clock.pauseAt(clockStart + refreshMilliseconds);
  await page.goto(link);
  await expectInRoom(page, tutorName, 'moderator');
  return { actions, link };
}

async function oneRefresh(page: Page): Promise<void> {
  const answered = page.waitForResponse((response) => new URL(response.url()).pathname === `${roomPath}/state`);
  await page.clock.runFor(refreshMilliseconds);
  await answered;
}

test.describe('the fake meeting room', () => {
  test('each participant sees the neutral room with their own name and role only', async ({
    page,
    context,
    deployment,
    control,
    diagnostics,
  }) => {
    const { tutor, student, lesson } = await lessonFor(deployment, control, { tutor: tutorName, student: studentName });
    const outsider = await seedVerifiedUser(deployment, 'Another tutor', 'en');
    const tutorLink = await joinLink(deployment, tutor, lesson);
    const studentLink = await joinLink(deployment, student, lesson);
    const studentPage = await context.newPage();
    await diagnostics.watch(studentPage);

    await page.goto(tutorLink);
    await studentPage.goto(studentLink);
    const outsiderJoin = await lessonAction(deployment, outsider, lesson, 'join');

    await expectInRoom(page, tutorName, 'moderator');
    await expectInRoom(studentPage, studentName, 'viewer');
    expect(new URL(tutorLink).origin).toBe(deployment.roomOrigin);
    expect(new URL(tutorLink).search).toBe('');
    expect(outsiderJoin.status).toBe(404);
    const tutorText = await page.locator('body').innerText();
    const studentText = await studentPage.locator('body').innerText();
    expect(tutorText).not.toContain(studentName);
    expect(studentText).not.toContain(tutorName);
    for (const text of [tutorText, studentText]) {
      expect(text).not.toContain(lessonTitle);
      expect(text.toLowerCase()).not.toContain('record');
    }
    await leaveRoom(page);
    await leaveRoom(studentPage);
  });

  test('a hostile name renders as text', async ({ page, deployment, control }) => {
    const { tutor, lesson } = await lessonFor(deployment, control, { tutor: hostileName, student: studentName });
    const link = await joinLink(deployment, tutor, lesson);

    await page.goto(link);

    await expectInRoom(page, hostileName, 'moderator');
    await expect(page.locator('img')).toHaveCount(0);
    expect(await page.evaluate(() => 'hostile' in window)).toBe(false);
    await leaveRoom(page);
  });

  test('a changed capability opens no room and stops', async ({ page, deployment, control }) => {
    const { tutor, lesson } = await lessonFor(deployment, control, { tutor: tutorName, student: studentName });
    const link = await joinLink(deployment, tutor, lesson);
    const lastCharacter = link.endsWith('0') ? '1' : '0';
    const actions = roomActions(page);
    await page.clock.install();

    await page.goto(`${link.slice(0, -1)}${lastCharacter}`);
    await roomReaches(page, 'refused');
    await page.clock.runFor(5 * refreshMilliseconds);

    await expect(page.locator('#identity')).toBeHidden();
    expect(actions).toEqual(['enter']);
  });

  test('a missing capability opens no room and asks nothing', async ({ page, deployment }) => {
    const actions = roomActions(page);

    await page.goto(`${deployment.roomOrigin}${roomPath}`);

    await expectRoomState(page, 'refused');
    expect(actions).toEqual([]);
  });

  test('an oversized capability opens no room and asks nothing', async ({ page, deployment }) => {
    const actions = roomActions(page);

    await page.goto(`${deployment.roomOrigin}${roomPath}#${'a'.repeat(5_000)}`);

    await expectRoomState(page, 'refused');
    expect(actions).toEqual([]);
  });

  test('a restart ends the room that is open', async ({ page, deployment, control }) => {
    const { tutor, lesson } = await lessonFor(deployment, control, { tutor: tutorName, student: studentName });
    const link = await joinLink(deployment, tutor, lesson);
    await page.clock.install();
    await page.goto(link);
    await expectInRoom(page, tutorName, 'moderator');

    await joinLink(deployment, tutor, lesson, 'restart-room');
    await page.clock.runFor(refreshMilliseconds);

    await expectRoomState(page, 'ended');
    await expect(page.locator('#identity')).toBeHidden();
    await expect(page.locator('body')).not.toContainText(tutorName);
  });

  test('the fake end control ends the room for its participant', async ({ page, deployment, control }) => {
    const { tutor, lesson } = await lessonFor(deployment, control, { tutor: tutorName, student: studentName });
    const link = await joinLink(deployment, tutor, lesson);
    await page.clock.install();
    await page.goto(link);
    await expectInRoom(page, tutorName, 'moderator');

    const ended = await testControlPost(deployment, '/__test__/fake-provider/end', {
      capability: new URL(link).hash.slice(1),
    });
    await page.clock.runFor(refreshMilliseconds);

    expect(ended.status).toBe(200);
    await expectRoomState(page, 'ended');
  });

  test('refreshing runs once a second', async ({ page, deployment, control }) => {
    const { actions } = await pausedRoom(page, deployment, control);

    await page.clock.runFor(refreshMilliseconds - 1);
    const beforeTheSecond = [...actions];
    await oneRefresh(page);

    expect(beforeTheSecond).toEqual(['enter']);
    expect(actions).toEqual(['enter', 'state']);
  });

  test('refreshing stops on leave', async ({ page, deployment, control }) => {
    const { actions } = await pausedRoom(page, deployment, control);

    await leaveRoom(page);
    await page.clock.runFor(5 * refreshMilliseconds);

    expect(actions).toEqual(['enter', 'leave']);
  });

  test('refreshing resumes after entering again', async ({ page, deployment, control }) => {
    const { actions } = await pausedRoom(page, deployment, control);
    await leaveRoom(page);

    await page.getByRole('button', { name: 'Enter again' }).click();
    await roomReaches(page, 'entered');
    await oneRefresh(page);

    expect(actions).toEqual(['enter', 'leave', 'enter', 'state']);
  });

  test('refreshing stops on disposal', async ({ page, deployment, control }) => {
    const { actions } = await pausedRoom(page, deployment, control);

    await page.evaluate(() => window.dispatchEvent(new Event('pagehide')));
    await page.clock.runFor(5 * refreshMilliseconds);

    expect(actions).toEqual(['enter']);
  });

  test('refreshing stops when the room ends', async ({ page, deployment, control }) => {
    const { actions, link } = await pausedRoom(page, deployment, control);
    const ended = await testControlPost(deployment, '/__test__/fake-provider/end', {
      capability: new URL(link).hash.slice(1),
    });

    await page.clock.runFor(refreshMilliseconds);
    await roomReaches(page, 'ended');
    await page.clock.runFor(5 * refreshMilliseconds);

    expect(ended.status).toBe(200);
    expect(actions).toEqual(['enter', 'state']);
  });

  test('a provider outage shows in the room and passes', async ({ page, deployment, control }) => {
    const { tutor, lesson } = await lessonFor(deployment, control, { tutor: tutorName, student: studentName });
    const link = await joinLink(deployment, tutor, lesson);
    await page.clock.install();
    await page.goto(link);
    await expectInRoom(page, tutorName, 'moderator');

    const down = await testControlPost(deployment, '/__test__/fake-provider/availability', { available: false });
    try {
      expect(down.status).toBe(200);
      await page.clock.runFor(refreshMilliseconds);
      await expectRoomState(page, 'unavailable');
    } finally {
      const up = await testControlPost(deployment, '/__test__/fake-provider/availability', { available: true });
      expect(up.status).toBe(200);
    }
    await page.clock.runFor(refreshMilliseconds);

    await expectInRoom(page, tutorName, 'moderator');
    await leaveRoom(page);
  });
});
