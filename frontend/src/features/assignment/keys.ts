export const assignmentsKey = ['assignments'] as const;

export function assignmentListKey(status: string, tutor: string | null, limit: number): readonly unknown[] {
  return [...assignmentsKey, 'list', status, tutor, limit];
}

export function submissionKey(id: string): readonly unknown[] {
  return [...assignmentsKey, 'submission', id];
}
