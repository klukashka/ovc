import type { FeatureSections } from '../../lib/i18n/resources.ts';
import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';

export type LibraryTexts = FeatureSections['library'];

export const libraryTextLoaders: TextLoaders<'library'> = {
  ru: async () => (await import('./texts.ru.ts')).libraryTexts,
  en: async () => (await import('./texts.en.ts')).libraryTexts,
};
