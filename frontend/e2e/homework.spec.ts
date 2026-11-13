import type { Locator, Page } from '@playwright/test';

import { type TestAccount, signInThroughTheApi } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import {
  chooseFromPageMenu,
  diagnosedTest as test,
  expect,
  openPageMenu,
  pageMenuButton,
  seedVerifiedUser,
  tableRows,
} from './fixtures.ts';
import {
  changeDeadlineThroughTheApi,
  createFolder,
  createIncompleteTemplate,
  createReadyTemplate,
  issueIdsOf,
  issueManyThroughTheApi,
  issueThroughTheApi,
  issueTitles,
  makeTemplateIncomplete,
  readIssue,
  withdrawThroughTheApi,
} from './homework.ts';
import { onlySubmission, submitThroughTheApi } from './assignment.ts';
import { expectRefetchedAfterTheSocketIsReady, trafficAfterReady } from './realtime.ts';
import { addDays, localDate, scheduleLessonsWithIds } from './schedule.ts';
import { createGroup, seedActiveStudent } from './tutoring.ts';

const moscow = 'Europe/Moscow';
const dayMilliseconds = 86_400_000;

interface PavelAndAnna {
  readonly pavel: TestAccount;
  readonly annaCard: string;
}

async function pavelWithAnna(deployment: Deployment): Promise<PavelAndAnna> {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const anna = await seedActiveStudent(deployment, pavel, 'Anna');
  return { pavel, annaCard: anna.cardId };
}

async function lessonTomorrow(
  deployment: Deployment,
  tutor: TestAccount,
  cardId: string,
  now: number,
): Promise<string> {
  const [lesson] = await scheduleLessonsWithIds(deployment, tutor, {
    relationship_id: cardId,
    title: 'Algebra',
    date: addDays(localDate(now, moscow), 1),
    start: '18:00',
    duration_minutes: 60,
  });
  if (lesson === undefined) {
    throw new Error('no lesson was scheduled');
  }
  return lesson.id;
}

async function holdSaves(page: Page, templateId: string): Promise<() => Promise<void>> {
  let resume: () => void = () => undefined;
  const resumed = new Promise<void>((resolve) => {
    resume = resolve;
  });
  await page.route('**/api/v1/library/templates/*', async (route) => {
    if (route.request().method() === 'PUT') {
      await resumed;
    }
    await route.continue();
  });
  return async () => {
    const answered = page.waitForResponse(
      (response) =>
        response.request().method() === 'PUT' &&
        new URL(response.url()).pathname === `/api/v1/library/templates/${templateId}`,
    );
    resume();
    await answered;
    await page.unroute('**/api/v1/library/templates/*');
  };
}

async function openCardIssueDialog(page: Page, tutor: TestAccount, cardId: string, listed: string): Promise<void> {
  await signInThroughTheApi(page, tutor);
  await page.goto(`/students/${cardId}`);
  await page.getByRole('button', { name: 'Issue assignment', exact: true }).click();
  await page
    .getByRole('dialog', { name: 'Issue assignment' })
    .getByRole('button', { name: listed, exact: true })
    .waitFor();
}

function issueDialog(page: Page): Locator {
  return page.getByRole('dialog', { name: 'Issue assignment' });
}

function audienceSearch(page: Page): Locator {
  return issueDialog(page).getByRole('searchbox', { name: 'Search for a student or group' });
}

function issuedNotice(page: Page): Locator {
  return page.getByText('Assignment issued.', { exact: true });
}

function rowMenuButton(page: Page, title: string): Locator {
  return page.getByRole('button', { name: `Actions for ${title}`, exact: true });
}

async function chooseRowAction(page: Page, title: string, action: string): Promise<void> {
  await rowMenuButton(page, title).click();
  await page.locator('main [aria-expanded="true"] + div').getByRole('button', { name: action, exact: true }).click();
}

async function openLibrary(page: Page, tutor: TestAccount): Promise<void> {
  await signInThroughTheApi(page, tutor);
  await page.goto('/homework/library');
  await page.getByRole('heading', { level: 1, name: 'Library' }).waitFor();
}

async function openIssuePage(page: Page, tutor: TestAccount, issueId: string, title: string): Promise<void> {
  await signInThroughTheApi(page, tutor);
  const traffic = trafficAfterReady(page, [`/api/v1/issues/${issueId}`]);
  await traffic.initialize();
  await page.goto(`/homework/issued/${issueId}`);
  await page.getByRole('heading', { level: 1, name: title }).waitFor();
  await expectRefetchedAfterTheSocketIsReady(traffic);
}

async function fillDeadline(dialog: Locator, instant: number): Promise<void> {
  await dialog.getByLabel('Date', { exact: true }).fill(localDate(instant, moscow));
  await dialog.getByLabel('Time', { exact: true }).fill('12:00');
}

test.describe('issuing from the library', () => {
  test('an incomplete template’s menu has no Issue assignment item', async ({ page, deployment }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await createIncompleteTemplate(deployment, pavel, 'Draft');
    await openLibrary(page, pavel);

    await rowMenuButton(page, 'Draft').click();

    const items = page.locator('main [aria-expanded="true"] + div');
    await expect(items.getByRole('button', { name: 'Open', exact: true })).toBeVisible();
    await expect(items.getByRole('button', { name: 'Issue assignment', exact: true })).toHaveCount(0);
  });

  test('Issue assignment opens the dialog on the audience search with the template preset', async ({
    page,
    deployment,
  }) => {
    const { pavel } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    await openLibrary(page, pavel);

    await chooseRowAction(page, 'Fractions', 'Issue assignment');

    await expect(audienceSearch(page)).toBeFocused();
    await expect(issueDialog(page).getByText('Fractions', { exact: true })).toBeVisible();
    await expect(issueDialog(page).getByRole('radio', { name: 'After checking' })).toBeChecked();
  });

  test('dismissing the dialog returns focus to the row’s menu button', async ({ page, deployment }) => {
    const { pavel } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    await openLibrary(page, pavel);
    await chooseRowAction(page, 'Fractions', 'Issue assignment');
    await audienceSearch(page).waitFor();

    await page.keyboard.press('Escape');

    await expect(issueDialog(page)).toBeHidden();
    await expect(rowMenuButton(page, 'Fractions')).toBeFocused();
  });

  test('issuing to a student closes the dialog onto the focused notice with a link to the issue', async ({
    page,
    deployment,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    await openLibrary(page, pavel);
    await chooseRowAction(page, 'Fractions', 'Issue assignment');
    await issueDialog(page).getByRole('button', { name: 'Anna', exact: true }).click();
    await issueDialog(page).getByText('1 student will receive this', { exact: true }).waitFor();

    await issueDialog(page).getByRole('button', { name: 'Issue', exact: true }).click();

    await expect(issueDialog(page)).toBeHidden();
    await expect(issuedNotice(page)).toBeFocused();
    const [issueId] = await issueIdsOf(deployment, pavel, `student=${annaCard}&status=active`);
    await expect(page.getByRole('link', { name: 'Open the issued assignment' })).toHaveAttribute(
      'href',
      `/homework/issued/${issueId ?? ''}`,
    );
  });

  test('a template that stopped being ready is refused in the dialog with a link to the builder', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const { pavel } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await openLibrary(page, pavel);
    await chooseRowAction(page, 'Fractions', 'Issue assignment');
    await issueDialog(page).getByRole('button', { name: 'Anna', exact: true }).click();
    await makeTemplateIncomplete(deployment, pavel, template, 'Fractions');
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/issues', status: 409 });

    await issueDialog(page).getByRole('button', { name: 'Issue', exact: true }).click();

    const alert = issueDialog(page).getByRole('alert');
    await expect(alert).toHaveText('This assignment is no longer ready to issue. Open it in the builder to fix it.');
    await expect(alert).toBeFocused();
    await expect(issueDialog(page).getByRole('link', { name: 'Open in the builder' })).toHaveAttribute(
      'href',
      `/homework/templates/${template.id}`,
    );
    expect(await issueTitles(deployment, pavel, 'status=active')).toEqual([]);
  });
  test('a past deadline in the dialog is refused under the deadline, whose date field takes focus', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const { pavel } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    const earlier = (await control.now()) - 3 * dayMilliseconds;
    await openLibrary(page, pavel);
    await chooseRowAction(page, 'Fractions', 'Issue assignment');
    await issueDialog(page).getByRole('button', { name: 'Anna', exact: true }).click();
    await issueDialog(page).getByRole('checkbox', { name: 'Set a deadline' }).check();
    await issueDialog(page).getByLabel('Date', { exact: true }).fill(localDate(earlier, moscow));
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/issues', status: 422 });

    await issueDialog(page).getByRole('button', { name: 'Issue', exact: true }).click();

    await expect(issueDialog(page).getByText('Choose a time in the future.', { exact: true })).toBeVisible();
    await expect(issueDialog(page).getByLabel('Date', { exact: true })).toBeFocused();
    expect(await issueTitles(deployment, pavel, 'status=active')).toEqual([]);
  });
});

test.describe('issuing from the builder, the card and the lesson', () => {
  test('the builder issues to a group and the notice takes focus', async ({ page, deployment }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = await seedActiveStudent(deployment, pavel, 'Anna');
    const boris = await seedActiveStudent(deployment, pavel, 'Boris');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId, boris.cardId]);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await signInThroughTheApi(page, pavel);
    await page.goto(`/homework/templates/${template.id}`);
    await page.getByRole('button', { name: 'Issue assignment', exact: true }).click();
    await issueDialog(page).getByRole('button', { name: 'Evening algebra', exact: true }).click();
    await issueDialog(page).getByText('2 students will receive this', { exact: true }).waitFor();

    await issueDialog(page).getByRole('button', { name: 'Issue', exact: true }).click();

    await expect(issueDialog(page)).toBeHidden();
    await expect(issuedNotice(page)).toBeFocused();
    const [issueId] = await issueIdsOf(deployment, pavel, `group=${groupId}&status=active`);
    expect((await readIssue(deployment, pavel, issueId ?? '')).students).toBe(2);
  });

  test('dismissing the builder’s dialog returns focus to its Issue assignment button', async ({ page, deployment }) => {
    const { pavel } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await signInThroughTheApi(page, pavel);
    await page.goto(`/homework/templates/${template.id}`);
    const opener = page.getByRole('button', { name: 'Issue assignment', exact: true });
    await opener.click();
    await audienceSearch(page).waitFor();

    await issueDialog(page).getByRole('button', { name: 'Cancel', exact: true }).click();

    await expect(issueDialog(page)).toBeHidden();
    await expect(opener).toBeFocused();
  });

  test('the card’s Issue assignment opens on the template search with the student preset', async ({
    page,
    deployment,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    await createIncompleteTemplate(deployment, pavel, 'Draft');
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${annaCard}`);
    await page.getByRole('heading', { level: 1, name: 'Anna' }).waitFor();

    await page.getByRole('button', { name: 'Issue assignment', exact: true }).click();

    await expect(issueDialog(page).getByRole('searchbox', { name: 'Search the library' })).toBeFocused();
    await expect(issueDialog(page).getByText('Anna', { exact: true })).toBeVisible();
    await expect(issueDialog(page).getByRole('button', { name: 'Draft' })).toHaveAttribute('aria-disabled', 'true');
    await expect(issueDialog(page).getByRole('button', { name: 'Draft' })).toHaveAccessibleDescription(
      /Only a ready assignment can be issued\./,
    );
  });

  test('the card issues a chosen template and its Overview lists it among the open assignments', async ({
    page,
    deployment,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${annaCard}`);
    await page.getByRole('button', { name: 'Issue assignment', exact: true }).click();
    await issueDialog(page).getByRole('button', { name: 'Fractions' }).click();

    await issueDialog(page).getByRole('button', { name: 'Issue', exact: true }).click();

    await expect(issueDialog(page)).toBeHidden();
    await expect(issuedNotice(page)).toBeFocused();
    const open = page.getByRole('list', { name: 'Open assignments', exact: true }).getByRole('listitem');
    await expect(open).toHaveCount(1);
    await expect(open).toContainText('Fractions');
    await expect(open).toContainText('Not submitted');
  });

  test('the lesson page issues with its lesson linked', async ({ page, deployment, control }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    const [lesson] = await scheduleLessonsWithIds(deployment, pavel, {
      relationship_id: annaCard,
      title: 'Algebra',
      date: addDays(localDate(await control.now(), moscow), 1),
      start: '18:00',
      duration_minutes: 60,
    });
    await signInThroughTheApi(page, pavel);
    await page.goto(`/lessons/${lesson?.id ?? ''}`);
    await page.getByRole('button', { name: 'Issue assignment', exact: true }).click();
    await issueDialog(page).getByRole('button', { name: 'Fractions' }).click();

    await issueDialog(page).getByRole('button', { name: 'Issue', exact: true }).click();

    await expect(issueDialog(page)).toBeHidden();
    await expect(issuedNotice(page)).toBeFocused();
    const [issueId] = await issueIdsOf(deployment, pavel, `student=${annaCard}&status=active`);
    expect((await readIssue(deployment, pavel, issueId ?? '')).lessonId).toBe(lesson?.id);
  });
  test('dismissing the card’s dialog returns focus to its Issue assignment button', async ({ page, deployment }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    await openCardIssueDialog(page, pavel, annaCard, 'Fractions');

    await page.keyboard.press('Escape');

    await expect(issueDialog(page)).toBeHidden();
    await expect(page.getByRole('button', { name: 'Issue assignment', exact: true })).toBeFocused();
  });

  test('dismissing the lesson page’s dialog returns focus to its Issue assignment button', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    const lesson = await lessonTomorrow(deployment, pavel, annaCard, await control.now());
    await signInThroughTheApi(page, pavel);
    await page.goto(`/lessons/${lesson}`);
    await page.getByRole('button', { name: 'Issue assignment', exact: true }).click();
    await issueDialog(page).getByRole('button', { name: 'Fractions', exact: true }).waitFor();

    await issueDialog(page).getByRole('button', { name: 'Cancel', exact: true }).click();

    await expect(issueDialog(page)).toBeHidden();
    await expect(page.getByRole('button', { name: 'Issue assignment', exact: true })).toBeFocused();
  });

  test('the builder’s button disappears when the flushed template is not ready, moving focus to the title', async ({
    page,
    deployment,
  }) => {
    const { pavel } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await signInThroughTheApi(page, pavel);
    await page.goto(`/homework/templates/${template.id}`);
    const opener = page.getByRole('button', { name: 'Issue assignment', exact: true });
    await opener.waitFor();
    const release = await holdSaves(page, template.id);
    const held = page.waitForRequest(
      (request) =>
        request.method() === 'PUT' && new URL(request.url()).pathname === `/api/v1/library/templates/${template.id}`,
    );
    await page.getByRole('textbox', { name: 'Question', exact: true }).fill('');
    await held;
    await opener.click();

    await release();

    await expect(opener).toHaveCount(0);
    await expect(page.getByRole('textbox', { name: 'Title', exact: true })).toBeFocused();
    await expect(issueDialog(page)).toHaveCount(0);
  });

  test('opening a folder in the template picker moves focus to the current path step', async ({ page, deployment }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const folderId = await createFolder(deployment, pavel, 'Algebra');
    await createReadyTemplate(deployment, pavel, 'Fractions', folderId);
    await openCardIssueDialog(page, pavel, annaCard, 'Algebra');

    await issueDialog(page).getByRole('button', { name: 'Algebra', exact: true }).click();

    await expect(issueDialog(page).locator('[aria-current="location"]')).toBeFocused();
    await expect(issueDialog(page).locator('[aria-current="location"]')).toHaveText('Algebra');
  });

  test('choosing a template in the picker moves focus to its Change button', async ({ page, deployment }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    await openCardIssueDialog(page, pavel, annaCard, 'Fractions');

    await issueDialog(page).getByRole('button', { name: 'Fractions' }).click();

    await expect(issueDialog(page).getByRole('button', { name: 'Change', exact: true }).first()).toBeFocused();
  });

  test('Change on a chosen template moves focus back to the picker’s search', async ({ page, deployment }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    await openCardIssueDialog(page, pavel, annaCard, 'Fractions');
    await issueDialog(page).getByRole('button', { name: 'Fractions' }).click();
    const change = issueDialog(page).getByRole('button', { name: 'Change', exact: true }).first();
    await change.waitFor();

    await change.click();

    await expect(issueDialog(page).getByRole('searchbox', { name: 'Search the library' })).toBeFocused();
  });

  test('a lesson that links 20 assignments is refused under the dialog’s lesson choice, which takes focus', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const lesson = await lessonTomorrow(deployment, pavel, annaCard, await control.now());
    await issueManyThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard }, 20, {
      lessonId: lesson,
    });
    await signInThroughTheApi(page, pavel);
    await page.goto(`/lessons/${lesson}`);
    await page.getByRole('button', { name: 'Issue assignment', exact: true }).click();
    await issueDialog(page).getByRole('button', { name: 'Fractions' }).click();
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/issues', status: 422 });

    await issueDialog(page).getByRole('button', { name: 'Issue', exact: true }).click();

    await expect(issueDialog(page).getByText('A lesson links at most 20 assignments.', { exact: true })).toBeVisible();
    await expect(issueDialog(page).getByRole('combobox', { name: 'Linked lesson' })).toBeFocused();
  });
});

test.describe('the issued work and the issue page', () => {
  test('the Issued tab lists active work with its audience and progress', async ({ page, deployment }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard });
    await signInThroughTheApi(page, pavel);

    await page.goto('/homework/issued');

    await expect(page.getByRole('link', { name: 'Issued', exact: true })).toHaveAttribute('aria-current', 'page');
    const rows = tableRows(page, 'Issued assignments');
    await expect(rows).toHaveCount(1);
    await expect(rows).toContainText('Fractions');
    await expect(rows).toContainText('Anna');
    await expect(rows).toContainText('0/1 submitted');
  });

  test('changing the deadline returns focus to the actions menu and announces it', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard });
    const later = (await control.now()) + 3 * dayMilliseconds;
    await openIssuePage(page, pavel, issueId, 'Fractions');
    await chooseFromPageMenu(page, 'Assignment actions', 'Change deadline');
    const dialog = page.getByRole('dialog', { name: 'Change deadline' });
    await fillDeadline(dialog, later);

    await dialog.getByRole('button', { name: 'Save', exact: true }).click();

    await expect(dialog).toBeHidden();
    await expect(pageMenuButton(page, 'Assignment actions')).toBeFocused();
    await expect(page.getByRole('status').filter({ hasText: 'Deadline changed' })).toBeVisible();
    await expect.poll(async () => (await readIssue(deployment, pavel, issueId)).deadlineAt).not.toBeNull();
  });

  test('a past deadline is refused under the field, which takes focus', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard });
    const earlier = (await control.now()) - 3 * dayMilliseconds;
    await openIssuePage(page, pavel, issueId, 'Fractions');
    await chooseFromPageMenu(page, 'Assignment actions', 'Change deadline');
    const dialog = page.getByRole('dialog', { name: 'Change deadline' });
    await fillDeadline(dialog, earlier);
    diagnostics.expectFailure({ method: 'PATCH', path: `/api/v1/issues/${issueId}`, status: 422 });

    await dialog.getByRole('button', { name: 'Save', exact: true }).click();

    await expect(dialog.getByText('Choose a time in the future.', { exact: true })).toBeVisible();
    await expect(dialog.getByLabel('Date', { exact: true })).toBeFocused();
    expect((await readIssue(deployment, pavel, issueId)).deadlineAt).toBeNull();
  });

  test('changing the linked lesson returns focus to the actions menu and links the lesson', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const [lesson] = await scheduleLessonsWithIds(deployment, pavel, {
      relationship_id: annaCard,
      title: 'Algebra',
      date: addDays(localDate(await control.now(), moscow), 1),
      start: '18:00',
      duration_minutes: 60,
    });
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard });
    await openIssuePage(page, pavel, issueId, 'Fractions');
    await chooseFromPageMenu(page, 'Assignment actions', 'Change linked lesson');
    const dialog = page.getByRole('dialog', { name: 'Change linked lesson' });
    const choice = dialog.getByRole('combobox', { name: 'Linked lesson' });
    await choice.selectOption({ value: lesson?.id ?? '' });

    await dialog.getByRole('button', { name: 'Save', exact: true }).click();

    await expect(dialog).toBeHidden();
    await expect(pageMenuButton(page, 'Assignment actions')).toBeFocused();
    await expect(page.getByRole('status').filter({ hasText: 'Linked lesson changed' })).toBeVisible();
    await expect(page.getByRole('link', { name: /Algebra/ })).toHaveAttribute('href', `/lessons/${lesson?.id ?? ''}`);
  });

  test('a change made elsewhere closes the open dialog onto the focused changed-meanwhile notice', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard });
    await openIssuePage(page, pavel, issueId, 'Fractions');
    await chooseFromPageMenu(page, 'Assignment actions', 'Change deadline');
    const dialog = page.getByRole('dialog', { name: 'Change deadline' });
    await dialog.waitFor();

    await changeDeadlineThroughTheApi(
      deployment,
      pavel,
      issueId,
      new Date((await control.now()) + 2 * dayMilliseconds).toISOString(),
    );

    await expect(dialog).toBeHidden();
    await expect(
      page.getByText('This assignment changed in the meantime. Its current state is shown below.', { exact: true }),
    ).toBeFocused();
  });

  test('each issue action’s dialog opens on its first field', async ({ page, deployment }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard });
    await openIssuePage(page, pavel, issueId, 'Fractions');

    await chooseFromPageMenu(page, 'Assignment actions', 'Change deadline');

    await expect(
      page.getByRole('dialog', { name: 'Change deadline' }).getByLabel('Date', { exact: true }),
    ).toBeFocused();
  });

  test('Change linked lesson opens on its lesson choice', async ({ page, deployment }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard });
    await openIssuePage(page, pavel, issueId, 'Fractions');

    await chooseFromPageMenu(page, 'Assignment actions', 'Change linked lesson');

    await expect(
      page.getByRole('dialog', { name: 'Change linked lesson' }).getByRole('combobox', { name: 'Linked lesson' }),
    ).toBeFocused();
  });

  test('a lesson that links 20 assignments is refused under the choice, which takes focus', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const lesson = await lessonTomorrow(deployment, pavel, annaCard, await control.now());
    await issueManyThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard }, 20, {
      lessonId: lesson,
    });
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard });
    await openIssuePage(page, pavel, issueId, 'Fractions');
    await chooseFromPageMenu(page, 'Assignment actions', 'Change linked lesson');
    const dialog = page.getByRole('dialog', { name: 'Change linked lesson' });
    const choice = dialog.getByRole('combobox', { name: 'Linked lesson' });
    await choice.selectOption({ value: lesson });
    diagnostics.expectFailure({ method: 'PATCH', path: `/api/v1/issues/${issueId}`, status: 422 });

    await dialog.getByRole('button', { name: 'Save', exact: true }).click();

    await expect(dialog.getByText('A lesson links at most 20 assignments.', { exact: true })).toBeVisible();
    await expect(choice).toBeFocused();
  });

  test('choosing Withdraw opens the confirmation on Cancel', async ({ page, deployment }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard });
    await openIssuePage(page, pavel, issueId, 'Fractions');

    await chooseFromPageMenu(page, 'Assignment actions', 'Withdraw');

    await expect(
      page
        .getByRole('dialog', { name: 'Withdraw this assignment?' })
        .getByRole('button', { name: 'Cancel', exact: true }),
    ).toBeFocused();
  });

  test('cancelling the withdrawal returns focus to the actions menu', async ({ page, deployment }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard });
    await openIssuePage(page, pavel, issueId, 'Fractions');
    await chooseFromPageMenu(page, 'Assignment actions', 'Withdraw');
    const dialog = page.getByRole('dialog', { name: 'Withdraw this assignment?' });
    await dialog.waitFor();

    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();

    await expect(dialog).toBeHidden();
    await expect(pageMenuButton(page, 'Assignment actions')).toBeFocused();
    expect((await readIssue(deployment, pavel, issueId)).status).toBe('active');
  });

  test('a withdrawal made elsewhere while focus is on a menu item moves focus to the page heading', async ({
    page,
    deployment,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard });
    await openIssuePage(page, pavel, issueId, 'Fractions');
    const items = await openPageMenu(page, 'Assignment actions');
    await items.getByRole('button', { name: 'Withdraw', exact: true }).focus();

    await withdrawThroughTheApi(deployment, pavel, issueId);

    await expect(pageMenuButton(page, 'Assignment actions')).toHaveCount(0);
    await expect(page.getByRole('heading', { level: 1, name: 'Fractions' })).toBeFocused();
  });

  test('a withdrawal refused because a student submitted meanwhile shows the focused alert', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student: anna, cardId } = await seedActiveStudent(deployment, pavel, 'Anna');
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: cardId });
    await openIssuePage(page, pavel, issueId, 'Fractions');
    await chooseFromPageMenu(page, 'Assignment actions', 'Withdraw');
    const dialog = page.getByRole('dialog', { name: 'Withdraw this assignment?' });
    await dialog.waitFor();
    await submitThroughTheApi(deployment, anna, await onlySubmission(deployment, anna));
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/issues/${issueId}/withdraw`, status: 409 });

    await dialog.getByRole('button', { name: 'Withdraw', exact: true }).click();

    await expect(dialog).toBeHidden();
    const alert = page.getByText('A student has already submitted this assignment, so it can no longer be withdrawn.', {
      exact: true,
    });
    await expect(alert).toBeFocused();
    expect((await readIssue(deployment, pavel, issueId)).status).toBe('active');
  });

  test('withdrawing closes the confirmation onto the focused notice and removes the actions', async ({
    page,
    deployment,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard });
    await openIssuePage(page, pavel, issueId, 'Fractions');
    await chooseFromPageMenu(page, 'Assignment actions', 'Withdraw');
    const dialog = page.getByRole('dialog', { name: 'Withdraw this assignment?' });

    await dialog.getByRole('button', { name: 'Withdraw', exact: true }).click();

    await expect(dialog).toBeHidden();
    await expect(page.getByText('The assignment is withdrawn.', { exact: true })).toBeFocused();
    await expect(pageMenuButton(page, 'Assignment actions')).toHaveCount(0);
    await expect(page.locator('main header')).toContainText('Withdrawn');
    expect((await readIssue(deployment, pavel, issueId)).status).toBe('withdrawn');
  });
});

test.describe('homework on the student card and the group page', () => {
  test('the group page lists its issued work and the Add students dialog notes that it does not reach new members', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = await seedActiveStudent(deployment, pavel, 'Anna');
    await seedActiveStudent(deployment, pavel, 'Boris');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId]);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await issueThroughTheApi(deployment, pavel, template.id, { group_id: groupId });
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);
    const assignments = page.getByRole('list', { name: 'Assignments', exact: true }).getByRole('listitem');
    await assignments.first().waitFor();

    await page.getByRole('button', { name: 'Add students', exact: true }).click();

    await expect(assignments).toHaveCount(1);
    await expect(assignments).toContainText('Fractions');
    await expect(assignments).toContainText('0/1 submitted');
    await expect(
      page
        .getByRole('dialog', { name: 'Add students' })
        .getByText('Students you add now do not get assignments issued earlier.', { exact: true }),
    ).toBeVisible();
  });

  test('Show more on the Issued tab moves focus to the first new row', async ({ page, deployment }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await issueManyThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard }, 21);
    await signInThroughTheApi(page, pavel);
    await page.goto('/homework/issued');
    const rows = tableRows(page, 'Issued assignments');
    await rows.nth(19).waitFor();

    await page.getByRole('button', { name: 'Show more', exact: true }).click();

    await expect(rows).toHaveCount(21);
    await expect(rows.nth(20).getByRole('link')).toBeFocused();
  });

  test('Show more on the card’s Homework tab moves focus to the first new row', async ({ page, deployment }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await issueManyThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard }, 21);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/students/${annaCard}?tab=homework`);
    const rows = page.getByRole('list', { name: 'Homework', exact: true }).getByRole('listitem');
    await rows.nth(19).waitFor();

    await page.getByRole('button', { name: 'Show more', exact: true }).click();

    await expect(rows).toHaveCount(21);
    await expect(rows.nth(20).locator('[data-row-focus]')).toBeFocused();
    await expect(rows.first()).toContainText('Fractions');
    await expect(rows.first()).toContainText('Not submitted');
  });

  test('Show more in the group’s assignments moves focus to the first new row', async ({ page, deployment }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [annaCard]);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await issueManyThroughTheApi(deployment, pavel, template.id, { group_id: groupId }, 21);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);
    const rows = page.getByRole('list', { name: 'Assignments', exact: true }).getByRole('listitem');
    await rows.nth(19).waitFor();

    await page.getByRole('button', { name: 'Show more', exact: true }).click();

    await expect(rows).toHaveCount(21);
    await expect(rows.nth(20).getByRole('link')).toBeFocused();
  });
});
