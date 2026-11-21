import type { Locator, Page } from '@playwright/test';

import { type TestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser, tableRows } from './fixtures.ts';
import { acceptSignedIn, createCard, createGroup, seedActiveStudent } from './tutoring.ts';

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
    const heading = (shown: Page): Locator => shown.locator('main h1');
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
