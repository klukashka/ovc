import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type AssignmentTexts = FeatureSections['assignment'];

export const assignmentTextLoaders: TextLoaders<'assignment'> = {
  ru: async () => (await import('./texts.ru.ts')).assignmentTexts,
  en: async () => (await import('./texts.en.ts')).assignmentTexts,
};
