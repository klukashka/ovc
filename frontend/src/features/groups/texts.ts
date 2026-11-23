import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type GroupsTexts = FeatureSections['groups'];

export const groupsTextLoaders: TextLoaders<'groups'> = {
  ru: async () => (await import('./texts.ru.ts')).groupsTexts,
  en: async () => (await import('./texts.en.ts')).groupsTexts,
};
