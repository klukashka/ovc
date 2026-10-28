import type { Language } from '../i18n/resources.ts';
import table from './timezone-cities.json' with { type: 'json' };

export type CityNames = Readonly<Record<Language, string>>;

const cities: ReadonlyMap<string, CityNames> = new Map(Object.entries(table));

export function cityOf(timeZone: string, language: Language): string | null {
  return cities.get(timeZone)?.[language] ?? null;
}
