import type { Locator, Page } from '@playwright/test';

import { type TestAccount, seedAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
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
  setTimezone,
} from './schedule.ts';
import { acceptSignedIn, createCard, createGroup, endRelationship } from './tutoring.ts';

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

function editButton(page: Page): Locator {
  return page.getByRole('button', { name: 'Изменить урок' });
}

function cancelButton(page: Page): Locator {
  return page.getByRole('button', { name: 'Отменить урок' });
}

function editDialog(page: Page): Locator {
  return page.getByRole('dialog', { name: 'Изменение урока' });
}

function cancelDialog(page: Page): Locator {
  return page.getByRole('dialog', { name: 'Отмена урока' });
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

function secondLessonOf(series: WeeklySeries): ScheduledLesson {
  const [, second] = series.lessons;
  if (second === undefined) {
    throw new Error('the series has fewer than two lessons');
  }
  return second;
}

function endSeriesButton(page: Page): Locator {
  return page.getByRole('button', { name: 'Завершить эту серию и создать новую' });
}

function endSeriesDialog(page: Page): Locator {
  return page.getByRole('dialog', { name: 'Завершить эту серию и создать новую' });
}

function newLessonDialog(page: Page): Locator {
  return page.getByRole('dialog', { name: 'Новый урок' });
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
  await endSeriesButton(page).click();
  await endSeriesDialog(page).getByRole('button', { name: 'Завершить серию' }).click();
  await newLessonDialog(page).getByLabel('Поиск ученика или группы').waitFor();
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
  await endSeriesButton(page).click();
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
  readonly opener: (page: Page) => Locator;
  readonly close: (page: Page) => Promise<void>;
}

const focusReturns: readonly FocusReturn[] = [
  {
    name: 'edit dismissed with Escape',
    opener: editButton,
    close: async (page): Promise<void> => {
      await page.keyboard.press('Escape');
    },
  },
  {
    name: 'cancellation dismissed',
    opener: cancelButton,
    close: async (page): Promise<void> => {
      await cancelDialog(page).getByRole('button', { name: 'Отмена', exact: true }).click();
    },
  },
  {
    name: 'series end dismissed',
    ofSeries: true,
    opener: endSeriesButton,
    close: async (page): Promise<void> => {
      await endSeriesDialog(page).getByRole('button', { name: 'Отмена', exact: true }).click();
    },
  },
  {
    name: 'edit saved',
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

  for (const { name, close, opener, ofSeries = false } of focusReturns) {
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
      await opener(page).click();
      await page.getByRole('dialog').waitFor();

      await close(page);

      await expect(page.getByRole('dialog')).toHaveCount(0);
      await expect(opener(page)).toBeFocused();
    });
  }

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

    await cancelButton(page).click();

    const dialog = cancelDialog(page);
    await expect(dialog.getByRole('button', { name: 'Отмена', exact: true })).toBeFocused();
    await expect(dialog.getByRole('radio')).toHaveCount(0);
    await expect(dialog).toHaveAccessibleDescription(
      'Урок будет отменён для всех и останется в истории как отменённый.',
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
    await cancelButton(page).click();

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
    await expect(cancelButton(page)).toHaveCount(0);
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

    await endSeriesButton(page).click();

    const dialog = endSeriesDialog(page);
    await expect(dialog.getByRole('button', { name: 'Отмена', exact: true })).toBeFocused();
    await expect(dialog.getByRole('radio')).toHaveCount(0);
    await expect(dialog).toHaveAccessibleDescription(
      'Этот и все следующие уроки серии будут отменены, а серия завершится. Затем откроется новый урок с тем же учеником или группой, временем и днями недели.',
    );
  });

  test('lesson › ending a series cancels this and the following lessons and opens New lesson with the same student, time and weekdays', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const series = await weeklySeries(deployment, seed, { relationship_id: seed.cardId }, await tomorrow(control.now));
    await signInThroughTheApi(page, seed.maria);
    await page.goto(lessonAddress(secondLessonOf(series)));
    await endSeriesButton(page).click();

    await endSeriesDialog(page).getByRole('button', { name: 'Завершить серию' }).click();

    const dialog = newLessonDialog(page);
    await expect(dialog.getByLabel('Поиск ученика или группы')).toBeFocused();
    await expect(dialog.getByRole('radio', { name: 'Анна', exact: true })).toBeChecked();
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
    await endSeriesButton(page).click();

    await endSeriesDialog(page).getByRole('button', { name: 'Завершить серию' }).click();

    const dialog = newLessonDialog(page);
    await expect(dialog.getByLabel('Поиск ученика или группы')).toBeFocused();
    await expect(dialog.getByRole('radio', { name: 'Вечерняя группа', exact: true })).toBeChecked();
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

    await newLessonDialog(page).getByRole('button', { name: 'Запланировать' }).click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: cancelledNotice })).toBeFocused();
    await expect(page.getByRole('status').filter({ hasText: 'Урок запланирован' })).toBeVisible();
    await expect(endSeriesButton(page)).toHaveCount(0);
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
    await newLessonDialog(page).getByLabel('Поиск ученика или группы').waitFor();

    await newLessonDialog(page).getByRole('button', { name: 'Запланировать' }).click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    const starts = await scheduledCardLessonStarts(deployment, seed.maria, seed.cardId);
    const cancelledStarts = series.lessons.slice(1).map((lesson) => lesson.startsAt);
    expect(starts).toHaveLength(9);
    expect(starts).toEqual(expect.arrayContaining(cancelledStarts));
  });

  test('lesson › ending a series whose lesson was cancelled meanwhile closes onto the changed notice and keeps the series', async ({
    page,
    deployment,
    control,
    socketSlots,
  }) => {
    const seed = await tutorAndStudent(deployment);
    const series = await weeklySeries(deployment, seed, { relationship_id: seed.cardId }, await tomorrow(control.now));
    const selected = secondLessonOf(series);
    await socketSlots.keepPageSocketsOff(seed.maria);
    await signInThroughTheApi(page, seed.maria);
    await page.goto(lessonAddress(selected));
    await endSeriesButton(page).click();
    await cancelThisLesson(deployment, seed.maria, selected.id);

    await endSeriesDialog(page).getByRole('button', { name: 'Завершить серию' }).click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: changedMeanwhile })).toBeFocused();
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
    await endSeriesButton(page).click();
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

    await expect(editButton(page)).toBeVisible();
    await expect(endSeriesButton(page)).toHaveCount(0);
  });

  test('lesson › a new series refused after the ending shows the refusal in the dialog', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/lessons', status: 409 });
    await refusedNewSeries(page, deployment, control.now);

    await newLessonDialog(page).getByRole('button', { name: 'Запланировать' }).click();

    await expect(newLessonDialog(page).getByText(audienceGone)).toBeVisible();
    await expect(newLessonDialog(page)).toBeVisible();
  });

  test('lesson › New lesson dismissed with its Cancel button after a refusal closes onto the cancelled notice', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/lessons', status: 409 });
    await refusedNewSeries(page, deployment, control.now);
    await newLessonDialog(page).getByRole('button', { name: 'Запланировать' }).click();
    await newLessonDialog(page).getByText(audienceGone).waitFor();

    await newLessonDialog(page).getByRole('button', { name: 'Отмена', exact: true }).click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(page.getByRole('status').filter({ hasText: cancelledNotice })).toBeFocused();
  });

  test('lesson › New lesson dismissed after ending a series closes onto the cancelled notice', async ({
    page,
    deployment,
    control,
  }) => {
    await endedSeriesWithNewLessonOpen(page, deployment, control.now);

    await page.keyboard.press('Escape');

    await expect(page.getByRole('dialog')).toHaveCount(0);
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
    await expect(cancelButton(page)).toHaveCount(0);
  });

  test('lesson › a student offers no series ending on a series lesson', async ({ page, deployment, control }) => {
    const seed = await tutorAndStudent(deployment);
    const [, selected] = await dailySeries(deployment, seed, await tomorrow(control.now), 2);
    await signInThroughTheApi(page, seed.anna);

    await page.goto(lessonAddress(selected));

    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Серия');
    await expect(endSeriesButton(page)).toHaveCount(0);
  });

  test('lesson › an open page updates when the tutor moves the lesson', async ({ page, deployment, control }) => {
    const seed = await tutorAndStudent(deployment);
    const lesson = await oneLesson(deployment, seed, await tomorrow(control.now));
    await signInThroughTheApi(page, seed.anna);
    const traffic = trafficAfterReady(page, [`/api/v1/lessons/${lesson.id}`]);
    await page.goto(lessonAddress(lesson));
    await expect(page.getByRole('main').getByText('18:00–19:00').first()).toBeVisible();
    await expectRefetchedAfterTheSocketIsReady(traffic);

    await editThisLesson(deployment, seed.maria, lesson.id, { start: '20:30' });

    await expect(page.getByRole('main').getByText('20:30–21:30').first()).toBeVisible();
    await expect(page.getByRole('main').getByText('18:00–19:00')).toHaveCount(0);
  });
});
