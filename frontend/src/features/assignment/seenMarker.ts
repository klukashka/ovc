import { useQueryClient } from '@tanstack/react-query';
import { useEffect, useRef } from 'react';

import { useApiClient } from '../../lib/api/client.ts';
import { homeKey } from '../home/api.ts';
import type { Submission } from './api.ts';

const conflictStatus = 409;

function shownReview(submission: Submission | undefined): string | null {
  if (submission?.review === undefined || submission.review === null) {
    return null;
  }
  const items = [...submission.review.items]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([questionId, item]) => [questionId, item.comment, item.correctedFiles.map((file) => file.attachmentId)]);
  return JSON.stringify([submission.id, submission.status, submission.review.feedback, items]);
}

export function useSeenMarker(submission: Submission | undefined): void {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const marked = useRef<string | null>(null);
  const id = submission?.id;
  const version = submission?.version;
  const shown = shownReview(submission);
  useEffect(() => {
    if (shown === null || id === undefined || version === undefined || marked.current === shown) {
      return;
    }
    marked.current = shown;
    client.POST('/api/v1/submissions/{id}/feedback-seen', { params: { path: { id } }, body: { version } }).then(
      ({ response }) => {
        if (response.ok) {
          void queryClient.invalidateQueries({ queryKey: homeKey });
        } else if (response.status === conflictStatus && marked.current === shown) {
          marked.current = null;
        }
      },
      () => undefined,
    );
  }, [client, queryClient, id, version, shown]);
}
