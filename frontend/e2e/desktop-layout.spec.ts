import type { Locator, Page } from '@playwright/test';

import { type TestAccount, pageLoaded, signInThroughTheApi } from './accounts.ts';
import { createPastSimpleTemplate, pastSimple, questionIds, studentWork } from './assignment.ts';
import type { Deployment } from './deployment.ts';
import { type TestControl, diagnosedTest as test, expect, seedVerifiedUser, tableRows } from './fixtures.ts';
import { issueThroughTheApi } from './homework.ts';
import { channelOf } from './messages.ts';
import { publishReview, saveReviewDraft, scoreFormsIn } from './results.ts';
import { answerTextAndSubmit } from './review.ts';
import { daytimeZone, localClock, localDate, scheduleLessonsWithIds, setTimezone } from './schedule.ts';
import { accessToken, acceptSignedIn, callApi, createCard, createGroup, seedActiveStudent } from './tutoring.ts';

const computer = { width: 1440, height: 900 } as const;
const smallComputer = { width: 1024, height: 768 } as const;
const phone = { width: 360, height: 780 } as const;
const markup = '<img src=x onerror="document.title=1"><b>Bold</b>';

interface Box {
  readonly top: number;
  readonly bottom: number;
  readonly centre: number;
}

async function boxOf(locator: Locator): Promise<Box> {
  const box = await locator.boundingBox();
  if (box === null) {
    throw new Error('the element has no layout box');
  }
  return { top: box.y, bottom: box.y + box.height, centre: box.y + box.height / 2 };
}

async function scrollsSideways(page: Page): Promise<boolean> {
  return page.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth);
}

const longWord = 'Длиннаягруппаизшестидесятибуквбезединогопробеладлятесташирин';

async function tutorWithStudentInGroup(
  deployment: Deployment,
  name = 'Vera Student',
  tutorName = 'Tutor',
): Promise<{ readonly tutor: TestAccount; readonly student: TestAccount }> {
  const tutor = await seedVerifiedUser(deployment, tutorName, 'en');
  const { student, cardId } = await seedActiveStudent(deployment, tutor, name, 'en');
  await createGroup(deployment, tutor, name === 'Vera Student' ? 'Evening group' : name, [cardId]);
  return { tutor, student };
}

const tutorPages = [
  { path: '/students', table: 'Active' },
  { path: '/groups', table: 'Groups' },
  { path: '/notifications', table: 'Notifications' },
] as const;

async function firstRowOf(page: Page, table: string): Promise<{ readonly text: string; readonly injected: number }> {
  const row = tableRows(page, table).first();
  await row.waitFor();
  return { text: (await row.textContent()) ?? '', injected: await row.locator('img, b').count() };
}

async function openAt(
  page: Page,
  size: { readonly width: number; readonly height: number },
  path: string,
  content: 'table' | 'heading' = 'table',
): Promise<void> {
  await page.setViewportSize(size);
  await page.goto(path);
  await page
    .locator(content === 'table' ? 'main table > tbody > tr' : 'main article h1')
    .first()
    .waitFor();
  await pageLoaded(page);
  await page.evaluate(async () => {
    await document.fonts.ready;
  });
}

const settingsPaths = ['/settings/profile', '/settings/account', '/settings/notifications', '/settings/teaching'];

function settingsReady(page: Page): Locator {
  return page.locator('main [data-settings-value], main form, main [role="switch"]').first();
}

function firstTableRow(page: Page): Locator {
  return page.locator('main table > tbody > tr').first();
}

type LayoutFacts = Readonly<Record<string, boolean>>;

interface SweptPage {
  readonly path: string;
  readonly ready: (page: Page) => Locator;
  readonly measure?: (page: Page) => Promise<LayoutFacts>;
}

async function pagesScrollingSideways(
  page: Page,
  pages: readonly SweptPage[],
  measured: Record<string, boolean> = {},
): Promise<string[]> {
  await page.setViewportSize(phone);
  const sideways: string[] = [];
  for (const { path, ready, measure } of pages) {
    await page.goto(path);
    await ready(page).waitFor();
    await pageLoaded(page);
    await page.evaluate(async () => {
      await document.fonts.ready;
    });
    if (await scrollsSideways(page)) {
      sideways.push(path);
    }
    Object.assign(measured, (await measure?.(page)) ?? {});
  }
  return sideways;
}

function allTrue(facts: LayoutFacts): LayoutFacts {
  return Object.fromEntries(Object.keys(facts).map((fact) => [fact, true]));
}

async function sizeOf(locator: Locator): Promise<{ readonly width: number; readonly height: number }> {
  const box = await locator.boundingBox();
  if (box === null) {
    throw new Error('the element has no layout box');
  }
  return { width: box.width, height: box.height };
}

async function atFullWidth(control: Locator, column: Locator): Promise<boolean> {
  return Math.abs((await sizeOf(control)).width - (await sizeOf(column)).width) <= 1;
}

async function isTarget(control: Locator): Promise<boolean> {
  const size = await sizeOf(control);
  return size.width >= 44 && size.height >= 44;
}

async function allAtLeast44High(controls: Locator): Promise<boolean> {
  const heights = await Promise.all((await controls.all()).map(async (control) => (await sizeOf(control)).height));
  return heights.length > 0 && heights.every((height) => height >= 44);
}

async function bothAtLeast44High(controls: Locator): Promise<boolean> {
  return (await controls.count()) === 2 && allAtLeast44High(controls);
}

async function isBelow(lower: Locator, upper: Locator): Promise<boolean> {
  return (await boxOf(lower)).top >= (await boxOf(upper)).bottom - 0.5;
}

function pageColumn(page: Page): Locator {
  return page.locator('main article').first();
}

async function studentSettingsFacts(page: Page): Promise<LayoutFacts> {
  const path = new URL(page.url()).pathname;
  const row = page.locator('main [data-settings-value]').first();
  return {
    [`${path}: every section link is 44 px high`]: await allAtLeast44High(
      page.getByRole('navigation', { name: 'Settings sections' }).getByRole('link'),
    ),
    ...(path === '/settings/notifications'
      ? {}
      : {
          [`${path}: a row puts its control under its label`]: await isBelow(
            row,
            page.locator('main [data-settings-label]').first(),
          ),
        }),
  };
}

interface MarkupSweep {
  readonly sideways: readonly string[];
  readonly withoutMarkupText: readonly string[];
}

async function pagesSweptForMarkup(page: Page, pages: readonly SweptPage[]): Promise<MarkupSweep> {
  await page.setViewportSize(phone);
  const sideways: string[] = [];
  const withoutMarkupText: string[] = [];
  for (const { path, ready } of pages) {
    await page.goto(path);
    await ready(page).waitFor();
    await page.evaluate(async () => {
      await document.fonts.ready;
    });
    if (await scrollsSideways(page)) {
      sideways.push(path);
    }
    const main = page.getByRole('main');
    if (!((await main.textContent()) ?? '').includes(markup) || (await main.locator('img, b').count()) > 0) {
      withoutMarkupText.push(path);
    }
  }
  return { sideways, withoutMarkupText };
}

interface TutorPagesSeed {
  readonly tutor: TestAccount;
  readonly markupCardId: string;
  readonly groupId: string;
  readonly lessonId: string;
  readonly now: number;
}

async function tutorPagesWithLongAndMarkupNames(deployment: Deployment, control: TestControl): Promise<TutorPagesSeed> {
  const now = await control.now();
  const zone = daytimeZone(now);
  const tutor = await seedVerifiedUser(deployment, longWord, 'en');
  await setTimezone(deployment, tutor, zone);
  const long = await seedActiveStudent(deployment, tutor, longWord, 'en');
  const marked = await seedActiveStudent(deployment, tutor, markup, 'en');
  const groupId = await createGroup(deployment, tutor, longWord, [long.cardId, marked.cardId]);
  const markupGroupId = await createGroup(deployment, tutor, markup, [marked.cardId]);
  const soon = now + 10 * 60_000;
  const later = now + 60 * 60_000;
  const [lesson] = await scheduleLessonsWithIds(deployment, tutor, {
    group_id: markupGroupId,
    title: longWord,
    date: localDate(soon, zone),
    start: localClock(soon, zone),
    duration_minutes: 45,
  });
  await scheduleLessonsWithIds(deployment, tutor, {
    relationship_id: long.cardId,
    title: markup,
    date: localDate(later, zone),
    start: localClock(later, zone),
    duration_minutes: 45,
  });
  if (lesson === undefined) {
    throw new Error('no lesson was scheduled');
  }
  return { tutor, markupCardId: marked.cardId, groupId, lessonId: lesson.id, now };
}

function notificationsReady(page: Page): Locator {
  return firstTableRow(page).or(page.getByRole('main').getByText(/No notifications yet/));
}

test.describe('the 360 px sweeps', () => {
  test('the tutor’s frame group: Notifications does not scroll sideways and shows a name with markup as text', async ({
    page,
    deployment,
  }) => {
    const { tutor } = await tutorWithStudentInGroup(deployment, longWord, longWord);
    await seedActiveStudent(deployment, tutor, markup, 'en');
    await signInThroughTheApi(page, tutor);

    const sideways = await pagesScrollingSideways(page, [{ path: '/notifications', ready: firstTableRow }]);

    expect(sideways).toEqual([]);
    const table = page.getByRole('table', { name: 'Notifications' });
    await expect(table).toContainText(markup);
    await expect(table.locator('img, b')).toHaveCount(0);
    await expect(page).not.toHaveTitle('1');
  });

  test('the tutor’s pages group: Today, Schedule, Lesson, Students, the card, Groups, the group page and Settings do not scroll sideways and show names and titles with markup as text', async ({
    page,
    deployment,
    control,
  }) => {
    const seeded = await tutorPagesWithLongAndMarkupNames(deployment, control);
    await page.clock.install({ time: seeded.now });
    await signInThroughTheApi(page, seeded.tutor);
    const markupPages = [
      {
        path: '/today',
        ready: (shown: Page): Locator => shown.getByRole('list', { name: 'Today’s lessons' }).getByRole('link').first(),
      },
      { path: '/schedule', ready: (shown: Page): Locator => shown.locator('main a[href^="/lessons/"]').first() },
      { path: `/lessons/${seeded.lessonId}`, ready: headingNamed(longWord) },
      { path: '/students', ready: firstTableRow },
      { path: `/students/${seeded.markupCardId}`, ready: headingNamed(markup) },
      { path: '/groups', ready: firstTableRow },
      { path: `/groups/${seeded.groupId}`, ready: headingNamed(longWord) },
    ];

    const swept = await pagesSweptForMarkup(page, markupPages);
    const settingsSideways = await pagesScrollingSideways(
      page,
      settingsPaths.map((path) => ({ path, ready: settingsReady })),
    );

    expect(swept.sideways).toEqual([]);
    expect(swept.withoutMarkupText).toEqual([]);
    expect(settingsSideways).toEqual([]);
    await expect(page).not.toHaveTitle('1');
  });

  test('the student’s frame group: Notifications does not scroll sideways', async ({ page, deployment }) => {
    const { student } = await tutorWithStudentInGroup(deployment, longWord, longWord);
    await signInThroughTheApi(page, student);

    const sideways = await pagesScrollingSideways(page, [{ path: '/notifications', ready: notificationsReady }]);

    expect(sideways).toEqual([]);
  });

  test('the student’s pages group: Home, Schedule, My tutors, Keep or drop and Settings do not scroll sideways and keep their phone layout', async ({
    page,
    deployment,
  }) => {
    const { student } = await tutorWithStudentInGroup(deployment, longWord, longWord);
    const secondTutor = await seedVerifiedUser(deployment, longWord, 'en');
    await acceptSignedIn(
      deployment,
      student,
      (await createCard(deployment, secondTutor, longWord, student.email)).link,
    );
    await signInThroughTheApi(page, student);
    const measured: Record<string, boolean> = {};

    const sideways = await pagesScrollingSideways(
      page,
      [
        {
          path: '/home',
          ready: (shown: Page): Locator =>
            shown.getByRole('main').getByRole('heading', { level: 2, name: 'My tutors' }),
          measure: async (shown: Page): Promise<LayoutFacts> => ({
            'Home: the side column follows the main column': await isBelow(
              shown.locator('[data-column="aside"]'),
              shown.locator('[data-column="main"]'),
            ),
            'Home: the tutor filter is at the full width': await atFullWidth(
              shown.getByRole('group', { name: 'Tutor' }),
              pageColumn(shown),
            ),
          }),
        },
        {
          path: '/schedule',
          ready: (shown: Page): Locator => shown.getByLabel('Tutor', { exact: true }),
          measure: async (shown: Page): Promise<LayoutFacts> => ({
            'Schedule: the tutor filter is at the full width': await atFullWidth(
              shown.getByLabel('Tutor', { exact: true }),
              pageColumn(shown),
            ),
            'Schedule: the toolbar follows the tutor filter': await isBelow(
              shown.getByRole('group', { name: 'Calendar view' }),
              shown.getByLabel('Tutor', { exact: true }),
            ),
          }),
        },
        { path: '/reclaim', ready: (shown: Page): Locator => shown.getByText('Nothing is waiting for your choice.') },
        ...['/settings/profile', '/settings/account', '/settings/notifications'].map((path): SweptPage => ({
          path,
          ready: settingsReady,
          measure: studentSettingsFacts,
        })),
        {
          path: '/tutors',
          ready: firstTableRow,
          measure: async (shown: Page): Promise<LayoutFacts> => ({
            'My tutors: Message is a 44 px target': await isTarget(
              firstTableRow(shown).getByRole('link', { name: 'Messages' }),
            ),
            'My tutors: the row menu is a 44 px target': await isTarget(
              firstTableRow(shown).getByRole('button', { name: /^More actions for/ }),
            ),
            'My tutors: Send request is at the full width of its field': await atFullWidth(
              shown.getByRole('button', { name: 'Send request' }),
              shown.getByRole('textbox', { name: 'Tutor code', exact: true }),
            ),
          }),
        },
      ],
      measured,
    );

    expect(sideways).toEqual([]);
    expect(measured).toEqual(allTrue(measured));
    expect(Object.keys(measured)).toHaveLength(12);
    const main = await boxOf(page.locator('[data-column="main"]'));
    const aside = await boxOf(page.locator('[data-column="aside"]'));
    expect(aside.top).toBeGreaterThanOrEqual(main.bottom);
  });

  test('the public pages group: Sign in, Sign up and the token pages do not scroll sideways and keep their phone layout', async ({
    page,
  }) => {
    const heading = (shown: Page): Locator => shown.locator('main article h1');
    const measured: Record<string, boolean> = {};

    const sideways = await pagesScrollingSideways(
      page,
      [
        {
          path: '/',
          ready: heading,
          measure: async (shown: Page): Promise<LayoutFacts> => ({
            'Sign in: Sign in is at the full width': await atFullWidth(
              shown.getByRole('button', { name: 'Sign in', exact: true }),
              shown.getByLabel('Email'),
            ),
            'Sign in: Forgot password and Sign up are 44 px high': await bothAtLeast44High(
              shown.getByRole('main').getByRole('link', { name: /^(Forgot password\?|Sign up)$/ }),
            ),
          }),
        },
        {
          path: '/signup',
          ready: heading,
          measure: async (shown: Page): Promise<LayoutFacts> => {
            const choices = shown.getByRole('navigation', { name: 'Who you are' }).getByRole('link');
            return {
              'Sign up: I teach is at the full width': await atFullWidth(choices.first(), pageColumn(shown)),
              'Sign up: I study is at the full width': await atFullWidth(choices.nth(1), pageColumn(shown)),
              'Sign up: I study follows I teach': await isBelow(choices.nth(1), choices.first()),
            };
          },
        },
        { path: '/signup/tutor', ready: heading },
        {
          path: '/signup/student',
          ready: heading,
          measure: async (shown: Page): Promise<LayoutFacts> => {
            const fields = shown.getByRole('main').locator('input:not([type="checkbox"])');
            return {
              'Sign up as a student: the tutor code comes first':
                (await fields.first().getAttribute('name')) === 'tutor_code',
              'Sign up as a student: the consent checkbox has a 44 px target': await allAtLeast44High(
                shown.getByRole('main').locator('label:has(input[type="checkbox"])'),
              ),
              'Sign up as a student: Sign up is at the full width': await atFullWidth(
                shown.getByRole('button', { name: 'Sign up' }),
                fields.first(),
              ),
            };
          },
        },
        {
          path: '/forgot-password',
          ready: heading,
          measure: async (shown: Page): Promise<LayoutFacts> => ({
            'Forgot password: Send link is at the full width': await atFullWidth(
              shown.getByRole('button', { name: 'Send link' }),
              pageColumn(shown),
            ),
            'Forgot password: Back to sign-in is at the full width': await atFullWidth(
              shown.getByRole('main').getByRole('link', { name: 'Back to sign-in' }),
              pageColumn(shown),
            ),
          }),
        },
        {
          path: '/verify-email',
          ready: heading,
          measure: async (shown: Page): Promise<LayoutFacts> => ({
            'Confirmation without a link: its next step is at the full width': await atFullWidth(
              shown.getByRole('main').getByRole('link', { name: 'Go to sign-in' }),
              pageColumn(shown),
            ),
          }),
        },
        {
          path: '/reset-password',
          ready: heading,
          measure: async (shown: Page): Promise<LayoutFacts> => ({
            'Reset without a link: its next step is at the full width': await atFullWidth(
              shown.getByRole('main').getByRole('link', { name: 'Request a new link' }),
              pageColumn(shown),
            ),
          }),
        },
        { path: '/invite', ready: heading },
        { path: '/unsubscribe', ready: heading },
      ],
      measured,
    );

    expect(sideways).toEqual([]);
    expect(measured).toEqual(allTrue(measured));
    expect(Object.keys(measured)).toHaveLength(12);
  });
});

interface HomeworkScene {
  readonly tutor: TestAccount;
  readonly student: TestAccount;
  readonly templateId: string;
  readonly issueId: string;
  readonly checkedIssueId: string;
  readonly submittedId: string;
  readonly toDoId: string;
  readonly checkedId: string;
}

const checkedTitle = 'Checked work';
const toDoTitle = 'Work to do';
const otherTutorsTitle = 'Other tutor’s work';

async function homeworkScene(deployment: Deployment, studentLanguage: 'ru' | 'en'): Promise<HomeworkScene> {
  const tutor = await seedVerifiedUser(deployment, longWord, 'en');
  const { student, cardId } = await seedActiveStudent(deployment, tutor, longWord, studentLanguage);
  const classmate = await seedActiveStudent(deployment, tutor, markup, studentLanguage);
  const audience = { group_id: await createGroup(deployment, tutor, markup, [cardId, classmate.cardId]) };
  const templateId = await createPastSimpleTemplate(deployment, tutor);
  const issueId = await issueThroughTheApi(deployment, tutor, templateId, audience);
  const issued = new Map<string, string>();
  for (const title of [markup, checkedTitle, toDoTitle]) {
    const other = await createPastSimpleTemplate(deployment, tutor, { texts: { ...pastSimple, title } });
    issued.set(title, await issueThroughTheApi(deployment, tutor, other, audience));
  }
  const checkedIssueId = issued.get(checkedTitle);
  if (checkedIssueId === undefined) {
    throw new Error('the checked work was not issued');
  }
  const work = await studentWork(deployment, student, 'todo');
  const idOf = (title: string): string => {
    const found = work.find((item) => item.title === title);
    if (found === undefined) {
      throw new Error('the seeded work is missing');
    }
    return found.submissionId;
  };
  const submittedId = idOf(pastSimple.title);
  const checkedId = idOf(checkedTitle);
  await answerTextAndSubmit(deployment, student, submittedId, 'went');
  await saveReviewDraft(deployment, tutor, submittedId, 'Draft notes.', []);
  await answerTextAndSubmit(deployment, student, checkedId, 'went');
  const classmateWork = await studentWork(deployment, classmate.student, 'todo');
  const classmateSubmission = classmateWork.find((item) => item.title === pastSimple.title);
  if (classmateSubmission === undefined) {
    throw new Error('the classmate’s seeded work is missing');
  }
  await answerTextAndSubmit(deployment, classmate.student, classmateSubmission.submissionId, 'went');
  const otherTutor = await seedVerifiedUser(deployment, markup, 'en');
  const otherCard = await createCard(deployment, otherTutor, longWord, student.email);
  await acceptSignedIn(deployment, student, otherCard.link);
  const otherTemplate = await createPastSimpleTemplate(deployment, otherTutor, {
    texts: { ...pastSimple, title: otherTutorsTitle },
  });
  await issueThroughTheApi(deployment, otherTutor, otherTemplate, { relationship_id: otherCard.id });
  const [, , fileQuestion] = await questionIds(deployment, student, checkedId);
  if (fileQuestion === undefined) {
    throw new Error('the seeded work has no file question');
  }
  await publishReview(deployment, tutor, checkedId, 'check', markup, [
    { questionId: fileQuestion, points: 5, comment: markup },
  ]);
  return { tutor, student, templateId, issueId, checkedIssueId, submittedId, toDoId: idOf(toDoTitle), checkedId };
}

interface HomeworkSweep {
  readonly sideways: readonly string[];
  readonly wideTables: readonly string[];
  readonly texts: readonly string[];
  readonly fullWidthButtons: readonly (readonly string[])[];
  readonly rowHeaders: readonly (readonly string[])[];
  readonly rowMenus: number;
  readonly smallRowMenus: number;
  readonly injected: number;
}

const columnInset = 64;

async function fullWidthButtonsOf(page: Page): Promise<string[]> {
  return page.evaluate((inset) => {
    const main = document.querySelector('main');
    if (main === null) {
      throw new Error('the page has no main element');
    }
    const limit = main.getBoundingClientRect().width - inset;
    return Array.from(main.querySelectorAll('button'))
      .filter((button) => button.getBoundingClientRect().width >= limit)
      .map((button) => button.textContent.trim());
  }, columnInset);
}

async function rowHeadersOf(page: Page): Promise<string[]> {
  return page.locator('main tbody th').allInnerTexts();
}

async function smallRowMenusOf(page: Page): Promise<number> {
  return page.evaluate(
    () =>
      Array.from(document.querySelectorAll('main tbody button[aria-expanded]')).filter((button) => {
        const box = button.getBoundingClientRect();
        return Math.min(box.width, box.height) < 44;
      }).length,
  );
}

async function overflowingElements(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const limit = document.documentElement.clientWidth;
    const overflowing = Array.from(document.querySelectorAll('body *')).filter(
      (element) => element.getBoundingClientRect().right > limit + 0.5,
    );
    return overflowing
      .filter((element) => !overflowing.some((other) => other !== element && element.contains(other)))
      .map(
        (element) =>
          `${element.tagName.toLowerCase()}[${(element.getAttribute('class') ?? '').slice(0, 80)}]${element.closest('[role="group"]') === null ? '' : ' in a group'}`,
      );
  });
}

async function sweepHomeworkPages(page: Page, pages: readonly SweptPage[]): Promise<HomeworkSweep> {
  await page.setViewportSize(phone);
  const sideways: string[] = [];
  const wideTables: string[] = [];
  const texts: string[] = [];
  const fullWidthButtons: string[][] = [];
  const rowHeaders: string[][] = [];
  let rowMenus = 0;
  let smallRowMenus = 0;
  let injected = 0;
  for (const { path, ready } of pages) {
    await page.goto(path);
    await ready(page).waitFor();
    await page.evaluate(async () => {
      await document.fonts.ready;
    });
    if (await scrollsSideways(page)) {
      sideways.push(`${path}: ${(await overflowingElements(page)).join(', ')}`);
    }
    const widest = await page.evaluate(() =>
      Math.max(
        0,
        ...Array.from(document.querySelectorAll('main table')).map(
          (table) =>
            Array.from(table.querySelectorAll('thead th')).filter(
              (header) => getComputedStyle(header).display !== 'none',
            ).length,
        ),
      ),
    );
    if (widest > 2) {
      wideTables.push(path);
    }
    texts.push(await page.locator('main').innerText());
    fullWidthButtons.push(await fullWidthButtonsOf(page));
    rowHeaders.push(await rowHeadersOf(page));
    rowMenus += await page.locator('main tbody button[aria-expanded]').count();
    smallRowMenus += await smallRowMenusOf(page);
    injected += await page.locator('main').locator('img, b').count();
  }
  return { sideways, wideTables, texts, fullWidthButtons, rowHeaders, rowMenus, smallRowMenus, injected };
}

function titleFieldOf(page: Page): Locator {
  return page.getByRole('textbox', { name: 'Title', exact: true });
}

function markCheckedOf(page: Page): Locator {
  return page.getByRole('button', { name: 'Mark checked', exact: true });
}

function headingNamed(name: string): (page: Page) => Locator {
  return (shown) => shown.getByRole('heading', { level: 1, name, exact: true });
}

test.describe('the 360 px sweeps of the homework pages', () => {
  test('the tutor’s homework group: Library, Builder, Issued work, the issue page, the review queue and the review keep two columns at most, do not scroll sideways and show a title with markup as text', async ({
    page,
    deployment,
  }) => {
    const scene = await homeworkScene(deployment, 'en');
    await signInThroughTheApi(page, scene.tutor);

    const swept = await sweepHomeworkPages(page, [
      {
        path: `/homework/templates/${scene.templateId}`,
        ready: titleFieldOf,
      },
      { path: '/homework/issued', ready: firstTableRow },
      { path: `/homework/issued/${scene.checkedIssueId}`, ready: firstTableRow },
      { path: '/homework/review', ready: firstTableRow },
      {
        path: `/homework/review/${scene.submittedId}`,
        ready: markCheckedOf,
      },
      { path: '/homework/library', ready: firstTableRow },
    ]);

    expect(swept.sideways).toEqual([]);
    expect(swept.wideTables).toEqual([]);
    expect([1, 2, 3, 5].map((index) => swept.texts[index]?.includes(markup))).toEqual([true, true, true, true]);
    expect(swept.injected).toBe(0);
    await expect(page).not.toHaveTitle('1');
    const [, issuedList, issuePage, queue, , library] = swept.texts;
    expect(issuedList).toMatch(/\d\/2 submitted/);
    expect(issuePage).toMatch(/\d+ of 10/);
    expect(queue).toContain(pastSimple.title);
    const draftedRow = swept.rowHeaders[3]?.find((header) => header.includes(longWord));
    expect(draftedRow).toContain('Review in progress');
    expect(draftedRow).toMatch(/\d{1,2}:\d{2}/);
    expect(library).toContain('3 questions');
    expect(library).toContain('Ready');
    expect(swept.fullWidthButtons[4]).toEqual(
      expect.arrayContaining(['Attach corrected file', 'Save draft', 'Return for revision', 'Mark checked']),
    );
    expect(swept.rowMenus).toBeGreaterThan(0);
    expect(swept.smallRowMenus).toBe(0);
  });

  test('the student’s homework group in Russian and the dark theme: Homework, checked results and the assignment page keep two columns at most, do not scroll sideways, show no points and put Attach files and Submit at the full width', async ({
    page,
    deployment,
  }) => {
    const scene = await homeworkScene(deployment, 'ru');
    await page.emulateMedia({ colorScheme: 'dark' });
    await signInThroughTheApi(page, scene.student);

    const swept = await sweepHomeworkPages(page, [
      { path: '/homework', ready: firstTableRow },
      { path: '/homework?status=checked', ready: firstTableRow },
      { path: `/homework/${scene.checkedId}`, ready: headingNamed(checkedTitle) },
      { path: `/homework/${scene.toDoId}`, ready: headingNamed(toDoTitle) },
    ]);

    expect(swept.sideways).toEqual([]);
    expect(swept.wideTables).toEqual([]);
    const [toDoList, checkedList, checkedResult] = swept.texts;
    expect([toDoList, checkedList, checkedResult].flatMap((text) => scoreFormsIn(text ?? '', 'ru'))).toEqual([]);
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    const otherTutorsRow = swept.rowHeaders[0]?.find((header) => header.includes(otherTutorsTitle));
    expect(otherTutorsRow).toContain(markup);
    expect(otherTutorsRow).toContain('Нужно сделать');
    expect(checkedList).toContain('Проверено');
    expect(checkedResult).toContain(markup);
    expect(swept.injected).toBe(0);
    await expect(page).not.toHaveTitle('1');
    expect(swept.fullWidthButtons[3]).toEqual(expect.arrayContaining(['Прикрепить файлы', 'Сдать']));
  });
});

interface MessagesScene {
  readonly tutor: TestAccount;
  readonly student: TestAccount;
  readonly direct: string;
  readonly group: string;
}

const lastHistoryLine = 'History message 8';

async function postMessage(deployment: Deployment, token: string, channel: string, body: string): Promise<void> {
  const sent = await callApi(deployment, token, 'POST', `/channels/${channel}/messages`, { body, attachment_ids: [] });
  if (sent.status !== 201) {
    throw new Error(`sending a seeded message answered ${String(sent.status)}`);
  }
}

async function messagesScene(deployment: Deployment, studentLanguage: 'ru' | 'en'): Promise<MessagesScene> {
  const tutor = await seedVerifiedUser(deployment, longWord, 'en');
  const { student, cardId } = await seedActiveStudent(deployment, tutor, longWord, studentLanguage);
  const groupId = await createGroup(deployment, tutor, markup, [cardId]);
  const direct = await channelOf(deployment, tutor, `/students/${cardId}`);
  const group = await channelOf(deployment, tutor, `/groups/${groupId}`);
  const tutorToken = await accessToken(deployment, tutor);
  const lines = Array.from({ length: 4 }, (_, line) => `Line ${String(line + 1)} of a long message`).join('\n');
  await postMessage(deployment, tutorToken, group, markup);
  await postMessage(deployment, tutorToken, direct, `${longWord}${longWord}`);
  await postMessage(deployment, tutorToken, direct, markup);
  for (let index = 1; index <= 8; index += 1) {
    await postMessage(deployment, tutorToken, direct, `${lines}\nHistory message ${String(index)}`);
  }
  return { tutor, student, direct, group };
}

interface ChatWords {
  readonly list: string;
  readonly back: string;
  readonly attach: string;
  readonly composer: string;
  readonly send: string;
  readonly history: string;
}

const chatWords: Readonly<Record<'ru' | 'en', ChatWords>> = {
  en: {
    list: 'Chats',
    back: 'Back to the list',
    attach: 'Attach files',
    composer: 'Message',
    send: 'Send',
    history: 'Messages',
  },
  ru: {
    list: 'Чаты',
    back: 'К списку чатов',
    attach: 'Прикрепить файлы',
    composer: 'Сообщение',
    send: 'Отправить',
    history: 'Сообщения',
  },
};

function historyOf(page: Page, words: ChatWords): Locator {
  return page.getByRole('list', { name: words.history, exact: true });
}

function chatListReady(words: ChatWords): (page: Page) => Locator {
  return (page: Page): Locator => page.getByRole('region', { name: words.list, exact: true }).getByRole('link').first();
}

function conversationReady(words: ChatWords, lastMessage: string): (page: Page) => Locator {
  return (page: Page): Locator => historyOf(page, words).getByRole('listitem').filter({ hasText: lastMessage });
}

function chatListFacts(words: ChatWords): (page: Page) => Promise<LayoutFacts> {
  return async (page: Page): Promise<LayoutFacts> => {
    const list = page.getByRole('region', { name: words.list, exact: true });
    return {
      'the list: the bottom bar is shown': (await page.locator('nav[data-frame-only]').count()) === 1,
      'the list: a group name and a preview with markup are text': (await list.getByText(markup).count()) === 2,
      'the list: no markup element is rendered': (await list.locator('img, b').count()) === 0,
    };
  };
}

function conversationFacts(
  words: ChatWords,
  name: 'direct' | 'group',
  lastMessage: string,
): (page: Page) => Promise<LayoutFacts> {
  return async (page: Page): Promise<LayoutFacts> => {
    const composer = await boxOf(page.getByRole('textbox', { name: words.composer, exact: true }));
    const send = await boxOf(page.getByRole('button', { name: words.send, exact: true }));
    const latest = await boxOf(historyOf(page, words).getByRole('listitem').filter({ hasText: lastMessage }));
    const viewport = page.viewportSize()?.height ?? 0;
    return {
      [`${name}: no top bar and no bottom bar`]: (await page.locator('[data-frame-only]').count()) === 0,
      ...(name === 'group'
        ? {
            'group: the heading with markup is text': (
              (await page.getByRole('heading', { level: 2 }).textContent()) ?? ''
            ).includes(markup),
            'group: the header renders no markup element':
              (await page.locator('main section > div').first().locator('img, b').count()) === 0,
          }
        : {}),
      [`${name}: Back to the list is a 44 px target`]: await isTarget(
        page.getByRole('link', { name: words.back, exact: true }),
      ),
      [`${name}: attach is a 44 px target`]: await isTarget(page.getByRole('button', { name: words.attach })),
      [`${name}: Send stays inside the window`]: send.bottom <= viewport,
      [`${name}: the latest message is above the composer`]: latest.bottom <= composer.top && latest.top >= 0,
      [`${name}: the history does not scroll sideways`]: await historyOf(page, words).evaluate((list) => {
        let pane = list.parentElement;
        while (pane !== null && getComputedStyle(pane).overflowY !== 'auto') pane = pane.parentElement;
        return pane !== null && pane.scrollWidth <= pane.clientWidth;
      }),
      [`${name}: the window does not scroll`]: await page.evaluate(
        () => document.documentElement.scrollHeight <= document.documentElement.clientHeight,
      ),
    };
  };
}

async function shownAsText(page: Page, words: ChatWords): Promise<{ text: string; injected: number }> {
  const message = historyOf(page, words).getByRole('listitem').filter({ hasText: markup });
  return {
    text: (await message.textContent()) ?? '',
    injected: await historyOf(page, words).locator('img, b').count(),
  };
}

test.describe('the 360 px sweeps of Messages', () => {
  test('the tutor’s Messages group: the list and the conversations do not scroll sideways, the conversation hides the frame above a pinned composer, and a body with markup is text', async ({
    page,
    deployment,
  }) => {
    const scene = await messagesScene(deployment, 'en');
    const words = chatWords.en;
    await signInThroughTheApi(page, scene.tutor);
    const measured: Record<string, boolean> = {};

    const sideways = await pagesScrollingSideways(
      page,
      [
        { path: '/messages', ready: chatListReady(words), measure: chatListFacts(words) },
        {
          path: `/messages/${scene.direct}`,
          ready: conversationReady(words, lastHistoryLine),
          measure: async (shown: Page): Promise<LayoutFacts> => ({
            ...(await conversationFacts(words, 'direct', lastHistoryLine)(shown)),
            'direct: Student card is 44 px high': await allAtLeast44High(
              shown.getByRole('link', { name: 'Student card', exact: true }),
            ),
          }),
        },
        {
          path: `/messages/${scene.group}`,
          ready: conversationReady(words, markup),
          measure: async (shown: Page): Promise<LayoutFacts> => ({
            ...(await conversationFacts(words, 'group', markup)(shown)),
            'group: Group is 44 px high': await allAtLeast44High(
              shown.getByRole('link', { name: 'Group', exact: true }),
            ),
            'group: the members list is collapsed behind a 44 px Members control': await allAtLeast44High(
              shown.locator('main details:not([open]) > summary').filter({ hasText: 'Members' }),
            ),
            'group: the members list is hidden while collapsed': await shown
              .getByRole('list', { name: 'Members', exact: true })
              .isHidden(),
          }),
        },
      ],
      measured,
    );
    const group = await shownAsText(page, words);
    const membersControl = page.locator('main details > summary').filter({ hasText: 'Members' });
    await membersControl.click();
    await membersControl.focus();
    await page.setViewportSize({ width: 1024, height: 780 });
    await page.getByRole('region', { name: words.list, exact: true }).waitFor();
    const headingFocusedAfterWidening = await page
      .getByRole('heading', { level: 2 })
      .evaluate((element) => element === document.activeElement);

    expect(sideways).toEqual([]);
    expect(measured).toEqual(allTrue(measured));
    expect(Object.keys(measured)).toHaveLength(23);
    expect(group.text).toContain(markup);
    expect(group.injected).toBe(0);
    await expect(page.getByRole('list', { name: 'Members', exact: true })).toContainText(longWord);
    expect(headingFocusedAfterWidening).toBe(true);
    await expect(page).not.toHaveTitle('1');
  });

  test('the student’s Messages group in Russian and the dark theme: the list and the conversations do not scroll sideways, the conversation hides the frame above a pinned composer, and a body with markup is text', async ({
    page,
    deployment,
  }) => {
    const scene = await messagesScene(deployment, 'ru');
    const words = chatWords.ru;
    await page.emulateMedia({ colorScheme: 'dark' });
    await signInThroughTheApi(page, scene.student);
    const measured: Record<string, boolean> = {};

    const sideways = await pagesScrollingSideways(
      page,
      [
        { path: '/messages', ready: chatListReady(words), measure: chatListFacts(words) },
        {
          path: `/messages/${scene.direct}`,
          ready: conversationReady(words, lastHistoryLine),
          measure: conversationFacts(words, 'direct', lastHistoryLine),
        },
        {
          path: `/messages/${scene.group}`,
          ready: conversationReady(words, markup),
          measure: conversationFacts(words, 'group', markup),
        },
      ],
      measured,
    );
    const group = await shownAsText(page, words);

    expect(sideways).toEqual([]);
    expect(measured).toEqual(allTrue(measured));
    expect(Object.keys(measured)).toHaveLength(19);
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    expect(group.text).toContain(markup);
    expect(group.injected).toBe(0);
    await expect(page).not.toHaveTitle('1');
  });
});

test.describe('desktop layout', () => {
  test('Students shows its table columns in one row with the filters above them at 1440 × 900', async ({
    page,
    deployment,
  }) => {
    const { tutor } = await tutorWithStudentInGroup(deployment);
    await signInThroughTheApi(page, tutor);

    await openAt(page, computer, '/students');
    const table = page.getByRole('table', { name: 'Active', exact: true });
    await tableRows(page, 'Active').first().waitFor();

    const headerTops = await Promise.all(
      (await table.getByRole('columnheader').all()).map(async (header) => (await boxOf(header)).top),
    );
    expect(headerTops).toHaveLength(6);
    expect(Math.max(...headerTops) - Math.min(...headerTops)).toBeLessThanOrEqual(1);
    const tableTop = (await boxOf(table)).top;
    expect((await boxOf(page.getByRole('searchbox', { name: 'Name or email' }))).bottom).toBeLessThan(tableTop);
    expect((await boxOf(page.getByRole('combobox', { name: 'Group' }))).bottom).toBeLessThan(tableTop);
  });

  test('a page title is 28 px at 1440 × 900', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Tutor', 'en');
    await signInThroughTheApi(page, tutor);

    await openAt(page, computer, '/students', 'heading');

    await expect(page.getByRole('heading', { level: 1, name: 'Students' })).toHaveCSS('font-size', '28px');
  });

  test('Today keeps the larger title at 1440 × 900', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Tutor', 'en');
    await signInThroughTheApi(page, tutor);

    await openAt(page, computer, '/today', 'heading');

    await expect(page.getByRole('heading', { level: 1, name: 'Today' })).toHaveCSS('font-size', '36px');
  });

  for (const { path } of tutorPages) {
    test(`${path} does not scroll sideways at 1024 px`, async ({ page, deployment }) => {
      const { tutor } = await tutorWithStudentInGroup(deployment, longWord);
      await signInThroughTheApi(page, tutor);

      await openAt(page, smallComputer, path);

      expect(await scrollsSideways(page)).toBe(false);
    });
  }

  test('My tutors does not scroll sideways at 1024 px', async ({ page, deployment }) => {
    const { student } = await tutorWithStudentInGroup(deployment);
    await signInThroughTheApi(page, student);

    await openAt(page, smallComputer, '/tutors');

    expect(await scrollsSideways(page)).toBe(false);
  });

  test('at 1440 px My tutors shows its side column beside the main column', async ({ page, deployment }) => {
    const { student } = await tutorWithStudentInGroup(deployment);
    await signInThroughTheApi(page, student);

    await openAt(page, computer, '/tutors');
    await tableRows(page, 'Tutors').first().waitFor();
    const main = await boxOf(page.locator('[data-column="main"]'));
    const aside = await boxOf(page.locator('[data-column="aside"]'));

    expect(Math.abs(aside.top - main.top)).toBeLessThanOrEqual(1);
  });

  test('a table row keeps the tutor as its header and its actions apart', async ({ page, deployment }) => {
    const { student } = await tutorWithStudentInGroup(deployment);
    await signInThroughTheApi(page, student);

    await openAt(page, computer, '/tutors');
    const row = tableRows(page, 'Tutors').first();

    await expect(row.getByRole('rowheader')).toContainText('Tutor');
    const actions = row.getByRole('cell').last();
    await expect(actions.getByRole('link', { name: 'Messages' })).toBeVisible();
    await expect(actions.getByRole('button', { name: /Tutor/ })).toBeVisible();
  });

  for (const { path, table } of tutorPages) {
    test(`${path} shows a name with markup as text in its table`, async ({ page, deployment }) => {
      const tutor = await seedVerifiedUser(deployment, 'Tutor', 'en');
      const { cardId } = await seedActiveStudent(deployment, tutor, markup, 'en');
      await createGroup(deployment, tutor, markup, [cardId]);
      await signInThroughTheApi(page, tutor);

      await openAt(page, computer, path);
      const shown = await firstRowOf(page, table);

      expect(shown.text).toContain(markup);
      expect(shown.injected).toBe(0);
      await expect(page).not.toHaveTitle('1');
    });
  }

  test('New group stands in the page header once a group exists and gets focus back from its dialog', async ({
    page,
    deployment,
  }) => {
    const { tutor } = await tutorWithStudentInGroup(deployment);
    await signInThroughTheApi(page, tutor);
    await openAt(page, computer, '/groups');
    const newGroup = page.locator('main header').getByRole('button', { name: 'New group' });

    await newGroup.click();
    await page.getByRole('dialog').getByRole('button', { name: 'Cancel' }).click();

    await expect(page.getByRole('main').getByRole('button', { name: 'New group' })).toHaveCount(1);
    await expect(newGroup).toBeFocused();
  });

  for (const { path, heading } of [
    { path: '/settings/account', heading: 'Timezone' },
    { path: '/settings/teaching', heading: 'Tutor code' },
  ] as const) {
    test(`a Settings row on ${path} has its label and value on one line at 1440 × 900`, async ({
      page,
      deployment,
    }) => {
      const tutor = await seedVerifiedUser(deployment, 'Tutor', 'en');
      await signInThroughTheApi(page, tutor);

      await openAt(page, computer, path, 'heading');
      const label = page.locator('[data-settings-label]').filter({ hasText: heading });
      const row = label.locator('xpath=..');
      const control = row.locator('[data-settings-value]').locator('select, button, p').first();
      await control.waitFor();
      const labelBox = await label.boundingBox();
      const controlBox = await control.boundingBox();
      const rowBox = await row.boundingBox();

      expect(labelBox).not.toBeNull();
      expect(controlBox).not.toBeNull();
      expect(rowBox).not.toBeNull();
      if (labelBox !== null && controlBox !== null && rowBox !== null) {
        expect(labelBox.x + labelBox.width).toBeLessThanOrEqual(controlBox.x);
        expect(Math.abs(labelBox.y + labelBox.height / 2 - (controlBox.y + controlBox.height / 2))).toBeLessThan(
          rowBox.height / 2,
        );
      }
    });
  }

  for (const path of settingsPaths) {
    test(`${path} does not scroll sideways at 1024 px`, async ({ page, deployment }) => {
      const tutor = await seedVerifiedUser(deployment, 'Tutor', 'en');
      await signInThroughTheApi(page, tutor);

      await openAt(page, smallComputer, path, 'heading');
      await settingsReady(page).waitFor();

      expect(await scrollsSideways(page)).toBe(false);
    });
  }

  test('a tutor name with markup is shown as text in My tutors', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, markup, 'en');
    const { student } = await seedActiveStudent(deployment, tutor, 'Vera Student', 'en');
    await signInThroughTheApi(page, student);

    await openAt(page, computer, '/tutors');
    const shown = await firstRowOf(page, 'Tutors');

    expect(shown.text).toContain(markup);
    expect(shown.injected).toBe(0);
    await expect(page).toHaveTitle('My tutors · OVC');
  });
});
