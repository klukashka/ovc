import { type LocalDate, addDays, localDateOf, localMinutesOf, localTimeOf } from '../../lib/time/localDates.ts';
import type { Audience } from './AudiencePicker.tsx';

export interface LessonPrefill {
  readonly date: LocalDate;
  readonly start: string;
  readonly audience?: Audience;
  readonly durationMinutes?: number;
  readonly weekdays?: readonly number[];
}

export function nextFullHour(now: number, timeZone: string): LessonPrefill {
  const today = localDateOf(now, timeZone);
  const hour = Math.floor(localMinutesOf(now, timeZone) / 60) + 1;
  return hour >= 24 ? { date: addDays(today, 1), start: '00:00' } : { date: today, start: localTimeOf(hour * 60) };
}

export function newLessonAddress(date?: LocalDate): string {
  return date === undefined ? '/schedule/new' : `/schedule/new?${new URLSearchParams({ date }).toString()}`;
}
