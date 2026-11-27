import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type TeachingSettingsTexts = FeatureSections['teachingSettings'];

export const teachingSettingsTextLoaders: TextLoaders<'teachingSettings'> = {
  ru: async () => (await import('./teachingTexts.ru.ts')).teachingSettingsTexts,
  en: async () => (await import('./teachingTexts.en.ts')).teachingSettingsTexts,
};
