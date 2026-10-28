import { useEffect, useState } from 'react';

import type { LessonDetail } from './api.ts';

export type LessonPhase = 'before' | 'window' | 'past' | 'live' | 'ended' | 'cancelled';

const joinWindowMilliseconds = 10 * 60_000;
const longestTimer = 2_147_483_647;

interface LessonTimes {
  readonly status: LessonDetail['status'];
  readonly starts_at: string;
  readonly ends_at: string;
}

function boundaries(lesson: LessonTimes): readonly number[] {
  const start = Date.parse(lesson.starts_at);
  return [start - joinWindowMilliseconds, start, Date.parse(lesson.ends_at)];
}

export function roomOpensAt(lesson: LessonTimes): number {
  return Date.parse(lesson.starts_at) - joinWindowMilliseconds;
}

export function phaseOf(lesson: LessonTimes, now: number): LessonPhase {
  if (lesson.status !== 'scheduled') {
    return lesson.status;
  }
  const [windowOpens = 0, , end = 0] = boundaries(lesson);
  if (now < windowOpens) {
    return 'before';
  }
  return now < end ? 'window' : 'past';
}

export function isEditable(lesson: LessonTimes, now: number): boolean {
  return lesson.status === 'scheduled' && now < Date.parse(lesson.starts_at);
}

export function dialogOutdated(
  opened: LessonTimes & { readonly version: number },
  current: (LessonTimes & { readonly version: number }) | undefined,
  now: number,
): boolean {
  return current?.version !== opened.version || current.status !== opened.status || !isEditable(current, now);
}

export function useLessonClock(lesson: LessonTimes | undefined): number {
  const [now, setNow] = useState(() => Date.now());
  const status = lesson?.status;
  const startsAt = lesson?.starts_at;
  const endsAt = lesson?.ends_at;

  useEffect(() => {
    const upcoming =
      status === 'scheduled' && startsAt !== undefined && endsAt !== undefined
        ? boundaries({ status, starts_at: startsAt, ends_at: endsAt })
        : [];
    let handle = 0;
    const refresh = (): void => {
      const current = Date.now();
      setNow(current);
      const next = upcoming.find((moment) => moment > current);
      if (next !== undefined) {
        handle = window.setTimeout(refresh, Math.min(next - current, longestTimer));
      }
    };
    handle = window.setTimeout(refresh, 0);
    return (): void => {
      window.clearTimeout(handle);
    };
  }, [status, startsAt, endsAt]);

  return now;
}
