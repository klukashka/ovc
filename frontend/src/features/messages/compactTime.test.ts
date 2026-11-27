import { expect, test } from 'vitest';

import { compactTime } from './compactTime.ts';

test.each([
  {
    name: 'today',
    instant: '2026-11-27T09:05:00Z',
    now: '2026-11-27T20:00:00Z',
    zone: 'Europe/Moscow',
    expected: '12:05',
  },
  {
    name: 'yesterday across local midnight',
    instant: '2026-11-26T20:59:00Z',
    now: '2026-11-26T21:01:00Z',
    zone: 'Europe/Moscow',
    expected: 'Thu',
  },
  {
    name: 'six days back',
    instant: '2026-11-21T09:00:00Z',
    now: '2026-11-27T09:00:00Z',
    zone: 'Europe/Moscow',
    expected: 'Sat',
  },
  {
    name: 'seven days back',
    instant: '2026-11-20T09:00:00Z',
    now: '2026-11-27T09:00:00Z',
    zone: 'Europe/Moscow',
    expected: '20 Nov',
  },
  {
    name: 'the previous year',
    instant: '2025-12-20T09:00:00Z',
    now: '2026-01-02T09:00:00Z',
    zone: 'Europe/Moscow',
    expected: '20 Dec 2025',
  },
  {
    name: 'a day different from UTC',
    instant: '2026-11-26T23:05:00Z',
    now: '2026-11-27T02:00:00Z',
    zone: 'America/Los_Angeles',
    expected: '15:05',
  },
  {
    name: 'six days across daylight saving',
    instant: '2026-03-24T12:00:00Z',
    now: '2026-03-30T11:00:00Z',
    zone: 'Europe/Berlin',
    expected: 'Tue',
  },
])('$name', ({ instant, now, zone, expected }) => {
  const current = Date.parse(now);

  const result = compactTime(instant, 'en', zone, current);

  expect(result).toBe(expected);
});
