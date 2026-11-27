import type { QueryClient } from '@tanstack/react-query';

import type { Scheduler } from '../../lib/api/session.ts';
import type { RealtimeEvent } from '../../lib/realtime/client.ts';
import { tutorHomeKey } from '../home/api.ts';
import { channelsKey, unreadChannelsKey } from './keys.ts';

export type HistoryIntent =
  | { readonly kind: 'full' }
  | { readonly kind: 'new'; readonly seq: number }
  | { readonly kind: 'changed'; readonly messageId: string };

interface HistoryOwner {
  readonly changed: (intent: HistoryIntent) => void;
  readonly flush: () => void;
  readonly cancel: () => void;
  readonly suspend: () => void;
  readonly resume: () => void;
}

interface ChatRefetch {
  readonly changed: (channelId?: string, event?: Extract<RealtimeEvent, { readonly kind: 'message' }>) => void;
  readonly pinChanged: (channelId: string) => void;
  readonly read: () => void;
  readonly mount: (channelId: string, owner: HistoryOwner) => () => void;
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
  const owners = new Map<string, HistoryOwner>();
  let suspended = false;
  const schedule = (): void => {
    cancelTimer ??= scheduler.schedule(1000, () => {
      cancelTimer = null;
      const changed = new Set(pending);
      pending.clear();
      for (const owner of owners.values()) owner.flush();
      void queryClient.invalidateQueries({
        predicate: (query) =>
          changed.has(query.queryHash) ||
          query.queryKey[0] === unreadChannelsKey[0] ||
          (query.queryKey[0] === tutorHomeKey[0] && query.queryKey[1] === tutorHomeKey[1]) ||
          (query.queryKey[0] === channelsKey[0] && query.queryKey.at(-1) === null),
      });
    });
  };
  const coalescer: ChatRefetch = {
    cancel: (): void => {
      cancelTimer?.();
      cancelTimer = null;
      pending.clear();
      for (const owner of owners.values()) owner.cancel();
    },
    suspend: (): void => {
      suspended = true;
      for (const owner of owners.values()) owner.suspend();
      coalescer.cancel();
    },
    resume: (): void => {
      suspended = false;
      for (const owner of owners.values()) owner.resume();
    },
    mount: (channelId, owner): (() => void) => {
      if (owners.has(channelId)) throw new Error('a mounted channel has one history owner');
      owners.set(channelId, owner);
      if (suspended) owner.suspend();
      else owner.resume();
      return (): void => {
        if (owners.get(channelId) === owner) owners.delete(channelId);
        owner.suspend();
      };
    },
    read: (): void => {
      if (!suspended) schedule();
    },
    pinChanged: (channelId): void => {
      if (suspended) return;
      for (const query of queryClient.getQueryCache().getAll()) {
        if (query.queryKey[0] === 'channel' && query.queryKey[1] === channelId) pending.add(query.queryHash);
      }
      schedule();
    },
    changed: (channelId, event): void => {
      if (suspended) return;
      if (channelId !== undefined) {
        const owner = owners.get(channelId);
        owner?.changed(
          event === undefined
            ? { kind: 'full' }
            : event.change === 'new'
              ? { kind: 'new', seq: event.seq }
              : { kind: 'changed', messageId: event.messageId },
        );
        for (const query of queryClient.getQueryCache().getAll()) {
          if (
            (query.queryKey[0] === 'channel' ||
              query.queryKey[0] === 'messages-around' ||
              (query.queryKey[0] === 'messages' && owner === undefined)) &&
            query.queryKey[1] === channelId
          )
            pending.add(query.queryHash);
        }
      }
      schedule();
    },
  };
  coalescers.set(queryClient, coalescer);
  return coalescer;
}

export function scheduleChatReads(queryClient: QueryClient, scheduler: Scheduler): void {
  chatRefetch(queryClient, scheduler).read();
}
