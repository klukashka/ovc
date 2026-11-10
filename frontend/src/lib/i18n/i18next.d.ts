import 'i18next';

import type { TranslationResources } from './resources.ts';

declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'translation';
    resources: { translation: TranslationResources };
  }
}
