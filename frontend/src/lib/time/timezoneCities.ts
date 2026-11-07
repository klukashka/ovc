import type { Language } from '../i18n/resources.ts';
import table from './timezone-cities.json' with { type: 'json' };

interface CityEntry {
  readonly en: string;
  readonly ru: string;
  readonly ruNominative: string;
}

const cities: ReadonlyMap<string, CityEntry> = new Map(Object.entries(table));

export function cityOf(timeZone: string, language: Language): string | null {
  return cities.get(timeZone)?.[language] ?? null;
}

export function cityNameOf(timeZone: string, language: Language): string | null {
  const entry = cities.get(timeZone);
  if (entry === undefined) {
    return null;
  }
  return language === 'ru' ? entry.ruNominative : entry.en;
}
