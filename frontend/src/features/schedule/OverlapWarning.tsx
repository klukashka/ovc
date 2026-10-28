import { type ReactElement, useEffect, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { ActionButton, quietButtonClass } from '../../components/ui/Form.tsx';
import type { ApiError, OverlappingLesson } from '../../lib/api/client.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import { localDateOf } from '../../lib/time/localDates.ts';
import { formatClock, formatDay } from './calendarFormat.ts';
import { LessonTitle } from './LessonParts.tsx';

export function OverlapWarning({
  error,
  language,
  timeZone,
  busy,
  unavailable,
  onConfirm,
}: {
  readonly error: ApiError;
  readonly language: Language;
  readonly timeZone: string;
  readonly busy: boolean;
  readonly unavailable: boolean;
  readonly onConfirm: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const warning = useRef<HTMLDivElement>(null);
  const introId = useId();

  useEffect(() => {
    warning.current?.focus();
  }, [error]);

  return (
    <div
      ref={warning}
      role="alert"
      tabIndex={-1}
      aria-labelledby={introId}
      className="flex flex-col gap-2 rounded-md border border-warning p-3 outline-none"
    >
      <p id={introId} className="text-strong">
        {t('newLesson.overlapIntro')}
      </p>
      <ul className="flex flex-col gap-1 text-small">
        {error.overlappingLessons.map((lesson: OverlappingLesson, index) => (
          <li key={`${lesson.starts_at}-${String(index)}`} className="flex flex-col">
            <span className="text-text-muted">
              {formatDay(localDateOf(Date.parse(lesson.starts_at), timeZone), language)},{' '}
              {t('lesson.timeRange', {
                start: formatClock(lesson.starts_at, language, timeZone),
                end: formatClock(lesson.ends_at, language, timeZone),
              })}
            </span>
            <LessonTitle title={lesson.title} cancelled={false} />
          </li>
        ))}
      </ul>
      {error.moreOverlapping > 0 ? (
        <p className="text-small text-text-muted">{t('newLesson.overlapMore', { count: error.moreOverlapping })}</p>
      ) : null}
      <div>
        <ActionButton busy={busy} unavailable={unavailable} onClick={onConfirm} className={quietButtonClass}>
          {t('newLesson.scheduleAnyway')}
        </ActionButton>
      </div>
    </div>
  );
}
