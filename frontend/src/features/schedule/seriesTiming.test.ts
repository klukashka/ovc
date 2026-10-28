import { describe, expect, test } from 'vitest';

import { seriesTimingIn } from './seriesTiming.ts';

describe('series timing in the tutor profile timezone', () => {
  test('the same zone keeps the date, start and weekdays', () => {
    const rule = { start: '18:00', weekdays: [2, 4], timezone: 'Europe/Moscow' };

    const timing = seriesTimingIn(rule, '2026-11-03', 'Europe/Moscow');

    expect(timing).toEqual({ date: '2026-11-03', start: '18:00', weekdays: [2, 4] });
  });

  test('a start that falls on the previous day moves the date and every weekday back, Monday to Sunday', () => {
    const rule = { start: '01:00', weekdays: [1, 3], timezone: 'Europe/Moscow' };

    const timing = seriesTimingIn(rule, '2026-11-02', 'Europe/London');

    expect(timing).toEqual({ date: '2026-11-01', start: '22:00', weekdays: [2, 7] });
  });

  test('a start that falls on the next day moves the date and every weekday forward, Sunday to Monday', () => {
    const rule = { start: '20:30', weekdays: [5, 7], timezone: 'America/New_York' };

    const timing = seriesTimingIn(rule, '2026-11-08', 'Asia/Tokyo');

    expect(timing).toEqual({ date: '2026-11-09', start: '10:30', weekdays: [1, 6] });
  });

  test('the offset is the one at the occurrence, across a daylight saving change of the series zone', () => {
    const rule = { start: '18:00', weekdays: [7], timezone: 'Europe/Berlin' };

    const summer = seriesTimingIn(rule, '2026-10-18', 'Europe/Moscow');
    const winter = seriesTimingIn(rule, '2026-11-01', 'Europe/Moscow');

    expect([summer.start, winter.start]).toEqual(['19:00', '20:00']);
  });
});
