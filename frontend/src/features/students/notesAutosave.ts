import { useEffect, useRef, useState } from 'react';

import { useScheduler } from '../../app/clock.ts';

export const autosaveDelay = 1000;

export type AutosaveStatus = 'idle' | 'editing' | 'saving' | 'saved' | 'failed';

export interface Autosave {
  readonly status: AutosaveStatus;
  readonly unsaved: boolean;
  readonly changed: (text: string) => void;
  readonly left: () => void;
  readonly retry: () => void;
}

function keptWhileTyping(status: AutosaveStatus): AutosaveStatus {
  return status === 'saving' || status === 'failed' ? status : 'editing';
}

export function useAutosave(save: (text: string) => Promise<boolean>): Autosave {
  const scheduler = useScheduler();
  const [status, setStatus] = useState<AutosaveStatus>('idle');
  const cancelTimer = useRef<() => void>(() => undefined);
  const latest = useRef('');
  const dirty = useRef(false);
  const sending = useRef(false);
  const queued = useRef(false);
  const mounted = useRef(true);
  const typedSinceFailure = useRef(false);

  useEffect(() => {
    mounted.current = true;
    return (): void => {
      mounted.current = false;
      cancelTimer.current();
    };
  }, []);

  function stopTimer(): void {
    cancelTimer.current();
    cancelTimer.current = (): void => undefined;
  }

  function settle(saved: boolean): void {
    sending.current = false;
    if (!mounted.current) {
      return;
    }
    if (!saved) {
      dirty.current = true;
      queued.current = false;
      typedSinceFailure.current = false;
      setStatus('failed');
      return;
    }
    if (!dirty.current) {
      setStatus('saved');
      return;
    }
    if (queued.current) {
      send();
      return;
    }
    setStatus('editing');
  }

  function send(): void {
    const text = latest.current;
    dirty.current = false;
    queued.current = false;
    sending.current = true;
    setStatus('saving');
    save(text).then(settle, () => {
      settle(false);
    });
  }

  function flush(): void {
    stopTimer();
    if (!dirty.current) {
      return;
    }
    if (sending.current) {
      queued.current = true;
      return;
    }
    send();
  }

  return {
    status,
    unsaved: status === 'editing' || status === 'saving' || status === 'failed',
    changed: (text: string): void => {
      latest.current = text;
      dirty.current = true;
      typedSinceFailure.current = true;
      setStatus(keptWhileTyping);
      stopTimer();
      cancelTimer.current = scheduler.schedule(autosaveDelay, flush);
    },
    left: (): void => {
      if (status !== 'failed' || typedSinceFailure.current) {
        flush();
      }
    },
    retry: (): void => {
      dirty.current = true;
      flush();
    },
  };
}
