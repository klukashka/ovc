import { type ReactElement, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useScheduler } from '../../app/clock.ts';
import { type PaletteColour, paletteDotClass } from '../../components/ui/palette.ts';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatDateTime } from '../../lib/format/dates.ts';
import { relativeHint } from '../../lib/format/relative.ts';
import type { SubmissionStatus } from './api.ts';
import { stateTones, studentStateWord } from './states.ts';

function isPaletteColour(value: string | null): value is PaletteColour {
  return value !== null && Object.keys(paletteDotClass).includes(value);
}

const nowRefreshMilliseconds = 60_000;

export function useNow(): number {
  const scheduler = useScheduler();
  const [now, setNow] = useState(() => scheduler.wallClock());
  useEffect(
    () =>
      scheduler.schedule(nowRefreshMilliseconds, () => {
        setNow(scheduler.wallClock());
      }),
    [scheduler, now],
  );
  return now;
}

export function TutorName({ name, colour }: { readonly name: string; readonly colour: string | null }): ReactElement {
  return (
    <span className="inline-flex min-w-0 items-center gap-2">
      {isPaletteColour(colour) ? (
        <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[colour]}`} />
      ) : null}
      <span dir="auto" className="break-words">
        {name}
      </span>
    </span>
  );
}

export function DeadlineHint({
  deadline,
  now,
}: {
  readonly deadline: string | null;
  readonly now: number;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  if (deadline === null) {
    return <span>{t('assignment.list.noDeadline')}</span>;
  }
  return (
    <span>
      {formatDateTime(deadline, me.language, me.timezone)}{' '}
      <span className="text-text-muted">({relativeHint(deadline, me.language, now)})</span>
    </span>
  );
}

export function StateLabel({
  status,
  deadline,
  late,
  now,
}: {
  readonly status: SubmissionStatus;
  readonly deadline: string | null;
  readonly late: boolean;
  readonly now: number;
}): ReactElement {
  const { t } = useTranslation();
  const word = studentStateWord(status, deadline, late, now);
  return <StatusWord tone={stateTones[word]}>{t(`assignment.states.${word}`)}</StatusWord>;
}

export function ReclaimCue(): ReactElement {
  const { t } = useTranslation();
  return <span className="text-small text-text-muted">{t('assignment.cue')}</span>;
}
