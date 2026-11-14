import { randomUUID } from 'node:crypto';

import type { TestAccount } from './accounts.ts';
import { questionIds, storedAnswers, submitThroughTheApi } from './assignment.ts';
import type { Deployment } from './deployment.ts';
import { accessToken, callApi } from './tutoring.ts';

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

function numberField(value: unknown, name: string): number {
  const found = field(value, name);
  if (typeof found !== 'number') {
    throw new Error(`${name} is not a number`);
  }
  return found;
}

function optionalNumber(value: unknown, name: string): number | null {
  const found = field(value, name);
  return typeof found === 'number' ? found : null;
}

function listField(value: unknown, name: string): readonly unknown[] {
  const found = field(value, name);
  if (!Array.isArray(found)) {
    throw new Error(`${name} is not a list`);
  }
  return found;
}

export interface StoredReviewItem {
  readonly questionId: string;
  readonly points: number | null;
  readonly overridePoints: number | null;
  readonly overrideReason: string | null;
  readonly comment: string;
  readonly correctedFiles: readonly string[];
}

export interface StoredRevision {
  readonly revision: number;
  readonly state: string;
  readonly version: number;
  readonly feedback: string;
  readonly items: readonly StoredReviewItem[];
}

export interface StoredReview {
  readonly status: string;
  readonly revisions: readonly StoredRevision[];
}

export async function storedReview(
  deployment: Deployment,
  tutor: TestAccount,
  submissionId: string,
): Promise<StoredReview> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'GET', `/submissions/${submissionId}`);
  if (answer.status !== 200) {
    throw new Error(`reading the tutor submission answered ${String(answer.status)}`);
  }
  return {
    status: textField(answer.body, 'status'),
    revisions: listField(answer.body, 'revisions').map((revision) => ({
      revision: numberField(revision, 'revision'),
      state: textField(revision, 'state'),
      version: numberField(revision, 'version'),
      feedback: textField(revision, 'feedback'),
      items: listField(revision, 'items').map((item) => ({
        questionId: textField(item, 'question_id'),
        points: optionalNumber(item, 'points'),
        overridePoints: optionalNumber(item, 'override_points'),
        overrideReason: typeof field(item, 'override_reason') === 'string' ? textField(item, 'override_reason') : null,
        comment: textField(item, 'comment'),
        correctedFiles: listField(item, 'corrected_files').map((file) => textField(file, 'filename')),
      })),
    })),
  };
}

export function draftOf(review: StoredReview): StoredRevision | undefined {
  return review.revisions.find((revision) => revision.state === 'draft');
}

export async function saveDraftFromAnotherTab(
  deployment: Deployment,
  tutor: TestAccount,
  submissionId: string,
  feedback: string,
): Promise<number> {
  const review = await storedReview(deployment, tutor, submissionId);
  const draft = draftOf(review);
  const published = Math.max(
    0,
    ...review.revisions.filter((revision) => revision.state === 'published').map((revision) => revision.revision),
  );
  const revision = draft?.revision ?? published + 1;
  const answer = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'PUT',
    `/submissions/${submissionId}/review`,
    { revision, version: draft?.version ?? 0, save_id: randomUUID(), feedback, items: [] },
  );
  if (answer.status !== 200) {
    throw new Error(`saving a draft from another tab answered ${String(answer.status)}`);
  }
  return revision;
}

export async function returnFromAnotherTab(
  deployment: Deployment,
  tutor: TestAccount,
  submissionId: string,
): Promise<void> {
  const revision = await saveDraftFromAnotherTab(deployment, tutor, submissionId, 'Please redo the file question.');
  const answer = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'POST',
    `/submissions/${submissionId}/return`,
    { revision },
  );
  if (answer.status !== 200) {
    throw new Error(`returning from another tab answered ${String(answer.status)}`);
  }
}

export async function answerTextAndSubmit(
  deployment: Deployment,
  student: TestAccount,
  submissionId: string,
  text: string,
): Promise<void> {
  const [, textQuestion] = await questionIds(deployment, student, submissionId);
  const stored = await storedAnswers(deployment, student, submissionId);
  const saved = await callApi(
    deployment,
    await accessToken(deployment, student),
    'PUT',
    `/submissions/${submissionId}/answers`,
    { version: stored.version, save_id: randomUUID(), answers: [{ question_id: textQuestion, value: { text } }] },
  );
  if (saved.status !== 200) {
    throw new Error(`saving the answer answered ${String(saved.status)}`);
  }
  await submitThroughTheApi(deployment, student, submissionId);
}

export async function studentSubmissionText(
  deployment: Deployment,
  student: TestAccount,
  submissionId: string,
): Promise<string> {
  const answer = await callApi(
    deployment,
    await accessToken(deployment, student),
    'GET',
    `/submissions/${submissionId}`,
  );
  if (answer.status !== 200) {
    throw new Error(`reading the student submission answered ${String(answer.status)}`);
  }
  return JSON.stringify(answer.body);
}
