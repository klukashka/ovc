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
  override async refetchQueries<TTaggedQueryKey extends QueryKey = QueryKey>(
    filters?: RefetchQueryFilters<TTaggedQueryKey>,
    options?: RefetchOptions,
  ): Promise<void> {
    const runningBefore = new Set(this.getQueryCache().findAll({ ...filters, fetchStatus: 'fetching' }));
    await super.refetchQueries(filters, { ...options, cancelRefetch: false });
    if (runningBefore.size === 0) {
      return;
    }
    await super.refetchQueries(
      { ...filters, predicate: (query) => runningBefore.has(query) && fetchedData(query) },
      { ...options, cancelRefetch: false },
    );
  }
}
