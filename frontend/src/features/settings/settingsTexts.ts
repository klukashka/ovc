import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type SettingsTexts = FeatureSections['settings'];

export const settingsTextLoaders: TextLoaders<'settings'> = {
  ru: async () => (await import('./settingsTexts.ru.ts')).settingsTexts,
  en: async () => (await import('./settingsTexts.en.ts')).settingsTexts,
};
