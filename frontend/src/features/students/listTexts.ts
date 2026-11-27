import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type StudentListTexts = FeatureSections['studentList'];

export const studentListTextLoaders: TextLoaders<'studentList'> = {
  ru: async () => (await import('./listTexts.ru.ts')).studentListTexts,
  en: async () => (await import('./listTexts.en.ts')).studentListTexts,
};
