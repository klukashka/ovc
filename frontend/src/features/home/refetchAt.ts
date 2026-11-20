import { useEffect, useRef } from 'react';

import { useScheduler } from '../../app/clock.ts';

export interface HomeRefetch {
  readonly refetch: () => unknown;
  readonly loadedAt: number;
  readonly failedAt: number;
}

const instantMarginMilliseconds = 1_000;
const passedInstantRetryMilliseconds = 30_000;
const passedInstantRetryCapMilliseconds = 600_000;

interface InstantRefetch {
  readonly instant: string;
  readonly delay: number;
  readonly afterInstant: boolean;
}

function nextInstantRefetch(instant: string, now: number, previous: InstantRefetch | null): InstantRefetch {
  const due = Date.parse(instant) - now;
  if (due > 0) {
    return { instant, delay: due + instantMarginMilliseconds, afterInstant: false };
  }
  if (previous?.instant !== instant || !previous.afterInstant) {
    return { instant, delay: instantMarginMilliseconds, afterInstant: true };
  }
  const delay = Math.min(
    Math.max(previous.delay * 2, passedInstantRetryMilliseconds),
    passedInstantRetryCapMilliseconds,
  );
  return { instant, delay, afterInstant: true };
}

export function useRefetchAt(instant: string | null, { refetch, loadedAt, failedAt }: HomeRefetch): void {
  const scheduler = useScheduler();
  const previous = useRef<InstantRefetch | null>(null);
  useEffect(() => {
    if (instant === null) {
      previous.current = null;
      return undefined;
    }
    const next = nextInstantRefetch(instant, scheduler.wallClock(), previous.current);
    previous.current = next;
    return scheduler.schedule(next.delay, () => {
      void refetch();
    });
  }, [instant, loadedAt, failedAt, refetch, scheduler]);
}
