import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type BuilderTexts = FeatureSections['builder'];

export const builderTextLoaders: TextLoaders<'builder'> = {
  ru: async () => (await import('./texts.ru.ts')).builderTexts,
  en: async () => (await import('./texts.en.ts')).builderTexts,
};
