import type { Locator, Page, Response } from '@playwright/test';

import { type TestAccount, newTestAccount, signInThroughTheApi } from './accounts.ts';
import {
  createPastSimpleTemplate,
  onlySubmission,
  pastSimple,
  pdfFile,
  pictureFile,
  questionIds,
  removeGroupMember,
  saveTextFromAnotherDevice,
  sentinel,
  bindAndDropFromAnotherDevice,
  storedAnswers,
  uploadedId,
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
  signUpAndConfirmTutor,
} from './fixtures.ts';
import { localDate } from './schedule.ts';
import { issueManyThroughTheApi, issueThroughTheApi, withdrawThroughTheApi } from './homework.ts';
import { storedReview, studentSubmissionText } from './review.ts';
import {
  createCard,
  createGroup,
  endRelationship,
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

async function issuedToAnna(deployment: Deployment, withSentinels = false): Promise<IssuedWork> {
  const actors = await pavelWithAnna(deployment);
  const template = await createPastSimpleTemplate(deployment, actors.pavel, { withSentinels });
  const issueId = await issueThroughTheApi(deployment, actors.pavel, template, { relationship_id: actors.annaCard });
  return { ...actors, issueId, submissionId: await onlySubmission(deployment, actors.anna) };
}

function submittedNotice(page: Page): Locator {
  return page.getByRole('status').filter({ hasText: /^Submitted$/ });
}

async function openAssignment(page: Page, student: TestAccount, submissionId: string): Promise<void> {
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
  await openAssignment(page, work.anna, work.submissionId);
  const release = await holdAnswerSaves(page);
  const firstSave = page.waitForRequest((request) => request.method() === 'PUT');
  diagnostics.expectFailure({ method: 'PUT', path: `/api/v1/submissions/${work.submissionId}/answers`, status: 409 });
  if (mine.choice !== undefined) {
    await question(page, 1).getByRole('radio', { name: mine.choice }).check();
  }
  await textAnswer(page).fill(mine.text);
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
  await openAssignment(page, work.anna, work.submissionId);
  await attach(page, 3, pdfFile);
  await expect(saveStatus(page)).toHaveText('Saved');
  await control.advance(1);
  await attach(page, 3, secondPdf);
  await expect(removeButton(page, secondPdf.name)).toBeVisible();
  await expect(saveStatus(page)).toHaveText('Saved');
  await control.advance(1);
  return work;
}

function collectStudentResponses(page: Page): () => Promise<string[]> {
  const bodies: Promise<string>[] = [];
  const collect = (response: Response): void => {
    const path = new URL(response.url()).pathname;
    if (path.startsWith('/api/v1/submissions/') || path === '/api/v1/assignments') {
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
    await openAssignment(page, anna, submissionId);
    const release = await holdAnswerSaves(page);
    const firstSave = page.waitForRequest((request) => request.method() === 'PUT');
    await textAnswer(page).fill('went');
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
    await openAssignment(page, anna, submissionId);

    await attach(page, 3, pdfFile);

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
    await openAssignment(page, anna, submissionId);
    await attach(page, 3, pdfFile);
    await expect(saveStatus(page)).toHaveText('Saved');
    await control.advance(1);

    await removeButton(page, pdfFile.name).click();

    await expect(question(page, 3).getByRole('button', { name: 'Attach files', exact: true })).toBeFocused();
    await expect(saveStatus(page)).toHaveText('Saved');
    expect((await storedAnswers(deployment, anna, submissionId)).files).toEqual([]);
  });

  test('Remove moves focus to the next file’s Remove', async ({ page, deployment, control }) => {
    const { anna, submissionId } = await twoFilesAttached(page, deployment, control);

    await removeButton(page, pdfFile.name).click();

    await expect(removeButton(page, secondPdf.name)).toBeFocused();
    await expect
      .poll(async () => (await storedAnswers(deployment, anna, submissionId)).files)
      .toEqual([secondPdf.name]);
  });

  test('Remove on the last file moves focus to the previous file’s Remove', async ({ page, deployment, control }) => {
    const { anna, submissionId } = await twoFilesAttached(page, deployment, control);

    await removeButton(page, secondPdf.name).click();

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
    await openAssignment(page, anna, submissionId);
    const release = await holdAnswerSaves(page);
    const firstSave = page.waitForRequest((request) => request.method() === 'PUT');
    const uploaded = page.waitForResponse(
      (response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/uploads',
    );
    diagnostics.expectFailure({ method: 'PUT', path: `/api/v1/submissions/${submissionId}/answers`, status: 409 });
    diagnostics.expectFailure({ method: 'PUT', path: `/api/v1/submissions/${submissionId}/answers`, status: 404 });
    await attach(page, 3, pdfFile);
    const attachment = await uploadedId(await uploaded);
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

  test('submitting shows the focused Submitted notice, with no points, accepted answers or explanations anywhere', async ({
    page,
    deployment,
    control,
  }) => {
    const { anna, submissionId } = await issuedToAnna(deployment, true);
    const bodies = collectStudentResponses(page);
    await openAssignment(page, anna, submissionId);
    await question(page, 1).getByRole('radio', { name: 'goed' }).check();
    await textAnswer(page).fill('went');
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

  test('a student removed from the group still submits the group’s work', async ({ page, deployment }) => {
    const { pavel, anna, annaCard } = await pavelWithAnna(deployment);
    const groupId = await createGroup(deployment, pavel, 'B1 evening', [annaCard]);
    const template = await createPastSimpleTemplate(deployment, pavel);
    await issueThroughTheApi(deployment, pavel, template, { group_id: groupId });
    await removeGroupMember(deployment, pavel, groupId, annaCard);
    const submissionId = await onlySubmission(deployment, anna);
    await openAssignment(page, anna, submissionId);
    await textAnswer(page).fill('went');

    await page.getByRole('button', { name: 'Submit', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Submit your work?' })
      .getByRole('button', { name: 'Submit', exact: true })
      .click();

    await expect(submittedNotice(page)).toBeFocused();
    expect((await storedAnswers(deployment, anna, submissionId)).status).toBe('submitted');
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
    await signInThroughTheApi(page, anna);
    await page.goto('/home');

    await page
      .getByRole('navigation', { name: 'Main navigation' })
      .getByRole('link', { name: 'Homework, 1 to do' })
      .click();
    await page.getByRole('table', { name: 'Assignments' }).getByRole('link', { name: pastSimple.title }).click();
    await textAnswer(page).fill('went');

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
    await openAssignment(page, anna, submissionId);
    const release = await holdAnswerSaves(page);
    const firstSave = page.waitForRequest((request) => request.method() === 'PUT');
    diagnostics.expectFailure({ method: 'PUT', path: `/api/v1/submissions/${submissionId}/answers`, status: 409 });
    await textAnswer(page).fill('went');
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

interface PilotTwoSeed {
  readonly maria: TestAccount;
  readonly anna: TestAccount;
  readonly boris: TestAccount;
}

let pilotTwoSeed: PilotTwoSeed | null = null;

function pilotTwo(): PilotTwoSeed {
  if (pilotTwoSeed === null) {
    throw new Error('the pilot 2 seed has not run');
  }
  return pilotTwoSeed;
}

const pilotTimezone = 'Europe/Moscow';

test.describe('pilot 2, group homework from the library', () => {
  test.describe.configure({ mode: 'serial' });
  test.use({ locale: 'ru-RU', timezoneId: pilotTimezone, colorScheme: 'light' });

  test('pilot-2 › seed Maria teaches Anna and Boris in B1 evening and has Past simple, week 1 in the library', async ({
    deployment,
    control,
  }) => {
    const maria = await signUpAndConfirmTutor(deployment, 'Мария');
    const anna = await signUpActiveStudentOnInvitation(deployment, control, maria, 'Анна', 'ru');
    const boris = await signUpStudentThroughCode(deployment, control, maria, 'Борис', 'ru');

    const groupId = await createGroup(deployment, maria, 'B1 evening', [anna.cardId, boris.cardId]);
    const templateId = await createPastSimpleTemplate(deployment, maria, { withMaterial: true });

    expect(groupId).not.toBe('');
    expect(templateId).not.toBe('');
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
    await dialog.getByLabel('Дата', { exact: true }).fill(localDate(deadline, pilotTimezone));
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
    page,
    deployment,
    control,
  }) => {
    const seed = pilotTwo();
    const submissionId = await onlySubmission(deployment, seed.anna);
    await page.setViewportSize({ width: 390, height: 844 });
    await signInThroughTheApi(page, seed.anna);
    await page.goto('/home');
    await page
      .getByRole('region', { name: 'Нужно сделать' })
      .getByRole('link', { name: `Начать ${pastSimple.title}` })
      .click();
    await page.getByRole('heading', { level: 1, name: pastSimple.title }).waitFor();

    await page.getByRole('group', { name: 'Вопрос 1' }).getByRole('radio', { name: 'went' }).check();
    await page.getByRole('group', { name: 'Вопрос 2' }).getByRole('textbox', { name: 'Ваш ответ' }).fill('went');
    await expect(page.getByRole('status').filter({ hasText: /^Сохранено$/ })).toBeVisible();
    await control.advance(1);
    const chooser = page.waitForEvent('filechooser');
    await page.getByRole('group', { name: 'Вопрос 3' }).getByRole('button', { name: 'Сфотографировать' }).click();
    await (await chooser).setFiles(pictureFile);
    await expect(
      page.getByRole('group', { name: 'Вопрос 3' }).getByRole('button', { name: `Удалить ${pictureFile.name}` }),
    ).toBeVisible();
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
    const stored = await storedAnswers(deployment, seed.anna, submissionId);
    expect(stored.status).toBe('submitted');
    expect(stored.files).toEqual([pictureFile.name]);
  });

  test('pilot-2 › steps 8–9 after the deadline Boris submits goed, Went and a PDF, and his work is marked late without points', async ({
    page,
    deployment,
    control,
  }) => {
    const seed = pilotTwo();
    await control.advance(3 * 86_400);
    const submissionId = await onlySubmission(deployment, seed.boris);
    await signInThroughTheApi(page, seed.boris);
    await page.goto(`/homework/${submissionId}`);
    await page.getByRole('heading', { level: 1, name: pastSimple.title }).waitFor();

    await page.getByRole('group', { name: 'Вопрос 1' }).getByRole('radio', { name: 'goed' }).check();
    await page.getByRole('group', { name: 'Вопрос 2' }).getByRole('textbox', { name: 'Ваш ответ' }).fill('Went ');
    await expect(page.getByRole('status').filter({ hasText: /^Сохранено$/ })).toBeVisible();
    await control.advance(1);
    const chooser = page.waitForEvent('filechooser');
    await page.getByRole('group', { name: 'Вопрос 3' }).getByRole('button', { name: 'Прикрепить файлы' }).click();
    await (await chooser).setFiles(pdfFile);
    await expect(
      page.getByRole('group', { name: 'Вопрос 3' }).getByRole('button', { name: `Удалить ${pdfFile.name}` }),
    ).toBeVisible();
    await expect(page.getByRole('status').filter({ hasText: /^Сохранено$/ })).toBeVisible();
    await control.advance(1);
    await page.getByRole('button', { name: 'Сдать', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Сдать работу?' })
      .getByRole('button', { name: 'Сдать', exact: true })
      .click();

    await expect(page.getByRole('status').filter({ hasText: /^Сдано$/ })).toBeFocused();
    await expect(page.locator('main')).toContainText('Сдано с опозданием');
    expect(await page.locator('main').innerText()).not.toMatch(/балл/);
    const stored = await storedAnswers(deployment, seed.boris, submissionId);
    expect(stored.late).toBe(true);
    expect(stored.status).toBe('submitted');
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
    await expect(saved).toBeVisible();
    await control.advance(1);
    await fileQuestion.getByRole('spinbutton', { name: 'Баллы' }).fill('6');
    await expect(saved).toBeVisible();
    await control.advance(1);
    await page
      .getByRole('textbox', { name: 'Общий отзыв' })
      .fill('**Great work.** Check the word order in sentence 2.');
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
});
