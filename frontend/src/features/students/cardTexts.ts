import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type StudentCardTexts = FeatureSections['studentCard'];

export const studentCardTextLoaders: TextLoaders<'studentCard'> = {
  ru: async () => (await import('./cardTexts.ru.ts')).studentCardTexts,
  en: async () => (await import('./cardTexts.en.ts')).studentCardTexts,
};
