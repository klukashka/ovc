import type { BrowserContext, Page } from '@playwright/test';

import type { ObservedBrowser } from './browser-diagnostics.ts';

import { type TestAccount, newTestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser, signUpAndConfirmTutor } from './fixtures.ts';
import { recordShownRegion, shownRegion } from './observers.ts';
import { expectRefetchedAfterTheSocketIsReady, trafficAfterReady } from './realtime.ts';
import {
  type LocalDate,
  addDays,
  cancelThisLesson,
  isoWeekday,
  lessonAddresses,
  lessonLinks,
  localClock,
  localDate,
  moscowInstant,
  nextWeekday,
  scheduleLessons,
  scheduleLessonsWithIds,
  seriesDates,
  setTimezone,
} from './schedule.ts';
import {
  acceptSignedIn,
  createCard,
  createGroup,
  seedActiveStudent,
  signUpActiveStudentOnInvitation,
  signUpOnInvitation,
} from './tutoring.ts';

const moscow = 'Europe/Moscow';
const berlin = 'Europe/Berlin';
const yekaterinburg = 'Asia/Yekaterinburg';
const lessonQueryPaths = ['/api/v1/calendar'];

async function moscowTomorrow(now: () => Promise<number>): Promise<LocalDate> {
  return addDays(localDate(await now(), moscow), 1);
}

async function moscowDayWhoseNextDayIsInTheSameWeek(now: () => Promise<number>): Promise<LocalDate> {
  const tomorrow = await moscowTomorrow(now);
  return isoWeekday(tomorrow) === 7 ? addDays(tomorrow, 1) : tomorrow;
}

function weekAddress(date: LocalDate): string {
  return `/schedule?view=week&date=${date}`;
}

async function phoneContext(
  browser: ObservedBrowser,
  deployment: Deployment,
  timezoneId: string,
): Promise<BrowserContext> {
  return browser.newContext({
    baseURL: deployment.origin,
    ignoreHTTPSErrors: true,
    locale: 'ru-RU',
    timezoneId,
    viewport: { width: 390, height: 844 },
    colorScheme: 'light',
  });
}

async function englishContext(
  browser: ObservedBrowser,
  deployment: Deployment,
  timezoneId: string,
): Promise<BrowserContext> {
  return browser.newContext({
    baseURL: deployment.origin,
    ignoreHTTPSErrors: true,
    locale: 'en-GB',
    timezoneId,
    viewport: { width: 1280, height: 800 },
    colorScheme: 'dark',
  });
}

async function openNewLesson(page: Page): Promise<ReturnType<Page['getByRole']>> {
  await page.getByRole('button', { name: 'Новый урок' }).click();
  return page.getByRole('dialog', { name: 'Новый урок' });
}

async function fillOneOffLesson(
  dialog: ReturnType<Page['getByRole']>,
  student: string,
  date: LocalDate,
  start: string,
): Promise<void> {
  await dialog.getByRole('button', { name: student, exact: true }).click();
  await dialog.getByLabel('Дата', { exact: true }).fill(date);
  await dialog.getByLabel('Время начала').fill(start);
}

const pagingControls: readonly { readonly view: 'week' | 'month'; readonly control: string }[] = [
  { view: 'week', control: 'Следующая неделя' },
  { view: 'week', control: 'Предыдущая неделя' },
  { view: 'month', control: 'Следующий месяц' },
];

async function scheduleConversation(
  deployment: Deployment,
  tutor: TestAccount,
  cardId: string,
  date: LocalDate,
): Promise<void> {
  await scheduleLessons(deployment, tutor, {
    relationship_id: cardId,
    title: conversationTitle,
    date,
    start: '15:00',
    duration_minutes: 60,
  });
}

const conversationTitle = '<b>Разговорная</b> практика';

async function scheduleShortLessons(
  deployment: Deployment,
  tutor: TestAccount,
  cardId: string,
  date: LocalDate,
): Promise<void> {
  for (const [day, start] of [
    [date, '10:00'],
    [date, '10:15'],
    [addDays(date, 1), '11:00'],
  ] as const) {
    await scheduleLessons(deployment, tutor, {
      relationship_id: cardId,
      title: `Короткий урок в ${start}`,
      date: day,
      start,
      duration_minutes: 15,
    });
  }
}

async function expectWeekListedByDay(page: Page): Promise<void> {
  const agenda = page.getByRole('region', { name: 'Уроки по дням' });
  await expect(agenda).toBeVisible();
  await expect(page.getByRole('region', { name: 'Уроки недели' })).toHaveCount(0);
  await expect(agenda.getByRole('heading', { level: 3 })).toHaveCount(2);
  await expect(lessonLinks(page)).toHaveCount(3);
  const heights = await Promise.all(
    (await lessonLinks(page).all()).map(async (link) => (await link.boundingBox())?.height ?? 0),
  );
  expect(Math.min(...heights)).toBeGreaterThanOrEqual(44);
}

interface Box {
  readonly x: number;
  readonly y: number;
  readonly width: number;
  readonly height: number;
}

function isInside(part: Box | null, outer: Box | null): boolean {
  return (
    part !== null &&
    outer !== null &&
    part.width > 0 &&
    part.x >= outer.x &&
    part.y >= outer.y &&
    part.x + part.width <= outer.x + outer.width &&
    part.y + part.height <= outer.y + outer.height
  );
}

interface TutorWithStudent {
  readonly maria: TestAccount;
  readonly cardId: string;
}

async function tutorWithStudent(deployment: Deployment): Promise<TutorWithStudent> {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const { cardId } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
  return { maria, cardId };
}

test.describe('schedule for a tutor in Russian', () => {
  test.use({ locale: 'ru-RU', timezoneId: moscow, viewport: { width: 1280, height: 800 } });

  test('schedule › a dialog opens on its first field', async ({ page, deployment, control }) => {
    const { maria } = await tutorWithStudent(deployment);
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(await moscowTomorrow(control.now)));

    const dialog = await openNewLesson(page);

    await expect(dialog.getByLabel('Поиск ученика или группы')).toBeFocused();
    await expect(dialog.getByLabel('Название')).toHaveValue('Урок');
  });

  for (const { view, control: name } of pagingControls) {
    test(`schedule › previous and next keep focus on the pressed button (${name})`, async ({
      page,
      deployment,
      control,
    }) => {
      const { maria } = await tutorWithStudent(deployment);
      const date = await moscowTomorrow(control.now);
      await signInThroughTheApi(page, maria);
      await page.goto(`/schedule?view=${view}&date=${date}`);
      const heading = page.getByRole('main').getByRole('heading', { level: 2 });
      const shownPeriod = await heading.textContent();
      const pressed = page.getByRole('button', { name });
      await pressed.focus();

      await page.keyboard.press('Enter');

      await expect(pressed).toBeFocused();
      await expect(heading).not.toHaveText(shownPeriod ?? '');
      await expect(page).not.toHaveURL(new RegExp(`date=${date}`));
    });
  }

  test('schedule › next week keeps the shown week and its scroll position while the next week loads, then opens at its lesson', async ({
    page,
    deployment,
    control,
  }) => {
    const { maria, cardId } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    await scheduleConversation(deployment, maria, cardId, addDays(date, 7));
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(date));
    const weekGrid = 'section[aria-label="Уроки недели"]';
    const hours = page.locator(`${weekGrid} [data-part="hours"]`);
    const scrolledTo = await hours.evaluate((element) => {
      element.scrollTop = 0;
      return element.scrollTop;
    });
    await recordShownRegion(page, weekGrid);
    let releaseNextWeek = (): void => undefined;
    const nextWeekReleased = new Promise<void>((resolve) => {
      releaseNextWeek = resolve;
    });
    await page.route('**/api/v1/calendar?**', async (route) => {
      await nextWeekReleased;
      await route.continue();
    });

    await page.getByRole('button', { name: 'Следующая неделя' }).click();
    await page.locator(`[aria-busy="true"]:has(${weekGrid})`).waitFor();
    const scrollWhileLoading = await hours.evaluate((element) => element.scrollTop);
    releaseNextWeek();

    const lesson = lessonLinks(page).filter({ hasText: conversationTitle });
    await expect(lesson).toBeVisible();
    await expect(page.locator(`[aria-busy]:has(${weekGrid})`)).toHaveAttribute('aria-busy', 'false');
    expect(scrollWhileLoading).toBe(scrolledTo);
    await expect(lesson).toBeInViewport();
    expect(await hours.evaluate((element) => element.scrollTop)).toBeGreaterThan(scrolledTo);
    expect(await shownRegion(page)).toEqual({ busy: true, replaced: false });
  });

  test('schedule › at 1440 × 900 the week opens at its only lesson at 20:30 without scrolling', async ({
    page,
    deployment,
    control,
  }) => {
    const { maria, cardId } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    await scheduleLessons(deployment, maria, {
      relationship_id: cardId,
      title: 'Вечерний урок',
      date,
      start: '20:30',
      duration_minutes: 60,
    });
    await page.setViewportSize({ width: 1440, height: 900 });
    await signInThroughTheApi(page, maria);

    await page.goto(weekAddress(date));
    const evening = lessonLinks(page).filter({ hasText: 'Вечерний урок' });
    await evening.waitFor();
    const shownWithoutScrolling = await evening.evaluate((link) => {
      const hours = link.closest('[data-part="hours"]');
      if (hours === null) throw new Error('the lesson is not in the hours area');
      const pane = hours.getBoundingClientRect();
      const block = link.getBoundingClientRect();
      return block.top >= pane.top && block.bottom <= pane.bottom && block.bottom <= window.innerHeight;
    });
    const blockText = await evening.innerText();
    await evening.click();

    expect(shownWithoutScrolling).toBe(true);
    expect(blockText).not.toContain('Запланирован');
    await expect(page.getByRole('main').getByText('Запланирован', { exact: true })).toBeVisible();
  });

  test('schedule › Ends on starts from the first date the tutor chose', async ({ page, deployment, control }) => {
    const { maria } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    const laterDate = addDays(date, 10);
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(date));
    const dialog = await openNewLesson(page);
    await dialog.getByLabel('Дата', { exact: true }).fill(laterDate);
    await dialog.getByLabel('Повторять каждую неделю').check();

    await dialog.getByLabel('В дату').check();

    await expect(dialog.getByLabel('Дата окончания')).toHaveValue(laterDate);
  });

  test('schedule › a free slot prefills the dialog', async ({ page, deployment, control }) => {
    const { maria } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(date));

    await page.locator(`[data-slot="${date}T14:30"]`).click();

    const dialog = page.getByRole('dialog', { name: 'Новый урок' });
    await expect(dialog.getByLabel('Дата', { exact: true })).toHaveValue(date);
    await expect(dialog.getByLabel('Время начала')).toHaveValue('14:30');
    await expect(dialog.getByLabel('Поиск ученика или группы')).toBeFocused();
  });

  test('schedule › the overlap warning takes focus', async ({ page, deployment, control, diagnostics }) => {
    const { maria, cardId } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    await scheduleConversation(deployment, maria, cardId, date);
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/lessons', status: 409 });
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(date));
    const dialog = await openNewLesson(page);
    await fillOneOffLesson(dialog, 'Анна', date, '15:30');

    await dialog.getByRole('button', { name: 'Запланировать' }).click();

    const warning = dialog.getByRole('alert');
    await expect(warning).toBeFocused();
    await expect(warning).toContainText('Этот урок пересекается с другими вашими уроками:');
    await expect(warning.getByText(conversationTitle, { exact: true })).toHaveAttribute('dir', 'auto');
    await expect(dialog.locator('b')).toHaveCount(0);
    await expect(warning).toContainText('15:00–16:00');
    await expect(warning.getByRole('button', { name: 'Всё равно запланировать' })).toBeVisible();
    await expect(lessonLinks(page)).toHaveCount(1);
  });

  test('schedule › the overlap warning takes focus and Schedule anyway saves', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const { maria, cardId } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    await scheduleConversation(deployment, maria, cardId, date);
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/lessons', status: 409 });
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(date));
    const dialog = await openNewLesson(page);
    await fillOneOffLesson(dialog, 'Анна', date, '15:30');
    await dialog.getByRole('button', { name: 'Запланировать' }).click();
    const scheduleAnyway = dialog.getByRole('alert').getByRole('button', { name: 'Всё равно запланировать' });
    await scheduleAnyway.waitFor();

    await scheduleAnyway.click();

    await expect(dialog).toBeHidden();
    await expect(page.getByRole('button', { name: 'Новый урок' })).toBeFocused();
    await expect(lessonLinks(page)).toHaveCount(2);
    await expect(lessonLinks(page).filter({ hasText: '15:30–16:30' })).toHaveCount(1);
  });

  test('schedule › Schedule anyway keeps focus while it saves', async ({ page, deployment, control, diagnostics }) => {
    const { maria, cardId } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    await scheduleConversation(deployment, maria, cardId, date);
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/lessons', status: 409 });
    let release: () => void = () => undefined;
    const released = new Promise<void>((resolve) => {
      release = resolve;
    });
    await page.route('**/api/v1/lessons', async (route) => {
      if (route.request().postData()?.includes('"allow_overlap":true') === true) {
        await released;
      }
      await route.continue();
    });
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(date));
    const dialog = await openNewLesson(page);
    await fillOneOffLesson(dialog, 'Анна', date, '15:30');
    await dialog.getByRole('button', { name: 'Запланировать' }).click();
    const scheduleAnyway = dialog.getByRole('alert').getByRole('button', { name: 'Всё равно запланировать' });
    await scheduleAnyway.waitFor();
    await scheduleAnyway.focus();

    await page.keyboard.press('Enter');

    await expect(scheduleAnyway).toBeFocused();
    await expect(scheduleAnyway).toHaveAttribute('aria-busy', 'true');
    await expect(dialog.getByRole('button', { name: 'Запланировать', exact: true })).toHaveAttribute(
      'aria-disabled',
      'true',
    );
    await expect(dialog.getByRole('button', { name: 'Запланировать', exact: true })).toHaveAttribute(
      'aria-busy',
      'false',
    );
    release();
    await expect(dialog).toBeHidden();
    await expect(page.getByRole('button', { name: 'Новый урок' })).toBeFocused();
    await expect(lessonLinks(page)).toHaveCount(2);
  });

  test('schedule › a plain resubmit hides the overlap warning while it is pending', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const { maria, cardId } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    await scheduleConversation(deployment, maria, cardId, date);
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/lessons', status: 409 });
    let release: () => void = () => undefined;
    const released = new Promise<void>((resolve) => {
      release = resolve;
    });
    let posts = 0;
    await page.route('**/api/v1/lessons', async (route) => {
      if (route.request().method() === 'POST') {
        posts += 1;
        if (posts === 2) {
          await released;
        }
      }
      await route.continue();
    });
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(date));
    const dialog = await openNewLesson(page);
    await fillOneOffLesson(dialog, 'Анна', date, '15:30');
    const submit = dialog.getByRole('button', { name: 'Запланировать', exact: true });
    await submit.click();
    await dialog.getByRole('alert').waitFor();
    await diagnostics.finishHttpPhase();
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/lessons', status: 409 });
    await submit.focus();

    await page.keyboard.press('Enter');

    await expect(submit).toHaveAttribute('aria-busy', 'true');
    await expect(submit).toBeFocused();
    await expect(dialog.getByRole('alert')).toHaveCount(0);
    release();
    await expect(dialog.getByRole('alert')).toBeFocused();
    await diagnostics.finishHttpPhase();
  });

  test('schedule › a series without weekdays says so and the message takes focus', async ({
    page,
    deployment,
    control,
  }) => {
    const { maria } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(date));
    const dialog = await openNewLesson(page);
    await fillOneOffLesson(dialog, 'Анна', date, '12:00');
    await dialog.getByRole('checkbox', { name: 'Повторять каждую неделю' }).check();

    await dialog.getByRole('button', { name: 'Запланировать' }).click();

    const message = dialog.getByRole('alert');
    await expect(message).toHaveText('Выберите хотя бы один день недели.');
    await expect(message).toBeFocused();
    await expect(dialog).toBeVisible();
  });

  test('schedule › a dialog opened from a free slot returns focus to New lesson', async ({
    page,
    deployment,
    control,
  }) => {
    const { maria } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(date));
    await page.locator(`[data-slot="${date}T09:00"]`).click();
    const dialog = page.getByRole('dialog', { name: 'Новый урок' });
    await expect(dialog.getByLabel('Поиск ученика или группы')).toBeFocused();

    await page.keyboard.press('Escape');

    await expect(dialog).toBeHidden();
    await expect(page.getByRole('button', { name: 'Новый урок' })).toBeFocused();
  });

  test('schedule › below 1024 px the week lists lessons by day with 44 px targets', async ({
    page,
    deployment,
    control,
  }) => {
    const { maria, cardId } = await tutorWithStudent(deployment);
    const date = await moscowDayWhoseNextDayIsInTheSameWeek(control.now);
    await scheduleShortLessons(deployment, maria, cardId, date);
    await page.setViewportSize({ width: 800, height: 1000 });
    await signInThroughTheApi(page, maria);

    await page.goto(weekAddress(date));

    await expectWeekListedByDay(page);
  });

  test('schedule › on a coarse pointer the week lists lessons by day with 44 px targets @coarse-pointer', async ({
    observedBrowser: browser,
    deployment,
    control,
    diagnostics,
  }) => {
    const { maria, cardId } = await tutorWithStudent(deployment);
    const date = await moscowDayWhoseNextDayIsInTheSameWeek(control.now);
    await scheduleShortLessons(deployment, maria, cardId, date);
    const context = await browser.newContext({
      baseURL: deployment.origin,
      ignoreHTTPSErrors: true,
      locale: 'ru-RU',
      timezoneId: moscow,
      viewport: { width: 1280, height: 800 },
      hasTouch: true,
      isMobile: true,
    });
    try {
      const touchPage = await context.newPage();
      await diagnostics.watch(touchPage);
      await signInThroughTheApi(touchPage, maria);

      await touchPage.goto(weekAddress(date));

      const media = await touchPage.evaluate(() => ({
        coarse: globalThis.matchMedia('(pointer: coarse)').matches,
        wide: globalThis.matchMedia('(min-width: 1024px)').matches,
      }));
      expect(media).toEqual({ coarse: true, wide: true });
      await expectWeekListedByDay(touchPage);
    } finally {
      await diagnostics.closeContext(context);
    }
  });

  test('schedule › at 1024 px a 15-minute lesson shows its time, and a cancelled one also its status icon, inside a 24 px link', async ({
    page,
    deployment,
    control,
  }) => {
    const { maria, cardId } = await tutorWithStudent(deployment);
    const date = await moscowDayWhoseNextDayIsInTheSameWeek(control.now);
    await scheduleShortLessons(deployment, maria, cardId, date);
    const [cancelled] = await scheduleLessonsWithIds(deployment, maria, {
      relationship_id: cardId,
      title: 'Отменённый урок',
      date,
      start: '12:00',
      duration_minutes: 15,
    });
    if (cancelled === undefined) throw new Error('the cancelled lesson was not scheduled');
    await cancelThisLesson(deployment, maria, cancelled.id);
    await page.setViewportSize({ width: 1024, height: 800 });
    await signInThroughTheApi(page, maria);

    await page.goto(weekAddress(date));

    await expect(page.getByRole('navigation', { name: 'Основная навигация' })).toBeVisible();
    const scheduled = lessonLinks(page).filter({ hasText: 'Короткий урок в 10:00' });
    const struck = lessonLinks(page).filter({ hasText: 'Отменённый урок' });
    await scheduled.scrollIntoViewIfNeeded();
    const [scheduledBox, scheduledTimeBox] = await Promise.all([
      scheduled.boundingBox(),
      scheduled.locator('[data-part="time"]').boundingBox(),
    ]);
    await struck.scrollIntoViewIfNeeded();
    const [struckBox, struckTimeBox, struckStatusBox] = await Promise.all([
      struck.boundingBox(),
      struck.locator('[data-part="time"]').boundingBox(),
      struck.locator('[data-part="status"]').boundingBox(),
    ]);
    expect(scheduledBox?.height ?? 0).toBeGreaterThanOrEqual(24);
    expect(isInside(scheduledTimeBox, scheduledBox)).toBe(true);
    await expect(scheduled.locator('[data-part="status"]')).toHaveCount(0);
    await expect(scheduled).toHaveAttribute('title', /Короткий урок в 10:00$/);
    expect(struckBox?.height ?? 0).toBeGreaterThanOrEqual(24);
    for (const part of [struckTimeBox, struckStatusBox]) {
      expect(part).not.toBeNull();
      expect(isInside(part, struckBox)).toBe(true);
    }
    await expect(struck).toHaveAttribute('title', /Отменённый урок · Отменён/);
  });

  test('schedule › a focused lesson keeps focus when the week switches between the grid and the list', async ({
    page,
    deployment,
    control,
  }) => {
    const { maria, cardId } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    await scheduleLessons(deployment, maria, {
      relationship_id: cardId,
      title: 'Урок для фокуса',
      date,
      start: '12:00',
      duration_minutes: 60,
    });
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(date));
    const lesson = lessonLinks(page).filter({ hasText: 'Урок для фокуса' });
    await lesson.focus();

    await page.setViewportSize({ width: 900, height: 800 });

    await expect(page.getByRole('region', { name: 'Уроки по дням' })).toBeVisible();
    await expect(lesson).toBeFocused();
  });

  test('schedule › a saved lesson returns focus to New lesson', async ({ page, deployment, control }) => {
    const { maria } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(date));
    const dialog = await openNewLesson(page);
    await fillOneOffLesson(dialog, 'Анна', date, '12:00');

    await dialog.getByRole('button', { name: 'Запланировать' }).click();

    await expect(dialog).toBeHidden();
    await expect(page.getByRole('button', { name: 'Новый урок' })).toBeFocused();
    await expect(page.getByRole('status').filter({ hasText: 'Урок запланирован' })).toBeVisible();
    await expect(lessonLinks(page).filter({ hasText: '12:00–13:00' })).toContainText('Анна');
  });

  test('schedule › choosing a student collapses the picker, and Change brings the search back with focus', async ({
    page,
    deployment,
    control,
  }) => {
    const { maria } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(date));
    const dialog = await openNewLesson(page);
    const change = dialog.getByRole('button', { name: 'Изменить', exact: true });

    await dialog.getByRole('button', { name: 'Анна', exact: true }).click();
    const collapsedFocus = await change.evaluate((element) => element === document.activeElement);
    const searchWhileCollapsed = await dialog.getByLabel('Поиск ученика или группы').count();
    await change.click();

    expect(collapsedFocus).toBe(true);
    expect(searchWhileCollapsed).toBe(0);
    await expect(dialog.getByLabel('Поиск ученика или группы')).toBeFocused();
    await expect(dialog.getByRole('button', { name: 'Анна', exact: true })).toBeVisible();
  });

  test('schedule › after Change, submitting from the search asks for an audience instead of scheduling the previous one', async ({
    page,
    deployment,
    control,
  }) => {
    const { maria } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    const lessonWrites: string[] = [];
    page.on('request', (request) => {
      if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/lessons') {
        lessonWrites.push(request.url());
      }
    });
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(date));
    const dialog = await openNewLesson(page);
    await fillOneOffLesson(dialog, 'Анна', date, '12:00');
    await dialog.getByRole('button', { name: 'Изменить', exact: true }).click();

    await dialog.getByLabel('Поиск ученика или группы').press('Enter');

    await expect(dialog.getByText('Выберите ученика или группу.')).toBeVisible();
    await expect(dialog.getByLabel('Поиск ученика или группы')).toBeFocused();
    await expect(dialog).toBeVisible();
    expect(lessonWrites).toEqual([]);
  });

  test('schedule › a duration preset and a free value set the saved length, and Add description opens its field', async ({
    page,
    deployment,
    control,
  }) => {
    const { maria } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    await signInThroughTheApi(page, maria);
    await page.goto(weekAddress(date));
    const first = await openNewLesson(page);
    await fillOneOffLesson(first, 'Анна', date, '12:00');
    const dateFits = await first
      .getByLabel('Дата', { exact: true })
      .evaluate((element) => element.scrollWidth <= element.clientWidth);
    await first.getByRole('button', { name: '90 мин', exact: true }).click();
    await first.getByRole('button', { name: 'Добавить описание', exact: true }).click();
    const descriptionFocused = await first
      .getByLabel('Описание', { exact: true })
      .evaluate((element) => element === document.activeElement);
    await first.getByLabel('Описание', { exact: true }).fill('Повторение глаголов');
    await first.getByRole('button', { name: 'Запланировать' }).click();
    await first.waitFor({ state: 'hidden' });
    const second = await openNewLesson(page);
    await fillOneOffLesson(second, 'Анна', date, '15:00');
    await second.getByLabel('Длительность, минут').fill('75');
    await second.getByRole('button', { name: 'Запланировать' }).click();
    await second.waitFor({ state: 'hidden' });

    expect(dateFits).toBe(true);
    expect(descriptionFocused).toBe(true);
    await expect(lessonLinks(page).filter({ hasText: '12:00–13:30' })).toHaveCount(1);
    await expect(lessonLinks(page).filter({ hasText: '15:00–16:15' })).toHaveCount(1);
    await lessonLinks(page).filter({ hasText: '12:00–13:30' }).click();
    await expect(page.getByRole('main').getByText('Повторение глаголов')).toBeVisible();
  });

  test('schedule › month shows "+{n} more" with 44 px targets', async ({ page, deployment, control }) => {
    const { maria, cardId } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    for (const start of ['09:00', '10:00', '11:00', '12:00', '13:00']) {
      await scheduleLessons(deployment, maria, {
        relationship_id: cardId,
        title: `Урок в ${start}`,
        date,
        start,
        duration_minutes: 45,
      });
    }
    await signInThroughTheApi(page, maria);
    await page.goto(`/schedule?view=month&date=${date}`);
    const more = page.getByRole('button', { name: 'Ещё 2' });
    await expect(lessonLinks(page)).toHaveCount(3);
    const targets = [...(await lessonLinks(page).all()), more];
    const heights = await Promise.all(targets.map(async (target) => (await target.boundingBox())?.height ?? 0));

    await more.click();

    expect(Math.min(...heights)).toBeGreaterThanOrEqual(44);
    await expect(lessonLinks(page)).toHaveCount(5);
    await expect(lessonLinks(page).nth(3)).toBeFocused();
    await expect(lessonLinks(page).nth(3)).toContainText('Урок в 12:00');
    await expect(more).toBeHidden();
  });

  test('schedule › a title with markup is shown as text', async ({ page, deployment, control }) => {
    const { maria, cardId } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    const title = '<img src=x onerror=alert(1)><b>Разметка</b>';
    await scheduleLessons(deployment, maria, {
      relationship_id: cardId,
      title,
      date,
      start: '17:00',
      duration_minutes: 60,
    });
    await signInThroughTheApi(page, maria);

    await page.goto(weekAddress(date));

    const shownTitle = lessonLinks(page).getByText(title, { exact: true });
    await expect(shownTitle).toBeVisible();
    await expect(shownTitle).toHaveAttribute('dir', 'auto');
    await expect(page.getByRole('main').locator('img, b')).toHaveCount(0);
  });

  test('schedule › an open calendar shows a lesson scheduled elsewhere', async ({ page, deployment, control }) => {
    const { maria, cardId } = await tutorWithStudent(deployment);
    const date = await moscowTomorrow(control.now);
    await signInThroughTheApi(page, maria);
    const traffic = trafficAfterReady(page, lessonQueryPaths);
    await traffic.initialize();
    await page.goto(weekAddress(date));
    await expect(page.getByRole('region', { name: 'Уроки недели' })).toBeVisible();
    await expectRefetchedAfterTheSocketIsReady(traffic);

    await scheduleLessons(deployment, maria, {
      relationship_id: cardId,
      title: 'Урок из другой вкладки',
      date,
      start: '19:00',
      duration_minutes: 60,
    });

    await expect(lessonLinks(page).filter({ hasText: 'Урок из другой вкладки' })).toHaveCount(1);
  });
});

test.describe('schedule for a tutor in English', () => {
  test.use({ locale: 'en-GB', timezoneId: moscow, viewport: { width: 1280, height: 800 } });

  test('schedule › the audience picker shows more students with focus on the first new one', async ({
    page,
    studentsOfPavel,
  }) => {
    await signInThroughTheApi(page, studentsOfPavel.pavel);
    await page.goto('/schedule');
    await page.getByRole('button', { name: 'New lesson' }).click();
    const dialog = page.getByRole('dialog', { name: 'New lesson' });
    const students = dialog.getByRole('list', { name: 'Students' }).getByRole('button');
    await expect(students).toHaveCount(30);

    await dialog.getByRole('button', { name: 'Show more' }).click();

    await expect(students).toHaveCount(31);
    await expect(students.nth(30)).toBeFocused();
    await expect(dialog.getByRole('button', { name: 'Show more' })).toHaveCount(0);
  });
});

interface PilotOneSeed {
  readonly maria: TestAccount;
  readonly anna: TestAccount;
  readonly firstLesson: number;
}

let pilotOneSeed: PilotOneSeed | null = null;

function pilotOne(): PilotOneSeed {
  if (pilotOneSeed === null) {
    throw new Error('pilot 1 step 6 has not run');
  }
  return pilotOneSeed;
}

test.describe('schedule for an unconfirmed student in English', () => {
  test.use({ locale: 'en-GB', timezoneId: moscow, viewport: { width: 1280, height: 800 } });

  test('unconfirmed student › Schedule shows the provisional link’s lessons', async ({ page, deployment, control }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const boris = newTestAccount('Boris', 'student');
    const card = await createCard(deployment, pavel, 'Boris', boris.email);
    await signUpOnInvitation(deployment, card.link, boris, 'en');
    const tomorrow = await moscowTomorrow(control.now);
    await scheduleLessons(deployment, pavel, {
      relationship_id: card.id,
      title: 'Lesson',
      date: tomorrow,
      start: '10:00',
      duration_minutes: 60,
    });
    await signInThroughTheApi(page, boris);

    await page.goto(weekAddress(tomorrow));

    await expect(page).toHaveURL(weekAddress(tomorrow));
    await expect(page.getByRole('navigation').getByRole('link', { name: 'Schedule' })).toBeVisible();
    const lessons = lessonLinks(page);
    await expect(lessons).toHaveCount(1);
    await expect(lessons).toContainText('10:00–11:00');
    await expect(lessons).toContainText('Individual lesson');
    await expect(lessons).toContainText('Pavel');
  });
});

test.describe('pilot 1, the first lessons of Maria and Anna', () => {
  test.describe.configure({ mode: 'serial' });
  test.use({ locale: 'ru-RU', timezoneId: moscow, viewport: { width: 1280, height: 800 } });

  test('pilot-1 › step 6 Maria schedules eight lessons on Tuesdays and Thursdays, which Anna sees in Berlin time', async ({
    page,
    observedBrowser: browser,
    deployment,
    control,
    diagnostics,
  }) => {
    const maria = await signUpAndConfirmTutor(deployment, 'Мария');
    const { student: anna } = await signUpActiveStudentOnInvitation(deployment, control, maria, 'Анна', 'ru');
    await setTimezone(deployment, anna, berlin);
    const firstDate = await moscowTomorrow(control.now);
    const lessonDates = seriesDates(firstDate, [2, 4], 8);
    await signInThroughTheApi(page, maria);
    await page.goto(`/schedule?view=month&date=${firstDate}`);
    const dialog = await openNewLesson(page);
    await dialog.getByRole('button', { name: 'Анна', exact: true }).click();
    await dialog.getByLabel('Дата', { exact: true }).fill(firstDate);
    await dialog.getByLabel('Время начала').fill('18:00');
    await dialog.getByRole('checkbox', { name: 'Повторять каждую неделю' }).check();
    await dialog.getByRole('checkbox', { name: 'вторник' }).check();
    await dialog.getByRole('checkbox', { name: 'четверг' }).check();
    await dialog.getByLabel('Число уроков').fill('8');

    await dialog.getByRole('button', { name: 'Запланировать' }).click();

    await expect(dialog).toBeHidden();
    await expect(page.getByRole('button', { name: 'Новый урок' })).toBeFocused();
    const addresses = new Set<string>();
    for (const month of [firstDate, lessonDates.at(-1) ?? firstDate]) {
      await page.goto(`/schedule?view=month&date=${month}`);
      await expect(lessonLinks(page).first()).toBeVisible();
      await expect(lessonLinks(page).filter({ hasNotText: '18:00–19:00' })).toHaveCount(0);
      for (const address of await lessonAddresses(page)) {
        addresses.add(address);
      }
    }
    expect(addresses.size).toBe(8);
    const annaContext = await phoneContext(browser, deployment, berlin);
    const annaPage = await annaContext.newPage();
    await diagnostics.watch(annaPage);
    await signInThroughTheApi(annaPage, anna);
    const firstLesson = moscowInstant(lessonDates[0] ?? firstDate, '18:00');
    await annaPage.goto(weekAddress(lessonDates[0] ?? firstDate));
    const annaLessons = lessonLinks(annaPage);
    await expect(annaLessons.first()).toBeVisible();
    const expectedClock = `${localClock(firstLesson, berlin)}–${localClock(firstLesson + 3_600_000, berlin)}`;
    await expect(annaLessons.first()).toContainText(expectedClock);
    await expect(annaLessons.first()).toContainText('Индивидуальный урок');
    await expect(annaLessons.first()).toContainText('Мария');
    await expect(annaLessons.filter({ hasText: 'Анна' })).toHaveCount(0);
    await annaLessons.first().click();
    await expect(
      annaPage
        .getByRole('main')
        .getByText(`18:00 по времени Москвы (${localClock(firstLesson, berlin)} по вашему)`)
        .first(),
    ).toBeVisible();
    await diagnostics.closeContext(annaContext);
    pilotOneSeed = { maria, anna, firstLesson };
  });

  test('pilot-1 › step 7 the one-hour reminders reach Maria and Anna in Russian, Anna’s in Berlin time', async ({
    control,
  }) => {
    const { maria, anna, firstLesson } = pilotOne();
    const now = await control.now();
    await control.advance(Math.ceil((firstLesson - 3_600_000 - now) / 1000));

    const reminders = async (email: string): Promise<string[]> => {
      await control.tick();
      return (await control.mail(email))
        .filter((mail) => mail.subject.startsWith('Урок через 1 час'))
        .map((mail) => mail.text);
    };

    await expect.poll(async () => (await reminders(maria.email)).length).toBe(1);
    await expect.poll(async () => (await reminders(anna.email)).length).toBe(1);
    const [mariaText] = await reminders(maria.email);
    const [annaText] = await reminders(anna.email);
    expect(mariaText).toContain(localClock(firstLesson, moscow));
    expect(annaText).toContain(localClock(firstLesson, berlin));
  });
});

interface PilotThreeSeed {
  readonly maria: TestAccount;
  readonly anna: TestAccount;
  readonly firstWednesday: LocalDate;
}

let pilotThreeSeed: PilotThreeSeed | null = null;
let pavelOfPilotThree: TestAccount | null = null;
let pavelCardOfPilotThree: string | null = null;
let movedLessonOfPilotThree: number | null = null;

function pilotThree(): PilotThreeSeed {
  if (pilotThreeSeed === null) {
    throw new Error('the pilot 3 seed has not run');
  }
  return pilotThreeSeed;
}

function pavelMonday(seed: PilotThreeSeed): LocalDate {
  return addDays(seed.firstWednesday, 5);
}

test.describe('pilot 3, two tutors in Anna’s schedule', () => {
  test.describe.configure({ mode: 'serial' });

  test('pilot-3 › seed Maria teaches Anna on Wednesdays, Boris, and the group B1 evening', async ({
    deployment,
    control,
  }) => {
    const maria = await signUpAndConfirmTutor(deployment, 'Мария');
    const anna = await signUpActiveStudentOnInvitation(deployment, control, maria, 'Анна', 'ru');
    const boris = await signUpActiveStudentOnInvitation(deployment, control, maria, 'Boris', 'en');
    await setTimezone(deployment, anna.student, berlin);
    await createGroup(deployment, maria, 'B1 evening', [anna.cardId, boris.cardId]);
    const firstWednesday = nextWeekday(await moscowTomorrow(control.now), [3]);

    const starts = await scheduleLessons(deployment, maria, {
      relationship_id: anna.cardId,
      title: 'Урок',
      date: firstWednesday,
      start: '19:00',
      duration_minutes: 60,
      weekdays: [3],
      count: 4,
    });

    expect(starts).toHaveLength(4);
    pilotThreeSeed = { maria, anna: anna.student, firstWednesday };
  });

  test('pilot-3 › step 3 Pavel schedules a Monday lesson at 10:00 his time for Anna', async ({
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const seed = pilotThree();
    const pavel = await signUpAndConfirmTutor(deployment, 'Pavel', 'en');
    await setTimezone(deployment, pavel, yekaterinburg);
    const card = await createCard(deployment, pavel, 'Anna', seed.anna.email);
    await acceptSignedIn(deployment, seed.anna, card.link);
    const context = await englishContext(browser, deployment, yekaterinburg);
    const page = await context.newPage();
    await diagnostics.watch(page);
    await signInThroughTheApi(page, pavel);
    const monday = pavelMonday(seed);
    await page.goto(weekAddress(monday));

    await page.locator(`[data-slot="${monday}T10:00"]`).click();
    const dialog = page.getByRole('dialog', { name: 'New lesson' });
    await dialog.getByRole('button', { name: 'Anna', exact: true }).click();
    await dialog.getByRole('button', { name: 'Schedule', exact: true }).click();

    await expect(dialog).toBeHidden();
    await expect(lessonLinks(page).filter({ hasText: '10:00–11:00' })).toContainText('Lesson');
    await diagnostics.closeContext(context);
    pavelOfPilotThree = pavel;
    pavelCardOfPilotThree = card.id;
  });

  test('pilot-3 › step 4 Anna’s Schedule shows both tutors with their labels, and the filter leaves Pavel’s', async ({
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const seed = pilotThree();
    if (pavelOfPilotThree === null) {
      throw new Error('pilot 3 step 3 has not run');
    }
    const monday = pavelMonday(seed);
    const pavelLesson = Date.parse(`${monday}T10:00:00+05:00`);
    const mariaLesson = moscowInstant(addDays(seed.firstWednesday, 7), '19:00');
    const context = await phoneContext(browser, deployment, berlin);
    const page = await context.newPage();
    await diagnostics.watch(page);
    await signInThroughTheApi(page, seed.anna);

    await page.goto(weekAddress(monday));

    const lessons = lessonLinks(page);
    await expect(lessons).toHaveCount(2);
    await expect(lessons.filter({ hasText: 'Pavel' })).toContainText(localClock(pavelLesson, berlin));
    await expect(lessons.filter({ hasText: 'Мария' })).toContainText(localClock(mariaLesson, berlin));
    await page.getByLabel('Репетитор').selectOption({ label: 'Pavel' });
    await expect(page).toHaveURL(/tutor=/);
    await expect(lessons).toHaveCount(1);
    await expect(lessons).toContainText('Pavel');
    await expect(lessons).toContainText('Индивидуальный урок');
    await diagnostics.closeContext(context);
  });

  test('pilot-3 › step 5 Pavel’s card for Anna lists only his lesson with her, none of Maria’s', async ({
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    if (pavelOfPilotThree === null || pavelCardOfPilotThree === null) {
      throw new Error('pilot 3 step 3 has not run');
    }
    const context = await englishContext(browser, deployment, yekaterinburg);
    const page = await context.newPage();
    await diagnostics.watch(page);
    await signInThroughTheApi(page, pavelOfPilotThree);

    await page.goto(`/students/${pavelCardOfPilotThree}?tab=lessons`);

    const history = page.getByRole('list', { name: 'Lessons', exact: true }).getByRole('listitem');
    await expect(history).toHaveCount(1);
    await expect(history).toContainText('10:00–11:00');
    await expect(history).toContainText('Lesson');
    await page.getByRole('link', { name: 'Overview' }).click();
    const next = page.getByRole('list', { name: 'Next lessons', exact: true }).getByRole('listitem');
    await expect(next).toHaveCount(1);
    await expect(next).toContainText('10:00–11:00');
    await diagnostics.closeContext(context);
  });

  test('pilot-3 › step 12 Maria moves the next Wednesday lesson to 20:00, and Anna’s email says 18:00 her time', async ({
    observedBrowser: browser,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = pilotThree();
    const context = await browser.newContext({
      baseURL: deployment.origin,
      ignoreHTTPSErrors: true,
      locale: 'ru-RU',
      timezoneId: moscow,
      viewport: { width: 1280, height: 800 },
      colorScheme: 'light',
    });
    const page = await context.newPage();
    await diagnostics.watch(page);
    await signInThroughTheApi(page, seed.maria);
    await page.goto(weekAddress(seed.firstWednesday));
    await lessonLinks(page).filter({ hasText: '19:00–20:00' }).first().click();
    await page.getByRole('button', { name: 'Изменить урок' }).click();
    const dialog = page.getByRole('dialog', { name: 'Изменение урока' });
    await expect(dialog.getByRole('radio', { name: 'Этот урок' })).toBeChecked();
    await dialog.getByLabel('Время начала').fill('20:00');

    await dialog.getByRole('button', { name: 'Сохранить' }).click();

    await expect(dialog).toBeHidden();
    await expect(page.getByRole('main').getByText('20:00–21:00').first()).toBeVisible();
    const moved = moscowInstant(seed.firstWednesday, '20:00');
    const movedEmails = async (): Promise<string[]> => {
      await control.tick();
      return (await control.mail(seed.anna.email))
        .filter((mail) => mail.subject.startsWith('Урок перенесён'))
        .map((mail) => mail.text);
    };
    await expect.poll(async () => (await movedEmails()).length).toBe(1);
    const [movedText] = await movedEmails();
    expect(movedText).toContain(localClock(moved, berlin));
    await diagnostics.closeContext(context);
    movedLessonOfPilotThree = moved;
  });

  test('pilot-3 › step 13 the reminder uses the new time, and none is sent for the old one', async ({ control }) => {
    const seed = pilotThree();
    if (movedLessonOfPilotThree === null) {
      throw new Error('pilot 3 step 12 has not run');
    }
    const moved = movedLessonOfPilotThree;
    const oldStart = moscowInstant(seed.firstWednesday, '19:00');
    const now = await control.now();
    await control.advance(Math.ceil((moved - 3_600_000 - now) / 1000));

    const reminders = async (): Promise<string[]> => {
      await control.tick();
      return (await control.mail(seed.anna.email))
        .filter((mail) => mail.subject.startsWith('Урок через 1 час'))
        .map((mail) => mail.text);
    };

    await expect.poll(async () => (await reminders()).length).toBe(1);
    const [reminder] = await reminders();
    expect(reminder).toContain(localClock(moved, berlin));
    expect(reminder).not.toContain(localClock(oldStart, berlin));
  });
});
