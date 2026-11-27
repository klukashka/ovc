import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type InvitationPageTexts = FeatureSections['invitationPage'];

export const invitationPageTextLoaders: TextLoaders<'invitationPage'> = {
  ru: async () => (await import('./invitationTexts.ru.ts')).invitationPageTexts,
  en: async () => (await import('./invitationTexts.en.ts')).invitationPageTexts,
};
