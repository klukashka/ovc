import { type RefObject, useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';

import { useScheduler } from '../../app/clock.ts';
import { useApiClient } from '../../lib/api/client.ts';
import { markRead } from './api.ts';
import { ReadTracker, type ReadView } from './ReadTracker.ts';
import { scheduleChatReads } from './refetch.ts';

function visibleMessages(list: RefObject<HTMLOListElement | null>): ReadView {
  let hidden = false;
  return {
    shownSequence: (): number => {
      if (hidden || document.visibilityState !== 'visible') return 0;
      let shown = 0;
      for (const row of list.current?.querySelectorAll<HTMLElement>('[data-seq]') ?? []) {
        const box = row.getBoundingClientRect();
        if (
          box.height > 0 &&
          box.bottom > 0 &&
          box.top < window.innerHeight &&
          box.right > 0 &&
          box.left < window.innerWidth
        ) {
          shown = Math.max(shown, Number(row.dataset.seq));
        }
      }
      return shown;
    },
    subscribe: (changed): (() => void) => {
      function hide(): void {
        hidden = true;
        changed();
      }
      function show(): void {
        hidden = false;
        changed();
      }
      document.addEventListener('scroll', changed, true);
      document.addEventListener('visibilitychange', changed);
      window.addEventListener('resize', changed);
      window.addEventListener('pagehide', hide);
      window.addEventListener('pageshow', show);
      return (): void => {
        document.removeEventListener('scroll', changed, true);
        document.removeEventListener('visibilitychange', changed);
        window.removeEventListener('resize', changed);
        window.removeEventListener('pagehide', hide);
        window.removeEventListener('pageshow', show);
      };
    },
  };
}

export function useReadMarkers(
  id: string,
  list: RefObject<HTMLOListElement | null>,
  revision: string,
): { readonly failed: boolean; readonly retry: () => void } {
  const scheduler = useScheduler();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const tracker = useRef<ReadTracker | null>(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    const current = new ReadTracker(
      scheduler,
      visibleMessages(list),
      { mark: (seq, signal): Promise<void> => markRead(client, id, seq, signal) },
      {
        read: (): void => {
          setFailed(false);
          scheduleChatReads(queryClient, scheduler);
        },
        failed: (): void => {
          setFailed(true);
        },
      },
    );
    tracker.current = current;
    return (): void => {
      current.stop();
      tracker.current = null;
    };
  }, [client, id, list, scheduler, queryClient]);
  useEffect(() => {
    tracker.current?.refresh();
  }, [revision]);
  return {
    failed,
    retry: (): void => {
      tracker.current?.retry();
    },
  };
}
