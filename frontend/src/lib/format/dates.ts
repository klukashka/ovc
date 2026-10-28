import type { Language } from '../i18n/resources.ts';

const locales: Readonly<Record<Language, string>> = { ru: 'ru-RU', en: 'en-GB' };

export function localeOf(language: Language): string {
  return locales[language];
}

export function formatDateTime(instant: string, language: Language, timeZone: string): string {
  const moment = new Date(instant);
  const day = new Intl.DateTimeFormat(locales[language], { day: 'numeric', month: 'short', timeZone }).format(moment);
  const time = new Intl.DateTimeFormat(locales[language], {
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
    timeZone,
  }).format(moment);
  return `${day}, ${time}`;
}

export function formatDate(instant: string, language: Language, timeZone: string): string {
  return new Intl.DateTimeFormat(locales[language], {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    timeZone,
  }).format(new Date(instant));
}

interface RelativeStep {
  readonly unit: Intl.RelativeTimeFormatUnit;
  readonly size: number;
  readonly below: number;
}

const relativeSteps: readonly RelativeStep[] = [
  { unit: 'second', size: 1, below: 60 },
  { unit: 'minute', size: 60, below: 3_600 },
  { unit: 'hour', size: 3_600, below: 86_400 },
  { unit: 'day', size: 86_400, below: 604_800 },
];

export function formatAgo(instant: string, language: Language, timeZone: string, nowMilliseconds: number): string {
  const elapsedSeconds = Math.max(0, Math.floor((nowMilliseconds - new Date(instant).getTime()) / 1_000));
  const step = relativeSteps.find((candidate) => elapsedSeconds < candidate.below);
  if (step === undefined) {
    return formatDateTime(instant, language, timeZone);
  }
  const amount = step.unit === 'second' ? 0 : Math.floor(elapsedSeconds / step.size);
  return new Intl.RelativeTimeFormat(locales[language], { numeric: 'auto' }).format(-amount, step.unit);
}
