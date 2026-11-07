import { describe, expect, test } from 'vitest';

import { cityNameOf, cityOf } from './timezoneCities.ts';

describe('timezone city table', () => {
  test('every zone the engine reports has an English and a Russian city', () => {
    const zones = Intl.supportedValuesOf('timeZone');

    const missing = zones.filter((zone) => cityOf(zone, 'en') === null || cityOf(zone, 'ru') === null);

    expect(zones.length).toBeGreaterThan(400);
    expect(missing).toEqual([]);
  });

  test('every zone the engine reports has a city name to list it by in both languages', () => {
    const zones = Intl.supportedValuesOf('timeZone');

    const missing = zones.filter((zone) => !cityNameOf(zone, 'en') || !cityNameOf(zone, 'ru'));

    expect(missing).toEqual([]);
    expect(cityNameOf('Asia/Novosibirsk', 'ru')).toBe('Новосибирск');
    expect(cityOf('Asia/Novosibirsk', 'ru')).toBe('Новосибирска');
  });
});
