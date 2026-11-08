import { type UseQueryResult, skipToken, useQuery, useQueryClient } from '@tanstack/react-query';

import { useApiClient } from '../../lib/api/client.ts';
import { type ChannelPage, fetchChannels } from './api.ts';
import { channelScopeRevision, channelsScopeRevisionKey } from './cache.ts';
import { channelsKey } from './keys.ts';

export type FirstChannelPage = ChannelPage & { readonly fetchedScopeRevision: number };

export function useFirstChannelPage(q: string): UseQueryResult<FirstChannelPage> {
  const client = useApiClient();
  const cache = useQueryClient();
  useQuery<number>({ queryKey: channelsScopeRevisionKey, queryFn: skipToken, enabled: false, initialData: 0 });
  return useQuery({
    queryKey: [...channelsKey, q, null],
    queryFn: async ({ signal }): Promise<FirstChannelPage> => {
      const fetchedScopeRevision = channelScopeRevision(cache);
      const page = await fetchChannels(client, q, null, signal);
      return { ...page, fetchedScopeRevision };
    },
    retry: false,
    refetchOnMount: 'always',
  });
}
