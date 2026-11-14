import { useQuery } from '@tanstack/react-query';

import { apiErrorFrom, useApiClient } from '../lib/api/client.ts';
import type { Role } from './sections.ts';

export const assignmentCountKey = ['assignments', 'count'] as const;

export const reviewCountKey = ['review', 'count'] as const;

export function useHomeworkCount(role: Role, enabled: boolean): number {
  const client = useApiClient();
  return (
    useQuery({
      queryKey: role === 'tutor' ? reviewCountKey : assignmentCountKey,
      queryFn: async ({ signal }) => {
        if (role === 'tutor') {
          const { data, error, response } = await client.GET('/api/v1/review-queue', {
            params: { query: { limit: 1 } },
            signal,
          });
          if (data === undefined) {
            throw apiErrorFrom(response.status, error);
          }
          return data.count;
        }
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
