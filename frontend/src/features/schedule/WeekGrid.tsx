import { type ReactElement, useLayoutEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { useScheduler } from '../../app/clock.ts';

import type { Language } from '../../lib/i18n/resources.ts';
import type { CalendarItem } from './api.ts';
import { formatDay, formatLongDay } from './calendarFormat.ts';
import { type LocalDate, localDateOf, localMinutesOf, localTimeOf } from '../../lib/time/localDates.ts';
import { type CalendarRange, dayMinutes, daySegments } from './calendarTime.ts';
import { LessonBlock } from './LessonParts.tsx';

export type SlotChosen = (date: LocalDate, start: string) => void;

const slotMinutes = 30;
const hours = Array.from({ length: 24 }, (_, hour) => hour);
const slots = Array.from({ length: dayMinutes / slotMinutes }, (_, index) => index * slotMinutes);
const firstShownHour = 8;
const openingLeadHours = 1;
const fullBlockMinutes = 45;
const minimumBlockMinutes = 30;
const minimumLanePixels = 64;

export function openingHour(
  range: CalendarRange,
  items: readonly CalendarItem[],
  timeZone: string,
  now: number,
): number {
  const ordered = [...items].sort((a, b) => Date.parse(a.starts_at) - Date.parse(b.starts_at));
  const target = ordered.find((item) => Date.parse(item.ends_at) > now) ?? ordered[0];
  if (target === undefined) {
    return firstShownHour;
  }
  const startsAt = Date.parse(target.starts_at);
  if (!range.days.includes(localDateOf(startsAt, timeZone))) {
    return 0;
  }
  return Math.max(0, localMinutesOf(startsAt, timeZone) / 60 - openingLeadHours);
}

function percent(minutes: number): string {
  return `${String((minutes / dayMinutes) * 100)}%`;
}

function DayColumn({
  day,
  items,
  timeZone,
  language,
  onSlot,
}: {
  readonly day: LocalDate;
  readonly items: readonly CalendarItem[];
  readonly timeZone: string;
  readonly language: Language;
  readonly onSlot: SlotChosen | null;
}): ReactElement {
  const segments = daySegments(items, day, timeZone, minimumBlockMinutes);
  const lanes = Math.max(1, ...segments.map((segment) => segment.lanes));
  return (
    <div data-day={day} className="relative h-[72rem] overflow-x-auto overflow-y-hidden border-l border-border">
      <h3 className="sr-only">{formatLongDay(day, language)}</h3>
      <div className="relative h-full" style={{ minWidth: `${String(lanes * minimumLanePixels)}px` }}>
        {onSlot === null
          ? null
          : slots.map((minutes) => (
              <button
                key={minutes}
                type="button"
                tabIndex={-1}
                aria-hidden="true"
                data-slot={`${day}T${localTimeOf(minutes)}`}
                onClick={() => {
                  onSlot(day, localTimeOf(minutes));
                }}
                className="block h-6 w-full border-b border-border/50 hover:bg-surface"
              />
            ))}
        {segments.map((segment) => (
          <div
            key={segment.item.id}
            data-lesson={segment.item.id}
            className="absolute overflow-hidden px-0.5"
            style={{
              top: percent(segment.top),
              height: percent(segment.bottom - segment.top),
              left: `${String((segment.lane / segment.lanes) * 100)}%`,
              width: `${String(100 / segment.lanes)}%`,
            }}
          >
            <LessonBlock
              item={segment.item}
              language={language}
              timeZone={timeZone}
              layout={segment.bottom - segment.top < fullBlockMinutes ? 'compact' : 'fill'}
            />
          </div>
        ))}
      </div>
    </div>
  );
}

export function WeekGrid({
  range,
  items,
  timeZone,
  language,
  today,
  onSlot,
}: {
  readonly range: CalendarRange;
  readonly items: readonly CalendarItem[];
  readonly timeZone: string;
  readonly language: Language;
  readonly today: LocalDate;
  readonly onSlot: SlotChosen | null;
}): ReactElement {
  const { t } = useTranslation();
  const scheduler = useScheduler();
  const scroller = useRef<HTMLDivElement>(null);
  const openedWeek = useRef<LocalDate | null>(null);
  const weekStart = range.days[0] ?? null;

  useLayoutEffect(() => {
    const element = scroller.current;
    if (element === null || openedWeek.current === weekStart) {
      return;
    }
    openedWeek.current = weekStart;
    element.scrollTop =
      (element.scrollHeight / hours.length) * openingHour(range, items, timeZone, scheduler.wallClock());
  }, [weekStart, range, items, timeZone, scheduler]);

  return (
    <section aria-label={t('schedule.weekGrid')} className="flex flex-col">
      <ol className="grid grid-cols-[3rem_repeat(7,minmax(0,1fr))] border-b border-border">
        <li aria-hidden="true" />
        {range.days.map((day) => (
          <li
            key={day}
            aria-current={day === today ? 'date' : undefined}
            className={`px-1 py-2 text-center text-small ${day === today ? 'text-strong text-accent-text' : 'text-text-muted'}`}
          >
            {formatDay(day, language)}
          </li>
        ))}
      </ol>
      <div
        ref={scroller}
        data-part="hours"
        className="max-h-[36rem] overflow-y-auto desktop:max-h-[calc(100dvh-(--spacing(70)))]"
      >
        <div className="grid grid-cols-[3rem_repeat(7,minmax(0,1fr))]">
          <div aria-hidden="true" className="flex flex-col">
            {hours.map((hour) => (
              <span key={hour} className="h-12 pr-2 text-right text-label font-medium text-text-muted">
                {localTimeOf(hour * 60)}
              </span>
            ))}
          </div>
          {range.days.map((day) => (
            <DayColumn key={day} day={day} items={items} timeZone={timeZone} language={language} onSlot={onSlot} />
          ))}
        </div>
      </div>
    </section>
  );
}
