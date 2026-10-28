import { describe, expect, test } from 'vitest';

import { localInstant, localMidnight, utcOffsetText } from './localDates.ts';

function midnightsAround(dates: readonly `${number}-${number}-${number}`[], timeZone: string): string[] {
  return dates.map((date) => new Date(localMidnight(date, timeZone)).toISOString());
}

describe('range helper', () => {
  test('local midnights on both sides of a change', () => {
    const springForward = ['2026-03-28', '2026-03-29', '2026-03-30'] as const;
    const fallBack = ['2026-10-24', '2026-10-25', '2026-10-26'] as const;
    const midnightGap = ['2026-03-07', '2026-03-08', '2026-03-09'] as const;

    const berlinSpring = midnightsAround(springForward, 'Europe/Berlin');
    const berlinAutumn = midnightsAround(fallBack, 'Europe/Berlin');
    const havana = midnightsAround(midnightGap, 'America/Havana');

    expect(berlinSpring).toEqual(['2026-03-27T23:00:00.000Z', '2026-03-28T23:00:00.000Z', '2026-03-29T22:00:00.000Z']);
    expect(berlinAutumn).toEqual(['2026-10-23T22:00:00.000Z', '2026-10-24T22:00:00.000Z', '2026-10-25T23:00:00.000Z']);
    expect(havana).toEqual(['2026-03-07T05:00:00.000Z', '2026-03-08T05:00:00.000Z', '2026-03-09T04:00:00.000Z']);
  });
});

describe('local instant', () => {
  test('a local time skipped by clocks jumping forward is shifted forward by the gap, east and west of UTC', () => {
    const skipped = [
      localInstant('2026-03-29', 150, 'Europe/Berlin'),
      localInstant('2026-03-08', 150, 'America/New_York'),
      localInstant('2026-10-04', 135, 'Australia/Lord_Howe'),
    ];

    expect(skipped.map((instant) => new Date(instant).toISOString())).toEqual([
      '2026-03-29T01:30:00.000Z',
      '2026-03-08T07:30:00.000Z',
      '2026-10-03T15:45:00.000Z',
    ]);
  });

  test('a local time repeated by clocks falling back takes the earlier instant, east and west of UTC', () => {
    const repeated = [
      localInstant('2026-10-25', 150, 'Europe/Berlin'),
      localInstant('2026-11-01', 90, 'America/New_York'),
    ];

    expect(repeated.map((instant) => new Date(instant).toISOString())).toEqual([
      '2026-10-25T00:30:00.000Z',
      '2026-11-01T05:30:00.000Z',
    ]);
  });

  test('an ordinary local time maps to its one instant', () => {
    const instant = localInstant('2026-11-03', 18 * 60, 'Europe/Moscow');

    expect(new Date(instant).toISOString()).toBe('2026-11-03T15:00:00.000Z');
  });
});

describe('offset text', () => {
  test('a zone without a city reads as its offset at the instant, half hours and daylight saving included', () => {
    const winter = Date.parse('2026-01-15T12:00:00Z');
    const summer = Date.parse('2026-07-15T12:00:00Z');

    const offsets = [
      utcOffsetText(winter, 'UTC'),
      utcOffsetText(winter, 'Asia/Kolkata'),
      utcOffsetText(winter, 'America/St_Johns'),
      utcOffsetText(summer, 'America/St_Johns'),
      utcOffsetText(summer, 'Pacific/Chatham'),
    ];

    expect(offsets).toEqual(['UTC+00:00', 'UTC+05:30', 'UTC-03:30', 'UTC-02:30', 'UTC+12:45']);
  });
});
