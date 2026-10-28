import { act, cleanup, render, screen } from '@testing-library/react';
import type { ReactElement } from 'react';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { type ManualScheduler, manualScheduler } from '../../lib/api/fakeTransport.ts';
import { phaseOf, useLessonClock } from './lessonTiming.ts';

afterEach(() => {
  cleanup();
});

const minute = 60_000;

const lesson = {
  status: 'scheduled',
  starts_at: '2026-11-03T15:00:00Z',
  ends_at: '2026-11-03T16:00:00Z',
} as const;

function Phase(): ReactElement {
  return <p>{phaseOf(lesson, useLessonClock(lesson))}</p>;
}

function showPhaseBeforeStart(minutesBefore: number): ManualScheduler {
  const scheduler = manualScheduler();
  scheduler.suspend(Date.parse(lesson.starts_at) - minutesBefore * minute);
  render(
    <SchedulerContext value={scheduler}>
      <Phase />
    </SchedulerContext>,
  );
  act(() => {
    scheduler.pass(0);
  });
  return scheduler;
}

describe('the lesson clock', () => {
  test('moves to the join window at its boundary while the page stays open', () => {
    const scheduler = showPhaseBeforeStart(20);

    act(() => {
      scheduler.pass(10 * minute);
    });

    expect(screen.getByRole('paragraph').textContent).toBe('window');
  });

  test.each([
    { event: 'visibilitychange', target: (): EventTarget => document },
    { event: 'pageshow', target: (): EventTarget => window },
  ])('is read again on $event after the computer slept past a boundary', ({ event, target }) => {
    const scheduler = showPhaseBeforeStart(20);
    scheduler.suspend(15 * minute);
    const asleep = screen.getByRole('paragraph').textContent;

    act(() => {
      target().dispatchEvent(new Event(event));
    });

    expect(asleep).toBe('before');
    expect(screen.getByRole('paragraph').textContent).toBe('window');
  });
});
