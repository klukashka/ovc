import type { Locator, Page } from '@playwright/test';

import { newTestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser, tableRows } from './fixtures.ts';
import { notificationRows } from './notifications.ts';
import { accountId, waitForSockets } from './realtime.ts';
import { addDays, editThisLesson, localDate, scheduleLessonsWithIds } from './schedule.ts';
import {
  registerStudentWithCode,
  seedActiveStudent,
  seedJoinRequests,
  seedStudentThroughCode,
  tutorCode,
} from './tutoring.ts';

function sidebarBell(page: Page): Locator {
  return page.getByRole('navigation').getByRole('button', { name: /^Уведомления/ });
}

function panel(page: Page): Locator {
  return page.getByRole('region', { name: 'Уведомления' });
}

function rowsOf(scope: Locator): Locator {
  return scope.getByRole('list', { name: 'Уведомления' }).getByRole('listitem');
}

async function openPanel(page: Page): Promise<Locator> {
  await sidebarBell(page).click();
  await expect(panel(page).getByRole('heading', { name: 'Уведомления' })).toBeFocused();
  return panel(page);
}

test.describe('notifications in Russian on a computer', () => {
  test.use({ locale: 'ru-RU', timezoneId: 'Europe/Moscow', viewport: { width: 1280, height: 800 } });

  test.describe('with 31 notifications', () => {
    test.describe.configure({ mode: 'default' });

    test('notifications › the panel shows the latest 10 and opens the page', async ({
      page,
      deployment,
      mariaWithJoinRequests,
    }) => {
      const { maria } = mariaWithJoinRequests;
      const newest = (await notificationRows(deployment, maria)).slice(0, 10).map((row) => row.label ?? '');
      await signInThroughTheApi(page, maria);
      await page.goto('/today');
      await expect(sidebarBell(page)).toHaveAccessibleName('Уведомления, 31 непрочитанное');

      await sidebarBell(page).click();

      await expect(panel(page).getByRole('heading', { name: 'Уведомления' })).toBeFocused();
      const rows = rowsOf(panel(page));
      await expect(rows).toHaveCount(10);
      await expect(rows).toContainText(newest);
      await expect(rows.first()).toContainText('Новый запрос на занятия');
      await expect(panel(page).getByRole('link', { name: 'Все уведомления' })).toHaveAttribute(
        'href',
        '/notifications',
      );
    });

    test('notifications › Show all notifications in the panel opens the page on its heading', async ({
      page,
      mariaWithJoinRequests,
    }) => {
      const { maria } = mariaWithJoinRequests;
      await signInThroughTheApi(page, maria);
      await page.goto('/today');
      const shown = await openPanel(page);
      await expect(rowsOf(shown)).toHaveCount(10);

      await shown.getByRole('link', { name: 'Все уведомления' }).click();

      await expect(page).toHaveURL('/notifications');
      await expect(page.getByRole('heading', { level: 1, name: 'Уведомления' })).toBeFocused();
      await expect(panel(page)).toHaveCount(0);
      await expect(tableRows(page.getByRole('main'), 'Уведомления')).toHaveCount(30);
    });

    test('notifications › Show more on the page moves focus to the first new row', async ({
      page,
      mariaWithJoinRequests,
    }) => {
      const { maria } = mariaWithJoinRequests;
      await signInThroughTheApi(page, maria);
      await page.goto('/notifications');
      const rows = tableRows(page.getByRole('main'), 'Уведомления');
      await expect(rows).toHaveCount(30);

      await page.getByRole('button', { name: 'Показать ещё' }).click();

      await expect(rows).toHaveCount(31);
      await expect(rows.nth(30).getByRole('link')).toBeFocused();
      await expect(page.getByRole('button', { name: 'Показать ещё' })).toHaveCount(0);
    });
  });

  test('notifications › opening the panel moves focus to its heading and Escape returns it to the bell', async ({
    page,
    deployment,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    await signInThroughTheApi(page, maria);
    await page.goto('/today');
    const shown = await openPanel(page);
    await expect(shown.getByText('Уведомлений пока нет')).toBeVisible();

    await page.keyboard.press('Escape');

    await expect(panel(page)).toHaveCount(0);
    await expect(sidebarBell(page)).toBeFocused();
    await expect(sidebarBell(page)).toHaveAttribute('aria-expanded', 'false');
  });

  test('notifications › opening a row marks it read and opens its page', async ({ page, deployment }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const { cardId } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
    await signInThroughTheApi(page, maria);
    await page.goto('/today');
    await expect(sidebarBell(page)).toHaveAccessibleName('Уведомления, 1 непрочитанное');
    const row = rowsOf(await openPanel(page)).first();
    await expect(row).toContainText('Анна');
    await expect(row).toContainText('Приглашение принято');
    await expect(row).toContainText('Не прочитано');

    await row.getByRole('link').click();

    await expect(page).toHaveURL(`/students/${cardId}`);
    await expect(page.getByRole('heading', { level: 1, name: 'Анна' })).toBeFocused();
    await expect(panel(page)).toHaveCount(0);
    await expect(sidebarBell(page)).toHaveAccessibleName('Уведомления');
  });

  test('notifications › a row of the notifications page marks it read and opens its page', async ({
    page,
    deployment,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const { cardId } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
    await signInThroughTheApi(page, maria);
    await page.goto('/notifications');
    await expect(sidebarBell(page)).toHaveAccessibleName('Уведомления, 1 непрочитанное');
    const row = tableRows(page.getByRole('main'), 'Уведомления').first();
    await expect(row).toContainText('Анна');
    await expect(row).toContainText('Приглашение принято');
    await expect(row).toContainText('Не прочитано');

    await row.getByRole('link', { name: 'Приглашение принято' }).click();

    await expect(page).toHaveURL(`/students/${cardId}`);
    await expect(page.getByRole('heading', { level: 1, name: 'Анна' })).toBeFocused();
    await expect(sidebarBell(page)).toHaveAccessibleName('Уведомления');
  });

  test('notifications › Mark all as read in the page header marks every row and keeps focus', async ({
    page,
    deployment,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    await seedActiveStudent(deployment, maria, 'Анна', 'ru');
    await signInThroughTheApi(page, maria);
    await page.goto('/notifications');
    const markAll = page.getByRole('main').getByRole('button', { name: 'Отметить все как прочитанные' });
    await expect(tableRows(page.getByRole('main'), 'Уведомления').first()).toContainText('Не прочитано');

    await markAll.press('Enter');

    await expect(page.getByRole('main').getByRole('status')).toHaveText('Все уведомления отмечены как прочитанные.');
    await expect(tableRows(page.getByRole('main'), 'Уведомления').first()).not.toContainText('Не прочитано');
    await expect(sidebarBell(page)).toHaveAccessibleName('Уведомления');
    await expect(markAll).toBeFocused();
  });

  test('notifications › a lesson change row links to the lesson', async ({ page, deployment, control }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const { student: anna, cardId } = await seedActiveStudent(deployment, maria, 'Анна', 'ru');
    const tomorrow = addDays(localDate(await control.now(), 'Europe/Moscow'), 1);
    const [lesson] = await scheduleLessonsWithIds(deployment, maria, {
      relationship_id: cardId,
      title: 'Разговорная практика',
      date: tomorrow,
      start: '10:00',
      duration_minutes: 60,
    });
    if (lesson === undefined) {
      throw new Error('no lesson was scheduled');
    }
    await editThisLesson(deployment, maria, lesson.id, { start: '11:00' });
    await expect
      .poll(async () => {
        await control.tick();
        return (await notificationRows(deployment, anna)).map((row) => row.event);
      })
      .toContain('lesson.moved');
    await signInThroughTheApi(page, anna);
    await page.goto('/home');
    const row = rowsOf(await openPanel(page)).filter({ hasText: 'Урок перенесён' });
    await expect(row).toContainText('Разговорная практика');

    await row.getByRole('link').click();

    await expect(page).toHaveURL(`/lessons/${lesson.id}`);
    await expect(page.getByRole('heading', { level: 1, name: 'Разговорная практика' })).toBeFocused();
    await expect(page.getByRole('main').getByText('11:00–12:00').first()).toBeVisible();
  });

  test('notifications › mark all as read leaves a row that arrived meanwhile unread', async ({
    page,
    deployment,
    control,
    socketSlots,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    await seedJoinRequests(deployment, maria, 1);
    await socketSlots.keepPageSocketsOff(maria);
    await signInThroughTheApi(page, maria);
    await page.goto('/today');
    const shown = await openPanel(page);
    await expect(rowsOf(shown)).toHaveCount(1);
    await control.advance(60);
    await seedJoinRequests(deployment, maria, 1);
    const markAll = shown.getByRole('button', { name: 'Отметить все как прочитанные' });

    await markAll.press('Enter');

    await expect(shown.getByRole('status')).toHaveText('Все уведомления отмечены как прочитанные.');
    await expect(markAll).toBeFocused();
    await expect(rowsOf(shown)).toHaveCount(2);
    await expect(rowsOf(shown).first()).toContainText('Не прочитано');
    await expect(rowsOf(shown).last()).not.toContainText('Не прочитано');
    await expect(sidebarBell(page)).toHaveAccessibleName('Уведомления, 1 непрочитанное');
  });

  test('notifications › a new row is announced politely and the badge updates in another tab', async ({
    page,
    context,
    deployment,
    diagnostics,
  }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    await signInThroughTheApi(page, maria);
    await page.goto('/today');
    const otherTab = await context.newPage();
    await diagnostics.watch(otherTab);
    await otherTab.goto('/students');
    await waitForSockets(deployment, await accountId(deployment, maria), 2);
    const heading = page.getByRole('heading', { level: 1, name: 'Сегодня' });
    await heading.focus();

    await seedJoinRequests(deployment, maria, 1);

    await expect(page.locator('[aria-live="polite"]').filter({ hasText: 'Новое уведомление' })).toHaveCount(1);
    await expect(heading).toBeFocused();
    await expect(sidebarBell(page)).toHaveAccessibleName('Уведомления, 1 непрочитанное');
    await expect(sidebarBell(otherTab)).toHaveAccessibleName('Уведомления, 1 непрочитанное');
  });

  test('notifications › a label with markup is shown as text', async ({ page, deployment }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const markup = '<img src=x onerror="document.title=1"><b>Боб</b>';
    await seedStudentThroughCode(deployment, maria, markup, 'ru');
    await signInThroughTheApi(page, maria);
    await page.goto('/today');
    await expect(sidebarBell(page)).toHaveAccessibleName('Уведомления, 1 непрочитанное');

    const shown = await openPanel(page);

    await expect(rowsOf(shown).first()).toContainText(markup);
    await expect(shown.locator('img, b')).toHaveCount(0);
    await expect(page).toHaveTitle('Сегодня · OVC');
  });

  test('unverified student › no bell and no panel', async ({ page, deployment }) => {
    const maria = await seedVerifiedUser(deployment, 'Мария');
    const student = newTestAccount('Вера', 'student');
    await registerStudentWithCode(deployment, student, await tutorCode(deployment, maria));
    await signInThroughTheApi(page, student);
    const notificationRequests: string[] = [];
    page.on('request', (request) => {
      if (new URL(request.url()).pathname.startsWith('/api/v1/notifications')) {
        notificationRequests.push(request.url());
      }
    });

    await page.goto('/home');

    await expect(page.getByRole('heading', { level: 1, name: 'Главная' })).toBeVisible();
    await expect(page.getByRole('navigation').getByRole('link', { name: 'Мои репетиторы' })).toBeVisible();
    await expect(page.getByRole('navigation').getByRole('button', { name: /Уведомления/ })).toHaveCount(0);
    await expect(page.getByRole('navigation').getByRole('link', { name: /Уведомления/ })).toHaveCount(0);
    await expect(panel(page)).toHaveCount(0);
    expect(notificationRequests).toEqual([]);
  });
});

test.describe('notifications on a phone', () => {
  test.use({ locale: 'ru-RU', timezoneId: 'Europe/Moscow', viewport: { width: 360, height: 780 } });

  function topBarButton(page: Page): Locator {
    return page.getByRole('banner').getByRole('button', { name: /^Уведомления/ });
  }

  async function signedInPhoneTutor(page: Page, deployment: Deployment): Promise<void> {
    const tutor = await seedVerifiedUser(deployment, 'Репетитор с телефоном');
    await signInThroughTheApi(page, tutor);
    await page.goto('/today');
    await page.getByRole('heading', { level: 1, name: 'Сегодня' }).waitFor();
  }

  test('the top bar button opens the panel at the full width on its heading', async ({ page, deployment }) => {
    await signedInPhoneTutor(page, deployment);

    await topBarButton(page).click();

    await expect(panel(page).getByRole('heading', { name: 'Уведомления' })).toBeFocused();
    const panelWidth = Math.round((await panel(page).boundingBox())?.width ?? 0);
    const windowWidth = await page.evaluate(() => document.documentElement.clientWidth);
    expect(panelWidth).toBe(windowWidth);
  });

  test('Escape closes the panel and returns focus to the top bar button', async ({ page, deployment }) => {
    await signedInPhoneTutor(page, deployment);
    await topBarButton(page).click();
    await expect(panel(page).getByRole('heading', { name: 'Уведомления' })).toBeFocused();

    await page.keyboard.press('Escape');

    await expect(panel(page)).toHaveCount(0);
    await expect(topBarButton(page)).toBeFocused();
  });
});
