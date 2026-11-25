import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type MyDataTexts = FeatureSections['myData'];

export const myDataTextLoaders: TextLoaders<'myData'> = {
  ru: async () => (await import('./myDataTexts.ru.ts')).myDataTexts,
  en: async () => (await import('./myDataTexts.en.ts')).myDataTexts,
};
