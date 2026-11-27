import { QueryObserver } from '@tanstack/react-query';
import { afterEach, describe, expect, test } from 'vitest';

import { FollowingQueryClient } from '../../app/queryClient.ts';
import { manualScheduler } from '../../lib/api/fakeTransport.ts';
import { channelScopeRevision, evictChannel } from './cache.ts';
import { ChannelScopeUnsettledError, firstPageInScope, firstPageScopeAttempts } from './firstPage.ts';
import { channelsKey } from './keys.ts';
import { chatRefetch } from './refetch.ts';

interface ListPage {
  readonly items: readonly { readonly id: string }[];
  readonly version: string;
}

type ScopedPage = ListPage & { readonly fetchedScopeRevision: number };

interface ListRequest {
  readonly signal: AbortSignal;
  readonly answer: (version: string, channels?: readonly string[]) => void;
}

const listKey = [...channelsKey, '', null] as const;
const clients: FollowingQueryClient[] = [];

afterEach(() => {
  for (const client of clients.splice(0)) client.clear();
});

function newClient(): FollowingQueryClient {
  const client = new FollowingQueryClient({ defaultOptions: { queries: { retry: false } } });
  clients.push(client);
  return client;
}

function listServer(): {
  readonly requests: ListRequest[];
  readonly fetchList: (context: { signal: AbortSignal }) => Promise<ListPage>;
} {
  const requests: ListRequest[] = [];
  return {
    requests,
    fetchList: ({ signal }) =>
      new Promise<ListPage>((resolve) => {
        requests.push({
          signal,
          answer: (version, channels = []) => {
            resolve({ items: channels.map((id) => ({ id })), version });
          },
        });
      }),
  };
}

async function settle(condition: () => boolean): Promise<void> {
  for (let turn = 0; turn < 200 && !condition(); turn += 1) {
    await Promise.resolve();
  }
}

function versionOf(queryClient: FollowingQueryClient): string | undefined {
  return queryClient.getQueryData<ScopedPage>(listKey)?.version;
}

function openList(
  queryClient: FollowingQueryClient,
  fetchList: (context: { signal: AbortSignal }) => Promise<ListPage>,
): string[][] {
  const shown: string[][] = [];
  const observer = new QueryObserver(queryClient, {
    queryKey: listKey,
    queryFn: (context): Promise<ScopedPage> =>
      firstPageInScope(
        () => fetchList(context),
        () => channelScopeRevision(queryClient),
      ),
    staleTime: Number.POSITIVE_INFINITY,
  });
  observer.subscribe((result) => {
    shown.push((result.data?.items ?? []).map((channel) => channel.id));
  });
  return shown;
}

describe('the chat list across live channel signals', () => {
  test('a live channel signal during a running list request does not abort it and the coalesced refetch ends with fresh data', async () => {
    const queryClient = newClient();
    const scheduler = manualScheduler();
    const server = listServer();
    openList(queryClient, server.fetchList);
    await settle(() => server.requests.length === 1);
    server.requests[0]?.answer('initial', ['kept']);
    await settle(() => versionOf(queryClient) === 'initial');
    const coalescer = chatRefetch(queryClient, scheduler);
    coalescer.changed();
    scheduler.pass(1000);
    await settle(() => server.requests.length === 2);

    evictChannel(queryClient, 'pinned-in', true);
    coalescer.changed('pinned-in');
    scheduler.pass(1000);
    server.requests[1]?.answer('during the signal', ['kept', 'pinned-in']);
    await settle(() => server.requests.length >= 3);
    server.requests.at(-1)?.answer('after the signal', ['kept', 'pinned-in']);
    await settle(() => versionOf(queryClient) === 'after the signal');

    expect(server.requests.map((request) => request.signal.aborted).every((aborted) => !aborted)).toBe(true);
    expect(versionOf(queryClient)).toBe('after the signal');
    expect(queryClient.getQueryData<ScopedPage>(listKey)?.fetchedScopeRevision).toBe(channelScopeRevision(queryClient));
  });

  test('a list response that started before a channel left the scope is fetched again and never shows that channel', async () => {
    const queryClient = newClient();
    const server = listServer();
    const shown = openList(queryClient, server.fetchList);
    await settle(() => server.requests.length === 1);

    evictChannel(queryClient, 'removed', true);
    server.requests[0]?.answer('before the removal', ['removed', 'kept']);
    await settle(() => server.requests.length === 2);
    server.requests[1]?.answer('after the removal', ['kept']);
    await settle(() => versionOf(queryClient) === 'after the removal');

    expect(shown.flat()).not.toContain('removed');
    expect(server.requests.map((request) => request.signal.aborted)).toEqual([false, false]);
    expect(queryClient.getQueryData<ScopedPage>(listKey)?.fetchedScopeRevision).toBe(channelScopeRevision(queryClient));
  });

  test('a list whose scope changes during every attempt ends in the retry state and never caches the stale page', async () => {
    const queryClient = newClient();
    const server = listServer();
    const shown = openList(queryClient, server.fetchList);
    await settle(() => server.requests.length === 1);

    for (let attempt = 1; attempt <= firstPageScopeAttempts; attempt += 1) {
      evictChannel(queryClient, `removed-${String(attempt)}`, true);
      server.requests[attempt - 1]?.answer(`attempt ${String(attempt)}`, [`removed-${String(attempt)}`, 'kept']);
      await settle(() => server.requests.length > attempt || queryClient.getQueryState(listKey)?.status === 'error');
    }

    expect(server.requests).toHaveLength(firstPageScopeAttempts);
    expect(queryClient.getQueryState(listKey)?.status).toBe('error');
    expect(queryClient.getQueryState(listKey)?.error).toBeInstanceOf(ChannelScopeUnsettledError);
    expect(versionOf(queryClient)).toBeUndefined();
    expect(shown.flat()).toEqual([]);
  });

  test('removing a channel from the caller’s scope still cancels a running list request', async () => {
    const queryClient = newClient();
    const server = listServer();
    openList(queryClient, server.fetchList);
    await settle(() => server.requests.length === 1);

    evictChannel(queryClient, 'removed');
    await settle(() => server.requests[0]?.signal.aborted === true);

    expect(server.requests[0]?.signal.aborted).toBe(true);
  });
});
