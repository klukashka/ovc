import type { FeatureSections } from '../../lib/i18n/resources.ts';
import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';

export type NotificationEventTexts = FeatureSections['notificationEvents'];

export const notificationEventTextLoaders: TextLoaders<'notificationEvents'> = {
  ru: async () => (await import('./texts.ru.ts')).notificationEventTexts,
  en: async () => (await import('./texts.en.ts')).notificationEventTexts,
};
