import type { ru } from './ru.ts';

type Shape<Tree> = { readonly [Key in keyof Tree]: Tree[Key] extends string ? string : Shape<Tree[Key]> };

export type Resources = Shape<typeof ru>;

export { isLanguage, type Language, languages } from './languages.ts';
