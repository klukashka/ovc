import type { Language } from '../i18n/resources.ts';

const locales: Readonly<Record<Language, string>> = { ru: 'ru-RU', en: 'en-GB' };

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
