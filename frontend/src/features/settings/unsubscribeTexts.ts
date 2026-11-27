import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type UnsubscribeTexts = FeatureSections['unsubscribe'];

export const unsubscribeTextLoaders: TextLoaders<'unsubscribe'> = {
  ru: async () => (await import('./unsubscribeTexts.ru.ts')).unsubscribeTexts,
  en: async () => (await import('./unsubscribeTexts.en.ts')).unsubscribeTexts,
};
