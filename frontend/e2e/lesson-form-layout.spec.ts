import type { Locator, Page } from '@playwright/test';

import { signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { type LocalDate, addDays, isoWeekday, localDate, scheduleLessonsWithIds } from './schedule.ts';
import { seedActiveStudent } from './tutoring.ts';

const languages = [
  {
    language: 'ru',
    locale: 'ru-RU',
    newLesson: 'Новый урок',
    editLesson: 'Изменить урок',
    editDialog: 'Изменение урока',
    title: 'Название',
    date: 'Дата',
    search: 'Поиск ученика или группы',
    following: 'Этот и следующие уроки',
    save: 'Сохранить',
  },
  {
    language: 'en',
    locale: 'en-GB',
    newLesson: 'New lesson',
    editLesson: 'Edit lesson',
    editDialog: 'Edit lesson',
    title: 'Title',
    date: 'Date',
    search: 'Search for a student or group',
    following: 'This and following lessons',
    save: 'Save',
  },
] as const;

const viewports = [
  { name: 'desktop', width: 1280, height: 900, columns: true },
  { name: 'narrow', width: 320, height: 800, columns: false },
] as const;

interface FieldGeometry {
  readonly left: number;
  readonly right: number;
  readonly top: number;
  readonly bottom: number;
  readonly width: number;
  readonly labelTop: number;
  readonly labelBottom: number;
}

async function fieldGeometry(field: Locator): Promise<FieldGeometry> {
  return field.evaluate((element) => {
    if (!(element instanceof HTMLInputElement)) {
      throw new Error('the lesson field is not an input');
    }
    const label = element.labels?.item(0);
    if (label === undefined) {
      throw new Error('the lesson field has no label');
    }
    const inputBox = element.getBoundingClientRect();
    const labelBox = label.getBoundingClientRect();
    return {
      left: inputBox.left,
      right: inputBox.right,
      top: inputBox.top,
      bottom: inputBox.bottom,
      width: inputBox.width,
      labelTop: labelBox.top,
      labelBottom: labelBox.bottom,
    };
  });
}

function expectAligned(actual: number, expected: number): void {
  expect(Math.abs(actual - expected)).toBeLessThanOrEqual(1);
}

async function expectLessonFieldLayout(dialog: Locator, columns: boolean, hasDate = true): Promise<void> {
  const title = await fieldGeometry(dialog.locator('input[name="title"]'));
  const start = await fieldGeometry(dialog.locator('input[name="start"]'));
  const duration = await fieldGeometry(dialog.locator('input[name="duration_minutes"]'));
  const overflow = await dialog.evaluate((element) => ({
    dialog: element.scrollWidth - element.clientWidth,
    document: document.documentElement.scrollWidth - document.documentElement.clientWidth,
  }));
  expect(overflow.dialog).toBeLessThanOrEqual(1);
  expect(overflow.document).toBeLessThanOrEqual(1);
  for (const field of [start, duration]) {
    expect(field.labelBottom).toBeLessThanOrEqual(field.top);
    expect(field.left).toBeGreaterThanOrEqual(title.left - 1);
    expect(field.right).toBeLessThanOrEqual(title.right + 1);
    expect(field.bottom - field.top).toBeGreaterThanOrEqual(44);
  }
  expectAligned(start.left, title.left);
  expectAligned(duration.right, title.right);
  if (columns) {
    expectAligned(start.top, duration.top);
    expectAligned(start.bottom, duration.bottom);
    expectAligned(start.labelTop, duration.labelTop);
    expectAligned(start.width, duration.width);
    expect(start.right).toBeLessThan(duration.left);
  } else {
    expectAligned(start.width, title.width);
    expectAligned(duration.width, title.width);
    expect(start.bottom).toBeLessThan(duration.labelTop);
  }
  if (hasDate) {
    const date = await fieldGeometry(dialog.locator('input[name="date"]'));
    expectAligned(date.left, title.left);
    expectAligned(date.width, title.width);
    expect(date.width).toBeGreaterThanOrEqual(240);
    expect(date.labelBottom).toBeLessThanOrEqual(date.top);
    expect(date.bottom).toBeLessThan(start.labelTop);
  } else {
    await expect(dialog.locator('input[name="date"]')).toHaveCount(0);
  }
}

async function openExistingLesson(
  page: Page,
  deployment: Deployment,
  date: LocalDate,
  language: 'ru' | 'en',
  series: boolean,
): Promise<void> {
  const tutor = await seedVerifiedUser(deployment, 'Tutor', language);
  const { cardId } = await seedActiveStudent(deployment, tutor, 'Student', language);
  const [lesson] = await scheduleLessonsWithIds(deployment, tutor, {
    relationship_id: cardId,
    title: 'Conversation practice',
    date,
    start: '18:00',
    duration_minutes: 60,
    ...(series ? { weekdays: [isoWeekday(date)], count: 3 } : {}),
  });
  if (lesson === undefined) {
    throw new Error('the layout fixture scheduled no lesson');
  }
  await signInThroughTheApi(page, tutor);
  await page.goto(`/lessons/${lesson.id}`);
}

for (const language of languages) {
  for (const viewport of viewports) {
    test.describe(`lesson form layout › ${language.language} › ${viewport.name}`, () => {
      test.use({ locale: language.locale, viewport: { width: viewport.width, height: viewport.height } });

      test('creation gives the full date its own row and aligns time with duration', async ({ page, deployment }) => {
        const tutor = await seedVerifiedUser(deployment, 'Tutor', language.language);
        await signInThroughTheApi(page, tutor);
        await page.goto('/schedule');
        const dialog = page.getByRole('dialog', { name: language.newLesson });

        await page.getByRole('button', { name: language.newLesson, exact: true }).click();

        await expect(dialog.getByLabel(language.search)).toBeFocused();
        await expect(dialog.getByLabel(language.date, { exact: true })).toHaveValue(/^\d{4}-\d{2}-\d{2}$/);
        await expectLessonFieldLayout(dialog, viewport.columns);
      });

      test('editing gives the full date its own row and aligns time with duration', async ({
        page,
        deployment,
        control,
      }) => {
        const date = addDays(localDate(await control.now(), 'Europe/Moscow'), 1);
        await openExistingLesson(page, deployment, date, language.language, false);
        const dialog = page.getByRole('dialog', { name: language.editDialog });

        await page.getByRole('button', { name: language.editLesson, exact: true }).click();

        await expect(dialog.getByLabel(language.title, { exact: true })).toBeFocused();
        await expect(dialog.getByLabel(language.date, { exact: true })).toHaveValue(date);
        await expectLessonFieldLayout(dialog, viewport.columns);
      });

      test('a duration refusal keeps the date and time fields aligned', async ({
        page,
        deployment,
        control,
        diagnostics,
      }) => {
        const date = addDays(localDate(await control.now(), 'Europe/Moscow'), 1);
        await openExistingLesson(page, deployment, date, language.language, false);
        await page.getByRole('button', { name: language.editLesson, exact: true }).click();
        const dialog = page.getByRole('dialog', { name: language.editDialog });

        diagnostics.expectFailure({ method: 'PATCH', path: `/api/v1${new URL(page.url()).pathname}`, status: 422 });

        await dialog.locator('input[name="duration_minutes"]').fill('1');
        await dialog.getByRole('button', { name: language.save, exact: true }).click();

        await expect(dialog.locator('input[name="duration_minutes"]')).toHaveAttribute('aria-invalid', 'true');
        await expect(dialog.getByRole('alert')).toBeFocused();
        await expect(dialog.getByLabel(language.date, { exact: true })).toHaveValue(date);
        await expectLessonFieldLayout(dialog, viewport.columns);
      });

      test('editing this and following lessons fills both columns without a date', async ({
        page,
        deployment,
        control,
      }) => {
        const date = addDays(localDate(await control.now(), 'Europe/Moscow'), 1);
        await openExistingLesson(page, deployment, date, language.language, true);
        await page.getByRole('button', { name: language.editLesson, exact: true }).click();
        const dialog = page.getByRole('dialog', { name: language.editDialog });

        await dialog.getByRole('radio', { name: language.following, exact: true }).focus();

        await dialog.getByRole('radio', { name: language.following, exact: true }).press('Space');

        await expect(dialog.getByRole('radio', { name: language.following, exact: true })).toBeFocused();
        await expect(dialog.getByRole('radio', { name: language.following, exact: true })).toBeChecked();
        await expectLessonFieldLayout(dialog, viewport.columns, false);
      });
    });
  }
}
