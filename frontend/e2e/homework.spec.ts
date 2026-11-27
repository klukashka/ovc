import type { Locator, Page } from '@playwright/test';

import { type TestAccount, newTestAccount, signInThroughTheApi } from './accounts.ts';
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
import { expectDangerConfirmation, expectLastDangerItem, openedMenu } from './dangerActions.ts';
import {
  changeDeadlineThroughTheApi,
  createFolder,
  createIncompleteTemplate,
  createReadyHandCheckedTemplate,
  createReadyTemplate,
  issueIdsOf,
  issueManyThroughTheApi,
  issueThroughTheApi,
  issueTitles,
  makeTemplateIncomplete,
  readIssue,
  withdrawThroughTheApi,
} from './homework.ts';
import { onlySubmission, studentWork as studentWorkTitles, submitThroughTheApi } from './assignment.ts';
import { expectRefetchedAfterTheSocketIsReady, trafficAfterReady } from './realtime.ts';
import { studentLessonIssues } from './results.ts';
import { addDays, localDate, scheduleLessonsWithIds } from './schedule.ts';
import { installPageClock, skipSaveDelay } from './save-delay.ts';
import {
  createCard,
  createGroup,
  endRelationship,
  removeStudent,
  seedActiveStudent,
  signUpOnInvitation,
} from './tutoring.ts';

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
  await page.getByRole('main').getByRole('button', { name: listed, exact: true }).waitFor();
}

async function layoutBox(
  locator: Locator,
): Promise<{ readonly x: number; readonly y: number; readonly width: number; readonly height: number }> {
  const box = await locator.boundingBox();
  if (box === null) {
    throw new Error('the element has no layout box');
  }
  return box;
}

function issueDialog(page: Page): Locator {
  return page.getByRole('main');
}

function issueHeading(page: Page): Locator {
  return page.getByRole('heading', { level: 1, name: 'Issue assignment' });
}

function audienceSearch(page: Page): Locator {
  return issueDialog(page).getByRole('searchbox', { name: 'Search for a student or group' });
}

function noManualQuestionHint(page: Page): Locator {
  return issueDialog(page).getByText(
    'This assignment has no questions you check by hand, so students see the answers right after submitting.',
    { exact: true },
  );
}

async function chooseTemplate(page: Page, title: string): Promise<void> {
  await issueDialog(page).getByRole('button', { name: title }).click();
  await expect(noManualQuestionHint(page)).toBeVisible();
}

async function holdTemplateRead(
  page: Page,
  templateId: string,
): Promise<{ readonly held: Promise<void>; readonly release: () => Promise<void> }> {
  const address = `**/api/v1/library/templates/${templateId}`;
  let arrived: () => void = () => undefined;
  const held = new Promise<void>((resolve) => {
    arrived = resolve;
  });
  let resume: () => void = () => undefined;
  const resumed = new Promise<void>((resolve) => {
    resume = resolve;
  });
  await page.route(address, async (route) => {
    if (route.request().method() === 'GET') {
      arrived();
      await resumed;
    }
    await route.continue();
  });
  return {
    held,
    release: async (): Promise<void> => {
      const answered = page.waitForResponse(
        (response) =>
          response.request().method() === 'GET' && new URL(response.url()).pathname.endsWith(`/${templateId}`),
      );
      resume();
      await answered;
      await page.unroute(address);
    },
  };
}

async function holdGroupRead(
  page: Page,
  groupId: string,
): Promise<{ readonly held: Promise<void>; readonly release: () => Promise<void> }> {
  const address = `**/api/v1/groups/${groupId}`;
  let arrived: () => void = () => undefined;
  const held = new Promise<void>((resolve) => {
    arrived = resolve;
  });
  let resume: () => void = () => undefined;
  const resumed = new Promise<void>((resolve) => {
    resume = resolve;
  });
  await page.route(address, async (route) => {
    if (route.request().method() === 'GET') {
      arrived();
      await resumed;
    }
    await route.continue();
  });
  return {
    held,
    release: async (): Promise<void> => {
      const answered = page.waitForResponse(
        (response) =>
          response.request().method() === 'GET' && new URL(response.url()).pathname === `/api/v1/groups/${groupId}`,
      );
      resume();
      await answered;
      await page.unroute(address);
    },
  };
}

function issueButton(page: Page): Locator {
  return issueDialog(page).getByRole('button', { name: 'Issue', exact: true });
}

async function chooseWhileTheTemplateReadIsHeld(
  page: Page,
  cardId: string,
  tutor: TestAccount,
  templateId: string,
  title: string,
): Promise<{
  readonly read: { readonly release: () => Promise<void> };
  readonly issueButtonBox: Awaited<ReturnType<typeof layoutBox>>;
}> {
  await signInThroughTheApi(page, tutor);
  await page.goto(`/students/${cardId}`);
  await page.getByRole('button', { name: 'Issue assignment', exact: true }).click();
  const read = await holdTemplateRead(page, templateId);
  await issueDialog(page).getByRole('button', { name: title }).click();
  await read.held;
  await expect(issueButton(page)).toHaveAttribute('aria-disabled', 'false');
  await expect(noManualQuestionHint(page)).toBeHidden();
  return { read, issueButtonBox: await layoutBox(issueButton(page)) };
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

async function holdIssueChanges(page: Page, issueId: string): Promise<() => Promise<void>> {
  let resume: () => void = () => undefined;
  const resumed = new Promise<void>((resolve) => {
    resume = resolve;
  });
  const address = `**/api/v1/issues/${issueId}`;
  await page.route(address, async (route) => {
    if (route.request().method() === 'PATCH') {
      await resumed;
    }
    await route.continue();
  });
  return async () => {
    const answered = page.waitForResponse(
      (response) =>
        response.request().method() === 'PATCH' && new URL(response.url()).pathname === `/api/v1/issues/${issueId}`,
    );
    resume();
    await answered;
    await page.unroute(address);
  };
}

function changedMeanwhileNotice(page: Page): Locator {
  return page.getByText('This assignment changed in the meantime. Its current state is shown below.', { exact: true });
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

  test('Issue assignment opens the Issue page on its heading with the template preset and warns under After checking that a template without hand-checked questions reveals answers at submission', async ({
    page,
    deployment,
  }) => {
    const { pavel } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await openLibrary(page, pavel);

    await chooseRowAction(page, 'Fractions', 'Issue assignment');

    await expect(issueHeading(page)).toBeFocused();
    await expect(page).toHaveURL(`/homework/issue?template=${template.id}`);
    await expect(audienceSearch(page)).toBeVisible();
    await expect(issueDialog(page).getByRole('link', { name: 'Back to the library' })).toBeVisible();
    await expect(issueDialog(page).getByText('Fractions', { exact: true })).toBeVisible();
    const afterChecking = issueDialog(page).getByRole('radio', { name: 'After checking' });
    await expect(afterChecking).toBeChecked();
    await expect(afterChecking).toHaveAccessibleDescription(
      'This assignment has no questions you check by hand, so students see the answers right after submitting.',
    );
    await expect(issueDialog(page).getByRole('radio', { name: 'After submission' })).toHaveAccessibleDescription(
      'Students see the correct answers as soon as they submit, so work cannot be returned for revision.',
    );
  });

  test('Back from the Issue page returns focus to the row’s menu button', async ({ page, deployment }) => {
    const { pavel } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    await openLibrary(page, pavel);
    await chooseRowAction(page, 'Fractions', 'Issue assignment');
    await audienceSearch(page).waitFor();

    await page.goBack();

    await expect(issueHeading(page)).toBeHidden();
    await expect(rowMenuButton(page, 'Fractions')).toBeFocused();
  });

  test('issuing to a student returns to the library onto the focused notice with a link to the issue', async ({
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

    await expect(issueHeading(page)).toBeHidden();
    await expect(issuedNotice(page)).toBeFocused();
    const [issueId] = await issueIdsOf(deployment, pavel, `student=${annaCard}&status=active`);
    await expect(page.getByRole('link', { name: 'Open the issued assignment' })).toHaveAttribute(
      'href',
      `/homework/issued/${issueId ?? ''}`,
    );
  });

  test('a template that stopped being ready is refused on the Issue page with a link to the builder', async ({
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

  test('choosing a student on the Issue page moves focus to the audience’s Change button', async ({
    page,
    deployment,
  }) => {
    const { pavel } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    await openLibrary(page, pavel);
    await chooseRowAction(page, 'Fractions', 'Issue assignment');
    await audienceSearch(page).waitFor();

    await issueDialog(page).getByRole('button', { name: 'Anna', exact: true }).click();

    await expect(issueDialog(page).getByRole('button', { name: 'Change', exact: true }).last()).toBeFocused();
    await expect(issueDialog(page).getByText('1 student will receive this', { exact: true })).toBeVisible();
  });

  test('a student whose relationship ended meanwhile is refused on the Issue page with an alert that takes focus', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    await openLibrary(page, pavel);
    await chooseRowAction(page, 'Fractions', 'Issue assignment');
    await issueDialog(page).getByRole('button', { name: 'Anna', exact: true }).click();
    await issueDialog(page).getByText('1 student will receive this', { exact: true }).waitFor();
    await endRelationship(deployment, pavel, annaCard);
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/issues', status: 409 });

    await issueDialog(page).getByRole('button', { name: 'Issue', exact: true }).click();

    const alert = issueDialog(page).getByRole('alert');
    await expect(alert).toHaveText('This student can no longer get assignments.');
    await expect(alert).toBeFocused();
    expect(await issueTitles(deployment, pavel, 'status=active')).toEqual([]);
  });

  test('a past deadline is refused under the deadline and in the error summary, which takes focus', async ({
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

    const summary = issueDialog(page).getByRole('alert', { name: 'Check these fields' });
    await expect(summary).toBeFocused();
    await expect(summary.getByRole('link')).toHaveText(['Choose a time in the future.']);
    await expect(issueDialog(page).getByRole('group', { name: 'Deadline' })).toContainText(
      'Choose a time in the future.',
    );
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

    await expect(issueHeading(page)).toBeHidden();
    await expect(issuedNotice(page)).toBeFocused();
    const [issueId] = await issueIdsOf(deployment, pavel, `group=${groupId}&status=active`);
    expect((await readIssue(deployment, pavel, issueId ?? '')).students).toBe(2);
  });

  test('the group summary that arrives after choosing a group leaves the Issue button in place', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = await seedActiveStudent(deployment, pavel, 'Anna');
    const boris = await seedActiveStudent(deployment, pavel, 'Boris');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [anna.cardId, boris.cardId]);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await signInThroughTheApi(page, pavel);
    await page.goto(`/homework/templates/${template.id}`);
    await page.getByRole('button', { name: 'Issue assignment', exact: true }).click();
    await expect(noManualQuestionHint(page)).toBeVisible();
    const read = await holdGroupRead(page, groupId);
    await issueDialog(page).getByRole('button', { name: 'Evening algebra', exact: true }).click();
    await read.held;
    await expect(issueButton(page)).toHaveAttribute('aria-disabled', 'false');
    await expect(issueDialog(page).getByText('2 students will receive this', { exact: true })).toHaveCount(0);
    await expect(issueButton(page)).not.toHaveAttribute('aria-describedby');
    const issueButtonBox = await layoutBox(issueButton(page));

    await read.release();

    await expect(issueDialog(page).getByText('2 students will receive this', { exact: true })).toBeVisible();
    await expect(issueButton(page)).toHaveAccessibleDescription('2 students will receive this');
    expect(await layoutBox(issueButton(page))).toEqual(issueButtonBox);
  });

  test('Cancel on the Issue page returns focus to the builder’s Issue assignment button', async ({
    page,
    deployment,
  }) => {
    const { pavel } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await signInThroughTheApi(page, pavel);
    await page.goto(`/homework/templates/${template.id}`);
    const opener = page.getByRole('button', { name: 'Issue assignment', exact: true });
    await opener.click();
    await audienceSearch(page).waitFor();

    await issueDialog(page).getByRole('button', { name: 'Cancel', exact: true }).click();

    await expect(issueHeading(page)).toBeHidden();
    await expect(opener).toBeFocused();
  });

  test('the card’s Issue assignment opens the Issue page on its heading with the student preset', async ({
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

    await expect(issueHeading(page)).toBeFocused();
    await expect(issueDialog(page).getByRole('searchbox', { name: 'Search the library' })).toBeVisible();
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
    await chooseTemplate(page, 'Fractions');

    await issueDialog(page).getByRole('button', { name: 'Issue', exact: true }).click();

    await expect(issueHeading(page)).toBeHidden();
    await expect(issuedNotice(page)).toBeFocused();
    const open = page.getByRole('list', { name: 'Open assignments', exact: true }).getByRole('listitem');
    await expect(open).toHaveCount(1);
    await expect(open).toContainText('Fractions');
    await expect(open).toContainText('Not submitted');
  });

  test('the answer-visibility hint that arrives after choosing a template leaves the Issue button in place', async ({
    page,
    deployment,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const pending = await chooseWhileTheTemplateReadIsHeld(page, annaCard, pavel, template.id, 'Fractions');

    await pending.read.release();

    await expect(noManualQuestionHint(page)).toBeVisible();
    expect(await layoutBox(issueButton(page))).toEqual(pending.issueButtonBox);
  });

  test('a template with a question checked by hand keeps the hint hidden and the Issue button in place', async ({
    page,
    deployment,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyHandCheckedTemplate(deployment, pavel, 'Essay');
    const pending = await chooseWhileTheTemplateReadIsHeld(page, annaCard, pavel, template.id, 'Essay');

    await pending.read.release();

    await expect(noManualQuestionHint(page)).toBeHidden();
    await expect(issueDialog(page).getByRole('radio', { name: 'After checking' })).not.toHaveAttribute(
      'aria-describedby',
    );
    expect(await layoutBox(issueButton(page))).toEqual(pending.issueButtonBox);
  });

  test('issuing from the empty assignments of a group returns to the group page onto the focused notice', async ({
    page,
    deployment,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [annaCard]);
    await signInThroughTheApi(page, pavel);
    await page.goto(`/groups/${groupId}`);
    await page.getByRole('button', { name: 'Issue to this group', exact: true }).click();
    await chooseTemplate(page, 'Fractions');

    await issueDialog(page).getByRole('button', { name: 'Issue', exact: true }).click();

    await expect(issueHeading(page)).toBeHidden();
    await expect(page.getByRole('heading', { level: 1, name: 'Evening algebra' })).toBeVisible();
    await expect(issuedNotice(page)).toBeFocused();
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
    await chooseTemplate(page, 'Fractions');

    await issueDialog(page).getByRole('button', { name: 'Issue', exact: true }).click();

    await expect(issueHeading(page)).toBeHidden();
    await expect(issuedNotice(page)).toBeFocused();
    const [issueId] = await issueIdsOf(deployment, pavel, `student=${annaCard}&status=active`);
    expect((await readIssue(deployment, pavel, issueId ?? '')).lessonId).toBe(lesson?.id);
  });
  test('Back from the card’s Issue page returns focus to its Issue assignment button', async ({ page, deployment }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    await openCardIssueDialog(page, pavel, annaCard, 'Fractions');

    await page.goBack();

    await expect(issueHeading(page)).toBeHidden();
    await expect(page.getByRole('button', { name: 'Issue assignment', exact: true })).toBeFocused();
  });

  test('Cancel on the lesson page’s Issue page returns focus to its Issue assignment button', async ({
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

    await expect(issueHeading(page)).toBeHidden();
    await expect(page.getByRole('button', { name: 'Issue assignment', exact: true })).toBeFocused();
  });

  test('the builder’s button disappears when the flushed template is not ready, moving focus to the title', async ({
    page,
    deployment,
  }) => {
    const { pavel } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await installPageClock(page, { pageClock: true });
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
    await skipSaveDelay(page);
    await held;
    await opener.click();

    await release();

    await expect(opener).toHaveCount(0);
    await expect(page.getByRole('textbox', { name: 'Title', exact: true })).toBeFocused();
    await expect(issueHeading(page)).toHaveCount(0);
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

  test('a lesson that links 20 assignments is refused under the lesson choice and in the error summary, which takes focus', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const lesson = await lessonTomorrow(deployment, pavel, annaCard, await control.now());
    await issueManyThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard }, 19, {
      lessonId: lesson,
    });
    await signInThroughTheApi(page, pavel);
    await page.goto(`/lessons/${lesson}`);
    await page.getByRole('button', { name: 'Issue assignment', exact: true }).click();
    await chooseTemplate(page, 'Fractions');
    await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard }, { lessonId: lesson });
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/issues', status: 422 });

    await issueDialog(page).getByRole('button', { name: 'Issue', exact: true }).click();

    const summary = issueDialog(page).getByRole('alert', { name: 'Check these fields' });
    await expect(summary).toBeFocused();
    await expect(summary.getByRole('link')).toHaveText(['A lesson links at most 20 assignments.']);
    await expect(issueDialog(page).getByRole('combobox', { name: 'Linked lesson' })).toHaveAccessibleDescription(
      'A lesson links at most 20 assignments.',
    );
  });
});

function assignmentsSection(page: Page): Locator {
  return page.getByRole('region', { name: 'Assignments', exact: true });
}

function assignmentRows(page: Page): Locator {
  return assignmentsSection(page).getByRole('listitem');
}

async function lessonWithAnna(
  deployment: Deployment,
  now: number,
): Promise<{
  readonly pavel: TestAccount;
  readonly anna: TestAccount;
  readonly annaCard: string;
  readonly lesson: string;
}> {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { student, cardId } = await seedActiveStudent(deployment, pavel, 'Anna', 'en');
  const lesson = await lessonTomorrow(deployment, pavel, cardId, now);
  return { pavel, anna: student, annaCard: cardId, lesson };
}

test.describe('the lesson page’s assignments', () => {
  test('the tutor sees each linked assignment with its deadline and progress', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, anna, annaCard, lesson } = await lessonWithAnna(deployment, await control.now());
    const fractions = await createReadyTemplate(deployment, pavel, 'Fractions');
    const decimals = await createReadyTemplate(deployment, pavel, 'Decimals');
    const deadline = new Date((await control.now()) + 3 * dayMilliseconds).toISOString();
    await issueThroughTheApi(deployment, pavel, fractions.id, { relationship_id: annaCard }, { lessonId: lesson });
    await issueThroughTheApi(
      deployment,
      pavel,
      decimals.id,
      { relationship_id: annaCard },
      { lessonId: lesson, deadlineAt: deadline },
    );
    const [decimalsWork] = (await studentWorkTitles(deployment, anna, 'todo')).filter(
      (work) => work.title === 'Decimals',
    );
    await submitThroughTheApi(deployment, anna, decimalsWork?.submissionId ?? '');
    await signInThroughTheApi(page, pavel);

    await page.goto(`/lessons/${lesson}`);

    await expect(assignmentRows(page)).toHaveCount(2);
    await expect(assignmentRows(page).filter({ hasText: 'Fractions' })).toContainText('No deadline');
    await expect(assignmentRows(page).filter({ hasText: 'Fractions' })).toContainText('0/1 submitted');
    await expect(assignmentRows(page).filter({ hasText: 'Decimals' })).toContainText('1/1 submitted');
    await expect(assignmentsSection(page).getByRole('link', { name: 'Fractions' })).toHaveAttribute(
      'href',
      /^\/homework\/issued\//,
    );
  });

  test('a lesson without linked assignments says so to the tutor and shows no section to the student', async ({
    observedBrowser: browser,
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const { pavel, anna, lesson } = await lessonWithAnna(deployment, await control.now());
    await signInThroughTheApi(page, pavel);
    const studentContext = await browser.newContext({ baseURL: deployment.origin, ignoreHTTPSErrors: true });
    const studentPage = await studentContext.newPage();
    await diagnostics.watch(studentPage);
    await signInThroughTheApi(studentPage, anna);

    await page.goto(`/lessons/${lesson}`);
    await studentPage.goto(`/lessons/${lesson}`);

    await expect(assignmentsSection(page)).toContainText('No assignments for this lesson');
    await studentPage.getByRole('heading', { level: 1, name: 'Algebra' }).waitFor();
    await expect(studentPage.getByRole('heading', { name: 'Assignments' })).toHaveCount(0);
    await diagnostics.closeContext(studentContext);
  });

  test('with 20 linked assignments Issue assignment is unavailable and says why', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, annaCard, lesson } = await lessonWithAnna(deployment, await control.now());
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await issueManyThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard }, 20, {
      lessonId: lesson,
    });
    await signInThroughTheApi(page, pavel);
    await page.goto(`/lessons/${lesson}`);
    const opener = page.getByRole('button', { name: 'Issue assignment', exact: true });
    await opener.focus();

    await page.keyboard.press('Enter');

    await expect(opener).toHaveAttribute('aria-disabled', 'true');
    await expect(opener).toHaveAccessibleDescription('A lesson links at most 20 assignments.');
    await expect(issueHeading(page)).toHaveCount(0);
  });

  test('the student sees their own work for the lesson with its state and opens it', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, anna, annaCard, lesson } = await lessonWithAnna(deployment, await control.now());
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard }, { lessonId: lesson });
    const submissionId = await onlySubmission(deployment, anna);
    await signInThroughTheApi(page, anna);

    await page.goto(`/lessons/${lesson}`);

    await expect(assignmentRows(page)).toHaveCount(1);
    await expect(assignmentRows(page)).toContainText('To do');
    await expect(assignmentRows(page)).toContainText('No deadline');
    await expect(assignmentsSection(page).getByRole('link', { name: 'Fractions' })).toHaveAttribute(
      'href',
      `/homework/${submissionId}`,
    );
    expect(await studentLessonIssues(deployment, anna, lesson)).toHaveLength(1);
  });

  test('work linked while the student’s lesson page is open appears, and its withdrawal moves focus to the heading', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, anna, annaCard, lesson } = await lessonWithAnna(deployment, await control.now());
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    await signInThroughTheApi(page, anna);
    const traffic = trafficAfterReady(page, [`/api/v1/lessons/${lesson}`]);
    await traffic.initialize();
    await page.goto(`/lessons/${lesson}`);
    await page.getByRole('heading', { level: 1, name: 'Algebra' }).waitFor();
    await expectRefetchedAfterTheSocketIsReady(traffic);
    const issueId = await issueThroughTheApi(
      deployment,
      pavel,
      template.id,
      { relationship_id: annaCard },
      { lessonId: lesson },
    );
    await assignmentsSection(page).getByRole('link', { name: 'Fractions' }).focus();

    await withdrawThroughTheApi(deployment, pavel, issueId);

    await expect(assignmentsSection(page)).toHaveCount(0);
    await expect(page.getByRole('heading', { level: 1, name: 'Algebra' })).toBeFocused();
  });

  test('a withdrawn assignment leaves the tutor’s list and focus moves to the row that took its place', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, annaCard, lesson } = await lessonWithAnna(deployment, await control.now());
    const fractions = await createReadyTemplate(deployment, pavel, 'Fractions');
    const decimals = await createReadyTemplate(deployment, pavel, 'Decimals');
    const first = await issueThroughTheApi(
      deployment,
      pavel,
      fractions.id,
      { relationship_id: annaCard },
      { lessonId: lesson },
    );
    const second = await issueThroughTheApi(
      deployment,
      pavel,
      decimals.id,
      { relationship_id: annaCard },
      { lessonId: lesson },
    );
    await signInThroughTheApi(page, pavel);
    const traffic = trafficAfterReady(page, [`/api/v1/lessons/${lesson}`]);
    await traffic.initialize();
    await page.goto(`/lessons/${lesson}`);
    await expect(assignmentRows(page)).toHaveCount(2);
    await expectRefetchedAfterTheSocketIsReady(traffic);
    const firstTitle = (await assignmentRows(page).first().getByRole('link').innerText()).trim();
    const firstId = firstTitle === 'Fractions' ? first : second;
    const remaining = firstTitle === 'Fractions' ? 'Decimals' : 'Fractions';
    await assignmentRows(page).first().getByRole('link').focus();

    await withdrawThroughTheApi(deployment, pavel, firstId);

    await expect(assignmentRows(page)).toHaveCount(1);
    await expect(assignmentsSection(page).getByRole('link', { name: remaining })).toBeFocused();
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

  test('a deadline change refused because the assignment changed meanwhile closes onto the focused notice', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard });
    const now = await control.now();
    await openIssuePage(page, pavel, issueId, 'Fractions');
    await chooseFromPageMenu(page, 'Assignment actions', 'Change deadline');
    const dialog = page.getByRole('dialog', { name: 'Change deadline' });
    await fillDeadline(dialog, now + 3 * dayMilliseconds);
    const release = await holdIssueChanges(page, issueId);
    const sent = page.waitForRequest(
      (request) => request.method() === 'PATCH' && new URL(request.url()).pathname === `/api/v1/issues/${issueId}`,
    );
    diagnostics.expectFailure({ method: 'PATCH', path: `/api/v1/issues/${issueId}`, status: 409 });

    await dialog.getByRole('button', { name: 'Save', exact: true }).click();
    await sent;
    await changeDeadlineThroughTheApi(deployment, pavel, issueId, new Date(now + 2 * dayMilliseconds).toISOString());
    await release();

    await expect(dialog).toBeHidden();
    await expect(changedMeanwhileNotice(page)).toBeFocused();
  });

  test('a linked lesson change refused because the assignment changed meanwhile closes onto the focused notice', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const now = await control.now();
    const lesson = await lessonTomorrow(deployment, pavel, annaCard, now);
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: annaCard });
    await openIssuePage(page, pavel, issueId, 'Fractions');
    await chooseFromPageMenu(page, 'Assignment actions', 'Change linked lesson');
    const dialog = page.getByRole('dialog', { name: 'Change linked lesson' });
    await dialog.getByRole('combobox', { name: 'Linked lesson' }).selectOption({ value: lesson });
    const release = await holdIssueChanges(page, issueId);
    const sent = page.waitForRequest(
      (request) => request.method() === 'PATCH' && new URL(request.url()).pathname === `/api/v1/issues/${issueId}`,
    );
    diagnostics.expectFailure({ method: 'PATCH', path: `/api/v1/issues/${issueId}`, status: 409 });

    await dialog.getByRole('button', { name: 'Save', exact: true }).click();
    await sent;
    await changeDeadlineThroughTheApi(deployment, pavel, issueId, new Date(now + 2 * dayMilliseconds).toISOString());
    await release();

    await expect(dialog).toBeHidden();
    await expect(changedMeanwhileNotice(page)).toBeFocused();
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

  test('Withdraw is the last item of Assignment actions after a separator and confirms with a filled button', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, pavel, 'Anna');
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: cardId });
    await openIssuePage(page, pavel, issueId, 'Fractions');
    await pageMenuButton(page, 'Assignment actions').click();
    await expectLastDangerItem(page, 'Withdraw', true);

    await openedMenu(page).getByRole('button', { name: 'Withdraw', exact: true }).click();

    await expectDangerConfirmation(
      page.getByRole('dialog', { name: 'Withdraw this assignment?' }),
      'Withdraw assignment',
      'Cancel',
    );
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

    await dialog.getByRole('button', { name: 'Withdraw assignment', exact: true }).click();

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

    await dialog.getByRole('button', { name: 'Withdraw assignment', exact: true }).click();

    await expect(dialog).toBeHidden();
    await expect(page.getByText('The assignment is withdrawn.', { exact: true })).toBeFocused();
    await expect(pageMenuButton(page, 'Assignment actions')).toHaveCount(0);
    await expect(page.locator('main header')).toContainText('Withdrawn');
    expect((await readIssue(deployment, pavel, issueId)).status).toBe('withdrawn');
  });

  test('an open assignment marked for erasure when its provisional student is removed shows the focused gone notice', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = newTestAccount('Anna', 'student');
    const card = await createCard(deployment, pavel, 'Anna', anna.email);
    await signUpOnInvitation(deployment, card.link, anna, 'en');
    const template = await createReadyTemplate(deployment, pavel, 'Fractions');
    const issueId = await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: card.id });
    await openIssuePage(page, pavel, issueId, 'Fractions');
    await page.getByRole('main').getByRole('link', { name: 'Anna' }).focus();
    diagnostics.expectFailure({ method: 'GET', path: `/api/v1/issues/${issueId}`, status: 404 });

    await removeStudent(deployment, pavel, card.id);

    await expect(page.getByText('This assignment is no longer available.', { exact: true })).toBeFocused();
    await expect(page.getByRole('main').getByRole('link', { name: 'Anna' })).toHaveCount(0);
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

  test('a withdrawal that removes the focused row of the card’s Homework tab moves focus to the row that took its place', async ({
    page,
    deployment,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const fractions = await issueThroughTheApi(
      deployment,
      pavel,
      (await createReadyTemplate(deployment, pavel, 'Fractions')).id,
      { relationship_id: annaCard },
    );
    const decimals = await issueThroughTheApi(
      deployment,
      pavel,
      (await createReadyTemplate(deployment, pavel, 'Decimals')).id,
      { relationship_id: annaCard },
    );
    await signInThroughTheApi(page, pavel);
    const traffic = trafficAfterReady(page, ['/api/v1/issues']);
    await traffic.initialize();
    await page.goto(`/students/${annaCard}?tab=homework`);
    const rows = page.getByRole('list', { name: 'Homework', exact: true }).getByRole('listitem');
    await expect(rows).toHaveCount(2);
    await expectRefetchedAfterTheSocketIsReady(traffic);
    const firstIsFractions = (await rows.first().innerText()).includes('Fractions');
    const remaining = firstIsFractions ? 'Decimals' : 'Fractions';
    await rows.first().locator('[data-row-focus]').focus();

    await withdrawThroughTheApi(deployment, pavel, firstIsFractions ? fractions : decimals);

    await expect(rows).toHaveCount(1);
    await expect(rows.filter({ hasText: remaining }).locator('[data-row-focus]')).toBeFocused();
  });

  test('a withdrawal that removes the focused row of the group’s assignments moves focus to the row that took its place', async ({
    page,
    deployment,
  }) => {
    const { pavel, annaCard } = await pavelWithAnna(deployment);
    const groupId = await createGroup(deployment, pavel, 'Evening algebra', [annaCard]);
    const fractions = await issueThroughTheApi(
      deployment,
      pavel,
      (await createReadyTemplate(deployment, pavel, 'Fractions')).id,
      { group_id: groupId },
    );
    const decimals = await issueThroughTheApi(
      deployment,
      pavel,
      (await createReadyTemplate(deployment, pavel, 'Decimals')).id,
      { group_id: groupId },
    );
    await signInThroughTheApi(page, pavel);
    const traffic = trafficAfterReady(page, ['/api/v1/issues']);
    await traffic.initialize();
    await page.goto(`/groups/${groupId}`);
    const rows = page.getByRole('list', { name: 'Assignments', exact: true }).getByRole('listitem');
    await expect(rows).toHaveCount(2);
    await expectRefetchedAfterTheSocketIsReady(traffic);
    const firstIsFractions = (await rows.first().innerText()).includes('Fractions');
    const remaining = firstIsFractions ? 'Decimals' : 'Fractions';
    await rows.first().getByRole('link').focus();

    await withdrawThroughTheApi(deployment, pavel, firstIsFractions ? fractions : decimals);

    await expect(rows).toHaveCount(1);
    await expect(rows.getByRole('link', { name: remaining })).toBeFocused();
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

test.describe('issuing on a phone', () => {
  test.use({ viewport: { width: 360, height: 780 } });

  test('Issue assignment opens a one-column page on its heading and keeps its actions at the end', async ({
    page,
    deployment,
  }) => {
    const { pavel } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    await openLibrary(page, pavel);

    await chooseRowAction(page, 'Fractions', 'Issue assignment');

    await expect(issueHeading(page)).toBeFocused();
    const search = await layoutBox(audienceSearch(page));
    const deadline = await layoutBox(issueDialog(page).getByRole('checkbox', { name: 'Set a deadline' }));
    const actions = await Promise.all(
      ['Issue', 'Cancel'].map(async (name) => layoutBox(issueDialog(page).getByRole('button', { name, exact: true }))),
    );
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(deadline.y).toBeGreaterThan(search.y + search.height);
    for (const action of actions) {
      expect(action.y).toBeGreaterThan(deadline.y + deadline.height);
    }
    expect(overflow).toBeLessThanOrEqual(0);
  });

  test('Back from Issue assignment returns focus to the row menu', async ({ page, deployment }) => {
    const { pavel } = await pavelWithAnna(deployment);
    await createReadyTemplate(deployment, pavel, 'Fractions');
    await openLibrary(page, pavel);
    await chooseRowAction(page, 'Fractions', 'Issue assignment');
    await audienceSearch(page).waitFor();

    await page.goBack();

    await expect(rowMenuButton(page, 'Fractions')).toBeFocused();
  });
});
