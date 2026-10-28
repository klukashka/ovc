import { describe, expect, test } from 'vitest';

import { dialogOutdated, isEditable, phaseOf } from './lessonTiming.ts';

const minute = 60_000;
const start = Date.parse('2026-11-03T15:00:00Z');
const scheduled = {
  status: 'scheduled' as const,
  starts_at: '2026-11-03T15:00:00Z',
  ends_at: '2026-11-03T16:00:00Z',
  version: 3,
};

describe('lesson timing', () => {
  test('a scheduled lesson changes its phase at the window, and stays scheduled until its end', () => {
    const moments = [
      start - 10 * minute - 1,
      start - 10 * minute,
      start - 1,
      start,
      start + 60 * minute - 1,
      start + 60 * minute,
    ];

    const phases = moments.map((moment) => phaseOf(scheduled, moment));

    expect(phases).toEqual(['before', 'window', 'window', 'window', 'window', 'past']);
  });

  test('a lesson is editable only while scheduled and before its start', () => {
    const cancelled = { ...scheduled, status: 'cancelled' as const };

    const editable = [
      isEditable(scheduled, start - 1),
      isEditable(scheduled, start),
      isEditable(cancelled, start - 60 * minute),
    ];

    expect(editable).toEqual([true, false, false]);
  });

  test('an open dialog is outdated by a new version, a new status, a missing lesson or its start passing', () => {
    const before = start - minute;

    const outdated = [
      dialogOutdated(scheduled, scheduled, before),
      dialogOutdated(scheduled, { ...scheduled, version: 4 }, before),
      dialogOutdated(scheduled, { ...scheduled, status: 'cancelled' }, before),
      dialogOutdated(scheduled, undefined, before),
      dialogOutdated(scheduled, scheduled, start),
    ];

    expect(outdated).toEqual([false, true, true, true, true]);
  });
});
