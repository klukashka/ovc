import type { Page } from '@playwright/test';

import { type TestAccount, newTestAccount, registerTutor, seedAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { type Diagnostics, diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { accountId, expectReadyForNavigation, todayQueryPaths, trafficAfterReady, waitForSockets } from './realtime.ts';
import { createPastSimpleTemplate, onlySubmission, pastSimple } from './assignment.ts';
import { issueThroughTheApi } from './homework.ts';
import { channelOf } from './messages.ts';
import { answerTextAndSubmit } from './review.ts';
import {
  addDays,
  cancelThisLesson,
  daytimeZone,
  hourIn,
  isoWeekday,
  localClock,
  localDate,
  scheduleLessonsWithIds,
} from './schedule.ts';
import {
  accessToken,
  callApi,
  createCard,
  createGroup,
  enterTutorCode,
  seedActiveStudent,
  seedJoinRequests,
  tutorCode,
} from './tutoring.ts';

const markup = '<img src=x onerror="document.title=1"><b>Bold</b>';

interface LocalTime {
  readonly zone: string;
  readonly date: string;
  readonly start: string;
}

function laterToday(now: number): LocalTime {
  const zone = daytimeZone(now);
  const start = hourIn(now, zone) + 3;
  return { zone, date: localDate(now, zone), start: `${String(start).padStart(2, '0')}:00` };
}

async function setTimezone(deployment: Deployment, account: TestAccount, zone: string): Promise<void> {
  const answer = await callApi(deployment, await accessToken(deployment, account), 'PATCH', '/users/me', {
    timezone: zone,
  });
  expect(answer.status).toBe(200);
}

async function setBio(deployment: Deployment, account: TestAccount): Promise<void> {
  const answer = await callApi(deployment, await accessToken(deployment, account), 'PATCH', '/users/me', {
    bio: 'I teach conversation.',
  });
  expect(answer.status).toBe(200);
}

async function tutorWithLessonToday(
  deployment: Deployment,
  now: number,
  names: { readonly tutor: string; readonly student: string; readonly title: string },
): Promise<{ readonly tutor: TestAccount; readonly student: TestAccount; readonly cardId: string }> {
  const when = laterToday(now);
  const tutor = await seedVerifiedUser(deployment, names.tutor, 'en');
  await setTimezone(deployment, tutor, when.zone);
  const { student, cardId } = await seedActiveStudent(deployment, tutor, names.student, 'en');
  await scheduleLessonsWithIds(deployment, tutor, {
    relationship_id: cardId,
    title: names.title,
    date: when.date,
    start: when.start,
    duration_minutes: 60,
  });
  return { tutor, student, cardId };
}

async function createAssignment(deployment: Deployment, account: TestAccount): Promise<void> {
  const answer = await callApi(deployment, await accessToken(deployment, account), 'POST', '/library/templates', {
    title: 'First quiz',
    folder_id: null,
    blocks: [],
  });
  expect(answer.status).toBe(201);
}

async function openToday(page: Page, now: number): Promise<void> {
  await page.clock.install({ time: now });
  await page.goto('/today');
}

function lessons(page: Page): ReturnType<Page['getByRole']> {
  return page.getByRole('list', { name: 'Today’s lessons' });
}

function checklist(page: Page): ReturnType<Page['getByRole']> {
  return page.getByRole('region', { name: 'Getting started' });
}

test.describe('Today', () => {
  test.use({ locale: 'en-GB', viewport: { width: 1440, height: 900 }, colorScheme: 'light' });

  test('a tutor with a lesson today sees it on Today', async ({ page, deployment, control }) => {
    const now = await control.now();
    const when = laterToday(now);
    const { tutor, cardId } = await tutorWithLessonToday(deployment, now, {
      tutor: 'Tutor',
      student: 'Vera Student',
      title: 'Conversation practice',
    });
    await scheduleLessonsWithIds(deployment, tutor, {
      relationship_id: cardId,
      title: 'Tomorrow lesson',
      date: addDays(when.date, 1),
      start: when.start,
      duration_minutes: 60,
    });
    await signInThroughTheApi(page, tutor);

    await openToday(page, now);

    const row = lessons(page).getByRole('link');
    await expect(row).toHaveCount(1);
    await expect(row).toContainText('Conversation practice');
    await expect(row).toContainText('Vera Student');
    await expect(row).not.toContainText('Scheduled');
    await expect(row).toContainText(when.start);
    await expect(row).toHaveAttribute('href', /^\/lessons\//);
    await expect(page.getByRole('main')).not.toContainText('Tomorrow lesson');
  });

  test('a new tutor sees the four undone checklist steps', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'New tutor', 'en');
    await signInThroughTheApi(page, tutor);

    await page.goto('/today');

    const steps = checklist(page).getByRole('listitem');
    await expect(steps).toHaveCount(4);
    await expect(steps).toHaveText([
      /Fill in your profile.*not done/,
      /Invite a student.*not done/,
      /Schedule a lesson.*not done/,
      /Create an assignment.*not done/,
    ]);
    await expect(checklist(page).getByRole('link', { name: 'Open your profile' })).toHaveAttribute(
      'href',
      '/settings/profile',
    );
  });

  test('a tutor with a bio, a student, a lesson and an assignment sees no checklist', async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const { tutor } = await tutorWithLessonToday(deployment, now, {
      tutor: 'Busy tutor',
      student: 'Vera Student',
      title: 'Lesson',
    });
    await setBio(deployment, tutor);
    await createAssignment(deployment, tutor);
    await signInThroughTheApi(page, tutor);
    const ready = trafficAfterReady(page, todayQueryPaths);
    await ready.initialize();

    await openToday(page, now);
    await expectReadyForNavigation(page, ready);

    await expect(lessons(page).getByRole('link')).toHaveCount(1);
    await expect(checklist(page)).toHaveCount(0);
  });

  test('an unconfirmed tutor sees the confirmation prompt in the checklist and no closed route is requested', async ({
    page,
    deployment,
  }) => {
    const account = newTestAccount('Unconfirmed tutor');
    await registerTutor(deployment, account, 'en');
    await signInThroughTheApi(page, account);
    const closedPaths = [
      '/api/v1/students',
      '/api/v1/join-requests',
      '/api/v1/groups',
      '/api/v1/issues',
      '/api/v1/review-queue',
    ];
    const closedRequests: string[] = [];
    page.on('request', (request) => {
      const path = new URL(request.url()).pathname;
      if (closedPaths.includes(path)) {
        closedRequests.push(path);
      }
    });

    await page.goto('/today');

    await expect(checklist(page).getByText('Confirm your email to invite students.')).toBeVisible();
    await expect(checklist(page).getByText('Confirm your email to schedule lessons.')).toBeVisible();
    await expect(page.getByRole('region', { name: 'Unread messages' }).getByText('No unread messages')).toBeVisible();
    expect(closedRequests).toEqual([]);
  });

  test('Dismiss hides the checklist for good and moves focus to the heading of today’s lessons', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Dismissing tutor', 'en');
    await signInThroughTheApi(page, tutor);
    let dismissals = 0;
    page.on('request', (request) => {
      if (new URL(request.url()).pathname === '/api/v1/tutor/settings' && request.method() === 'PATCH') {
        dismissals += 1;
      }
    });
    await page.goto('/today');
    const dismiss = checklist(page).getByRole('button', { name: 'Dismiss' });

    await dismiss.click();
    await expect(checklist(page)).toHaveCount(0);
    await page.reload();
    await page.getByRole('heading', { name: 'Today’s lessons' }).waitFor();

    await expect(checklist(page)).toHaveCount(0);
    expect(dismissals).toBe(1);
    const settings = await callApi(deployment, await accessToken(deployment, tutor), 'GET', '/tutor/settings');
    expect(settings.body).toMatchObject({ onboarding_dismissed: true });
  });

  test('unread messages show the channel name and count without a preview', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Tutor', 'en');
    const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Vera Student', 'en');
    const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
    const sent = await callApi(
      deployment,
      await accessToken(deployment, student),
      'POST',
      `/channels/${channel}/messages`,
      {
        body: 'A secret question about homework',
        attachment_ids: [],
      },
    );
    expect(sent.status).toBe(201);
    await signInThroughTheApi(page, tutor);

    await page.goto('/today');

    const unread = page.getByRole('region', { name: 'Unread messages' });
    const row = unread.getByRole('listitem');
    await expect(row).toHaveCount(1);
    await expect(row).toContainText('Vera Student');
    await expect(row).toContainText('1');
    await expect(row.getByRole('link')).toHaveAttribute('href', `/messages/${channel}`);
    await expect(page.getByRole('main')).not.toContainText('A secret question about homework');
    await expect(unread.getByRole('link', { name: 'All messages, 1 unread' })).toHaveAttribute('href', '/messages');
  });

  test('Today makes only its listed requests', async ({ page, deployment, control }) => {
    const now = await control.now();
    const when = laterToday(now);
    const { tutor, cardId } = await tutorWithLessonToday(deployment, now, {
      tutor: 'Tutor',
      student: 'Vera Student',
      title: 'First lesson',
    });
    for (const [title, hourOffset] of [
      ['Second lesson', 1],
      ['Third lesson', 2],
    ] as const) {
      await scheduleLessonsWithIds(deployment, tutor, {
        relationship_id: cardId,
        title,
        date: when.date,
        start: `${String(Number(when.start.slice(0, 2)) + hourOffset).padStart(2, '0')}:00`,
        duration_minutes: 60,
      });
    }
    await signInThroughTheApi(page, tutor);
    const requested: string[] = [];
    page.on('request', (request) => {
      const url = new URL(request.url());
      if (
        url.pathname.startsWith('/api/v1/') &&
        url.pathname !== '/api/v1/ws' &&
        !url.pathname.startsWith('/api/v1/auth/')
      ) {
        requested.push(`${url.pathname}${url.search}`);
      }
    });
    const ready = trafficAfterReady(page, todayQueryPaths);
    await ready.initialize();

    await openToday(page, now);
    await expectReadyForNavigation(page, ready);
    await expect(lessons(page).getByRole('link')).toHaveCount(3);

    const paths = requested.map((read) => new URL(read, 'https://ovc.invalid').pathname);
    expect([...new Set(paths)].sort()).toEqual(
      [
        '/api/v1/channels/unread',
        '/api/v1/home',
        '/api/v1/notifications',
        '/api/v1/review-queue',
        '/api/v1/users/me',
      ].sort(),
    );
    expect(requested.filter((read) => read === '/api/v1/home').length).toBeGreaterThanOrEqual(1);
    expect(
      Math.max(...[...new Set(requested)].map((read) => requested.filter((entry) => entry === read).length)),
    ).toBeLessThanOrEqual(2);
  });

  test('a weekly series of three lessons, whose three lesson frames commit together, costs at most two /home reads on an open Today', async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const when = laterToday(now);
    const { tutor, cardId } = await tutorWithLessonToday(deployment, now, {
      tutor: 'Tutor',
      student: 'Vera Student',
      title: 'First lesson',
    });
    await signInThroughTheApi(page, tutor);
    const ready = trafficAfterReady(page, todayQueryPaths);
    await ready.initialize();
    await openToday(page, now);
    await expectReadyForNavigation(page, ready);
    await lessons(page).getByRole('link').first().waitFor();
    let started = 0;
    let finished = 0;
    page.on('request', (request) => {
      if (new URL(request.url()).pathname === '/api/v1/home') started += 1;
    });
    page.on('requestfinished', (request) => {
      if (new URL(request.url()).pathname === '/api/v1/home') finished += 1;
    });

    await scheduleLessonsWithIds(deployment, tutor, {
      relationship_id: cardId,
      title: 'Series lesson',
      date: when.date,
      start: `${String(Number(when.start.slice(0, 2)) + 1).padStart(2, '0')}:00`,
      duration_minutes: 60,
      weekdays: [isoWeekday(when.date)],
      count: 3,
    });
    await lessons(page).getByRole('link').nth(1).waitFor();

    await expect.poll(() => started > 0 && started === finished).toBe(true);
    expect(started).toBeLessThanOrEqual(2);
  });

  test('a lesson title, group name, student name and channel label with markup show as text on Today', async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const when = laterToday(now);
    const tutor = await seedVerifiedUser(deployment, 'Tutor', 'en');
    await setTimezone(deployment, tutor, when.zone);
    const { student, cardId } = await seedActiveStudent(deployment, tutor, markup, 'en');
    const groupId = await createGroup(deployment, tutor, markup, [cardId]);
    await scheduleLessonsWithIds(deployment, tutor, {
      group_id: groupId,
      title: markup,
      date: when.date,
      start: when.start,
      duration_minutes: 60,
    });
    const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
    const sent = await callApi(
      deployment,
      await accessToken(deployment, student),
      'POST',
      `/channels/${channel}/messages`,
      {
        body: 'Hello',
        attachment_ids: [],
      },
    );
    expect(sent.status).toBe(201);
    const template = await createPastSimpleTemplate(deployment, tutor, { texts: { ...pastSimple, title: markup } });
    await issueThroughTheApi(
      deployment,
      tutor,
      template,
      { relationship_id: cardId },
      { deadlineAt: new Date(now + 2 * 24 * 60 * 60 * 1_000).toISOString() },
    );
    await answerTextAndSubmit(deployment, student, await onlySubmission(deployment, student), 'went');
    const requester = await seedAccount(deployment, markup, 'student', 'en');
    await enterTutorCode(deployment, requester, await tutorCode(deployment, tutor));
    await signInThroughTheApi(page, tutor);

    await openToday(page, now);

    await expect(lessons(page).getByRole('link')).toContainText(markup);
    await expect(page.getByRole('region', { name: 'Unread messages' }).getByRole('listitem')).toContainText(markup);
    await expect(page.getByRole('region', { name: /^Needs checking/ }).getByRole('listitem')).toContainText(markup);
    await expect(page.getByRole('region', { name: 'Upcoming deadlines' }).getByRole('listitem')).toContainText(markup);
    await expect(page.getByRole('region', { name: 'Join requests' }).getByRole('listitem')).toContainText(markup);
    await expect(page.getByRole('main').locator('img, b')).toHaveCount(0);
    await expect(page).toHaveTitle('Today · OVC');
  });

  test("an account change inside the app clears Today's lessons", async ({ page, deployment, control }) => {
    const now = await control.now();
    const { tutor } = await tutorWithLessonToday(deployment, now, {
      tutor: 'First tutor',
      student: 'Vera Student',
      title: 'First tutor lesson',
    });
    const other = await seedVerifiedUser(deployment, 'Second tutor', 'en');
    await signInThroughTheApi(page, tutor);
    await openToday(page, now);
    await expect(lessons(page).getByRole('link')).toHaveCount(1);
    await page.evaluate(() => {
      document.documentElement.dataset.sameDocument = 'yes';
    });

    await page.getByRole('button', { name: /Account menu/ }).click();
    await page.getByRole('button', { name: 'Sign out' }).click();
    await page.getByLabel('Email').fill(other.email);
    await page.getByLabel('Password', { exact: true }).fill(other.password);
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();

    await expect(page.getByRole('region', { name: 'Today’s lessons' }).getByText('No lessons today')).toBeVisible();
    await expect(page.getByRole('main')).not.toContainText('First tutor lesson');
    await expect(page.locator('html')).toHaveAttribute('data-same-document', 'yes');
  });

  test('focus moves to the unread heading when a live refetch removes the focused unread row', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Tutor', 'en');
    const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Vera Student', 'en');
    const channel = await channelOf(deployment, tutor, `/students/${cardId}`);
    const sent = await callApi(
      deployment,
      await accessToken(deployment, student),
      'POST',
      `/channels/${channel}/messages`,
      {
        body: 'Hello',
        attachment_ids: [],
      },
    );
    expect(sent.status).toBe(201);
    const seq = typeof sent.body === 'object' && sent.body !== null && 'seq' in sent.body ? sent.body.seq : null;
    if (typeof seq !== 'number') {
      throw new Error('the sent message has no sequence number');
    }
    await signInThroughTheApi(page, tutor);
    await page.goto('/today');
    const unread = page.getByRole('region', { name: 'Unread messages' });
    await unread.getByRole('listitem').getByRole('link').focus();
    await waitForSockets(deployment, await accountId(deployment, tutor), 1);

    const read = await callApi(deployment, await accessToken(deployment, tutor), 'POST', `/channels/${channel}/read`, {
      seq,
    });

    expect(read.status).toBe(200);
    await expect(unread.getByText('No unread messages')).toBeVisible();
    await expect(unread.getByRole('heading', { name: 'Unread messages' })).toBeFocused();
  });

  test('focus moves to the page heading when the last checklist step is done and the checklist disappears', async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const when = laterToday(now);
    const tutor = await seedVerifiedUser(deployment, 'Tutor', 'en');
    await setTimezone(deployment, tutor, when.zone);
    await setBio(deployment, tutor);
    await createAssignment(deployment, tutor);
    const student = await seedActiveStudent(deployment, tutor, 'Anna Student', 'en');
    await signInThroughTheApi(page, tutor);
    await openToday(page, now);
    await checklist(page).getByRole('link', { name: 'Open the schedule' }).focus();
    await waitForSockets(deployment, await accountId(deployment, tutor), 1);

    await scheduleLessonsWithIds(deployment, tutor, {
      relationship_id: student.cardId,
      title: 'First lesson',
      date: when.date,
      start: when.start,
      duration_minutes: 60,
    });

    await expect(checklist(page)).toHaveCount(0);
    await expect(page.getByRole('heading', { level: 1, name: 'Today' })).toBeFocused();
  });

  test('focus moves to the checklist heading when a live refetch marks the focused step done', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Tutor', 'en');
    await signInThroughTheApi(page, tutor);
    await page.goto('/today');
    await checklist(page).getByRole('link', { name: 'Open Students' }).focus();
    await waitForSockets(deployment, await accountId(deployment, tutor), 1);

    await createCard(deployment, tutor, 'Vera Student', null);

    await expect(checklist(page).getByRole('link', { name: 'Open Students' })).toHaveCount(0);
    await expect(checklist(page).getByRole('heading', { name: 'Getting started' })).toBeFocused();
  });
});

interface SoonLesson {
  readonly title: string;
  readonly minutesAhead: number;
}

interface TutorWithSoonLessons {
  readonly tutor: TestAccount;
  readonly zone: string;
  readonly lessonIds: readonly string[];
}

async function tutorWithSoonLessons(
  deployment: Deployment,
  now: number,
  soon: readonly SoonLesson[],
): Promise<TutorWithSoonLessons> {
  const zone = daytimeZone(now);
  const tutor = await seedVerifiedUser(deployment, 'Tutor', 'en');
  await setTimezone(deployment, tutor, zone);
  const { cardId } = await seedActiveStudent(deployment, tutor, 'Vera Student', 'en');
  const lessonIds: string[] = [];
  for (const lesson of soon) {
    const start = now + lesson.minutesAhead * 60_000;
    const [scheduled] = await scheduleLessonsWithIds(deployment, tutor, {
      relationship_id: cardId,
      title: lesson.title,
      date: localDate(start, zone),
      start: localClock(start, zone),
      duration_minutes: 30,
    });
    if (scheduled === undefined) {
      throw new Error('no lesson was scheduled');
    }
    lessonIds.push(scheduled.id);
  }
  return { tutor, zone, lessonIds };
}

function lessonRow(page: Page, title: string): ReturnType<Page['getByRole']> {
  return lessons(page).getByRole('listitem').filter({ hasText: title });
}

async function joinFromToday(page: Page, title: string, diagnostics: Diagnostics): Promise<Page> {
  const opened = page.context().waitForEvent('page');
  await lessonRow(page, title).getByRole('button', { name: 'Join lesson', exact: true }).click();
  const room = await opened;
  await diagnostics.watch(room);
  await expect(room.locator('#room')).toHaveAttribute('data-state', 'entered');
  return room;
}

test.describe('Join on Today', () => {
  test.use({ locale: 'en-GB', viewport: { width: 1440, height: 900 }, colorScheme: 'light' });

  test("the tutor's Join opens the room in one new tab without an opener and focus lands on Open again", async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const now = await control.now();
    const { tutor, lessonIds } = await tutorWithSoonLessons(deployment, now, [
      { title: 'Conversation practice', minutesAhead: 5 },
    ]);
    await signInThroughTheApi(page, tutor);
    const lessonReads: string[] = [];
    page.on('request', (request) => {
      const path = new URL(request.url()).pathname;
      if (request.method() === 'GET' && path.startsWith('/api/v1/lessons/')) {
        lessonReads.push(path);
      }
    });
    await openToday(page, now);
    const row = lessonRow(page, 'Conversation practice');
    await expect(row.getByRole('button', { name: 'Join lesson', exact: true })).toBeVisible();

    const room = await joinFromToday(page, 'Conversation practice', diagnostics);

    await expect(room.locator('#participant-name')).toHaveText('Tutor');
    await expect(room.locator('#participant-role')).toHaveText('moderator');
    expect(await room.evaluate(() => window.opener === null)).toBe(true);
    expect(page.context().pages()).toHaveLength(2);
    await expect(page).toHaveURL('/today');
    await expect(row.getByRole('button', { name: 'Open again', exact: true })).toBeFocused();
    await expect(row.getByRole('link')).toHaveAttribute('href', `/lessons/${lessonIds[0] ?? ''}`);
    await expect(row.getByRole('link').getByRole('button')).toHaveCount(0);
    expect(lessonReads).toEqual([]);
  });

  test('before its window the next lesson shows when the room opens with no button, and a cancelled lesson shows no control', async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const { tutor, zone, lessonIds } = await tutorWithSoonLessons(deployment, now, [
      { title: 'Cancelled lesson', minutesAhead: 5 },
      { title: 'Later lesson', minutesAhead: 60 },
      { title: 'Last lesson', minutesAhead: 120 },
    ]);
    await cancelThisLesson(deployment, tutor, lessonIds[0] ?? '');
    await signInThroughTheApi(page, tutor);
    const opensAt = localClock(now + 60 * 60_000 - 10 * 60_000, zone);

    await openToday(page, now);

    await expect(lessons(page).getByRole('link')).toHaveCount(3);
    await expect(lessonRow(page, 'Later lesson')).toContainText(`Room opens at ${opensAt}`);
    await expect(lessons(page).getByRole('button')).toHaveCount(0);
    await expect(lessonRow(page, 'Cancelled lesson')).not.toContainText('Room opens at');
    await expect(lessonRow(page, 'Last lesson')).not.toContainText('Room opens at');
  });
});

interface Box {
  readonly left: number;
  readonly top: number;
  readonly bottom: number;
  readonly width: number;
}

async function boxOf(locator: ReturnType<Page['getByRole']>): Promise<Box> {
  const box = await locator.boundingBox();
  if (box === null) {
    throw new Error('the element has no layout box');
  }
  return { left: box.x, top: box.y, bottom: box.y + box.height, width: box.width };
}

interface PhoneLayout {
  readonly lessonLink: Box;
  readonly join: Box;
  readonly requesterName: Box;
  readonly accept: Box;
  readonly decline: Box;
  readonly newLessonInHeader: number;
  readonly week: Box;
  readonly today: Box;
  readonly next: Box;
}

async function phoneLayoutOfTodayAndSchedule(page: Page, now: number, title: string): Promise<PhoneLayout> {
  await openToday(page, now);
  const row = lessonRow(page, title);
  const request = page.getByRole('main').locator('li[data-row]').filter({ hasText: 'Requester 01' });
  const requesterName = request.getByText('Requester 01', { exact: true });
  await requesterName.waitFor();
  const today = {
    lessonLink: await boxOf(row.getByRole('link')),
    join: await boxOf(row.getByRole('button', { name: 'Join lesson', exact: true })),
    requesterName: await boxOf(requesterName),
    accept: await boxOf(request.getByRole('button', { name: 'Accept Requester 01' })),
    decline: await boxOf(request.getByRole('button', { name: 'Decline Requester 01' })),
  };
  await page.goto('/schedule');
  const toolbar = page.getByRole('group', { name: 'Schedule view and period' });
  await toolbar.waitFor();
  return {
    ...today,
    newLessonInHeader: await page.locator('main header').getByRole('button', { name: 'New lesson' }).count(),
    week: await boxOf(toolbar.getByRole('button', { name: 'Week', exact: true })),
    today: await boxOf(toolbar.getByRole('button', { name: 'Today', exact: true })),
    next: await boxOf(toolbar.getByRole('button', { name: 'Next week', exact: true })),
  };
}

test.describe('Today and Schedule on a phone', () => {
  test.use({ locale: 'en-GB', viewport: { width: 360, height: 780 }, colorScheme: 'light' });

  test('at 360 px Join stands under its lesson’s link, Accept and Decline sit under the requester, and Schedule keeps New lesson in the header above a two-row toolbar', async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const { tutor } = await tutorWithSoonLessons(deployment, now, [
      { title: 'Conversation practice', minutesAhead: 5 },
    ]);
    await seedJoinRequests(deployment, tutor, 1);
    await signInThroughTheApi(page, tutor);

    const shown = await phoneLayoutOfTodayAndSchedule(page, now, 'Conversation practice');

    expect(shown.join.top).toBeGreaterThanOrEqual(shown.lessonLink.bottom);
    expect(shown.accept.top).toBeGreaterThanOrEqual(shown.requesterName.bottom);
    expect(Math.abs(shown.decline.top - shown.accept.top)).toBeLessThanOrEqual(1);
    expect(shown.decline.left).toBeGreaterThan(shown.accept.left);
    expect(shown.newLessonInHeader).toBe(1);
    expect(shown.today.top).toBeGreaterThanOrEqual(shown.week.bottom);
    expect(Math.abs(shown.next.top - shown.today.top)).toBeLessThanOrEqual(1);
  });
});

test.describe('Dismiss on a phone', () => {
  test.use({ locale: 'en-GB', viewport: { width: 360, height: 780 }, colorScheme: 'light' });

  test('focus moves to the heading of today’s lessons when the checklist is dismissed', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Focused tutor', 'en');
    await signInThroughTheApi(page, tutor);
    await page.goto('/today');
    const dismiss = checklist(page).getByRole('button', { name: 'Dismiss' });

    await dismiss.click();

    await expect(page.getByRole('heading', { name: 'Today’s lessons' })).toBeFocused();
    await expect(checklist(page)).toHaveCount(0);
  });
});
