import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { isLanguage } from '../../lib/i18n/languages.ts';
import { reportLanguageLoadFailure } from '../../lib/i18n/i18n.ts';
import type { chatTexts } from './texts.en.ts';

export type ChatTexts = { readonly [Key in keyof typeof chatTexts]: string };

export function useChatTexts(): {
  readonly texts: ChatTexts | null;
  readonly failed: boolean;
  readonly retry: () => void;
} {
  const { i18n } = useTranslation();
  const language = i18n.resolvedLanguage ?? i18n.language;
  const [attempt, setAttempt] = useState(0);
  const [loaded, setLoaded] = useState<{ readonly language: string; readonly texts: ChatTexts } | null>(null);
  const [failed, setFailed] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    if (!isLanguage(language)) {
      throw new Error('Unsupported chat language');
    }
    const loading = language === 'ru' ? import('./texts.ru.ts') : import('./texts.en.ts');
    void loading
      .then(({ chatTexts: texts }) => {
        if (active) {
          i18n.addResourceBundle(language, 'chat', texts);
          setLoaded({ language, texts });
          setFailed(null);
        }
      })
      .catch((error: unknown) => {
        if (active) {
          reportLanguageLoadFailure(error);
          setFailed(language);
        }
      });
    return (): void => {
      active = false;
    };
  }, [language, i18n, attempt]);

  return {
    texts: loaded?.texts ?? null,
    failed: failed === language,
    retry: (): void => {
      setFailed(null);
      setAttempt((value) => value + 1);
    },
  };
}
