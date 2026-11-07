import type { TFunction } from 'i18next';
import { type ReactElement, type RefObject, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { paletteEdgeClass } from '../../components/ui/palette.ts';
import type { Me } from '../../lib/api/session.ts';
import { localDateOf } from '../../lib/time/localDates.ts';
import type { CalendarItem } from './api.ts';
import { formatClock, formatDay } from './calendarFormat.ts';
import { LessonStatusWord, LessonTitle, audienceLabel } from './LessonParts.tsx';

export function lessonWhen(t: TFunction, item: CalendarItem, me: Me): string {
  const day = formatDay(localDateOf(Date.parse(item.starts_at), me.timezone), me.language);
  const times = t('lesson.timeRange', {
    start: formatClock(item.starts_at, me.language, me.timezone),
    end: formatClock(item.ends_at, me.language, me.timezone),
  });
  return `${day}, ${times}`;
}

export function NextLessonLine({ item, me }: { readonly item: CalendarItem | null; readonly me: Me }): ReactElement {
  const { t } = useTranslation();
  if (item === null) {
    return <span className="text-small text-text-muted">{t('lessons.noNextLesson')}</span>;
  }
  return (
    <span className="flex min-w-0 flex-col text-small">
      <span className="text-text-muted">{t('lessons.nextLesson')}</span>
      <span className="tabular">{lessonWhen(t, item, me)}</span>
    </span>
  );
}

export function LessonList({
  items,
  label,
  me,
  showAudience,
  listRef,
}: {
  readonly items: readonly CalendarItem[];
  readonly label: string;
  readonly me: Me;
  readonly showAudience: boolean;
  readonly listRef?: RefObject<HTMLUListElement | null>;
}): ReactElement {
  const { t } = useTranslation();
  const ownList = useRef<HTMLUListElement>(null);
  const list = listRef ?? ownList;
  useFocusAfterRemovedRow(
    list,
    items.map((item) => item.id),
    ':scope > a',
  );
  return (
    <ul ref={list} aria-label={label} className="flex flex-col gap-2">
      {items.map((item) => {
        const edge = item.audience.colour === null ? 'border-l-border-strong' : paletteEdgeClass[item.audience.colour];
        return (
          <li key={item.id} data-row={item.id}>
            <Link
              to={`/lessons/${encodeURIComponent(item.id)}`}
              className={`flex min-h-11 min-w-0 flex-col gap-0.5 rounded-sm border border-l-4 border-border bg-surface px-3 py-2 text-small hover:border-border-strong ${edge}`}
            >
              <span className="tabular text-text-muted">{lessonWhen(t, item, me)}</span>
              <span className="text-strong">
                <LessonTitle title={item.title} cancelled={item.status === 'cancelled'} />
              </span>
              {showAudience ? (
                <span dir="auto" className="truncate text-text-muted">
                  {audienceLabel(t, item.audience)}
                </span>
              ) : null}
              <LessonStatusWord lesson={item} />
            </Link>
          </li>
        );
      })}
    </ul>
  );
}
