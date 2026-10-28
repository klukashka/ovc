import { type ReactElement, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { Language } from '../../lib/i18n/resources.ts';
import type { CalendarItem } from './api.ts';
import { formatDayNumber, formatLongDay, formatWeekdayName } from './calendarFormat.ts';
import { type LocalDate, sameMonth } from '../../lib/time/localDates.ts';
import { type CalendarRange, byStartDay } from './calendarTime.ts';
import { LessonBlock } from './LessonParts.tsx';

export const lessonsShownPerDay = 3;

const weekdays = [1, 2, 3, 4, 5, 6, 7];
const noLessons: readonly CalendarItem[] = [];

function DayCell({
  day,
  items,
  inMonth,
  isToday,
  timeZone,
  language,
}: {
  readonly day: LocalDate;
  readonly items: readonly CalendarItem[];
  readonly inMonth: boolean;
  readonly isToday: boolean;
  readonly timeZone: string;
  readonly language: Language;
}): ReactElement {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const list = useRef<HTMLUListElement>(null);
  const focusFirstNew = useRef(false);
  const shown = expanded ? items : items.slice(0, lessonsShownPerDay);
  const hidden = items.length - shown.length;

  useEffect(() => {
    if (!focusFirstNew.current) {
      return;
    }
    focusFirstNew.current = false;
    [...(list.current?.querySelectorAll<HTMLElement>(':scope > li > a') ?? [])].at(lessonsShownPerDay)?.focus();
  }, [expanded]);

  return (
    <li
      aria-current={isToday ? 'date' : undefined}
      className={`flex min-h-28 min-w-0 flex-col gap-1 border-t border-l border-border p-1 ${inMonth ? '' : 'bg-surface'}`}
    >
      <h3
        className={`text-small ${isToday ? 'text-strong text-accent-text' : inMonth ? 'text-text' : 'text-text-muted'}`}
      >
        <span aria-hidden="true">{formatDayNumber(day, language)}</span>
        <span className="sr-only">{formatLongDay(day, language)}</span>
      </h3>
      {shown.length === 0 ? null : (
        <ul ref={list} className="flex flex-col gap-1">
          {shown.map((item) => (
            <li key={item.id} className="contents">
              <LessonBlock item={item} language={language} timeZone={timeZone} />
            </li>
          ))}
        </ul>
      )}
      {hidden > 0 ? (
        <button
          type="button"
          onClick={() => {
            focusFirstNew.current = true;
            setExpanded(true);
          }}
          className="inline-flex min-h-11 items-center rounded-sm px-2 text-small text-accent-text underline"
        >
          {t('schedule.more', { count: hidden })}
        </button>
      ) : null}
    </li>
  );
}

export function MonthGrid({
  range,
  items,
  timeZone,
  language,
  today,
}: {
  readonly range: CalendarRange;
  readonly items: readonly CalendarItem[];
  readonly timeZone: string;
  readonly language: Language;
  readonly today: LocalDate;
}): ReactElement {
  const { t } = useTranslation();
  const days = byStartDay(items, timeZone);
  return (
    <section aria-label={t('schedule.monthGrid')} className="flex flex-col">
      <ol aria-hidden="true" className="grid grid-cols-7 text-center text-small text-text-muted">
        {weekdays.map((weekday) => (
          <li key={weekday} className="py-2">
            {formatWeekdayName(weekday, language, 'short')}
          </li>
        ))}
      </ol>
      <ol className="grid grid-cols-7 border-r border-b border-border">
        {range.days.map((day) => (
          <DayCell
            key={day}
            day={day}
            items={days.get(day) ?? noLessons}
            inMonth={sameMonth(day, range.anchor)}
            isToday={day === today}
            timeZone={timeZone}
            language={language}
          />
        ))}
      </ol>
    </section>
  );
}
