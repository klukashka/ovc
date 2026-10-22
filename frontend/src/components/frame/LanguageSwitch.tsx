import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';

import { usePreferences } from '../../app/context.ts';
import { type Language, languages } from '../../lib/i18n/resources.ts';
import { preferenceKeys } from '../../lib/preferences/store.ts';

export function LanguageSwitch({
  onChosen,
}: {
  readonly onChosen?: ((language: Language) => void) | undefined;
}): ReactElement {
  const { t, i18n } = useTranslation();
  const preferences = usePreferences();

  function choose(language: Language): void {
    preferences.write(preferenceKeys.language, language);
    void i18n.changeLanguage(language);
    onChosen?.(language);
  }

  return (
    <div role="group" aria-label={t('language.label')} className="flex rounded-sm bg-hover p-0.5">
      {languages.map((language) => {
        const pressed = i18n.language === language;
        return (
          <button
            key={language}
            type="button"
            lang={language}
            aria-pressed={pressed}
            onClick={() => {
              choose(language);
            }}
            className={`min-h-11 rounded-sm px-3 text-small desktop:pointer-fine:min-h-10 ${pressed ? 'border border-border-strong bg-field font-medium text-text' : 'border border-transparent text-text-muted hover:text-text'}`}
          >
            {t(`language.${language}`)}
          </button>
        );
      })}
    </div>
  );
}
