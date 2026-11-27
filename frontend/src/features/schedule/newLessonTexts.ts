import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type NewLessonFormTexts = FeatureSections['newLessonForm'];

export const newLessonFormTextLoaders: TextLoaders<'newLessonForm'> = {
  ru: async () => (await import('./newLessonTexts.ru.ts')).newLessonFormTexts,
  en: async () => (await import('./newLessonTexts.en.ts')).newLessonFormTexts,
};
