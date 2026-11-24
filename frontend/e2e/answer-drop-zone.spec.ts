import { randomUUID } from 'node:crypto';

import type { Locator, Page } from '@playwright/test';

import { type TestAccount, signInThroughTheApi } from './accounts.ts';
import { onlySubmission, pictureFile, storedAnswers } from './assignment.ts';
import type { Deployment } from './deployment.ts';
import { diagnosedTest as test, expect, seedVerifiedUser } from './fixtures.ts';
import { issueThroughTheApi } from './homework.ts';
import { scoreFormsIn } from './results.ts';
import { installPageClock, skipVisibleSaveDelay } from './save-delay.ts';
import { accessToken, callApi, seedActiveStudent } from './tutoring.ts';

const title = 'Reading, unit 7';
const choicePoints = 37;
const essayPoints = 41;

interface IssuedWork {
  readonly tutor: TestAccount;
  readonly student: TestAccount;
  readonly submissionId: string;
}

async function issueMixedTask(deployment: Deployment, fileKinds: readonly string[]): Promise<IssuedWork> {
  const tutor = await seedVerifiedUser(deployment, 'Maria', 'en');
  const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Anna');
  const created = await callApi(deployment, await accessToken(deployment, tutor), 'POST', '/library/templates', {
    title,
    folder_id: null,
    blocks: [
      { id: randomUUID(), type: 'instruction', text: 'Read the text and answer the questions.' },
      {
        id: randomUUID(),
        type: 'single_choice',
        prompt: 'What is the main idea?',
        points: choicePoints,
        options: [
          { id: randomUUID(), text: 'Cities grow', correct: false },
          { id: randomUUID(), text: 'Transport helps', correct: true },
        ],
      },
      {
        id: randomUUID(),
        type: 'long_text',
        prompt: 'Explain why the author disagrees',
        points: essayPoints,
        max_length: 5000,
      },
      {
        id: randomUUID(),
        type: 'file',
        prompt: 'Photograph exercise 6',
        points: 6,
        allowed_kinds: [...fileKinds],
        max_files: 2,
      },
    ],
  });
  expect(created.status).toBe(201);
  const body: unknown = created.body;
  const templateId =
    typeof body === 'object' && body !== null && 'id' in body && typeof body.id === 'string' ? body.id : '';
  await issueThroughTheApi(deployment, tutor, templateId, { relationship_id: cardId });
  return { tutor, student, submissionId: await onlySubmission(deployment, student) };
}

async function openWork(page: Page, work: IssuedWork): Promise<void> {
  await installPageClock(page, { pageClock: true });
  await signInThroughTheApi(page, work.student);
  await page.goto(`/homework/${work.submissionId}`);
  await page.getByRole('heading', { level: 1, name: title }).waitFor();
}

function question(page: Page, number: number): Locator {
  return page.getByRole('group', { name: new RegExp(`^Question ${String(number)} · `) });
}

function dropZone(page: Page, number: number): Locator {
  return question(page, number).getByRole('button', { name: 'Choose files', exact: true });
}

async function dropPicture(zone: Locator, name: string): Promise<void> {
  await zone.evaluate(
    (element, { bytes, fileName }) => {
      const transfer = new DataTransfer();
      transfer.items.add(new File([new Uint8Array(bytes)], fileName, { type: 'image/png' }));
      element.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: transfer }));
    },
    { bytes: [...pictureFile.buffer], fileName: name },
  );
}

async function reviewQueue(deployment: Deployment, tutor: TestAccount): Promise<unknown> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'GET', '/review-queue');
  return answer.body;
}

test.describe('the answer drop zone', () => {
  test('a student answers the mixed task’s text and file questions through the drop zone and submits, and the tutor’s review queue lists it', async ({
    page,
    deployment,
    control,
  }) => {
    const work = await issueMixedTask(deployment, ['images']);
    await openWork(page, work);
    await question(page, 1).getByRole('radio', { name: 'Transport helps' }).check();
    await question(page, 2).getByRole('textbox', { name: 'Your answer' }).fill('The author thinks cars matter.');

    await dropPicture(dropZone(page, 3), 'exercise 6.png');
    await question(page, 3).getByRole('button', { name: 'Remove exercise 6.png' }).waitFor();
    await skipVisibleSaveDelay(page);
    await page
      .getByRole('status')
      .filter({ hasText: /^Saved$/ })
      .waitFor();
    const saved = await storedAnswers(deployment, work.student, work.submissionId);
    await control.advance(1);
    await page.getByRole('button', { name: 'Submit', exact: true }).click();
    await page
      .getByRole('dialog', { name: 'Submit your work?' })
      .getByRole('button', { name: 'Submit', exact: true })
      .click();
    await page
      .getByRole('status')
      .filter({ hasText: /^Submitted$/ })
      .waitFor();
    const queue = JSON.stringify(await reviewQueue(deployment, work.tutor));

    expect(saved.texts).toEqual(['The author thinks cars matter.']);
    expect(saved.files).toEqual(['exercise 6.png']);
    expect(queue).toContain(work.submissionId);
  });

  test('a file of a kind outside the question’s kinds dropped on the drop zone shows the inline refusal, which takes focus', async ({
    page,
    deployment,
  }) => {
    const work = await issueMixedTask(deployment, ['pdf']);
    await openWork(page, work);
    const uploads: string[] = [];
    page.on('request', (request) => {
      if (new URL(request.url()).pathname === '/api/v1/uploads') {
        uploads.push(request.method());
      }
    });

    await dropPicture(dropZone(page, 3), 'exercise 6.png');

    await expect(question(page, 3).getByRole('alert')).toHaveText('This file type is not allowed here');
    await expect(question(page, 3).getByRole('alert')).toBeFocused();
    expect(uploads).toEqual([]);
  });

  test('each question is marked as checked automatically or by the tutor, with no points', async ({
    page,
    deployment,
  }) => {
    const work = await issueMixedTask(deployment, ['images']);

    await openWork(page, work);
    const groups = await page
      .locator('fieldset[data-question] > legend')
      .evaluateAll((elements) => elements.map((element) => element.textContent));
    const text = await page.getByRole('main').innerText();

    expect(groups).toEqual([
      'Question 1 · Checked automatically',
      'Question 2 · Your tutor checks it',
      'Question 3 · Your tutor checks it',
    ]);
    expect(text).not.toMatch(new RegExp(`\\b(${String(choicePoints)}|${String(essayPoints)})\\b`));
    expect(scoreFormsIn(text, 'en')).toEqual([]);
    expect(text.toLowerCase()).not.toContain('point');
  });
});

test.describe('file answers on a phone', () => {
  test.use({ viewport: { width: 360, height: 780 } });

  test('Remove on the only file moves focus to Attach files', async ({ page, deployment }) => {
    const work = await issueMixedTask(deployment, ['images']);
    await openWork(page, work);
    const chooser = page.waitForEvent('filechooser');
    await question(page, 3).getByRole('button', { name: 'Attach files', exact: true }).click();
    await (await chooser).setFiles({ ...pictureFile, name: 'exercise 6.png' });
    const remove = question(page, 3).getByRole('button', { name: 'Remove exercise 6.png' });
    await remove.waitFor();

    await remove.click();

    await expect(question(page, 3).getByRole('button', { name: 'Attach files', exact: true })).toBeFocused();
    await expect(question(page, 3).getByRole('button', { name: 'Choose files', exact: true })).toHaveCount(0);
  });
});
