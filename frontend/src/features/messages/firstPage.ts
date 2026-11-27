import { type UseQueryResult, skipToken, useQuery, useQueryClient } from '@tanstack/react-query';

import { useApiClient } from '../../lib/api/client.ts';
import { type ChannelPage, fetchChannels } from './api.ts';
import { channelScopeRevision, channelsScopeRevisionKey } from './cache.ts';
import type { ChannelFilter } from './filters.ts';
import { channelPageKey } from './keys.ts';

export type FirstChannelPage = ChannelPage & { readonly fetchedScopeRevision: number };

export const firstPageScopeAttempts = 3;

export class ChannelScopeUnsettledError extends Error {
  override readonly name = 'ChannelScopeUnsettledError';
}

export async function firstPageInScope<Page extends object>(
  fetchPage: () => Promise<Page>,
  revision: () => number,
): Promise<Page & { readonly fetchedScopeRevision: number }> {
  for (let attempt = 1; attempt <= firstPageScopeAttempts; attempt += 1) {
    const fetchedScopeRevision = revision();
    const page = await fetchPage();
    if (revision() === fetchedScopeRevision) return { ...page, fetchedScopeRevision };
  }
  throw new ChannelScopeUnsettledError('the channel scope changed during every first page attempt');
}

export function useFirstChannelPage(q: string, filter: ChannelFilter): UseQueryResult<FirstChannelPage> {
  const client = useApiClient();
  const cache = useQueryClient();
  useQuery<number>({ queryKey: channelsScopeRevisionKey, queryFn: skipToken, enabled: false, initialData: 0 });
  return useQuery({
    queryKey: channelPageKey(q, filter, 0, null),
    queryFn: ({ signal }): Promise<FirstChannelPage> =>
      firstPageInScope(
        () => fetchChannels(client, q, null, signal, filter),
        () => channelScopeRevision(cache),
      ),
    retry: false,
    refetchOnMount: 'always',
  });
}
