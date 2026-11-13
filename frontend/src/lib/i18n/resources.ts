import type { assignmentTexts } from '../../features/assignment/texts.en.ts';
import type { builderTexts } from '../../features/builder/texts.en.ts';
import type { homeworkPanelTexts } from '../../features/home/panelTexts.en.ts';
import type { homeworkTexts } from '../../features/homework/texts.en.ts';
import type { libraryTexts } from '../../features/library/texts.en.ts';
import type { notificationEventTexts } from '../../features/notifications/texts.en.ts';
import type { ru } from './ru.ts';

export type Shape<Tree> = { readonly [Key in keyof Tree]: Tree[Key] extends string ? string : Shape<Tree[Key]> };

export type Resources = Shape<typeof ru>;

export interface FeatureSections {
  readonly library: Shape<typeof libraryTexts>;
  readonly builder: Shape<typeof builderTexts>;
  readonly assignment: Shape<typeof assignmentTexts>;
  readonly notificationEvents: Shape<typeof notificationEventTexts>;
  readonly homework: Shape<typeof homeworkTexts>;
  readonly homeworkPanels: Shape<typeof homeworkPanelTexts>;
}

export type FeatureSection = keyof FeatureSections;

export type TranslationResources = Resources & FeatureSections;

export { isLanguage, type Language, languages } from './languages.ts';
