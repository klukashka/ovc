import { expect, test } from 'vitest';

import type { components } from '../../lib/api/schema.ts';
import { studentWorkOf, tutorIssuesOf } from './lessonIssues.ts';

type LessonIssue = components['schemas']['LessonIssue'];

const tutorIssue: LessonIssue = {
  issue_id: '3d9e1f2a-4b5c-4d6e-8f70-1a2b3c4d5e6f',
  title: 'Fractions',
  deadline_at: null,
  submitted: 1,
  students: 2,
};

const studentIssue: LessonIssue = {
  submission_id: '9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d',
  title: 'Fractions',
  deadline_at: '2026-11-20T15:00:00Z',
  status: 'submitted',
  late: true,
};

test('tutor rows keep only items that carry an issue and its progress', () => {
  const rows = tutorIssuesOf([tutorIssue, studentIssue, { title: 'Broken', deadline_at: null, issue_id: 'x' }]);

  expect(rows).toEqual([
    { issueId: tutorIssue.issue_id, title: 'Fractions', deadlineAt: null, submitted: 1, students: 2 },
  ]);
});

test('student rows keep only items that carry a submission and its status', () => {
  const rows = studentWorkOf([studentIssue, tutorIssue, { title: 'Broken', deadline_at: null, submission_id: 'y' }]);

  expect(rows).toEqual([
    {
      submissionId: studentIssue.submission_id,
      title: 'Fractions',
      deadlineAt: '2026-11-20T15:00:00Z',
      status: 'submitted',
      late: true,
    },
  ]);
});
