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

export interface MediaQuerySource {
  readonly subscribe: (onChange: () => void) => () => void;
  readonly snapshot: () => boolean;
}

export type MediaQueries = (query: string) => MediaQuerySource;

const mediaQuerySources = new Map<string, MediaQuerySource>();

export function windowMediaQueries(query: string): MediaQuerySource {
  const existing = mediaQuerySources.get(query);
  if (existing !== undefined) {
    return existing;
  }
  const list = window.matchMedia(query);
  const source: MediaQuerySource = {
    subscribe: (onChange) => {
      list.addEventListener('change', onChange);
      return (): void => {
        list.removeEventListener('change', onChange);
      };
    },
    snapshot: () => list.matches,
  };
  mediaQuerySources.set(query, source);
  return source;
}

export function fixedMediaQueries(matching: readonly string[]): MediaQueries {
  return (query) => ({
    subscribe: () => (): void => undefined,
    snapshot: () => matching.includes(query),
  });
}

export const MediaQueriesContext = createContext<MediaQueries>(windowMediaQueries);

export function useMediaQuery(query: string): boolean {
  const source = useContext(MediaQueriesContext)(query);
  return useSyncExternalStore(source.subscribe, source.snapshot);
}

export const wideLayoutQuery = '(min-width: 1024px)';
