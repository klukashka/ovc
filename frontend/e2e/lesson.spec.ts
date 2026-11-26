import type { Locator, Page } from '@playwright/test';

import { type TestAccount, seedAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import {
  attachLessonMaterials,
  createLibraryFolder,
  createLibraryMaterial,
  deleteLibraryMaterial,
  lessonMaterialTitles,
  scheduledCardLessonIds,
} from './materials.ts';
import { expectRefetchedAfterTheSocketIsReady, trafficAfterReady } from './realtime.ts';
import {
  type LessonRequest,
  type LocalDate,
  type ScheduledLesson,
  addDays,
  cancelThisLesson,
  cardLessonStatuses,
  editThisLesson,
  isoWeekday,
  localClock,
  localDate,
  readLesson,
  scheduleLessonsWithIds,
  scheduledCardLessonStarts,
  seriesDates,
  setTimezone,
} from './schedule.ts';
import { acceptSignedIn, accessToken, callApi, createCard, createGroup, endRelationship } from './tutoring.ts';
import { expectDangerConfirmation } from './dangerActions.ts';

const moscow = 'Europe/Moscow';
const hour = 3_600_000;

interface TutorAndStudent {
  readonly maria: TestAccount;
  readonly anna: TestAccount;
  readonly cardId: string;
}

async function tutorAndStudent(deployment: Deployment, cardName = 'Анна'): Promise<TutorAndStudent> {
  const maria = await seedVerifiedUser(deployment, 'Мария');
  const anna = await seedAccount(deployment, 'Анна Петрова', 'student', 'ru');
  const card = await createCard(deployment, maria, cardName, anna.email);
  await acceptSignedIn(deployment, anna, card.link);
  return { maria, anna, cardId: card.id };
}

async function tomorrow(now: () => Promise<number>): Promise<LocalDate> {
  return addDays(localDate(await now(), moscow), 1);
}

async function oneLesson(
  deployment: Deployment,
  seed: TutorAndStudent,
  date: LocalDate,
  start = '18:00',
): Promise<ScheduledLesson> {
  const [lesson] = await scheduleLessonsWithIds(deployment, seed.maria, {
    relationship_id: seed.cardId,
    title: 'Разговорная практика',
    date,
    start,
    duration_minutes: 60,
  });
  if (lesson === undefined) {
    throw new Error('no lesson was scheduled');
  }
  return lesson;
}

function lessonAddress(lesson: ScheduledLesson): string {
  return `/lessons/${lesson.id}`;
}

async function lessonInTheJoinWindow(
  deployment: Deployment,
  seed: TutorAndStudent,
  now: () => Promise<number>,
): Promise<ScheduledLesson> {
  const soon = (await now()) + 5 * 60_000;
  return oneLesson(deployment, seed, localDate(soon, moscow), localClock(soon, moscow));
}

async function tutorJoinsThroughTheApi(
  deployment: Deployment,
  seed: TutorAndStudent,
  lesson: ScheduledLesson,
): Promise<void> {
  const joined = await callApi(
    deployment,
    await accessToken(deployment, seed.maria),
    'POST',
    `/lessons/${lesson.id}/join`,
  );
  if (joined.status !== 200) {
    throw new Error('the tutor could not join through the API');
  }
}

async function openOnTheSharedClockAt1440(
  page: Page,
  account: TestAccount,
  lesson: ScheduledLesson,
  now: () => Promise<number>,
): Promise<void> {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.clock.install({ time: await now() });
  await signInThroughTheApi(page, account);
  await page.goto(lessonAddress(lesson));
}

function firstOfTheMainColumn(page: Page): Locator {
  return page.locator('[data-column="main"] > *').first();
}

function editButton(page: Page): Locator {
  return page.getByRole('button', { name: 'Изменить урок' });
}

function lessonActions(page: Page): Locator {
  return page.getByRole('button', { name: 'Действия с уроком' });
}

function cancelItem(page: Page): Locator {
  return page.getByRole('button', { name: 'Отменить урок' });
}

async function openCancel(page: Page): Promise<void> {
  await lessonActions(page).click();
  await cancelItem(page).click();
}

function editDialog(page: Page): Locator {
  return page.getByRole('dialog', { name: 'Изменение урока' });
}

function cancelDialog(page: Page): Locator {
  return page.getByRole('dialog', { name: 'Отменить урок?' });
}

async function dailySeries(
  deployment: Deployment,
  seed: TutorAndStudent,
  date: LocalDate,
  count: number,
): Promise<readonly [ScheduledLesson, ScheduledLesson, ...ScheduledLesson[]]> {
  const lessons = await scheduleLessonsWithIds(deployment, seed.maria, {
    relationship_id: seed.cardId,
    title: 'Серия',
    date,
    start: '18:00',
    duration_minutes: 60,
    weekdays: [1, 2, 3, 4, 5, 6, 7],
    count,
  });
  const [first, second, ...rest] = lessons;
  if (first === undefined || second === undefined) {
    throw new Error('the series has fewer than two lessons');
  }
  return [first, second, ...rest];
}

type SeriesAudience = Pick<LessonRequest, 'relationship_id' | 'group_id'>;

interface WeeklySeries {
  readonly lessons: readonly ScheduledLesson[];
  readonly weekdays: readonly number[];
  readonly secondDate: LocalDate;
}

async function weeklySeries(
  deployment: Deployment,
  seed: TutorAndStudent,
  audience: SeriesAudience,
  first: LocalDate,
  start = '18:00',
): Promise<WeeklySeries> {
  const secondDate = addDays(first, 2);
  const weekdays = [isoWeekday(first), isoWeekday(secondDate)].sort((left, right) => left - right);
  const lessons = await scheduleLessonsWithIds(deployment, seed.maria, {
    ...audience,
    title: 'Серия',
    date: first,
    start,
    duration_minutes: 90,
    weekdays,
    count: 4,
  });
  return { lessons, weekdays, secondDate };
}

interface SeriesUntilItsEndDate {
  readonly lessons: readonly ScheduledLesson[];
  readonly endDate: LocalDate;
}

async function weeklySeriesUntilItsEndDate(
  deployment: Deployment,
  seed: TutorAndStudent,
  first: LocalDate,
): Promise<SeriesUntilItsEndDate> {
  const weekdays = [isoWeekday(first), isoWeekday(addDays(first, 2))].sort((left, right) => left - right);
  const endDate = seriesDates(first, weekdays, 4).at(-1);
  if (endDate === undefined) {
    throw new Error('the series has no dates');
  }
  const lessons = await scheduleLessonsWithIds(deployment, seed.maria, {
    relationship_id: seed.cardId,
    title: 'Серия',
    date: first,
    start: '18:00',
    duration_minutes: 90,
    weekdays,
    end_date: endDate,
  });
  if (lessons.length !== 4) {
    throw new Error('the series does not hold four lessons');
  }
  return { lessons, endDate };
}

function secondLessonOf(series: Pick<WeeklySeries, 'lessons'>): ScheduledLesson {
  const [, second] = series.lessons;
  if (second === undefined) {
    throw new Error('the series has fewer than two lessons');
  }
  return second;
}

function endSeriesItem(page: Page): Locator {
  return page.getByRole('button', { name: 'Завершить эту серию и создать новую' });
}

async function openEndSeries(page: Page): Promise<void> {
  await lessonActions(page).click();
  await endSeriesItem(page).click();
}

function endSeriesDialog(page: Page): Locator {
  return page.getByRole('dialog', { name: 'Завершить эту серию и создать новую?' });
}

function newLessonDialog(page: Page): Locator {
  return page.getByRole('main');
}

function newLessonHeading(page: Page): Locator {
  return page.getByRole('heading', { level: 1, name: 'Новый урок' });
}

async function checkedWeekdays(dialog: Locator): Promise<number[]> {
  return dialog
    .getByRole('group', { name: 'Дни недели' })
    .getByRole('checkbox', { checked: true })
    .evaluateAll((boxes) => boxes.map((box) => Number(box.getAttribute('value'))));
}

interface EndedSeries {
  readonly seed: TutorAndStudent;
  readonly series: WeeklySeries;
}

async function endedSeriesWithNewLessonOpen(
  page: Page,
  deployment: Deployment,
  now: () => Promise<number>,
): Promise<EndedSeries> {
  const seed = await tutorAndStudent(deployment);
  const series = await weeklySeries(deployment, seed, { relationship_id: seed.cardId }, await tomorrow(now));
  await signInThroughTheApi(page, seed.maria);
  await page.goto(lessonAddress(secondLessonOf(series)));
  await openEndSeries(page);
  await endSeriesDialog(page).getByRole('button', { name: 'Завершить серию' }).click();
  await newLessonDialog(page).getByRole('button', { name: 'Изменить', exact: true }).waitFor();
  return { seed, series };
}

const cancelledNotice = 'Урок отменён';
const changedMeanwhile = 'Этот урок тем временем изменился. Ниже показано его текущее состояние.';
const audienceGone = 'Этот ученик или группа больше недоступны для уроков. Выберите другого.';

async function endSeriesInReykjavik(
  page: Page,
  deployment: Deployment,
  now: () => Promise<number>,
): Promise<EndedSeries> {
  const seed = await tutorAndStudent(deployment);
  const series = await weeklySeries(deployment, seed, { relationship_id: seed.cardId }, await tomorrow(now), '01:00');
  await setTimezone(deployment, seed.maria, 'Atlantic/Reykjavik');
  await signInThroughTheApi(page, seed.maria);
  await page.goto(lessonAddress(secondLessonOf(series)));
  await openEndSeries(page);
  return { seed, series };
}

function weekdayBefore(weekday: number): number {
  return weekday === 1 ? 7 : weekday - 1;
}

async function refusedNewSeries(page: Page, deployment: Deployment, now: () => Promise<number>): Promise<EndedSeries> {
  const ended = await endedSeriesWithNewLessonOpen(page, deployment, now);
  await endRelationship(deployment, ended.seed.maria, ended.seed.cardId);
  return ended;
}

interface FocusReturn {
  readonly name: string;
  readonly ofSeries?: boolean;
  readonly open: (page: Page) => Promise<void>;
  readonly opener: (page: Page) => Locator;
  readonly close: (page: Page) => Promise<void>;
}

async function openEdit(page: Page): Promise<void> {
  await editButton(page).click();
}

const focusReturns: readonly FocusReturn[] = [
  {
    name: 'edit dismissed with Escape',
    open: openEdit,
    opener: editButton,
    close: async (page): Promise<void> => {
      await page.keyboard.press('Escape');
    },
  },
  {
    name: 'cancellation dismissed from the lesson actions menu',
    open: openCancel,
    opener: lessonActions,
    close: async (page): Promise<void> => {
      await cancelDialog(page).getByRole('button', { name: 'Отмена', exact: true }).click();
    },
  },
  {
    name: 'series end dismissed from the lesson actions menu',
    ofSeries: true,
    open: openEndSeries,
    opener: lessonActions,
    close: async (page): Promise<void> => {
      await endSeriesDialog(page).getByRole('button', { name: 'Отмена', exact: true }).click();
    },
  },
  {
    name: 'edit saved',
    open: openEdit,
    opener: editButton,
    close: async (page): Promise<void> => {
      await editDialog(page).getByLabel('Время начала').fill('18:30');
      await editDialog(page).getByRole('button', { name: 'Сохранить' }).click();
    },
  },
];

test.describe('lesson page for a tutor in Russian', () => {
  test.use({ locale: 'ru-RU', timezoneId: moscow, viewport: { width: 1280, height: 800 } });

  test('lesson › editing a series occurrence asks for the scope', async ({ page, deployment, control }) => {
    const seed = await tutorAndStudent(deployment);
    const [, selected] = await dailySeries(deployment, seed, await tomorrow(control.now), 3);
    await signInThroughTheApi(page, seed.maria);
    await page.goto(lessonAddress(selected));

    await editButton(page).click();

    const scope = editDialog(page).getByRole('group', { name: 'Какие уроки изменить' });
    await expect(scope.getByRole('radio')).toHaveCount(2);
    await expect(scope.getByRole('radio', { name: 'Этот урок' })).toBeFocused();
    await expect(scope.getByRole('radio', { name: 'Этот урок' })).toBeChecked();
    await expect(scope.getByRole('radio', { name: 'Этот и следующие уроки' })).not.toBeChecked();
    await expect(editDialog(page).getByLabel('Дата', { exact: true })).toBeVisible();
  });

  test('lesson › this and following lessons offers no date and moves the following lessons', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lessons = await dailySeries(deployment, seed, await tomorrow(control.now), 3);
    await signInThroughTheApi(page, seed.maria);
    await page.goto(lessonAddress(lessons[1]));
    await editButton(page).click();
    const dialog = editDialog(page);

    await dialog.getByRole('radio', { name: 'Этот и следующие уроки' }).check();
    await dialog.getByLabel('Дата', { exact: true }).waitFor({ state: 'detached' });
    await dialog.getByLabel('Время начала').fill('19:00');
    await dialog.getByRole('button', { name: 'Сохранить' }).click();

    await expect(dialog).toBeHidden();
    await expect(editButton(page)).toBeFocused();
    await expect(page.getByRole('status').filter({ hasText: 'Урок изменён' })).toBeVisible();
    const starts = await Promise.all(
      lessons.map(async (lesson) => (await readLesson(deployment, seed.maria, lesson.id)).startsAt),
    );
    expect(starts.map((start) => localClock(Date.parse(start), moscow))).toEqual(['18:00', '19:00', '19:00']);
  });

  for (const { name, close, open, opener, ofSeries = false } of focusReturns) {
    test(`lesson › a saved edit and a dismissed dialog return focus to the opening control (${name})`, async ({
      page,
      deployment,
      control,
    }) => {
      const seed = await tutorAndStudent(deployment);
      const date = await tomorrow(control.now);
      const [lesson] = ofSeries
        ? await dailySeries(deployment, seed, date, 2)
        : [await oneLesson(deployment, seed, date)];
      await signInThroughTheApi(page, seed.maria);
      await page.goto(lessonAddress(lesson));
      await open(page);
      await page.getByRole('dialog').waitFor();

      await close(page);

      await expect(page.getByRole('dialog')).toHaveCount(0);
      await expect(opener(page)).toBeFocused();
    });
  }

  test('lesson › at 1440 × 900 the join block is the first content of the main column and in view for a tutor in the join window', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await lessonInTheJoinWindow(deployment, seed, control.now);

    await openOnTheSharedClockAt1440(page, seed.maria, lesson, control.now);

    const joinBlock = firstOfTheMainColumn(page);
    const phase = joinBlock.getByRole('status').filter({ hasText: 'Скоро начало' });
    await expect(phase).toBeInViewport({ ratio: 1 });
    await expect(joinBlock.getByRole('region', { name: 'Видеокомната' })).toBeVisible();
    await expect(joinBlock.getByRole('button', { name: 'Подключиться к уроку', exact: true })).toBeInViewport({
      ratio: 1,
    });
  });

  test('lesson › the date and time appear once on the page', async ({ page, deployment, control }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    await signInThroughTheApi(page, seed.maria);

    await page.goto(lessonAddress(lesson));

    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Разговорная практика');
    await expect(page.getByRole('main').getByText('18:00–19:00')).toHaveCount(1);
    await expect(page.getByRole('complementary', { name: 'Об уроке' }).getByText('18:00–19:00')).toBeVisible();
  });

  test('lesson › a cancel dialog opened from the lesson actions menu closes onto the notice when the lesson is cancelled elsewhere', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    await signInThroughTheApi(page, seed.maria);
    const traffic = trafficAfterReady(page, [`/api/v1/lessons/${lesson.id}`]);
    await traffic.initialize();
    await page.goto(lessonAddress(lesson));
    await expectRefetchedAfterTheSocketIsReady(traffic);
    await openCancel(page);
    await cancelDialog(page).getByRole('button', { name: 'Отмена', exact: true }).waitFor();

    await cancelThisLesson(deployment, seed.maria, lesson.id);

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: changedMeanwhile })).toBeFocused();
    await expect(lessonActions(page)).toHaveCount(0);
    await expect(editButton(page)).toHaveCount(0);
  });

  test('lesson › an edit dialog of a one-off lesson opens on its title', async ({ page, deployment, control }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    await signInThroughTheApi(page, seed.maria);
    await page.goto(lessonAddress(lesson));

    await editButton(page).click();

    await expect(editDialog(page).getByLabel('Название')).toBeFocused();
    await expect(editDialog(page).getByRole('radio')).toHaveCount(0);
    await expect(editDialog(page).getByLabel('Время начала')).toHaveValue('18:00');
  });

  test('lesson › cancelling opens on the dismiss button', async ({ page, deployment, control }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    await signInThroughTheApi(page, seed.maria);
    await page.goto(lessonAddress(lesson));

    await openCancel(page);

    const dialog = cancelDialog(page);
    await expectDangerConfirmation(dialog, 'Отменить урок', 'Отмена');
    await expect(dialog.getByRole('radio')).toHaveCount(0);
    await expect(dialog).toHaveAccessibleDescription(
      'Урок будет отменён для всех и останется в истории как отменённый. Ученики получат уведомление. Это нельзя отменить.',
    );
  });

  test('lesson › a confirmed cancellation shows the cancelled lesson and its notice', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    await signInThroughTheApi(page, seed.maria);
    await page.goto(lessonAddress(lesson));
    await openCancel(page);

    await cancelDialog(page).getByRole('button', { name: 'Отменить урок' }).click();

    await expect(cancelDialog(page)).toBeHidden();
    await expect(page.getByRole('status').filter({ hasText: 'Урок отменён' })).toBeFocused();
    await expect(
      page
        .getByRole('main')
        .getByText(/^Отменён /)
        .first(),
    ).toBeVisible();
    await expect(editButton(page)).toHaveCount(0);
    await expect(lessonActions(page)).toHaveCount(0);
    expect((await readLesson(deployment, seed.maria, lesson.id)).status).toBe('cancelled');
  });

  test('lesson › ending a series opens its confirmation on the dismiss button', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const series = await weeklySeries(deployment, seed, { relationship_id: seed.cardId }, await tomorrow(control.now));
    await signInThroughTheApi(page, seed.maria);
    await page.goto(lessonAddress(secondLessonOf(series)));

    await openEndSeries(page);

    const dialog = endSeriesDialog(page);
    await expectDangerConfirmation(dialog, 'Завершить серию', 'Отмена');
    await expect(dialog.getByRole('radio')).toHaveCount(0);
    await expect(dialog).toHaveAccessibleDescription(
      'Этот и все следующие уроки серии будут отменены, а серия завершится; каждый затронутый ученик получит одно уведомление. Затем откроется новый урок с тем же учеником или группой, временем и днями недели.',
    );
  });

  test('lesson › ending a series from an occurrence moved to an earlier date cancels this and the following lessons and opens New lesson with the same student, time and weekdays on its original date', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const first = await tomorrow(control.now);
    const series = await weeklySeries(deployment, seed, { relationship_id: seed.cardId }, first);
    const selected = secondLessonOf(series);
    await editThisLesson(deployment, seed.maria, selected.id, { date: addDays(first, 1) });
    await signInThroughTheApi(page, seed.maria);
    await page.goto(lessonAddress(selected));
    await openEndSeries(page);

    await endSeriesDialog(page).getByRole('button', { name: 'Завершить серию' }).click();

    const dialog = newLessonDialog(page);
    await expect(newLessonHeading(page)).toBeFocused();
    await expect(page.getByText(changedMeanwhile)).toHaveCount(0);
    await expect(dialog.getByRole('button', { name: 'Изменить', exact: true })).toHaveAccessibleDescription('Анна');
    await expect(dialog.getByLabel('Дата', { exact: true })).toHaveValue(series.secondDate);
    await expect(dialog.getByLabel('Время начала')).toHaveValue('18:00');
    await expect(dialog.getByLabel('Длительность, минут')).toHaveValue('90');
    await expect(dialog.getByRole('checkbox', { name: 'Повторять каждую неделю' })).toBeChecked();
    expect(await checkedWeekdays(dialog)).toEqual(series.weekdays);
    const statuses = await Promise.all(
      series.lessons.map(async (lesson) => (await readLesson(deployment, seed.maria, lesson.id)).status),
    );
    expect(statuses).toEqual(['scheduled', 'cancelled', 'cancelled', 'cancelled']);
  });

  test('lesson › ending a group series opens New lesson with the same group and weekdays', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const groupId = await createGroup(deployment, seed.maria, 'Вечерняя группа', [seed.cardId]);
    const series = await weeklySeries(deployment, seed, { group_id: groupId }, await tomorrow(control.now));
    await signInThroughTheApi(page, seed.maria);
    await page.goto(lessonAddress(secondLessonOf(series)));
    await openEndSeries(page);

    await endSeriesDialog(page).getByRole('button', { name: 'Завершить серию' }).click();

    const dialog = newLessonDialog(page);
    await expect(newLessonHeading(page)).toBeFocused();
    await expect(dialog.getByRole('button', { name: 'Изменить', exact: true })).toHaveAccessibleDescription(
      'Вечерняя группа',
    );
    await expect(dialog.getByLabel('Название')).toHaveValue('Урок Вечерняя группа');
    await expect(dialog.getByLabel('Время начала')).toHaveValue('18:00');
    expect(await checkedWeekdays(dialog)).toEqual(series.weekdays);
  });

  test('lesson › the new series saved after ending one closes onto the cancelled notice and is announced', async ({
    page,
    deployment,
    control,
  }) => {
    const { seed } = await endedSeriesWithNewLessonOpen(page, deployment, control.now);

    await newLessonDialog(page).getByRole('button', { name: 'Создать урок' }).click();

    await expect(newLessonHeading(page)).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: cancelledNotice })).toBeFocused();
    await expect(page.getByRole('status').filter({ hasText: 'Урок запланирован' })).toBeVisible();
    await expect(lessonActions(page)).toHaveCount(0);
    const statuses = await cardLessonStatuses(deployment, seed.maria, seed.cardId);
    expect(statuses.filter((status) => status === 'scheduled')).toHaveLength(9);
  });

  test('lesson › ending a series in another timezone prefills the same instants in the profile timezone', async ({
    page,
    deployment,
    control,
  }) => {
    const { series } = await endSeriesInReykjavik(page, deployment, control.now);

    await endSeriesDialog(page).getByRole('button', { name: 'Завершить серию' }).click();

    const dialog = newLessonDialog(page);
    await expect(dialog.getByLabel('Дата', { exact: true })).toHaveValue(addDays(series.secondDate, -1));
    await expect(dialog.getByLabel('Время начала')).toHaveValue('22:00');
    expect(await checkedWeekdays(dialog)).toEqual(
      series.weekdays.map(weekdayBefore).sort((left, right) => left - right),
    );
  });

  test('lesson › a new series saved in another timezone starts at the instants of the cancelled lessons', async ({
    page,
    deployment,
    control,
  }) => {
    const { seed, series } = await endSeriesInReykjavik(page, deployment, control.now);
    await endSeriesDialog(page).getByRole('button', { name: 'Завершить серию' }).click();
    await newLessonDialog(page).getByRole('button', { name: 'Изменить', exact: true }).waitFor();

    await newLessonDialog(page).getByRole('button', { name: 'Создать урок' }).click();

    await expect(newLessonHeading(page)).toHaveCount(0);
    const starts = await scheduledCardLessonStarts(deployment, seed.maria, seed.cardId);
    const cancelledStarts = series.lessons.slice(1).map((lesson) => lesson.startsAt);
    expect(starts).toHaveLength(9);
    expect(starts).toEqual(expect.arrayContaining(cancelledStarts));
  });

  test('lesson › ending a series from an occurrence moved beyond its end and cancelled meanwhile closes onto the changed notice and keeps the series', async ({
    page,
    deployment,
    control,
    socketSlots,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const series = await weeklySeriesUntilItsEndDate(deployment, seed, await tomorrow(control.now));
    const selected = secondLessonOf(series);
    await editThisLesson(deployment, seed.maria, selected.id, { date: addDays(series.endDate, 2) });
    await socketSlots.keepPageSocketsOff(seed.maria);
    await signInThroughTheApi(page, seed.maria);
    await page.goto(lessonAddress(selected));
    await openEndSeries(page);
    await cancelThisLesson(deployment, seed.maria, selected.id);

    await endSeriesDialog(page).getByRole('button', { name: 'Завершить серию' }).click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: changedMeanwhile })).toBeFocused();
    await expect(newLessonHeading(page)).toHaveCount(0);
    const statuses = await Promise.all(
      series.lessons.map(async (lesson) => (await readLesson(deployment, seed.maria, lesson.id)).status),
    );
    expect(statuses).toEqual(['scheduled', 'cancelled', 'scheduled', 'scheduled']);
  });

  test('lesson › ending a series refused because the lesson changed closes onto the changed notice', async ({
    page,
    deployment,
    control,
    diagnostics,
    socketSlots,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const series = await weeklySeries(deployment, seed, { relationship_id: seed.cardId }, await tomorrow(control.now));
    const selected = secondLessonOf(series);
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/lessons/${selected.id}/cancel`, status: 409 });
    await socketSlots.keepPageSocketsOff(seed.maria);
    await signInThroughTheApi(page, seed.maria);
    await page.goto(lessonAddress(selected));
    await openEndSeries(page);
    await editThisLesson(deployment, seed.maria, selected.id, { title: 'Изменено в другой вкладке' });

    await endSeriesDialog(page).getByRole('button', { name: 'Завершить серию' }).click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: changedMeanwhile })).toBeFocused();
    expect((await readLesson(deployment, seed.maria, selected.id)).status).toBe('scheduled');
  });

  test('lesson › a one-off lesson offers no series ending', async ({ page, deployment, control }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    await signInThroughTheApi(page, seed.maria);
    await page.goto(lessonAddress(lesson));

    await lessonActions(page).click();

    await expect(editButton(page)).toBeVisible();
    await expect(cancelItem(page)).toBeVisible();
    await expect(endSeriesItem(page)).toHaveCount(0);
  });

  test('lesson › a new series refused after the ending shows the refusal on the New lesson page', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/lessons', status: 409 });
    await refusedNewSeries(page, deployment, control.now);

    await newLessonDialog(page).getByRole('button', { name: 'Создать урок' }).click();

    await expect(newLessonDialog(page).getByText(audienceGone)).toBeVisible();
    await expect(newLessonHeading(page)).toBeVisible();
  });

  test('lesson › New lesson dismissed with its Cancel button after a refusal closes onto the cancelled notice', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/lessons', status: 409 });
    await refusedNewSeries(page, deployment, control.now);
    await newLessonDialog(page).getByRole('button', { name: 'Создать урок' }).click();
    await newLessonDialog(page).getByText(audienceGone).waitFor();

    await newLessonDialog(page).getByRole('button', { name: 'Отмена', exact: true }).click();

    await expect(newLessonHeading(page)).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: cancelledNotice })).toBeFocused();
  });

  test('lesson › New lesson left with Back after ending a series returns onto the cancelled notice', async ({
    page,
    deployment,
    control,
  }) => {
    await endedSeriesWithNewLessonOpen(page, deployment, control.now);

    await page.goBack();

    await expect(newLessonHeading(page)).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: cancelledNotice })).toBeFocused();
    await expect(page.getByRole('status').filter({ hasText: 'Урок запланирован' })).toHaveCount(0);
  });

  test('lesson › a stale edit closes onto the notice', async ({
    page,
    deployment,
    control,
    diagnostics,
    socketSlots,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    diagnostics.expectFailure({ method: 'PATCH', path: `/api/v1/lessons/${lesson.id}`, status: 409 });
    await socketSlots.keepPageSocketsOff(seed.maria);
    await signInThroughTheApi(page, seed.maria);
    await page.goto(lessonAddress(lesson));
    await editButton(page).click();
    await editDialog(page).getByLabel('Время начала').fill('19:00');
    await editThisLesson(deployment, seed.maria, lesson.id, { title: 'Изменено в другой вкладке' });

    await editDialog(page).getByRole('button', { name: 'Сохранить' }).click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(
      page
        .getByRole('status')
        .filter({ hasText: 'Этот урок тем временем изменился. Ниже показано его текущее состояние.' }),
    ).toBeFocused();
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Изменено в другой вкладке');
    await expect(page.getByRole('main').getByText('18:00–19:00').first()).toBeVisible();
  });

  test('lesson › a lesson changed elsewhere closes the open dialog onto the notice', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    await signInThroughTheApi(page, seed.maria);
    const traffic = trafficAfterReady(page, [`/api/v1/lessons/${lesson.id}`]);
    await traffic.initialize();
    await page.goto(lessonAddress(lesson));
    await expectRefetchedAfterTheSocketIsReady(traffic);
    await editButton(page).click();
    await editDialog(page).getByLabel('Название').waitFor();

    await editThisLesson(deployment, seed.maria, lesson.id, { title: 'Изменено в другой вкладке' });

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(
      page
        .getByRole('status')
        .filter({ hasText: 'Этот урок тем временем изменился. Ниже показано его текущее состояние.' }),
    ).toBeFocused();
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Изменено в другой вкладке');
  });

  test('lesson › a past lesson shows Did not take place', async ({ page, deployment, control }) => {
    const seed = await tutorAndStudent(deployment);
    const soon = (await control.now()) + 10 * 60_000;
    const lesson = await oneLesson(deployment, seed, localDate(soon, moscow), localClock(soon, moscow));
    await control.advance(Math.ceil((Date.parse(lesson.startsAt) + 2 * hour - (await control.now())) / 1000));
    await expect
      .poll(async () => {
        await control.tick();
        return (await readLesson(deployment, seed.maria, lesson.id)).status;
      })
      .toBe('ended');
    await signInThroughTheApi(page, seed.maria);

    await page.goto(lessonAddress(lesson));

    await expect(page.getByRole('main').getByRole('status').filter({ hasText: 'Не состоялся' })).toBeVisible();
    await expect(editButton(page)).toHaveCount(0);
    await expect(lessonActions(page)).toHaveCount(0);
    await expect(page.getByRole('main').getByText(/^Комната откроется/)).toHaveCount(0);
  });
});

test.describe('lesson page for a student in Russian', () => {
  test.use({ locale: 'ru-RU', timezoneId: moscow, viewport: { width: 1280, height: 800 } });

  test('lesson › a student sees Individual lesson, never the card name', async ({ page, deployment, control }) => {
    const seed = await tutorAndStudent(deployment, 'Аня из вторника');
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    await signInThroughTheApi(page, seed.anna);

    await page.goto(lessonAddress(lesson));

    const main = page.getByRole('main');
    await expect(main.getByText('Индивидуальный урок').first()).toBeVisible();
    await expect(main.getByText('Мария').first()).toBeVisible();
    await expect(main.getByText('Аня из вторника')).toHaveCount(0);
    await expect(main.getByText('Комната откроется в 17:50')).toBeVisible();
    await expect(editButton(page)).toHaveCount(0);
    await expect(lessonActions(page)).toHaveCount(0);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Разговорная практика');
    await expect(materialsBlock(page)).toHaveCount(0);
  });

  test('lesson › at 1440 × 900 the join block is the first content of the main column and in view for a student in a live lesson', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await lessonInTheJoinWindow(deployment, seed, control.now);
    await tutorJoinsThroughTheApi(deployment, seed, lesson);

    await openOnTheSharedClockAt1440(page, seed.anna, lesson, control.now);

    const joinBlock = firstOfTheMainColumn(page);
    const phase = joinBlock.getByRole('status').filter({ hasText: 'Идёт сейчас' });
    await expect(phase).toBeInViewport({ ratio: 1 });
    await expect(joinBlock.getByRole('region', { name: 'Видеокомната' })).toBeVisible();
    await expect(joinBlock.getByRole('button', { name: 'Подключиться', exact: true })).toBeInViewport({ ratio: 1 });
  });

  test('lesson › a student offers no series ending on a series lesson', async ({ page, deployment, control }) => {
    const seed = await tutorAndStudent(deployment);
    const [, selected] = await dailySeries(deployment, seed, await tomorrow(control.now), 2);
    await signInThroughTheApi(page, seed.anna);

    await page.goto(lessonAddress(selected));

    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Серия');
    await expect(lessonActions(page)).toHaveCount(0);
  });

  test('lesson › an open page updates when the tutor moves the lesson', async ({ page, deployment, control }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    await signInThroughTheApi(page, seed.anna);
    const traffic = trafficAfterReady(page, [`/api/v1/lessons/${lesson.id}`]);
    await traffic.initialize();
    await page.goto(lessonAddress(lesson));
    await expect(page.getByRole('main').getByText('18:00–19:00').first()).toBeVisible();
    await expectRefetchedAfterTheSocketIsReady(traffic);

    await editThisLesson(deployment, seed.maria, lesson.id, { start: '20:30' });

    await expect(page.getByRole('main').getByText('20:30–21:30').first()).toBeVisible();
    await expect(page.getByRole('main').getByText('18:00–19:00')).toHaveCount(0);
  });
});

const markupTitle = '<img src=x onerror="document.title=1">Карточки';

function materialsBlock(page: Page): Locator {
  return page.getByRole('region', { name: 'Материалы' });
}

function attachButton(page: Page): Locator {
  return materialsBlock(page).getByRole('button', { name: 'Прикрепить материал' });
}

function attachDialog(page: Page): Locator {
  return page.getByRole('dialog', { name: 'Прикрепить материал' });
}

function librarySearch(scope: Locator): Locator {
  return scope.getByRole('searchbox', { name: 'Поиск по библиотеке' });
}

function libraryChoice(scope: Locator, title: string): Locator {
  return scope.getByRole('list', { name: 'Материалы' }).getByRole('checkbox', { name: title, exact: true });
}

function folderPath(scope: Locator): Locator {
  return scope.getByRole('list', { name: 'Путь к папке' });
}

function materialRow(page: Page, title: string): Locator {
  return materialsBlock(page)
    .getByRole('listitem')
    .filter({ has: page.getByRole('link', { name: title, exact: true }) });
}

function detachButton(page: Page, title: string): Locator {
  return materialRow(page, title).getByRole('button', { name: 'Открепить' });
}

async function lessonWithMaterials(
  deployment: Deployment,
  seed: TutorAndStudent,
  date: LocalDate,
  titles: readonly string[],
): Promise<ScheduledLesson> {
  const lesson = await oneLesson(deployment, seed, date);
  const ids: string[] = [];
  for (const title of titles) {
    ids.push(await createLibraryMaterial(deployment, seed.maria, title));
  }
  await attachLessonMaterials(deployment, seed.maria, lesson.id, ids);
  return lesson;
}

async function openLessonAsTutor(page: Page, seed: TutorAndStudent, lesson: ScheduledLesson): Promise<void> {
  await signInThroughTheApi(page, seed.maria);
  await page.goto(lessonAddress(lesson));
  await materialsBlock(page).waitFor();
}

async function seriesWithMaterial(
  deployment: Deployment,
  seed: TutorAndStudent,
  date: LocalDate,
  materialId: string,
): Promise<readonly ScheduledLesson[]> {
  return scheduleLessonsWithIds(deployment, seed.maria, {
    relationship_id: seed.cardId,
    title: 'Серия',
    date,
    start: '18:00',
    duration_minutes: 60,
    weekdays: [1, 2, 3, 4, 5, 6, 7],
    count: 3,
    material_ids: [materialId],
  });
}

async function openFollowingEdit(page: Page, seed: TutorAndStudent, lesson: ScheduledLesson): Promise<Locator> {
  await openLessonAsTutor(page, seed, lesson);
  await editButton(page).click();
  const dialog = editDialog(page);
  await dialog.getByRole('radio', { name: 'Этот и следующие уроки' }).check();
  return dialog;
}

test.describe('lesson materials for a tutor in Russian', () => {
  test.use({ locale: 'ru-RU', timezoneId: moscow, viewport: { width: 1280, height: 800 } });

  test('lesson › Attach material opens the picker on its search field', async ({ page, deployment, control }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    await openLessonAsTutor(page, seed, lesson);

    await attachButton(page).click();

    await expect(librarySearch(attachDialog(page))).toBeFocused();
    await expect(materialsBlock(page).getByText('Материалов нет')).toBeVisible();
  });

  test('lesson › an attached material shows once in the picker, among the chosen ones', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await lessonWithMaterials(deployment, seed, await tomorrow(control.now), ['Рабочий лист']);
    await createLibraryMaterial(deployment, seed.maria, 'Таблица глаголов');
    await openLessonAsTutor(page, seed, lesson);

    await attachButton(page).click();

    const dialog = attachDialog(page);
    await expect(libraryChoice(dialog, 'Таблица глаголов')).toBeVisible();
    await expect(dialog.getByRole('checkbox', { name: 'Рабочий лист', exact: true })).toHaveCount(1);
    await expect(
      dialog.getByRole('list', { name: 'Выбрано' }).getByRole('checkbox', { name: 'Рабочий лист', exact: true }),
    ).toBeChecked();
  });

  test('lesson › opening a folder in the picker moves focus to its name in the path and lists its materials as text', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    const folder = await createLibraryFolder(deployment, seed.maria, 'Грамматика <b>1</b>');
    await createLibraryMaterial(deployment, seed.maria, markupTitle, folder);
    await openLessonAsTutor(page, seed, lesson);
    await attachButton(page).click();
    const dialog = attachDialog(page);

    await dialog.getByRole('list', { name: 'Папки' }).getByRole('button', { name: 'Грамматика <b>1</b>' }).click();

    await expect(folderPath(dialog).getByText('Грамматика <b>1</b>', { exact: true })).toBeFocused();
    await expect(libraryChoice(dialog, markupTitle)).toBeVisible();
    await expect(dialog.locator('img, b')).toHaveCount(0);
  });

  test('lesson › a path entry in the picker moves focus to the folder it opens', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    const folder = await createLibraryFolder(deployment, seed.maria, 'Грамматика');
    await createLibraryMaterial(deployment, seed.maria, 'Таблица глаголов', folder);
    await openLessonAsTutor(page, seed, lesson);
    await attachButton(page).click();
    const dialog = attachDialog(page);
    await dialog.getByRole('list', { name: 'Папки' }).getByRole('button', { name: 'Грамматика' }).click();
    await libraryChoice(dialog, 'Таблица глаголов').waitFor();

    await folderPath(dialog).getByRole('button', { name: 'Библиотека' }).click();

    await expect(folderPath(dialog).getByText('Библиотека', { exact: true })).toBeFocused();
    await expect(dialog.getByRole('list', { name: 'Папки' }).getByRole('button', { name: 'Грамматика' })).toBeVisible();
  });

  test('lesson › Show more in the picker moves focus to the first new material', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    for (let index = 1; index <= 31; index += 1) {
      await createLibraryMaterial(deployment, seed.maria, `Лист ${String(index).padStart(2, '0')}`);
    }
    await openLessonAsTutor(page, seed, lesson);
    await attachButton(page).click();
    const dialog = attachDialog(page);
    await libraryChoice(dialog, 'Лист 30').waitFor();

    await dialog.getByRole('button', { name: 'Показать ещё' }).click();

    await expect(libraryChoice(dialog, 'Лист 31')).toBeFocused();
    await expect(dialog.getByRole('button', { name: 'Показать ещё' })).toHaveCount(0);
  });

  test('lesson › the picker saves a root and a folder material in order and returns focus to Attach material', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    const folder = await createLibraryFolder(deployment, seed.maria, 'Грамматика');
    await createLibraryMaterial(deployment, seed.maria, 'Рабочий лист');
    await createLibraryMaterial(deployment, seed.maria, 'Таблица глаголов', folder);
    await openLessonAsTutor(page, seed, lesson);
    await attachButton(page).click();
    const dialog = attachDialog(page);

    await libraryChoice(dialog, 'Рабочий лист').check();
    await dialog.getByRole('list', { name: 'Папки' }).getByRole('button', { name: 'Грамматика' }).click();
    await libraryChoice(dialog, 'Таблица глаголов').check();
    await dialog.getByRole('button', { name: 'Сохранить' }).click();

    await expect(dialog).toBeHidden();
    await expect(attachButton(page)).toBeFocused();
    await expect(materialsBlock(page).getByRole('link')).toHaveText(['Рабочий лист', 'Таблица глаголов']);
    await expect(materialRow(page, 'Рабочий лист')).toContainText('Рабочий лист.pdf');
    expect(await lessonMaterialTitles(deployment, seed.maria, lesson.id)).toEqual(['Рабочий лист', 'Таблица глаголов']);
  });

  test('lesson › detaching a material moves focus to the row that took its place', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await lessonWithMaterials(deployment, seed, await tomorrow(control.now), [
      'Рабочий лист',
      'Таблица глаголов',
    ]);
    await openLessonAsTutor(page, seed, lesson);

    await detachButton(page, 'Рабочий лист').click();

    await expect(materialRow(page, 'Рабочий лист')).toHaveCount(0);
    await expect(detachButton(page, 'Таблица глаголов')).toBeFocused();
    expect(await lessonMaterialTitles(deployment, seed.maria, lesson.id)).toEqual(['Таблица глаголов']);
  });

  test('lesson › detaching the last row of several moves focus to the previous row', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await lessonWithMaterials(deployment, seed, await tomorrow(control.now), [
      'Рабочий лист',
      'Таблица глаголов',
    ]);
    await openLessonAsTutor(page, seed, lesson);

    await detachButton(page, 'Таблица глаголов').click();

    await expect(materialRow(page, 'Таблица глаголов')).toHaveCount(0);
    await expect(detachButton(page, 'Рабочий лист')).toBeFocused();
  });

  test('lesson › detaching the only material moves focus to the Materials heading', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await lessonWithMaterials(deployment, seed, await tomorrow(control.now), ['Рабочий лист']);
    await openLessonAsTutor(page, seed, lesson);

    await detachButton(page, 'Рабочий лист').click();

    await expect(materialsBlock(page).getByText('Материалов нет')).toBeVisible();
    await expect(materialsBlock(page).getByRole('heading', { name: 'Материалы' })).toBeFocused();
  });

  test('lesson › a material deleted from the library while it is being attached closes onto the changed notice', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    const material = await createLibraryMaterial(deployment, seed.maria, 'Рабочий лист');
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/lessons/${lesson.id}/materials`, status: 404 });
    await openLessonAsTutor(page, seed, lesson);
    await attachButton(page).click();
    await libraryChoice(attachDialog(page), 'Рабочий лист').check();
    await deleteLibraryMaterial(deployment, seed.maria, material);

    await attachDialog(page).getByRole('button', { name: 'Сохранить' }).click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: changedMeanwhile })).toBeFocused();
    await expect(materialsBlock(page).getByText('Материалов нет')).toBeVisible();
  });

  test('lesson › a material attached elsewhere while the picker is open refuses the stale save with the changed notice', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    const worksheet = await createLibraryMaterial(deployment, seed.maria, 'Рабочий лист');
    await createLibraryMaterial(deployment, seed.maria, 'Таблица глаголов');
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/lessons/${lesson.id}/materials`, status: 409 });
    const traffic = trafficAfterReady(page, [`/api/v1/lessons/${lesson.id}`]);
    await signInThroughTheApi(page, seed.maria);
    await traffic.initialize();
    await page.goto(lessonAddress(lesson));
    await expectRefetchedAfterTheSocketIsReady(traffic);
    await attachButton(page).click();
    await libraryChoice(attachDialog(page), 'Таблица глаголов').check();
    const refetched = page.waitForResponse(
      (response) =>
        response.request().method() === 'GET' && new URL(response.url()).pathname === `/api/v1/lessons/${lesson.id}`,
    );
    await attachLessonMaterials(deployment, seed.maria, lesson.id, [worksheet]);
    await refetched;

    await attachDialog(page).getByRole('button', { name: 'Сохранить' }).click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: changedMeanwhile })).toBeFocused();
    expect(await lessonMaterialTitles(deployment, seed.maria, lesson.id)).toEqual(['Рабочий лист']);
  });

  test('lesson › Attach material in New lesson opens the picker on its search field', async ({ page, deployment }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    await signInThroughTheApi(page, maria);
    await page.goto('/schedule');
    await page.getByRole('button', { name: 'Новый урок' }).click();
    await expect(newLessonHeading(page)).toBeFocused();
    const dialog = newLessonDialog(page);

    await dialog.getByRole('button', { name: 'Прикрепить материал' }).click();

    await expect(librarySearch(dialog)).toBeFocused();
  });

  test('lesson › a series scheduled in New lesson with a material found by search carries it on every lesson', async ({
    page,
    deployment,
    control,
  }) => {
    const { seed } = await endedSeriesWithNewLessonOpen(page, deployment, control.now);
    await createLibraryMaterial(deployment, seed.maria, 'Рабочий лист');
    await createLibraryMaterial(deployment, seed.maria, 'Таблица глаголов');
    const dialog = newLessonDialog(page);
    await dialog.getByRole('button', { name: 'Прикрепить материал' }).click();
    await libraryChoice(dialog, 'Таблица глаголов').waitFor();

    await librarySearch(dialog).fill('лист');
    await libraryChoice(dialog, 'Таблица глаголов').waitFor({ state: 'detached' });
    await libraryChoice(dialog, 'Рабочий лист').check();
    await dialog.getByRole('button', { name: 'Создать урок' }).click();

    await expect(newLessonHeading(page)).toHaveCount(0);
    const lessons = await scheduledCardLessonIds(deployment, seed.maria, seed.cardId);
    expect(lessons).toHaveLength(9);
    const titles = await Promise.all(lessons.map((id) => lessonMaterialTitles(deployment, seed.maria, id)));
    expect(titles.filter((materials) => materials.length > 0)).toEqual(Array(8).fill(['Рабочий лист']));
  });

  test('lesson › Attach material in Edit lesson opens the picker on its search field', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const worksheet = await createLibraryMaterial(deployment, seed.maria, 'Рабочий лист');
    const lessons = await seriesWithMaterial(deployment, seed, await tomorrow(control.now), worksheet);
    const dialog = await openFollowingEdit(page, seed, secondLessonOf({ lessons }));

    await dialog.getByRole('button', { name: 'Прикрепить материал' }).click();

    await expect(librarySearch(dialog)).toBeFocused();
  });

  test('lesson › editing this and following lessons replaces the materials of the following lessons', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const worksheet = await createLibraryMaterial(deployment, seed.maria, 'Рабочий лист');
    await createLibraryMaterial(deployment, seed.maria, 'Таблица глаголов');
    const lessons = await seriesWithMaterial(deployment, seed, await tomorrow(control.now), worksheet);
    const dialog = await openFollowingEdit(page, seed, secondLessonOf({ lessons }));
    await dialog.getByRole('group', { name: 'Материалы' }).getByText('Рабочий лист').waitFor();

    await dialog.getByRole('button', { name: 'Прикрепить материал' }).click();
    await dialog.getByRole('list', { name: 'Выбрано' }).getByRole('checkbox', { name: 'Рабочий лист' }).uncheck();
    await libraryChoice(dialog, 'Таблица глаголов').check();
    await dialog.getByRole('button', { name: 'Сохранить' }).click();

    await expect(dialog).toBeHidden();
    await expect(materialsBlock(page).getByRole('link')).toHaveText(['Таблица глаголов']);
    const titles = await Promise.all(lessons.map((lesson) => lessonMaterialTitles(deployment, seed.maria, lesson.id)));
    expect(titles).toEqual([['Рабочий лист'], ['Таблица глаголов'], ['Таблица глаголов']]);
  });
});

test.describe('lesson materials for a student in Russian', () => {
  test.use({ locale: 'ru-RU', timezoneId: moscow, viewport: { width: 1280, height: 800 } });

  test('lesson › a student downloads a material and sees one attached meanwhile without reload', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    const worksheet = await createLibraryMaterial(deployment, seed.maria, 'Рабочий лист');
    const cards = await createLibraryMaterial(deployment, seed.maria, markupTitle);
    await attachLessonMaterials(deployment, seed.maria, lesson.id, [worksheet]);
    await signInThroughTheApi(page, seed.anna);
    const traffic = trafficAfterReady(page, [`/api/v1/lessons/${lesson.id}`]);
    await traffic.initialize();
    await page.goto(lessonAddress(lesson));
    await materialsBlock(page).getByRole('link', { name: 'Рабочий лист' }).waitFor();
    await expectRefetchedAfterTheSocketIsReady(traffic);

    const downloaded = page.waitForEvent('download');
    await materialsBlock(page).getByRole('link', { name: 'Рабочий лист' }).click();
    const download = await downloaded;
    await attachLessonMaterials(deployment, seed.maria, lesson.id, [worksheet, cards]);

    expect(download.suggestedFilename().normalize('NFC')).toBe('Рабочий лист.pdf');
    expect(await download.failure()).toBeNull();
    await expect(materialsBlock(page).getByRole('link')).toHaveText(['Рабочий лист', markupTitle]);
    await expect(materialsBlock(page).locator('img, b')).toHaveCount(0);
    await expect(materialsBlock(page).getByRole('button')).toHaveCount(0);
  });
});
