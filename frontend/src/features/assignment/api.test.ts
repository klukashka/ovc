import { expect, test } from 'vitest';

import type { components } from '../../lib/api/schema.ts';
import { parseSubmission } from './api.ts';

type StudentSubmission = components['schemas']['StudentSubmission'];

const questionId = '5c4b3a29-1807-4f6e-9d5c-4b3a29180716';
const optionId = '8d7c6b5a-4938-4271-a0b9-c8d7e6f5a4b3';
const attachmentId = '0b5e2c8a-3f41-4d7e-9a6b-2c1d8e7f4a90';

const stray = { points: 6, final_points: 6, percentage: 60 };

const base: StudentSubmission = {
  id: '1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d',
  title: 'Past simple',
  tutor: { relationship_id: '2b3c4d5e-6f70-4b8c-9d0e-1f2a3b4c5d6e', name: 'Maria', colour: null },
  status: 'checked',
  late: false,
  issued_before_reclaim: false,
  deadline_at: null,
  version: 4,
  blocks: [],
  answers: [],
  editable_question_ids: [],
  review: null,
};

test('a submission without a published review or revealed answers has neither', () => {
  const submission = parseSubmission(base);

  expect(submission.review).toBeNull();
  expect(submission.revealed).toBeNull();
});

test('the published review and the revealed answers keep only their named fields', () => {
  const response: StudentSubmission = {
    ...base,
    review: {
      ...stray,
      feedback: '**Great work.**',
      items: [
        {
          ...stray,
          question_id: questionId,
          comment: 'Check the word order.',
          corrected_files: [
            {
              attachment_id: attachmentId,
              filename: 'Corrections.pdf',
              media_type: 'application/pdf',
              size_bytes: 2048,
              width: null,
              height: null,
              url: `/api/v1/files/${attachmentId}?t=signed`,
              expires_at: '2026-11-15T12:00:00Z',
            },
          ],
        },
      ],
    },
    revealed: {
      questions: [
        { ...stray, question_id: questionId, correct_option_ids: [optionId], explanation: 'Go is irregular.' },
      ],
    },
  };

  const submission = parseSubmission(response);

  expect(submission.review).toEqual({
    feedback: '**Great work.**',
    items: new Map([
      [
        questionId,
        {
          comment: 'Check the word order.',
          correctedFiles: [
            {
              attachmentId,
              filename: 'Corrections.pdf',
              mediaType: 'application/pdf',
              sizeBytes: 2048,
              url: `/api/v1/files/${attachmentId}?t=signed`,
              expiresAt: '2026-11-15T12:00:00Z',
            },
          ],
        },
      ],
    ]),
  });
  expect(submission.revealed).toEqual(
    new Map([
      [
        questionId,
        {
          correctOptionIds: [optionId],
          acceptedAnswers: [],
          correctValue: null,
          tolerance: null,
          explanation: 'Go is irregular.',
        },
      ],
    ]),
  );
});
