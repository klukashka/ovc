import { createContext, useContext, useSyncExternalStore } from 'react';

import type { PreferenceStore } from '../lib/preferences/store.ts';

export const PreferencesContext = createContext<PreferenceStore | null>(null);

export function usePreferences(): PreferenceStore {
  const store = useContext(PreferencesContext);
  if (store === null) {
    throw new Error('PreferencesContext is not provided');
  }
  return store;
}

export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const list = window.matchMedia(query);
      list.addEventListener('change', onChange);
      return () => {
        list.removeEventListener('change', onChange);
      };
    },
    () => window.matchMedia(query).matches,
  );
}

export const wideLayoutQuery = '(min-width: 1024px)';
