import type { i18n } from 'i18next';

import type { Me } from '../lib/api/session.ts';
import { reportLanguageLoadFailure, switchLanguage } from '../lib/i18n/i18n.ts';
import { type PreferenceStore, preferenceKeys } from '../lib/preferences/store.ts';

export const darkSchemeQuery = '(prefers-color-scheme: dark)';

export function resolvedTheme(preference: Me['theme'], prefersDark: boolean): 'light' | 'dark' {
  if (preference === 'system') {
    return prefersDark ? 'dark' : 'light';
  }
  return preference;
}

export function followSystemTheme(root: HTMLElement, scheme: MediaQueryList): () => void {
  function follow(): void {
    if (root.dataset.themePreference === 'system') {
      root.dataset.theme = resolvedTheme('system', scheme.matches);
    }
  }
  scheme.addEventListener('change', follow);
  return (): void => {
    scheme.removeEventListener('change', follow);
  };
}

export async function applyAccountPreferences(
  me: Me,
  preferences: PreferenceStore,
  translations: i18n,
  root: HTMLElement,
  prefersDark: boolean,
): Promise<void> {
  preferences.write(preferenceKeys.theme, me.theme);
  root.dataset.themePreference = me.theme;
  root.dataset.theme = resolvedTheme(me.theme, prefersDark);
  const applied =
    translations.language === me.language ||
    (await switchLanguage(translations, me.language).catch((error: unknown) => {
      reportLanguageLoadFailure(error);
      return false;
    }));
  if (applied) {
    preferences.write(preferenceKeys.language, me.language);
  }
}
