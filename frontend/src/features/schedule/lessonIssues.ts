import type { components } from '../../lib/api/schema.ts';

type LessonIssueResponse = components['schemas']['LessonIssue'];

export type LessonWorkStatus = NonNullable<LessonIssueResponse['status']>;

export interface TutorLessonIssue {
  readonly issueId: string;
  readonly title: string;
  readonly deadlineAt: string | null;
  readonly submitted: number;
  readonly students: number;
}

export interface StudentLessonWork {
  readonly submissionId: string;
  readonly title: string;
  readonly deadlineAt: string | null;
  readonly status: LessonWorkStatus;
  readonly late: boolean;
}

export const lessonIssueLimit = 20;

export function tutorIssuesOf(issues: readonly LessonIssueResponse[]): readonly TutorLessonIssue[] {
  return issues.flatMap((issue) =>
    issue.issue_id === undefined || issue.submitted === undefined || issue.students === undefined
      ? []
      : [
          {
            issueId: issue.issue_id,
            title: issue.title,
            deadlineAt: issue.deadline_at,
            submitted: issue.submitted,
            students: issue.students,
          },
        ],
  );
}

export function studentWorkOf(issues: readonly LessonIssueResponse[]): readonly StudentLessonWork[] {
  return issues.flatMap((issue) =>
    issue.submission_id === undefined || issue.status === undefined
      ? []
      : [
          {
            submissionId: issue.submission_id,
            title: issue.title,
            deadlineAt: issue.deadline_at,
            status: issue.status,
            late: issue.late ?? false,
          },
        ],
  );
}
