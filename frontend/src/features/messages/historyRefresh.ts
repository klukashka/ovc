import type { QueryClient } from '@tanstack/react-query';

import { asApiError, type ApiClient } from '../../lib/api/client.ts';
import { fetchMessages, fetchMessagesAfter, type Message, type MessagePage } from './api.ts';
import { channelScopeRevision } from './cache.ts';
import { messagesKey } from './keys.ts';
import type { HistoryIntent } from './refetch.ts';

const maximumDirtyMessages = 100;

interface PendingHistory {
  full: boolean;
  through: number;
  fresh: boolean;
  readonly changed: Set<string>;
}

function emptyPending(): PendingHistory {
  return { full: false, through: 0, fresh: false, changed: new Set() };
}

function cancelled(): DOMException {
  return new DOMException('history ownership ended', 'AbortError');
}

export class HistoryRefresh {
  private tail: Promise<void> = Promise.resolve();
  private pending = emptyPending();
  private ready = false;
  private draining = false;
  private generation = 0;
  private fullReads = 0;
  private fullRequired = false;
  private suspended = false;
  private incremental: AbortController | null = null;

  private readonly cache: QueryClient;
  private readonly client: ApiClient;
  private readonly channelId: string;
  private readonly current: () => boolean;
  private readonly unavailable: () => void;
  private readonly requestFull: () => void;

  constructor(
    cache: QueryClient,
    client: ApiClient,
    channelId: string,
    current: () => boolean,
    unavailable: () => void,
    requestFull: () => void,
  ) {
    this.cache = cache;
    this.client = client;
    this.channelId = channelId;
    this.current = current;
    this.unavailable = unavailable;
    this.requestFull = requestFull;
  }

  private quotes(messageId: string): boolean {
    return (
      this.cache
        .getQueryData<MessagePage>(messagesKey(this.channelId))
        ?.items.some((item) => item.reply_to?.message_id === messageId) ?? false
    );
  }

  changed(intent: HistoryIntent): void {
    if (intent.kind === 'full') this.pending.full = true;
    else if (intent.kind === 'new') {
      this.pending.through = Math.max(this.pending.through, intent.seq);
      this.pending.fresh ||= this.fullReads > 0 || this.incremental !== null;
    } else if (this.quotes(intent.messageId)) {
      this.pending.full = true;
      this.pending.changed.clear();
    } else if (!this.pending.full) {
      this.pending.changed.add(intent.messageId);
      if (this.pending.changed.size > maximumDirtyMessages) {
        this.pending.full = true;
        this.pending.changed.clear();
      }
    }
  }

  cancel(): void {
    this.generation += 1;
    this.pending = emptyPending();
    this.ready = false;
    this.incremental?.abort();
    this.incremental = null;
    this.scheduleFull();
  }

  suspend(): void {
    this.suspended = true;
    this.cancel();
  }

  resume(): void {
    this.suspended = false;
    this.scheduleFull();
  }

  private scheduleFull(): void {
    if (this.fullRequired && this.fullReads === 0 && !this.suspended && this.current()) this.requestFull();
  }

  flush(): void {
    if (this.suspended) return;
    if (!this.pending.full && this.pending.through === 0 && this.pending.changed.size === 0) return;
    this.ready = true;
    if (!this.draining) void this.drain();
  }

  private async acquire(): Promise<() => void> {
    const previous = this.tail;
    let release = (): void => {
      throw new Error('history lease is not initialized');
    };
    this.tail = new Promise<void>((resolve) => {
      release = resolve;
    });
    await previous;
    return release;
  }

  private releaseAfterPublication(signal: AbortSignal, release: () => void): void {
    const query = this.cache.getQueryCache().find({ queryKey: messagesKey(this.channelId), exact: true });
    let unsubscribe = (): void => undefined;
    let finished = false;
    const finish = (): void => {
      if (finished) return;
      finished = true;
      unsubscribe();
      signal.removeEventListener('abort', finish);
      release();
    };
    const settled = (): void => {
      const current = this.cache.getQueryCache().find({ queryKey: messagesKey(this.channelId), exact: true });
      if (current !== query || current?.state.fetchStatus !== 'fetching') finish();
    };
    unsubscribe = this.cache.getQueryCache().subscribe(settled);
    signal.addEventListener('abort', finish, { once: true });
    if (signal.aborted) finish();
    else settled();
  }

  async full(signal: AbortSignal, oldest: () => number | null): Promise<MessagePage> {
    this.fullReads += 1;
    this.pending = emptyPending();
    this.ready = false;
    const release = await this.acquire();
    const revision = channelScopeRevision(this.cache);
    try {
      signal.throwIfAborted();
      if (!this.current()) throw cancelled();
      this.fullRequired = false;
      const floor = this.cache.getQueryData<MessagePage>(messagesKey(this.channelId))?.items.at(-1)?.seq ?? oldest();
      const items: Message[] = [];
      let cursor: string | null = null;
      let peerReadSeq: number | null = null;
      do {
        const page = await fetchMessages(this.client, this.channelId, cursor, signal);
        if (!this.current()) throw cancelled();
        items.push(...page.items);
        peerReadSeq ??= page.peer_read_seq;
        cursor = page.next_cursor;
        if ((items.at(-1)?.seq ?? 0) <= Math.min(floor ?? Infinity, oldest() ?? Infinity)) break;
      } while (cursor !== null);
      signal.throwIfAborted();
      if (!this.current()) throw cancelled();
      if (channelScopeRevision(this.cache) !== revision) {
        this.fullRequired = true;
        throw cancelled();
      }
      return { items, next_cursor: cursor, has_newer: false, peer_read_seq: peerReadSeq };
    } finally {
      this.releaseAfterPublication(signal, () => {
        this.fullReads -= 1;
        release();
        this.scheduleFull();
      });
    }
  }

  async older(signal: AbortSignal): Promise<MessagePage | null> {
    const release = await this.acquire();
    const revision = channelScopeRevision(this.cache);
    try {
      signal.throwIfAborted();
      if (!this.current()) throw cancelled();
      const previous = this.cache.getQueryData<MessagePage>(messagesKey(this.channelId));
      if (previous?.next_cursor == null) return null;
      const page = await fetchMessages(this.client, this.channelId, previous.next_cursor, signal);
      signal.throwIfAborted();
      if (!this.current() || channelScopeRevision(this.cache) !== revision) throw cancelled();
      const ids = new Set(previous.items.map((item) => item.id));
      const merged = {
        items: [...previous.items, ...page.items.filter((item) => !ids.has(item.id))],
        next_cursor: page.next_cursor,
        has_newer: false,
        peer_read_seq: page.peer_read_seq,
      };
      this.cache.setQueryData<MessagePage>(messagesKey(this.channelId), merged);
      return merged;
    } finally {
      release();
    }
  }

  private async incrementalPage(intent: PendingHistory, signal: AbortSignal): Promise<MessagePage | null> {
    const previous = this.cache.getQueryData<MessagePage>(messagesKey(this.channelId));
    if (previous === undefined || intent.full) return null;
    const replacements = new Map<string, Message>();
    let after = previous.items.reduce((maximum, item) => Math.max(maximum, item.seq), 0);
    let peerReadSeq = previous.peer_read_seq;
    let fresh = intent.fresh;
    while (after < intent.through || fresh) {
      fresh = false;
      const page = await fetchMessagesAfter(this.client, this.channelId, after, maximumDirtyMessages, signal);
      if (page.next_cursor !== null) return null;
      peerReadSeq = page.peer_read_seq;
      if (page.items.length === 0) {
        if (after < intent.through) return null;
        break;
      }
      for (const item of page.items) {
        if (item.seq !== after + 1) return null;
        replacements.set(item.id, item);
        after = item.seq;
      }
    }
    for (const id of intent.changed) {
      const known = previous.items.find((item) => item.id === id) ?? replacements.get(id);
      if (known === undefined) continue;
      const page = await fetchMessagesAfter(this.client, this.channelId, known.seq - 1, 1, signal);
      const fresh = page.items[0];
      if (page.items.length !== 1 || fresh?.id !== id || fresh.seq !== known.seq || page.next_cursor !== null)
        return null;
      replacements.set(id, fresh);
    }
    const items = new Map(previous.items.map((item) => [item.id, item]));
    for (const [id, item] of replacements) items.set(id, item);
    return {
      items: [...items.values()].sort((a, b) => b.seq - a.seq),
      next_cursor: previous.next_cursor,
      has_newer: false,
      peer_read_seq: peerReadSeq,
    };
  }

  private async drain(): Promise<void> {
    this.draining = true;
    try {
      while (this.ready) {
        const queued = this.pending;
        const release = await this.acquire();
        if (queued !== this.pending) {
          release();
          continue;
        }
        const generation = this.generation;
        const revision = channelScopeRevision(this.cache);
        const intent = this.pending;
        this.pending = emptyPending();
        this.ready = false;
        const request = new AbortController();
        this.incremental = request;
        let reconcile = false;
        try {
          if (!this.current()) continue;
          const page = await this.incrementalPage(intent, request.signal);
          if (request.signal.aborted || generation !== this.generation || !this.current()) continue;
          reconcile = page === null || channelScopeRevision(this.cache) !== revision;
          if (!reconcile) this.cache.setQueryData(messagesKey(this.channelId), page);
        } catch (error: unknown) {
          if (!request.signal.aborted && generation === this.generation && this.current()) {
            if (channelScopeRevision(this.cache) === revision && asApiError(error).status === 404) this.unavailable();
            else reconcile = true;
          }
        } finally {
          if (this.incremental === request) this.incremental = null;
          release();
        }
        if (reconcile && this.current())
          await this.cache.invalidateQueries({ queryKey: messagesKey(this.channelId), exact: true });
      }
    } finally {
      this.draining = false;
    }
  }
}
