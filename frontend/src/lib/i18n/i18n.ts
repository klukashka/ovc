import i18next, { type i18n } from 'i18next';
import ICU from 'i18next-icu';
import { initReactI18next } from 'react-i18next';

import { type Language, type Resources, languages } from './resources.ts';

const namespace = 'translation';

const catalogues: Readonly<Record<Language, () => Promise<Resources>>> = {
  ru: async () => (await import('./ru.ts')).ru,
  en: async () => (await import('./en.ts')).en,
};

const requestedLanguages = new WeakMap<i18n, Language>();

export async function createI18n(language: Language): Promise<i18n> {
  const catalogue = await catalogues[language]();
  const instance = i18next.createInstance();
  await instance
    .use(ICU)
    .use(initReactI18next)
    .init({
      lng: language,
      fallbackLng: false,
      supportedLngs: [...languages],
      ns: [namespace],
      defaultNS: namespace,
      resources: { [language]: { [namespace]: catalogue } },
      interpolation: { escapeValue: false },
      returnNull: false,
    });
  return instance;
}

export function isLanguageLoaded(translations: i18n, language: Language): boolean {
  return translations.hasResourceBundle(language, namespace);
}

export function reportLanguageLoadFailure(error: unknown): void {
  console.error('language_load_failed', error instanceof Error ? error.name : typeof error);
}

export async function switchLanguage(translations: i18n, language: Language): Promise<boolean> {
  requestedLanguages.set(translations, language);
  if (!isLanguageLoaded(translations, language)) {
    const catalogue = await catalogues[language]();
    if (!isLanguageLoaded(translations, language)) {
      translations.addResourceBundle(language, namespace, catalogue);
    }
  }
  if (requestedLanguages.get(translations) !== language) {
    return false;
  }
  await translations.changeLanguage(language);
  return true;
}
