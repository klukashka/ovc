import type { ru } from './ru.ts';

type Shape<Tree> = { readonly [Key in keyof Tree]: Tree[Key] extends string ? string : Shape<Tree[Key]> };

export type Resources = Shape<typeof ru>;

export const languages = ['ru', 'en'] as const;

export type Language = (typeof languages)[number];

export function isLanguage(value: string | null): value is Language {
  return value === 'ru' || value === 'en';
}
