import type { Locator, Page } from '@playwright/test';

import { type TestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser, tableRows } from './fixtures.ts';
import { createGroup, seedActiveStudent } from './tutoring.ts';

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
    .locator(content === 'table' ? 'main table > tbody > tr' : 'main h1')
    .first()
    .waitFor();
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

interface SweptPage {
  readonly path: string;
  readonly ready: (page: Page) => Locator;
}

async function pagesScrollingSideways(page: Page, pages: readonly SweptPage[]): Promise<string[]> {
  await page.setViewportSize(phone);
  const sideways: string[] = [];
  for (const { path, ready } of pages) {
    await page.goto(path);
    await ready(page).waitFor();
    await page.evaluate(async () => {
      await document.fonts.ready;
    });
    if (await scrollsSideways(page)) {
      sideways.push(path);
    }
  }
  return sideways;
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

  test('the tutor’s pages group: Students, Groups and Settings do not scroll sideways', async ({
    page,
    deployment,
  }) => {
    const { tutor } = await tutorWithStudentInGroup(deployment, longWord, longWord);
    await signInThroughTheApi(page, tutor);

    const sideways = await pagesScrollingSideways(page, [
      { path: '/students', ready: firstTableRow },
      { path: '/groups', ready: firstTableRow },
      ...settingsPaths.map((path) => ({ path, ready: settingsReady })),
    ]);

    expect(sideways).toEqual([]);
  });

  test('the student’s frame group: Notifications does not scroll sideways', async ({ page, deployment }) => {
    const { student } = await tutorWithStudentInGroup(deployment, longWord, longWord);
    await signInThroughTheApi(page, student);

    const sideways = await pagesScrollingSideways(page, [{ path: '/notifications', ready: notificationsReady }]);

    expect(sideways).toEqual([]);
  });

  test('the student’s pages group: My tutors does not scroll sideways and shows its side column below the main column', async ({
    page,
    deployment,
  }) => {
    const { student } = await tutorWithStudentInGroup(deployment, longWord, longWord);
    await signInThroughTheApi(page, student);

    const sideways = await pagesScrollingSideways(page, [{ path: '/tutors', ready: firstTableRow }]);

    expect(sideways).toEqual([]);
    const main = await boxOf(page.locator('[data-column="main"]'));
    const aside = await boxOf(page.locator('[data-column="aside"]'));
    expect(aside.top).toBeGreaterThanOrEqual(main.bottom);
  });

  test('the public pages group: Sign in and Sign up do not scroll sideways', async ({ page }) => {
    const heading = (shown: Page): Locator => shown.locator('main h1');

    const sideways = await pagesScrollingSideways(page, [
      { path: '/', ready: heading },
      { path: '/signup', ready: heading },
    ]);

    expect(sideways).toEqual([]);
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
