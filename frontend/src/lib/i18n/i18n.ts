import i18next, { type i18n } from 'i18next';
import ICU from 'i18next-icu';
import { initReactI18next } from 'react-i18next';

import { en } from './en.ts';
import { type Language } from './resources.ts';
import { ru } from './ru.ts';

export async function createI18n(language: Language): Promise<i18n> {
  const instance = i18next.createInstance();
  await instance
    .use(ICU)
    .use(initReactI18next)
    .init({
      lng: language,
      fallbackLng: false,
      supportedLngs: ['ru', 'en'],
      resources: { ru: { translation: ru }, en: { translation: en } },
      interpolation: { escapeValue: false },
      returnNull: false,
    });
  return instance;
}
