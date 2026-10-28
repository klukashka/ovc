import { act, cleanup, render, screen } from '@testing-library/react';
import type { ReactElement } from 'react';
import { afterEach, describe, expect, test } from 'vitest';

import { type ManualScheduler, manualScheduler } from '../lib/api/fakeTransport.ts';
import { SchedulerContext, useLocalToday } from './clock.ts';

afterEach(() => {
  cleanup();
});

const minute = 60_000;

function Today({ timeZone }: { readonly timeZone: string }): ReactElement {
  return <p>{useLocalToday(timeZone)}</p>;
}

function schedulerAt(instant: string): ManualScheduler {
  const scheduler = manualScheduler();
  scheduler.suspend(Date.parse(instant));
  return scheduler;
}

function showToday(scheduler: ManualScheduler, timeZone: string): void {
  render(
    <SchedulerContext value={scheduler}>
      <Today timeZone={timeZone} />
    </SchedulerContext>,
  );
}

describe('the local today', () => {
  test('turns to the next day at local midnight in the viewer’s timezone, not in UTC', () => {
    const scheduler = schedulerAt('2026-11-03T20:59:30Z');
    showToday(scheduler, 'Europe/Moscow');
    const before = screen.getByRole('paragraph').textContent;

    act(() => {
      scheduler.pass(30_000);
    });

    expect(before).toBe('2026-11-03');
    expect(screen.getByRole('paragraph').textContent).toBe('2026-11-04');
  });

  test('stays on the same day until local midnight', () => {
    const scheduler = schedulerAt('2026-11-03T20:58:00Z');
    showToday(scheduler, 'Europe/Moscow');

    act(() => {
      scheduler.pass(minute);
    });

    expect(screen.getByRole('paragraph').textContent).toBe('2026-11-03');
  });

  test.each([
    {
      day: '2026-03-29',
      next: '2026-03-30',
      startedAt: '2026-03-28T23:00:30Z',
      nextMidnight: '2026-03-29T22:00:00Z',
      hours: 23,
    },
    {
      day: '2026-10-25',
      next: '2026-10-26',
      startedAt: '2026-10-24T22:00:30Z',
      nextMidnight: '2026-10-25T23:00:00Z',
      hours: 25,
    },
  ])(
    'turns after the $hours-hour day $day in Berlin exactly at its local midnight',
    ({ day, next, startedAt, nextMidnight }) => {
      const scheduler = schedulerAt(startedAt);
      showToday(scheduler, 'Europe/Berlin');
      const untilMidnight = Date.parse(nextMidnight) - Date.parse(startedAt);
      act(() => {
        scheduler.pass(untilMidnight - 1_000);
      });
      const secondBefore = screen.getByRole('paragraph').textContent;

      act(() => {
        scheduler.pass(1_000);
      });

      expect(secondBefore).toBe(day);
      expect(screen.getByRole('paragraph').textContent).toBe(next);
    },
  );

  test.each([
    { event: 'visibilitychange', target: (): EventTarget => document },
    { event: 'pageshow', target: (): EventTarget => window },
  ])('is read again on $event after the computer slept past midnight', ({ event, target }) => {
    const scheduler = schedulerAt('2026-11-03T20:00:00Z');
    showToday(scheduler, 'Europe/Moscow');
    scheduler.suspend(120 * minute);

    act(() => {
      target().dispatchEvent(new Event(event));
    });

    expect(screen.getByRole('paragraph').textContent).toBe('2026-11-04');
  });
});
