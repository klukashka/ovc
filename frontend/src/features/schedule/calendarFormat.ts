import { dateTimeFormat } from '../../lib/format/dates.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import { type LocalDate, utcNoon as noonOf } from '../../lib/time/localDates.ts';
import type { CalendarRange } from './calendarTime.ts';

export function formatClock(instant: string, language: Language, timeZone: string): string {
  return dateTimeFormat(language, {
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
    timeZone,
  }).format(new Date(instant));
}

export function formatDay(date: LocalDate, language: Language): string {
  return dateTimeFormat(language, {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    timeZone: 'UTC',
  }).format(noonOf(date));
}

export function formatLongDay(date: LocalDate, language: Language): string {
  return dateTimeFormat(language, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
    timeZone: 'UTC',
  }).format(noonOf(date));
}

export function formatFullDate(date: LocalDate, language: Language): string {
  return dateTimeFormat(language, {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
    timeZone: 'UTC',
  }).format(noonOf(date));
}

export function formatWeekdayName(isoWeekday: number, language: Language, width: 'short' | 'long'): string {
  const monday = Date.UTC(2024, 0, 1, 12);
  return dateTimeFormat(language, { weekday: width, timeZone: 'UTC' }).format(
    new Date(monday + (isoWeekday - 1) * 86_400_000),
  );
}

export function formatDayNumber(date: LocalDate, language: Language): string {
  return dateTimeFormat(language, { day: 'numeric', timeZone: 'UTC' }).format(noonOf(date));
}

export function formatRangeTitle(range: CalendarRange, language: Language): string {
  if (range.view === 'month') {
    return dateTimeFormat(language, { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(noonOf(range.anchor));
  }
  const first = range.days[0];
  const last = range.days.at(-1);
  if (first === undefined || last === undefined) {
    throw new Error('a week range has no days');
  }
  return dateTimeFormat(language, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    timeZone: 'UTC',
  }).formatRange(noonOf(first), noonOf(last));
}
