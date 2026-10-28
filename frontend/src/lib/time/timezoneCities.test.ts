import { describe, expect, test } from 'vitest';

import { cityOf } from './timezoneCities.ts';

describe('timezone city table', () => {
  test('every zone the engine reports has an English and a Russian city', () => {
    const zones = Intl.supportedValuesOf('timeZone');

    const missing = zones.filter((zone) => cityOf(zone, 'en') === null || cityOf(zone, 'ru') === null);

    expect(zones.length).toBeGreaterThan(400);
    expect(missing).toEqual([]);
  });
});
