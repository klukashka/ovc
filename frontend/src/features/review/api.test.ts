import { describe, expect, test } from 'vitest';

import { ApiError } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import { parseSubmissionDetail } from './api.ts';
import { draftBody, draftFrom, draftPosition } from './draft.ts';

type TutorSubmission = components['schemas']['TutorSubmission'];
type StudentSubmission = components['schemas']['StudentSubmission'];

const studentDetail: StudentSubmission = {
  answers: [],
  blocks: [],
  deadline_at: null,
  editable_question_ids: [],
  id: 'submission-1',
  issued_before_reclaim: false,
  late: false,
  review: null,
  status: 'submitted',
  title: 'Past simple',
  tutor: { name: 'Maria', colour: null, relationship_id: 'relationship-1' },
  version: 3,
};

const tutorDetail: TutorSubmission = {
  answer_policy: 'after_check',
  answers: [
    {
      auto_correct: true,
      auto_points: 2,
      files: [],
      frozen_at: '2026-11-01T10:00:00Z',
      normalised: 'went',
      normaliser_version: 1,
      question_id: 'question-auto',
      value: { text: 'Went ' },
    },
  ],
  auto_points: 2,
  blocks: [
    { id: 'instruction-1', type: 'instruction', text: 'Read first' },
    { id: 'question-auto', type: 'short_text', prompt: 'Past of go', points: 2, accepted_answers: ['went'] },
    { id: 'question-file', type: 'file', prompt: 'Upload', points: 6, allowed_kinds: ['pdf'], max_files: 1 },
  ],
  checked_at: null,
  deadline_at: null,
  final_points: 2,
  first_submitted_at: '2026-11-01T10:00:00Z',
  id: 'submission-1',
  issue_id: 'issue-1',
  issued_before_reclaim: false,
  late: true,
  max_points: 8,
  percentage: 25,
  revisions: [
    {
      action: 'check',
      feedback: 'Good',
      items: [
        {
          comment: 'Nice',
          corrected_files: [],
          override_points: null,
          override_reason: null,
          points: 5,
          question_id: 'question-file',
        },
      ],
      published_at: '2026-11-02T10:00:00Z',
      revision: 1,
      state: 'published',
      version: 2,
    },
  ],
  status: 'checked',
  student: { display_name: 'Anna', relationship_id: 'relationship-1' },
  submitted_at: '2026-11-01T10:00:00Z',
  title: 'Past simple',
  version: 4,
};

describe('the review screen boundary', () => {
  test('refuses the student schema instead of reading student fields', () => {
    const parse = (): unknown => parseSubmissionDetail(studentDetail);

    expect(parse).toThrow(ApiError);
  });

  test('keeps only questions and starts a new draft after the latest publication', () => {
    const submission = parseSubmissionDetail(tutorDetail);

    const position = draftPosition(submission);
    const body = draftBody(submission.questions, draftFrom(submission));

    expect(submission.questions.map((question) => question.id)).toEqual(['question-auto', 'question-file']);
    expect(position).toEqual({ revision: 2, version: 0 });
    expect(body).toEqual({
      feedback: 'Good',
      items: [
        {
          question_id: 'question-auto',
          points: null,
          override_points: null,
          override_reason: null,
          comment: '',
          corrected_attachment_ids: [],
        },
        {
          question_id: 'question-file',
          points: 5,
          override_points: null,
          override_reason: null,
          comment: 'Nice',
          corrected_attachment_ids: [],
        },
      ],
    });
  });
});
