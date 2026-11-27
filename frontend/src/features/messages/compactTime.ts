import { dateTimeFormat } from '../../lib/format/dates.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import { localDateOf } from '../../lib/time/localDates.ts';

export function compactTime(instant: string, language: Language, timeZone: string, now: number): string {
  const moment = new Date(instant);
  const day = localDateOf(moment.getTime(), timeZone);
  const today = localDateOf(now, timeZone);
  const elapsedDays = (Date.parse(today) - Date.parse(day)) / 86_400_000;
  const options: Intl.DateTimeFormatOptions =
    elapsedDays === 0
      ? { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }
      : elapsedDays > 0 && elapsedDays <= 6
        ? { weekday: 'short' }
        : { day: 'numeric', month: 'short', ...(day.slice(0, 4) === today.slice(0, 4) ? {} : { year: 'numeric' }) };
  return dateTimeFormat(language, { ...options, timeZone }).format(moment);
}
