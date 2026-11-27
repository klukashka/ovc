import type { Locator, Page } from '@playwright/test';

import { signInThroughTheApi } from './accounts.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { createFolder } from './homework.ts';
import { expectSettledAfterTheSocketIsReady, trafficAfterReady } from './realtime.ts';
import { addDays, localDate, scheduleLessonsWithIds } from './schedule.ts';
import { createGroup, endRelationship, seedActiveStudent } from './tutoring.ts';

const moscow = 'Europe/Moscow';

test.use({ locale: 'en-GB', viewport: { width: 1280, height: 800 }, reducedMotion: 'reduce' });

function issueHeading(page: Page): Locator {
  return page.getByRole('heading', { level: 1, name: 'Issue assignment' });
}

function addressOf(page: Page): string {
  const address = new URL(page.url());
  return `${address.pathname}${address.search}`;
}

test.describe('empty states with a next step', () => {
  test('an empty library folder offers New task and Upload, and Nothing found offers Clear search', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const folder = await createFolder(deployment, tutor, 'Reading');
    await signInThroughTheApi(page, tutor);
    await page.goto(`/homework/library?folder=${folder}`);
    await page.getByRole('heading', { level: 2, name: 'This folder is empty' }).waitFor();
    const search = page.getByRole('searchbox', { name: 'Search the library' });

    await search.fill('nothing like this');
    await page.getByRole('heading', { level: 2, name: 'Nothing found' }).waitFor();
    await page.getByRole('button', { name: 'Clear search', exact: true }).click();

    await expect(search).toBeFocused();
    await expect(search).toHaveValue('');
    await expect(page.getByRole('heading', { level: 2, name: 'This folder is empty' })).toBeVisible();
    await expect(page.getByText('Upload a material to this folder, or start a new task.')).toBeVisible();
    await expect(page.locator('main [data-empty-state]').getByRole('button')).toHaveText([
      'New task',
      'Upload material',
    ]);
  });

  test('empty issued work links to the library, and the empty withdrawn filter links back to the active one', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    await signInThroughTheApi(page, tutor);
    await page.goto('/homework/issued');
    await page.getByRole('heading', { level: 2, name: 'Nothing issued yet' }).waitFor();
    const libraryLink = await page
      .getByRole('main')
      .getByRole('link', { name: 'Open the library', exact: true })
      .getAttribute('href');
    await page.goto('/homework/issued?status=withdrawn');
    await page.getByText('No withdrawn assignments.').waitFor();

    await page.getByRole('link', { name: 'Show active assignments', exact: true }).click();

    expect(libraryLink).toBe('/homework/library');
    await expect(page.getByRole('heading', { level: 2, name: 'Nothing issued yet' })).toBeVisible();
    expect(addressOf(page)).toBe('/homework/issued');
    await expect(
      page.getByRole('navigation', { name: 'Show' }).getByRole('link', { name: 'Active', exact: true }),
    ).toBeFocused();
  });

  test('the group page, the student card and the lesson page offer issuing to their audience', async ({
    page,
    deployment,
    control,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = await seedActiveStudent(deployment, tutor, 'Anna');
    const groupId = await createGroup(deployment, tutor, 'Evening algebra', [anna.cardId]);
    const [lesson] = await scheduleLessonsWithIds(deployment, tutor, {
      relationship_id: anna.cardId,
      title: 'Algebra',
      date: addDays(localDate(await control.now(), moscow), 1),
      start: '18:00',
      duration_minutes: 60,
    });
    await signInThroughTheApi(page, tutor);
    const opened: string[] = [];

    for (const [address, action] of [
      [`/groups/${groupId}`, 'Issue to this group'],
      [`/students/${anna.cardId}?tab=homework`, 'Issue to this student'],
      [`/lessons/${lesson?.id ?? ''}`, 'Issue for this lesson'],
    ] as const) {
      const traffic = trafficAfterReady(page, ['/api/v1/library']);
      await page.goto(address);
      await page.getByRole('button', { name: action, exact: true }).click();
      await issueHeading(page).waitFor();
      await page.getByRole('main').getByText('1 student will receive this', { exact: true }).waitFor();
      await expectSettledAfterTheSocketIsReady(page, traffic);
      opened.push(addressOf(page));
    }

    expect(opened).toEqual(['/homework/issue', '/homework/issue', '/homework/issue']);
  });

  test('a group without members offers no issuing from its empty assignments', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const groupId = await createGroup(deployment, tutor, 'Evening algebra', []);
    await signInThroughTheApi(page, tutor);

    await page.goto(`/groups/${groupId}`);

    await expect(page.getByRole('heading', { level: 3, name: 'No students in this group yet' })).toBeVisible();
    await expect(page.getByText('No assignments for this group.', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Issue to this group', exact: true })).toHaveCount(0);
  });

  test('a student search without matches offers Clear filter, which returns to the list with focus on the search', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await signInThroughTheApi(page, tutor);
    await page.goto('/students?q=nobody');
    await page.getByText('Nobody matches the search.').waitFor();

    await page.getByRole('link', { name: 'Clear filter', exact: true }).click();

    await expect(page.getByRole('heading', { level: 2, name: 'Invite your first student' })).toBeVisible();
    await expect(page.getByLabel('Name or email')).toBeFocused();
    await expect(page.getByLabel('Name or email')).toHaveValue('');
  });
});

interface ShownEmptyState {
  readonly sentence: string | null;
  readonly link: string | null;
  readonly href: string | null;
}

test.describe('the student’s empty Homework tabs and My tutors', () => {
  test('each empty Homework tab names what arrives there and links to another tab', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student } = await seedActiveStudent(deployment, tutor, 'Anna');
    await signInThroughTheApi(page, student);
    const shown: ShownEmptyState[] = [];

    for (const [address, heading] of [
      ['/homework', 'Nothing to do'],
      ['/homework?status=submitted', 'Nothing submitted'],
      ['/homework?status=checked', 'Nothing checked yet'],
      ['/homework?status=closed', 'Nothing closed'],
    ] as const) {
      await page.goto(address);
      const empty = page.locator('main [data-empty-state]');
      await empty.getByRole('heading', { level: 2, name: heading }).waitFor();
      const link = empty.getByRole('link');
      shown.push({
        sentence: await empty.locator('p').textContent(),
        link: await link.textContent(),
        href: await link.getAttribute('href'),
      });
    }

    expect(shown).toEqual([
      {
        sentence: 'New assignments from your tutors appear here.',
        link: 'See checked work',
        href: '/homework?status=checked',
      },
      {
        sentence: 'Work you submit waits here until your tutor checks it.',
        link: 'See what to do',
        href: '/homework',
      },
      {
        sentence: 'Feedback appears here when your tutor checks your work.',
        link: 'See submitted work',
        href: '/homework?status=submitted',
      },
      {
        sentence: 'Assignments that can no longer be submitted appear here.',
        link: 'See what to do',
        href: '/homework',
      },
    ]);
  });

  test('My tutors without a current tutor points to Add a tutor beside the list', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    await endRelationship(deployment, tutor, cardId);
    await signInThroughTheApi(page, student);

    await page.goto('/tutors');

    const empty = page.locator('main [data-empty-state]');
    await expect(empty.getByRole('heading', { level: 2, name: 'No tutor yet.' })).toBeVisible();
    await expect(empty.locator('p')).toHaveText('Enter your tutor’s code under “Add a tutor” to start studying.');
    await expect(
      page.getByRole('complementary', { name: 'Adding a tutor and sent requests' }).getByRole('heading', {
        level: 2,
        name: 'Add a tutor',
      }),
    ).toBeVisible();
  });
});

test.describe('the student’s empty Home sections', () => {
  test('an empty Recently checked says when feedback arrives and an empty Next lesson offers the schedule', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
    const { student } = await seedActiveStudent(deployment, tutor, 'Anna', 'en');
    await signInThroughTheApi(page, student);
    const checked = page.getByRole('region', { name: 'Recently checked', exact: true });
    const nextLesson = page.getByRole('region', { name: 'Next lesson', exact: true });

    await page.goto('/home');
    await checked.locator('[data-empty-state]').waitFor();
    await nextLesson.locator('[data-empty-state]').waitFor();

    await expect(checked.getByRole('heading', { level: 3, name: 'Nothing checked yet' })).toBeVisible();
    await expect(checked.locator('[data-empty-state] p')).toHaveText(
      'Feedback appears here when your tutor checks your work.',
    );
    await expect(nextLesson.getByRole('heading', { level: 3, name: 'No lesson scheduled' })).toBeVisible();
    await expect(nextLesson.locator('[data-empty-state] p')).toHaveText(
      'Your next lesson appears here once your tutor schedules it.',
    );
    await expect(
      nextLesson.locator('[data-empty-state]').getByRole('link', { name: 'Open the schedule', exact: true }),
    ).toHaveAttribute('href', '/schedule');
    await expect(page.getByRole('main')).not.toContainText(/points|score|answer/i);
  });
});
