import type { Language } from '../i18n/languages.ts';
import { localeOf } from './dates.ts';

const secondsPerMinute = 60;
const secondsPerHour = 3_600;
const secondsPerDay = 86_400;

export function relativeHint(instant: string, language: Language, now: number): string {
  const seconds = Math.round((Date.parse(instant) - now) / 1_000);
  const format = new Intl.RelativeTimeFormat(localeOf(language), { numeric: 'auto' });
  const size = Math.abs(seconds);
  if (size < secondsPerHour) {
    return format.format(Math.round(seconds / secondsPerMinute), 'minute');
  }
  if (size < secondsPerDay) {
    return format.format(Math.round(seconds / secondsPerHour), 'hour');
  }
  return format.format(Math.round(seconds / secondsPerDay), 'day');
}
