import type { TFunction } from 'i18next';
import { type ReactElement, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { paletteDotClass } from '../../components/ui/palette.ts';
import { StatusWord, type Tone } from '../../components/ui/StatusWord.tsx';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatDateTime, localeOf } from '../../lib/format/dates.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import type { GroupSummary } from '../groups/api.ts';
import { type IssueAudience, type IssueItem, type SubmissionStatus, audienceName } from './api.ts';

type StateKey = 'submitted' | 'checkedAutomatically' | 'returned' | 'checked' | 'notSubmitted' | 'closed';

const stateTones: Readonly<Record<StateKey, Tone>> = {
  submitted: 'accent',
  checkedAutomatically: 'success',
  returned: 'warning',
  checked: 'success',
  notSubmitted: 'muted',
  closed: 'muted',
};

function stateKeyOf(status: SubmissionStatus, manual: boolean): StateKey {
  switch (status) {
    case 'submitted':
      return 'submitted';
    case 'checked':
      return manual ? 'checked' : 'checkedAutomatically';
    case 'returned':
      return 'returned';
    case 'closed':
      return 'closed';
    case 'not_started':
    case 'draft':
      return 'notSubmitted';
  }
}

export function lateShown(status: SubmissionStatus, late: boolean): boolean {
  return late && (status === 'submitted' || status === 'returned' || status === 'checked');
}

export function SubmissionState({
  status,
  late,
  manual,
  draft = false,
}: {
  readonly status: SubmissionStatus;
  readonly late: boolean;
  readonly manual: boolean;
  readonly draft?: boolean;
}): ReactElement {
  const { t } = useTranslation();
  const key = stateKeyOf(status, manual);
  return (
    <span className="inline-flex flex-wrap items-center gap-x-3 gap-y-1">
      <StatusWord tone={stateTones[key]}>{t(`homework.states.${key}`)}</StatusWord>
      {lateShown(status, late) ? <StatusWord tone="warning">{t('homework.states.late')}</StatusWord> : null}
      {draft ? <StatusWord tone="info">{t('homework.states.reviewInProgress')}</StatusWord> : null}
    </span>
  );
}

export function ReclaimCue(): ReactElement {
  const { t } = useTranslation();
  return <span className="block text-small text-text-muted">{t('homework.cue')}</span>;
}

export function scoreText(
  t: TFunction,
  language: Language,
  earned: number,
  max: number,
  percentage: number | null,
): string {
  if (max === 0 || percentage === null) {
    return t('homework.not_scored');
  }
  const percent = new Intl.NumberFormat(localeOf(language), { style: 'percent' }).format(percentage / 100);
  return t('homework.score', { earned, max, percent });
}

export function pointsText(t: TFunction, earned: number, max: number): string {
  return max === 0 ? t('homework.not_scored') : t('homework.points', { earned, max });
}

const secondsPerMinute = 60;
const secondsPerHour = 3_600;
const secondsPerDay = 86_400;

function relativeHint(instant: string, language: Language, now: number): string {
  const seconds = Math.round((Date.parse(instant) - now) / 1_000);
  const format = new Intl.RelativeTimeFormat(localeOf(language), { numeric: 'auto' });
  const size = Math.abs(seconds);
  if (size < secondsPerHour) {
    return format.format(Math.round(seconds / secondsPerMinute), 'minute');
  }
  if (size < secondsPerDay) {
    return format.format(Math.round(seconds / secondsPerHour), 'hour');
  }
  return format.format(Math.round(seconds / secondsPerDay), 'day');
}

export function DeadlineText({ deadline }: { readonly deadline: string | null }): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const [now] = useState(() => Date.now());
  if (deadline === null) {
    return <span>{t('homework.list.noDeadline')}</span>;
  }
  return (
    <span>
      {formatDateTime(deadline, me.language, me.timezone)}{' '}
      <span className="text-text-muted">({relativeHint(deadline, me.language, now)})</span>
    </span>
  );
}

export function AudienceLabel({
  audience,
  groups,
}: {
  readonly audience: IssueAudience;
  readonly groups: readonly GroupSummary[];
}): ReactElement {
  const group =
    audience.group_id === undefined ? null : (groups.find((entry) => entry.id === audience.group_id) ?? null);
  return (
    <span className="inline-flex min-w-0 items-center gap-2">
      {group === null ? null : (
        <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[group.colour]}`} />
      )}
      <span dir="auto" className="min-w-0 [overflow-wrap:anywhere]">
        {audienceName(audience)}
      </span>
    </span>
  );
}

export function ProgressText({
  progress,
  full = false,
}: {
  readonly progress: IssueItem['progress'];
  readonly full?: boolean;
}): ReactElement {
  const { t } = useTranslation();
  return (
    <span className="flex flex-col text-small">
      <span>{t('homework.progress.submitted', { submitted: progress.submitted, total: progress.students })}</span>
      {full ? (
        <span className="text-text-muted">
          {t('homework.progress.checked', { count: progress.checked })}
          {', '}
          {t('homework.progress.late', { count: progress.late })}
        </span>
      ) : null}
    </span>
  );
}

export function ToCheckBadge({ count }: { readonly count: number }): ReactElement | null {
  const { t } = useTranslation();
  if (count === 0) {
    return null;
  }
  return <span className="text-small font-medium text-accent-text">{t('homework.progress.toCheck', { count })}</span>;
}
