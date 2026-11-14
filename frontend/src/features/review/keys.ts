export const reviewKey = ['review'] as const;

export interface QueueFilter {
  readonly issue: string | null;
  readonly group: string | null;
  readonly student: string | null;
}

export function queueKey(filter: QueueFilter): readonly unknown[] {
  return [...reviewKey, 'queue', filter.issue, filter.group, filter.student];
}

export function queuePanelKey(limit: number): readonly unknown[] {
  return [...reviewKey, 'panel', limit];
}

export function reviewSubmissionKey(id: string): readonly unknown[] {
  return [...reviewKey, 'submission', id];
}

export function nextSubmissionKey(id: string, filter: QueueFilter): readonly unknown[] {
  return [...reviewKey, 'next', id, filter.issue, filter.group, filter.student];
}
