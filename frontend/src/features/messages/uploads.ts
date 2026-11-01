import { useEffect, useState, useSyncExternalStore } from 'react';

import { apiErrorFrom, useApiClient } from '../../lib/api/client.ts';
import { useSession } from '../../lib/api/session.ts';
import { UploadQueue } from './UploadQueue.ts';

export function useUploads(
  channelId: string,
  writable: boolean,
  current: () => boolean,
): {
  readonly queue: UploadQueue;
  readonly items: ReturnType<UploadQueue['snapshot']>;
} {
  const client = useApiClient();
  const session = useSession();
  const [epoch] = useState(() => session.currentEpoch());
  const [queue] = useState(
    () =>
      new UploadQueue(async (file, signal, progress) => {
        const body = new FormData();
        body.append('file', file);
        const { data, error, response } = await client.POST('/api/v1/uploads', {
          params: { query: { context: 'chat', scope: `channel:${channelId}` } },
          signal,
          body: { file: '' },
          bodySerializer: () => null,
          fetch: (request) => session.authorizedUpload(request, { body, progress }),
        });
        if (data === undefined) throw apiErrorFrom(response.status, error);
        return data;
      }, current),
  );
  const items = useSyncExternalStore(queue.subscribe, queue.snapshot);
  useEffect(() => {
    const unsubscribe = session.subscribe(() => {
      if (session.currentEpoch() !== epoch) queue.stop();
    });
    const stop = (): void => {
      queue.clear();
    };
    window.addEventListener('pagehide', stop);
    return (): void => {
      unsubscribe();
      window.removeEventListener('pagehide', stop);
      queue.clear();
    };
  }, [queue, session, epoch]);
  useEffect(() => {
    if (!writable) queue.clear();
  }, [queue, writable]);
  return { queue, items };
}
