import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';

import type { Language } from '../../lib/i18n/resources.ts';
import type { CalendarItem } from './api.ts';
import { formatLongDay } from './calendarFormat.ts';
import { type LocalDate, sameMonth } from '../../lib/time/localDates.ts';
import { type CalendarRange, byStartDay } from './calendarTime.ts';
import { LessonBlock } from './LessonParts.tsx';

export function Agenda({
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
  const lessonsByDay = byStartDay(items, timeZone);
  const periodDays = range.view === 'month' ? range.days.filter((day) => sameMonth(day, range.anchor)) : range.days;
  const days = periodDays.flatMap((day) => {
    const lessons = lessonsByDay.get(day);
    return lessons === undefined ? [] : [{ day, lessons }];
  });
  if (days.length === 0) {
    return <p className="text-text-muted">{t('schedule.empty')}</p>;
  }
  return (
    <section aria-label={t('schedule.agenda')} className="flex flex-col gap-4">
      {days.map(({ day, lessons }) => (
        <section key={day} aria-current={day === today ? 'date' : undefined} className="flex flex-col gap-2">
          <h3 className={day === today ? 'text-strong text-accent-text' : 'text-strong'}>
            {formatLongDay(day, language)}
          </h3>
          <ul className="flex flex-col gap-2">
            {lessons.map((item) => (
              <li key={item.id}>
                <LessonBlock item={item} language={language} timeZone={timeZone} />
              </li>
            ))}
          </ul>
        </section>
      ))}
    </section>
  );
}
