import type { TextLoaders } from '../../lib/i18n/featureTexts.ts';
import type { FeatureSections } from '../../lib/i18n/resources.ts';

export type HomeworkPanelTexts = FeatureSections['homeworkPanels'];

export const homeworkPanelTextLoaders: TextLoaders<'homeworkPanels'> = {
  ru: async () => (await import('./panelTexts.ru.ts')).homeworkPanelTexts,
  en: async () => (await import('./panelTexts.en.ts')).homeworkPanelTexts,
};
