import { QueryObserver } from '@tanstack/react-query';
import { describe, expect, test } from 'vitest';

import { eventually } from '../lib/eventually.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../lib/api/fakeTransport.ts';
import { Session } from '../lib/api/session.ts';
import type { components } from '../lib/api/schema.ts';
import { ApiError, apiErrorFrom, createApiClient } from '../lib/api/client.ts';
import { fetchChannels } from '../features/messages/api.ts';
import { DocumentLifecycle } from './documentLifecycle.ts';
import { FollowingQueryClient, accountCacheOwnership } from './queryClient.ts';

async function heldFetch(held: readonly HeldFetch[], index: number): Promise<HeldFetch> {
  await eventually(() => {
    expect(held[index]).toBeDefined();
  });
  const found = held[index];
  if (found === undefined) {
    throw new Error(`fetch ${String(index)} never started`);
  }
  return found;
}

interface HeldFetch {
  readonly signal: AbortSignal;
  readonly answer: (value: number) => void;
}

describe('the following query client', () => {
  test('an invalidation during a fetch lets it finish and fetches once more instead of aborting it', async () => {
    const queryClient = new FollowingQueryClient();
    const held: HeldFetch[] = [];
    const observer = new QueryObserver(queryClient, {
      queryKey: ['students'],
      queryFn: ({ signal }): Promise<number> =>
        new Promise((resolve) => {
          held.push({ signal, answer: resolve });
        }),
    });
    const unsubscribe = observer.subscribe(() => undefined);
    const first = await heldFetch(held, 0);

    const invalidation = queryClient.invalidateQueries({ queryKey: ['students'] });
    first.answer(1);
    (await heldFetch(held, 1)).answer(2);
    await invalidation;

    expect(first.signal.aborted).toBe(false);
    expect(held).toHaveLength(2);
    expect(queryClient.isFetching()).toBe(0);
    expect(queryClient.getQueryData(['students'])).toBe(2);
    unsubscribe();
  });

  test('a follow-up queued behind a fetch is reported until it is abandoned by suspended follow-ups', async () => {
    const queryClient = new FollowingQueryClient();
    const held: HeldFetch[] = [];
    const observer = new QueryObserver(queryClient, {
      queryKey: ['home'],
      queryFn: ({ signal }): Promise<number> =>
        new Promise((resolve) => {
          held.push({ signal, answer: resolve });
        }),
    });
    const unsubscribe = observer.subscribe(() => undefined);
    const first = await heldFetch(held, 0);
    const queuedBeforeInvalidation = queryClient.followUpQueuedFor(['home']);

    const invalidation = queryClient.invalidateQueries({ queryKey: ['home'] });
    const queued = queryClient.followUpQueuedFor(['home']);
    queryClient.suspendFollowUps();
    first.answer(1);
    await invalidation;
    await queued;

    expect(queuedBeforeInvalidation).toBeNull();
    expect(queued).not.toBeNull();
    expect(queryClient.followUpQueuedFor(['home'])).toBeNull();
    expect(held).toHaveLength(1);
    unsubscribe();
  });
});

test.each([false, true])('a departed first-wave refusal preserves throwOnError=%s', async (throwOnError) => {
  const server = new FakeAuthServer();
  const account = { id: 'owner-a', name: 'A tutor', email: 'owner@example.org', password: 'a long test password' };
  server.addAccount(account);
  const clock = manualScheduler();
  const session = new Session({
    origin: 'https://ovc.example',
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: clock,
  });
  await session.signIn(account.email, account.password);
  const cache = new FollowingQueryClient();
  const events = new EventTarget();
  const lifecycle = new DocumentLifecycle({ events, loaded: (): boolean => true }, clock, cache);
  const client = createApiClient('https://ovc.example', session.authorizedFetch);
  const held = server.pauseJoinRequestReads();
  server.failJoinRequestReads(1);
  const key = ['refused-follow-up'];
  const observer = new QueryObserver(cache, {
    queryKey: key,
    queryFn: async (): Promise<components['schemas']['TutorJoinRequestPage']> => {
      const { data, error, response } = await client.GET('/api/v1/join-requests');
      if (data === undefined) throw apiErrorFrom(response.status, error);
      return data;
    },
    retry: false,
  });
  const unsubscribe = observer.subscribe(() => undefined);
  await held.answered;
  const outcome = cache.invalidateQueries({ queryKey: key }, { throwOnError }).then(
    () => ({ rejected: false, error: null }),
    (error: unknown) => ({ rejected: true, error }),
  );

  events.dispatchEvent(new Event('beforeunload'));
  held.resume();
  const result = await outcome;

  expect(result.rejected).toBe(throwOnError);
  if (throwOnError) expect(result.error).toBeInstanceOf(ApiError);
  expect(observer.getCurrentResult().status).toBe('error');
  expect(server.sentAddresses.filter((address) => address.pathname === '/api/v1/join-requests')).toHaveLength(1);
  unsubscribe();
  lifecycle.dispose();
  cache.clear();
});

test('an account replacement retains only its own data after the old first wave resolves', async () => {
  const server = new FakeAuthServer();
  const first = { id: 'first-owner', name: 'First owner', email: 'first@example.org', password: 'first long password' };
  const second = {
    id: 'second-owner',
    name: 'Second owner',
    email: 'second@example.org',
    password: 'second long password',
  };
  for (const account of [first, second]) {
    server.addAccount(account);
    server.chat.addChannel({
      channel: {
        id: account.id,
        kind: 'direct',
        label: account.name,
        label_kind: null,
        colour: null,
        tutor: null,
        last_message: null,
        unread_count: 0,
        pinned: null,
        peer_read_seq: null,
        writable: true,
        online: null,
        unconfirmed: false,
        user_id: null,
      },
      members: [{ accountId: account.id }],
    });
  }
  const session = new Session({
    origin: 'https://ovc.example',
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  const cache = new FollowingQueryClient();
  const ownCache = accountCacheOwnership(cache);
  const stopOwner = session.subscribe(() => {
    const state = session.snapshot();
    ownCache(state.kind === 'signedIn' ? state.me : null);
  });
  await session.signIn(first.email, first.password);
  const client = createApiClient('https://ovc.example', session.authorizedFetch);
  const held = server.chat.pauseNextChannelListResponse(false);
  const key = ['account-channels'] as const;
  const observer = new QueryObserver(cache, {
    queryKey: key,
    queryFn: ({ signal }): ReturnType<typeof fetchChannels> => fetchChannels(client, '', null, signal),
    retry: false,
  });
  const unsubscribe = observer.subscribe(() => undefined);
  const signal = await held.answered;
  const oldQuery = cache.getQueryCache().find({ queryKey: key });
  const invalidation = cache.invalidateQueries({ queryKey: key });

  await session.signIn(second.email, second.password);
  const current = await cache.query({
    queryKey: key,
    queryFn: ({ signal }): ReturnType<typeof fetchChannels> => fetchChannels(client, '', null, signal),
  });
  held.resume();
  await invalidation;

  expect(signal.aborted).toBe(true);
  expect(cache.getQueryCache().find({ queryKey: key })).not.toBe(oldQuery);
  expect(current.items.map((channel) => channel.id)).toEqual([second.id]);
  expect(cache.getQueryData(key)).toEqual(current);
  unsubscribe();
  stopOwner();
  cache.clear();
});
