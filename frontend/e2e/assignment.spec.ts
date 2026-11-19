import type { BrowserContext, Locator, Page, Response } from '@playwright/test';

import { type TestAccount, newTestAccount, signInThroughTheApi } from './accounts.ts';
import {
  answerSentinelsAndSubmit,
  createPastSimpleTemplate,
  onlySubmission,
  pastSimple,
  pdfFile,
  pictureFile,
  questionIds,
  saveTextFromAnotherDevice,
  sentinel,
  bindAndDropFromAnotherDevice,
  storedAnswers,
  uploadedId,
  studentWork,
  submitThroughTheApi,
} from './assignment.ts';
import type { ObservedBrowser } from './browser-diagnostics.ts';
import type { Deployment } from './deployment.ts';
import {
  type Diagnostics,
  type PageDiagnostics,
  type TestControl,
  diagnosedTest as test,
  expect,
  seedVerifiedUser,
  signUpAndConfirmTutor,
} from './fixtures.ts';
import { localDate } from './schedule.ts';
import { type AnswerPolicy, issueManyThroughTheApi, issueThroughTheApi, withdrawThroughTheApi } from './homework.ts';
import {
  type ReviewItemSeed,
  expectAssignmentListKeys,
  expectStudentSubmissionKeys,
  publishReview,
  reviewQueueIds,
  scoreFormsIn,
  studentRead,
  tutorFinalPoints,
  uploadCorrectedFile,
} from './results.ts';
import { answerTextAndSubmit, storedReview, studentSubmissionText } from './review.ts';
import { type PageClockOption, installPageClock, skipVisibleSaveDelay } from './save-delay.ts';
import {
  accountPreferences,
  createCard,
  createGroup,
  deleteAccount,
  endRelationship,
  saveTheme,
  seedActiveStudent,
  signUpActiveStudentOnInvitation,
  signUpOnInvitation,
  signUpStudentThroughCode,
} from './tutoring.ts';

interface TutorWithStudent {
  readonly pavel: TestAccount;
  readonly anna: TestAccount;
  readonly annaCard: string;
}

async function pavelWithAnna(deployment: Deployment): Promise<TutorWithStudent> {
  const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
  const { student, cardId } = await seedActiveStudent(deployment, pavel, 'Anna', 'en');
  return { pavel, anna: student, annaCard: cardId };
}

interface IssuedWork extends TutorWithStudent {
  readonly issueId: string;
  readonly submissionId: string;
}

async function issuedToAnna(
  deployment: Deployment,
  withSentinels = false,
  policy: AnswerPolicy = 'after_check',
): Promise<IssuedWork> {
  const actors = await pavelWithAnna(deployment);
  const template = await createPastSimpleTemplate(deployment, actors.pavel, { withSentinels });
  const issueId = await issueThroughTheApi(
    deployment,
    actors.pavel,
    template,
    { relationship_id: actors.annaCard },
    { policy },
  );
  return { ...actors, issueId, submissionId: await onlySubmission(deployment, actors.anna) };
}

function submittedNotice(page: Page): Locator {
  return page.getByRole('status').filter({ hasText: /^Submitted$/ });
}

async function openAssignment(
  page: Page,
  student: TestAccount,
  submissionId: string,
  option: PageClockOption = {},
): Promise<void> {
  await installPageClock(page, option);
  await signInThroughTheApi(page, student);
  await page.goto(`/homework/${submissionId}`);
  await page.getByRole('heading', { level: 1, name: pastSimple.title }).waitFor();
}

function question(page: Page, number: number): Locator {
  return page.getByRole('group', { name: `Question ${String(number)}` });
}

function textAnswer(page: Page): Locator {
  return question(page, 2).getByRole('textbox', { name: 'Your answer' });
}

function saveStatus(page: Page): Locator {
  return page.getByRole('status').filter({ hasText: /^(Saving…|Saved|Not saved\. Trying again…)$/ });
}

async function holdAnswerSaves(page: Page): Promise<() => Promise<void>> {
  let resume: () => void = () => undefined;
  const resumed = new Promise<void>((resolve) => {
    resume = resolve;
  });
  await page.route('**/api/v1/submissions/*/answers', async (route) => {
    if (route.request().method() === 'PUT') {
      await resumed;
    }
    await route.continue();
  });
  return async () => {
    const answered = page.waitForResponse((response) => response.request().method() === 'PUT');
    resume();
    await answered;
    await page.unroute('**/api/v1/submissions/*/answers');
  };
}

async function questionNumbered(deployment: Deployment, work: IssuedWork, number: number): Promise<string> {
  const id = (await questionIds(deployment, work.anna, work.submissionId))[number - 1];
  if (id === undefined) {
    throw new Error(`the assignment has no question ${String(number)}`);
  }
  return id;
}

async function holdSubmit(page: Page, submissionId: string): Promise<() => Promise<void>> {
  let resume: () => void = () => undefined;
  const resumed = new Promise<void>((resolve) => {
    resume = resolve;
  });
  const address = `**/api/v1/submissions/${submissionId}/submit`;
  await page.route(address, async (route) => {
    await resumed;
    await route.continue();
  });
  return async () => {
    const answered = page.waitForResponse(
      (response) => new URL(response.url()).pathname === `/api/v1/submissions/${submissionId}/submit`,
    );
    resume();
    await answered;
    await page.unroute(address);
  };
}

interface HeldSubmit extends IssuedWork {
  readonly dialog: Locator;
  readonly sent: Promise<unknown>;
  readonly release: () => Promise<void>;
}

async function confirmationWithHeldSubmit(
  page: Page,
  deployment: Deployment,
  control: TestControl,
  diagnostics: Diagnostics,
): Promise<HeldSubmit> {
  const work = await issuedToAnna(deployment);
  await openAssignment(page, work.anna, work.submissionId, { pageClock: true });
  await textAnswer(page).fill('went');
  await skipVisibleSaveDelay(page);
  await expect(saveStatus(page)).toHaveText('Saved');
  await control.advance(1);
  await page.getByRole('button', { name: 'Submit', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Submit your work?' });
  await dialog.getByRole('button', { name: 'Cancel', exact: true }).waitFor();
  const release = await holdSubmit(page, work.submissionId);
  const sent = page.waitForRequest(
    (request) => new URL(request.url()).pathname === `/api/v1/submissions/${work.submissionId}/submit`,
  );
  diagnostics.expectFailure({ method: 'POST', path: `/api/v1/submissions/${work.submissionId}/submit`, status: 409 });
  return { ...work, dialog, sent, release };
}

async function attach(
  page: Page,
  number: number,
  file: { name: string; mimeType: string; buffer: Buffer },
): Promise<void> {
  const chooser = page.waitForEvent('filechooser');
  await question(page, number).getByRole('button', { name: 'Attach files', exact: true }).click();
  await (await chooser).setFiles(file);
}

const secondPdf = { ...pdfFile, name: 'Notes.pdf' };

function conflictBanner(page: Page): Locator {
  return page.getByRole('alert').filter({ hasText: 'Your answers changed on another device' });
}

function removeButton(page: Page, name: string): Locator {
  return question(page, 3).getByRole('button', { name: `Remove ${name}`, exact: true });
}

interface HeldConflict extends IssuedWork {
  readonly release: () => Promise<void>;
}

async function holdConflictingSave(
  page: Page,
  deployment: Deployment,
  diagnostics: Diagnostics,
  mine: { readonly choice?: string; readonly text: string },
  theirs: string,
): Promise<HeldConflict> {
  const work = await issuedToAnna(deployment);
  const [, textQuestion] = await questionIds(deployment, work.anna, work.submissionId);
  if (textQuestion === undefined) {
    throw new Error('the assignment has no text question');
  }
  await openAssignment(page, work.anna, work.submissionId, { pageClock: true });
  const release = await holdAnswerSaves(page);
  const firstSave = page.waitForRequest((request) => request.method() === 'PUT');
  diagnostics.expectFailure({ method: 'PUT', path: `/api/v1/submissions/${work.submissionId}/answers`, status: 409 });
  if (mine.choice !== undefined) {
    await question(page, 1).getByRole('radio', { name: mine.choice }).check();
  }
  await textAnswer(page).fill(mine.text);
  await skipVisibleSaveDelay(page);
  await firstSave;
  await saveTextFromAnotherDevice(deployment, work.anna, work.submissionId, textQuestion, theirs);
  return { ...work, release };
}

async function reachConflict(
  page: Page,
  deployment: Deployment,
  diagnostics: Diagnostics,
  mine: { readonly choice?: string; readonly text: string },
  theirs: string,
): Promise<IssuedWork> {
  const conflict = await holdConflictingSave(page, deployment, diagnostics, mine, theirs);
  await conflict.release();
  await conflictBanner(page).waitFor();
  return conflict;
}

async function twoFilesAttached(page: Page, deployment: Deployment, control: TestControl): Promise<IssuedWork> {
  const work = await issuedToAnna(deployment);
  await openAssignment(page, work.anna, work.submissionId, { pageClock: true });
  await attach(page, 3, pdfFile);
  await skipVisibleSaveDelay(page);
  await expect(saveStatus(page)).toHaveText('Saved');
  await control.advance(1);
  await attach(page, 3, secondPdf);
  await expect(removeButton(page, secondPdf.name)).toBeVisible();
  await skipVisibleSaveDelay(page);
  await expect(saveStatus(page)).toHaveText('Saved');
  await control.advance(1);
  return work;
}

function collectStudentResponses(page: Page): () => Promise<string[]> {
  const bodies: Promise<string>[] = [];
  const collect = (response: Response): void => {
    const path = new URL(response.url()).pathname;
    if ((path.startsWith('/api/v1/submissions/') || path === '/api/v1/assignments') && response.status() !== 204) {
      bodies.push(response.text());
    }
  };
  page.on('response', collect);
  return async () => {
    page.off('response', collect);
    return Promise.all(bodies);
  };
}

function expectNoAnswerKeys(texts: readonly string[]): void {
  const sentinelNumber = new RegExp(`(^|[^0-9a-f-])${String(sentinel.points)}($|[^0-9a-f-])`);
  for (const text of texts) {
    expect(text).not.toContain(sentinel.accepted);
    expect(text).not.toContain(sentinel.explanation);
    expect(text).not.toMatch(sentinelNumber);
    for (const key of ['"points"', '"score"', '"correct"', '"accepted_answers"', '"explanation"', '"revealed"']) {
      expect(text).not.toContain(key);
    }
  }
}

function expectNoPoints(texts: readonly string[], values: readonly number[]): void {
  for (const text of texts) {
    for (const value of values) {
      expect(text).not.toMatch(new RegExp(`(^|[^0-9a-f:.-])${String(value)}($|[^0-9a-f:.-])`));
      expect(text).not.toMatch(new RegExp(`:${String(value)}[,}\\]]`));
    }
    for (const key of [
      '"points"',
      '"score"',
      '"auto_points"',
      '"final_points"',
      '"max_points"',
      '"percentage"',
      '"override_points"',
      '"override_reason"',
    ]) {
      expect(text).not.toContain(key);
    }
  }
}

test.describe('answering an assignment', () => {
  test('typing an answer shows Saving… then Saved and stores the answer', async ({ page, deployment }) => {
    const { anna, submissionId } = await issuedToAnna(deployment);
    await openAssignment(page, anna, submissionId);

    await textAnswer(page).fill('went');

    await expect(saveStatus(page)).toHaveText('Saving…');
    await expect(saveStatus(page)).toHaveText('Saved');
    await expect(page.getByText('1/3 answered', { exact: true })).toBeVisible();
    expect((await storedAnswers(deployment, anna, submissionId)).texts).toEqual(['went']);
  });

  test('a save answering after newer typing keeps that typing and saves it next', async ({ page, deployment }) => {
    const { anna, submissionId } = await issuedToAnna(deployment);
    await openAssignment(page, anna, submissionId, { pageClock: true });
    const release = await holdAnswerSaves(page);
    const firstSave = page.waitForRequest((request) => request.method() === 'PUT');
    await textAnswer(page).fill('went');
    await skipVisibleSaveDelay(page);
    await firstSave;

    await textAnswer(page).fill('went away');
    await release();

    await expect(saveStatus(page)).toHaveText('Saved');
    await expect(textAnswer(page)).toHaveValue('went away');
    await expect.poll(async () => (await storedAnswers(deployment, anna, submissionId)).texts).toEqual(['went away']);
  });

  test('a save refused because another device saved meanwhile shows the focused conflict banner', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const conflict = await holdConflictingSave(page, deployment, diagnostics, { text: 'went' }, 'gone');

    await conflict.release();

    await expect(conflictBanner(page)).toBeFocused();
    await expect(saveStatus(page)).toHaveCount(0);
  });

  test('Load the latest shows the stored answer and keeps the differing unsaved one, focusing Use this answer', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    await reachConflict(page, deployment, diagnostics, { text: 'went' }, 'gone');

    await conflictBanner(page).getByRole('button', { name: 'Load the latest', exact: true }).click();

    await expect(textAnswer(page)).toHaveValue('gone');
    await expect(question(page, 2).getByText('Your unsaved answer: went', { exact: true })).toBeVisible();
    await expect(question(page, 2).getByRole('button', { name: 'Use this answer', exact: true })).toBeFocused();
  });

  test('Load the latest moves focus to the page heading when no unsaved answer differs', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    await reachConflict(page, deployment, diagnostics, { text: 'went' }, 'went');

    await conflictBanner(page).getByRole('button', { name: 'Load the latest', exact: true }).click();

    await expect(page.getByRole('heading', { level: 1, name: pastSimple.title })).toBeFocused();
    await expect(page.getByRole('button', { name: 'Use this answer' })).toHaveCount(0);
  });

  test('Use this answer puts the unsaved answer back, focuses its field and saves it', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const { anna, submissionId } = await reachConflict(page, deployment, diagnostics, { text: 'went' }, 'gone');
    await conflictBanner(page).getByRole('button', { name: 'Load the latest', exact: true }).click();
    const use = question(page, 2).getByRole('button', { name: 'Use this answer', exact: true });
    await expect(use).toBeFocused();
    await control.advance(1);

    await use.click();
    await skipVisibleSaveDelay(page);

    await expect(textAnswer(page)).toHaveValue('went');
    await expect(textAnswer(page)).toBeFocused();
    await expect(saveStatus(page)).toHaveText('Saved');
    expect((await storedAnswers(deployment, anna, submissionId)).texts).toEqual(['went']);
  });

  test('Use this answer moves focus to the next kept answer’s Use this answer', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    await reachConflict(page, deployment, diagnostics, { choice: 'goed', text: 'went' }, 'gone');
    await conflictBanner(page).getByRole('button', { name: 'Load the latest', exact: true }).click();
    const first = question(page, 1).getByRole('button', { name: 'Use this answer', exact: true });
    await expect(first).toBeFocused();
    await control.advance(1);

    await first.click();

    await expect(question(page, 2).getByRole('button', { name: 'Use this answer', exact: true })).toBeFocused();
  });

  test('a file answer uploads and is saved', async ({ page, deployment }) => {
    const { anna, submissionId } = await issuedToAnna(deployment);
    await openAssignment(page, anna, submissionId, { pageClock: true });

    await attach(page, 3, pdfFile);
    await skipVisibleSaveDelay(page);

    await expect(removeButton(page, pdfFile.name)).toBeVisible();
    await expect(saveStatus(page)).toHaveText('Saved');
    expect((await storedAnswers(deployment, anna, submissionId)).files).toEqual([pdfFile.name]);
  });

  test('Remove on the only file moves focus to Attach files and saves the removal', async ({
    page,
    deployment,
    control,
  }) => {
    const { anna, submissionId } = await issuedToAnna(deployment);
    await openAssignment(page, anna, submissionId, { pageClock: true });
    await attach(page, 3, pdfFile);
    await skipVisibleSaveDelay(page);
    await expect(saveStatus(page)).toHaveText('Saved');
    await control.advance(1);

    await removeButton(page, pdfFile.name).click();
    await skipVisibleSaveDelay(page);

    await expect(question(page, 3).getByRole('button', { name: 'Attach files', exact: true })).toBeFocused();
    await expect(saveStatus(page)).toHaveText('Saved');
    expect((await storedAnswers(deployment, anna, submissionId)).files).toEqual([]);
  });

  test('Remove moves focus to the next file’s Remove', async ({ page, deployment, control }) => {
    const { anna, submissionId } = await twoFilesAttached(page, deployment, control);

    await removeButton(page, pdfFile.name).click();
    await skipVisibleSaveDelay(page);

    await expect(removeButton(page, secondPdf.name)).toBeFocused();
    await expect
      .poll(async () => (await storedAnswers(deployment, anna, submissionId)).files)
      .toEqual([secondPdf.name]);
  });

  test('Remove on the last file moves focus to the previous file’s Remove', async ({ page, deployment, control }) => {
    const { anna, submissionId } = await twoFilesAttached(page, deployment, control);

    await removeButton(page, secondPdf.name).click();
    await skipVisibleSaveDelay(page);

    await expect(removeButton(page, pdfFile.name)).toBeFocused();
    await expect.poll(async () => (await storedAnswers(deployment, anna, submissionId)).files).toEqual([pdfFile.name]);
  });

  test('putting back a file another device removed is refused, returning the question to its saved files under a focused alert', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const { anna, submissionId } = await issuedToAnna(deployment);
    const [, , fileQuestion] = await questionIds(deployment, anna, submissionId);
    if (fileQuestion === undefined) {
      throw new Error('the assignment has no file question');
    }
    await openAssignment(page, anna, submissionId, { pageClock: true });
    const release = await holdAnswerSaves(page);
    const firstSave = page.waitForRequest((request) => request.method() === 'PUT');
    const uploaded = page.waitForResponse(
      (response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/uploads',
    );
    diagnostics.expectFailure({ method: 'PUT', path: `/api/v1/submissions/${submissionId}/answers`, status: 409 });
    diagnostics.expectFailure({ method: 'PUT', path: `/api/v1/submissions/${submissionId}/answers`, status: 404 });
    await attach(page, 3, pdfFile);
    const attachment = await uploadedId(await uploaded);
    await skipVisibleSaveDelay(page);
    await firstSave;
    await bindAndDropFromAnotherDevice(deployment, anna, submissionId, fileQuestion, attachment);
    await control.advance(1);
    await release();
    await conflictBanner(page).getByRole('button', { name: 'Load the latest', exact: true }).click();
    const use = question(page, 3).getByRole('button', { name: 'Use this answer', exact: true });
    await use.waitFor();

    await use.click();

    const alert = question(page, 3).getByRole('alert');
    await expect(alert).toHaveText('This file is no longer available. Attach it again.');
    await expect(alert).toBeFocused();
    await expect(removeButton(page, pdfFile.name)).toHaveCount(0);
    await expect(saveStatus(page)).toHaveText('Saved');
    expect((await storedAnswers(deployment, anna, submissionId)).files).toEqual([]);
  });

  test('a file of a kind the question does not allow is refused before sending, in a focused alert', async ({
    page,
    deployment,
  }) => {
    const { anna, submissionId } = await issuedToAnna(deployment);
    await openAssignment(page, anna, submissionId);
    const uploads: string[] = [];
    page.on('request', (request) => {
      if (new URL(request.url()).pathname === '/api/v1/uploads') {
        uploads.push(request.url());
      }
    });

    await attach(page, 3, { name: 'notes.txt', mimeType: 'text/plain', buffer: Buffer.from('notes') });

    await expect(question(page, 3).getByRole('alert')).toHaveText('This file type is not allowed here');
    await expect(question(page, 3).getByRole('alert')).toBeFocused();
    expect(uploads).toEqual([]);
  });

  test('Submit saves the pending answer and opens the confirmation on Cancel, listing the unanswered questions', async ({
    page,
    deployment,
  }) => {
    const { anna, submissionId } = await issuedToAnna(deployment);
    await openAssignment(page, anna, submissionId);
    await textAnswer(page).fill('went');

    await page.getByRole('button', { name: 'Submit', exact: true }).click();

    const dialog = page.getByRole('dialog', { name: 'Submit your work?' });
    await expect(dialog.getByRole('listitem')).toHaveText(['Question 1', 'Question 3']);
    await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeFocused();
    expect((await storedAnswers(deployment, anna, submissionId)).texts).toEqual(['went']);
  });

  test('Cancel in the confirmation returns focus to Submit', async ({ page, deployment }) => {
    const { anna, submissionId } = await issuedToAnna(deployment);
    await openAssignment(page, anna, submissionId);
    const submit = page.getByRole('button', { name: 'Submit', exact: true });
    await submit.click();
    const dialog = page.getByRole('dialog', { name: 'Submit your work?' });
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).waitFor();

    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();

    await expect(dialog).toBeHidden();
    await expect(submit).toBeFocused();
  });

  test('a submission refused because the answers changed on another device closes onto the focused conflict banner', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const held = await confirmationWithHeldSubmit(page, deployment, control, diagnostics);
    const textQuestion = await questionNumbered(deployment, held, 2);

    await held.dialog.getByRole('button', { name: 'Submit', exact: true }).click();
    await held.sent;
    await saveTextFromAnotherDevice(deployment, held.anna, held.submissionId, textQuestion, 'gone');
    await held.release();

    await expect(held.dialog).toBeHidden();
    await expect(conflictBanner(page)).toBeFocused();
    expect((await storedAnswers(deployment, held.anna, held.submissionId)).status).toBe('draft');
  });

  test('a submission refused because the work closed meanwhile closes onto the focused changed notice', async ({
    page,
    deployment,
    control,
    diagnostics,
  }) => {
    const held = await confirmationWithHeldSubmit(page, deployment, control, diagnostics);

    await held.dialog.getByRole('button', { name: 'Submit', exact: true }).click();
    await held.sent;
    await endRelationship(deployment, held.pavel, held.annaCard);
    await held.release();

    await expect(held.dialog).toBeHidden();
    await expect(
      page.getByText('This assignment changed in the meantime. Its current state is shown below.', { exact: true }),
    ).toBeFocused();
  });

  test('submitting shows the focused Submitted notice, with no points, accepted answers or explanations anywhere', async ({
    page,
    deployment,
    control,
  }) => {
    const { anna, submissionId } = await issuedToAnna(deployment, true);
    const bodies = collectStudentResponses(page);
    await openAssignment(page, anna, submissionId, { pageClock: true });
    await question(page, 1).getByRole('radio', { name: 'goed' }).check();
    await textAnswer(page).fill('went');
    await skipVisibleSaveDelay(page);
    await expect(saveStatus(page)).toHaveText('Saved');
    await control.advance(1);

    await page.getByRole('button', { name: 'Submit', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Submit your work?' })
      .getByRole('button', { name: 'Submit', exact: true })
      .click();

    await expect(submittedNotice(page)).toBeFocused();
    await expect(page.getByRole('button', { name: 'Submit', exact: true })).toHaveCount(0);
    await expect(textAnswer(page)).toHaveAttribute('readonly');
    const responses = await bodies();
    expectNoAnswerKeys([await page.locator('body').innerText(), ...responses]);
    expect(responses.length).toBeGreaterThan(2);
    expect((await storedAnswers(deployment, anna, submissionId)).status).toBe('submitted');
  });

  test('the Submitted tab lists submitted work with no points, accepted answers or explanations', async ({
    page,
    deployment,
  }) => {
    const { anna, submissionId } = await issuedToAnna(deployment, true);
    await submitThroughTheApi(deployment, anna, submissionId);
    const bodies = collectStudentResponses(page);
    await signInThroughTheApi(page, anna);

    await page.goto('/homework?status=submitted');

    await expect(
      page.getByRole('table', { name: 'Assignments' }).getByRole('link', { name: pastSimple.title }),
    ).toBeVisible();
    const responses = await bodies();
    expectNoAnswerKeys([await page.locator('body').innerText(), ...responses]);
    expect(responses.length).toBeGreaterThan(0);
  });

  test('an unconfirmed student of a provisional link reaches Homework, answers, and cannot attach files', async ({
    page,
    deployment,
  }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = newTestAccount('Anna', 'student');
    const card = await createCard(deployment, pavel, 'Anna', anna.email);
    await signUpOnInvitation(deployment, card.link, anna, 'en');
    const template = await createPastSimpleTemplate(deployment, pavel);
    await issueThroughTheApi(deployment, pavel, template, { relationship_id: card.id });
    await installPageClock(page, { pageClock: true });
    await signInThroughTheApi(page, anna);
    await page.goto('/home');

    await page
      .getByRole('navigation', { name: 'Main navigation' })
      .getByRole('link', { name: 'Homework, 1 to do' })
      .click();
    await page.getByRole('table', { name: 'Assignments' }).getByRole('link', { name: pastSimple.title }).click();
    await textAnswer(page).fill('went');
    await skipVisibleSaveDelay(page);

    await expect(saveStatus(page)).toHaveText('Saved');
    await expect(question(page, 3).getByText('Confirm your email to attach files.', { exact: true })).toBeVisible();
    await expect(question(page, 3).getByRole('button', { name: 'Attach files' })).toHaveCount(0);
  });

  test('an answer saved after the relationship ended reloads the page onto the focused changed notice', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const { pavel, anna, annaCard, submissionId } = await issuedToAnna(deployment);
    await openAssignment(page, anna, submissionId, { pageClock: true });
    const release = await holdAnswerSaves(page);
    const firstSave = page.waitForRequest((request) => request.method() === 'PUT');
    diagnostics.expectFailure({ method: 'PUT', path: `/api/v1/submissions/${submissionId}/answers`, status: 409 });
    await textAnswer(page).fill('went');
    await skipVisibleSaveDelay(page);
    await firstSave;

    await endRelationship(deployment, pavel, annaCard);
    await release();

    await expect(
      page.getByText('This assignment changed in the meantime. Its current state is shown below.', { exact: true }),
    ).toBeFocused();
    await expect(page.getByText('This assignment is closed and can no longer be submitted.')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Submit', exact: true })).toHaveCount(0);
  });

  test('a withdrawal while the student types replaces the page with the focused gone notice', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const { pavel, anna, issueId, submissionId } = await issuedToAnna(deployment);
    await openAssignment(page, anna, submissionId);
    await textAnswer(page).focus();
    diagnostics.expectFailure({ method: 'GET', path: `/api/v1/submissions/${submissionId}`, status: 404 });

    await withdrawThroughTheApi(deployment, pavel, issueId);

    await expect(page.getByText('This assignment is no longer available.', { exact: true })).toBeFocused();
    await expect(page.getByRole('link', { name: 'All homework', exact: true })).toBeVisible();
  });
});

test.describe('the student’s Homework list', () => {
  test('choosing a tab keeps focus on it, and the Closed tab shows only while closed work exists', async ({
    page,
    deployment,
  }) => {
    const { anna, submissionId } = await issuedToAnna(deployment);
    await submitThroughTheApi(deployment, anna, submissionId);
    await signInThroughTheApi(page, anna);
    await page.goto('/homework');
    const tabs = page.getByRole('navigation', { name: 'Homework lists' });
    await page.getByText('Nothing to do', { exact: true }).waitFor();

    await tabs.getByRole('link', { name: 'Submitted', exact: true }).focus();
    await page.keyboard.press('Enter');

    await expect(tabs.getByRole('link', { name: 'Submitted', exact: true })).toBeFocused();
    await expect(tabs.getByRole('link', { name: 'Submitted', exact: true })).toHaveAttribute('aria-current', 'page');
    await expect(page.getByRole('table', { name: 'Assignments' }).getByRole('row')).toHaveCount(2);
    await expect(page.getByRole('table', { name: 'Assignments' })).toContainText('Submitted');
    await expect(tabs.getByRole('link')).toHaveText(['To do', 'Submitted', 'Checked']);
  });

  test('a tutor filter that no current tutor holds shows the refusal without a list', async ({ page, deployment }) => {
    const { anna } = await issuedToAnna(deployment);
    await signInThroughTheApi(page, anna);

    await page.goto('/homework?tutor=6f1c2a9e-0b7d-4c55-9a21-5d2e8f4a6b70');

    await expect(page.getByText('This tutor is not available.', { exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'All tutors', exact: true })).toHaveAttribute('href', '/homework');
    await expect(page.getByRole('table', { name: 'Assignments' })).toHaveCount(0);
  });

  test('Show more moves focus to the first new row', async ({ page, deployment }) => {
    const { pavel, anna, annaCard } = await pavelWithAnna(deployment);
    const template = await createPastSimpleTemplate(deployment, pavel);
    await issueManyThroughTheApi(deployment, pavel, template, { relationship_id: annaCard }, 21);
    await signInThroughTheApi(page, anna);
    await page.goto('/homework');
    const links = page.getByRole('table', { name: 'Assignments' }).locator('[data-row-link]');
    await expect(links).toHaveCount(20);

    await page.getByRole('button', { name: 'Show more', exact: true }).click();

    await expect(links).toHaveCount(21);
    await expect(links.nth(20)).toBeFocused();
  });

  test('a live removal of the focused row moves focus to the row that took its place', async ({ page, deployment }) => {
    const { pavel, anna, annaCard } = await pavelWithAnna(deployment);
    const first = await createPastSimpleTemplate(deployment, pavel, { texts: { ...pastSimple, title: 'Week 1' } });
    const second = await createPastSimpleTemplate(deployment, pavel, { texts: { ...pastSimple, title: 'Week 2' } });
    const firstIssue = await issueThroughTheApi(deployment, pavel, first, { relationship_id: annaCard });
    await issueThroughTheApi(deployment, pavel, second, { relationship_id: annaCard });
    await signInThroughTheApi(page, anna);
    await page.goto('/homework');
    const table = page.getByRole('table', { name: 'Assignments' });
    await expect(table.locator('[data-row-link]')).toHaveCount(2);
    await table.getByRole('link', { name: 'Week 1', exact: true }).focus();

    await withdrawThroughTheApi(deployment, pavel, firstIssue);

    await expect(table.locator('[data-row-link]')).toHaveCount(1);
    await expect(table.getByRole('link', { name: 'Week 2', exact: true })).toBeFocused();
  });
});

test.describe('Home’s To do panel and the Homework count', () => {
  test('Home lists the work to do with Start, the count link and the sidebar count', async ({ page, deployment }) => {
    const { pavel, anna, annaCard } = await pavelWithAnna(deployment);
    const template = await createPastSimpleTemplate(deployment, pavel);
    await issueManyThroughTheApi(deployment, pavel, template, { relationship_id: annaCard }, 2);
    await signInThroughTheApi(page, anna);

    await page.goto('/home');

    const panel = page.getByRole('region', { name: 'To do' });
    await expect(panel.getByRole('listitem')).toHaveCount(2);
    await expect(panel.getByRole('link', { name: `Start ${pastSimple.title}` })).toHaveCount(2);
    await expect(panel.getByRole('link', { name: 'All homework, 2 to do' })).toHaveAttribute('href', '/homework');
    await expect(
      page.getByRole('navigation', { name: 'Main navigation' }).getByRole('link', { name: 'Homework, 2 to do' }),
    ).toBeVisible();
  });

  test('when a focused To do row that is not the last goes, focus moves to the row that took its place', async ({
    page,
    deployment,
  }) => {
    const { pavel, anna, annaCard } = await pavelWithAnna(deployment);
    const first = await createPastSimpleTemplate(deployment, pavel, { texts: { ...pastSimple, title: 'Week 1' } });
    const second = await createPastSimpleTemplate(deployment, pavel, { texts: { ...pastSimple, title: 'Week 2' } });
    const firstIssue = await issueThroughTheApi(deployment, pavel, first, { relationship_id: annaCard });
    const secondIssue = await issueThroughTheApi(deployment, pavel, second, { relationship_id: annaCard });
    await signInThroughTheApi(page, anna);
    await page.goto('/home');
    const panel = page.getByRole('region', { name: 'To do' });
    const rows = panel.locator('[data-row-link]');
    await expect(rows).toHaveCount(2);
    const firstIsWeek1 = (await rows.first().innerText()).includes('Week 1');
    const remaining = firstIsWeek1 ? 'Week 2' : 'Week 1';
    await rows.first().focus();

    await withdrawThroughTheApi(deployment, pavel, firstIsWeek1 ? firstIssue : secondIssue);

    await expect(rows).toHaveCount(1);
    await expect(rows.filter({ hasText: remaining })).toBeFocused();
  });

  test('when the last row of To do goes while focused, focus moves to the panel’s heading', async ({
    page,
    deployment,
  }) => {
    const { pavel, anna, issueId, submissionId } = await issuedToAnna(deployment);
    await signInThroughTheApi(page, anna);
    await page.goto('/home');
    const panel = page.getByRole('region', { name: 'To do' });
    await panel.locator(`[data-row="${submissionId}"] [data-row-link]`).focus();

    await withdrawThroughTheApi(deployment, pavel, issueId);

    await expect(panel.getByText('Nothing to do', { exact: true })).toBeVisible();
    await expect(panel.getByRole('heading', { name: 'To do' })).toBeFocused();
  });
});

interface SubmittedWork extends IssuedWork {
  readonly choiceQuestion: string;
  readonly textQuestion: string;
  readonly fileQuestion: string;
}

async function submittedByAnna(deployment: Deployment, policy: AnswerPolicy = 'after_check'): Promise<SubmittedWork> {
  const work = await issuedToAnna(deployment, false, policy);
  const [choiceQuestion, textQuestion, fileQuestion] = await questionIds(deployment, work.anna, work.submissionId);
  if (choiceQuestion === undefined || textQuestion === undefined || fileQuestion === undefined) {
    throw new Error('the assignment lacks its three questions');
  }
  await answerTextAndSubmit(deployment, work.anna, work.submissionId, 'went');
  return { ...work, choiceQuestion, textQuestion, fileQuestion };
}

const correctedPdf = { ...pdfFile, name: 'Corrections.pdf' };

async function correctedReview(
  deployment: Deployment,
  work: SubmittedWork,
  comment: string,
  points: number | null,
): Promise<ReviewItemSeed> {
  const corrected = await uploadCorrectedFile(
    deployment,
    work.pavel,
    work.submissionId,
    work.fileQuestion,
    correctedPdf,
  );
  return {
    questionId: work.fileQuestion,
    comment,
    correctedIds: [corrected],
    ...(points === null ? {} : { points }),
  };
}

function feedbackSection(page: Page): Locator {
  return page.getByRole('region', { name: 'Overall feedback' });
}

function optionLabel(page: Page, text: string): Locator {
  return question(page, 1).locator('label').filter({ hasText: text });
}

async function expectNoScoreText(page: Page): Promise<void> {
  expect(scoreFormsIn(await page.locator('main').innerText(), 'en')).toEqual([]);
}

async function shownResults(page: Page, student: TestAccount, submissionId: string): Promise<string> {
  await openAssignment(page, student, submissionId);
  await question(page, 3).waitFor();
  return page.locator('main').innerText();
}

test.describe('results of checked and returned work', () => {
  test('checked work shows the comment, the corrected file, the feedback and the revealed answers, and no points', async ({
    page,
    deployment,
  }) => {
    const work = await submittedByAnna(deployment);
    const item = await correctedReview(deployment, work, 'Line one.\nLine two.', 5);
    await publishReview(deployment, work.pavel, work.submissionId, 'check', '**Well done.** Mind the spelling.', [
      item,
    ]);

    await openAssignment(page, work.anna, work.submissionId);

    await expect(optionLabel(page, 'went')).toContainText('Correct answer');
    await expect(optionLabel(page, 'goed')).not.toContainText('Correct answer');
    await expect(question(page, 1)).toContainText('Go is irregular: go, went, gone.');
    await expect(question(page, 2)).toContainText('Accepted answers: went');
    await expect(question(page, 2)).toContainText('The past form of go is went.');
    await expect(question(page, 3)).toContainText('Tutor’s comment');
    await expect(question(page, 3).locator('[data-review-item] p[dir="auto"]')).toHaveText('Line one.\nLine two.');
    await expect(feedbackSection(page).locator('strong')).toHaveText('Well done.');
    const link = question(page, 3).getByRole('link', { name: correctedPdf.name, exact: true });
    await expect(link).toHaveAttribute('referrerpolicy', 'no-referrer');
    await expect(link).toHaveAttribute('rel', 'noopener noreferrer');
    await expectNoScoreText(page);
    expectStudentSubmissionKeys(await studentRead(deployment, work.anna, `/submissions/${work.submissionId}`));
  });

  test('the corrected file of checked work downloads under its name', async ({ page, deployment }) => {
    const work = await submittedByAnna(deployment);
    const item = await correctedReview(deployment, work, 'Line one.', 5);
    await publishReview(deployment, work.pavel, work.submissionId, 'check', 'Well done.', [item]);
    await openAssignment(page, work.anna, work.submissionId);
    const link = question(page, 3).getByRole('link', { name: correctedPdf.name, exact: true });
    await link.waitFor();
    const download = page.waitForEvent('download');

    await link.click();

    expect((await download).suggestedFilename()).toBe(correctedPdf.name);
  });

  test('checked work of a tutor who deleted their account stays readable and read-only', async ({
    page,
    deployment,
  }) => {
    const work = await submittedByAnna(deployment);
    const item = await correctedReview(deployment, work, 'Line one.', 5);
    await publishReview(deployment, work.pavel, work.submissionId, 'check', 'Well done.', [item]);
    await deleteAccount(deployment, work.pavel);

    await openAssignment(page, work.anna, work.submissionId);

    await expect(feedbackSection(page)).toContainText('Well done.');
    await expect(question(page, 3).getByRole('link', { name: correctedPdf.name, exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Submit', exact: true })).toHaveCount(0);
    await expect(textAnswer(page)).not.toBeEditable();
  });

  test('checked work’s results show no points: the points sentinel and every total stay out of the page and its responses', async ({
    page,
    deployment,
  }) => {
    const work = await issuedToAnna(deployment, true);
    const fileQuestion = await questionNumbered(deployment, work, 3);
    await answerSentinelsAndSubmit(deployment, work.anna, work.submissionId);
    await publishReview(deployment, work.pavel, work.submissionId, 'check', 'Checked.', [
      { questionId: fileQuestion, points: 5 },
    ]);
    const total = sentinel.points + 2 + 5;
    const finalPoints = await tutorFinalPoints(deployment, work.pavel, work.submissionId);
    const bodies = collectStudentResponses(page);

    await openAssignment(page, work.anna, work.submissionId);

    await expect(question(page, 1)).toContainText('Correct answer');
    await expect(feedbackSection(page)).toContainText('Checked.');
    const responses = await bodies();
    expect(responses.length).toBeGreaterThan(0);
    expect(finalPoints).toBe(total);
    expectNoPoints(
      [await page.locator('body').innerText(), ...responses],
      [sentinel.points, sentinel.points + 2, total, sentinel.points + 2 + 6],
    );
    await expectNoScoreText(page);
  });

  test('answers are revealed only as the policy and state allow', async ({ page, deployment }) => {
    const never = await submittedByAnna(deployment, 'never');
    await publishReview(deployment, never.pavel, never.submissionId, 'check', 'Checked.', [
      { questionId: never.fileQuestion, points: 4 },
    ]);
    const afterCheck = await submittedByAnna(deployment, 'after_check');
    const afterSubmission = await submittedByAnna(deployment, 'after_submission');

    const neverText = await shownResults(page, never.anna, never.submissionId);
    const afterCheckText = await shownResults(page, afterCheck.anna, afterCheck.submissionId);
    await shownResults(page, afterSubmission.anna, afterSubmission.submissionId);

    expect(neverText).toContain('Checked.');
    expect(neverText).not.toMatch(/Correct answer|Accepted answers|Explanation/);
    expect(afterCheckText).not.toMatch(/Correct answer|Accepted answers|Explanation/);
    await expect(optionLabel(page, 'went')).toContainText('Correct answer');
    await expect(question(page, 2)).toContainText('Accepted answers: went');
    await expectNoScoreText(page);
  });

  test('returned work shows the banner and the whole review, with only the file question open', async ({
    page,
    deployment,
  }) => {
    const work = await submittedByAnna(deployment);
    const item = await correctedReview(deployment, work, 'Your photo is blurred.', null);
    await publishReview(deployment, work.pavel, work.submissionId, 'return', 'Please **redo** the photo.', [item]);

    await openAssignment(page, work.anna, work.submissionId);

    await expect(page.getByText('Your tutor returned this work for revision.', { exact: true })).toBeVisible();
    await expect(feedbackSection(page).locator('strong')).toHaveText('redo');
    await expect(question(page, 3).locator('[data-review-item]')).toContainText('Your photo is blurred.');
    await expect(question(page, 3).getByRole('link', { name: correctedPdf.name })).toBeVisible();
    await expect(question(page, 1).getByRole('radio', { name: 'went' })).toBeDisabled();
    await expect(textAnswer(page)).not.toBeEditable();
    await expect(question(page, 3).getByRole('button', { name: 'Attach files', exact: true })).toBeVisible();
    expect(await page.locator('main').innerText()).not.toMatch(/Correct answer|Explanation/);
  });

  test('returned work resubmits with a new file and keeps the comment shown', async ({ page, deployment, control }) => {
    const work = await submittedByAnna(deployment);
    await publishReview(deployment, work.pavel, work.submissionId, 'return', 'Redo it.', [
      { questionId: work.fileQuestion, comment: 'Your photo is blurred.' },
    ]);
    await control.advance(1);
    await openAssignment(page, work.anna, work.submissionId, { pageClock: true });
    await attach(page, 3, secondPdf);
    await expect(removeButton(page, secondPdf.name)).toBeVisible();
    await skipVisibleSaveDelay(page);
    await expect(saveStatus(page)).toHaveText('Saved');
    await control.advance(1);

    await page.getByRole('button', { name: 'Submit again', exact: true }).click();
    await page.getByRole('dialog', { name: 'Submit your work?' }).getByRole('button', { name: 'Submit' }).click();

    await expect(submittedNotice(page)).toBeFocused();
    expect((await storedAnswers(deployment, work.anna, work.submissionId)).status).toBe('submitted');
    await expect(question(page, 3).locator('[data-review-item]')).toContainText('Your photo is blurred.');
  });

  test('closed work shows its published review read-only', async ({ page, deployment }) => {
    const work = await submittedByAnna(deployment);
    await publishReview(deployment, work.pavel, work.submissionId, 'return', 'Try again.', [
      { questionId: work.fileQuestion, comment: 'Add a second photo.' },
    ]);
    await endRelationship(deployment, work.pavel, work.annaCard);

    await openAssignment(page, work.anna, work.submissionId);

    await expect(page.getByText('This assignment is closed and can no longer be submitted.')).toBeVisible();
    await expect(question(page, 3).locator('[data-review-item]')).toContainText('Add a second photo.');
    await expect(feedbackSection(page)).toContainText('Try again.');
    await expect(page.getByRole('button', { name: /^Submit/ })).toHaveCount(0);
  });

  test('a review published while the page is open appears without a reload', async ({ page, deployment }) => {
    const work = await submittedByAnna(deployment);
    await openAssignment(page, work.anna, work.submissionId);
    await expect(page.locator('main')).toContainText('Submitted');

    await publishReview(deployment, work.pavel, work.submissionId, 'check', 'Published live.', [
      { questionId: work.fileQuestion, points: 3 },
    ]);

    await expect(feedbackSection(page)).toContainText('Published live.');
    await expect(optionLabel(page, 'went')).toContainText('Correct answer');
  });
});

test.describe('Home’s Recently checked panel', () => {
  test('lists checked work with the excerpt and the corrected-file note, and no points', async ({
    page,
    deployment,
  }) => {
    const work = await submittedByAnna(deployment);
    const item = await correctedReview(deployment, work, 'Fine.', 5);
    await publishReview(deployment, work.pavel, work.submissionId, 'check', '**Nice.** Keep going.', [item]);
    await signInThroughTheApi(page, work.anna);

    await page.goto('/home');

    const panel = page.getByRole('region', { name: 'Recently checked' });
    await expect(panel.getByRole('listitem')).toHaveCount(1);
    await expect(panel.getByRole('link', { name: pastSimple.title })).toHaveAttribute(
      'href',
      `/homework/${work.submissionId}`,
    );
    await expect(panel.locator('[data-excerpt]')).toHaveText('Nice. Keep going.');
    await expect(panel).toContainText('A corrected file came back');
    await expect(panel).toContainText('Checked ');
    expect(scoreFormsIn(await panel.innerText(), 'en')).toEqual([]);
    expectAssignmentListKeys(await studentRead(deployment, work.anna, '/assignments?status=checked&limit=5'));
  });

  test('without checked work it says so, and work checked meanwhile appears without a reload', async ({
    page,
    deployment,
  }) => {
    const work = await submittedByAnna(deployment);
    await signInThroughTheApi(page, work.anna);
    await page.goto('/home');
    const panel = page.getByRole('region', { name: 'Recently checked' });
    await expect(panel).toContainText('Nothing checked yet');

    await publishReview(deployment, work.pavel, work.submissionId, 'check', '', [
      { questionId: work.fileQuestion, points: 6 },
    ]);

    await expect(panel.getByRole('listitem')).toHaveCount(1);
    await expect(panel.locator('[data-excerpt]')).toHaveCount(0);
  });

  test('a newer check pushing the focused last row out moves focus to the row that took its place', async ({
    page,
    deployment,
    control,
  }) => {
    const { pavel, anna, annaCard } = await pavelWithAnna(deployment);
    const weeks = ['Week 1', 'Week 2', 'Week 3', 'Week 4', 'Week 5', 'Week 6'];
    const submissions: string[] = [];
    for (const title of weeks) {
      const template = await createPastSimpleTemplate(deployment, pavel, { texts: { ...pastSimple, title } });
      await issueThroughTheApi(deployment, pavel, template, { relationship_id: annaCard });
      const submissionId = await onlySubmission(deployment, anna);
      await answerTextAndSubmit(deployment, anna, submissionId, 'went');
      submissions.push(submissionId);
    }
    const checkWeek = async (index: number): Promise<void> => {
      await control.advance(60);
      const submissionId = submissions[index] ?? '';
      const [, , fileQuestion] = await questionIds(deployment, anna, submissionId);
      await publishReview(deployment, pavel, submissionId, 'check', weeks[index] ?? '', [
        { questionId: fileQuestion ?? '', points: 6 },
      ]);
    };
    for (const index of [0, 1, 2, 3, 4]) {
      await checkWeek(index);
    }
    await signInThroughTheApi(page, anna);
    await page.goto('/home');
    const links = page.getByRole('region', { name: 'Recently checked' }).locator('[data-row-link]');
    await expect(links).toHaveCount(5);
    await expect(links.last()).toHaveText('Week 1');
    await links.last().focus();

    await checkWeek(5);

    await expect(links.first()).toHaveText('Week 6');
    await expect(links).toHaveText(['Week 6', 'Week 5', 'Week 4', 'Week 3', 'Week 2']);
    await expect(links.last()).toBeFocused();
  });

  test('an unconfirmed student on a provisional link sees their checked work', async ({ page, deployment }) => {
    const pavel = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const anna = newTestAccount('Anna', 'student');
    const card = await createCard(deployment, pavel, 'Anna', anna.email);
    await signUpOnInvitation(deployment, card.link, anna, 'en');
    const template = await createPastSimpleTemplate(deployment, pavel);
    await issueThroughTheApi(deployment, pavel, template, { relationship_id: card.id });
    const submissionId = await onlySubmission(deployment, anna);
    await answerTextAndSubmit(deployment, anna, submissionId, 'went');
    const [, , fileQuestion] = await questionIds(deployment, anna, submissionId);
    await publishReview(deployment, pavel, submissionId, 'check', 'Provisional work checked.', [
      { questionId: fileQuestion ?? '', points: 6 },
    ]);
    await signInThroughTheApi(page, anna);

    await page.goto('/home');

    await expect(page.getByRole('region', { name: 'Recently checked' }).locator('[data-excerpt]')).toHaveText(
      'Provisional work checked.',
    );
  });
});

test.describe('the New marker on Home’s Recently checked panel on a phone', () => {
  test.use({ viewport: { width: 360, height: 780 } });

  test('shows until the results are opened, which sends one feedback-seen, and is gone back on Home', async ({
    page,
    deployment,
  }) => {
    const work = await submittedByAnna(deployment);
    await publishReview(deployment, work.pavel, work.submissionId, 'check', 'Well done.', [
      { questionId: work.fileQuestion, points: 6 },
    ]);
    await signInThroughTheApi(page, work.anna);
    const seen: string[] = [];
    page.on('request', (request) => {
      if (new URL(request.url()).pathname === `/api/v1/submissions/${work.submissionId}/feedback-seen`) {
        seen.push(request.method());
      }
    });
    await page.goto('/home');
    const panel = page.getByRole('region', { name: 'Recently checked' });
    await panel.getByRole('img', { name: 'New feedback' }).waitFor();

    await panel.getByRole('link', { name: pastSimple.title }).click();
    await page.getByRole('heading', { level: 1, name: pastSimple.title }).waitFor();
    await expect.poll(() => seen.length).toBe(1);
    await page.goBack();
    await panel.getByRole('link', { name: pastSimple.title }).waitFor();

    await expect(panel.getByRole('img', { name: 'New feedback' })).toHaveCount(0);
    expect(seen).toEqual(['POST']);
    expect(scoreFormsIn(await panel.innerText(), 'en')).toEqual([]);
  });
});

interface PilotTwoSeed {
  readonly maria: TestAccount;
  readonly anna: TestAccount;
  readonly boris: TestAccount;
}

let pilotTwoSeed: PilotTwoSeed | null = null;
let pilotTwoBorisWork: string | null = null;
let pilotTwoDeadlineDate: string | null = null;

function pilotTwoDeadline(): string {
  if (pilotTwoDeadlineDate === null) {
    throw new Error('pilot 2 step 6 has not run');
  }
  return pilotTwoDeadlineDate;
}

function pilotTwoBoris(): string {
  if (pilotTwoBorisWork === null) {
    throw new Error('pilot 2 step 11 has not run');
  }
  return pilotTwoBorisWork;
}

function pilotTwo(): PilotTwoSeed {
  if (pilotTwoSeed === null) {
    throw new Error('the pilot 2 seed has not run');
  }
  return pilotTwoSeed;
}

const pilotTimezone = 'Europe/Moscow';
const annaTimezone = 'Europe/Berlin';

interface ActorBrowser {
  readonly locale: string;
  readonly timezoneId: string;
  readonly colorScheme: 'light' | 'dark';
  readonly viewport: { readonly width: number; readonly height: number };
}

const annaBrowser: ActorBrowser = {
  locale: 'ru-RU',
  timezoneId: annaTimezone,
  colorScheme: 'light',
  viewport: { width: 390, height: 844 },
};

const borisBrowser: ActorBrowser = {
  locale: 'en-GB',
  timezoneId: pilotTimezone,
  colorScheme: 'dark',
  viewport: { width: 1280, height: 800 },
};

interface ActorPage {
  readonly context: BrowserContext;
  readonly page: Page;
  readonly watched: PageDiagnostics;
}

async function openActorPage(
  browser: ObservedBrowser,
  deployment: Deployment,
  diagnostics: Diagnostics,
  actor: ActorBrowser,
): Promise<ActorPage> {
  const context = await browser.newContext({
    baseURL: deployment.origin,
    ignoreHTTPSErrors: true,
    locale: actor.locale,
    timezoneId: actor.timezoneId,
    colorScheme: actor.colorScheme,
    viewport: { ...actor.viewport },
  });
  const page = await context.newPage();
  return { context, page, watched: await diagnostics.watch(page) };
}

function deadlineInBerlin(moscowDate: string): string {
  const instant = Date.parse(`${moscowDate}T23:59:00+03:00`);
  const day = new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'short', timeZone: annaTimezone }).format(
    instant,
  );
  const clock = new Intl.DateTimeFormat('ru-RU', {
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
    timeZone: annaTimezone,
  }).format(instant);
  return `${day}, ${clock}`;
}

test.describe('pilot 2, group homework from the library', () => {
  test.describe.configure({ mode: 'serial' });
  test.use({ locale: 'ru-RU', timezoneId: pilotTimezone, colorScheme: 'light' });

  test('pilot-2 › seed Maria teaches Anna and Boris in B1 evening and has Past simple, week 1 in the library', async ({
    deployment,
    control,
  }) => {
    const maria = await signUpAndConfirmTutor(deployment, 'Мария');
    const anna = await signUpActiveStudentOnInvitation(deployment, control, maria, 'Анна', 'ru', annaTimezone);
    const boris = await signUpStudentThroughCode(deployment, control, maria, 'Борис', 'en');

    await saveTheme(deployment, boris.student, 'dark');
    const groupId = await createGroup(deployment, maria, 'B1 evening', [anna.cardId, boris.cardId]);
    const templateId = await createPastSimpleTemplate(deployment, maria, { withMaterial: true });

    expect(groupId).not.toBe('');
    expect(templateId).not.toBe('');
    expect(await accountPreferences(deployment, anna.student)).toEqual({
      language: 'ru',
      timezone: annaTimezone,
      theme: 'system',
    });
    expect(await accountPreferences(deployment, boris.student)).toEqual({
      language: 'en',
      timezone: pilotTimezone,
      theme: 'dark',
    });
    pilotTwoSeed = { maria, anna: anna.student, boris: boris.student };
  });

  test('pilot-2 › step 6 Maria issues Past simple, week 1 to B1 evening with a deadline in 2 days, answers after checking', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = pilotTwo();
    const deadline = (await control.now()) + 2 * 86_400_000;
    await signInThroughTheApi(page, seed.maria);
    await page.goto('/homework/library');
    await page.getByRole('button', { name: `Действия: ${pastSimple.title}`, exact: true }).click();
    await page
      .locator('main [aria-expanded="true"] + div')
      .getByRole('button', { name: 'Выдать задание', exact: true })
      .click();
    const dialog = page.getByRole('dialog', { name: 'Выдать задание' });
    await dialog.getByRole('button', { name: 'B1 evening', exact: true }).click();
    await dialog.getByRole('checkbox', { name: 'Указать срок' }).check();
    pilotTwoDeadlineDate = localDate(deadline, pilotTimezone);
    await dialog.getByLabel('Дата', { exact: true }).fill(pilotTwoDeadlineDate);
    await dialog.getByRole('radio', { name: 'После проверки' }).check();

    await dialog.getByRole('button', { name: 'Выдать', exact: true }).click();

    await expect(dialog).toBeHidden();
    await expect(page.getByText('Задание выдано.', { exact: true })).toBeFocused();
    expect((await studentWork(deployment, seed.anna, 'todo')).map((work) => work.title)).toEqual([pastSimple.title]);
    expect((await studentWork(deployment, seed.boris, 'todo')).map((work) => work.title)).toEqual([pastSimple.title]);
    await control.tick();
    for (const student of [seed.anna, seed.boris]) {
      const mail = await control.mail(student.email);
      expect(
        mail.some((message) => message.links.some((link) => new URL(link).pathname.startsWith('/homework/'))),
      ).toBe(true);
    }
  });

  test('pilot-2 › step 7 Anna answers on her phone, the draft saves while she types, and after submitting she sees Submitted with no points or correct answers', async ({
    observedBrowser: browser,
    deployment,
    diagnostics,
    control,
  }) => {
    const seed = pilotTwo();
    const submissionId = await onlySubmission(deployment, seed.anna);
    const { context, page } = await openActorPage(browser, deployment, diagnostics, annaBrowser);
    await installPageClock(page, { pageClock: true });
    await signInThroughTheApi(page, seed.anna);
    await page.goto('/home');
    await page
      .getByRole('region', { name: 'Нужно сделать' })
      .getByRole('link', { name: `Начать ${pastSimple.title}` })
      .click();
    await page.getByRole('heading', { level: 1, name: pastSimple.title }).waitFor();

    await page.getByRole('group', { name: 'Вопрос 1' }).getByRole('radio', { name: 'went' }).check();
    await page.getByRole('group', { name: 'Вопрос 2' }).getByRole('textbox', { name: 'Ваш ответ' }).fill('went');
    await skipVisibleSaveDelay(page);
    await expect(page.getByRole('status').filter({ hasText: /^Сохранено$/ })).toBeVisible();
    await control.advance(1);
    const chooser = page.waitForEvent('filechooser');
    await page.getByRole('group', { name: 'Вопрос 3' }).getByRole('button', { name: 'Сфотографировать' }).click();
    await (await chooser).setFiles(pictureFile);
    await expect(
      page.getByRole('group', { name: 'Вопрос 3' }).getByRole('button', { name: `Удалить ${pictureFile.name}` }),
    ).toBeVisible();
    await skipVisibleSaveDelay(page);
    await expect(page.getByRole('status').filter({ hasText: /^Сохранено$/ })).toBeVisible();
    await control.advance(1);
    await page.getByRole('button', { name: 'Сдать', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Сдать работу?' })
      .getByRole('button', { name: 'Сдать', exact: true })
      .click();

    await expect(page.getByRole('status').filter({ hasText: /^Сдано$/ })).toBeFocused();
    const text = await page.locator('main').innerText();
    expect(text).not.toMatch(/балл|Правильн|Пояснение/);
    expect(text).toContain(deadlineInBerlin(pilotTwoDeadline()));
    expect(text).not.toContain('23:59');
    const stored = await storedAnswers(deployment, seed.anna, submissionId);
    expect(stored.status).toBe('submitted');
    expect(stored.files).toEqual([pictureFile.name]);
    await diagnostics.closeContext(context);
  });

  test('pilot-2 › steps 8–9 after the deadline Boris submits goed, Went and a PDF, and his work is marked late without points', async ({
    observedBrowser: browser,
    deployment,
    diagnostics,
    control,
  }) => {
    const seed = pilotTwo();
    await control.advance(3 * 86_400);
    const submissionId = await onlySubmission(deployment, seed.boris);
    const { context, page } = await openActorPage(browser, deployment, diagnostics, borisBrowser);
    await installPageClock(page, { pageClock: true });
    await signInThroughTheApi(page, seed.boris);
    await page.goto(`/homework/${submissionId}`);
    await page.getByRole('heading', { level: 1, name: pastSimple.title }).waitFor();

    await page.getByRole('group', { name: 'Question 1' }).getByRole('radio', { name: 'goed' }).check();
    await page.getByRole('group', { name: 'Question 2' }).getByRole('textbox', { name: 'Your answer' }).fill('Went ');
    await skipVisibleSaveDelay(page);
    await expect(page.getByRole('status').filter({ hasText: /^Saved$/ })).toBeVisible();
    await control.advance(1);
    const chooser = page.waitForEvent('filechooser');
    await page.getByRole('group', { name: 'Question 3' }).getByRole('button', { name: 'Attach files' }).click();
    await (await chooser).setFiles(pdfFile);
    await expect(
      page.getByRole('group', { name: 'Question 3' }).getByRole('button', { name: `Remove ${pdfFile.name}` }),
    ).toBeVisible();
    await skipVisibleSaveDelay(page);
    await expect(page.getByRole('status').filter({ hasText: /^Saved$/ })).toBeVisible();
    await control.advance(1);
    await page.getByRole('button', { name: 'Submit', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Submit your work?' })
      .getByRole('button', { name: 'Submit', exact: true })
      .click();

    await expect(page.getByRole('status').filter({ hasText: /^Submitted$/ })).toBeFocused();
    await expect(page.locator('main')).toContainText('Submitted late');
    expect(await page.locator('main').innerText()).not.toMatch(/\bpoints?\b/i);
    const stored = await storedAnswers(deployment, seed.boris, submissionId);
    expect(stored.late).toBe(true);
    expect(stored.status).toBe('submitted');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await diagnostics.closeContext(context);
  });

  test('pilot-2 › step 9 Maria’s view of Boris’s late work shows 2 automatic points for “Went ”', async ({
    page,
    deployment,
  }) => {
    const seed = pilotTwo();
    const [work] = await studentWork(deployment, seed.boris, 'submitted');
    if (work === undefined) {
      throw new Error('Boris has no submitted work');
    }
    await signInThroughTheApi(page, seed.maria);

    await page.goto(`/homework/review/${work.submissionId}`);

    await page.getByRole('heading', { level: 1, name: pastSimple.title }).waitFor();
    await expect(page.getByRole('region', { name: 'Вопрос 2' })).toContainText('2 балла за автопроверку');
    await expect(page.locator('main')).toContainText('С опозданием');
  });

  test('pilot-2 › step 10 Maria opens Anna’s work from Today, attaches a corrected file, gives 6 points, writes the feedback and marks it checked', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = pilotTwo();
    const [annaWork] = await studentWork(deployment, seed.anna, 'submitted');
    if (annaWork === undefined) {
      throw new Error('Anna has no submitted work');
    }
    const mailBefore = (await control.mail(seed.anna.email)).length;
    const corrected = { ...pdfFile, name: 'Исправления.pdf' };
    const fileQuestion = page.getByRole('region', { name: 'Вопрос 3' });
    const saved = page.getByRole('status').filter({ hasText: /^Сохранено$/ });
    await installPageClock(page, { pageClock: true });
    await signInThroughTheApi(page, seed.maria);
    await page.goto('/today');
    await page
      .getByRole('region', { name: /^На проверке/ })
      .getByRole('link', { name: 'Анна' })
      .click();
    await page.getByRole('heading', { level: 1, name: pastSimple.title }).waitFor();
    const chooser = page.waitForEvent('filechooser');
    await fileQuestion.getByRole('button', { name: 'Прикрепить исправленный файл' }).click();
    await (await chooser).setFiles(corrected);
    await expect(fileQuestion.getByRole('button', { name: `Удалить ${corrected.name}` })).toBeVisible();
    await skipVisibleSaveDelay(page);
    await expect(saved).toBeVisible();
    await control.advance(1);
    await fileQuestion.getByRole('spinbutton', { name: 'Баллы' }).fill('6');
    await skipVisibleSaveDelay(page);
    await expect(saved).toBeVisible();
    await control.advance(1);
    await page
      .getByRole('textbox', { name: 'Общий отзыв' })
      .fill('**Great work.** Check the word order in sentence 2.');
    await skipVisibleSaveDelay(page);
    await expect(saved).toBeVisible();
    await control.advance(1);
    await page.getByRole('button', { name: 'Предпросмотр' }).click();
    await expect(page.locator('[data-feedback-preview] strong')).toHaveText('Great work.');

    await page.getByRole('button', { name: 'Отметить проверенной' }).click();

    await expect(page.getByRole('status').filter({ hasText: 'Проверено. Ученик видит вашу проверку.' })).toBeFocused();
    const review = await storedReview(deployment, seed.maria, annaWork.submissionId);
    expect(review.status).toBe('checked');
    const published = review.revisions.find((revision) => revision.state === 'published');
    expect(published?.feedback).toBe('**Great work.** Check the word order in sentence 2.');
    expect(published?.items.flatMap((item) => item.correctedFiles)).toEqual([corrected.name]);
    expect(await studentSubmissionText(deployment, seed.anna, annaWork.submissionId)).not.toMatch(
      /"(points|final_points|score|percentage|max_points)"/,
    );
    await control.tick();
    const mail = await control.mail(seed.anna.email);
    expect(mail.length).toBeGreaterThan(mailBefore);
    expect(
      mail
        .slice(mailBefore)
        .some((message) =>
          message.links.some((link) => new URL(link).pathname === `/homework/${annaWork.submissionId}`),
        ),
    ).toBe(true);
  });
  test('pilot-2 › step 11 Maria comments on Boris’s file question and returns his work', async ({
    page,
    deployment,
  }) => {
    const seed = pilotTwo();
    const [borisWork] = await studentWork(deployment, seed.boris, 'submitted');
    if (borisWork === undefined) {
      throw new Error('Boris has no submitted work');
    }
    const fileQuestion = page.getByRole('region', { name: 'Вопрос 3' });
    await installPageClock(page, { pageClock: true });
    await signInThroughTheApi(page, seed.maria);
    await page.goto(`/homework/review/${borisWork.submissionId}`);
    await page.getByRole('heading', { level: 1, name: pastSimple.title }).waitFor();
    await fileQuestion.getByRole('textbox', { name: 'Комментарий' }).fill('Сфотографируйте все предложения.');
    await skipVisibleSaveDelay(page);
    await expect(page.getByRole('status').filter({ hasText: /^Сохранено$/ })).toBeVisible();

    await page.getByRole('button', { name: 'Вернуть на доработку' }).click();

    await expect(page.getByRole('status').filter({ hasText: 'Возвращено на доработку.' })).toBeFocused();
    expect((await storedAnswers(deployment, seed.boris, borisWork.submissionId)).status).toBe('returned');
    pilotTwoBorisWork = borisWork.submissionId;
  });

  test('pilot-2 › step 12 Boris sees the comment with only the file question open, uploads a new file and submits again', async ({
    observedBrowser: browser,
    deployment,
    diagnostics,
    control,
  }) => {
    const seed = pilotTwo();
    const submissionId = pilotTwoBoris();
    const mailBefore = (await control.mail(seed.maria.email)).length;
    const { context, page } = await openActorPage(browser, deployment, diagnostics, borisBrowser);
    await installPageClock(page, { pageClock: true });
    await signInThroughTheApi(page, seed.boris);
    await page.goto(`/homework/${submissionId}`);
    await page.getByRole('heading', { level: 1, name: pastSimple.title }).waitFor();
    await expect(page.getByText('Your tutor returned this work for revision.', { exact: true })).toBeVisible();
    await expect(page.getByRole('group', { name: 'Question 3' }).locator('[data-review-item]')).toContainText(
      'Сфотографируйте все предложения.',
    );
    await expect(page.getByRole('group', { name: 'Question 1' }).getByRole('radio', { name: 'goed' })).toBeDisabled();
    await expect(
      page.getByRole('group', { name: 'Question 2' }).getByRole('textbox', { name: 'Your answer' }),
    ).not.toBeEditable();
    const chooser = page.waitForEvent('filechooser');
    await page.getByRole('group', { name: 'Question 3' }).getByRole('button', { name: 'Attach files' }).click();
    await (await chooser).setFiles(pictureFile);
    await expect(
      page.getByRole('group', { name: 'Question 3' }).getByRole('button', { name: `Remove ${pictureFile.name}` }),
    ).toBeVisible();
    await skipVisibleSaveDelay(page);
    await expect(page.getByRole('status').filter({ hasText: /^Saved$/ })).toBeVisible();
    await control.advance(1);

    await page.getByRole('button', { name: 'Submit again', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Submit your work?' })
      .getByRole('button', { name: 'Submit', exact: true })
      .click();

    await expect(page.getByRole('status').filter({ hasText: /^Submitted$/ })).toBeFocused();
    expect((await storedAnswers(deployment, seed.boris, submissionId)).status).toBe('submitted');
    expect(await reviewQueueIds(deployment, seed.maria)).toContain(submissionId);
    await control.tick();
    const mail = await control.mail(seed.maria.email);
    expect(
      mail
        .slice(mailBefore)
        .some((message) => message.links.some((link) => new URL(link).pathname.endsWith(submissionId))),
    ).toBe(true);
    await diagnostics.closeContext(context);
  });

  test('pilot-2 › step 13 Maria gives the file question 4 points and checks Boris’s work at 6 of 10', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = pilotTwo();
    const submissionId = pilotTwoBoris();
    const fileQuestion = page.getByRole('region', { name: 'Вопрос 3' });
    await installPageClock(page, { pageClock: true });
    await signInThroughTheApi(page, seed.maria);
    await page.goto(`/homework/review/${submissionId}`);
    await page.getByRole('heading', { level: 1, name: pastSimple.title }).waitFor();
    await fileQuestion.getByRole('spinbutton', { name: 'Баллы' }).fill('4');
    await skipVisibleSaveDelay(page);
    await expect(page.getByRole('status').filter({ hasText: /^Сохранено$/ })).toBeVisible();
    await control.advance(1);

    await page.getByRole('button', { name: 'Отметить проверенной' }).click();

    await expect(page.getByRole('status').filter({ hasText: 'Проверено. Ученик видит вашу проверку.' })).toBeFocused();
    expect((await storedReview(deployment, seed.maria, submissionId)).status).toBe('checked');
    expect(await tutorFinalPoints(deployment, seed.maria, submissionId)).toBe(6);
  });

  test('pilot-2 › step 14 Anna reads her results and Home’s Recently checked on her phone, with no points anywhere', async ({
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const seed = pilotTwo();
    const [annaWork] = await studentWork(deployment, seed.anna, 'checked');
    if (annaWork === undefined) {
      throw new Error('Anna has no checked work');
    }
    const { context, page } = await openActorPage(browser, deployment, diagnostics, annaBrowser);
    await signInThroughTheApi(page, seed.anna);
    await page.goto(`/homework/${annaWork.submissionId}`);
    await page.getByRole('heading', { level: 1, name: pastSimple.title }).waitFor();
    await expect(page.getByRole('region', { name: 'Общий отзыв' }).locator('strong')).toHaveText('Great work.');
    await expect(
      page.getByRole('group', { name: 'Вопрос 1' }).locator('label').filter({ hasText: 'went' }),
    ).toContainText('Правильный ответ');
    await expect(page.getByRole('group', { name: 'Вопрос 2' })).toContainText('Правильные ответы: went');
    await expect(page.getByRole('group', { name: 'Вопрос 2' })).toContainText('Пояснение');
    await expect(page.getByRole('group', { name: 'Вопрос 3' })).toContainText('Check the word order.');
    const download = page.waitForEvent('download');

    await page.getByRole('group', { name: 'Вопрос 3' }).getByRole('link', { name: 'Исправления.pdf' }).click();

    expect((await download).suggestedFilename()).toBe('Исправления.pdf');
    expect(scoreFormsIn(await page.locator('main').innerText(), 'ru')).toEqual([]);
    expect(await page.locator('main').innerText()).not.toMatch(/(?<!\d)60(?!\d)/);
    expectStudentSubmissionKeys(await studentRead(deployment, seed.anna, `/submissions/${annaWork.submissionId}`));
    await page.goto('/home');
    const panel = page.getByRole('region', { name: 'Недавно проверенные' });
    await expect(panel.locator('[data-excerpt]')).toHaveText('Great work. Check the word order in sentence 2.');
    await expect(panel).toContainText('Есть исправленный файл');
    expect(scoreFormsIn(await page.locator('main').innerText(), 'ru')).toEqual([]);
    expectAssignmentListKeys(await studentRead(deployment, seed.anna, '/assignments?status=checked&limit=5'));
    await diagnostics.closeContext(context);
  });

  test('pilot-2 › step 16 Anna opens Boris’s submission address and it is not found', async ({
    observedBrowser: browser,
    deployment,
    diagnostics,
  }) => {
    const seed = pilotTwo();
    const submissionId = pilotTwoBoris();
    const { context, page, watched } = await openActorPage(browser, deployment, diagnostics, annaBrowser);
    watched.expectFailure({ method: 'GET', path: `/api/v1/submissions/${submissionId}`, status: 404 });
    await signInThroughTheApi(page, seed.anna);

    await page.goto(`/homework/${submissionId}`);

    await expect(page.getByRole('heading', { level: 1, name: 'Страница не найдена' })).toBeVisible();
    await expect(page.locator('main')).not.toContainText(pastSimple.choicePrompt);
    await diagnostics.closeContext(context);
  });
});
