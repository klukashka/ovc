import { useQuery } from '@tanstack/react-query';

import { apiErrorFrom, useApiClient } from '../../lib/api/client.ts';
import { unreadChannelsKey } from './keys.ts';

export function useUnreadChannels(enabled: boolean): number {
  const client = useApiClient();
  return (
    useQuery({
      queryKey: unreadChannelsKey,
      queryFn: async ({ signal }) => {
        const { data, error, response } = await client.GET('/api/v1/channels/unread', { signal });
        if (data === undefined) {
          throw apiErrorFrom(response.status, error);
        }
        return data.unread_channels;
      },
      enabled,
      retry: false,
    }).data ?? 0
  );
}
