import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type MyTutorsTexts = FeatureSections['myTutors'];

export const myTutorsTextLoaders: TextLoaders<'myTutors'> = {
  ru: async () => (await import('./texts.ru.ts')).myTutorsTexts,
  en: async () => (await import('./texts.en.ts')).myTutorsTexts,
};
