export const languages = ['ru', 'en'] as const;

export type Language = (typeof languages)[number];

export function isLanguage(value: string | null): value is Language {
  return languages.some((language) => language === value);
}
