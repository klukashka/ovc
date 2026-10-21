export const preferenceKeys = {
  language: 'ovc.language',
  theme: 'ovc.theme',
  sidebar: 'ovc.sidebar',
} as const;

export type PreferenceKey = (typeof preferenceKeys)[keyof typeof preferenceKeys];

export interface PreferenceStore {
  read(key: PreferenceKey): string | null;
  write(key: PreferenceKey, value: string): void;
}

export function browserPreferenceStore(storage: Storage): PreferenceStore {
  return {
    read: (key): string | null => storage.getItem(key),
    write: (key, value): void => {
      storage.setItem(key, value);
    },
  };
}

export function browserPreferenceStoreOrMemory(): PreferenceStore {
  return availablePreferenceStore(() => window.localStorage);
}

export function availablePreferenceStore(readStorage: () => Storage): PreferenceStore {
  try {
    return browserPreferenceStore(readStorage());
  } catch (error) {
    if (error instanceof DOMException && error.name === 'SecurityError') {
      return memoryPreferenceStore();
    }
    throw error;
  }
}

export function memoryPreferenceStore(initial: Partial<Record<PreferenceKey, string>> = {}): PreferenceStore {
  const values = new Map<PreferenceKey, string>(Object.entries(initial) as [PreferenceKey, string][]);
  return {
    read: (key): string | null => values.get(key) ?? null,
    write: (key, value): void => {
      values.set(key, value);
    },
  };
}
