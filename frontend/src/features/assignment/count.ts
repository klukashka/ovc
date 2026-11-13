import { useQuery } from '@tanstack/react-query';

import { apiErrorFrom, useApiClient } from '../../lib/api/client.ts';
export const assignmentCountKey = ['assignments', 'count'] as const;

export function useToDoCount(enabled: boolean): number {
  const client = useApiClient();
  return (
    useQuery({
      queryKey: assignmentCountKey,
      queryFn: async ({ signal }) => {
        const { data, error, response } = await client.GET('/api/v1/assignments', {
          params: { query: { status: 'todo', limit: 1 } },
          signal,
        });
        if (data === undefined) {
          throw apiErrorFrom(response.status, error);
        }
        return data.counts.todo;
      },
      enabled,
      retry: false,
    }).data ?? 0
  );
}
