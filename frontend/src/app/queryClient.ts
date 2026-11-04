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

export class FollowingQueryClient extends QueryClient {
  private followUpGeneration = 0;
  private followUpsActive = true;

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
    await super.refetchQueries(filters, { ...options, cancelRefetch: false });
    if (runningBefore.size === 0 || !activeAtEntry || !this.followUpsActive || generation !== this.followUpGeneration) {
      return;
    }
    await super.refetchQueries(
      { ...filters, predicate: (query) => runningBefore.has(query) && fetchedData(query) },
      { ...options, cancelRefetch: false },
    );
  }
}

export function accountCacheOwnership(cache: QueryClient): (account: Pick<Me, 'id'> | null) => void {
  let owner: Me['id'] | undefined;
  return (account): void => {
    if (account?.id !== owner) cache.clear();
    owner = account?.id;
  };
}
