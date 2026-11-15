import { randomUUID } from 'node:crypto';

import { expect } from '@playwright/test';

import { type TestAccount, testNetAddress } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { storedReview } from './review.ts';
import { accessToken, callApi } from './tutoring.ts';

export interface UploadFile {
  readonly name: string;
  readonly mimeType: string;
  readonly buffer: Buffer;
}

export interface ReviewItemSeed {
  readonly questionId: string;
  readonly points?: number;
  readonly comment?: string;
  readonly correctedIds?: readonly string[];
}

function idOf(value: unknown): string {
  if (typeof value !== 'object' || value === null || !('id' in value) || typeof value.id !== 'string') {
    throw new Error('the answer has no id');
  }
  return value.id;
}

export async function uploadCorrectedFile(
  deployment: Deployment,
  tutor: TestAccount,
  submissionId: string,
  questionId: string,
  file: UploadFile,
): Promise<string> {
  const body = new FormData();
  body.append('file', new Blob([new Uint8Array(file.buffer)], { type: file.mimeType }), file.name);
  const scope = new URLSearchParams({ context: 'corrected', scope: `submission:${submissionId}:${questionId}` });
  const uploaded = await fetch(`${deployment.apiUrl}/api/v1/uploads?${scope.toString()}`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: {
      Origin: deployment.origin,
      'X-Forwarded-For': testNetAddress(),
      Authorization: `Bearer ${await accessToken(deployment, tutor)}`,
    },
    body,
  });
  if (uploaded.status !== 201) {
    throw new Error(`uploading a corrected file answered ${String(uploaded.status)}`);
  }
  return idOf(await uploaded.json());
}

export async function saveReviewDraft(
  deployment: Deployment,
  tutor: TestAccount,
  submissionId: string,
  feedback: string,
  items: readonly ReviewItemSeed[],
): Promise<number> {
  const review = await storedReview(deployment, tutor, submissionId);
  const draft = review.revisions.find((revision) => revision.state === 'draft');
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
    {
      revision,
      version: draft?.version ?? 0,
      save_id: randomUUID(),
      feedback,
      items: items.map((item) => ({
        question_id: item.questionId,
        ...(item.points === undefined ? {} : { points: item.points }),
        ...(item.comment === undefined ? {} : { comment: item.comment }),
        ...(item.correctedIds === undefined ? {} : { corrected_attachment_ids: item.correctedIds }),
      })),
    },
  );
  if (answer.status !== 200) {
    throw new Error(`saving a review draft answered ${String(answer.status)}`);
  }
  return revision;
}

export async function publishReview(
  deployment: Deployment,
  tutor: TestAccount,
  submissionId: string,
  action: 'check' | 'return',
  feedback: string,
  items: readonly ReviewItemSeed[],
): Promise<void> {
  const revision = await saveReviewDraft(deployment, tutor, submissionId, feedback, items);
  const answer = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'POST',
    `/submissions/${submissionId}/${action}`,
    { revision },
  );
  if (answer.status !== 200) {
    throw new Error(`publishing the review answered ${String(answer.status)}`);
  }
}

export async function studentRead(deployment: Deployment, student: TestAccount, path: string): Promise<unknown> {
  const answer = await callApi(deployment, await accessToken(deployment, student), 'GET', path);
  if (answer.status !== 200) {
    throw new Error(`reading ${path} answered ${String(answer.status)}`);
  }
  return answer.body;
}

const fileKeys = ['expires_at', 'filename', 'height', 'media_type', 'size_bytes', 'url', 'width'];

const submissionKeys: Readonly<Record<string, readonly string[]>> = {
  '': [
    'id',
    'title',
    'tutor',
    'status',
    'late',
    'issued_before_reclaim',
    'deadline_at',
    'version',
    'blocks',
    'answers',
    'editable_question_ids',
    'review',
    'revealed',
  ],
  tutor: ['relationship_id', 'name', 'colour'],
  'blocks[]': [
    'id',
    'type',
    'text',
    'prompt',
    'images',
    'options',
    'max_length',
    'allowed_kinds',
    'max_files',
    'material',
  ],
  'blocks[].images[]': fileKeys,
  'blocks[].options[]': ['id', 'text'],
  'blocks[].material': [...fileKeys, 'title'],
  'answers[]': ['question_id', 'value', 'files'],
  'answers[].value': ['option_ids', 'text', 'attachment_ids'],
  'answers[].files[]': [...fileKeys, 'attachment_id'],
  review: ['feedback', 'items'],
  'review.items[]': ['question_id', 'comment', 'corrected_files'],
  'review.items[].corrected_files[]': [...fileKeys, 'attachment_id'],
  revealed: ['questions'],
  'revealed.questions[]': [
    'question_id',
    'correct_option_ids',
    'accepted_answers',
    'correct_value',
    'tolerance',
    'explanation',
  ],
};

const assignmentListKeys: Readonly<Record<string, readonly string[]>> = {
  '': ['items', 'next_cursor', 'counts'],
  counts: ['todo', 'closed'],
  'items[]': [
    'submission_id',
    'title',
    'tutor',
    'status',
    'late',
    'issued_before_reclaim',
    'deadline_at',
    'issued_at',
    'submitted_at',
    'checked_at',
    'feedback_excerpt',
    'has_corrected_files',
  ],
  'items[].tutor': ['relationship_id', 'name', 'colour'],
};

function keyProblems(value: unknown, path: string, allowed: Readonly<Record<string, readonly string[]>>): string[] {
  if (Array.isArray(value)) {
    return value.flatMap((entry: unknown) => keyProblems(entry, `${path}[]`, allowed));
  }
  if (typeof value !== 'object' || value === null) {
    return [];
  }
  const keys = allowed[path];
  if (keys === undefined) {
    return [`an object at ${path === '' ? 'the top' : path} that the student schema does not define`];
  }
  return Object.entries(value).flatMap(([key, entry]) => [
    ...(keys.includes(key) ? [] : [`${path === '' ? '' : `${path}.`}${key}`]),
    ...keyProblems(entry, path === '' ? key : `${path}.${key}`, allowed),
  ]);
}

export function expectStudentSubmissionKeys(body: unknown): void {
  expect(keyProblems(body, '', submissionKeys)).toEqual([]);
}

export function expectAssignmentListKeys(body: unknown): void {
  expect(keyProblems(body, '', assignmentListKeys)).toEqual([]);
}

export const scoreForms = {
  ru: [/балл/i, /очк/i, /\d+\s*из\s*\d+/, /(?<![\d/])\d+\s*\/\s*10(?![\d/])/, /%/],
  en: [/\bpoints?\b/i, /\bscore/i, /\b\d+\s*of\s*\d+\b/, /(?<![\d/])\d+\s*\/\s*10(?![\d/])/, /%/],
} as const;

export function scoreFormsIn(text: string, language: keyof typeof scoreForms): string[] {
  return scoreForms[language].filter((form) => form.test(text)).map((form) => form.source);
}

export async function tutorFinalPoints(
  deployment: Deployment,
  tutor: TestAccount,
  submissionId: string,
): Promise<number | null> {
  const body = await studentRead(deployment, tutor, `/submissions/${submissionId}`);
  if (typeof body !== 'object' || body === null || !('final_points' in body)) {
    throw new Error('the tutor submission has no final_points');
  }
  return typeof body.final_points === 'number' ? body.final_points : null;
}

export async function reviewQueueIds(deployment: Deployment, tutor: TestAccount): Promise<string[]> {
  const body = await studentRead(deployment, tutor, '/review-queue?limit=100');
  if (typeof body !== 'object' || body === null || !('items' in body) || !Array.isArray(body.items)) {
    throw new Error('the review queue has no items');
  }
  return body.items.flatMap((item: unknown) =>
    typeof item === 'object' && item !== null && 'submission_id' in item && typeof item.submission_id === 'string'
      ? [item.submission_id]
      : [],
  );
}

const studentLessonIssueKeys: Readonly<Record<string, readonly string[]>> = {
  '[]': ['submission_id', 'title', 'deadline_at', 'status', 'late'],
};

export async function studentLessonIssues(
  deployment: Deployment,
  student: TestAccount,
  lessonId: string,
): Promise<readonly unknown[]> {
  const body = await studentRead(deployment, student, `/lessons/${lessonId}`);
  if (typeof body !== 'object' || body === null || !('issues' in body) || !Array.isArray(body.issues)) {
    throw new Error('the lesson detail has no issues');
  }
  const issues: readonly unknown[] = body.issues;
  expect(keyProblems(issues, '', studentLessonIssueKeys)).toEqual([]);
  return issues;
}
