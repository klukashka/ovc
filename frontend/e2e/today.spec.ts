import type { Page } from '@playwright/test';

import { type TestAccount, newTestAccount, registerTutor, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { type Diagnostics, diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { accountId, expectReadyForNavigation, todayQueryPaths, trafficAfterReady, waitForSockets } from './realtime.ts';
import { channelOf } from './messages.ts';
import { addDays, cancelThisLesson, localClock, localDate, scheduleLessonsWithIds } from './schedule.ts';
import { acceptSignedIn, accessToken, callApi, createCard, createGroup, seedActiveStudent } from './tutoring.ts';

const markup = '<img src=x onerror="document.title=1"><b>Bold</b>';
const daytimeZones = [
  'Pacific/Kiritimati',
  'Pacific/Auckland',
  'Asia/Tokyo',
  'Asia/Dubai',
  'Europe/Moscow',
  'Europe/London',
  'America/Sao_Paulo',
  'America/New_York',
  'America/Denver',
  'Pacific/Honolulu',
] as const;

interface LocalTime {
  readonly zone: string;
  readonly date: string;
  readonly start: string;
}

function hourIn(instant: number, zone: string): number {
  return Number(
    new Intl.DateTimeFormat('en-GB', { timeZone: zone, hour: '2-digit', hourCycle: 'h23' }).format(instant),
  );
}

function daytimeZone(now: number): string {
  const zone = daytimeZones.find((candidate) => {
    const hour = hourIn(now, candidate);
    return hour >= 5 && hour <= 17;
  });
  if (zone === undefined) {
    throw new Error('no daytime zone in the list');
  }
  return zone;
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

  test('a new tutor sees the three undone checklist steps', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'New tutor', 'en');
    await signInThroughTheApi(page, tutor);

    await page.goto('/today');

    const steps = checklist(page).getByRole('listitem');
    await expect(steps).toHaveCount(3);
    await expect(steps).toHaveText([
      /Fill in your profile.*not done/,
      /Invite a student.*not done/,
      /Schedule a lesson.*not done/,
    ]);
    await expect(checklist(page).getByRole('link', { name: 'Open your profile' })).toHaveAttribute(
      'href',
      '/settings/profile',
    );
  });

  test('a tutor with a bio, a student and a lesson sees no checklist', async ({ page, deployment, control }) => {
    const now = await control.now();
    const { tutor } = await tutorWithLessonToday(deployment, now, {
      tutor: 'Busy tutor',
      student: 'Vera Student',
      title: 'Lesson',
    });
    await setBio(deployment, tutor);
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
    const closedRequests: string[] = [];
    page.on('request', (request) => {
      const path = new URL(request.url()).pathname;
      if (path === '/api/v1/students' || path === '/api/v1/join-requests') {
        closedRequests.push(path);
      }
    });

    await page.goto('/today');

    await expect(checklist(page).getByText('Confirm your email to invite students.')).toBeVisible();
    await expect(checklist(page).getByText('Confirm your email to schedule lessons.')).toBeVisible();
    await expect(page.getByRole('region', { name: 'Unread messages' }).getByText('No unread messages')).toBeVisible();
    expect(closedRequests).toEqual([]);
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
    const studentQueries: string[] = [];
    page.on('request', (request) => {
      const url = new URL(request.url());
      if (
        url.pathname.startsWith('/api/v1/') &&
        url.pathname !== '/api/v1/ws' &&
        !url.pathname.startsWith('/api/v1/auth/')
      ) {
        requested.push(url.pathname);
      }
      if (url.pathname === '/api/v1/students') {
        studentQueries.push(url.search);
      }
    });
    const ready = trafficAfterReady(page, todayQueryPaths);
    await ready.initialize();

    await openToday(page, now);
    await expectReadyForNavigation(page, ready);
    await expect(lessons(page).getByRole('link')).toHaveCount(3);

    const counts = Object.fromEntries(
      [...new Set(requested)].map((path) => [path, requested.filter((entry) => entry === path).length]),
    );
    expect(Object.keys(counts).sort()).toEqual(
      [
        '/api/v1/calendar',
        '/api/v1/channels',
        '/api/v1/channels/unread',
        '/api/v1/join-requests',
        '/api/v1/notifications',
        '/api/v1/students',
        '/api/v1/tutor/code',
        '/api/v1/users/me',
      ].sort(),
    );
    expect(Math.max(...Object.values(counts))).toBeLessThanOrEqual(2);
    expect(new Set(studentQueries)).toEqual(new Set(['?limit=1']));
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
    await signInThroughTheApi(page, tutor);

    await openToday(page, now);

    await expect(lessons(page).getByRole('link')).toContainText(markup);
    await expect(page.getByRole('region', { name: 'Unread messages' }).getByRole('listitem')).toContainText(markup);
    await expect(page.getByRole('main').locator('img, b')).toHaveCount(0);
    await expect(page).toHaveTitle('Today · OVC');
  });

  for (const viewer of ['Maria', 'Pavel'] as const) {
    test(`one tutor's Today never shows another tutor's lessons, for a student they share, seen by ${viewer}`, async ({
      page,
      deployment,
      control,
    }) => {
      const now = await control.now();
      const when = laterToday(now);
      const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
      const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
      await setTimezone(deployment, maria, when.zone);
      await setTimezone(deployment, pavel, when.zone);
      const anna = await seedActiveStudent(deployment, maria, 'Anna', 'en');
      const pavelCard = await createCard(deployment, pavel, 'Anna', anna.student.email);
      await acceptSignedIn(deployment, anna.student, pavelCard.link);
      for (const [tutor, relationship, title] of [
        [maria, anna.cardId, 'Maria lesson'],
        [pavel, pavelCard.id, 'Pavel lesson'],
      ] as const) {
        await scheduleLessonsWithIds(deployment, tutor, {
          relationship_id: relationship,
          title,
          date: when.date,
          start: when.start,
          duration_minutes: 60,
        });
      }
      await signInThroughTheApi(page, viewer === 'Maria' ? maria : pavel);

      await openToday(page, now);

      await expect(lessons(page).getByRole('link')).toHaveText([new RegExp(`${viewer} lesson`)]);
      await expect(page.getByRole('main')).not.toContainText(viewer === 'Maria' ? 'Pavel lesson' : 'Maria lesson');
    });
  }

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
