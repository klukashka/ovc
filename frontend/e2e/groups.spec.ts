import type { Locator, Page } from '@playwright/test';

import { type TestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import {
  diagnosedTest as test,
  expect,
  expectRowsNamed,
  seedVerifiedUser,
  signUpAndConfirmTutor,
  tableRows,
} from './fixtures.ts';
import { busyControls, recordBusyControls } from './observers.ts';
import { expectRefetchedAfterTheSocketIsReady, trafficAfterReady } from './realtime.ts';
import {
  addDays,
  cancelThisLesson,
  isoWeekday,
  localDate,
  scheduleLessons,
  scheduleLessonsWithIds,
} from './schedule.ts';
import {
  accessToken,
  addToGroup,
  archiveGroup,
  callApi,
  cardState,
  createGroup,
  endRelationship,
  groupMemberNames,
  seedActiveStudent,
  seedEmptyGroups,
  unarchiveGroup,
  signUpActiveStudentOnInvitation,
  signUpStudentThroughCode,
} from './tutoring.ts';

const unknownGroupId = '00000000-0000-4000-8000-00000000abcd';

async function seedForeignGroup(
  deployment: Deployment,
): Promise<{ readonly olga: TestAccount; readonly groupId: string }> {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const anna = await seedActiveStudent(deployment, pavel, 'Anna');
  const groupId = await createGroup(deployment, pavel, 'Secret algebra', [anna.cardId]);
  return { olga: await seedVerifiedUser(deployment, 'Olga', 'en'), groupId };
}

async function cardBeyondTheFirstPage(
  deployment: Deployment,
  tutor: TestAccount,
  cardIds: readonly string[],
): Promise<string> {
  const firstPage = await callApi(deployment, await accessToken(deployment, tutor), 'GET', '/students?state=active');
  const items: unknown =
    typeof firstPage.body === 'object' && firstPage.body !== null && 'items' in firstPage.body
      ? firstPage.body.items
      : [];
  const listed = new Set(
    (Array.isArray(items) ? items : []).map((item: unknown) =>
      typeof item === 'object' && item !== null && 'id' in item ? String(item.id) : '',
    ),
  );
  const beyond = cardIds.find((id) => !listed.has(id));
  if (beyond === undefined) {
    throw new Error('every card is on the first page');
  }
  return beyond;
}

function studentName(cardIds: readonly string[], cardId: string): string {
  return `Student ${String(cardIds.indexOf(cardId) + 1).padStart(2, '0')}`;
}

async function groupWithTwoFutureLessons(
  deployment: Deployment,
  now: () => Promise<number>,
): Promise<{ readonly pavel: TestAccount; readonly annaCard: string; readonly groupId: string }> {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const anna = await seedActiveStudent(deployment, pavel, 'Anna');
  const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId]);
  const tomorrow = addDays(localDate(await now(), 'Europe/Moscow'), 1);
  await scheduleLessons(deployment, pavel, {
    group_id: groupId,
    title: 'Lesson Evening algebra',
    date: tomorrow,
    start: '18:00',
    duration_minutes: 60,
    weekdays: [isoWeekday(tomorrow)],
    count: 2,
  });
  return { pavel, annaCard: anna.cardId, groupId };
}

async function groupPageWithLessons(
  page: Page,
  deployment: Deployment,
  now: () => Promise<number>,
  starts: readonly string[],
): Promise<{ readonly pavel: TestAccount; readonly lessonIds: readonly string[]; readonly lessons: Locator }> {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const anna = await seedActiveStudent(deployment, pavel, 'Anna');
  const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId]);
  const tomorrow = addDays(localDate(await now(), 'Europe/Moscow'), 1);
  const lessonIds: string[] = [];
  for (const start of starts) {
    const [lesson] = await scheduleLessonsWithIds(deployment, pavel, {
      group_id: groupId,
      title: `Lesson at ${start}`,
      date: tomorrow,
      start,
      duration_minutes: 60,
    });
    if (lesson === undefined) {
      throw new Error('no lesson was scheduled');
    }
    lessonIds.push(lesson.id);
  }
  await signInThroughTheApi(page, pavel);
  const traffic = trafficAfterReady(page, [`/api/v1/groups/${groupId}`]);
  await traffic.initialize();
  await page.goto(`/groups/${groupId}`);
  await expectRefetchedAfterTheSocketIsReady(traffic);
  const lessons = page.getByRole('list', { name: 'Next lessons', exact: true }).getByRole('link');
  return { pavel, lessonIds, lessons };
}

function members(page: Page, name: string): Locator {
  return page.getByRole('list', { name, exact: true }).getByRole('listitem');
}

test.describe('Maria in Russian', () => {
  test.use({
    locale: 'ru-RU',
    timezoneId: 'Europe/Moscow',
    viewport: { width: 1280, height: 800 },
    colorScheme: 'light',
  });

  test('pilot-2 › step 4 Maria creates the group B1 evening with Anna and Boris', async ({
    page,
    deployment,
    control,
  }) => {
    const maria = await signUpAndConfirmTutor(deployment, 'Мария');
    const anna = await signUpActiveStudentOnInvitation(deployment, control, maria, 'Анна', 'ru');
    const boris = await signUpStudentThroughCode(deployment, control, maria, 'Boris', 'en');
    await signInThroughTheApi(page, maria);
    await page.goto('/groups');
    await expect(page.getByRole('heading', { level: 2, name: 'Групп пока нет' })).toBeVisible();

    await page.getByRole('button', { name: 'Новая группа' }).click();
    const dialog = page.getByRole('dialog', { name: 'Новая группа' });
    await expect(dialog.getByLabel('Название')).toBeFocused();
    await dialog.getByLabel('Название').fill('B1 evening');
    await dialog.getByRole('radio', { name: 'Зелёный' }).check();
    await dialog.getByRole('checkbox', { name: 'Анна' }).check();
    await dialog.getByRole('checkbox', { name: 'Boris' }).check();
    await expect(dialog.getByText('Выбрано: 2 из 30')).toBeVisible();
    await dialog.getByRole('button', { name: 'Создать группу' }).click();

    await expect(page).toHaveURL(/\/groups\/[0-9a-f-]{36}$/);
    await expect(page.getByRole('heading', { level: 1, name: 'B1 evening' })).toBeFocused();
    await expectRowsNamed(members(page, 'Ученики'), ['Анна', 'Boris']);
    expect(await cardState(deployment, maria, anna.cardId)).toEqual({ state: 'active', groups: ['B1 evening'] });
    expect(await cardState(deployment, maria, boris.cardId)).toEqual({ state: 'active', groups: ['B1 evening'] });
    await page.getByRole('link', { name: 'Все группы' }).click();
    await expect(tableRows(page, 'Группы')).toHaveText([/B1 evening.*2 ученика/]);
    await page.goto('/messages');
    await expect(
      page.getByRole('region', { name: 'Чаты', exact: true }).getByRole('link').filter({ hasText: 'B1 evening' }),
    ).toBeVisible();
  });
});

test.describe('in English', () => {
  test.use({ locale: 'en-GB', colorScheme: 'dark' });

  test('groups › removing a student from the group moves focus to the notice', async ({ page, deployment }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = await seedActiveStudent(deployment, pavel, 'Anna');
    const boris = await seedActiveStudent(deployment, pavel, 'Boris');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId, boris.cardId]);
    await recordBusyControls(page);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);
    const groupMembers = members(page, 'Students');
    await expectRowsNamed(groupMembers, ['Anna', 'Boris']);

    await page.getByRole('button', { name: 'Remove from the group: Boris' }).click();

    const removed = page.getByRole('status').filter({ hasText: 'The student was removed from the group.' });
    await expect(removed).toBeFocused();
    await expect(groupMembers).toHaveText([/Anna/]);
    expect(await busyControls(page)).toEqual([['Remove from the group: Boris']]);
    expect(await cardState(deployment, pavel, boris.cardId)).toEqual({ state: 'active', groups: [] });
  });

  test('groups › archiving a group moves focus to the notice and takes no new students', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = await seedActiveStudent(deployment, pavel, 'Anna');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId]);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);
    await expectRowsNamed(members(page, 'Students'), ['Anna']);

    await page.getByRole('button', { name: 'Archive group' }).click();

    const archived = page.getByRole('status').filter({ hasText: 'The group is archived.' });
    await expect(archived).toBeFocused();
    await expect(page.getByRole('main').getByText('Archived', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Add students' })).toHaveCount(0);
    await expect(page.getByText('An archived group takes no new students.')).toBeVisible();
    expect(await cardState(deployment, pavel, anna.cardId)).toEqual({ state: 'active', groups: [] });
  });

  test('groups › archiving with future lessons asks first, with Cancel focused and the count of lessons it cancels', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, groupId } = await groupWithTwoFutureLessons(deployment, control.now);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);

    await page.getByRole('button', { name: 'Archive group' }).click();

    const dialog = page.getByRole('dialog', { name: 'Archive the group?' });
    await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeFocused();
    await expect(dialog).toContainText(
      '2 future lessons will be cancelled. Unarchiving the group restores neither lessons nor series.',
    );
  });

  test('groups › dismissing the archive confirmation returns focus to Archive group and archives nothing', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, annaCard, groupId } = await groupWithTwoFutureLessons(deployment, control.now);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);
    const archive = page.getByRole('button', { name: 'Archive group' });
    await archive.click();
    const dialog = page.getByRole('dialog', { name: 'Archive the group?' });

    await dialog.getByRole('button', { name: 'Cancel' }).click();

    await expect(dialog).toBeHidden();
    await expect(archive).toBeFocused();
    expect(await cardState(deployment, pavel, annaCard)).toEqual({ state: 'active', groups: ['Evening algebra'] });
  });

  test('groups › confirming the archive cancels the future lessons and moves focus to the notice', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, groupId } = await groupWithTwoFutureLessons(deployment, control.now);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);
    await page.getByRole('button', { name: 'Archive group' }).click();
    const dialog = page.getByRole('dialog', { name: 'Archive the group?' });

    await dialog.getByRole('button', { name: 'Archive group' }).click();

    await expect(page.getByRole('status').filter({ hasText: 'The group is archived.' })).toBeFocused();
    await expect(dialog).toBeHidden();
    await expect(page.getByRole('main').getByText('No lessons scheduled.')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Schedule a lesson' })).toHaveCount(0);
  });

  test('groups › a next lesson cancelled elsewhere while focused moves focus to the lesson that took its place', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, lessonIds, lessons } = await groupPageWithLessons(page, deployment, control.now, ['10:00', '12:00']);
    await lessons.filter({ hasText: 'Lesson at 10:00' }).focus();

    await cancelThisLesson(deployment, pavel, lessonIds[0] ?? '');

    await expect(lessons).toHaveCount(1);
    await expect(lessons.filter({ hasText: 'Lesson at 12:00' })).toBeFocused();
  });

  test('groups › the last next lesson cancelled elsewhere while focused moves focus to the previous lesson', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, lessonIds, lessons } = await groupPageWithLessons(page, deployment, control.now, ['10:00', '12:00']);
    await lessons.filter({ hasText: 'Lesson at 12:00' }).focus();

    await cancelThisLesson(deployment, pavel, lessonIds[1] ?? '');

    await expect(lessons).toHaveCount(1);
    await expect(lessons.filter({ hasText: 'Lesson at 10:00' })).toBeFocused();
  });

  test('groups › the only next lesson cancelled elsewhere while focused moves focus to the page heading', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, lessonIds, lessons } = await groupPageWithLessons(page, deployment, control.now, ['10:00']);
    await lessons.first().focus();

    await cancelThisLesson(deployment, pavel, lessonIds[0] ?? '');

    await expect(page.getByRole('main').getByText('No lessons scheduled.')).toBeVisible();
    await expect(page.getByRole('heading', { level: 1, name: 'Evening algebra' })).toBeFocused();
  });

  test('groups › Schedule a lesson on the group page preselects the group and lists the new lesson', async ({
    page,
    deployment,
    control,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = await seedActiveStudent(deployment, pavel, 'Anna');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId]);
    const tomorrow = addDays(localDate(await control.now(), 'Europe/Moscow'), 1);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);
    const opener = page.getByRole('button', { name: 'Schedule a lesson' });
    await opener.click();
    const dialog = page.getByRole('dialog', { name: 'New lesson' });
    await dialog.getByLabel('Date', { exact: true }).fill(tomorrow);
    await dialog.getByLabel('Start time').fill('18:00');

    await dialog.getByRole('button', { name: 'Schedule', exact: true }).click();

    await expect(dialog).toBeHidden();
    await expect(opener).toBeFocused();
    const next = page.getByRole('list', { name: 'Next lessons', exact: true }).getByRole('listitem');
    await expect(next).toHaveCount(1);
    await expect(next).toContainText('18:00–19:00');
    await expect(next).toContainText('Lesson Evening algebra');
  });

  test('groups › unarchiving a group moves focus to the notice and lists the group on its cards again', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = await seedActiveStudent(deployment, pavel, 'Anna');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId]);
    await archiveGroup(deployment, pavel, groupId);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);
    await expect(page.getByRole('main').getByText('Archived', { exact: true })).toBeVisible();

    await page.getByRole('button', { name: 'Unarchive group' }).click();

    const unarchived = page.getByRole('status').filter({ hasText: 'The group is active again.' });
    await expect(unarchived).toBeFocused();
    await expect(page.getByRole('main').getByText('Archived', { exact: true })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Add students' })).toBeVisible();
    expect(await cardState(deployment, pavel, anna.cardId)).toEqual({ state: 'active', groups: ['Evening algebra'] });
  });

  test('groups › the Add students dialog of an unarchived group opens on its search with only the students outside it', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = await seedActiveStudent(deployment, pavel, 'Anna');
    await seedActiveStudent(deployment, pavel, 'Boris');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId]);
    await archiveGroup(deployment, pavel, groupId);
    await unarchiveGroup(deployment, pavel, groupId);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);

    await page.getByRole('button', { name: 'Add students' }).click();

    const dialog = page.getByRole('dialog', { name: 'Add students' });
    await expect(dialog.getByRole('searchbox', { name: 'Search students' })).toBeFocused();
    await expect(dialog.getByRole('checkbox')).toHaveCount(1);
    await expect(dialog.getByRole('checkbox', { name: 'Boris' })).toHaveCount(1);
  });

  test('groups › adding a student to an unarchived group closes the dialog and moves focus to the notice', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = await seedActiveStudent(deployment, pavel, 'Anna');
    await seedActiveStudent(deployment, pavel, 'Boris');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId]);
    await archiveGroup(deployment, pavel, groupId);
    await unarchiveGroup(deployment, pavel, groupId);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);
    await page.getByRole('button', { name: 'Add students' }).click();
    const dialog = page.getByRole('dialog', { name: 'Add students' });
    await dialog.getByRole('checkbox', { name: 'Boris' }).check();

    await dialog.getByRole('button', { name: 'Add', exact: true }).click();

    const added = page.getByRole('status').filter({ hasText: 'The students were added to the group.' });
    await expect(added).toBeFocused();
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expectRowsNamed(members(page, 'Students'), ['Anna', 'Boris']);
    expect((await groupMemberNames(deployment, pavel, groupId)).sort()).toEqual(['Anna', 'Boris']);
  });

  test('groups › renaming saves the name with a notice that takes focus', async ({ page, deployment }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = await seedActiveStudent(deployment, pavel, 'Anna');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId]);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);
    const details = page.getByRole('form', { name: 'Name and colour' });
    await details.getByLabel('Name', { exact: true }).fill('Morning algebra');
    await details.getByRole('radio', { name: 'Teal' }).check();

    await details.getByRole('button', { name: 'Save' }).click();

    await expect(details.getByRole('status')).toHaveText('Saved.');
    await expect(details.getByRole('status')).toBeFocused();
    await expect(page.getByRole('heading', { level: 1, name: 'Morning algebra' })).toBeVisible();
    expect(await cardState(deployment, pavel, anna.cardId)).toEqual({ state: 'active', groups: ['Morning algebra'] });
  });

  test('groups › a background refetch of a group changed meanwhile keeps the unsaved name and colour', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = await seedActiveStudent(deployment, pavel, 'Anna');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId]);
    await signInThroughTheApi(page, pavel);
    const traffic = trafficAfterReady(page, [`/api/v1/groups/${groupId}`]);
    await traffic.initialize();
    await page.goto(`/groups/${groupId}`);
    await expectRefetchedAfterTheSocketIsReady(traffic);
    const details = page.getByRole('form', { name: 'Name and colour' });
    await details.getByLabel('Name', { exact: true }).fill('Typed name');
    await details.getByRole('radio', { name: 'Purple' }).check();
    const renamed = await callApi(deployment, await accessToken(deployment, pavel), 'PATCH', `/groups/${groupId}`, {
      name: 'Morning algebra',
      colour: 'teal',
    });
    expect(renamed.status).toBe(200);

    await page.evaluate(() => {
      document.dispatchEvent(new Event('visibilitychange', { bubbles: true }));
    });

    await expect(page.getByRole('heading', { level: 1, name: 'Morning algebra' })).toBeVisible();
    await expect(details.getByLabel('Name', { exact: true })).toHaveValue('Typed name');
    await expect(details.getByRole('radio', { name: 'Purple' })).toBeChecked();
    await expect(details.getByRole('radio', { name: 'Teal' })).not.toBeChecked();
  });

  test('groups › adding to a group archived meanwhile closes the dialog and shows the current group', async ({
    socketSlots,
    page,
    deployment,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = await seedActiveStudent(deployment, pavel, 'Anna');
    await seedActiveStudent(deployment, pavel, 'Boris');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId]);
    diagnostics.expectFailure({ method: 'PATCH', path: `/api/v1/groups/${groupId}`, status: 409 });
    await socketSlots.keepPageSocketsOff(pavel);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);
    await page.getByRole('button', { name: 'Add students' }).click();
    const dialog = page.getByRole('dialog', { name: 'Add students' });
    await dialog.getByRole('checkbox', { name: 'Boris' }).check();
    const renamed = await callApi(deployment, await accessToken(deployment, pavel), 'PATCH', `/groups/${groupId}`, {
      name: 'Morning algebra',
      colour: 'teal',
    });
    expect(renamed.status).toBe(200);
    await archiveGroup(deployment, pavel, groupId);

    await dialog.getByRole('button', { name: 'Add', exact: true }).click();

    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(
      page
        .getByRole('status')
        .filter({ hasText: 'This group changed in the meantime. Its current state is shown below.' }),
    ).toBeFocused();
    await expect(page.getByRole('main').getByText('Archived', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Add students' })).toHaveCount(0);
    const details = page.getByRole('form', { name: 'Name and colour' });
    await expect(details.getByLabel('Name', { exact: true })).toHaveValue('Morning algebra');
    await expect(details.getByRole('radio', { name: 'Teal' })).toBeChecked();
    expect(await groupMemberNames(deployment, pavel, groupId)).toEqual(['Anna']);
  });
});

test.describe('in English, the student list', () => {
  test.use({ locale: 'en-GB', colorScheme: 'light' });

  test('students › the group filter lives in the address and lists only the group’s students', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = await seedActiveStudent(deployment, pavel, 'Anna');
    await seedActiveStudent(deployment, pavel, 'Boris');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId]);
    await signInThroughTheApi(page, pavel);
    await page.goto('/students');
    const rows = tableRows(page, 'Active');
    await expectRowsNamed(rows, ['Anna', 'Boris']);
    await page.getByLabel('Group', { exact: true }).selectOption({ label: 'Evening algebra' });
    await page.getByRole('search', { name: 'Search students' }).getByRole('button', { name: 'Search' }).click();
    await expect(page).toHaveURL(`/students?state=active&group=${groupId}`);
    await expectRowsNamed(rows, ['Anna']);

    await page.reload();

    await expectRowsNamed(rows, ['Anna']);
    await expect(page.getByLabel('Group', { exact: true })).toHaveValue(groupId);
  });
});

test.describe('in English, refusals', () => {
  test.use({ locale: 'en-GB', colorScheme: 'dark' });

  test('groups › a chosen student who stopped being active meanwhile leaves the choice and the dialog stays open', async ({
    socketSlots,
    page,
    deployment,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await seedActiveStudent(deployment, pavel, 'Anna');
    const boris = await seedActiveStudent(deployment, pavel, 'Boris');
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/groups', status: 409 });
    await socketSlots.keepPageSocketsOff(pavel);
    await signInThroughTheApi(page, pavel);
    await page.goto('/groups');
    await page.getByRole('button', { name: 'New group' }).click();
    const dialog = page.getByRole('dialog', { name: 'New group' });
    await dialog.getByLabel('Name', { exact: true }).fill('Evening algebra');
    await dialog.getByRole('checkbox', { name: 'Anna' }).check();
    await dialog.getByRole('checkbox', { name: 'Boris' }).check();
    await endRelationship(deployment, pavel, boris.cardId);

    await dialog.getByRole('button', { name: 'Create group' }).click();

    const alert = dialog.getByRole('alert');
    await expect(alert).toHaveText(
      'Some of the chosen students no longer study with you, so they were taken off the choice. Check the choice and try again.',
    );
    await expect(alert).toBeFocused();
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole('checkbox', { name: 'Boris' })).toHaveCount(0);
    await expect(dialog.getByRole('checkbox', { name: 'Anna' })).toBeChecked();
    await expect(dialog.getByText('Chosen: 1 of 30')).toBeVisible();
  });

  test('groups › the group after the 200th is refused with the limit message', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await seedEmptyGroups(deployment, pavel, 199);
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/groups', status: 422 });
    await signInThroughTheApi(page, pavel);
    await page.goto('/groups');
    await page.getByRole('button', { name: 'New group' }).click();
    const dialog = page.getByRole('dialog', { name: 'New group' });
    await dialog.getByLabel('Name', { exact: true }).fill('One too many');
    await createGroup(deployment, pavel, 'The 200th group', []);

    await dialog.getByRole('button', { name: 'Create group' }).click();

    const alert = dialog.getByRole('alert');
    await expect(alert).toHaveText('You have reached the limit of 200 groups, archived groups included.');
    await expect(alert).toBeFocused();
    await expect(dialog).toBeVisible();
  });

  test('students › another tutor’s group and an unknown group in the filter are not available and show nothing of the group', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const { olga, groupId } = await seedForeignGroup(deployment);
    const unknownPage = await page.context().newPage();
    const unknownDiagnostics = await diagnostics.watch(unknownPage);
    diagnostics.expectFailure({ method: 'GET', path: `/api/v1/students?state=active&group=${groupId}`, status: 404 });
    unknownDiagnostics.expectFailure({
      method: 'GET',
      path: `/api/v1/students?state=active&group=${unknownGroupId}`,
      status: 404,
    });
    await signInThroughTheApi(page, olga);

    await page.goto(`/students?group=${groupId}`);
    await unknownPage.goto(`/students?group=${unknownGroupId}`);

    for (const shown of [page, unknownPage]) {
      await expect(shown.getByText('This group is not available.')).toBeVisible();
      await expect(shown.getByRole('main').getByRole('link', { name: 'All groups' })).toBeVisible();
      await expect(shown.getByText('Secret algebra')).toHaveCount(0);
    }
  });

  test('groups › another tutor’s group and an unknown group are not found and show nothing of the group', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const { olga, groupId } = await seedForeignGroup(deployment);
    const unknownPage = await page.context().newPage();
    const unknownDiagnostics = await diagnostics.watch(unknownPage);
    diagnostics.expectFailure({ method: 'GET', path: `/api/v1/groups/${groupId}`, status: 404 });
    unknownDiagnostics.expectFailure({ method: 'GET', path: `/api/v1/groups/${unknownGroupId}`, status: 404 });
    await signInThroughTheApi(page, olga);

    await page.goto(`/groups/${groupId}`);
    await unknownPage.goto(`/groups/${unknownGroupId}`);

    for (const shown of [page, unknownPage]) {
      await expect(shown.getByRole('heading', { level: 1, name: 'Page not found' })).toBeVisible();
      await expect(shown.getByText('Secret algebra')).toHaveCount(0);
    }
  });
});

test.describe('in English, one tutor with 31 active students shared by the tests below', () => {
  test.describe.configure({ mode: 'default' });
  test.use({ locale: 'en-GB', colorScheme: 'dark' });

  test('groups › show more in the member choice moves focus to the first new student', async ({
    page,
    studentsOfPavel: { pavel },
  }) => {
    await signInThroughTheApi(page, pavel);
    await page.goto('/groups');
    await page.getByRole('button', { name: 'New group' }).click();
    const choices = page.getByRole('dialog', { name: 'New group' }).getByRole('checkbox');
    await expect(choices).toHaveCount(30);

    await page.getByRole('dialog', { name: 'New group' }).getByRole('button', { name: 'Show more' }).click();

    await expect(choices).toHaveCount(31);
    await expect(choices.nth(30)).toBeFocused();
  });

  test('groups › show more that adds no visible student moves focus to the last student shown', async ({
    page,
    deployment,
    studentsOfPavel: { pavel, cardIds },
  }) => {
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [
      await cardBeyondTheFirstPage(deployment, pavel, cardIds),
    ]);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);
    await page.getByRole('button', { name: 'Add students' }).click();
    const dialog = page.getByRole('dialog', { name: 'Add students' });
    const choices = dialog.getByRole('checkbox');
    await expect(choices).toHaveCount(30);

    await dialog.getByRole('button', { name: 'Show more' }).press('Enter');

    await expect(dialog.getByRole('button', { name: 'Show more' })).toHaveCount(0);
    await expect(choices).toHaveCount(30);
    await expect(choices.nth(29)).toBeFocused();
  });

  test('groups › adding to a group that filled up meanwhile says that a group holds at most 30 students', async ({
    socketSlots,
    page,
    deployment,
    diagnostics,
    studentsOfPavel: { pavel, cardIds },
  }) => {
    const beyond = await cardBeyondTheFirstPage(deployment, pavel, cardIds);
    const [chosen = '', meanwhile = '', ...others] = cardIds.filter((id) => id !== beyond);
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [beyond, ...others]);
    diagnostics.expectFailure({ method: 'PATCH', path: `/api/v1/groups/${groupId}`, status: 422 });
    await socketSlots.keepPageSocketsOff(pavel);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);
    await page.getByRole('button', { name: 'Add students' }).click();
    const dialog = page.getByRole('dialog', { name: 'Add students' });
    await dialog.getByRole('checkbox', { name: studentName(cardIds, chosen), exact: true }).check();
    await addToGroup(deployment, pavel, groupId, [meanwhile]);

    await dialog.getByRole('button', { name: 'Add', exact: true }).click();

    const alert = dialog.getByRole('alert');
    await expect(alert).toHaveText('A group holds at most 30 students.');
    await expect(alert).toBeFocused();
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText('Chosen: 0 of 0')).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Add', exact: true })).toHaveAttribute('aria-disabled', 'true');
    expect(await groupMemberNames(deployment, pavel, groupId)).toHaveLength(30);
  });
});

test.describe('in English, the colour swatches', () => {
  test.use({ locale: 'en-GB', viewport: { width: 1280, height: 800 }, colorScheme: 'light' });

  async function openNewGroup(page: Page, deployment: Deployment): Promise<Locator> {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await createGroup(deployment, pavel, 'First group', []);
    await signInThroughTheApi(page, pavel);
    await page.goto('/groups');
    await page.locator('main header').getByRole('button', { name: 'New group' }).click();
    const dialog = page.getByRole('dialog', { name: 'New group' });
    await expect(dialog.getByLabel('Name')).toBeFocused();
    return dialog.getByRole('group', { name: 'Colour' });
  }

  test('the arrow keys move the chosen colour like native radios', async ({ page, deployment }) => {
    const colours = await openNewGroup(page, deployment);
    const first = colours.getByRole('radio').first();
    await first.focus();
    await page.keyboard.press('Space');

    await page.keyboard.press('ArrowRight');

    await expect(colours.getByRole('radio').nth(1)).toBeChecked();
    await expect(colours.getByRole('radio').nth(1)).toBeFocused();
    await expect(first).not.toBeChecked();
  });

  test('only the chosen swatch shows its check mark', async ({ page, deployment }) => {
    const colours = await openNewGroup(page, deployment);

    await colours.getByRole('radio', { name: 'Pink' }).check();

    await expect(colours.locator('[data-swatch="rust"] [data-swatch-check]')).toBeVisible();
    await expect(colours.locator('[data-swatch-check]').filter({ visible: true })).toHaveCount(1);
    await expect(colours.locator('[data-swatch="rust"]')).toHaveAttribute('title', 'Pink');
  });

  test('the colour swatches are at least 44 px on a coarse pointer @coarse-pointer', async ({
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await createGroup(deployment, pavel, 'First group', []);
    const context = await browser.newContext({
      baseURL: deployment.origin,
      ignoreHTTPSErrors: true,
      locale: 'en-GB',
      viewport: { width: 1280, height: 800 },
      hasTouch: true,
      isMobile: true,
    });
    try {
      const touchPage = await context.newPage();
      await diagnostics.watch(touchPage);
      await signInThroughTheApi(touchPage, pavel);
      await touchPage.goto('/groups');
      await touchPage.locator('main header').getByRole('button', { name: 'New group' }).click();
      const swatches = touchPage.getByRole('dialog', { name: 'New group' }).locator('[data-swatch]');
      await swatches.first().waitFor();

      const sizes = await swatches.evaluateAll((labels) =>
        labels.map((label) => {
          const box = label.getBoundingClientRect();
          return Math.min(box.width, box.height);
        }),
      );

      expect(sizes).toHaveLength(8);
      expect(Math.min(...sizes)).toBeGreaterThanOrEqual(44);
    } finally {
      await diagnostics.closeContext(context);
    }
  });
});
