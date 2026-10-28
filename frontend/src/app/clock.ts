import { createContext, useCallback, useContext, useEffect, useState, useSyncExternalStore } from 'react';

import { type Scheduler, browserScheduler } from '../lib/api/session.ts';
import { type LocalDate, addDays, localDateOf, localMidnight } from '../lib/time/localDates.ts';

export const SchedulerContext = createContext<Scheduler>(browserScheduler);

export function useScheduler(): Scheduler {
  return useContext(SchedulerContext);
}

export function watchPageReturns(onReturn: () => void): () => void {
  document.addEventListener('visibilitychange', onReturn);
  window.addEventListener('pageshow', onReturn);
  return (): void => {
    document.removeEventListener('visibilitychange', onReturn);
    window.removeEventListener('pageshow', onReturn);
  };
}

function untilNextLocalMidnight(scheduler: Scheduler, timeZone: string): number {
  const now = scheduler.wallClock();
  return localMidnight(addDays(localDateOf(now, timeZone), 1), timeZone) - now;
}

export function useLocalToday(timeZone: string): LocalDate {
  const scheduler = useScheduler();
  const subscribe = useCallback(
    (onChange: () => void): (() => void) => {
      let cancel = (): void => undefined;
      const arm = (): void => {
        cancel = scheduler.schedule(untilNextLocalMidnight(scheduler, timeZone), () => {
          onChange();
          arm();
        });
      };
      const stopWatching = watchPageReturns(() => {
        cancel();
        onChange();
        arm();
      });
      arm();
      return (): void => {
        cancel();
        stopWatching();
      };
    },
    [scheduler, timeZone],
  );
  const snapshot = useCallback(() => localDateOf(scheduler.wallClock(), timeZone), [scheduler, timeZone]);
  return useSyncExternalStore(subscribe, snapshot);
}

export function useDebouncedValue<Value>(value: Value, milliseconds: number): Value {
  const scheduler = useScheduler();
  const [settled, setSettled] = useState(value);

  useEffect(
    () =>
      scheduler.schedule(milliseconds, () => {
        setSettled(value);
      }),
    [scheduler, value, milliseconds],
  );

  return settled;
}
