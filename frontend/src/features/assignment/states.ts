import type { Tone } from '../../components/ui/StatusWord.tsx';
import type { SubmissionStatus } from './api.ts';

export type StudentStateWord =
  'todo' | 'dueSoon' | 'overdue' | 'inProgress' | 'submitted' | 'submittedLate' | 'returned' | 'checked' | 'closed';

const dueSoonMilliseconds = 24 * 60 * 60 * 1000;

export function studentStateWord(
  status: SubmissionStatus,
  deadlineAt: string | null,
  late: boolean,
  now: number,
): StudentStateWord {
  switch (status) {
    case 'not_started':
    case 'draft': {
      const deadline = deadlineAt === null ? null : Date.parse(deadlineAt);
      if (deadline !== null && deadline <= now) {
        return 'overdue';
      }
      if (deadline !== null && deadline - now <= dueSoonMilliseconds) {
        return 'dueSoon';
      }
      return status === 'draft' ? 'inProgress' : 'todo';
    }
    case 'submitted':
      return late ? 'submittedLate' : 'submitted';
    case 'returned':
      return 'returned';
    case 'checked':
      return 'checked';
    case 'closed':
      return 'closed';
  }
}

export const stateTones: Readonly<Record<StudentStateWord, Tone>> = {
  todo: 'info',
  dueSoon: 'warning',
  overdue: 'danger',
  inProgress: 'info',
  submitted: 'info',
  submittedLate: 'warning',
  returned: 'warning',
  checked: 'success',
  closed: 'muted',
};
