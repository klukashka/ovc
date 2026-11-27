import { randomUUID } from 'node:crypto';

import type { Locator, Page } from '@playwright/test';

import { type TestAccount, newTestAccount, registerTutor, signInThroughTheApi } from './accounts.ts';
import { pictureFile } from './assignment.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { accessToken, callApi, endRelationship, seedActiveStudent } from './tutoring.ts';

interface UploadFile {
  readonly name: string;
  readonly mimeType: string;
  readonly buffer: Buffer;
}

const pageOne: UploadFile = { ...pictureFile, name: 'Page one.png' };
const pageTwo: UploadFile = { ...pictureFile, name: 'Page two.png' };
const keySentinel = 'quintessentially';
const optionSentinel = 'Option sentinel 4be1';
const automaticPoints = 37;
const manualPoints = 23;

function field(value: unknown, name: string): unknown {
  if (typeof value !== 'object' || value === null || !(name in value)) {
    throw new Error(`the answer has no ${name}`);
  }
  return Object.entries(value).find(([key]) => key === name)?.[1];
}

function textField(value: unknown, name: string): string {
  const found = field(value, name);
  if (typeof found !== 'string') {
    throw new Error(`${name} is not a string`);
  }
  return found;
}

function listField(value: unknown, name: string): readonly unknown[] {
  const found = field(value, name);
  if (!Array.isArray(found)) {
    throw new Error(`${name} is not a list`);
  }
  return found;
}

async function tutorGet(deployment: Deployment, tutor: TestAccount, path: string): Promise<unknown> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'GET', path);
  if (answer.status !== 200) {
    throw new Error(`GET ${path} answered ${String(answer.status)}`);
  }
  return answer.body;
}

function main(page: Page): Locator {
  return page.getByRole('main');
}

async function openNewTask(page: Page, tutor: TestAccount): Promise<void> {
  await signInThroughTheApi(page, tutor);
  await page.goto('/homework/library');
  await page.getByRole('heading', { level: 1, name: 'Library' }).waitFor();
  await main(page).getByRole('button', { name: 'New task', exact: true }).click();
  await page.getByRole('heading', { level: 1, name: 'New task' }).waitFor();
}

async function writeTask(page: Page, title: string): Promise<void> {
  await main(page).getByLabel('Title', { exact: true }).fill(title);
  await main(page).getByLabel('What to do', { exact: true }).fill('Read the pages and answer the questions.');
}

async function attach(page: Page, files: readonly UploadFile[]): Promise<void> {
  const chooser = page.waitForEvent('filechooser');
  await main(page)
    .getByRole('button', { name: /^Choose files/ })
    .click();
  await (await chooser).setFiles([...files]);
}

async function dropFileOn(target: Locator, file: UploadFile): Promise<boolean> {
  return target.evaluate(
    (element, { bytes, fileName, mimeType }) => {
      const transfer = new DataTransfer();
      transfer.items.add(new File([new Uint8Array(bytes)], fileName, { type: mimeType }));
      const drop = new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: transfer });
      element.dispatchEvent(drop);
      return drop.defaultPrevented;
    },
    { bytes: [...file.buffer], fileName: file.name, mimeType: file.mimeType },
  );
}

function attachmentTile(page: Page, name: string): Locator {
  return main(page).getByRole('list', { name: 'Attached files' }).getByRole('listitem', { name, exact: true });
}

function question(page: Page, number: number): Locator {
  return main(page)
    .getByRole('listitem')
    .filter({ has: page.getByRole('heading', { level: 3, name: new RegExp(`^Question ${String(number)} ·`) }) });
}

async function addQuestion(page: Page, type: string): Promise<Locator> {
  await main(page).getByLabel('Question type', { exact: true }).selectOption({ label: type });
  await main(page).getByRole('button', { name: 'Add question', exact: true }).click();
  const count = await main(page).getByRole('heading', { level: 3 }).count();
  return question(page, count);
}

async function setPoints(card: Locator, points: number): Promise<void> {
  await card.getByLabel('Points', { exact: true }).fill(String(points));
}

async function addSingleChoice(page: Page): Promise<void> {
  const card = await addQuestion(page, 'Single choice');
  await card.getByLabel('Question', { exact: true }).fill('Which word fits?');
  await card.getByLabel('Option 1', { exact: true }).fill(optionSentinel);
  await card.getByLabel('Option 2', { exact: true }).fill('Another option');
  await card.getByRole('radio', { name: 'Option 1 is correct' }).check();
  await setPoints(card, automaticPoints);
}

async function addShortText(page: Page): Promise<void> {
  const card = await addQuestion(page, 'Short answer');
  await card.getByLabel('Question', { exact: true }).fill('A synonym of “very”');
  await card.getByLabel('Accepted answers', { exact: true }).fill(keySentinel);
  await setPoints(card, automaticPoints);
}

async function addManual(page: Page, type: 'Text answer' | 'File or photo', prompt: string): Promise<void> {
  const card = await addQuestion(page, type);
  await card.getByLabel('Question', { exact: true }).fill(prompt);
  await setPoints(card, manualPoints);
}

async function chooseStudent(page: Page, name: string): Promise<void> {
  await main(page).getByRole('button', { name, exact: true }).click();
}

function policyToggle(page: Page): Locator {
  return main(page).getByRole('button', { name: /^Answer visibility:/ });
}

async function choosePolicy(page: Page, label: 'Never' | 'After submission' | 'After checking'): Promise<void> {
  if ((await policyToggle(page).getAttribute('aria-expanded')) !== 'true') {
    await policyToggle(page).click();
  }
  await main(page).getByRole('radio', { name: label }).click();
}

async function issue(page: Page): Promise<void> {
  await main(page).getByRole('button', { name: 'Issue', exact: true }).click();
}

function issuedNotice(page: Page): Locator {
  return page.getByText('Assignment issued.', { exact: true });
}

function errorSummary(page: Page): Locator {
  return main(page).getByRole('alert').filter({ hasText: 'Check these fields' });
}

async function descriptionOf(locator: Locator): Promise<string> {
  return locator.evaluate((element) =>
    (element.getAttribute('aria-describedby') ?? '')
      .split(' ')
      .filter((id) => id !== '')
      .map((id) => document.getElementById(id)?.textContent ?? '')
      .join(' '),
  );
}

async function addMixedQuestions(page: Page): Promise<void> {
  await addSingleChoice(page);
  await addShortText(page);
  await addManual(page, 'Text answer', 'Explain your choice');
}

interface AnswerInput {
  readonly question_id: string;
  readonly value: { readonly option_ids: readonly string[] } | { readonly text: string };
}

async function onlyStudentSubmission(deployment: Deployment, student: TestAccount): Promise<string> {
  const answer = await callApi(deployment, await accessToken(deployment, student), 'GET', '/assignments?status=todo');
  const [first] = listField(answer.body, 'items');
  return textField(first, 'submission_id');
}

async function answerAndSubmit(deployment: Deployment, student: TestAccount, submissionId: string): Promise<string[]> {
  const token = await accessToken(deployment, student);
  const read = await callApi(deployment, token, 'GET', `/submissions/${submissionId}`);
  const blocks = listField(read.body, 'blocks');
  const answers = blocks.flatMap((block): AnswerInput[] => {
    const type = textField(block, 'type');
    if (type === 'single_choice') {
      const option = listField(block, 'options').find((entry) => textField(entry, 'text') === optionSentinel);
      return [{ question_id: textField(block, 'id'), value: { option_ids: [textField(option, 'id')] } }];
    }
    if (type === 'short_text') {
      return [{ question_id: textField(block, 'id'), value: { text: 'very' } }];
    }
    if (type === 'long_text') {
      return [{ question_id: textField(block, 'id'), value: { text: 'My essay.' } }];
    }
    return [];
  });
  const saved = await callApi(deployment, token, 'PUT', `/submissions/${submissionId}/answers`, {
    version: field(read.body, 'version'),
    save_id: randomUUID(),
    answers,
  });
  expect(saved.status).toBe(200);
  const before = await callApi(deployment, token, 'GET', `/submissions/${submissionId}`);
  const submitted = await callApi(deployment, token, 'POST', `/submissions/${submissionId}/submit`, {
    version: field(before.body, 'version'),
  });
  expect(submitted.status).toBe(200);
  const after = await callApi(deployment, token, 'GET', `/submissions/${submissionId}`);
  const results = await callApi(deployment, token, 'GET', '/assignments?status=submitted');
  const checked = await callApi(deployment, token, 'GET', '/assignments?status=checked');
  expect([after.status, results.status, checked.status]).toEqual([200, 200, 200]);
  const listed = [...listField(results.body, 'items'), ...listField(checked.body, 'items')];
  expect(listed.map((item) => textField(item, 'submission_id'))).toEqual([submissionId]);
  return [submitted.body, after.body, results.body, checked.body].map((body) => JSON.stringify(body));
}

function expectNoKeysOrPoints(texts: readonly string[]): void {
  for (const text of texts) {
    expect(text).not.toContain(keySentinel);
    expect(text).not.toMatch(new RegExp(`:${String(automaticPoints)}[,}\\]]`));
    expect(text).not.toMatch(new RegExp(`:${String(manualPoints)}[,}\\]]`));
    for (const key of [
      '"revealed"',
      '"correct"',
      '"correct_option_ids"',
      '"correct_value"',
      '"auto_correct"',
      '"override_points"',
      '"accepted_answers"',
      '"explanation"',
      '"points"',
      '"auto_points"',
      '"max_points"',
      '"score"',
    ]) {
      expect(text).not.toContain(key);
    }
  }
}

async function tutorAutoPoints(
  deployment: Deployment,
  tutor: TestAccount,
  submissionId: string,
): Promise<Record<string, number>> {
  const submission = await tutorGet(deployment, tutor, `/submissions/${submissionId}`);
  const types = new Map(
    listField(submission, 'blocks').map((block) => [textField(block, 'id'), textField(block, 'type')]),
  );
  return Object.fromEntries(
    listField(submission, 'answers').flatMap((answer) => {
      const points = field(answer, 'auto_points');
      return typeof points === 'number' ? [[types.get(textField(answer, 'question_id')) ?? 'unknown', points]] : [];
    }),
  );
}

async function makeSavedTaskAllAutomatic(deployment: Deployment, tutor: TestAccount, title: string): Promise<void> {
  const root = listField(await tutorGet(deployment, tutor, '/library'), 'items');
  const templateId = textField(
    root.find((item) => textField(item, 'title') === title),
    'id',
  );
  const template = await tutorGet(deployment, tutor, `/library/templates/${templateId}`);
  const saved = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'PUT',
    `/library/templates/${templateId}`,
    {
      title,
      version: field(template, 'version'),
      save_id: randomUUID(),
      blocks: [
        { id: randomUUID(), type: 'instruction', text: 'Read the pages and answer the questions.' },
        { id: randomUUID(), type: 'short_text', prompt: 'A synonym of “very”', points: 2, accepted_answers: ['quite'] },
      ],
    },
  );
  if (saved.status !== 200 || field(saved.body, 'ready') !== true) {
    throw new Error(`saving the template answered ${String(saved.status)}`);
  }
}

async function reviewQueueSubmissions(deployment: Deployment, tutor: TestAccount): Promise<string[]> {
  const queue = await tutorGet(deployment, tutor, '/review-queue');
  return listField(queue, 'items').map((item) => textField(item, 'submission_id'));
}

test.describe('new task', () => {
  const afterSubmissionNote =
    'Students see the correct answers as soon as they submit, so work cannot be returned for revision.';
  const noManualHint =
    'This assignment has no questions you check by hand, so students see the answers right after submitting.';

  test('a mixed task with two pictures is issued, auto part scored and work queued', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    await openNewTask(page, tutor);
    await writeTask(page, 'Reading, unit 7');
    await attach(page, [pageOne, pageTwo]);
    await main(page)
      .getByRole('progressbar', { name: `Uploaded ${pageOne.name}` })
      .waitFor();
    await main(page)
      .getByRole('progressbar', { name: `Uploaded ${pageTwo.name}` })
      .waitFor();
    await addSingleChoice(page);
    await addShortText(page);
    await addManual(page, 'Text answer', 'Explain why the author disagrees');
    await addManual(page, 'File or photo', 'Photograph exercise 6');
    await chooseStudent(page, 'Anna');
    await main(page)
      .getByRole('checkbox', { name: /Set a deadline/ })
      .check();

    const shownPolicy = await policyToggle(page).textContent();
    const shownCounts = await main(page)
      .getByText(/^4 · automatically/)
      .textContent();
    await issue(page);
    await issuedNotice(page).waitFor();
    const submissionId = await onlyStudentSubmission(deployment, student);
    const studentBodies = await answerAndSubmit(deployment, student, submissionId);

    expect(shownPolicy).toBe('Answer visibility: After checking');
    expect(shownCounts).toBe('4 · automatically 2 · you check 2');
    await expect(issuedNotice(page)).toBeFocused();
    const folders = listField(await tutorGet(deployment, tutor, '/library/folders'), 'folders');
    const folder = folders.find((entry) => textField(entry, 'name') === 'Task attachments');
    const materials = listField(
      await tutorGet(deployment, tutor, `/library?folder=${textField(folder, 'id')}`),
      'items',
    );
    expect(materials.map((item) => textField(item, 'title')).sort()).toEqual(['Page one', 'Page two']);
    const root = listField(await tutorGet(deployment, tutor, '/library'), 'items');
    const template = root.find((item) => textField(item, 'title') === 'Reading, unit 7');
    expect(field(template, 'ready')).toBe(true);
    const issues = listField(await tutorGet(deployment, tutor, `/issues?status=active&student=${cardId}`), 'items');
    expect(issues.map((item) => [field(item, 'answer_policy'), field(item, 'deadline_at') !== null])).toEqual([
      ['after_check', true],
    ]);
    expectNoKeysOrPoints(studentBodies);
    expect(await tutorAutoPoints(deployment, tutor, submissionId)).toEqual({
      single_choice: automaticPoints,
      short_text: 0,
    });
    expect(await reviewQueueSubmissions(deployment, tutor)).toEqual([submissionId]);
  });

  test('two attached files show as tiles named by their files, each with a large enough Remove and no picture of the file', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await openNewTask(page, tutor);

    await attach(page, [pageOne, pageTwo]);
    await main(page)
      .getByRole('progressbar', { name: `Uploaded ${pageTwo.name}` })
      .waitFor();

    for (const file of [pageOne, pageTwo]) {
      const tile = attachmentTile(page, file.name);
      await expect(tile).toBeVisible();
      const remove = tile.getByRole('button', { name: `Remove ${file.name}`, exact: true });
      const box = await remove.boundingBox();
      expect(box?.width ?? 0).toBeGreaterThanOrEqual(24);
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(24);
      await expect(tile.locator('img')).toHaveCount(0);
    }
    await expect(main(page).locator('img[src^="blob:"]')).toHaveCount(0);
  });

  test('a file dropped outside the drop zone is held by the page, which keeps its typed title and attaches nothing', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await openNewTask(page, tutor);
    await writeTask(page, 'Kept title');
    const address = page.url();

    const held = await dropFileOn(page.getByRole('heading', { level: 1, name: 'New task' }), pageOne);

    expect(held).toBe(true);
    expect(page.url()).toBe(address);
    await expect(page.getByRole('heading', { level: 1, name: 'New task' })).toBeVisible();
    await expect(main(page).getByLabel('Title', { exact: true })).toHaveValue('Kept title');
    await expect(main(page).getByRole('list', { name: 'Attached files' })).toHaveCount(0);
  });

  test('a question without a prompt is refused inline before issuing', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await seedActiveStudent(deployment, tutor, 'Anna');
    await openNewTask(page, tutor);
    await writeTask(page, 'Unfinished');
    await addQuestion(page, 'Text answer');
    await chooseStudent(page, 'Anna');
    const saves: string[] = [];
    page.on('request', (request) => {
      if (new URL(request.url()).pathname === '/api/v1/library/templates') {
        saves.push(request.method());
      }
    });

    await issue(page);

    await expect(errorSummary(page)).toBeFocused();
    await expect(question(page, 1).getByText('Question 1: write the question.', { exact: true })).toBeVisible();
    await expect(question(page, 1).getByLabel('Question', { exact: true })).toHaveAttribute('aria-invalid', 'true');
    expect(saves).toEqual([]);
  });

  test('choosing After submission for a mixed task shows the reveal warning', async ({ page, deployment }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await openNewTask(page, tutor);
    await writeTask(page, 'Mixed');
    await addMixedQuestions(page);

    await choosePolicy(page, 'After submission');

    await expect(main(page).getByRole('radio', { name: 'After submission' })).toHaveAccessibleDescription(
      afterSubmissionNote,
    );
  });

  test('removing the last manual question under After checking shows the warning and focuses the previous question', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await openNewTask(page, tutor);
    await writeTask(page, 'Mixed');
    await addMixedQuestions(page);
    await choosePolicy(page, 'After checking');

    await question(page, 3).getByRole('button', { name: 'Remove question 3', exact: true }).click();

    await expect(main(page).getByRole('heading', { level: 3, name: /^Question 2 ·/ })).toBeFocused();
    await expect(main(page).getByRole('radio', { name: 'After checking' })).toHaveAccessibleDescription(noManualHint);
  });

  test('mixed work issued after checking reveals no key, explanation or points while it waits for checking', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student } = await seedActiveStudent(deployment, tutor, 'Anna');
    await openNewTask(page, tutor);
    await writeTask(page, 'Mixed');
    await addMixedQuestions(page);
    await chooseStudent(page, 'Anna');

    await issue(page);
    await issuedNotice(page).waitFor();
    const submissionId = await onlyStudentSubmission(deployment, student);
    const studentBodies = await answerAndSubmit(deployment, student, submissionId);

    await expect(issuedNotice(page)).toBeFocused();
    expectNoKeysOrPoints(studentBodies);
    expect(await reviewQueueSubmissions(deployment, tutor)).toEqual([submissionId]);
  });

  test('an all-automatic task defaults to Never and warns on the policies that reveal answers', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    await openNewTask(page, tutor);
    await writeTask(page, 'Automatic');
    await addSingleChoice(page);
    await addShortText(page);

    const shownDefault = await policyToggle(page).textContent();
    await choosePolicy(page, 'After checking');
    const afterChecking = await descriptionOf(main(page).getByRole('radio', { name: 'After checking' }));
    await choosePolicy(page, 'After submission');
    const afterSubmission = await descriptionOf(main(page).getByRole('radio', { name: 'After submission' }));

    expect(shownDefault).toBe('Answer visibility: Never');
    expect(afterChecking).toBe(noManualHint);
    expect(afterSubmission).toBe(afterSubmissionNote);
  });

  test('an all-automatic task issued under Never is scored at submission and reveals nothing', async ({
    page,
    deployment,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { student } = await seedActiveStudent(deployment, tutor, 'Anna');
    await openNewTask(page, tutor);
    await writeTask(page, 'Automatic');
    await addSingleChoice(page);
    await addShortText(page);
    await chooseStudent(page, 'Anna');

    await issue(page);
    await issuedNotice(page).waitFor();
    const submissionId = await onlyStudentSubmission(deployment, student);
    const studentBodies = await answerAndSubmit(deployment, student, submissionId);

    await expect(issuedNotice(page)).toBeFocused();
    expectNoKeysOrPoints(studentBodies);
    expect(await tutorAutoPoints(deployment, tutor, submissionId)).toEqual({
      single_choice: automaticPoints,
      short_text: 0,
    });
    expect(await reviewQueueSubmissions(deployment, tutor)).toEqual([]);
  });

  test('an upload over the library quota is refused inline, keeps the uploaded material listed and issues nothing', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    await openNewTask(page, tutor);
    await writeTask(page, 'Quota');
    await attach(page, [pageOne]);
    await main(page)
      .getByRole('progressbar', { name: `Uploaded ${pageOne.name}` })
      .waitFor();
    deployment.administer(['tutors', 'set-quota', tutor.email, '--files', '1', '--recordings', '1']);
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/uploads?context=library', status: 507 });
    await chooseStudent(page, 'Anna');
    const refused = attachmentTile(page, pageTwo.name);

    await attach(page, [pageTwo]);
    await refused.getByRole('alert').waitFor();
    await issue(page);

    await expect(refused.getByRole('alert')).toHaveText('The file storage is full.');
    await expect(main(page).getByRole('progressbar', { name: `Uploaded ${pageOne.name}` })).toBeVisible();
    await expect(errorSummary(page)).toBeFocused();
    expect(listField(await tutorGet(deployment, tutor, `/issues?status=active&student=${cardId}`), 'items')).toEqual(
      [],
    );
  });

  test('Issue again on a saved task whose copy lost its last hand-checked question is held back with an alert that takes focus', async ({
    page,
    deployment,
    diagnostics,
  }) => {
    const tutor = await seedVerifiedUser(deployment, 'Pavel', 'en');
    const { cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
    await openNewTask(page, tutor);
    await writeTask(page, 'Held back');
    await addSingleChoice(page);
    await addManual(page, 'Text answer', 'Explain your choice');
    await chooseStudent(page, 'Anna');
    await choosePolicy(page, 'After checking');
    await endRelationship(deployment, tutor, cardId);
    diagnostics.expectFailure({ method: 'POST', path: '/api/v1/issues', status: 409 });
    await issue(page);
    await main(page).getByRole('alert').filter({ hasText: 'This student can no longer get assignments.' }).waitFor();
    await makeSavedTaskAllAutomatic(deployment, tutor, 'Held back');
    const issueRequests: string[] = [];
    page.on('request', (request) => {
      if (new URL(request.url()).pathname === '/api/v1/issues' && request.method() === 'POST') {
        issueRequests.push(request.url());
      }
    });

    await issue(page);

    const heldBack = main(page).getByRole('alert').filter({
      hasText: 'The saved task now gives answer visibility “After checking”. Check it and press Issue again.',
    });
    await expect(heldBack).toContainText(noManualHint);
    await expect(heldBack).toBeFocused();
    expect(issueRequests).toEqual([]);
  });

  test('an unconfirmed tutor sees the confirmation notice instead of the page', async ({ page, deployment }) => {
    const account = newTestAccount('Unconfirmed tutor');
    await registerTutor(deployment, account, 'en');
    await signInThroughTheApi(page, account);

    await page.goto('/homework/new-task');

    await expect(page.getByRole('heading', { level: 1, name: 'New task' })).toBeVisible();
    await expect(page.getByText('Confirm your email to issue and check assignments.')).toBeVisible();
    await expect(main(page).getByLabel('Title', { exact: true })).toHaveCount(0);
  });
});
