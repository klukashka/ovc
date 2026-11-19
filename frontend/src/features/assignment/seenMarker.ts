import { useQueryClient } from '@tanstack/react-query';
import { useEffect, useRef } from 'react';

import { useApiClient } from '../../lib/api/client.ts';
import { homeKey } from '../home/api.ts';
import type { Submission } from './api.ts';

export function useSeenMarker(submission: Submission | undefined): void {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const marked = useRef<string | null>(null);
  const id = submission?.id;
  const version = submission?.version;
  const reviewShown = submission !== undefined && submission.review !== null;
  useEffect(() => {
    if (!reviewShown || id === undefined || version === undefined) {
      return;
    }
    const shown = `${id}:${String(version)}`;
    if (marked.current === shown) {
      return;
    }
    marked.current = shown;
    client.POST('/api/v1/submissions/{id}/feedback-seen', { params: { path: { id } }, body: { version } }).then(
      ({ response }) => {
        if (response.ok) {
          void queryClient.invalidateQueries({ queryKey: homeKey });
        }
      },
      () => undefined,
    );
  }, [client, queryClient, id, version, reviewShown]);
}
