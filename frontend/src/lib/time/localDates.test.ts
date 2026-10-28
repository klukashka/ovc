import { describe, expect, test } from 'vitest';

import { localMidnight } from './localDates.ts';

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
