import type { QueryClient } from '@tanstack/react-query';

import type { Scheduler } from '../../lib/api/session.ts';
import { channelsKey, unreadChannelsKey } from './keys.ts';

interface ChatRefetch {
  readonly changed: (channelId?: string) => void;
  readonly cancel: () => void;
  readonly suspend: () => void;
  readonly resume: () => void;
}

const coalescers = new WeakMap<QueryClient, ChatRefetch>();

export function chatRefetch(queryClient: QueryClient, scheduler: Scheduler): ChatRefetch {
  const existing = coalescers.get(queryClient);
  if (existing !== undefined) return existing;
  let cancelTimer: (() => void) | null = null;
  const pending = new Set<string>();
  let suspended = false;
  const coalescer: ChatRefetch = {
    cancel: (): void => {
      cancelTimer?.();
      cancelTimer = null;
      pending.clear();
    },
    suspend: (): void => {
      suspended = true;
      coalescer.cancel();
    },
    resume: (): void => {
      suspended = false;
    },
    changed: (channelId): void => {
      if (suspended) return;
      if (channelId !== undefined) {
        for (const query of queryClient.getQueryCache().getAll()) {
          if (
            (query.queryKey[0] === 'channel' || query.queryKey[0] === 'messages') &&
            query.queryKey[1] === channelId
          ) {
            pending.add(query.queryHash);
          }
        }
      }
      cancelTimer ??= scheduler.schedule(1000, () => {
        cancelTimer = null;
        const changed = new Set(pending);
        pending.clear();
        void queryClient.invalidateQueries({
          predicate: (query) =>
            changed.has(query.queryHash) ||
            query.queryKey[0] === unreadChannelsKey[0] ||
            (query.queryKey[0] === channelsKey[0] && query.queryKey.at(-1) === null),
        });
      });
    },
  };
  coalescers.set(queryClient, coalescer);
  return coalescer;
}

export function scheduleChatReads(queryClient: QueryClient, scheduler: Scheduler): void {
  chatRefetch(queryClient, scheduler).changed();
}
