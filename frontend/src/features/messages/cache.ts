import type { QueryClient } from '@tanstack/react-query';

import type { ChannelPage } from './api.ts';
import { channelKey, channelsKey, messagesKey } from './keys.ts';

export const channelsScopeRevisionKey = ['channels-scope-revision'] as const;

export function channelScopeRevision(queryClient: QueryClient): number {
  return queryClient.getQueryData<number>(channelsScopeRevisionKey) ?? 0;
}

export function evictChannel(queryClient: QueryClient, id: string, inactiveOnly = false): void {
  for (const key of [channelKey(id), messagesKey(id)]) {
    const query = queryClient.getQueryCache().find({ queryKey: key, exact: true });
    if (query !== undefined && (!inactiveOnly || !query.isActive())) {
      void queryClient.cancelQueries({ queryKey: key, exact: true });
      queryClient.removeQueries({ queryKey: key, exact: true });
    }
  }
  void queryClient.cancelQueries({ queryKey: channelsKey });
  queryClient.setQueryData<number>(channelsScopeRevisionKey, (revision) => (revision ?? 0) + 1);
  queryClient.setQueriesData<ChannelPage>({ queryKey: channelsKey }, (page) => {
    if (page === undefined) return undefined;
    return { ...page, items: page.items.filter((channel) => channel.id !== id) };
  });
}
