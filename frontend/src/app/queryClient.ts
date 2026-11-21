import type { Me } from '../lib/api/session.ts';

import {
  type Query,
  QueryClient,
  type QueryKey,
  type RefetchOptions,
  type RefetchQueryFilters,
} from '@tanstack/react-query';

function fetchedData(query: Query): boolean {
  return query.state.status === 'success';
}

class QueuedFollowUp {
  readonly queries: ReadonlySet<Query>;
  readonly started: Promise<void>;
  private markStarted: () => void = () => undefined;

  constructor(queries: ReadonlySet<Query>) {
    this.queries = queries;
    this.started = new Promise((resolve) => {
      this.markStarted = resolve;
    });
  }

  release(): void {
    this.markStarted();
  }
}

export class FollowingQueryClient extends QueryClient {
  private followUpGeneration = 0;
  private followUpsActive = true;
  private readonly queuedFollowUps = new Set<QueuedFollowUp>();

  followUpQueuedFor(queryKey: QueryKey): Promise<void> | null {
    const query = this.getQueryCache().find({ queryKey, exact: true });
    const waiting = [...this.queuedFollowUps].filter((queued) => query !== undefined && queued.queries.has(query));
    return waiting.length === 0 ? null : Promise.all(waiting.map((queued) => queued.started)).then(() => undefined);
  }

  suspendFollowUps(): void {
    this.followUpGeneration += 1;
    this.followUpsActive = false;
  }

  resumeFollowUps(): void {
    this.followUpGeneration += 1;
    this.followUpsActive = true;
  }

  override clear(): void {
    this.followUpGeneration += 1;
    super.clear();
  }

  override async refetchQueries<TTaggedQueryKey extends QueryKey = QueryKey>(
    filters?: RefetchQueryFilters<TTaggedQueryKey>,
    options?: RefetchOptions,
  ): Promise<void> {
    const generation = this.followUpGeneration;
    const activeAtEntry = this.followUpsActive;
    const runningBefore = new Set(this.getQueryCache().findAll({ ...filters, fetchStatus: 'fetching' }));
    const queued = runningBefore.size === 0 || !activeAtEntry ? null : this.queueFollowUp(runningBefore);
    try {
      await super.refetchQueries(filters, { ...options, cancelRefetch: false });
      if (queued === null || !this.followUpsActive || generation !== this.followUpGeneration) {
        return;
      }
      const followUp = super.refetchQueries(
        { ...filters, predicate: (query) => runningBefore.has(query) && fetchedData(query) },
        { ...options, cancelRefetch: false },
      );
      this.releaseFollowUp(queued);
      await followUp;
    } finally {
      if (queued !== null) {
        this.releaseFollowUp(queued);
      }
    }
  }

  private queueFollowUp(queries: ReadonlySet<Query>): QueuedFollowUp {
    const queued = new QueuedFollowUp(queries);
    this.queuedFollowUps.add(queued);
    return queued;
  }

  private releaseFollowUp(queued: QueuedFollowUp): void {
    this.queuedFollowUps.delete(queued);
    queued.release();
  }
}

export function followUpQueuedFor(client: QueryClient, queryKey: QueryKey): Promise<void> | null {
  return client instanceof FollowingQueryClient ? client.followUpQueuedFor(queryKey) : null;
}

export function accountCacheOwnership(cache: QueryClient): (account: Pick<Me, 'id'> | null) => void {
  let owner: Me['id'] | undefined;
  return (account): void => {
    if (account?.id !== owner) cache.clear();
    owner = account?.id;
  };
}
