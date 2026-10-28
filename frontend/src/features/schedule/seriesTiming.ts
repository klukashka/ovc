import {
  type LocalDate,
  daysBetween,
  localDateOf,
  localInstant,
  localMinutesOf,
  localTimeOf,
} from '../../lib/time/localDates.ts';

export interface SeriesRule {
  readonly start: string;
  readonly weekdays: readonly number[];
  readonly timezone: string;
}

export interface SeriesTiming {
  readonly date: LocalDate;
  readonly start: string;
  readonly weekdays: readonly number[];
}

const daysPerWeek = 7;

function minutesOf(clock: string): number {
  const [hours, minutes] = clock.split(':').map(Number);
  return (hours ?? 0) * 60 + (minutes ?? 0);
}

function shiftedWeekday(weekday: number, days: number): number {
  return ((((weekday - 1 + days) % daysPerWeek) + daysPerWeek) % daysPerWeek) + 1;
}

export function seriesTimingIn(rule: SeriesRule, occurrenceDate: LocalDate, timeZone: string): SeriesTiming {
  const instant = localInstant(occurrenceDate, minutesOf(rule.start), rule.timezone);
  const date = localDateOf(instant, timeZone);
  const shift = daysBetween(occurrenceDate, date);
  const weekdays = new Set(rule.weekdays.map((weekday) => shiftedWeekday(weekday, shift)));
  return {
    date,
    start: localTimeOf(localMinutesOf(instant, timeZone)),
    weekdays: [...weekdays].sort((left, right) => left - right),
  };
}
