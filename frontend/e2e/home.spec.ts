import type { Locator, Page, Route } from '@playwright/test';

import { type TestAccount, emailedLinkTo, newTestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { type Diagnostics, diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { accountId, expectReadyForNavigation, trafficAfterReady, waitForSockets } from './realtime.ts';
import { addDays, cancelThisLesson, localClock, localDate, scheduleLessonsWithIds } from './schedule.ts';
import {
  acceptSignedIn,
  accessToken,
  callApi,
  createCard,
  createGroup,
  seedActiveStudent,
  signUpOnInvitation,
} from './tutoring.ts';

const markup = '<img src=x onerror="document.title=1"><b>Bold</b>';
const homeQueryPaths = ['/api/v1/tutors', '/api/v1/join-requests/sent', '/api/v1/calendar'];

interface TwoTutors {
  readonly maria: TestAccount;
  readonly pavel: TestAccount;
  readonly anna: TestAccount;
  readonly mariaCard: string;
  readonly pavelCard: string;
  readonly date: string;
}

async function studentWithTwoTutors(
  deployment: Deployment,
  now: number,
  names: { readonly maria: string; readonly pavel: string } = { maria: 'Maria', pavel: 'Pavel' },
): Promise<TwoTutors> {
  const maria = await seedVerifiedUser(deployment, names.maria, 'en');
  const pavel = await seedVerifiedUser(deployment, names.pavel, 'en');
  const { student: anna, cardId: mariaCard } = await seedActiveStudent(deployment, maria, 'Anna', 'en');
  const pavelCard = await createCard(deployment, pavel, 'Anna', anna.email);
  await acceptSignedIn(deployment, anna, pavelCard.link);
  const date = addDays(localDate(now, 'Europe/Moscow'), 1);
  for (const [tutor, relationship, title, start] of [
    [maria, mariaCard, 'Maria lesson', '17:00'],
    [pavel, pavelCard.id, 'Pavel lesson', '19:00'],
  ] as const) {
    await scheduleLessonsWithIds(deployment, tutor, {
      relationship_id: relationship,
      title,
      date,
      start,
      duration_minutes: 60,
    });
  }
  return { maria, pavel, anna, mariaCard, pavelCard: pavelCard.id, date };
}

async function openHome(page: Page, now: number, address = '/home'): Promise<void> {
  await page.clock.install({ time: now });
  await page.goto(address);
}

function region(page: Page, name: string): Locator {
  return page.getByRole('region', { name, exact: true });
}

function weekRows(page: Page): Locator {
  return region(page, 'Next seven days').getByRole('list').getByRole('link');
}

test.describe('Home', () => {
  test.use({
    locale: 'en-GB',
    timezoneId: 'Europe/Moscow',
    viewport: { width: 1440, height: 900 },
    colorScheme: 'light',
  });

  test('a student with two tutors sees each lesson on Home with its tutor label', async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const { anna } = await studentWithTwoTutors(deployment, now);
    await signInThroughTheApi(page, anna);

    await openHome(page, now);

    await expect(weekRows(page)).toHaveText([/Maria lesson.*Maria/, /Pavel lesson.*Pavel/]);
    await expect(region(page, 'Next lesson').getByRole('link')).toContainText('Maria lesson');
    await expect(region(page, 'Next lesson').getByRole('link')).toContainText('Maria');
    await expect(weekRows(page).first()).toHaveAttribute('href', /^\/lessons\//);
    for (const row of await weekRows(page).all()) {
      await expect(row.locator('span[aria-hidden="true"].rounded-full')).toHaveCount(1);
    }
  });

  test("filtering by one tutor shows only that tutor's lessons and keeps My tutors whole", async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const { anna, pavelCard } = await studentWithTwoTutors(deployment, now);
    await signInThroughTheApi(page, anna);
    await openHome(page, now);
    await expect(weekRows(page)).toHaveCount(2);
    const filter = page.getByRole('group', { name: 'Tutor' });

    await filter.getByRole('button', { name: 'Pavel' }).click();

    await expect(page).toHaveURL(`/home?tutor=${pavelCard}`);
    await expect(filter.getByRole('button', { name: 'Pavel' })).toHaveAttribute('aria-pressed', 'true');
    await expect(weekRows(page)).toHaveText([/Pavel lesson/]);
    await expect(region(page, 'My tutors').getByRole('listitem')).toHaveCount(2);
  });

  test('Back after filtering restores all tutors', async ({ page, deployment, control }) => {
    const now = await control.now();
    const { anna, pavelCard } = await studentWithTwoTutors(deployment, now);
    await signInThroughTheApi(page, anna);
    await openHome(page, now);
    await expect(weekRows(page)).toHaveCount(2);
    await page.getByRole('group', { name: 'Tutor' }).getByRole('button', { name: 'Pavel' }).click();
    await expect(page).toHaveURL(`/home?tutor=${pavelCard}`);

    await page.goBack();

    await expect(page).toHaveURL('/home');
    await expect(weekRows(page)).toHaveCount(2);
    await expect(
      page.getByRole('group', { name: 'Tutor' }).getByRole('button', { name: 'All tutors' }),
    ).toHaveAttribute('aria-pressed', 'true');
  });

  test('a tutor value no current tutor holds shows that the tutor is not available and never reaches the calendar', async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const { anna } = await studentWithTwoTutors(deployment, now);
    const other = await seedVerifiedUser(deployment, 'Olga', 'en');
    const { cardId: otherCard } = await seedActiveStudent(deployment, other, 'Boris', 'en');
    await signInThroughTheApi(page, anna);
    const values = [otherCard, 'not-an-identifier'];
    const calendarQueries: string[] = [];
    page.on('request', (request) => {
      const url = new URL(request.url());
      if (url.pathname === '/api/v1/calendar') {
        calendarQueries.push(url.search);
      }
    });

    const shown: string[] = [];
    for (const value of values) {
      await openHome(page, now, `/home?${new URLSearchParams({ tutor: value }).toString()}`);
      await expect(region(page, 'My tutors').getByRole('listitem')).toHaveCount(2);
      shown.push((await page.getByRole('main').textContent()) ?? '');
    }

    for (const [index, value] of values.entries()) {
      expect(shown[index]).toContain('This tutor is not available.');
      expect(shown[index]).not.toContain(value);
      expect(shown[index]).not.toContain('Maria lesson');
    }
    expect(
      calendarQueries.filter((query) => values.some((value) => query.includes(encodeURIComponent(value)))),
    ).toEqual([]);
    await expect(page.getByRole('main').getByRole('link', { name: 'All tutors' })).toHaveAttribute('href', '/home');
  });

  test('a cancelled lesson is left out of the next seven days and the next lesson', async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const { anna, maria, mariaCard, date } = await studentWithTwoTutors(deployment, now);
    const [cancelled] = await scheduleLessonsWithIds(deployment, maria, {
      relationship_id: mariaCard,
      title: 'Cancelled lesson',
      date,
      start: '09:00',
      duration_minutes: 60,
    });
    if (cancelled === undefined) {
      throw new Error('no lesson was created');
    }
    await cancelThisLesson(deployment, maria, cancelled.id);
    await signInThroughTheApi(page, anna);

    await openHome(page, now);

    await expect(weekRows(page)).toHaveCount(2);
    await expect(page.getByRole('main')).not.toContainText('Cancelled lesson');
    await expect(region(page, 'Next lesson').getByRole('link')).toContainText('Maria lesson');
  });

  test("an unconfirmed student's Home shows the provisional link's lesson", async ({ page, deployment, control }) => {
    const now = await control.now();
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris', 'student');
    const card = await createCard(deployment, pavel, 'Boris', boris.email);
    await signUpOnInvitation(deployment, card.link, boris, 'en');
    await scheduleLessonsWithIds(deployment, pavel, {
      relationship_id: card.id,
      title: 'Provisional lesson',
      date: addDays(localDate(now, 'Europe/Moscow'), 1),
      start: '10:00',
      duration_minutes: 60,
    });
    await signInThroughTheApi(page, boris);

    await openHome(page, now);

    await expect(region(page, 'Next lesson').getByRole('link')).toContainText('Provisional lesson');
    await expect(region(page, 'Next lesson').getByRole('link')).toContainText('Pavel');
    await expect(page.getByRole('group', { name: 'Tutor' })).toHaveCount(0);
  });

  test("a reclaim_pending link's lessons are not shown on Home", async ({ page, deployment, control, diagnostics }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
    const now = await control.now();
    const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
    const owner = newTestAccount('Owner', 'student');
    const card = await createCard(deployment, maria, 'Anna', owner.email);
    await signUpOnInvitation(deployment, card.link, owner, 'en');
    await scheduleLessonsWithIds(deployment, maria, {
      relationship_id: card.id,
      title: 'Squatted lesson',
      date: addDays(localDate(now, 'Europe/Moscow'), 1),
      start: '10:00',
      duration_minutes: 60,
    });
    await control.tick();
    await page.goto(emailedLinkTo(await control.mail(owner.email), '/verify-email'));
    await page.getByLabel('New password').fill(`${owner.password} reclaimed`);
    const reclaimRead = page.waitForResponse(
      (response) => new URL(response.url()).pathname === '/api/v1/users/me/reclaim',
    );
    await page.getByRole('button', { name: 'Confirm and sign in' }).click();
    await expect(page).toHaveURL('/reclaim');
    await reclaimRead;

    await openHome(page, now);
    await expect(region(page, 'Next lesson').getByText('No lesson scheduled')).toBeVisible();
    const hiddenLesson = await page.getByRole('main').textContent();
    await openHome(page, now, `/home?tutor=${card.id}`);

    expect(hiddenLesson).not.toContain('Squatted lesson');
    await expect(page.getByRole('main')).toContainText('This tutor is not available.');
    await expect(page.getByRole('main')).not.toContainText('Squatted lesson');
  });

  test("a removed group member's open Home drops the group lesson without reloading", async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
    const anna = await seedActiveStudent(deployment, maria, 'Anna', 'en');
    const groupId = await createGroup(deployment, maria, 'Evening group', [anna.cardId]);
    await scheduleLessonsWithIds(deployment, maria, {
      group_id: groupId,
      title: 'Group lesson',
      date: addDays(localDate(now, 'Europe/Moscow'), 1),
      start: '18:00',
      duration_minutes: 60,
    });
    await signInThroughTheApi(page, anna.student);
    await openHome(page, now);
    await expect(weekRows(page)).toHaveText([/Group lesson/]);
    await waitForSockets(deployment, await accountId(deployment, anna.student), 1);
    await page.evaluate(() => {
      document.documentElement.dataset.sameDocument = 'yes';
    });
    await weekRows(page).first().focus();

    const removed = await callApi(deployment, await accessToken(deployment, maria), 'PATCH', `/groups/${groupId}`, {
      remove: [anna.cardId],
    });

    expect(removed.status).toBe(200);
    await expect(region(page, 'Next seven days').getByText('No lessons in the next seven days')).toBeVisible();
    await expect(page.locator('html')).toHaveAttribute('data-same-document', 'yes');
    await expect(page.getByRole('heading', { level: 1, name: 'Home' })).toBeFocused();
  });

  test('focus moves to the lesson that takes the place of a cancelled focused next lesson', async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const { anna, maria, mariaCard, date } = await studentWithTwoTutors(deployment, now);
    const [first] = await scheduleLessonsWithIds(deployment, maria, {
      relationship_id: mariaCard,
      title: 'Earliest lesson',
      date,
      start: '09:00',
      duration_minutes: 60,
    });
    if (first === undefined) {
      throw new Error('no lesson was created');
    }
    await signInThroughTheApi(page, anna);
    await openHome(page, now);
    const next = region(page, 'Next lesson').getByRole('link');
    await expect(next).toContainText('Earliest lesson');
    await waitForSockets(deployment, await accountId(deployment, anna), 1);
    await next.focus();

    await cancelThisLesson(deployment, maria, first.id);

    await expect(next).toContainText('Maria lesson');
    await expect(next).toBeFocused();
  });

  test("an ended relationship's group lesson leaves an open Home", async ({ page, deployment, control }) => {
    const now = await control.now();
    const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
    const anna = await seedActiveStudent(deployment, maria, 'Anna', 'en');
    const groupId = await createGroup(deployment, maria, 'Evening group', [anna.cardId]);
    await scheduleLessonsWithIds(deployment, maria, {
      group_id: groupId,
      title: 'Group lesson',
      date: addDays(localDate(now, 'Europe/Moscow'), 1),
      start: '18:00',
      duration_minutes: 60,
    });
    await signInThroughTheApi(page, anna.student);
    await openHome(page, now);
    await expect(weekRows(page)).toHaveText([/Group lesson/]);
    await waitForSockets(deployment, await accountId(deployment, anna.student), 1);
    await page.evaluate(() => {
      document.documentElement.dataset.sameDocument = 'yes';
    });

    const ended = await callApi(
      deployment,
      await accessToken(deployment, maria),
      'POST',
      `/students/${anna.cardId}/end`,
      {},
    );

    expect(ended.status).toBe(200);
    await expect(page.getByRole('main')).not.toContainText('Group lesson');
    await expect(page.locator('html')).toHaveAttribute('data-same-document', 'yes');
  });

  test('an account change inside the app clears Home', async ({ page, deployment, control }) => {
    const now = await control.now();
    const { anna } = await studentWithTwoTutors(deployment, now);
    const other = await seedVerifiedUser(deployment, 'Olga', 'en');
    const { student: boris } = await seedActiveStudent(deployment, other, 'Boris', 'en');
    await signInThroughTheApi(page, anna);
    await openHome(page, now);
    await expect(weekRows(page)).toHaveCount(2);
    await page.getByRole('group', { name: 'Tutor' }).getByRole('button', { name: 'Pavel' }).click();
    await expect(weekRows(page)).toHaveText([/Pavel lesson/]);
    await page.evaluate(() => {
      document.documentElement.dataset.sameDocument = 'yes';
      const watcher = new MutationObserver(() => {
        const signedOutOnce = document.querySelector('main h1')?.textContent === 'Sign in';
        if (signedOutOnce) {
          document.documentElement.dataset.signedOut = 'yes';
        }
        const text = document.querySelector('main')?.textContent ?? '';
        if (document.documentElement.dataset.signedOut === 'yes' && /Maria lesson|Pavel lesson/.test(text)) {
          document.documentElement.dataset.leaked = 'yes';
        }
      });
      watcher.observe(document.body, { subtree: true, childList: true, characterData: true });
    });

    await page.getByRole('button', { name: /Account menu/ }).click();
    await page.getByRole('button', { name: 'Sign out' }).click();
    await page.getByLabel('Email').fill(boris.email);
    await page.getByLabel('Password', { exact: true }).fill(boris.password);
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();

    await expect(region(page, 'My tutors').getByRole('listitem')).toHaveText([/Olga/]);
    await expect(region(page, 'Next seven days').getByText('No lessons in the next seven days')).toBeVisible();
    await expect(page.getByRole('main')).not.toContainText('Maria lesson');
    await expect(page.getByRole('main')).not.toContainText('Pavel lesson');
    await expect(page.locator('html')).toHaveAttribute('data-same-document', 'yes');
    await expect(page.locator('html')).toHaveAttribute('data-signed-out', 'yes');
    await expect(page.locator('html')).not.toHaveAttribute('data-leaked', 'yes');
  });

  test('focus moves to the page heading when the filter disappears with focus on it', async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const { anna, pavel, pavelCard } = await studentWithTwoTutors(deployment, now);
    await signInThroughTheApi(page, anna);
    await openHome(page, now);
    const filter = page.getByRole('group', { name: 'Tutor' });
    await expect(weekRows(page)).toHaveCount(2);
    await waitForSockets(deployment, await accountId(deployment, anna), 1);
    await filter.getByRole('button', { name: 'Pavel' }).focus();

    const ended = await callApi(
      deployment,
      await accessToken(deployment, pavel),
      'POST',
      `/students/${pavelCard}/end`,
      {},
    );

    expect(ended.status).toBe(200);
    await expect(filter).toHaveCount(0);
    await expect(page.getByRole('heading', { level: 1, name: 'Home' })).toBeFocused();
  });

  test('Home makes only its listed requests', async ({ page, deployment, control }) => {
    const now = await control.now();
    const { anna } = await studentWithTwoTutors(deployment, now);
    await signInThroughTheApi(page, anna);
    const requested: string[] = [];
    const queries: string[] = [];
    page.on('request', (request) => {
      const url = new URL(request.url());
      if (
        url.pathname.startsWith('/api/v1/') &&
        url.pathname !== '/api/v1/ws' &&
        !url.pathname.startsWith('/api/v1/auth/')
      ) {
        requested.push(url.pathname);
        queries.push(url.pathname === '/api/v1/assignments' ? `${url.pathname}${url.search}` : url.pathname);
      }
    });
    const ready = trafficAfterReady(page, homeQueryPaths);
    await ready.initialize();

    await openHome(page, now);
    await expectReadyForNavigation(page, ready);
    await expect(weekRows(page)).toHaveCount(2);

    const counts = Object.fromEntries(
      [...new Set(queries)].map((query) => [query, queries.filter((entry) => entry === query).length]),
    );
    expect([...new Set(requested)].sort()).toEqual(
      [
        '/api/v1/assignments',
        '/api/v1/calendar',
        '/api/v1/channels/unread',
        '/api/v1/join-requests/sent',
        '/api/v1/notifications',
        '/api/v1/tutors',
        '/api/v1/users/me',
      ].sort(),
    );
    expect(Math.max(...Object.values(counts))).toBeLessThanOrEqual(2);
  });

  test('a lesson title, group name and tutor name with markup show as text on Home', async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const { anna, maria, mariaCard, date } = await studentWithTwoTutors(deployment, now, {
      maria: markup,
      pavel: 'Pavel',
    });
    const groupId = await createGroup(deployment, maria, markup, [mariaCard]);
    await scheduleLessonsWithIds(deployment, maria, {
      group_id: groupId,
      title: markup,
      date,
      start: '12:00',
      duration_minutes: 60,
    });
    await signInThroughTheApi(page, anna);

    await openHome(page, now);

    await expect(weekRows(page).first()).toContainText(markup);
    await expect(page.getByRole('group', { name: 'Tutor' }).getByRole('button', { name: markup })).toBeVisible();
    await expect(page.getByRole('main').locator('img, b')).toHaveCount(0);
    await expect(page).toHaveTitle('Home · OVC');
  });
});

const moscow = 'Europe/Moscow';
const waitingText = 'The room opens when your tutor joins';

type LessonAudience = { readonly relationship_id: string } | { readonly group_id: string };

async function lessonSoon(
  deployment: Deployment,
  tutor: TestAccount,
  audience: LessonAudience,
  title: string,
  start: number,
): Promise<string> {
  const [lesson] = await scheduleLessonsWithIds(deployment, tutor, {
    ...audience,
    title,
    date: localDate(start, moscow),
    start: localClock(start, moscow),
    duration_minutes: 60,
  });
  if (lesson === undefined) {
    throw new Error('no lesson was scheduled');
  }
  return lesson.id;
}

async function tutorJoins(deployment: Deployment, tutor: TestAccount, lessonId: string): Promise<void> {
  const joined = await callApi(deployment, await accessToken(deployment, tutor), 'POST', `/lessons/${lessonId}/join`);
  expect(joined.status).toBe(200);
}

function nextLessonRegion(page: Page): Locator {
  return region(page, 'Next lesson');
}

function homeJoinButton(page: Page, name: string): Locator {
  return nextLessonRegion(page).getByRole('button', { name, exact: true });
}

async function joinFromHome(page: Page, name: string, diagnostics: Diagnostics): Promise<Page> {
  const opened = page.context().waitForEvent('page');
  await homeJoinButton(page, name).click();
  const room = await opened;
  await diagnostics.watch(room);
  await expect(room.locator('#room')).toHaveAttribute('data-state', 'entered');
  return room;
}

function recordLessonReads(page: Page): string[] {
  const reads: string[] = [];
  page.on('request', (request) => {
    const path = new URL(request.url()).pathname;
    if (request.method() === 'GET' && path.startsWith('/api/v1/lessons/')) {
      reads.push(path);
    }
  });
  return reads;
}

async function holdJoin(page: Page): Promise<{ readonly held: Promise<Route>; readonly release: () => Promise<void> }> {
  let received: (route: Route) => void = () => undefined;
  const held = new Promise<Route>((resolve) => {
    received = resolve;
  });
  await page.route('**/api/v1/lessons/*/join', (route) => {
    received(route);
  });
  return {
    held,
    release: async (): Promise<void> => {
      await (await held).continue();
      await page.unroute('**/api/v1/lessons/*/join');
    },
  };
}

test.describe('Join on Home', () => {
  test.use({
    locale: 'en-GB',
    timezoneId: moscow,
    viewport: { width: 1440, height: 900 },
    colorScheme: 'light',
  });

  test('the student waits in the window, Join appears without a reload once the tutor joins, and the room opens as viewer with her name', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const now = await control.now();
    const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
    const { student: anna, cardId } = await seedActiveStudent(deployment, maria, 'Anna', 'en');
    const lessonId = await lessonSoon(deployment, maria, { relationship_id: cardId }, 'Soon lesson', now + 5 * 60_000);
    await signInThroughTheApi(page, anna);
    const lessonReads = recordLessonReads(page);
    const consoleTexts: string[] = [];
    page.on('console', (message) => {
      consoleTexts.push(message.text());
    });
    const requests: string[] = [];
    page.on('request', (request) => {
      requests.push(`${request.url()} ${request.postData() ?? ''}`);
    });
    await openHome(page, now);
    await expect(nextLessonRegion(page).getByText(waitingText)).toBeVisible();
    await expect(nextLessonRegion(page).getByRole('button')).toHaveCount(0);
    await expect.poll(() => lessonReads.length).toBeGreaterThan(0);
    await waitForSockets(deployment, await accountId(deployment, anna), 1);
    await page.evaluate(() => {
      document.documentElement.dataset.sameDocument = 'yes';
    });

    await tutorJoins(deployment, maria, lessonId);
    const room = await joinFromHome(page, 'Join', diagnostics);

    await expect(room.locator('#participant-name')).toHaveText('Anna');
    await expect(room.locator('#participant-role')).toHaveText('viewer');
    await expect(room.locator('body')).not.toContainText('Maria');
    expect(await room.evaluate(() => window.opener === null)).toBe(true);
    await expect(page).toHaveURL('/home');
    await expect(page.locator('html')).toHaveAttribute('data-same-document', 'yes');
    await expect(homeJoinButton(page, 'Open again')).toBeFocused();
    expect(new Set(lessonReads)).toEqual(new Set([`/api/v1/lessons/${lessonId}`]));
    const capability = new URL(room.url()).hash.slice(1);
    const stored = await page.evaluate(() =>
      JSON.stringify([Object.entries(window.localStorage), Object.entries(window.sessionStorage), history.state]),
    );
    expect(capability).not.toBe('');
    expect(stored).not.toContain(capability);
    expect(consoleTexts.join('\n')).not.toContain(capability);
    expect(requests.join('\n')).not.toContain(capability);
    expect(page.url()).not.toContain(capability);
  });

  test("Home reads only the next lesson's detail while two lessons are in their window", async ({
    page,
    deployment,
    control,
  }) => {
    const now = await control.now();
    const { anna, maria, pavel, mariaCard, pavelCard } = await studentWithTwoTutors(deployment, now);
    const nextId = await lessonSoon(deployment, maria, { relationship_id: mariaCard }, 'First soon', now + 4 * 60_000);
    await lessonSoon(deployment, pavel, { relationship_id: pavelCard }, 'Second soon', now + 8 * 60_000);
    await signInThroughTheApi(page, anna);
    const lessonReads = recordLessonReads(page);

    await openHome(page, now);
    await expect(weekRows(page)).toHaveCount(4);
    await expect(nextLessonRegion(page).getByText(waitingText)).toBeVisible();
    await expect.poll(() => lessonReads.length).toBeGreaterThan(0);

    expect(new Set(lessonReads)).toEqual(new Set([`/api/v1/lessons/${nextId}`]));
    await expect(region(page, 'Next seven days').getByRole('button')).toHaveCount(0);
  });

  test('an unconfirmed student with a provisional link joins a live lesson from Home', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const now = await control.now();
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris', 'student');
    const card = await createCard(deployment, pavel, 'Boris', boris.email);
    await signUpOnInvitation(deployment, card.link, boris, 'en');
    const lessonId = await lessonSoon(
      deployment,
      pavel,
      { relationship_id: card.id },
      'Provisional lesson',
      now + 5 * 60_000,
    );
    await tutorJoins(deployment, pavel, lessonId);
    await signInThroughTheApi(page, boris);
    await openHome(page, now);

    const room = await joinFromHome(page, 'Join', diagnostics);

    await expect(room.locator('#participant-name')).toHaveText('Boris');
    await expect(room.locator('#participant-role')).toHaveText('viewer');
  });

  test("a reclaim_pending account's Home shows no lesson and no Join for the live lesson", async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/auth/verify-email', status: 422 });
    const now = await control.now();
    const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
    const owner = newTestAccount('Owner', 'student');
    const card = await createCard(deployment, maria, 'Anna', owner.email);
    await signUpOnInvitation(deployment, card.link, owner, 'en');
    const lessonId = await lessonSoon(
      deployment,
      maria,
      { relationship_id: card.id },
      'Squatted lesson',
      now + 5 * 60_000,
    );
    await tutorJoins(deployment, maria, lessonId);
    await control.tick();
    await page.goto(emailedLinkTo(await control.mail(owner.email), '/verify-email'));
    await page.getByLabel('New password').fill(`${owner.password} reclaimed`);
    const reclaimRead = page.waitForResponse(
      (response) => new URL(response.url()).pathname === '/api/v1/users/me/reclaim',
    );
    await page.getByRole('button', { name: 'Confirm and sign in' }).click();
    await expect(page).toHaveURL('/reclaim');
    await reclaimRead;

    await openHome(page, now);

    await expect(nextLessonRegion(page).getByText('No lesson scheduled')).toBeVisible();
    await expect(page.getByRole('main')).not.toContainText('Squatted lesson');
    await expect(page.getByRole('main').getByRole('button', { name: 'Join', exact: true })).toHaveCount(0);
  });

  test('after an ending the live lesson seen only through the roster refuses Join, closes the tab and keeps no button', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const now = await control.now();
    const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
    const { student: anna, cardId } = await seedActiveStudent(deployment, maria, 'Anna', 'en');
    const lessonId = await lessonSoon(deployment, maria, { relationship_id: cardId }, 'Live lesson', now + 5 * 60_000);
    await lessonSoon(deployment, maria, { relationship_id: cardId }, 'Future lesson', now + 26 * 60 * 60_000);
    await tutorJoins(deployment, maria, lessonId);
    await signInThroughTheApi(page, anna);
    await openHome(page, now);
    await expect(homeJoinButton(page, 'Join')).toBeVisible();
    await expect(weekRows(page)).toHaveCount(2);
    await waitForSockets(deployment, await accountId(deployment, anna), 1);
    const ended = await callApi(
      deployment,
      await accessToken(deployment, maria),
      'POST',
      `/students/${cardId}/end`,
      {},
    );
    expect(ended.status).toBe(200);
    await expect(page.getByRole('main')).not.toContainText('Future lesson');
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/lessons/${lessonId}/join`, status: 404 });
    const opened = page.context().waitForEvent('page');

    await homeJoinButton(page, 'Join').click();
    const refusedTab = await opened;
    await diagnostics.watch(refusedTab);

    await expect(nextLessonRegion(page).getByRole('alert')).toHaveText("You can't join this lesson");
    await expect.poll(() => refusedTab.isClosed()).toBe(true);
    await expect(nextLessonRegion(page).getByRole('button')).toHaveCount(0);
    await expect(nextLessonRegion(page).getByRole('link')).toContainText('Live lesson');
  });

  test('after a group removal the live lesson seen only through the roster refuses Open again and closes the tab', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const now = await control.now();
    const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
    const anna = await seedActiveStudent(deployment, maria, 'Anna', 'en');
    const groupId = await createGroup(deployment, maria, 'Evening group', [anna.cardId]);
    const lessonId = await lessonSoon(deployment, maria, { group_id: groupId }, 'Group lesson', now + 5 * 60_000);
    await tutorJoins(deployment, maria, lessonId);
    await signInThroughTheApi(page, anna.student);
    await openHome(page, now);
    const firstRoom = await joinFromHome(page, 'Join', diagnostics);
    const removed = await callApi(deployment, await accessToken(deployment, maria), 'PATCH', `/groups/${groupId}`, {
      remove: [anna.cardId],
    });
    expect(removed.status).toBe(200);
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/lessons/${lessonId}/join`, status: 404 });
    const opened = page.context().waitForEvent('page');

    await homeJoinButton(page, 'Open again').click();
    const refusedTab = await opened;
    await diagnostics.watch(refusedTab);

    await expect(nextLessonRegion(page).getByRole('alert')).toHaveText("You can't join this lesson");
    await expect.poll(() => refusedTab.isClosed()).toBe(true);
    await expect(nextLessonRegion(page).getByRole('button')).toHaveCount(0);
    await expect(firstRoom.locator('#room')).toHaveAttribute('data-state', 'entered');
  });

  test('an account change during a held join closes the blank tab and the next account sees nothing of the previous one', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const now = await control.now();
    const maria = await seedVerifiedUser(deployment, 'Maria', 'en');
    const { student: anna, cardId } = await seedActiveStudent(deployment, maria, 'Anna', 'en');
    const olga = await seedVerifiedUser(deployment, 'Olga', 'en');
    const { student: boris } = await seedActiveStudent(deployment, olga, 'Boris', 'en');
    const lessonId = await lessonSoon(deployment, maria, { relationship_id: cardId }, 'Held lesson', now + 5 * 60_000);
    await tutorJoins(deployment, maria, lessonId);
    await signInThroughTheApi(page, anna);
    await openHome(page, now);
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/lessons/${lessonId}/join`, status: 401 });
    const write = await holdJoin(page);
    const opened = page.context().waitForEvent('page');
    await homeJoinButton(page, 'Join').click();
    const blank = await opened;
    await diagnostics.watch(blank);
    await write.held;
    const closed = blank.waitForEvent('close');
    const refused = page.waitForResponse((response) => response.url().endsWith('/join'));

    await page.getByRole('button', { name: /Account menu/ }).click();
    await page.getByRole('button', { name: 'Sign out' }).click();
    await closed;
    await write.release();
    await refused;
    await diagnostics.finishHttpPhase();
    await page.getByLabel('Email').fill(boris.email);
    await page.getByLabel('Password', { exact: true }).fill(boris.password);
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();

    await expect(region(page, 'My tutors').getByRole('listitem')).toHaveText([/Olga/]);
    expect(page.context().pages()).toEqual([page]);
    await expect(page.getByRole('main')).not.toContainText('Held lesson');
    await expect(page.getByRole('main').getByRole('button', { name: 'Join', exact: true })).toHaveCount(0);
  });
});
