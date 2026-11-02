import { QueryClient, QueryObserver } from '@tanstack/react-query';
import { describe, expect, test } from 'vitest';

import { channelsKey, unreadChannelsKey, messagesKey } from '../features/messages/keys.ts';
import { manualScheduler } from '../lib/api/fakeTransport.ts';
import { latestNotificationsKey } from '../features/notifications/api.ts';
import { eventually } from '../lib/eventually.ts';
import { liveRefetch } from './liveRefetch.ts';
import { NotificationArrivals } from './notificationArrivals.ts';

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

describe('chat event refetches', () => {
  test('bursts refetch unread and only the first channel page once, together with the affected history', async () => {
    const client = new QueryClient();
    const scheduler = manualScheduler();
    const keys = [
      unreadChannelsKey,
      [...channelsKey, '', null],
      [...channelsKey, '', 'older'],
      messagesKey('a'),
      messagesKey('b'),
    ];
    const counts = keys.map(() => ({ value: 0 }));
    const observers = keys.map(
      (queryKey, index) =>
        new QueryObserver(client, {
          queryKey,
          queryFn: (): Promise<number> => {
            const count = counts[index];
            if (count === undefined) {
              throw new Error('missing counter');
            }
            count.value += 1;
            return Promise.resolve(count.value);
          },
        }),
    );
    const unsubscribes = observers.map((observer) => observer.subscribe(() => undefined));
    await eventually(() => {
      expect(observers.map((observer) => observer.getCurrentResult().status)).toEqual([
        'success',
        'success',
        'success',
        'success',
        'success',
      ]);
    });
    const onEvent = liveRefetch(client, new NotificationArrivals(), scheduler);

    onEvent({ kind: 'message', change: 'new', channelId: 'a', messageId: 'one', seq: 1 });
    onEvent({ kind: 'message', change: 'deleted', channelId: 'a', messageId: 'two' });
    onEvent({ kind: 'read', channelId: 'a' });
    scheduler.pass(999);
    const beforeDeadline = counts.map((count) => count.value);
    scheduler.pass(1);

    expect(beforeDeadline).toEqual([1, 1, 1, 1, 1]);
    await eventually(() => {
      expect(counts.map((count) => count.value)).toEqual([2, 2, 1, 2, 1]);
    });
    expect(scheduler.pendingDelays()).toEqual([]);
    for (const unsubscribe of unsubscribes) {
      unsubscribe();
    }
    client.clear();
  });

  test('cancellation prevents an old account timer from invalidating new account data', () => {
    const client = new QueryClient();
    const scheduler = manualScheduler();
    client.setQueryData(unreadChannelsKey, 1);
    const onEvent = liveRefetch(client, new NotificationArrivals(), scheduler);

    onEvent({ kind: 'entity', entity: 'channel', id: 'a' });
    onEvent.cancel();
    client.clear();
    client.setQueryData(unreadChannelsKey, 7);
    scheduler.pass(1000);

    expect(client.getQueryState(unreadChannelsKey)?.isInvalidated).toBe(false);
    expect(client.getQueryData(unreadChannelsKey)).toBe(7);
    expect(scheduler.pendingDelays()).toEqual([]);
    client.clear();
  });
});

test('channel changes evict inactive private data and previews from every cached list page', () => {
  const client = new QueryClient();
  const scheduler = manualScheduler();
  const channel = {
    id: 'a',
    label: 'Private label',
    kind: 'direct',
    label_kind: null,
    colour: null,
    last_message: null,
    online: null,
    tutor: null,
    unconfirmed: false,
    unread_count: 0,
    user_id: null,
    writable: true,
  } as const;
  client.setQueryData(['channel', 'a'], channel);
  client.setQueryData(messagesKey('a'), { items: [{ body: 'Private history' }], next_cursor: null });
  const firstKey = [...channelsKey, '', null];
  const laterKey = [...channelsKey, '', 'later'];
  client.setQueryData(firstKey, { items: [channel], next_cursor: 'later', unread_channels: 0 });
  client.setQueryData(laterKey, { items: [channel, { ...channel, id: 'b' }], next_cursor: null, unread_channels: 0 });
  const onEvent = liveRefetch(client, new NotificationArrivals(), scheduler);

  onEvent({ kind: 'entity', entity: 'channel', id: 'a' });

  expect(client.getQueryData(['channel', 'a'])).toBeUndefined();
  expect(client.getQueryData(messagesKey('a'))).toBeUndefined();
  expect(client.getQueryData(firstKey)).toEqual({ items: [], next_cursor: 'later', unread_channels: 0 });
  expect(client.getQueryData(laterKey)).toEqual({
    items: [{ ...channel, id: 'b' }],
    next_cursor: null,
    unread_channels: 0,
  });
  onEvent.cancel();
  client.clear();
});
