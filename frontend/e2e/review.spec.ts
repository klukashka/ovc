import type { Locator, Page } from '@playwright/test';

import { type TestAccount, newTestAccount, pageLoaded, registerTutor, signInThroughTheApi } from './accounts.ts';
import {
  createPastSimpleTemplate,
  onlySubmission,
  pastSimple,
  pdfFile,
  studentWork,
  submitThroughTheApi,
} from './assignment.ts';
import type { Deployment } from './deployment.ts';
import {
  type Diagnostics,
  type TestControl,
  diagnosedTest as test,
  expect,
  seedVerifiedUser,
  tableRows,
} from './fixtures.ts';
import { createReadyTemplate, issueThroughTheApi, withdrawThroughTheApi } from './homework.ts';
import { expectSettledAfterTheSocketIsReady, todayQueryPaths, trafficAfterReady } from './realtime.ts';
import { answerTextAndSubmit, draftOf, returnFromAnotherTab, saveDraftFromAnotherTab, storedReview } from './review.ts';
import { type PageClockOption, installPageClock, skipVisibleSaveDelay } from './save-delay.ts';
import { endRelationship, inSeedBatches, seedActiveStudent } from './tutoring.ts';

interface ReviewWork {
  readonly pavel: TestAccount;
  readonly anna: TestAccount;
  readonly annaCard: string;
  readonly templateId: string;
  readonly issueId: string;
  readonly submissionId: string;
}

async function issuedToAnna(deployment: Deployment): Promise<ReviewWork> {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { student: anna, cardId: annaCard } = await seedActiveStudent(deployment, pavel, 'Anna', 'en');
  const templateId = await createPastSimpleTemplate(deployment, pavel);
  const issueId = await issueThroughTheApi(deployment, pavel, templateId, { relationship_id: annaCard });
  return { pavel, anna, annaCard, templateId, issueId, submissionId: await onlySubmission(deployment, anna) };
}

async function submittedByAnna(deployment: Deployment): Promise<ReviewWork> {
  const work = await issuedToAnna(deployment);
  await answerTextAndSubmit(deployment, work.anna, work.submissionId, 'went');
  return work;
}

async function openReview(
  page: Page,
  tutor: TestAccount,
  submissionId: string,
  option: PageClockOption = {},
): Promise<void> {
  await installPageClock(page, option);
  await signInThroughTheApi(page, tutor);
  await page.goto(`/homework/review/${submissionId}`);
  await page.getByRole('heading', { level: 1, name: pastSimple.title }).waitFor();
}

function question(page: Page, number: number): Locator {
  return page.getByRole('region', { name: `Question ${String(number)}` });
}

function saveStatus(page: Page): Locator {
  return page.getByRole('status').filter({ hasText: /^(Saving…|Saved|Not saved)$/ });
}

async function enterPoints(page: Page, value: string): Promise<void> {
  await question(page, 3).getByRole('spinbutton', { name: 'Points' }).fill(value);
}

async function savedAndTicked(page: Page, control: TestControl): Promise<void> {
  await skipVisibleSaveDelay(page);
  await expect(saveStatus(page)).toHaveText('Saved');
  await control.advance(1);
}

function queueRows(page: Page): Locator {
  return tableRows(page, 'Work to check');
}

const textFile = { name: 'Notes.txt', mimeType: 'text/plain', buffer: Buffer.from('plain notes') };

async function attachCorrected(page: Page, file: { name: string; mimeType: string; buffer: Buffer }): Promise<void> {
  const chooser = page.waitForEvent('filechooser');
  await question(page, 3).getByRole('button', { name: 'Attach corrected file' }).click();
  await (await chooser).setFiles(file);
}

async function secondStudentSubmits(deployment: Deployment, work: ReviewWork, name: string): Promise<string> {
  const { student, cardId } = await seedActiveStudent(deployment, work.pavel, name, 'en');
  await issueThroughTheApi(deployment, work.pavel, work.templateId, { relationship_id: cardId });
  const submissionId = await onlySubmission(deployment, student);
  await submitThroughTheApi(deployment, student, submissionId);
  return submissionId;
}

interface UpcomingDeadlines {
  readonly pavel: TestAccount;
  readonly issues: readonly string[];
  readonly now: number;
}

async function deadlinesForAnna(
  deployment: Deployment,
  control: TestControl,
  titles: readonly string[],
): Promise<UpcomingDeadlines> {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { cardId } = await seedActiveStudent(deployment, pavel, 'Anna', 'en');
  const now = await control.now();
  const issues: string[] = [];
  for (const [index, title] of titles.entries()) {
    const template = await createReadyTemplate(deployment, pavel, title);
    const deadlineAt = new Date(now + (index + 1) * 86_400_000).toISOString();
    issues.push(await issueThroughTheApi(deployment, pavel, template.id, { relationship_id: cardId }, { deadlineAt }));
  }
  return { pavel, issues, now };
}

function expectConflictSave(diagnostics: Diagnostics, submissionId: string): void {
  diagnostics.expectFailure({ method: 'PUT', path: `/api/v1/submissions/${submissionId}/review`, status: 409 });
}

test.describe('the review queue', () => {
  test('lists submitted work oldest first with its count, and a row opens its review screen', async ({
    page,
    deployment,
    control,
  }) => {
    const work = await submittedByAnna(deployment);
    await control.advance(60);
    await secondStudentSubmits(deployment, work, 'Boris');
    await signInThroughTheApi(page, work.pavel);
    const groupsLoaded = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/v1/groups');
    await page.goto('/homework/review');
    await groupsLoaded;
    await expect(page.getByRole('heading', { level: 2, name: 'Needs checking 2' })).toBeVisible();
    await expect(queueRows(page).getByRole('link')).toHaveText(['Anna', 'Boris']);
    await expect(page.getByRole('link', { name: 'Homework, 2 to check' })).toBeVisible();

    await queueRows(page).getByRole('link', { name: 'Anna' }).click();

    await expect(page).toHaveURL(`/homework/review/${work.submissionId}`);
    await expect(page.getByRole('heading', { level: 1, name: pastSimple.title })).toBeVisible();
  });

  test('the issue page links to the review screen filtered by the issue, and Clear filter moves focus to Group', async ({
    page,
    deployment,
  }) => {
    const work = await submittedByAnna(deployment);
    await signInThroughTheApi(page, work.pavel);
    await page.goto(`/homework/issued/${work.issueId}`);
    await page.getByRole('link', { name: 'Anna' }).waitFor();
    await pageLoaded(page);
    await page.getByRole('link', { name: 'Anna' }).click();
    await page.getByRole('heading', { level: 1, name: pastSimple.title }).waitFor();
    await pageLoaded(page);
    await page.getByRole('link', { name: 'Back to the queue' }).click();
    await expect(page.getByText(`Assignment: ${pastSimple.title}`)).toBeVisible();

    await page.getByRole('button', { name: 'Clear filter' }).click();

    await expect(page.getByRole('combobox', { name: 'Group' })).toBeFocused();
    await expect(page).toHaveURL('/homework/review');
  });

  test('a filter the API refuses shows the refusal with a link to all work and no Retry', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const unknown = '0b5f2c1e-3c3a-4b8e-9d55-7f1a2b3c4d5e';
    diagnostics.expectFailure({ method: 'GET', path: `/api/v1/review-queue?issue=${unknown}&limit=20`, status: 404 });
    diagnostics.expectFailure({ method: 'GET', path: `/api/v1/issues/${unknown}`, status: 404 });
    await signInThroughTheApi(page, pavel);

    await page.goto(`/homework/review?issue=${unknown}`);

    await expect(page.getByRole('alert')).toHaveText('This filter is not available.');
    await expect(page.getByRole('link', { name: 'All work to check' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Retry' })).toHaveCount(0);
  });

  test('when a focused queue row goes, focus moves to the row that took its place', async ({
    page,
    deployment,
    control,
  }) => {
    const work = await submittedByAnna(deployment);
    await control.advance(60);
    await secondStudentSubmits(deployment, work, 'Boris');
    await signInThroughTheApi(page, work.pavel);
    await page.goto('/homework/review');
    await expect(queueRows(page).getByRole('link')).toHaveText(['Anna', 'Boris']);
    await queueRows(page).getByRole('link', { name: 'Anna' }).focus();

    await returnFromAnotherTab(deployment, work.pavel, work.submissionId);

    await expect(queueRows(page).getByRole('link')).toHaveText(['Boris']);
    await expect(queueRows(page).getByRole('link', { name: 'Boris' })).toBeFocused();
  });

  test('Show more moves focus to the first new row', async ({ page, deployment }) => {
    const work = await issuedToAnna(deployment);
    await inSeedBatches(20, () =>
      issueThroughTheApi(deployment, work.pavel, work.templateId, { relationship_id: work.annaCard }),
    );
    const todo = await studentWork(deployment, work.anna, 'todo');
    await inSeedBatches(todo.length, async (index) => {
      const item = todo[index];
      if (item === undefined) {
        throw new Error(`no work ${String(index)}`);
      }
      await submitThroughTheApi(deployment, work.anna, item.submissionId);
    });
    await signInThroughTheApi(page, work.pavel);
    await page.goto('/homework/review');
    await expect(queueRows(page)).toHaveCount(20);

    await page.getByRole('button', { name: 'Show more' }).click();

    await expect(queueRows(page)).toHaveCount(21);
    await expect(queueRows(page).nth(20).getByRole('link')).toBeFocused();
  });

  test('an empty queue says Nothing to check and links to the issued work', async ({ page, deployment }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await signInThroughTheApi(page, pavel);

    await page.goto('/homework/review');

    await expect(page.getByRole('heading', { level: 3, name: 'Nothing to check' })).toBeVisible();
    await expect(page.getByText('Submitted work appears here as soon as students send it.')).toBeVisible();
    await expect(page.getByRole('main').getByRole('link', { name: 'See issued work', exact: true })).toHaveAttribute(
      'href',
      '/homework/issued',
    );
  });

  test('the Homework count and the queue follow a new submission without a reload', async ({ page, deployment }) => {
    const work = await issuedToAnna(deployment);
    await signInThroughTheApi(page, work.pavel);
    await page.goto('/homework/review');
    await expect(page.getByText('Nothing to check')).toBeVisible();

    await submitThroughTheApi(deployment, work.anna, work.submissionId);

    await expect(page.getByRole('link', { name: 'Homework, 1 to check' })).toBeVisible();
    await expect(queueRows(page).getByRole('link', { name: 'Anna' })).toBeVisible();
  });

  test('an unconfirmed tutor sees the confirmation prompt on Needs checking and the review screen without requesting them', async ({
    page,
    deployment,
  }) => {
    const account = newTestAccount('Unconfirmed tutor');
    await registerTutor(deployment, account, 'en');
    await signInThroughTheApi(page, account);
    const closed: string[] = [];
    page.on('request', (request) => {
      const path = new URL(request.url()).pathname;
      if (path === '/api/v1/review-queue' || path.startsWith('/api/v1/submissions/')) {
        closed.push(path);
      }
    });

    await page.goto('/homework/review');
    await expect(page.getByText('Confirm your email to issue and check assignments.')).toBeVisible();
    await pageLoaded(page);
    await page.goto('/homework/review/0b5f2c1e-3c3a-4b8e-9d55-7f1a2b3c4d5e');

    await expect(page.getByText('Confirm your email to issue and check assignments.')).toBeVisible();
    expect(closed).toEqual([]);
  });
});

test.describe('the review screen', () => {
  test('shows each answer beside its scoring and saves the draft with Saving… then Saved', async ({
    page,
    deployment,
  }) => {
    const work = await submittedByAnna(deployment);
    await openReview(page, work.pavel, work.submissionId, { pageClock: true });
    await expect(question(page, 2)).toContainText('went');
    await expect(question(page, 2)).toContainText('2 automatic points');

    await enterPoints(page, '5');
    await question(page, 3).getByRole('textbox', { name: 'Comment' }).fill('Check the word order.');
    await skipVisibleSaveDelay(page);

    await expect(saveStatus(page)).toHaveText('Saved');
    const draft = draftOf(await storedReview(deployment, work.pavel, work.submissionId));
    const fileItem = draft?.items.find((item) => item.points !== null);
    expect(fileItem?.points).toBe(5);
    expect(fileItem?.comment).toBe('Check the word order.');
  });

  test('Override moves focus to its points, Remove override back to Override, and the override saves with its reason', async ({
    page,
    deployment,
    control,
  }) => {
    const work = await submittedByAnna(deployment);
    await openReview(page, work.pavel, work.submissionId, { pageClock: true });
    await question(page, 2).getByRole('button', { name: 'Override' }).click();
    await expect(question(page, 2).getByRole('spinbutton', { name: 'Override points' })).toBeFocused();

    await question(page, 2).getByRole('spinbutton', { name: 'Override points' }).fill('1');
    await question(page, 2).getByRole('textbox', { name: 'Reason' }).fill('Capital letter missing');
    await savedAndTicked(page, control);
    const draft = draftOf(await storedReview(deployment, work.pavel, work.submissionId));
    await question(page, 2).getByRole('button', { name: 'Remove override' }).click();

    expect(draft?.items.find((item) => item.overridePoints !== null)?.overrideReason).toBe('Capital letter missing');
    await expect(question(page, 2).getByRole('button', { name: 'Override' })).toBeFocused();
  });

  test('a corrected file uploads and saves, and Remove on it moves focus to Attach corrected file', async ({
    page,
    deployment,
    control,
  }) => {
    const work = await submittedByAnna(deployment);
    await openReview(page, work.pavel, work.submissionId, { pageClock: true });
    await attachCorrected(page, pdfFile);
    await expect(question(page, 3).getByRole('button', { name: `Remove ${pdfFile.name}` })).toBeVisible();
    await savedAndTicked(page, control);
    const saved = draftOf(await storedReview(deployment, work.pavel, work.submissionId));

    await question(page, 3)
      .getByRole('button', { name: `Remove ${pdfFile.name}` })
      .click();

    expect(saved?.items.flatMap((item) => item.correctedFiles)).toEqual([pdfFile.name]);
    await expect(question(page, 3).getByRole('button', { name: 'Attach corrected file' })).toBeFocused();
  });

  test('Remove on the first of two corrected files moves focus to the next file’s Remove', async ({
    page,
    deployment,
    control,
  }) => {
    const work = await submittedByAnna(deployment);
    const second = { ...pdfFile, name: 'Second.pdf' };
    await openReview(page, work.pavel, work.submissionId, { pageClock: true });
    await attachCorrected(page, pdfFile);
    await expect(question(page, 3).getByRole('button', { name: `Remove ${pdfFile.name}` })).toBeVisible();
    await savedAndTicked(page, control);
    await attachCorrected(page, second);
    await expect(question(page, 3).getByRole('button', { name: `Remove ${second.name}` })).toBeVisible();
    await savedAndTicked(page, control);

    await question(page, 3)
      .getByRole('button', { name: `Remove ${pdfFile.name}` })
      .click();

    await expect(question(page, 3).getByRole('button', { name: `Remove ${second.name}` })).toBeFocused();
  });

  test('Remove on the last of two corrected files moves focus to the previous file’s Remove', async ({
    page,
    deployment,
    control,
  }) => {
    const work = await submittedByAnna(deployment);
    const second = { ...pdfFile, name: 'Second.pdf' };
    await openReview(page, work.pavel, work.submissionId, { pageClock: true });
    await attachCorrected(page, pdfFile);
    await expect(question(page, 3).getByRole('button', { name: `Remove ${pdfFile.name}` })).toBeVisible();
    await savedAndTicked(page, control);
    await attachCorrected(page, second);
    await expect(question(page, 3).getByRole('button', { name: `Remove ${second.name}` })).toBeVisible();
    await savedAndTicked(page, control);

    await question(page, 3)
      .getByRole('button', { name: `Remove ${second.name}` })
      .click();

    await expect(question(page, 3).getByRole('button', { name: `Remove ${pdfFile.name}` })).toBeFocused();
  });

  test('Return for revision on a relationship that has ended shows the focused refusal', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const work = await submittedByAnna(deployment);
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/submissions/${work.submissionId}/return`, status: 409 });
    await openReview(page, work.pavel, work.submissionId);
    await endRelationship(deployment, work.pavel, work.annaCard);

    await page.getByRole('button', { name: 'Return for revision' }).click();

    await expect(
      page.getByRole('alert').filter({ hasText: 'This work can no longer be returned for revision.' }),
    ).toBeFocused();
  });

  test('a corrected file of a kind not allowed is refused before sending in a focused alert', async ({
    page,
    deployment,
  }) => {
    const work = await submittedByAnna(deployment);
    await openReview(page, work.pavel, work.submissionId);
    const uploads: string[] = [];
    page.on('request', (request) => {
      if (new URL(request.url()).pathname === '/api/v1/uploads') {
        uploads.push(request.url());
      }
    });

    await attachCorrected(page, textFile);

    await expect(question(page, 3).getByRole('alert')).toHaveText('This file type is not allowed here');
    await expect(question(page, 3).getByRole('alert')).toBeFocused();
    expect(uploads).toEqual([]);
  });

  test('Mark checked without points on a manual question shows the field text and the focused alert', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const work = await submittedByAnna(deployment);
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/submissions/${work.submissionId}/check`, status: 422 });
    await openReview(page, work.pavel, work.submissionId);

    await page.getByRole('button', { name: 'Mark checked' }).click();

    await expect(
      page.getByRole('alert').filter({ hasText: 'Enter points for every question you check by hand.' }),
    ).toBeFocused();
    await expect(question(page, 3)).toContainText('Enter points for this question');
  });

  test('Mark checked publishes the review with the focused notice', async ({ page, deployment, control }) => {
    const work = await submittedByAnna(deployment);
    await openReview(page, work.pavel, work.submissionId, { pageClock: true });
    await enterPoints(page, '6');
    await savedAndTicked(page, control);

    await page.getByRole('button', { name: 'Mark checked' }).click();

    await expect(
      page.getByRole('status').filter({ hasText: 'Checked. The student can see your review.' }),
    ).toBeFocused();
    expect((await storedReview(deployment, work.pavel, work.submissionId)).status).toBe('checked');
  });

  test('after Mark checked, Back to the queue no longer lists the work', async ({ page, deployment, control }) => {
    const work = await submittedByAnna(deployment);
    await openReview(page, work.pavel, work.submissionId, { pageClock: true });
    await enterPoints(page, '6');
    await savedAndTicked(page, control);
    await page.getByRole('button', { name: 'Mark checked' }).click();
    await page.getByRole('status').filter({ hasText: 'Checked. The student can see your review.' }).waitFor();

    await page.getByRole('link', { name: 'Back to the queue' }).click();

    await expect(page.getByText('Nothing to check')).toBeVisible();
  });

  test('Return for revision first asks for a comment, then returns with the focused notice onto a read-only screen', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const work = await submittedByAnna(deployment);
    diagnostics.expectFailure({ method: 'POST', path: `/api/v1/submissions/${work.submissionId}/return`, status: 422 });
    await openReview(page, work.pavel, work.submissionId, { pageClock: true });
    await page.getByRole('button', { name: 'Return for revision' }).click();
    await expect(
      page.getByRole('alert').filter({ hasText: 'Write a comment or feedback before returning the work.' }),
    ).toBeFocused();
    await control.advance(1);
    await question(page, 3).getByRole('textbox', { name: 'Comment' }).fill('Please rewrite sentence 2.');
    await savedAndTicked(page, control);

    await page.getByRole('button', { name: 'Return for revision' }).click();

    await expect(page.getByRole('status').filter({ hasText: 'Returned for revision.' })).toBeFocused();
    await expect(page.getByRole('button', { name: 'Mark checked' })).toHaveCount(0);
    expect((await storedReview(deployment, work.pavel, work.submissionId)).status).toBe('returned');
  });

  test('a draft saved in another tab makes the next save show the focused conflict notice, and Reload moves focus to the heading', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const work = await submittedByAnna(deployment);
    expectConflictSave(diagnostics, work.submissionId);
    await openReview(page, work.pavel, work.submissionId, { pageClock: true });
    await saveDraftFromAnotherTab(deployment, work.pavel, work.submissionId, 'From the other tab');
    await control.advance(1);
    await enterPoints(page, '4');
    await skipVisibleSaveDelay(page);
    const notice = page.getByRole('alert').filter({ hasText: 'This changed elsewhere' });
    await expect(notice).toBeFocused();
    await expect(question(page, 3).getByRole('spinbutton', { name: 'Points' })).toHaveValue('4');

    await notice.getByRole('button', { name: 'Reload' }).click();

    await expect(page.getByRole('heading', { level: 1, name: pastSimple.title })).toBeFocused();
    await expect(page.getByRole('textbox', { name: 'Overall feedback' })).toHaveValue('From the other tab');
  });

  test('work returned in another tab turns the screen read-only under the focused changed-meanwhile notice', async ({
    page,
    deployment,
  }) => {
    const work = await submittedByAnna(deployment);
    await openReview(page, work.pavel, work.submissionId);
    await question(page, 3).getByRole('spinbutton', { name: 'Points' }).focus();

    await returnFromAnotherTab(deployment, work.pavel, work.submissionId);

    await expect(
      page
        .getByRole('status')
        .filter({ hasText: 'This work changed in the meantime. Its current state is shown below.' }),
    ).toBeFocused();
    await expect(page.getByRole('button', { name: 'Mark checked' })).toHaveCount(0);
  });

  test('Preview renders the feedback through the restricted Markdown renderer and keeps focus on the switch', async ({
    page,
    deployment,
  }) => {
    const work = await submittedByAnna(deployment);
    await openReview(page, work.pavel, work.submissionId);
    await page.getByRole('textbox', { name: 'Overall feedback' }).fill('**Great work.** Check the word order.');
    await page.getByRole('button', { name: 'Preview' }).focus();

    await page.keyboard.press('Enter');

    await expect(page.getByRole('button', { name: 'Preview' })).toBeFocused();
    await expect(page.locator('[data-feedback-preview] strong')).toHaveText('Great work.');
  });

  test('Next submission opens the next work of the queue', async ({ page, deployment, control }) => {
    const work = await submittedByAnna(deployment);
    await control.advance(60);
    const borisSubmission = await secondStudentSubmits(deployment, work, 'Boris');
    await openReview(page, work.pavel, work.submissionId);

    await page.getByRole('link', { name: 'Next submission' }).click();

    await expect(page).toHaveURL(`/homework/review/${borisSubmission}`);
    await expect(page.getByText('Boris', { exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Next submission' })).toHaveCount(0);
  });

  test('work not yet submitted is shown read-only with its sentence', async ({ page, deployment }) => {
    const work = await issuedToAnna(deployment);

    await openReview(page, work.pavel, work.submissionId);

    await expect(page.getByText('This work has not been submitted yet.')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Mark checked' })).toHaveCount(0);
    await expect(page.getByRole('spinbutton')).toHaveCount(0);
  });
});

test.describe('Today’s homework panels', () => {
  test('Needs checking and Upcoming deadlines list the work with their links', async ({
    page,
    deployment,
    control,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student: anna, cardId } = await seedActiveStudent(deployment, pavel, 'Anna', 'en');
    const templateId = await createPastSimpleTemplate(deployment, pavel);
    const now = await control.now();
    const deadline = new Date(now + 2 * 86_400_000).toISOString();
    await issueThroughTheApi(deployment, pavel, templateId, { relationship_id: cardId }, { deadlineAt: deadline });
    await submitThroughTheApi(deployment, anna, await onlySubmission(deployment, anna));
    await page.clock.install({ time: now });
    await signInThroughTheApi(page, pavel);

    await page.goto('/today');

    const checking = page.getByRole('region', { name: /^Needs checking/ });
    await expect(checking.getByRole('link', { name: 'Anna' })).toBeVisible();
    await expect(checking.getByRole('link', { name: 'Open review queue' })).toBeVisible();
    const deadlines = page.getByRole('region', { name: 'Upcoming deadlines' });
    await expect(deadlines.getByRole('link', { name: pastSimple.title })).toBeVisible();
    await expect(deadlines).toContainText('1/1 submitted');
  });

  test('when the focused last Needs checking row goes, focus moves to the panel heading', async ({
    page,
    deployment,
  }) => {
    const work = await submittedByAnna(deployment);
    await signInThroughTheApi(page, work.pavel);
    await page.goto('/today');
    const checking = page.getByRole('region', { name: /^Needs checking/ });
    await checking.getByRole('link', { name: 'Anna' }).focus();

    await returnFromAnotherTab(deployment, work.pavel, work.submissionId);

    await expect(checking.getByRole('heading', { level: 2 })).toBeFocused();
    await expect(checking.getByText('Nothing to check')).toBeVisible();
  });

  test('when a focused Needs checking row that is not the last goes, focus moves to the row that took its place', async ({
    page,
    deployment,
    control,
  }) => {
    const work = await submittedByAnna(deployment);
    await control.advance(60);
    await secondStudentSubmits(deployment, work, 'Boris');
    await signInThroughTheApi(page, work.pavel);
    await page.goto('/today');
    const checking = page.getByRole('region', { name: /^Needs checking/ });
    await expect(checking.locator('[data-row-link]')).toHaveText(['Anna', 'Boris']);
    await checking.getByRole('link', { name: 'Anna' }).focus();

    await returnFromAnotherTab(deployment, work.pavel, work.submissionId);

    await expect(checking.locator('[data-row-link]')).toHaveText(['Boris']);
    await expect(checking.getByRole('link', { name: 'Boris' })).toBeFocused();
  });

  test('when a focused Upcoming deadlines row that is not the last goes, focus moves to the row that took its place', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, issues, now } = await deadlinesForAnna(deployment, control, ['Fractions', 'Decimals']);
    await page.clock.install({ time: now });
    await signInThroughTheApi(page, pavel);
    await page.goto('/today');
    const deadlines = page.getByRole('region', { name: 'Upcoming deadlines' });
    await expect(deadlines.locator('[data-row-link]')).toHaveText(['Fractions', 'Decimals']);
    await deadlines.getByRole('link', { name: 'Fractions' }).focus();

    await withdrawThroughTheApi(deployment, pavel, issues[0] ?? '');

    await expect(deadlines.locator('[data-row-link]')).toHaveText(['Decimals']);
    await expect(deadlines.getByRole('link', { name: 'Decimals' })).toBeFocused();
  });

  test('when the focused last Upcoming deadlines row goes, focus moves to the panel heading', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, issues, now } = await deadlinesForAnna(deployment, control, ['Fractions']);
    await page.clock.install({ time: now });
    await signInThroughTheApi(page, pavel);
    const traffic = trafficAfterReady(page, todayQueryPaths);
    await page.goto('/today');
    await expectSettledAfterTheSocketIsReady(page, traffic);
    const deadlines = page.getByRole('region', { name: 'Upcoming deadlines' });
    await deadlines.getByRole('link', { name: 'Fractions' }).focus();

    await withdrawThroughTheApi(deployment, pavel, issues[0] ?? '');

    await expect(deadlines.locator('[data-row-link]')).toHaveCount(0);
    await expect(deadlines.getByRole('heading', { level: 2, name: 'Upcoming deadlines' })).toBeFocused();
  });
});
