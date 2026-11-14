import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type ReclaimTexts = FeatureSections['reclaim'];

export const reclaimTextLoaders: TextLoaders<'reclaim'> = {
  ru: async () => (await import('./reclaimTexts.ru.ts')).reclaimTexts,
  en: async () => (await import('./reclaimTexts.en.ts')).reclaimTexts,
};
