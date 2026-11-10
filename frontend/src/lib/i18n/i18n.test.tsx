import { cleanup, render } from '@testing-library/react';
import type { ReactElement } from 'react';
import { I18nextProvider, useTranslation } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { type Language } from './resources.ts';
import { createI18n, isLanguageLoaded, switchLanguage } from './i18n.ts';

function RenderedLabels({ into }: { readonly into: string[] }): ReactElement {
  const { t } = useTranslation();
  const label = t('language.label');
  into.push(label);
  return <p>{label}</p>;
}

function loadedLanguages(translations: Awaited<ReturnType<typeof createI18n>>): Language[] {
  return (['ru', 'en'] as const).filter((language) => isLanguageLoaded(translations, language));
}

afterEach(() => {
  cleanup();
});

describe('i18n', () => {
  test('the active language is loaded before the first render', async () => {
    const renderedLabels: string[] = [];

    const translations = await createI18n('en');
    render(
      <I18nextProvider i18n={translations}>
        <RenderedLabels into={renderedLabels} />
      </I18nextProvider>,
    );

    expect(renderedLabels[0]).toBe('Language');
    expect(loadedLanguages(translations)).toEqual(['en']);
  });

  test('a language switch applies only once its texts have loaded', async () => {
    const translations = await createI18n('en');

    const switching = switchLanguage(translations, 'ru');
    const languageWhileLoading = translations.language;
    const applied = await switching;

    expect(languageWhileLoading).toBe('en');
    expect(applied).toBe(true);
    expect(translations.language).toBe('ru');
    expect(translations.t('language.label')).toBe('Язык');
  });

  test('feature texts added in both languages leave the other catalogue to load on a switch', async () => {
    const translations = await createI18n('en');
    for (const language of ['en', 'ru'] as const) {
      translations.addResourceBundle(language, 'translation', { library: { root: 'Library' } }, true, true);
    }

    const russianLoadedBefore = isLanguageLoaded(translations, 'ru');
    const applied = await switchLanguage(translations, 'ru');

    expect(russianLoadedBefore).toBe(false);
    expect(applied).toBe(true);
    expect(translations.t('language.label')).toBe('Язык');
    expect(translations.t('library.root')).toBe('Library');
  });

  test('a switch overtaken by a later choice does not apply', async () => {
    const translations = await createI18n('en');

    const overtaken = switchLanguage(translations, 'ru');
    const latest = switchLanguage(translations, 'en');
    const results = await Promise.all([overtaken, latest]);

    expect(results).toEqual([false, true]);
    expect(translations.language).toBe('en');
  });
});
