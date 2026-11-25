import { QueryObserver } from '@tanstack/react-query';
import { describe, expect, test } from 'vitest';

import { FollowingQueryClient } from '../../app/queryClient.ts';
import { liveRefetch } from '../../app/liveRefetch.ts';
import { createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import type { ApiClient } from '../../lib/api/client.ts';
import type { Scheduler } from '../../lib/api/session.ts';
import type { RealtimeEvent } from '../../lib/realtime/client.ts';
import { Session } from '../../lib/api/session.ts';
import { eventually } from '../../lib/eventually.ts';
import { type Channel, type Message, type MessagePage, deleteMessage, editMessage, fetchChannel } from './api.ts';
import { evictChannel } from './cache.ts';
import { HistoryRefresh } from './historyRefresh.ts';
import { channelKey, messagesKey } from './keys.ts';
import { chatRefetch } from './refetch.ts';

const channelId = 'history-channel';
const account = { id: 'history-tutor', name: 'Tutor', email: 'history@example.org', password: 'a long password' };
const channel: Channel = {
  id: channelId,
  kind: 'direct',
  label: 'Student',
  label_kind: null,
  colour: null,
  last_message: null,
  online: false,
  tutor: null,
  unconfirmed: false,
  unread_count: 0,
  pinned: null,
  user_id: 'history-student',
  writable: true,
};

function message(seq: number): Message & { readonly senderId: string } {
  return {
    id: `history-${String(seq)}`,
    seq,
    body: `Text ${String(seq)}`,
    attachments: [],
    created_at: '2026-11-02T00:00:00Z',
    edited_at: null,
    deleted: false,
    erased: false,
    own: true,
    sender_label: 'Tutor',
    sender_label_kind: null,
    sent_before_reclaim: false,
    reply_to: null,
    version: 1,
    senderId: account.id,
  };
}

interface HistoryFixture {
  readonly server: FakeAuthServer;
  readonly api: ApiClient;
  readonly cache: FollowingQueryClient;
  readonly clock: Scheduler & { readonly pass: (milliseconds: number) => void };
  readonly session: Session;
  readonly owner: HistoryRefresh;
  readonly receive: ((event: RealtimeEvent) => void) & {
    readonly cancel: () => void;
    readonly suspend: () => void;
    readonly resume: () => void;
  };
  readonly data: () => MessagePage | undefined;
  readonly reads: () => readonly URL[];
  readonly stop: () => void;
}

async function fixture(count = 50, all = false, suspended = false): Promise<HistoryFixture> {
  const server = new FakeAuthServer();
  server.addAccount(account);
  server.chat.addChannel({ channel, members: [{ accountId: account.id }] });
  for (let seq = 1; seq <= count; seq += 1) server.chat.addMessage(channelId, message(seq));
  const clock = manualScheduler();
  const session = new Session({
    origin: 'https://ovc.example',
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: clock,
  });
  await session.signIn(account.email, account.password);
  const api = createApiClient('https://ovc.example', session.authorizedFetch);
  const cache = new FollowingQueryClient({ defaultOptions: { queries: { retry: false } } });
  cache.setQueryData(channelKey(channelId), channel);
  const detail = cache.getQueryCache().find({ queryKey: channelKey(channelId), exact: true });
  const epoch = session.currentEpoch();
  let mounted = true;
  const owner = new HistoryRefresh(
    cache,
    api,
    channelId,
    () =>
      mounted &&
      session.currentEpoch() === epoch &&
      cache.getQueryCache().find({ queryKey: channelKey(channelId), exact: true }) === detail,
    () => {
      evictChannel(cache, channelId);
    },
    () => {
      chatRefetch(cache, clock).changed(channelId);
    },
  );
  const observer = new QueryObserver(cache, {
    queryKey: messagesKey(channelId),
    queryFn: ({ signal }): Promise<MessagePage> => owner.full(signal, () => null),
    retry: false,
  });
  const unsubscribe = observer.subscribe(() => undefined);
  if (suspended) chatRefetch(cache, clock).suspend();
  const unmount = chatRefetch(cache, clock).mount(channelId, owner);
  const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);
  await eventually(() => {
    expect(observer.getCurrentResult().isSuccess).toBe(true);
  });
  if (all)
    while (cache.getQueryData<MessagePage>(messagesKey(channelId))?.next_cursor != null)
      await owner.older(new AbortController().signal);
  return {
    server,
    api,
    cache,
    clock,
    session,
    owner,
    receive,
    data: (): MessagePage | undefined => cache.getQueryData<MessagePage>(messagesKey(channelId)),
    reads: (): readonly URL[] =>
      server.sentAddresses.filter((url) => url.pathname.endsWith(`/channels/${channelId}/messages`)),
    stop: (): void => {
      mounted = false;
      receive.cancel();
      unmount();
      unsubscribe();
      cache.clear();
    },
  };
}

function announce(rig: Awaited<ReturnType<typeof fixture>>, seq: number): void {
  rig.receive({ kind: 'message', change: 'new', channelId, messageId: message(seq).id, seq });
}

async function settled(rig: Awaited<ReturnType<typeof fixture>>, seq: number): Promise<void> {
  await eventually(() => {
    expect(rig.data()?.items.some((item) => item.seq === seq)).toBe(true);
  });
}

describe('mounted authoritative history refresh', () => {
  test('a live new-message wave fetches after_seq without rereading loaded history', async () => {
    const rig = await fixture(150, true);
    const before = rig.reads().length;
    rig.server.chat.addMessage(channelId, message(151));

    announce(rig, 151);
    rig.clock.pass(1000);
    await settled(rig, 151);

    expect(
      rig
        .reads()
        .slice(before)
        .map((url) => [url.searchParams.get('after_seq'), url.searchParams.get('limit')]),
    ).toEqual([['150', '100']]);
    expect(rig.data()?.items).toHaveLength(151);
    expect(rig.data()?.next_cursor).toBeNull();
    rig.stop();
  });

  test('a live edit or delete rereads the identified cached row', async () => {
    const rig = await fixture(120, true);
    const before = rig.reads().length;
    await editMessage(rig.api, message(1), 'Edited old text');
    await deleteMessage(rig.api, message(2).id);

    rig.receive({ kind: 'message', change: 'updated', channelId, messageId: message(1).id });
    rig.receive({ kind: 'message', change: 'deleted', channelId, messageId: message(2).id });
    rig.clock.pass(1000);
    await eventually(() => {
      expect(rig.data()?.items.find((item) => item.seq === 2)?.deleted).toBe(true);
    });

    expect(rig.data()?.items.find((item) => item.seq === 1)).toMatchObject({ body: 'Edited old text', version: 2 });
    expect(
      rig
        .reads()
        .slice(before)
        .map((url) => [url.searchParams.get('after_seq'), url.searchParams.get('limit')]),
    ).toEqual([
      ['0', '1'],
      ['1', '1'],
    ]);
    rig.stop();
  });

  test('read updates do not fetch conversation history', async () => {
    const rig = await fixture();
    const before = rig.reads().length;

    rig.receive({ kind: 'read', channelId });
    rig.clock.pass(1000);
    await Promise.resolve();

    expect(rig.reads()).toHaveLength(before);
    rig.stop();
  });

  test('events during a running full read cause one fresh follow-up', async () => {
    const rig = await fixture(120, true);
    const pause = rig.server.chat.pauseNextHistoryResponse();
    const full = rig.cache.invalidateQueries({ queryKey: messagesKey(channelId) });
    await pause.answered;
    const before = rig.reads().length;
    rig.server.chat.addMessage(channelId, message(121));

    announce(rig, 121);
    rig.clock.pass(1000);
    pause.resume();
    await full;
    await settled(rig, 121);

    expect(
      rig
        .reads()
        .slice(before)
        .filter((url) => url.searchParams.has('after_seq')),
    ).toHaveLength(1);
    expect(rig.data()?.items).toHaveLength(121);
    rig.stop();
  });

  test('reconnect reconciles an old edit and deletion outside the newest page', async () => {
    const rig = await fixture(120, true);
    await editMessage(rig.api, message(1), 'Changed while disconnected');
    await deleteMessage(rig.api, message(2).id);
    const before = rig.reads().length;

    rig.receive({ kind: 'connected', reconnect: true });
    await eventually(() => {
      expect(rig.data()?.items.find((item) => item.seq === 2)?.deleted).toBe(true);
    });

    expect(rig.data()?.items.find((item) => item.seq === 1)?.body).toBe('Changed while disconnected');
    expect(rig.reads().slice(before)).toHaveLength(3);
    expect(
      rig
        .reads()
        .slice(before)
        .every((url) => !url.searchParams.has('after_seq')),
    ).toBe(true);
    rig.stop();
  });

  test('catchup crosses the 100-item boundary without gaps', async () => {
    const rig = await fixture();
    for (let seq = 51; seq <= 190; seq += 1) rig.server.chat.addMessage(channelId, message(seq));
    const before = rig.reads().length;

    announce(rig, 190);
    rig.clock.pass(1000);
    await settled(rig, 190);

    expect(rig.data()?.items.map((item) => item.seq)).toEqual(Array.from({ length: 190 }, (_, index) => 190 - index));
    expect(
      rig
        .reads()
        .slice(before)
        .map((url) => url.searchParams.get('after_seq')),
    ).toEqual(['50', '150']);
    rig.stop();
  });
});

test('mixed new update delete duplicates and out-of-order frames retain every same-wave intent', async () => {
  const rig = await fixture();
  for (let seq = 51; seq <= 55; seq += 1) rig.server.chat.addMessage(channelId, message(seq));
  await editMessage(rig.api, message(1), 'Mixed edit');
  await deleteMessage(rig.api, message(2).id);
  const before = rig.reads().length;

  announce(rig, 55);
  announce(rig, 53);
  for (let count = 0; count < 2; count += 1) {
    rig.receive({ kind: 'message', change: 'updated', channelId, messageId: message(1).id });
    rig.receive({ kind: 'message', change: 'deleted', channelId, messageId: message(2).id });
  }
  announce(rig, 50);
  rig.clock.pass(1000);
  await settled(rig, 55);

  expect(rig.data()?.items).toHaveLength(55);
  expect(rig.data()?.items.find((item) => item.seq === 1)).toMatchObject({ body: 'Mixed edit', version: 2 });
  expect(rig.data()?.items.find((item) => item.seq === 2)).toMatchObject({
    deleted: true,
    body: null,
    attachments: [],
  });
  expect(rig.reads().slice(before)).toHaveLength(3);
  rig.stop();
});

test('dirty metadata overflow reconciles fully without losing a change', async () => {
  const rig = await fixture(120, true);
  await editMessage(rig.api, message(1), 'Overflow edit');
  rig.server.chat.addMessage(channelId, message(121));
  const before = rig.reads().length;

  for (let seq = 1; seq <= 101; seq += 1)
    rig.receive({ kind: 'message', change: 'updated', channelId, messageId: message(seq).id });
  announce(rig, 121);
  rig.clock.pass(1000);
  await settled(rig, 121);

  expect(rig.data()?.items.find((item) => item.seq === 1)?.body).toBe('Overflow edit');
  expect(rig.data()?.items).toHaveLength(121);
  expect(rig.reads().slice(before)).toHaveLength(3);
  expect(
    rig
      .reads()
      .slice(before)
      .every((url) => !url.searchParams.has('after_seq')),
  ).toBe(true);
  rig.stop();
});

test('incremental refresh overlapping older-page load preserves both coverage and cursor', async () => {
  const rig = await fixture(120);
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const older = rig.owner.older(new AbortController().signal);
  await pause.answered;
  rig.server.chat.addMessage(channelId, message(121));

  announce(rig, 121);
  rig.clock.pass(1000);
  pause.resume();
  await older;
  await settled(rig, 121);

  expect(rig.data()?.items.map((item) => item.seq)).toEqual(Array.from({ length: 101 }, (_, index) => 121 - index));
  expect(rig.data()?.next_cursor).toBe('21');
  rig.stop();
});

test('scope eviction wins over late incremental responses', async () => {
  const rig = await fixture();
  rig.server.chat.addMessage(channelId, message(51));
  const pause = rig.server.chat.pauseNextHistoryResponse();
  announce(rig, 51);
  rig.clock.pass(1000);
  await pause.answered;

  evictChannel(rig.cache, channelId);
  pause.resume();
  await expect(rig.owner.older(new AbortController().signal)).rejects.toMatchObject({ name: 'AbortError' });

  expect(rig.data()).toBeUndefined();
  expect(rig.cache.getQueryData(channelKey(channelId))).toBeUndefined();
  rig.stop();
});

test('an unrelated channel revision does not permanently disable the mounted history owner', async () => {
  const rig = await fixture();
  rig.server.chat.addMessage(channelId, message(51));
  const pause = rig.server.chat.pauseNextHistoryResponse();
  announce(rig, 51);
  rig.clock.pass(1000);
  await pause.answered;

  rig.receive({ kind: 'entity', entity: 'channel', id: 'another-channel' });
  pause.resume();
  await settled(rig, 51);
  rig.server.chat.addMessage(channelId, message(52));
  announce(rig, 52);
  rig.clock.pass(1000);
  await settled(rig, 52);

  expect(rig.data()?.items).toHaveLength(52);
  expect(rig.data()?.items.some((item) => item.seq === 51)).toBe(true);
  rig.stop();
});

test('a held full read cannot republish history after its scope is evicted', async () => {
  const rig = await fixture();
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const full = rig.cache.invalidateQueries({ queryKey: messagesKey(channelId) });
  await pause.answered;

  evictChannel(rig.cache, channelId);
  pause.resume();
  await full;

  expect(rig.data()).toBeUndefined();
  rig.stop();
});

test('account change rejects a withheld old-generation incremental response', async () => {
  const rig = await fixture();
  rig.server.chat.addMessage(channelId, message(51));
  const pause = rig.server.chat.pauseNextHistoryResponse();
  announce(rig, 51);
  rig.clock.pass(1000);
  await pause.answered;
  const other = {
    id: 'another-tutor',
    name: 'Another tutor',
    email: 'another@example.org',
    password: 'another long password',
  };
  rig.server.addAccount(other);
  const login = await rig.server.transport().login(other.email, other.password);
  const token: unknown = await login.json();

  await rig.session.adopt(token);
  rig.cache.clear();
  pause.resume();
  await expect(rig.owner.older(new AbortController().signal)).rejects.toMatchObject({ name: 'AbortError' });

  expect(rig.data()).toBeUndefined();
  rig.stop();
});

test('a failed incremental read falls back to authoritative history without dropping the tail', async () => {
  const rig = await fixture();
  rig.server.chat.addMessage(channelId, message(51));
  rig.server.chat.failNextHistoryRead();
  const before = rig.reads().length;

  announce(rig, 51);
  rig.clock.pass(1000);
  await settled(rig, 51);

  expect(
    rig
      .reads()
      .slice(before)
      .map((url) => url.searchParams.has('after_seq')),
  ).toEqual([true, false, false]);
  expect(rig.data()?.items).toHaveLength(51);
  rig.stop();
});

test('reconnect during a running full read preserves the following fresh authoritative read', async () => {
  const rig = await fixture(120, true);
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const full = rig.cache.invalidateQueries({ queryKey: messagesKey(channelId) });
  await pause.answered;
  await editMessage(rig.api, message(1), 'A reconnect edit');
  rig.server.chat.addMessage(channelId, message(121));
  const before = rig.reads().length;

  announce(rig, 121);
  rig.clock.pass(1000);
  rig.receive({ kind: 'connected', reconnect: true });
  pause.resume();
  await full;
  await eventually(() => {
    expect(rig.data()?.items.some((item) => item.seq === 121)).toBe(true);
  });
  await eventually(() => {
    expect(rig.cache.getQueryState(messagesKey(channelId))?.fetchStatus).toBe('idle');
  });

  expect(rig.data()?.items.find((item) => item.seq === 1)?.body).toBe('A reconnect edit');
  expect(
    rig
      .reads()
      .slice(before)
      .filter((url) => !url.searchParams.has('after_seq')).length,
  ).toBeGreaterThanOrEqual(5);
  rig.stop();
});

test('an active channel scope revision rejects a withheld full response before fresh reconciliation', async () => {
  const rig = await fixture();
  const detailObserver = new QueryObserver(rig.cache, {
    queryKey: channelKey(channelId),
    queryFn: ({ signal }): Promise<Channel> => fetchChannel(rig.api, channelId, signal),
    staleTime: Infinity,
  });
  const unsubscribeDetail = detailObserver.subscribe(() => undefined);
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const before = rig.cache.getQueryState(messagesKey(channelId))?.dataUpdateCount;
  const full = rig.cache.invalidateQueries({ queryKey: messagesKey(channelId) });
  await pause.answered;
  rig.server.chat.addMessage(channelId, message(51));

  rig.receive({ kind: 'entity', entity: 'channel', id: channelId });
  pause.resume();
  await full;
  const heldUpdates = rig.cache.getQueryState(messagesKey(channelId))?.dataUpdateCount;
  rig.clock.pass(1000);
  await settled(rig, 51);

  expect(heldUpdates).toBe(before);
  expect(rig.data()?.items.some((item) => item.seq === 51)).toBe(true);
  expect(rig.data()?.items).toHaveLength(51);
  unsubscribeDetail();
  rig.stop();
});

test('a new frame during a queued full read still performs a fresh follow-up when full already includes its row', async () => {
  const rig = await fixture(120);
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const older = rig.owner.older(new AbortController().signal);
  await pause.answered;
  const full = rig.cache.invalidateQueries({ queryKey: messagesKey(channelId) });
  rig.server.chat.addMessage(channelId, message(121));
  const before = rig.reads().length;

  announce(rig, 121);
  rig.clock.pass(1000);
  pause.resume();
  await older;
  await full;
  await eventually(() => {
    expect(
      rig
        .reads()
        .slice(before)
        .some((url) => url.searchParams.get('after_seq') === '121'),
    ).toBe(true);
  });

  expect(rig.data()?.items.some((item) => item.seq === 121)).toBe(true);
  expect(rig.data()?.items.at(-1)?.seq).toBe(1);
  expect(
    rig
      .reads()
      .slice(before)
      .filter((url) => url.searchParams.has('after_seq')),
  ).toHaveLength(1);
  expect(rig.reads().slice(before)).toHaveLength(4);
  rig.stop();
});

test('a withheld old-scope not-found response reconciles restored membership without evicting active queries', async () => {
  const rig = await fixture();
  const detailObserver = new QueryObserver(rig.cache, {
    queryKey: channelKey(channelId),
    queryFn: ({ signal }): Promise<Channel> => fetchChannel(rig.api, channelId, signal),
    staleTime: Infinity,
  });
  const unsubscribe = detailObserver.subscribe(() => undefined);
  const identity = rig.cache.getQueryCache().find({ queryKey: channelKey(channelId), exact: true });
  rig.server.chat.suspendMember(channelId, account.id, true);
  const pause = rig.server.chat.pauseNextHistoryResponse();
  announce(rig, 51);
  rig.clock.pass(1000);
  await pause.answered;
  await eventually(() => {
    expect(detailObserver.getCurrentResult().isError).toBe(true);
  });
  rig.server.chat.suspendMember(channelId, account.id, false);
  rig.server.chat.addMessage(channelId, message(51));

  rig.receive({ kind: 'entity', entity: 'channel', id: channelId });
  rig.clock.pass(1000);
  await eventually(() => {
    expect(detailObserver.getCurrentResult().isSuccess).toBe(true);
    expect(detailObserver.getCurrentResult().isFetching).toBe(false);
  });
  pause.resume();
  await settled(rig, 51);
  rig.clock.pass(1000);
  await rig.owner.older(new AbortController().signal);

  expect(rig.cache.getQueryCache().find({ queryKey: channelKey(channelId), exact: true })).toBe(identity);
  expect(rig.cache.getQueryData(channelKey(channelId))).toBeDefined();
  expect(rig.data()?.items).toHaveLength(51);
  expect(
    rig
      .reads()
      .slice(2)
      .some((url) => !url.searchParams.has('after_seq')),
  ).toBe(true);
  unsubscribe();
  rig.stop();
});

test('a not-found incremental response in the current scope evicts the unavailable channel', async () => {
  const rig = await fixture();
  rig.server.chat.suspendMember(channelId, account.id, true);
  const pause = rig.server.chat.pauseNextHistoryResponse();
  announce(rig, 51);
  rig.clock.pass(1000);
  await pause.answered;

  pause.resume();
  await eventually(() => {
    expect(rig.data()).toBeUndefined();
  });

  expect(rig.cache.getQueryData(channelKey(channelId))).toBeUndefined();
  rig.stop();
});

test('cancelling a queued live wave discards it before its history lease starts', async () => {
  const rig = await fixture(120);
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const older = rig.owner.older(new AbortController().signal);
  await pause.answered;
  rig.server.chat.addMessage(channelId, message(121));
  const before = rig.reads().length;
  announce(rig, 121);
  rig.clock.pass(1000);

  rig.owner.cancel();
  pause.resume();
  await older;
  await rig.owner.older(new AbortController().signal);

  expect(rig.data()?.items.some((item) => item.seq === 121)).toBe(false);
  expect(
    rig
      .reads()
      .slice(before)
      .every((url) => !url.searchParams.has('after_seq')),
  ).toBe(true);
  rig.stop();
});

test('a new wave after queued-wave cancellation runs with its own pending identity', async () => {
  const rig = await fixture(120);
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const older = rig.owner.older(new AbortController().signal);
  await pause.answered;
  rig.server.chat.addMessage(channelId, message(121));
  announce(rig, 121);
  rig.clock.pass(1000);
  rig.owner.cancel();
  rig.server.chat.addMessage(channelId, message(122));
  const before = rig.reads().length;

  announce(rig, 122);
  rig.clock.pass(1000);
  pause.resume();
  await older;
  await settled(rig, 122);

  expect(
    rig
      .data()
      ?.items.slice(0, 2)
      .map((item) => item.seq),
  ).toEqual([122, 121]);
  expect(rig.data()?.next_cursor).toBe('21');
  expect(
    rig
      .reads()
      .slice(before)
      .map((url) => url.searchParams.get('after_seq')),
  ).toEqual(['120']);
  rig.stop();
});

test('reconnect after an unrelated scope revision refreshes a withheld full history read', async () => {
  const rig = await fixture(120, true);
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const before = rig.reads().length;
  const full = rig.cache.invalidateQueries({ queryKey: messagesKey(channelId) });
  await pause.answered;
  await editMessage(rig.api, message(1), 'Fresh after unrelated scope change');
  await deleteMessage(rig.api, message(2).id);
  rig.server.chat.addMessage(channelId, message(121));

  try {
    rig.receive({ kind: 'entity', entity: 'channel', id: 'another-channel' });
    rig.clock.pass(1000);
    rig.receive({ kind: 'connected', reconnect: true });
    pause.resume();
    await full;
    await eventually(() => {
      expect(rig.cache.getQueryState(messagesKey(channelId))?.fetchStatus).toBe('idle');
    });
    rig.clock.pass(1000);
    await eventually(() => {
      expect(rig.cache.getQueryState(messagesKey(channelId))?.status).toBe('success');
      expect(rig.cache.getQueryState(messagesKey(channelId))?.fetchStatus).toBe('idle');
    });

    expect({
      status: rig.cache.getQueryState(messagesKey(channelId))?.status,
      errorKind: rig.cache.getQueryState(messagesKey(channelId))?.error?.name ?? null,
      historyReads: rig.reads().length - before,
      oldDeletionPresent: rig.data()?.items.find((item) => item.seq === 2)?.deleted,
      newRowPresent: rig.data()?.items.some((item) => item.seq === 121),
      oldEditPresent: rig.data()?.items.find((item) => item.seq === 1)?.body === 'Fresh after unrelated scope change',
    }).toMatchObject({
      status: 'success',
      errorKind: null,
      newRowPresent: true,
      oldEditPresent: true,
      oldDeletionPresent: true,
    });
  } finally {
    pause.resume();
    rig.stop();
  }
});

test('an unrelated scope revision schedules one full repair after publication without reconnect', async () => {
  const rig = await fixture(120, true);
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const before = rig.reads().length;
  const full = rig.cache.invalidateQueries({ queryKey: messagesKey(channelId) });
  await pause.answered;
  await editMessage(rig.api, message(1), 'Repair without reconnect');
  await deleteMessage(rig.api, message(2).id);
  rig.server.chat.addMessage(channelId, message(121));
  const published: boolean[] = [];
  const historyQuery = rig.cache.getQueryCache().find({ queryKey: messagesKey(channelId), exact: true });
  const unsubscribe = rig.cache.getQueryCache().subscribe((event) => {
    if (event.query === historyQuery && event.type === 'updated' && event.action.type === 'success')
      published.push(rig.data()?.items.some((item) => item.seq === 121) === true);
  });

  rig.receive({ kind: 'entity', entity: 'channel', id: 'another-channel' });
  rig.clock.pass(1000);
  pause.resume();
  await full;
  const failedReadCount = rig.reads().length - before;
  const beforeRepairPublications = [...published];
  rig.clock.pass(1000);
  await settled(rig, 121);

  expect(failedReadCount).toBe(3);
  expect(beforeRepairPublications).toEqual([]);
  expect(published).toEqual([true]);
  expect(rig.reads().length - before).toBe(6);
  expect(rig.data()?.items.find((item) => item.seq === 1)?.body).toBe('Repair without reconnect');
  expect(rig.data()?.items.find((item) => item.seq === 2)?.deleted).toBe(true);
  unsubscribe();
  rig.stop();
});

test('repeated scope changes retain one scheduled full obligation for each settled read', async () => {
  const rig = await fixture();
  const firstPause = rig.server.chat.pauseNextHistoryResponse();
  const first = rig.cache.invalidateQueries({ queryKey: messagesKey(channelId) });
  await firstPause.answered;
  rig.server.chat.addMessage(channelId, message(51));
  rig.receive({ kind: 'entity', entity: 'channel', id: 'another-channel' });
  rig.clock.pass(1000);
  firstPause.resume();
  await first;
  const firstCount = rig.reads().length;
  const secondPause = rig.server.chat.pauseNextHistoryResponse();

  rig.clock.pass(1000);
  await secondPause.answered;
  rig.server.chat.addMessage(channelId, message(52));
  for (let count = 0; count < 5; count += 1) rig.receive({ kind: 'entity', entity: 'channel', id: 'another-channel' });
  rig.clock.pass(1000);
  secondPause.resume();
  await eventually(() => {
    expect(rig.cache.getQueryState(messagesKey(channelId))?.fetchStatus).toBe('idle');
  });
  const secondCount = rig.reads().length;
  rig.clock.pass(1000);
  await settled(rig, 52);

  expect(firstCount).toBe(2);
  expect(secondCount).toBe(4);
  expect(rig.reads()).toHaveLength(6);
  expect(rig.cache.getQueryState(messagesKey(channelId))?.status).toBe('success');
  expect(rig.data()?.items.some((item) => item.seq === 51)).toBe(true);
  rig.stop();
});

test('page suspension retains a full repair but starts no owned reads until resumed', async () => {
  const rig = await fixture();
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const full = rig.cache.invalidateQueries({ queryKey: messagesKey(channelId) });
  await pause.answered;
  rig.server.chat.addMessage(channelId, message(51));
  rig.receive({ kind: 'entity', entity: 'channel', id: 'another-channel' });
  rig.clock.pass(1000);

  rig.receive.suspend();
  pause.resume();
  await full;
  const before = rig.reads().length;
  rig.clock.pass(5000);
  const suspendedReads = rig.reads().length;
  rig.receive.resume();
  rig.clock.pass(1000);
  await settled(rig, 51);

  expect(suspendedReads).toBe(before);
  expect(rig.reads().length - before).toBe(2);
  expect(rig.cache.getQueryState(messagesKey(channelId))?.status).toBe('success');
  rig.stop();
});

test('a history owner mounted during suspension defers its full repair until resume', async () => {
  const rig = await fixture(50, false, true);
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const full = rig.cache.invalidateQueries({ queryKey: messagesKey(channelId) });
  await pause.answered;
  rig.server.chat.addMessage(channelId, message(51));
  evictChannel(rig.cache, 'another-channel', true);

  pause.resume();
  await full;
  const before = rig.reads().length;
  rig.clock.pass(5000);
  const suspendedReads = rig.reads().length;
  rig.receive.resume();
  rig.clock.pass(1000);
  await settled(rig, 51);

  expect(suspendedReads).toBe(before);
  expect(rig.reads().length - before).toBe(2);
  rig.stop();
});

test('unmount after full scope drift cancels the scheduled repair without new reads', async () => {
  const rig = await fixture();
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const full = rig.cache.invalidateQueries({ queryKey: messagesKey(channelId) });
  await pause.answered;
  rig.receive({ kind: 'entity', entity: 'channel', id: 'another-channel' });
  rig.clock.pass(1000);
  pause.resume();
  await full;
  const before = rig.reads().length;

  rig.stop();
  rig.clock.pass(5000);
  await Promise.resolve();

  expect(rig.reads()).toHaveLength(before);
  expect(rig.data()).toBeUndefined();
});

test('reconnect after failed full publication preserves the scheduled authoritative obligation', async () => {
  const rig = await fixture();
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const full = rig.cache.invalidateQueries({ queryKey: messagesKey(channelId) });
  await pause.answered;
  rig.server.chat.addMessage(channelId, message(51));
  rig.receive({ kind: 'entity', entity: 'channel', id: 'another-channel' });
  rig.clock.pass(1000);
  pause.resume();
  await full;
  const before = rig.reads().length;

  rig.receive({ kind: 'connected', reconnect: true });
  await settled(rig, 51);
  await eventually(() => {
    expect(rig.cache.getQueryState(messagesKey(channelId))?.fetchStatus).toBe('idle');
  });
  rig.clock.pass(1000);
  await rig.owner.older(new AbortController().signal);

  expect(rig.reads().length - before).toBe(2);
  expect(rig.cache.getQueryState(messagesKey(channelId))?.status).toBe('success');
  rig.stop();
});

test('account replacement after full scope drift prevents the queued repair from starting', async () => {
  const rig = await fixture();
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const full = rig.cache.invalidateQueries({ queryKey: messagesKey(channelId) });
  await pause.answered;
  rig.receive({ kind: 'entity', entity: 'channel', id: 'another-channel' });
  rig.clock.pass(1000);
  pause.resume();
  await full;
  const other = {
    id: 'repair-other-tutor',
    name: 'Other tutor',
    email: 'repair-other@example.org',
    password: 'another long password',
  };
  rig.server.addAccount(other);
  const login = await rig.server.transport().login(other.email, other.password);
  const token: unknown = await login.json();
  const before = rig.reads().length;

  await rig.session.adopt(token);
  rig.clock.pass(5000);
  await expect(rig.owner.older(new AbortController().signal)).rejects.toMatchObject({ name: 'AbortError' });

  expect(rig.reads()).toHaveLength(before);
  expect(rig.cache.getQueryState(messagesKey(channelId))?.status).toBe('error');
  rig.stop();
});

test('a failed authoritative repair reports its real error without scheduling automatic retries', async () => {
  const rig = await fixture();
  const pause = rig.server.chat.pauseNextHistoryResponse();
  const full = rig.cache.invalidateQueries({ queryKey: messagesKey(channelId) });
  await pause.answered;
  rig.receive({ kind: 'entity', entity: 'channel', id: 'another-channel' });
  rig.clock.pass(1000);
  pause.resume();
  await full;
  rig.server.chat.failNextHistoryRead();

  rig.clock.pass(1000);
  await eventually(() => {
    expect(rig.cache.getQueryState(messagesKey(channelId))?.error).toMatchObject({ status: 503 });
    expect(rig.cache.getQueryState(messagesKey(channelId))?.fetchStatus).toBe('idle');
  });
  const before = rig.reads().length;
  rig.clock.pass(5000);
  await rig.owner.older(new AbortController().signal);

  expect(rig.reads()).toHaveLength(before);
  expect(rig.cache.getQueryState(messagesKey(channelId))?.status).toBe('error');
  rig.stop();
});
