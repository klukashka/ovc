import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type EmailSettingsTexts = FeatureSections['emailSettings'];

export const emailSettingsTextLoaders: TextLoaders<'emailSettings'> = {
  ru: async () => (await import('./emailSettingsTexts.ru.ts')).emailSettingsTexts,
  en: async () => (await import('./emailSettingsTexts.en.ts')).emailSettingsTexts,
};
