import { QueryClient, QueryObserver } from '@tanstack/react-query';
import { describe, expect, test } from 'vitest';

import { assignmentListKey, submissionKey } from '../features/assignment/keys.ts';
import { studentHomeKey, tutorHomeKey } from '../features/home/api.ts';
import { issueKey, issueListKey } from '../features/homework/api.ts';
import { channelPageKey, channelKey, unreadChannelsKey, messagesKey } from '../features/messages/keys.ts';
import { queuePanelKey, reviewSubmissionKey } from '../features/review/keys.ts';
import { lessonKey } from '../features/schedule/keys.ts';
import { manualScheduler } from '../lib/api/fakeTransport.ts';
import { latestNotificationsKey } from '../features/notifications/api.ts';
import { eventually } from '../lib/eventually.ts';
import { channelScopeRevision } from '../features/messages/cache.ts';
import { type HistoryIntent, chatRefetch } from '../features/messages/refetch.ts';
import { liveRefetch } from './liveRefetch.ts';
import { FollowingQueryClient } from './queryClient.ts';
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
      channelPageKey('', 'all', 0, null),
      channelPageKey('', 'all', 0, 'older'),
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

  test('a read frame of a group chat leaves its detail alone, since a group exposes no peer read', () => {
    const client = new QueryClient();
    const scheduler = manualScheduler();
    client.setQueryData(channelKey('g'), { kind: 'group' });
    const onEvent = liveRefetch(client, new NotificationArrivals(), scheduler);

    onEvent({ kind: 'read', channelId: 'g' });

    expect(client.getQueryState(channelKey('g'))?.isInvalidated).toBe(false);
    client.clear();
  });

  test('a read frame refetches only its channel detail, without waiting for the burst window', async () => {
    const client = new QueryClient();
    const scheduler = manualScheduler();
    const counts = [{ value: 0 }, { value: 0 }];
    const observers = [channelKey('a'), channelKey('b')].map(
      (queryKey, index) =>
        new QueryObserver(client, {
          queryKey,
          queryFn: (): Promise<number> => {
            const count = counts[index];
            if (count === undefined) throw new Error('missing counter');
            count.value += 1;
            return Promise.resolve(count.value);
          },
        }),
    );
    const unsubscribes = observers.map((observer) => observer.subscribe(() => undefined));
    await eventually(() => {
      expect(observers.map((observer) => observer.getCurrentResult().status)).toEqual(['success', 'success']);
    });
    const onEvent = liveRefetch(client, new NotificationArrivals(), scheduler);

    onEvent({ kind: 'read', channelId: 'a' });

    await eventually(() => {
      expect(counts.map((count) => count.value)).toEqual([2, 1]);
    });
    for (const unsubscribe of unsubscribes) unsubscribe();
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

describe('pin change refetches', () => {
  interface PinPage {
    readonly items: readonly { readonly id: string }[];
    readonly next_cursor: string | null;
    readonly read: number;
  }

  const firstKey = channelPageKey('', 'all', 0, null);
  const laterKey = channelPageKey('', 'all', 0, 'later');

  async function watchPinQueries(
    client: FollowingQueryClient,
    scheduler: ReturnType<typeof manualScheduler>,
  ): Promise<{
    readonly reads: () => Record<string, number>;
    readonly intents: HistoryIntent[];
    readonly stop: () => void;
  }> {
    const watched: Record<string, readonly unknown[]> = {
      detail: channelKey('a'),
      first: firstKey,
      later: laterKey,
      history: messagesKey('a'),
    };
    const counts: Record<string, number> = { detail: 0, first: 0, later: 0, history: 0 };
    const observers = Object.entries(watched).map(
      ([name, queryKey]) =>
        new QueryObserver(client, {
          queryKey,
          queryFn: (): Promise<PinPage> => {
            const read = (counts[name] ?? 0) + 1;
            counts[name] = read;
            const items = name === 'later' ? [{ id: 'c' }, { id: 'a' }] : [{ id: 'a' }];
            return Promise.resolve({ items, next_cursor: null, read });
          },
          staleTime: Number.POSITIVE_INFINITY,
        }),
    );
    const unsubscribes = observers.map((observer) => observer.subscribe(() => undefined));
    await eventually(() => {
      expect(observers.every((observer) => observer.getCurrentResult().status === 'success')).toBe(true);
    });
    const intents: HistoryIntent[] = [];
    const unmount = chatRefetch(client, scheduler).mount('a', {
      changed: (intent) => {
        intents.push(intent);
      },
      flush: (): void => undefined,
      cancel: (): void => undefined,
      suspend: (): void => undefined,
      resume: (): void => undefined,
    });
    return {
      reads: (): Record<string, number> => ({ ...counts }),
      intents,
      stop: (): void => {
        unmount();
        for (const unsubscribe of unsubscribes) unsubscribe();
      },
    };
  }

  test('a channel_pin frame keeps the list pages and refreshes the detail once without re-reading the history', async () => {
    const scheduler = manualScheduler();
    const client = new FollowingQueryClient({ defaultOptions: { queries: { retry: false } } });
    const queries = await watchPinQueries(client, scheduler);
    const onEvent = liveRefetch(client, new NotificationArrivals(), scheduler);

    onEvent({ kind: 'entity', entity: 'channel_pin', id: 'a' });
    scheduler.pass(1000);

    await eventually(() => {
      expect(queries.reads()).toEqual({ detail: 2, first: 2, later: 1, history: 1 });
    });
    expect(client.getQueryData<PinPage>(firstKey)?.items).toEqual([{ id: 'a' }]);
    expect(client.getQueryData<PinPage>(laterKey)).toEqual({
      items: [{ id: 'c' }, { id: 'a' }],
      next_cursor: null,
      read: 1,
    });
    expect(channelScopeRevision(client)).toBe(0);
    expect(queries.intents).toEqual([]);
    expect(scheduler.pendingDelays()).toEqual([]);
    queries.stop();
    onEvent.cancel();
    client.clear();
  });

  test('a channel_pin frame refreshes nothing before the coalescer’s second has passed', () => {
    const scheduler = manualScheduler();
    const client = new FollowingQueryClient();
    client.setQueryData(channelKey('a'), { id: 'a' });
    client.setQueryData(unreadChannelsKey, 1);
    const onEvent = liveRefetch(client, new NotificationArrivals(), scheduler);

    onEvent({ kind: 'entity', entity: 'channel_pin', id: 'a' });
    scheduler.pass(999);

    expect(client.getQueryState(channelKey('a'))?.isInvalidated).toBe(false);
    expect(client.getQueryState(unreadChannelsKey)?.isInvalidated).toBe(false);
    expect(client.getQueryData(channelKey('a'))).toEqual({ id: 'a' });
    onEvent.cancel();
    client.clear();
  });

  test('a channel_pin frame while the coalescer is suspended schedules nothing', () => {
    const scheduler = manualScheduler();
    const client = new FollowingQueryClient();
    client.setQueryData(channelKey('a'), { id: 'a' });
    const onEvent = liveRefetch(client, new NotificationArrivals(), scheduler);
    onEvent.suspend();

    onEvent({ kind: 'entity', entity: 'channel_pin', id: 'a' });

    expect(scheduler.pendingDelays()).toEqual([]);
    expect(client.getQueryState(channelKey('a'))?.isInvalidated).toBe(false);
    onEvent.resume();
    onEvent.cancel();
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
  const firstKey = channelPageKey('', 'all', 0, null);
  const laterKey = channelPageKey('', 'all', 0, 'later');
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

describe('homework frame refetches', () => {
  const watched: readonly (readonly unknown[])[] = [
    submissionKey('submission-a'),
    submissionKey('submission-b'),
    assignmentListKey('todo', null, 5),
    reviewSubmissionKey('submission-a'),
    reviewSubmissionKey('submission-b'),
    queuePanelKey(5),
    issueKey('issue-a'),
    issueKey('issue-b'),
    issueListKey({ status: 'active' }),
    lessonKey('lesson-a'),
    lessonKey('lesson-b'),
  ];

  async function watchAll(
    client: QueryClient,
  ): Promise<{ readonly fetched: () => string[]; readonly stop: () => void }> {
    const counts = new Map<string, number>();
    const observers = watched.map(
      (queryKey) =>
        new QueryObserver(client, {
          queryKey,
          queryFn: (): Promise<number> => {
            const name = JSON.stringify(queryKey);
            const next = (counts.get(name) ?? 0) + 1;
            counts.set(name, next);
            return Promise.resolve(next);
          },
        }),
    );
    const unsubscribes = observers.map((observer) => observer.subscribe(() => undefined));
    await eventually(() => {
      expect(observers.every((observer) => observer.getCurrentResult().status === 'success')).toBe(true);
    });
    return {
      fetched: (): string[] =>
        [...counts.entries()]
          .filter(([, count]) => count > 1)
          .map(([name]) => name)
          .sort(),
      stop: (): void => {
        for (const unsubscribe of unsubscribes) {
          unsubscribe();
        }
      },
    };
  }

  function names(...keys: readonly (readonly unknown[])[]): string[] {
    return keys.map((key) => JSON.stringify(key)).sort();
  }

  test('a student submission frame refetches the lists and only its own assignment page and lessons', async () => {
    const client = new QueryClient();
    const queries = await watchAll(client);
    const onEvent = liveRefetch(client, new NotificationArrivals());

    onEvent({ kind: 'entity', entity: 'submission', id: 'submission-a', lessonIds: ['lesson-a'] });

    await eventually(() => {
      expect(queries.fetched()).toEqual(
        names(
          submissionKey('submission-a'),
          assignmentListKey('todo', null, 5),
          reviewSubmissionKey('submission-a'),
          queuePanelKey(5),
          lessonKey('lesson-a'),
        ),
      );
    });
    queries.stop();
  });

  test('a tutor submission frame refetches its issue but no other issue page and no lesson it does not name', async () => {
    const client = new QueryClient();
    const queries = await watchAll(client);
    const onEvent = liveRefetch(client, new NotificationArrivals());

    onEvent({ kind: 'entity', entity: 'submission', id: 'submission-b', issueId: 'issue-a' });

    await eventually(() => {
      expect(queries.fetched()).toEqual(
        names(
          submissionKey('submission-b'),
          assignmentListKey('todo', null, 5),
          reviewSubmissionKey('submission-b'),
          queuePanelKey(5),
          issueKey('issue-a'),
          issueListKey({ status: 'active' }),
        ),
      );
    });
    queries.stop();
  });

  test('an issue frame refetches the issue lists, its own page and the lessons it names', async () => {
    const client = new QueryClient();
    const queries = await watchAll(client);
    const onEvent = liveRefetch(client, new NotificationArrivals());

    onEvent({ kind: 'entity', entity: 'issue', id: 'issue-b', lessonIds: ['lesson-a', 'lesson-b'] });

    await eventually(() => {
      expect(queries.fetched()).toEqual(
        names(issueKey('issue-b'), issueListKey({ status: 'active' }), lessonKey('lesson-a'), lessonKey('lesson-b')),
      );
    });
    queries.stop();
  });
});

type EntityEvent = Extract<Parameters<ReturnType<typeof liveRefetch>>[0], { readonly kind: 'entity' }>;

function homeInvalidations(event: Parameters<ReturnType<typeof liveRefetch>>[0]): {
  readonly tutor: boolean;
  readonly student: boolean;
} {
  const client = new QueryClient();
  const scheduler = manualScheduler();
  client.setQueryData(tutorHomeKey, 'tutor home');
  client.setQueryData(studentHomeKey(null), 'student home');
  const onEvent = liveRefetch(client, new NotificationArrivals(), scheduler);
  onEvent(event);
  scheduler.pass(1_000);
  const invalidated = {
    tutor: client.getQueryState(tutorHomeKey)?.isInvalidated === true,
    student: client.getQueryState(studentHomeKey(null))?.isInvalidated === true,
  };
  onEvent.cancel();
  client.clear();
  return invalidated;
}

describe('the /home live refetch rows', () => {
  test('each frame kind invalidates the home read of the roles the mapping lists', () => {
    const entity = (kind: EntityEvent['entity']): EntityEvent => ({ kind: 'entity', entity: kind, id: 'x' });

    const answers = {
      relationship: homeInvalidations(entity('relationship')),
      joinRequest: homeInvalidations(entity('join_request')),
      notification: homeInvalidations(entity('notification')),
      notificationNew: homeInvalidations({ kind: 'notification', id: 'x' }),
      channel: homeInvalidations(entity('channel')),
      channelPin: homeInvalidations(entity('channel_pin')),
      lesson: homeInvalidations(entity('lesson')),
      issue: homeInvalidations(entity('issue')),
      submission: homeInvalidations(entity('submission')),
      message: homeInvalidations({ kind: 'message', change: 'new', channelId: 'a', messageId: 'one', seq: 1 }),
      read: homeInvalidations({ kind: 'read', channelId: 'a' }),
    };

    const both = { tutor: true, student: true };
    const tutorOnly = { tutor: true, student: false };
    const neither = { tutor: false, student: false };
    expect(answers).toEqual({
      relationship: both,
      joinRequest: tutorOnly,
      notification: neither,
      notificationNew: neither,
      channel: both,
      channelPin: tutorOnly,
      lesson: both,
      issue: tutorOnly,
      submission: both,
      message: tutorOnly,
      read: tutorOnly,
    });
  });

  test('three frames in one burst while a home read is in flight cost at most two home reads', async () => {
    const client = new FollowingQueryClient();
    const reads: { readonly resolve: (value: number) => void }[] = [];
    const observer = new QueryObserver(client, {
      queryKey: tutorHomeKey,
      queryFn: (): Promise<number> =>
        new Promise<number>((resolve) => {
          reads.push({ resolve });
        }),
      retry: false,
    });
    const unsubscribe = observer.subscribe(() => undefined);
    await eventually(() => {
      expect(reads).toHaveLength(1);
    });
    reads[0]?.resolve(1);
    await eventually(() => {
      expect(observer.getCurrentResult().status).toBe('success');
    });
    const onEvent = liveRefetch(client, new NotificationArrivals(), manualScheduler());
    const lesson = (id: string): EntityEvent => ({ kind: 'entity', entity: 'lesson', id });

    onEvent(lesson('a'));
    await eventually(() => {
      expect(reads).toHaveLength(2);
    });
    onEvent(lesson('b'));
    onEvent(lesson('c'));
    reads[1]?.resolve(2);
    await eventually(() => {
      expect(reads).toHaveLength(3);
    });
    reads[2]?.resolve(3);
    await eventually(() => {
      expect(observer.getCurrentResult().data).toBe(3);
    });

    expect(reads).toHaveLength(3);
    onEvent.cancel();
    unsubscribe();
    client.clear();
  });
});
