import { QueryClient, QueryObserver } from '@tanstack/react-query';
import { describe, expect, test } from 'vitest';

import { latestNotificationsKey } from '../features/notifications/api.ts';
import { liveRefetch } from './liveRefetch.ts';
import { NotificationArrivals } from './notificationArrivals.ts';

const deadlineMilliseconds = 2_000;

async function eventually(condition: () => void): Promise<void> {
  const deadline = Date.now() + deadlineMilliseconds;
  for (;;) {
    try {
      condition();
      return;
    } catch (error) {
      if (Date.now() >= deadline) {
        throw error;
      }
    }
    await new Promise<void>((resolve) => {
      setTimeout(resolve, 5);
    });
  }
}

function countFailingFirst(fetches: { count: number }): () => Promise<number> {
  return () => {
    fetches.count += 1;
    return fetches.count === 1 ? Promise.reject(new Error('the first read fails')) : Promise.resolve(fetches.count);
  };
}

describe('the live refetch after a reconnect', () => {
  test('a reconnect refetches a shown query whose last fetch failed', async () => {
    const queryClient = new QueryClient();
    const fetches = { count: 0 };
    const observer = new QueryObserver(queryClient, {
      queryKey: latestNotificationsKey,
      queryFn: countFailingFirst(fetches),
      retry: false,
    });
    const unsubscribe = observer.subscribe(() => undefined);
    await eventually(() => {
      expect(observer.getCurrentResult().status).toBe('error');
    });
    const onEvent = liveRefetch(queryClient, new NotificationArrivals());

    onEvent({ kind: 'connected', reconnect: true });

    await eventually(() => {
      expect(observer.getCurrentResult().status).toBe('success');
    });
    expect(fetches.count).toBe(2);
    expect(queryClient.getQueryData(latestNotificationsKey)).toBe(2);
    unsubscribe();
  });

  test('the first connection leaves a query whose last fetch failed alone', async () => {
    const queryClient = new QueryClient();
    const fetches = { count: 0 };
    const observer = new QueryObserver(queryClient, {
      queryKey: latestNotificationsKey,
      queryFn: countFailingFirst(fetches),
      retry: false,
    });
    const unsubscribe = observer.subscribe(() => undefined);
    await eventually(() => {
      expect(observer.getCurrentResult().status).toBe('error');
    });
    const onEvent = liveRefetch(queryClient, new NotificationArrivals());

    onEvent({ kind: 'connected', reconnect: false });

    const state = queryClient.getQueryState(latestNotificationsKey);
    expect(state?.fetchStatus).toBe('idle');
    expect(fetches.count).toBe(1);
    unsubscribe();
  });
});
