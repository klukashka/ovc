import { type UseQueryResult, skipToken, useQuery, useQueryClient } from '@tanstack/react-query';

import { useApiClient } from '../../lib/api/client.ts';
import { type ChannelPage, fetchChannels } from './api.ts';
import { channelScopeRevision, channelsScopeRevisionKey } from './cache.ts';
import { channelsKey } from './keys.ts';

export type FirstChannelPage = ChannelPage & { readonly fetchedScopeRevision: number };

export const firstPageScopeAttempts = 3;

export async function firstPageInScope<Page extends object>(
  fetchPage: () => Promise<Page>,
  revision: () => number,
): Promise<Page & { readonly fetchedScopeRevision: number }> {
  for (let attempt = 1; ; attempt += 1) {
    const fetchedScopeRevision = revision();
    const page = await fetchPage();
    if (revision() === fetchedScopeRevision || attempt === firstPageScopeAttempts)
      return { ...page, fetchedScopeRevision };
  }
}

export function useFirstChannelPage(q: string): UseQueryResult<FirstChannelPage> {
  const client = useApiClient();
  const cache = useQueryClient();
  useQuery<number>({ queryKey: channelsScopeRevisionKey, queryFn: skipToken, enabled: false, initialData: 0 });
  return useQuery({
    queryKey: [...channelsKey, q, null],
    queryFn: ({ signal }): Promise<FirstChannelPage> =>
      firstPageInScope(
        () => fetchChannels(client, q, null, signal),
        () => channelScopeRevision(cache),
      ),
    retry: false,
    refetchOnMount: 'always',
  });
}
