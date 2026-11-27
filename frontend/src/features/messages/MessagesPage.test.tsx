import { QueryClient, QueryClientProvider, skipToken } from '@tanstack/react-query';
import { StrictMode } from 'react';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { FollowingQueryClient } from '../../app/queryClient.ts';
import { SignedInFrame } from '../../components/frame/SignedInFrame.tsx';
import type { FakeInitialReadObservation } from '../../lib/api/fakeChat.ts';
import { memoryPreferenceStore } from '../../lib/preferences/store.ts';
import { liveRefetch } from '../../app/liveRefetch.ts';
import { SchedulerContext } from '../../app/clock.ts';
import { ConnectionContext, ConnectionState } from '../../app/connectionState.ts';
import {
  type MediaQueries,
  MediaQueriesContext,
  PreferencesContext,
  fixedMediaQueries,
  phoneLayoutQuery,
  wideLayoutQuery,
} from '../../app/context.ts';
import { type ApiClient, ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { formatDate } from '../../lib/format/dates.ts';
import { type Channel, type Message, type MessagePage, deleteMessage, editMessage } from './api.ts';
import { channelsScopeRevisionKey, channelScopeRevision, evictChannel } from './cache.ts';
import { channelPageKey, aroundKey, aroundPrefix, channelKey, channelsKey, messagesKey } from './keys.ts';
import { DeleteMessageForm, MessageKeyContext } from './Conversation.tsx';
import { chatTexts } from './texts.en.ts';
import { MessagesPage } from './MessagesPage.tsx';
import { MessageBody } from './MessageBody.tsx';

const origin = 'https://ovc.example';
const tutor = { id: 'tutor-1', name: 'Tutor', email: 'tutor@example.org', password: 'a long password' };
const channel: Channel = {
  id: 'channel-1',
  kind: 'direct',
  label: 'Student',
  label_kind: null,
  colour: 'moss',
  last_message: null,
  online: false,
  tutor: null,
  unconfirmed: false,
  unread_count: 0,
  pinned: null,
  peer_read_seq: null,
  user_id: 'student-1',
  writable: true,
  relationship_id: 'card-1',
};
function message(seq: number): Message & { readonly senderId: string } {
  return {
    id: `message-${String(seq)}`,
    seq,
    body: `Message ${String(seq)}`,
    attachments: [],
    created_at: '2026-11-01T00:00:00Z',
    deleted: false,
    erased: false,
    edited_at: null,
    own: true,
    sender_label: 'Tutor',
    sender_label_kind: null,
    sent_before_reclaim: false,
    reply_to: null,
    reactions: [],
    version: 1,
    senderId: tutor.id,
  };
}

async function fixture(
  count = 1,
  address = '/messages/channel-1',
  wide = true,
  options: {
    readonly role?: 'tutor' | 'student';
    readonly reclaimed?: boolean;
    readonly initialReadMode?: 'strict' | 'single';
    readonly followingQueryClient?: boolean;
    readonly mediaQueries?: MediaQueries;
    readonly kind?: Channel['kind'];
    readonly seeds?: readonly (Message & { readonly senderId: string; readonly replyTo?: string })[];
  } = {},
): Promise<{
  readonly server: FakeAuthServer;
  readonly initialReads: FakeInitialReadObservation | null;
  readonly api: ApiClient;
  readonly session: Session;
  readonly cache: QueryClient;
  readonly clock: ReturnType<typeof manualScheduler>;
  readonly connection: ConnectionState;
  readonly router: ReturnType<typeof createMemoryRouter>;
}> {
  const server = new FakeAuthServer();
  server.addAccount({ ...tutor, role: options.role ?? 'tutor' });
  server.chat.addChannel({
    channel: { ...channel, kind: options.kind ?? channel.kind },
    members: [{ accountId: tutor.id }, { accountId: 'student-1' }],
  });
  for (let seq = 1; seq <= count; seq += 1)
    server.chat.addMessage(channel.id, { ...message(seq), sent_before_reclaim: options.reclaimed ?? false });
  for (const seed of options.seeds ?? []) server.chat.addMessage(channel.id, seed);
  const clock = manualScheduler();
  const session = new Session({ origin, transport: server.transport(), locks: memoryLocks(), scheduler: clock });
  await session.signIn(tutor.email, tutor.password);
  const connection = new ConnectionState(clock);
  connection.account(1);
  connection.readiness(1, true);
  const queryOptions = { defaultOptions: { queries: { retry: false } } };
  const cache =
    options.initialReadMode === undefined && options.followingQueryClient !== true
      ? new QueryClient(queryOptions)
      : new FollowingQueryClient(queryOptions);
  const initialReads =
    options.initialReadMode === undefined ? null : server.chat.observeInitialReads(origin, channel.id);
  const state = session.snapshot();
  if (state.kind !== 'signedIn') throw new Error('causal_component_account_not_signed_in');
  const router = createMemoryRouter(
    [
      {
        path: '/messages/:channelId?',
        element:
          options.initialReadMode === undefined ? (
            <main>
              <MessagesPage />
            </main>
          ) : (
            <SignedInFrame me={state.me}>
              <MessagesPage />
            </SignedInFrame>
          ),
        handle: { title: 'page.messages' },
      },
      {
        path: '/elsewhere',
        element: (
          <main>
            <h1>Elsewhere</h1>
          </main>
        ),
      },
    ],
    { initialEntries: [address] },
  );
  const i18n = await createI18n('en');
  const api = createApiClient(origin, session.authorizedFetch);
  let issued = 0;
  const tree = (
    <PreferencesContext value={memoryPreferenceStore()}>
      <SessionContext value={session}>
        <ApiClientContext value={api}>
          <QueryClientProvider client={cache}>
            <I18nextProvider i18n={i18n}>
              <SchedulerContext value={clock}>
                <ConnectionContext value={connection}>
                  <MediaQueriesContext
                    value={
                      options.mediaQueries ??
                      fixedMediaQueries(
                        wide ? (options.initialReadMode === undefined ? [] : [wideLayoutQuery]) : [phoneLayoutQuery],
                      )
                    }
                  >
                    <MessageKeyContext
                      value={() => {
                        issued += 1;
                        return `send-${String(issued)}`;
                      }}
                    >
                      <RouterProvider router={router} />
                    </MessageKeyContext>
                  </MediaQueriesContext>
                </ConnectionContext>
              </SchedulerContext>
            </I18nextProvider>
          </QueryClientProvider>
        </ApiClientContext>
      </SessionContext>
    </PreferencesContext>
  );
  render(options.initialReadMode === 'strict' ? <StrictMode>{tree}</StrictMode> : tree);
  await screen.findByRole('heading', { level: address.split('?')[0] === '/messages' ? 1 : 2 });
  return { server, initialReads, api, cache, clock, connection, router, session };
}

function switchableMediaQueries(matching: readonly string[]): {
  readonly queries: MediaQueries;
  readonly match: (next: readonly string[]) => void;
} {
  let current = matching;
  const listeners = new Set<() => void>();
  return {
    queries: (query) => ({
      subscribe: (onChange) => {
        listeners.add(onChange);
        return (): void => {
          listeners.delete(onChange);
        };
      },
      snapshot: () => current.includes(query),
    }),
    match: (next): void => {
      current = next;
      for (const listener of listeners) listener();
    },
  };
}

function seedChannels(server: FakeAuthServer, count: number): void {
  for (let index = count; index >= 1; index -= 1) {
    server.chat.addChannel({
      channel: { ...channel, id: `pagination-${String(index)}`, label: `Chat ${String(index)}` },
      members: [{ accountId: tutor.id }],
    });
  }
}

afterEach(cleanup);

test.each(['strict', 'single'] as const)(
  'the real Messages and frame query lifetimes prove all initial targets under %s mounting',
  async (mode) => {
    const expectedAttempts = mode === 'strict' ? 2 : 1;

    const result = await fixture(1, '/messages/channel-1', true, { initialReadMode: mode });
    const observed = result.initialReads;
    if (observed === null) throw new Error('causal_observer_not_armed');
    await screen.findByRole('textbox', { name: 'Message' });
    await waitFor(() => {
      expect(observed.snapshot().every((entry) => entry.totalAttempts >= expectedAttempts)).toBe(true);
    });
    const snapshot = observed.snapshot();
    observed.stop();

    expect(snapshot.map((entry) => entry.target)).toEqual(['unread', 'list', 'current_channel']);
    for (const entry of snapshot) {
      expect(entry.retainedLifetimes).toBe(expectedAttempts);
      expect(entry.firstAborted).toBe(mode === 'strict');
      expect(entry.firstAbortedAtSecondInvocation).toBe(mode === 'strict' ? true : null);
      expect(entry.secondAborted).toBe(false);
    }
    for (const entry of snapshot)
      console.info(JSON.stringify({ event: 'component.initial_read_lifetime', mount: mode, ...entry }));
  },
);

test.each(['strict', 'single'] as const)('real Link destination query producer under %s mounting', async (mode) => {
  const result = await fixture(1, '/messages/channel-1', true, { initialReadMode: mode });
  const destination = { ...channel, id: 'channel-destination', label: 'Destination conversation' };
  result.server.chat.addChannel({
    channel: destination,
    members: [{ accountId: tutor.id }, { accountId: 'student-1' }],
  });
  result.server.chat.addMessage(destination.id, message(2));
  await result.cache.invalidateQueries({ queryKey: channelsKey });
  const link = await screen.findByRole('link', { name: /Destination conversation/ });
  await screen.findByRole('textbox', { name: 'Message' });
  const observed = result.server.chat.observeDestinationDetail(origin, destination.id);

  fireEvent.click(link);
  await screen.findByRole('heading', { name: 'Destination conversation' });
  await screen.findByText('Message 2');
  const snapshot = observed.snapshot();
  observed.stop();

  expect(snapshot[0]).toMatchObject({
    totalAttempts: mode === 'strict' ? 2 : 1,
    retainedLifetimes: mode === 'strict' ? 2 : 1,
    firstAborted: mode === 'strict',
    secondAborted: false,
    firstAbortedAtSecondInvocation: mode === 'strict' ? true : null,
  });
  console.info(JSON.stringify({ event: 'component.destination_read_lifetime', mount: mode, ...snapshot[0] }));
});

test('the internal scope observer cannot fetch and eviction still advances the observed revision', async () => {
  const { cache } = await fixture(1);
  const observer = cache.getQueryCache().find({ queryKey: channelsScopeRevisionKey, exact: true });
  const before = channelScopeRevision(cache);

  await act(async () => {
    await cache.refetchQueries(
      { queryKey: channelsScopeRevisionKey, exact: true, type: 'all' },
      { throwOnError: true },
    );
    evictChannel(cache, channel.id);
  });

  expect(observer?.options.queryFn).toBe(skipToken);
  expect(observer?.state.fetchStatus).toBe('idle');
  expect(channelScopeRevision(cache)).toBe(before + 1);
  expect(cache.getQueryData(channelKey(channel.id))).toBeUndefined();
  expect(cache.getQueryData(messagesKey(channel.id))).toBeUndefined();
});

describe('message body', () => {
  test('markup and unsafe URL candidates stay text while plain http and https link safely', () => {
    const body =
      '<img src=x> javascript:alert(1) https://user:secret@example.org https://exa\u200bmple.org http://example.org/a https://example.org/b';

    const { container } = render(<MessageBody body={body} />);

    expect(container.querySelector('img')).toBeNull();
    expect(container.textContent).toBe(body);
    expect(screen.getAllByRole('link').map((link) => link.getAttribute('href'))).toEqual([
      'http://example.org/a',
      'https://example.org/b',
    ]);
    for (const link of screen.getAllByRole('link')) {
      expect(link.getAttribute('rel')).toBe('noopener noreferrer nofollow');
      expect(link.getAttribute('target')).toBe('_blank');
    }
  });
});

describe('Messages', () => {
  test.each([
    { role: 'tutor' as const, wide: true },
    { role: 'student' as const, wide: false },
  ])('channel show more focuses its first new link for $role', async ({ role, wide }) => {
    const { server, cache } = await fixture(0, '/messages', wide, { role });
    seedChannels(server, 60);
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelsKey });
    });
    const more = await screen.findByRole('button', { name: 'Show more' });
    more.focus();

    fireEvent.click(more);
    const firstAdded = await screen.findByRole('link', { name: 'Chat 31' });
    const intermediateFocus = document.activeElement;
    const lastMore = await screen.findByRole('button', { name: 'Show more' });
    lastMore.focus();
    fireEvent.click(lastMore);
    const finalAdded = await screen.findByRole('link', { name: 'Student' });

    expect(intermediateFocus).toBe(firstAdded);
    expect(document.activeElement).toBe(finalAdded);
    expect(screen.queryByRole('button', { name: 'Show more' })).toBeNull();
  });

  test('final channel page without a new visible row restores focus to the last link', async () => {
    const { server, cache } = await fixture(0, '/messages');
    seedChannels(server, 31);
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelsKey });
    });
    const more = await screen.findByRole('button', { name: 'Show more' });
    for (let index = 30; index <= 31; index += 1) server.chat.removeMember(`pagination-${String(index)}`, tutor.id);
    server.chat.removeMember(channel.id, tutor.id);
    more.focus();

    fireEvent.click(more);
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Show more' })).toBeNull();
    });

    expect(document.activeElement).toBe(screen.getByRole('link', { name: 'Chat 30' }));
  });

  test('channel continuation retry focuses only its new rows and retains the loading control', async () => {
    const { server, cache } = await fixture(0, '/messages');
    seedChannels(server, 31);
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelsKey });
    });
    const more = await screen.findByRole('button', { name: 'Show more' });
    const pause = server.chat.pauseNextChannelListResponse();
    more.focus();

    fireEvent.click(more);
    await pause.answered;
    const loadingFocus = document.activeElement;
    const loadingDisabled = more.getAttribute('aria-disabled');
    const loadingBusy = more.getAttribute('aria-busy');
    const loadingSpinner = more.querySelector('[aria-hidden="true"]');
    fireEvent.click(more);
    await act(async () => {
      pause.resume();
      await Promise.resolve();
    });
    const firstAdded = await screen.findByRole('link', { name: 'Chat 31' });

    expect(loadingFocus).toBe(more);
    expect(loadingDisabled).toBe('true');
    expect(loadingBusy).toBe('true');
    expect(loadingSpinner?.classList.contains('animate-spin')).toBe(true);
    expect(document.activeElement).toBe(firstAdded);
  });

  test('channel continuation retry focuses only its new rows after a refusal', async () => {
    const { server, cache } = await fixture(0, '/messages');
    seedChannels(server, 31);
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelsKey });
    });
    server.chat.failNextChannelListRead();
    const more = await screen.findByRole('button', { name: 'Show more' });
    more.focus();

    fireEvent.click(more);
    await screen.findByRole('alert');
    const refusalFocus = document.activeElement;
    const refusalBusy = more.getAttribute('aria-busy');
    const refusalSpinner = more.querySelector('[aria-hidden="true"]');
    const retry = screen.getByRole('button', { name: 'Try again' });
    retry.focus();
    fireEvent.click(retry);
    const firstAdded = await screen.findByRole('link', { name: 'Chat 31' });

    expect(refusalFocus).toBe(more);
    expect(refusalBusy).toBe('false');
    expect(refusalSpinner).toBeNull();
    expect(document.activeElement).toBe(firstAdded);
  });

  test('raw search changes discard pagination focus before the query debounce', async () => {
    const { server, cache, clock } = await fixture(0, '/messages');
    seedChannels(server, 60);
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelsKey });
    });
    const pause = server.chat.pauseNextChannelListResponse();
    const more = await screen.findByRole('button', { name: 'Show more' });
    more.focus();

    fireEvent.click(more);
    await pause.answered;
    const search = screen.getByRole('searchbox', { name: 'Search chats' });
    search.focus();
    fireEvent.change(search, { target: { value: 'Chat 1' } });
    await act(async () => {
      pause.resume();
      await Promise.resolve();
    });
    const oldRow = await screen.findByRole('link', { name: 'Chat 31' });
    const focusBeforeDebounce = document.activeElement;
    act(() => {
      clock.pass(200);
    });
    await screen.findByRole('link', { name: 'Chat 19' });

    expect(focusBeforeDebounce).toBe(search);
    expect(focusBeforeDebounce).not.toBe(oldRow);
    expect(document.activeElement).toBe(search);
    expect(screen.queryByRole('link', { name: 'Chat 31' })).toBeNull();
  });

  test('a superseded channel continuation cannot steal focus', async () => {
    const { server, cache, clock } = await fixture(0, '/messages');
    seedChannels(server, 60);
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelsKey });
    });
    const pause = server.chat.pauseNextChannelListResponse();
    const more = await screen.findByRole('button', { name: 'Show more' });
    more.focus();

    fireEvent.click(more);
    await pause.answered;
    const search = screen.getByRole('searchbox', { name: 'Search chats' });
    search.focus();
    fireEvent.change(search, { target: { value: 'Chat 1' } });
    act(() => {
      clock.pass(200);
    });
    await screen.findByRole('link', { name: 'Chat 19' });
    await act(async () => {
      pause.resume();
      await Promise.resolve();
    });

    expect(document.activeElement).toBe(search);
    expect(screen.queryByRole('link', { name: 'Chat 31' })).toBeNull();
  });

  test('show older focuses the first newly displayed message', async () => {
    await fixture(55);
    const older = await screen.findByRole('button', { name: 'Show older' });

    fireEvent.click(older);
    const first = await screen.findByText('Message 1');

    await waitFor(() => {
      expect(document.activeElement).toBe(first.closest('li'));
    });
    expect(screen.getByText('Message 55')).not.toBeNull();
  });

  test('menu opens on its first action and Escape returns to its button', async () => {
    await fixture();
    const menu = await screen.findByRole('button', { name: 'Message actions' });

    fireEvent.click(menu);
    const reply = screen.getByRole('button', { name: 'Reply' });
    const focusedFirst = document.activeElement;
    fireEvent.keyDown(reply, { key: 'Escape' });

    expect(focusedFirst).toBe(reply);
    expect(document.activeElement).toBe(menu);
    expect(screen.queryByRole('button', { name: 'Reply' })).toBeNull();
  });

  test('React opens the picker on its first reaction and choosing one shows a marked chip and returns to the actions', async () => {
    await fixture();
    const menu = await screen.findByRole('button', { name: 'Message actions' });
    fireEvent.click(menu);

    fireEvent.click(screen.getByRole('button', { name: 'React' }));
    const first = screen.getByRole('button', { name: 'Thumbs up' });
    const focusedFirst = document.activeElement;
    fireEvent.click(screen.getByRole('button', { name: 'Heart' }));
    const chip = await screen.findByRole('button', { name: /Heart\s*1\s*your reaction/ });

    expect(focusedFirst).toBe(first);
    expect(chip.getAttribute('aria-pressed')).toBe('true');
    expect(document.activeElement).toBe(menu);
  });

  test('Escape in the reaction picker returns focus to React', async () => {
    await fixture();
    fireEvent.click(await screen.findByRole('button', { name: 'Message actions' }));
    const react = screen.getByRole('button', { name: 'React' });
    fireEvent.click(react);

    fireEvent.keyDown(screen.getByRole('button', { name: 'Thumbs up' }), { key: 'Escape' });

    expect(document.activeElement).toBe(react);
    expect(screen.queryByRole('button', { name: 'Thumbs up' })).toBeNull();
  });

  test('a chip toggles the caller\u2019s own reaction in a writable chat', async () => {
    const { server, cache, clock } = await fixture();
    await screen.findByText('Message 1');
    server.chat.memberReact('message-1', tutor.id, 'check');
    server.chat.memberReact('message-1', 'student-1', 'check');
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);
    act(() => {
      receive({ kind: 'message', change: 'updated', channelId: channel.id, messageId: 'message-1' });
      clock.pass(1000);
    });
    const chip = await screen.findByRole('button', { name: /Done\s*2\s*your reaction/ });

    fireEvent.click(chip);

    expect(await screen.findByRole('button', { name: /^Done\s*1(?!\d)(?!.*your reaction)/ })).toBeTruthy();
  });

  test('React focuses the current reaction and Remove reaction removes the chip and returns to the actions', async () => {
    const { server, cache, clock } = await fixture();
    await screen.findByText('Message 1');
    server.chat.memberReact('message-1', tutor.id, 'sad');
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);
    act(() => {
      receive({ kind: 'message', change: 'updated', channelId: channel.id, messageId: 'message-1' });
      clock.pass(1000);
    });
    await screen.findByRole('button', { name: /Sad\s*1\s*your reaction/ });
    const menu = screen.getByRole('button', { name: 'Message actions' });
    fireEvent.click(menu);

    fireEvent.click(screen.getByRole('button', { name: 'React' }));
    const focusedCurrent = document.activeElement;
    fireEvent.click(screen.getByRole('button', { name: 'Remove reaction' }));
    await waitFor(() => {
      expect(screen.queryByRole('list', { name: 'Reactions' })).toBeNull();
    });

    expect(focusedCurrent?.getAttribute('aria-label')).toBe('Sad');
    expect(focusedCurrent?.getAttribute('aria-pressed')).toBe('true');
    expect(document.activeElement).toBe(menu);
  });

  test('toggling off the last chip of a kind moves focus to its message', async () => {
    const { server, cache, clock } = await fixture();
    await screen.findByText('Message 1');
    server.chat.memberReact('message-1', tutor.id, 'heart');
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);
    act(() => {
      receive({ kind: 'message', change: 'updated', channelId: channel.id, messageId: 'message-1' });
      clock.pass(1000);
    });
    const chip = await screen.findByRole('button', { name: /Heart\s*1\s*your reaction/ });
    chip.focus();

    fireEvent.click(chip);
    await waitFor(() => {
      expect(screen.queryByRole('list', { name: 'Reactions' })).toBeNull();
    });

    expect(document.activeElement).toBe(screen.getByText('Message 1').closest('li'));
  });

  test('toggling a chip whose kind remains keeps focus on it', async () => {
    const { server, cache, clock } = await fixture();
    await screen.findByText('Message 1');
    server.chat.memberReact('message-1', tutor.id, 'heart');
    server.chat.memberReact('message-1', 'student-1', 'heart');
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);
    act(() => {
      receive({ kind: 'message', change: 'updated', channelId: channel.id, messageId: 'message-1' });
      clock.pass(1000);
    });
    const chip = await screen.findByRole('button', { name: /Heart\s*2\s*your reaction/ });
    chip.focus();

    fireEvent.click(chip);
    await screen.findByRole('button', { name: /^Heart\s*1(?!\d)(?!.*your reaction)/ });

    expect(document.activeElement).toBe(chip);
  });

  test('in a window around an older message a chip whose kind remains keeps focus', async () => {
    const { server } = await fixture(60, '/messages/channel-1', true, {
      seeds: [{ ...message(61), body: 'Reply far back', replyTo: 'message-1' }],
    });
    await screen.findByText('Reply far back');
    server.chat.memberReact('message-1', tutor.id, 'heart');
    server.chat.memberReact('message-1', 'student-1', 'heart');
    fireEvent.click(within(rowOf('Reply far back')).getByRole('button', { name: /Message 1$/ }));
    const chip = await screen.findByRole('button', { name: /Heart\s*2\s*your reaction/ });
    chip.focus();

    fireEvent.click(chip);
    await screen.findByRole('button', { name: /^Heart\s*1(?!\d)(?!.*your reaction)/ });

    expect(chip.closest('[hidden]')).toBeNull();
    expect(document.activeElement).toBe(chip);
  });

  test('in a window around an older message toggling off the last chip of a kind focuses that message', async () => {
    const { server } = await fixture(60, '/messages/channel-1', true, {
      seeds: [{ ...message(61), body: 'Reply far back', replyTo: 'message-1' }],
    });
    await screen.findByText('Reply far back');
    server.chat.memberReact('message-1', tutor.id, 'heart');
    fireEvent.click(within(rowOf('Reply far back')).getByRole('button', { name: /Message 1$/ }));
    const chip = await screen.findByRole('button', { name: /Heart\s*1\s*your reaction/ });
    chip.focus();

    fireEvent.click(chip);
    await waitFor(() => {
      expect(screen.queryByRole('list', { name: 'Reactions' })).toBeNull();
    });

    expect(document.activeElement?.getAttribute('data-row')).toBe('message-1');
    expect(document.activeElement?.closest('[hidden]')).toBeNull();
  });

  test('after leaving a window around an older message, toggling off the last chip of a kind still focuses that message', async () => {
    const { server, cache, clock } = await fixture(60, '/messages/channel-1', true, {
      seeds: [{ ...message(61), body: 'Reply far back', replyTo: 'message-1' }],
    });
    await screen.findByText('Reply far back');
    server.chat.memberReact('message-20', tutor.id, 'heart');
    fireEvent.click(within(rowOf('Reply far back')).getByRole('button', { name: /Message 1$/ }));
    await waitFor(() => {
      expect(document.activeElement?.getAttribute('data-row')).toBe('message-1');
    });
    fireEvent.click(screen.getByRole('button', { name: /^Jump to latest/ }));
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /^Jump to latest/ })).toBeNull();
    });
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);
    act(() => {
      receive({ kind: 'message', change: 'updated', channelId: channel.id, messageId: 'message-20' });
      clock.pass(1000);
    });
    const chip = await within(rowOf('Message 20')).findByRole('button', { name: /Heart\s*1\s*your reaction/ });
    chip.focus();

    fireEvent.click(chip);
    await waitFor(() => {
      expect(screen.queryByRole('list', { name: 'Reactions' })).toBeNull();
    });

    expect(document.activeElement).toBe(rowOf('Message 20'));
  });

  test('a tutor\u2019s chip names the other reactors in its name and tooltip, a deleted account as Deleted user', async () => {
    const { server, cache, clock } = await fixture();
    await screen.findByText('Message 1');
    server.chat.memberReact('message-1', tutor.id, 'joy');
    server.chat.memberReact('message-1', 'former-student', 'joy');
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);

    act(() => {
      receive({ kind: 'message', change: 'updated', channelId: channel.id, messageId: 'message-1' });
      clock.pass(1000);
    });
    const chip = await screen.findByRole('button', { name: /Laughing\s*2\s*your reaction/ });

    expect(chip.getAttribute('title')).toBe('Deleted user');
    expect(chip.textContent).toContain('Deleted user');
  });

  test('a read-only chat offers no React and shows plain chips', async () => {
    const { server, cache, clock } = await fixture();
    await screen.findByText('Message 1');
    server.chat.memberReact('message-1', 'student-1', 'joy');
    server.chat.setWritable(channel.id, false);
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);

    act(() => {
      receive({ kind: 'entity', entity: 'channel', id: channel.id });
      receive({ kind: 'message', change: 'updated', channelId: channel.id, messageId: 'message-1' });
      clock.pass(1000);
    });

    const reactions = await screen.findByRole('list', { name: 'Reactions' });
    expect(within(reactions).queryByRole('button')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Message actions' }));
    expect(screen.queryByRole('button', { name: 'React' })).toBeNull();
  });

  test('a student\u2019s chip names nobody', async () => {
    const { server, cache, clock } = await fixture(1, '/messages/channel-1', true, { role: 'student' });
    await screen.findByText('Message 1');
    server.chat.memberReact('message-1', 'tutor-1', 'wow');
    server.chat.memberReact('message-1', 'student-1', 'wow');
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);

    act(() => {
      receive({ kind: 'message', change: 'updated', channelId: channel.id, messageId: 'message-1' });
      clock.pass(1000);
    });
    const chip = await screen.findByRole('button', { name: /Surprised\s*2/ });

    expect(chip.getAttribute('title')).toBeNull();
    expect(chip.textContent).not.toContain('Student');
  });

  test('editing focuses the composer and Escape restores the draft and message focus', async () => {
    await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    fireEvent.change(field, { target: { value: 'Unsent draft' } });

    fireEvent.click(await screen.findByRole('button', { name: 'Message actions' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    const editingFocus = document.activeElement;
    fireEvent.keyDown(field, { key: 'Escape' });

    expect(editingFocus).toBe(field);
    expect((field as HTMLTextAreaElement).value).toBe('Unsent draft');
    expect(document.activeElement).toBe(screen.getByText('Message 1').closest('li'));
  });

  test('offline blocks Send, Edit, Delete and every attach path and preserves the composer', async () => {
    const { connection, clock, server } = await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    await screen.findByRole('button', { name: 'Message actions' });
    fireEvent.change(field, { target: { value: 'Unsent draft' } });
    field.focus();

    act(() => {
      connection.readiness(1, false);
      clock.pass(5000);
    });
    const preservedFocus = document.activeElement;
    const file = new File(['image'], 'picture.png', { type: 'image/png' });
    const attach = screen.getByRole('button', { name: 'Attach files' });
    fireEvent.click(attach);
    fireEvent.change(screen.getByLabelText('Attach files', { selector: 'input' }), { target: { files: [file] } });
    fireEvent.drop(field, { dataTransfer: { files: [file] } });
    fireEvent.paste(field, { clipboardData: { files: [file] } });
    fireEvent.submit(field.closest('form') ?? field);
    fireEvent.click(screen.getByRole('button', { name: 'Message actions' }));
    const edit = screen.getByRole('button', { name: 'Edit' });
    const remove = screen.getByRole('button', { name: 'Delete' });
    fireEvent.click(edit);
    fireEvent.click(remove);

    expect(preservedFocus).toBe(field);
    expect((field as HTMLTextAreaElement).value).toBe('Unsent draft');
    for (const button of [screen.getByRole('button', { name: 'Send' }), edit, remove, attach]) {
      expect(button.getAttribute('aria-disabled')).toBe('true');
      expect(button.getAttribute('title')).toBe('No connection');
    }
    expect(within(screen.getByRole('list', { name: 'Attachments' })).queryAllByRole('listitem')).toHaveLength(0);
    expect(server.chat.history(channel.id).length).toBe(1);
  });

  test('read-only live updates preserve draft and focus but permit own deletion', async () => {
    const { server, cache } = await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    fireEvent.change(field, { target: { value: 'Unsent draft' } });
    field.focus();

    server.chat.setWritable(channel.id, false);
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelKey(channel.id) });
    });
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Send' }).getAttribute('aria-disabled')).toBe('true');
    });
    const preservedFocus = document.activeElement;
    fireEvent.click(screen.getByRole('button', { name: 'Message actions' }));

    expect(preservedFocus).toBe(field);
    expect((field as HTMLTextAreaElement).value).toBe('Unsent draft');
    expect(screen.getByRole('button', { name: 'Send' }).getAttribute('aria-disabled')).toBe('true');
    expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Delete' }).getAttribute('aria-disabled')).toBe('false');
  });

  test('live channel removal clears the conversation and focuses its notice', async () => {
    const { server, cache } = await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    field.focus();

    server.chat.removeMember(channel.id, tutor.id);
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelKey(channel.id) });
    });
    const notice = await screen.findByText('This chat is no longer available.');

    await waitFor(() => {
      expect(document.activeElement).toBe(notice);
    });
    expect(screen.queryByText('Message 1')).toBeNull();
    expect(cache.getQueryData(messagesKey(channel.id))).toBeUndefined();
  });

  test('a history read refused as not found clears the conversation and focuses its notice', async () => {
    const { server, cache } = await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    await screen.findByText('Message 1');
    field.focus();

    server.chat.removeMember(channel.id, tutor.id);
    await act(async () => {
      await cache.invalidateQueries({ queryKey: messagesKey(channel.id) });
    });
    const notice = await screen.findByText('This chat is no longer available.');

    await waitFor(() => {
      expect(document.activeElement).toBe(notice);
    });
    expect(screen.queryByText('Message 1')).toBeNull();
  });

  test('live history removal moves focus to the next message', async () => {
    const { cache } = await fixture(3);
    const middle = await screen.findByText('Message 2');
    middle.closest('li')?.focus();

    act(() => {
      cache.setQueryData(messagesKey(channel.id), {
        items: [message(3), message(1)],
        next_cursor: null,
      });
    });

    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByText('Message 3').closest('li'));
    });
  });

  test('search filters labels and a narrow conversation has a back link', async () => {
    const { clock, router } = await fixture(1, '/messages', false);
    const search = await screen.findByRole('searchbox', { name: 'Search chats' });

    fireEvent.change(search, { target: { value: 'missing' } });
    act(() => {
      clock.pass(200);
    });
    const empty = await screen.findByText('No chats');
    await act(async () => {
      await router.navigate('/messages/channel-1');
    });

    await screen.findByText('Message 1');

    expect(empty.textContent).toBe('No chats');
    expect(screen.queryByRole('searchbox')).toBeNull();
    expect(screen.getByRole('link', { name: 'Back to the list' }).getAttribute('href')).toBe('/messages');
    expect(within(screen.getByRole('list', { name: 'Messages' })).getAllByRole('listitem').length).toBe(1);
  });
  test('widening past 768 px with focus on Back of a missing channel moves focus to the page heading', async () => {
    const media = switchableMediaQueries([phoneLayoutQuery]);
    const { router } = await fixture(1, '/messages', false, { mediaQueries: media.queries });
    await act(async () => {
      await router.navigate('/messages/missing-channel');
    });
    const back = await screen.findByRole('link', { name: 'Back to the list' });
    await waitFor(() => {
      expect(document.activeElement?.matches('p[role="status"]')).toBe(true);
    });
    back.focus();

    act(() => {
      media.match([]);
    });

    await waitFor(() => {
      expect(screen.queryByRole('link', { name: 'Back to the list' })).toBeNull();
    });
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });

  test.each(['tutor', 'student'] as const)(
    'the %s sees reclaimed sender cues without account identity details',
    async (role) => {
      await fixture(1, '/messages/channel-1', true, { role, reclaimed: true });

      const cue = await screen.findByText('Sent before the account was reclaimed');

      expect(cue.closest('li')?.textContent).toContain('Message 1');
      expect(cue.textContent).toBe('Sent before the account was reclaimed');
    },
  );

  test('an ordinary confirmed message has no reclaim cue', async () => {
    await fixture();

    await screen.findByText('Message 1');

    expect(screen.queryByText('Sent before the account was reclaimed')).toBeNull();
  });

  test('own messages show one tick until a read frame reports the peer read, then two', async () => {
    const { server, cache, clock } = await fixture(2);
    await screen.findByText('Message 2');
    const sentBefore = screen.getAllByText('Sent').length;
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);

    server.chat.memberRead(channel.id, 'student-1', 1);
    act(() => {
      receive({ kind: 'read', channelId: channel.id });
    });

    expect(sentBefore).toBe(2);
    expect(await screen.findByText('Read')).toBeTruthy();
    expect(screen.getAllByText('Sent')).toHaveLength(1);
  });

  test('only own messages carry ticks in a direct chat', async () => {
    const fromStudent = { ...message(2), id: 'message-2', body: 'From the student', senderId: 'student-1' };
    await fixture(1, '/messages/channel-1', true, { seeds: [fromStudent] });

    const studentRow = (await screen.findByText('From the student')).closest('li');
    const ownRow = screen.getByText('Message 1').closest('li');

    if (studentRow === null || ownRow === null) throw new Error('missing message rows');
    expect(within(studentRow).queryByText('Sent')).toBeNull();
    expect(within(ownRow).getByText('Sent')).toBeTruthy();
  });

  test('a group chat shows no ticks', async () => {
    await fixture(1, '/messages/channel-1', true, { kind: 'group' });

    await screen.findByText('Message 1');

    expect(screen.queryByText('Sent')).toBeNull();
    expect(screen.queryByText('Read')).toBeNull();
  });

  test('search opens on its field and Escape returns focus to the header button', async () => {
    await fixture();
    const open = await screen.findByRole('button', { name: 'Search in this chat' });

    fireEvent.click(open);
    const field = await screen.findByRole('searchbox', { name: 'Search messages' });
    const focusedOnOpen = document.activeElement;
    fireEvent.keyDown(field, { key: 'Escape' });

    expect(focusedOnOpen).toBe(field);
    expect(screen.queryByRole('searchbox', { name: 'Search messages' })).toBeNull();
    expect(document.activeElement).toBe(open);
  });

  test('a one-character term sends no search and shows the hint', async () => {
    const { server } = await fixture(3);
    fireEvent.click(await screen.findByRole('button', { name: 'Search in this chat' }));
    const field = await screen.findByRole('searchbox', { name: 'Search messages' });
    server.chat.refuseNextSearch(503, 'service_unavailable');

    fireEvent.change(field, { target: { value: 'M' } });
    fireEvent.keyDown(field, { key: 'Enter' });

    expect(await screen.findByText('Type at least 2 characters')).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  test.each([
    { reclaimed: true, cue: 1 },
    { reclaimed: false, cue: 0 },
  ])('search shows the reclaim cue only when sent_before_reclaim: $reclaimed', async ({ reclaimed, cue }) => {
    await fixture(0, '/messages/channel-1', true, {
      seeds: [{ ...message(1), body: 'Reclaim needle', sent_before_reclaim: reclaimed, senderId: tutor.id }],
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Search in this chat' }));
    const field = await screen.findByRole('searchbox', { name: 'Search messages' });
    fireEvent.change(field, { target: { value: 'needle' } });

    fireEvent.keyDown(field, { key: 'Enter' });

    const result = await within(await screen.findByRole('list', { name: 'Search results' })).findByRole('button');
    expect(within(result).queryAllByText(chatTexts.reclaimed)).toHaveLength(cue);
    expect(result.textContent).toContain('Reclaim needle');
  });

  test('results render as text and opening one focuses its message', async () => {
    await fixture(2, '/messages/channel-1', true, {
      seeds: [{ ...message(3), id: 'message-3', body: '<b>Bold needle</b>', senderId: tutor.id }],
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Search in this chat' }));
    const field = await screen.findByRole('searchbox', { name: 'Search messages' });

    fireEvent.change(field, { target: { value: 'needle' } });
    fireEvent.keyDown(field, { key: 'Enter' });
    const result = await within(await screen.findByRole('list', { name: 'Search results' })).findByRole('button');
    fireEvent.click(result);

    expect(result.textContent).toContain('<b>Bold needle</b>');
    expect(result.querySelector('b')).toBeNull();
    await waitFor(() => {
      expect(document.activeElement instanceof HTMLElement ? document.activeElement.dataset.row : undefined).toBe(
        'message-3',
      );
    });
  });

  test('Show more focuses the first new result', async () => {
    await fixture(31);
    fireEvent.click(await screen.findByRole('button', { name: 'Search in this chat' }));
    const field = await screen.findByRole('searchbox', { name: 'Search messages' });
    fireEvent.change(field, { target: { value: 'Message' } });
    fireEvent.keyDown(field, { key: 'Enter' });
    const list = await screen.findByRole('list', { name: 'Search results' });

    fireEvent.click(await within(list.parentElement ?? list).findByRole('button', { name: 'Show more' }));

    await waitFor(() => {
      expect(document.activeElement?.textContent).toContain('Message 1');
    });
    expect(within(list).getAllByRole('button')).toHaveLength(31);
  });

  test('a busy or rate-limited search shows its refusal and keeps focus in the field', async () => {
    const { server } = await fixture(2);
    fireEvent.click(await screen.findByRole('button', { name: 'Search in this chat' }));
    const field = await screen.findByRole('searchbox', { name: 'Search messages' });
    fireEvent.change(field, { target: { value: 'Message' } });
    server.chat.refuseNextSearch(503, 'service_unavailable');
    server.chat.refuseNextSearch(429, 'rate_limited');

    fireEvent.keyDown(field, { key: 'Enter' });
    const busy = (await screen.findByRole('alert')).textContent;
    fireEvent.keyDown(field, { key: 'Enter' });
    await waitFor(() => {
      expect(screen.getByRole('alert').textContent).not.toBe(busy);
    });

    expect(busy).toBe('The service is temporarily unavailable. Try again later.');
    expect(screen.getByRole('alert').textContent).toBe('Too many attempts, try again in 1 minute.');
    expect(document.activeElement).toBe(field);
  });

  test('the search term stays out of the address, form history and the query cache', async () => {
    const { cache, router } = await fixture(2);
    fireEvent.click(await screen.findByRole('button', { name: 'Search in this chat' }));
    const field = await screen.findByRole('searchbox', { name: 'Search messages' });

    fireEvent.change(field, { target: { value: 'zebrafinch' } });
    fireEvent.keyDown(field, { key: 'Enter' });
    await screen.findByText('No messages found');

    const location = router.state.location;
    expect(`${location.pathname}${location.search}${location.hash}`).not.toContain('zebrafinch');
    expect(JSON.stringify(window.history.state ?? null)).not.toContain('zebrafinch');
    expect(field.getAttribute('autocomplete')).toBe('off');
    expect(field.hasAttribute('name')).toBe(false);
    expect(field.closest('form')).toBeNull();
    expect(
      JSON.stringify(
        cache
          .getQueryCache()
          .getAll()
          .map((query) => query.queryKey),
      ),
    ).not.toContain('zebrafinch');
  });

  test('a late search response never renders after the conversation changed', async () => {
    const { server, router } = await fixture(2);
    const other = { ...channel, id: 'channel-other', label: 'Other conversation' };
    server.chat.addChannel({ channel: other, members: [{ accountId: tutor.id }, { accountId: 'student-1' }] });
    fireEvent.click(await screen.findByRole('button', { name: 'Search in this chat' }));
    const field = await screen.findByRole('searchbox', { name: 'Search messages' });
    fireEvent.change(field, { target: { value: 'Message' } });
    const pause = server.chat.pauseNextSearchResponse();
    fireEvent.keyDown(field, { key: 'Enter' });
    await pause.answered;

    await act(async () => {
      await router.navigate('/messages/channel-other');
    });
    await screen.findByRole('heading', { name: 'Other conversation' });
    await act(async () => {
      pause.resume();
      await Promise.resolve();
    });

    expect(screen.queryByRole('list', { name: 'Search results' })).toBeNull();
  });

  test('touch long press keeps the menu open through pointerup and its generated click', async () => {
    const { clock } = await fixture();
    const menu = await screen.findByRole('button', { name: 'Message actions' });

    fireEvent.pointerDown(menu, { pointerType: 'touch' });
    act(() => {
      clock.pass(500);
    });
    fireEvent.pointerUp(menu, { pointerType: 'touch' });
    fireEvent.click(menu);

    expect(screen.getByRole('button', { name: 'Reply' })).toBe(document.activeElement);
    expect(menu.getAttribute('aria-expanded')).toBe('true');
  });
  test('retrying an uncertain send keeps its key and creates only one message', async () => {
    const { server } = await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    server.chat.loseNextSendResponse();
    fireEvent.change(field, { target: { value: 'Recovered once' } });

    fireEvent.click(screen.getByRole('button', { name: 'Send' }));
    await screen.findByRole('alert');
    const failedDraft = (field as HTMLTextAreaElement).value;
    fireEvent.click(screen.getByRole('button', { name: 'Send' }));
    await screen.findByText('Recovered once');
    await waitFor(() => {
      expect((field as HTMLTextAreaElement).value).toBe('');
    });

    expect(failedDraft).toBe('Recovered once');
    expect(server.chat.history(channel.id).filter((item) => item.body === 'Recovered once').length).toBe(1);
    await waitFor(() => {
      expect(document.activeElement).toBe(field);
    });
  });

  test.each(['Keep my text', 'Use latest text'] as const)(
    'a stale edit adopts a fresh version only after %s and saves explicitly',
    async (choice) => {
      const { server, api } = await fixture();
      const field = await screen.findByRole('textbox', { name: 'Message' });
      fireEvent.change(field, { target: { value: 'Original draft' } });
      fireEvent.click(await screen.findByRole('button', { name: 'Message actions' }));
      fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
      fireEvent.change(field, { target: { value: 'My unsaved edit' } });
      await editMessage(api, message(1), 'Changed elsewhere');

      fireEvent.click(screen.getByRole('button', { name: 'Save' }));
      const choose = await screen.findByRole('button', { name: choice });
      const retained = (field as HTMLTextAreaElement).value;
      fireEvent.submit(field.closest('form') ?? field);
      const beforeChoice = server.chat.history(channel.id)[0];
      fireEvent.click(choose);
      const chosenFocus = document.activeElement;
      const chosenText = (field as HTMLTextAreaElement).value;
      const beforeSave = server.chat.history(channel.id)[0];
      fireEvent.click(screen.getByRole('button', { name: 'Save' }));
      await screen.findByRole('button', { name: 'Send' });
      await waitFor(() => {
        expect(document.activeElement?.getAttribute('data-row')).toBe('message-1');
      });

      expect(retained).toBe('My unsaved edit');
      expect(beforeChoice?.version).toBe(2);
      expect(beforeSave?.version).toBe(2);
      expect(chosenFocus).toBe(field);
      expect(chosenText).toBe(choice === 'Keep my text' ? 'My unsaved edit' : 'Changed elsewhere');
      expect(server.chat.history(channel.id)[0]?.body).toBe(chosenText);
      expect(server.chat.history(channel.id)[0]?.version).toBe(3);
      expect((field as HTMLTextAreaElement).value).toBe('Original draft');
    },
  );

  test('another concurrent edit after the choice requires a new explicit choice', async () => {
    const { api, server } = await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    fireEvent.click(await screen.findByRole('button', { name: 'Message actions' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    fireEvent.change(field, { target: { value: 'Local edit' } });
    const second = await editMessage(api, message(1), 'Second version');

    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Keep my text' }));
    await editMessage(api, second, 'Third version');
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await screen.findByRole('button', { name: 'Use latest text' });

    expect(
      within(screen.getByRole('region', { name: 'Current saved text' })).getByText('Third version'),
    ).not.toBeNull();
    expect((field as HTMLTextAreaElement).value).toBe('Local edit');
    expect(server.chat.history(channel.id)[0]?.version).toBe(3);
    expect(screen.getByRole('button', { name: 'Save' }).getAttribute('aria-disabled')).toBe('true');
  });

  test.each([1, 2])('a late conflict refresh cannot replace a reopened editor for message %s', async (next) => {
    const { api, server, cache } = await fixture(2);
    const field = await screen.findByRole('textbox', { name: 'Message' });
    const first = (await screen.findByText('Message 1')).closest('li');
    if (first === null) throw new Error('Missing message');
    fireEvent.click(within(first).getByRole('button', { name: 'Message actions' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await editMessage(api, message(1), 'Changed elsewhere');
    const pause = server.chat.pauseNextChannelResponse();

    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await pause.answered;
    await screen.findByText('Changed elsewhere');
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    const row = screen.getByText(next === 1 ? 'Changed elsewhere' : 'Message 2').closest('li');
    if (row === null) throw new Error('Missing next message');
    fireEvent.click(within(row).getByRole('button', { name: 'Message actions' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    fireEvent.change(field, { target: { value: 'New editing lifetime' } });
    act(() => {
      pause.resume();
    });
    await waitFor(() => {
      expect(cache.isFetching()).toBe(0);
    });

    expect((field as HTMLTextAreaElement).value).toBe('New editing lifetime');
    expect(document.activeElement).toBe(field);
    expect(screen.queryByRole('button', { name: 'Keep my text' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Save' }).getAttribute('aria-disabled')).toBe('false');
  });
  test('a failed conflict read offers retry without adopting stale cached content', async () => {
    const { api, server } = await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    fireEvent.click(await screen.findByRole('button', { name: 'Message actions' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    fireEvent.change(field, { target: { value: 'Local edit' } });
    await editMessage(api, message(1), 'Latest server text');
    server.chat.failNextHistoryRead();

    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    const retry = await screen.findByRole('button', { name: 'Try again' });
    const staleChoice = screen.queryByRole('button', { name: 'Keep my text' });
    const blockedSave = screen.getByRole('button', { name: 'Save' }).getAttribute('aria-disabled');
    fireEvent.click(retry);
    fireEvent.click(await screen.findByRole('button', { name: 'Use latest text' }));
    const resolvedFocus = document.activeElement;
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(staleChoice).toBeNull();
    expect(blockedSave).toBe('true');
    expect(resolvedFocus).toBe(field);
    expect(server.chat.history(channel.id)[0]?.version).toBe(2);
    expect((field as HTMLTextAreaElement).value).toBe('');
    expect(document.activeElement?.getAttribute('data-row')).toBe('message-1');
  });

  test('a paused conflict recovery waits for both new reads after reconnect', async () => {
    const { api, server, cache } = await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    fireEvent.click(await screen.findByRole('button', { name: 'Message actions' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    fireEvent.change(field, { target: { value: 'Local edit' } });
    await editMessage(api, message(1), 'Latest server text');
    await waitFor(() => {
      expect(cache.isFetching()).toBe(0);
    });
    const historyUpdates = cache.getQueryState(messagesKey(channel.id))?.dataUpdateCount;
    const channelUpdates = cache.getQueryState(channelKey(channel.id))?.dataUpdateCount;
    const patch = server.chat.pauseNextMessageResponse();
    const channelRead = server.chat.pauseNextChannelResponse();

    try {
      fireEvent.click(screen.getByRole('button', { name: 'Save' }));
      await patch.answered;
      act(() => {
        window.dispatchEvent(new Event('offline'));
        patch.resume();
      });
      await waitFor(() => {
        expect(cache.getQueryState(messagesKey(channel.id))?.fetchStatus).toBe('paused');
        expect(cache.getQueryState(channelKey(channel.id))?.fetchStatus).toBe('paused');
      });
      const pausedChoice = screen.queryByRole('button', { name: 'Keep my text' });
      const pausedSave = screen.getByRole('button', { name: 'Save' }).getAttribute('aria-disabled');
      const pausedHistoryUpdates = cache.getQueryState(messagesKey(channel.id))?.dataUpdateCount;
      const pausedChannelUpdates = cache.getQueryState(channelKey(channel.id))?.dataUpdateCount;
      act(() => {
        window.dispatchEvent(new Event('online'));
      });
      await channelRead.answered;
      await waitFor(() => {
        expect(cache.getQueryState(messagesKey(channel.id))?.dataUpdateCount).toBeGreaterThan(historyUpdates ?? 0);
      });
      const oneReadChoice = screen.queryByRole('button', { name: 'Keep my text' });
      act(() => {
        channelRead.resume();
      });
      fireEvent.click(await screen.findByRole('button', { name: 'Keep my text' }));
      const resolvedFocus = document.activeElement;
      const versionBeforeSave = server.chat.history(channel.id)[0]?.version;
      fireEvent.click(screen.getByRole('button', { name: 'Save' }));
      await screen.findByRole('button', { name: 'Send' });

      expect(pausedChoice).toBeNull();
      expect(pausedSave).toBe('true');
      expect(pausedHistoryUpdates).toBe(historyUpdates);
      expect(pausedChannelUpdates).toBe(channelUpdates);
      expect(oneReadChoice).toBeNull();
      expect(resolvedFocus).toBe(field);
      expect(versionBeforeSave).toBe(2);
      expect(server.chat.history(channel.id)[0]).toMatchObject({ body: 'Local edit', version: 3 });
    } finally {
      act(() => {
        patch.resume();
        channelRead.resume();
        window.dispatchEvent(new Event('online'));
      });
    }
  });

  test.each([1, 2])('cancel while offline releases the old recovery before editing message %s', async (next) => {
    const { api, server, cache } = await fixture(2);
    const field = await screen.findByRole('textbox', { name: 'Message' });
    const first = (await screen.findByText('Message 1')).closest('li');
    if (first === null) throw new Error('Missing first message');
    fireEvent.click(within(first).getByRole('button', { name: 'Message actions' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await editMessage(api, message(1), 'Changed elsewhere');
    const patch = server.chat.pauseNextMessageResponse();

    try {
      fireEvent.click(screen.getByRole('button', { name: 'Save' }));
      await patch.answered;
      act(() => {
        window.dispatchEvent(new Event('offline'));
        patch.resume();
      });
      await waitFor(() => {
        expect(cache.getQueryState(messagesKey(channel.id))?.fetchStatus).toBe('paused');
        expect(cache.getQueryState(channelKey(channel.id))?.fetchStatus).toBe('paused');
      });
      fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
      const cancelledFocus = document.activeElement;
      const row = screen.getByText(`Message ${String(next)}`).closest('li');
      if (row === null) throw new Error('Missing next message');
      fireEvent.click(within(row).getByRole('button', { name: 'Message actions' }));
      fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
      fireEvent.change(field, { target: { value: 'New editing lifetime' } });
      const preservedHistory = cache.getQueryState(messagesKey(channel.id))?.fetchStatus;
      const preservedChannel = cache.getQueryState(channelKey(channel.id))?.fetchStatus;
      act(() => {
        window.dispatchEvent(new Event('online'));
      });
      await waitFor(() => {
        expect(cache.isFetching()).toBe(0);
      });
      await screen.findByText('Changed elsewhere');

      expect(cancelledFocus).toBe(first);
      expect(preservedHistory).toBe('paused');
      expect(preservedChannel).toBe('paused');
      expect((field as HTMLTextAreaElement).value).toBe('New editing lifetime');
      expect(document.activeElement).toBe(field);
      expect(screen.queryByRole('button', { name: 'Keep my text' })).toBeNull();
      expect(screen.getByRole('button', { name: 'Save' }).getAttribute('aria-disabled')).toBe('false');
      expect(server.chat.history(channel.id).find((item) => item.id === 'message-1')?.version).toBe(2);
    } finally {
      act(() => {
        patch.resume();
        window.dispatchEvent(new Event('online'));
      });
    }
  });

  test('a failed channel conflict read has one retry that completes the whole recovery', async () => {
    const { api, server } = await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    fireEvent.click(await screen.findByRole('button', { name: 'Message actions' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    fireEvent.change(field, { target: { value: 'Local edit' } });
    await editMessage(api, message(1), 'Latest server text');
    server.chat.failNextChannelRead();

    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    const retry = await screen.findByRole('button', { name: 'Try again' });
    const retryCount = screen.getAllByRole('button', { name: 'Try again' }).length;
    const choiceBeforeRetry = screen.queryByRole('button', { name: 'Keep my text' });
    fireEvent.click(retry);
    fireEvent.click(await screen.findByRole('button', { name: 'Keep my text' }));
    const resolvedFocus = document.activeElement;
    const resolvedSave = screen.getByRole('button', { name: 'Save' }).getAttribute('aria-disabled');
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await screen.findByRole('button', { name: 'Send' });

    expect(retryCount).toBe(1);
    expect(choiceBeforeRetry).toBeNull();
    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull();
    expect(resolvedFocus).toBe(field);
    expect(resolvedSave).toBe('false');
    expect(server.chat.history(channel.id)[0]).toMatchObject({ body: 'Local edit', version: 3 });
  });

  test.each(['read-only', 'deleted', 'revoked'] as const)(
    'a conflict cannot restore resolution after the message becomes %s',
    async (change) => {
      const { api, server, cache } = await fixture();
      const field = await screen.findByRole('textbox', { name: 'Message' });
      fireEvent.click(await screen.findByRole('button', { name: 'Message actions' }));
      fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
      await editMessage(api, message(1), 'Latest server text');
      fireEvent.click(screen.getByRole('button', { name: 'Save' }));
      (await screen.findByRole('button', { name: 'Keep my text' })).focus();

      if (change === 'read-only') server.chat.setWritable(channel.id, false);
      else if (change === 'revoked') server.chat.removeMember(channel.id, tutor.id);
      else await deleteMessage(api, 'message-1');
      await act(async () => {
        await cache.invalidateQueries({
          queryKey: change === 'deleted' ? messagesKey(channel.id) : channelKey(channel.id),
        });
      });

      await waitFor(() => {
        expect(screen.queryByRole('button', { name: 'Keep my text' })).toBeNull();
      });
      expect(screen.queryByRole('region', { name: 'Current saved text' })).toBeNull();
      if (change === 'revoked') expect(screen.queryByRole('textbox', { name: 'Message' })).toBeNull();
      else {
        expect((field as HTMLTextAreaElement).value).toBe('Message 1');
        expect(document.activeElement).toBe(field);
      }
    },
  );

  test('reconnect retains older loaded history and fills more than one hundred missed messages', async () => {
    const { server, cache } = await fixture(60);
    fireEvent.click(await screen.findByRole('button', { name: 'Show older' }));
    await screen.findByText('Message 1');
    for (let seq = 61; seq <= 200; seq += 1) server.chat.addMessage(channel.id, message(seq));

    await act(async () => {
      await cache.invalidateQueries({ queryKey: messagesKey(channel.id) });
    });
    await screen.findByText('Message 200');

    const rows = within(screen.getByRole('list', { name: 'Messages' })).getAllByRole('listitem');
    expect(rows.map((row) => Number(row.dataset.seq))).toEqual(Array.from({ length: 200 }, (_, index) => index + 1));
    expect(screen.getByText('Message 1')).not.toBeNull();
  });

  test('rebasing a changed first channel page makes every boundary row reachable without duplicates', async () => {
    const { server, cache } = await fixture(0, '/messages');
    for (let index = 60; index >= 1; index -= 1)
      server.chat.addChannel({
        channel: { ...channel, id: `c-${String(index)}`, label: `Channel ${String(index)}` },
        members: [{ accountId: tutor.id }],
      });
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelsKey });
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Show more' }));
    await screen.findByText('Channel 60');

    server.chat.addMessage('c-40', message(1));
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelPageKey('', 'all', 0, null), exact: true });
    });
    await waitFor(() => {
      expect(screen.queryByText('Channel 60')).toBeNull();
    });
    fireEvent.click(screen.getByRole('button', { name: 'Show more' }));
    await screen.findByText('Channel 30');

    const addresses = within(screen.getByRole('region', { name: 'Chats' }))
      .getAllByRole('link')
      .map((link) => link.getAttribute('href'));
    expect(addresses.length).toBe(60);
    expect(new Set(addresses).size).toBe(60);
    expect(addresses).toContain('/messages/c-30');
  });

  test.each(['Escape', 'Save'] as const)(
    'switching edited messages preserves the original unsent draft on %s',
    async (finish) => {
      await fixture(2);
      const field = await screen.findByRole('textbox', { name: 'Message' });
      fireEvent.change(field, { target: { value: 'Original unsent draft' } });

      const first = (await screen.findByText('Message 1')).closest('li');
      const second = screen.getByText('Message 2').closest('li');
      if (first === null || second === null) throw new Error('Missing message rows');
      fireEvent.click(within(first).getByRole('button', { name: 'Message actions' }));
      fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
      fireEvent.click(within(second).getByRole('button', { name: 'Message actions' }));
      fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
      if (finish === 'Escape') fireEvent.keyDown(field, { key: 'Escape' });
      else {
        fireEvent.click(screen.getByRole('button', { name: 'Save' }));
        await screen.findByRole('button', { name: 'Send' });
        await waitFor(() => {
          expect(document.activeElement).toBe(second);
        });
      }

      expect((field as HTMLTextAreaElement).value).toBe('Original unsent draft');
      expect(document.activeElement).toBe(second);
    },
  );

  test('a remote deletion while its menu owns focus restores focus to the placeholder', async () => {
    const { api, cache } = await fixture();
    const row = (await screen.findByText('Message 1')).closest('li');
    fireEvent.click(screen.getByRole('button', { name: 'Message actions' }));
    screen.getByRole('button', { name: 'Delete' }).focus();

    const result = await api.DELETE('/api/v1/messages/{id}', { params: { path: { id: 'message-1' } } });
    if (!result.response.ok) throw new Error('Deletion failed');
    await act(async () => {
      await cache.invalidateQueries({ queryKey: messagesKey(channel.id) });
    });
    await screen.findByText('Message deleted');

    expect(document.activeElement).toBe(row);
  });
  test('reopening a revoked cached channel never renders old history before its fresh refusal', async () => {
    const { server, cache, router, clock } = await fixture();
    await screen.findByText('Message 1');
    for (let index = 1; index <= 35; index += 1)
      server.chat.addChannel({
        channel: { ...channel, id: `other-${String(index)}`, label: `Other ${String(index)}` },
        members: [{ accountId: tutor.id }],
      });
    await act(async () => {
      await router.navigate('/elsewhere');
    });
    server.chat.removeMember(channel.id, tutor.id);
    const pause = server.chat.pauseNextChannelResponse();

    await act(async () => {
      await router.navigate('/messages/channel-1');
    });
    await pause.answered;
    const staleBody = screen.queryByText('Message 1');
    const staleHeading = screen.queryByRole('heading', { level: 2, name: 'Student' });
    act(() => {
      pause.resume();
    });
    await screen.findByText('Not found');
    const pendingAfterRefusal = clock.pendingDelays();
    act(() => {
      clock.pass(1000);
    });
    await screen.findByRole('button', { name: 'Show more' });

    expect(pendingAfterRefusal).toContain(1000);
    expect(staleBody).toBeNull();
    expect(staleHeading).toBeNull();
    expect(cache.getQueryData(messagesKey(channel.id))).toBeUndefined();
    expect(cache.getQueryData(channelKey(channel.id))).toBeUndefined();
    for (const [, page] of cache.getQueriesData<import('./api.ts').ChannelPage>({ queryKey: channelsKey })) {
      expect(page?.items.some((item) => item.id === channel.id) ?? false).toBe(false);
    }
  });

  test('a denied channel aborts a held authorized list and its late response cannot restore private data', async () => {
    const { server, cache } = await fixture();
    await screen.findByText('Message 1');
    await waitFor(() => {
      expect(cache.isFetching()).toBe(0);
    });
    const pause = server.chat.pauseNextChannelListResponse(false);
    const fetching = cache.invalidateQueries({ queryKey: channelsKey });
    const signal = await pause.answered;

    server.chat.removeMember(channel.id, tutor.id);
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelKey(channel.id), exact: true });
    });
    await screen.findByText('This chat is no longer available.');
    const abortedBeforeRelease = signal.aborted;
    const bodyBeforeRelease = screen.queryByText('Message 1');
    const labelBeforeRelease = screen.queryAllByText('Student');
    const detailBeforeRelease = cache.getQueryData(channelKey(channel.id));
    const historyBeforeRelease = cache.getQueryData(messagesKey(channel.id));
    const listsBeforeRelease = cache.getQueriesData<import('./api.ts').ChannelPage>({ queryKey: channelsKey });
    await act(async () => {
      pause.resume();
      await fetching;
    });

    expect(abortedBeforeRelease).toBe(true);
    expect(bodyBeforeRelease).toBeNull();
    expect(labelBeforeRelease).toHaveLength(0);
    expect(detailBeforeRelease).toBeUndefined();
    expect(historyBeforeRelease).toBeUndefined();
    expect(screen.queryByText('Message 1')).toBeNull();
    expect(screen.queryAllByText('Student')).toHaveLength(0);
    expect(cache.getQueryData(channelKey(channel.id))).toBeUndefined();
    expect(cache.getQueryData(messagesKey(channel.id))).toBeUndefined();
    for (const [, page] of [
      ...listsBeforeRelease,
      ...cache.getQueriesData<import('./api.ts').ChannelPage>({ queryKey: channelsKey }),
    ]) {
      expect(page?.items.some((item) => item.id === channel.id) ?? false).toBe(false);
    }
  });

  test('a failed deletion shows its refusal inside the dialog, is reported to the conversation and keeps the message', async () => {
    const { server, api } = await fixture();
    await screen.findByText('Message 1');
    cleanup();
    server.chat.failNextDelete();
    const reported: string[] = [];
    const i18n = await createI18n('en');
    render(
      <I18nextProvider i18n={i18n}>
        <ApiClientContext value={api}>
          <DeleteMessageForm
            messageId="message-1"
            texts={chatTexts}
            headingId="delete-heading"
            consequenceId="delete-consequence"
            cancelRef={{ current: null }}
            unavailable={false}
            unavailableReason={undefined}
            onBusy={() => undefined}
            onDeleted={() => {
              reported.push('deleted');
            }}
            onFailed={() => {
              reported.push('failed');
            }}
            onCancel={() => {
              reported.push('cancel');
            }}
          />
        </ApiClientContext>
      </I18nextProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Delete message' }));

    await waitFor(() => {
      expect(reported).toEqual(['failed']);
    });
    const alert = await screen.findByRole('alert');
    expect(document.activeElement).toBe(alert);
    expect(screen.getByRole('button', { name: 'Delete message' }).getAttribute('aria-busy')).not.toBe('true');
    expect(server.chat.history(channel.id)[0]?.body).toBe('Message 1');
  });

  test('a held continuation cannot restore revoked rows after a channel event and fresh pagination resumes', async () => {
    const { server, cache, clock } = await fixture(0, '/messages');
    for (let index = 60; index >= 1; index -= 1) {
      const id = `c-${String(index)}`;
      server.chat.addChannel({
        channel: { ...channel, id, label: `Channel ${String(index)}` },
        members: [{ accountId: tutor.id }],
      });
      server.chat.addMessage(id, {
        ...message(1),
        body: index === 40 ? 'Revoked private preview' : `Preview ${String(index)}`,
      });
    }
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelsKey });
    });
    const pause = server.chat.pauseNextChannelListResponse();
    fireEvent.click(await screen.findByRole('button', { name: 'Show more' }));
    await pause.answered;
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);

    server.chat.removeMember('c-40', tutor.id);
    act(() => {
      receive({ kind: 'entity', entity: 'channel', id: 'c-40' });
    });
    const moreWhileStale = screen.queryByRole('button', { name: 'Show more' });
    await act(async () => {
      pause.resume();
      await Promise.resolve();
    });
    const leakedBeforeRefresh = screen.queryByText(/Revoked private preview/);
    act(() => {
      clock.pass(1000);
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Show more' }));
    await screen.findByText('Channel 60');
    receive.cancel();

    expect(moreWhileStale).toBeNull();
    expect(leakedBeforeRefresh).toBeNull();
    expect(screen.queryByText('Channel 40')).toBeNull();
    expect(screen.queryByText(/Revoked private preview/)).toBeNull();
    expect(screen.getByText('Channel 31')).not.toBeNull();
    for (const [, page] of cache.getQueriesData<import('./api.ts').ChannelPage>({ queryKey: channelsKey })) {
      expect(page?.items.some((item) => item.id === 'c-40') ?? false).toBe(false);
    }
  });

  test('a send response from an old session epoch cannot mutate the still mounted conversation', async () => {
    const { server, cache, session } = await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    const send = server.chat.pauseNextSendResponse();
    fireEvent.change(field, { target: { value: 'Old account send' } });
    fireEvent.click(screen.getByRole('button', { name: 'Send' }));
    await send.answered;
    const other = {
      id: 'other-tutor',
      name: 'Other tutor',
      email: 'other@example.org',
      password: 'another long password',
    };
    server.addAccount(other);
    const login = await server.transport().login(other.email, other.password);
    const token: unknown = await login.json();
    const account = server.pauseAccountReads();
    const changing = session.adopt(token);
    await account.answered;
    const before = server.sentAddresses.filter(
      (address) => address.pathname === '/api/v1/channels/channel-1/messages',
    ).length;

    await act(async () => {
      send.resume();
      await Promise.resolve();
    });
    const after = server.sentAddresses.filter(
      (address) => address.pathname === '/api/v1/channels/channel-1/messages',
    ).length;
    const draftBeforeRemount = (field as HTMLTextAreaElement).value;
    const historyBeforeRemount = cache.getQueryData<import('./api.ts').MessagePage>(messagesKey(channel.id));
    await act(async () => {
      cache.clear();
      account.resume();
      await changing;
    });

    expect(after).toBe(before);
    expect(draftBeforeRemount).toBe('Old account send');
    expect(historyBeforeRemount?.items.map((item) => item.body)).toEqual(['Message 1']);
  });
});

describe('attachment sending and lifecycle', () => {
  test.each(['', 'A message with a file'])(
    'replays the same body and attachment IDs after an uncertain send: %j',
    async (body) => {
      const { server } = await fixture(0);
      const field = await screen.findByRole('textbox', { name: 'Message' });
      fireEvent.change(screen.getByLabelText('Attach files', { selector: 'input' }), {
        target: { files: [new File(['lesson notes'], 'notes.txt', { type: 'text/plain' })] },
      });
      await screen.findByText('Uploaded');
      fireEvent.change(field, { target: { value: body } });
      server.chat.loseNextSendResponse();

      fireEvent.click(screen.getByRole('button', { name: 'Send' }));
      await screen.findByRole('alert');
      const retainedBody = (field as HTMLTextAreaElement).value;
      const retainedAttachment = within(screen.getByRole('list', { name: 'Attachments' })).getByText(
        'notes.txt',
      ).textContent;
      fireEvent.click(screen.getByRole('button', { name: 'Send' }));
      await waitFor(() => {
        expect(within(screen.getByRole('list', { name: 'Attachments' })).queryAllByRole('listitem')).toHaveLength(0);
      });

      expect(retainedBody).toBe(body);
      expect(retainedAttachment).toBe('notes.txt');
      expect(server.chat.sends).toHaveLength(2);
      expect(server.chat.sends[0]).toEqual(server.chat.sends[1]);
      expect(server.chat.sends[0]?.ids).toHaveLength(1);
      expect(server.chat.history(channel.id)).toHaveLength(1);
      expect(server.chat.history(channel.id)[0]?.attachments).toHaveLength(1);
      expect(server.chat.history(channel.id)[0]?.body).toBe(body);
      await waitFor(() => {
        expect(document.activeElement).toBe(field);
      });
    },
  );

  test('removing the last uploaded file restores focus to the composer', async () => {
    await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    fireEvent.change(screen.getByLabelText('Attach files', { selector: 'input' }), {
      target: { files: [new File(['lesson notes'], 'notes.txt', { type: 'text/plain' })] },
    });
    await screen.findByText('Uploaded');
    const remove = screen.getByRole('button', { name: 'Remove file' });
    remove.focus();

    fireEvent.click(remove);

    expect(document.activeElement).toBe(field);
    expect(screen.queryByText('notes.txt')).toBeNull();
  });

  test('a read-only transition clears a focused refused upload onto the retained composer', async () => {
    const { server, cache } = await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    fireEvent.change(field, { target: { value: 'Retained draft' } });
    fireEvent.change(screen.getByLabelText('Attach files', { selector: 'input' }), {
      target: { files: [new File(['bad image'], 'image.png', { type: 'image/png' })] },
    });
    const refusal = await screen.findByRole('alert');
    await waitFor(() => {
      expect(document.activeElement).toBe(refusal);
    });

    server.chat.setWritable(channel.id, false);
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelKey(channel.id) });
    });

    await waitFor(() => {
      expect((field as HTMLTextAreaElement).readOnly).toBe(true);
    });
    expect(document.activeElement).toBe(field);
    expect((field as HTMLTextAreaElement).value).toBe('Retained draft');
    expect(screen.queryByRole('alert')).toBeNull();
  });

  test('pagehide cancels an image recovery waiting to refresh history and pageshow restores expiry refresh', async () => {
    const { server, cache, clock } = await fixture();
    await screen.findByText('Message 1');
    server.chat.addMessage(channel.id, {
      ...message(2),
      attachments: [
        {
          id: 'picture',
          filename: 'picture.png',
          media_type: 'image/png',
          width: 2,
          height: 2,
          size_bytes: 24,
          url: '/api/v1/files/picture?t=test',
          expires_at: new Date(60_000).toISOString(),
        },
      ],
    });
    await act(async () => {
      await cache.invalidateQueries({ queryKey: messagesKey(channel.id) });
    });
    const picture = await screen.findByRole('img', { name: 'picture.png' });
    const reads = (): number =>
      server.sentAddresses.filter((address) => address.pathname === `/api/v1/channels/${channel.id}/messages`).length;
    const before = reads();

    fireEvent.error(picture);
    fireEvent.pageHide(window);
    await act(async () => {
      clock.pass(1000);
      await Promise.resolve();
    });
    const hiddenReads = reads();
    fireEvent.pageShow(window);
    await act(async () => {
      clock.pass(30_000);
      clock.pass(1000);
      await Promise.resolve();
    });
    await waitFor(() => {
      expect(reads()).toBe(before + 1);
    });

    expect(hiddenReads).toBe(before);
    expect(reads()).toBe(before + 1);
  });
});

test('history reconciliation retains draft read marker and focus', async () => {
  const { server, cache, clock } = await fixture(3);
  const field = await screen.findByRole('textbox', { name: 'Message' });
  fireEvent.change(field, { target: { value: 'An unsent draft' } });
  field.focus();
  const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);
  server.chat.addMessage(channel.id, { ...message(4), senderId: 'student-1' });

  act(() => {
    receive({ kind: 'message', change: 'new', channelId: channel.id, messageId: 'message-4', seq: 4 });
    clock.pass(1000);
  });
  await screen.findByText('Message 4');

  expect((field as HTMLTextAreaElement).value).toBe('An unsent draft');
  expect(document.activeElement).toBe(field);
  expect(server.chat.readSeq(channel.id, tutor.id)).toBe(0);
  expect(screen.getByRole('list', { name: 'Messages' }).querySelectorAll('[data-seq]')).toHaveLength(4);
  receive.cancel();
});

test('reopening the same channel rejects a withheld old-generation incremental response', async () => {
  const { server, api, cache, clock, router } = await fixture();
  await screen.findByText('Message 1');
  const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);
  server.chat.addMessage(channel.id, message(2));
  const pause = server.chat.pauseNextHistoryResponse();
  act(() => {
    receive({ kind: 'message', change: 'new', channelId: channel.id, messageId: 'message-2', seq: 2 });
    clock.pass(1000);
  });
  await pause.answered;
  await editMessage(api, message(2), 'Fresh reopened text');

  await act(async () => {
    await router.navigate('/elsewhere');
  });
  await screen.findByRole('heading', { name: 'Elsewhere' });
  await act(async () => {
    await router.navigate('/messages/channel-1');
  });
  await screen.findByText('Fresh reopened text');
  act(() => {
    pause.resume();
  });

  expect(screen.queryByText('Message 2')).toBeNull();
  expect(
    cache
      .getQueryData<import('./api.ts').MessagePage>(messagesKey(channel.id))
      ?.items.find((item) => item.id === 'message-2'),
  ).toMatchObject({ body: 'Fresh reopened text', version: 2 });
  expect(document.activeElement).toBe(screen.getByRole('heading', { level: 2 }));
  receive.cancel();
});

test('conflict adoption requires fresh authoritative history after incremental work', async () => {
  const { server, api, cache, clock } = await fixture();
  const field = await screen.findByRole('textbox', { name: 'Message' });
  fireEvent.click(await screen.findByRole('button', { name: 'Message actions' }));
  fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
  fireEvent.change(field, { target: { value: 'Retained conflict draft' } });
  await editMessage(api, message(1), 'Incrementally observed text');
  const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);
  act(() => {
    receive({ kind: 'message', change: 'updated', channelId: channel.id, messageId: 'message-1' });
    clock.pass(1000);
  });
  await screen.findByText('Incrementally observed text');
  const before = server.sentAddresses.filter(
    (url) => url.pathname.endsWith('/messages') && !url.searchParams.has('after_seq'),
  ).length;

  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  const choose = await screen.findByRole('button', { name: 'Keep my text' });
  const blockedSave = screen.getByRole('button', { name: 'Save' }).getAttribute('aria-disabled');
  const after = server.sentAddresses.filter(
    (url) => url.pathname.endsWith('/messages') && !url.searchParams.has('after_seq'),
  ).length;
  fireEvent.click(choose);

  expect(after).toBeGreaterThan(before);
  expect(blockedSave).toBe('true');
  expect(server.chat.history(channel.id)[0]?.version).toBe(2);
  expect((field as HTMLTextAreaElement).value).toBe('Retained conflict draft');
  expect(document.activeElement).toBe(field);
  receive.cancel();
});

function fromOther(seq: number, senderLabel: string, createdAt: string): Message & { readonly senderId: string } {
  return {
    ...message(seq),
    own: false,
    body: `Other ${String(seq)}`,
    sender_label: senderLabel,
    senderId: `other-${senderLabel}`,
    created_at: createdAt,
  };
}

function rowOf(text: string): HTMLElement {
  const row = screen.getByText(text).closest('li');
  if (row === null) throw new Error('the message has no row');
  return row;
}

function pressEnter(
  field: HTMLElement,
  init: { readonly shiftKey?: boolean; readonly repeat?: boolean } = {},
): boolean {
  fireEvent.keyDown(field, { key: 'Enter', ...init });
  const lineBreakKept = fireEvent(
    field,
    new InputEvent('beforeinput', { inputType: 'insertLineBreak', bubbles: true, cancelable: true }),
  );
  fireEvent.keyUp(field, { key: 'Enter', ...init });
  return lineBreakKept;
}

async function typedComposer(text: string): Promise<HTMLTextAreaElement> {
  const field = await screen.findByRole('textbox', { name: 'Message' });
  if (!(field instanceof HTMLTextAreaElement)) throw new Error('the composer is not a text area');
  fireEvent.change(field, { target: { value: text } });
  return field;
}

describe('messenger layout', () => {
  test('own messages are marked as own and others are not, and a group chat names the first message of each group', async () => {
    const seeds = [
      fromOther(2, 'Anna', '2026-11-01T00:00:10Z'),
      fromOther(3, 'Anna', '2026-11-01T00:00:20Z'),
      fromOther(4, '<b>Boris</b>', '2026-11-01T00:00:30Z'),
    ];

    await fixture(1, '/messages/channel-1', true, { kind: 'group', seeds });
    await screen.findByText('Other 4');

    expect(rowOf('Message 1').hasAttribute('data-own')).toBe(true);
    expect(rowOf('Other 2').hasAttribute('data-own')).toBe(false);
    expect(within(rowOf('Message 1')).getByText('You').className).toBe('sr-only');
    expect(within(rowOf('Other 2')).getByText('Anna').className).not.toBe('sr-only');
    expect(within(rowOf('Other 3')).getByText('Anna').className).toBe('sr-only');
    expect(within(rowOf('Other 4')).getByText('<b>Boris</b>').className).not.toBe('sr-only');
    expect(screen.getByRole('list', { name: 'Messages' }).querySelector('b')).toBeNull();
  });

  test('a direct chat keeps every sender label for screen readers only', async () => {
    await fixture(1, '/messages/channel-1', true, { seeds: [fromOther(2, 'Anna', '2026-11-01T00:00:10Z')] });

    await screen.findByText('Other 2');

    expect(within(rowOf('Other 2')).getByText('Anna').className).toBe('sr-only');
  });

  test('each day starts with one date chip', async () => {
    const seeds = [
      { ...message(2), created_at: '2026-11-01T10:00:00Z' },
      { ...message(3), created_at: '2026-11-02T10:00:00Z' },
      { ...message(4), created_at: '2026-11-02T10:00:30Z' },
    ];

    await fixture(1, '/messages/channel-1', true, { seeds });
    await screen.findByText('Message 4');
    const first = formatDate('2026-11-01T10:00:00Z', 'ru', 'Europe/Moscow');
    const second = formatDate('2026-11-02T10:00:00Z', 'ru', 'Europe/Moscow');

    expect(within(screen.getByRole('list', { name: 'Messages' })).getAllByText(first)).toHaveLength(1);
    expect(within(screen.getByRole('list', { name: 'Messages' })).getAllByText(second)).toHaveLength(1);
    expect(within(rowOf('Message 3')).queryByText(second)).not.toBeNull();
    expect(within(rowOf('Message 4')).queryByText(second)).toBeNull();
  });

  test('Enter sends through the form instead of a line break and keeps focus in the emptied composer', async () => {
    const { server } = await fixture();
    const field = await typedComposer('Sent with Enter');

    const lineBreakKept = pressEnter(field);
    await screen.findByText('Sent with Enter');

    expect(lineBreakKept).toBe(false);
    expect(server.chat.sends.map((send) => send.body)).toEqual(['Sent with Enter']);
    await waitFor(() => {
      expect(field.value).toBe('');
    });
    expect(document.activeElement).toBe(field);
  });

  test.each([
    {
      name: 'Shift+Enter',
      lineBreak: true,
      press: (field: HTMLElement): boolean => pressEnter(field, { shiftKey: true }),
    },
    {
      name: 'a held Enter',
      lineBreak: false,
      press: (field: HTMLElement): boolean => pressEnter(field, { repeat: true }),
    },
    {
      name: 'the Enter that commits a composition before it ends',
      lineBreak: null,
      press: (field: HTMLElement): null => {
        fireEvent.compositionStart(field);
        fireEvent.keyDown(field, { key: 'Enter', isComposing: true });
        fireEvent.compositionEnd(field);
        fireEvent.keyUp(field, { key: 'Enter' });
        return null;
      },
    },
    {
      name: 'the Enter that commits a composition after it ends',
      lineBreak: null,
      press: (field: HTMLElement): null => {
        fireEvent.compositionStart(field);
        fireEvent.compositionEnd(field);
        fireEvent.keyDown(field, { key: 'Enter' });
        fireEvent.keyUp(field, { key: 'Enter' });
        return null;
      },
    },
  ])('$name sends nothing and keeps the text', async ({ press, lineBreak }) => {
    const { server } = await fixture();
    const field = await typedComposer('Not sent yet');

    const lineBreakKept = press(field);
    const busyAfterPress = field.readOnly;
    await act(async () => {
      await Promise.resolve();
    });

    expect(lineBreakKept).toBe(lineBreak);
    expect(busyAfterPress).toBe(false);
    expect(field.value).toBe('Not sent yet');
    expect(server.chat.sends).toHaveLength(0);
    expect(screen.getByRole('button', { name: 'Send' }).getAttribute('aria-busy')).not.toBe('true');
  });

  test('Enter sends after a composition that ended without a key, as dictation or a picked candidate does', async () => {
    const { server } = await fixture();
    const field = await typedComposer('Dictated text');
    fireEvent.compositionStart(field);
    fireEvent.compositionEnd(field);

    pressEnter(field);
    await screen.findByText('Dictated text');

    expect(server.chat.sends.map((send) => send.body)).toEqual(['Dictated text']);
  });

  test('Enter sends once when a text replacement comes before its line break', async () => {
    const { server } = await fixture();
    const field = await typedComposer('Corrected text');

    fireEvent.keyDown(field, { key: 'Enter' });
    fireEvent(
      field,
      new InputEvent('beforeinput', { inputType: 'insertReplacementText', bubbles: true, cancelable: true }),
    );
    fireEvent(field, new InputEvent('beforeinput', { inputType: 'insertLineBreak', bubbles: true, cancelable: true }));
    fireEvent.keyUp(field, { key: 'Enter' });
    await screen.findByText('Corrected text');

    expect(server.chat.sends.map((send) => send.body)).toEqual(['Corrected text']);
  });

  test('a second Enter while a send is in flight sends nothing more', async () => {
    const { server } = await fixture();
    const field = await typedComposer('Only once');
    const pause = server.chat.pauseNextSendResponse();

    pressEnter(field);
    await pause.answered;
    pressEnter(field);
    pressEnter(field);
    pause.resume();
    await screen.findByText('Only once');

    expect(server.chat.sends).toHaveLength(1);
    expect(server.chat.history(channel.id).filter((item) => item.body === 'Only once')).toHaveLength(1);
  });

  test('Enter offline or during an edit conflict sends nothing', async () => {
    const { server, connection, clock, api } = await fixture();
    const field = await typedComposer('Kept offline');
    act(() => {
      connection.readiness(1, false);
      clock.pass(5000);
    });

    pressEnter(field);
    act(() => {
      connection.readiness(1, true);
      clock.pass(5000);
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Message actions' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    fireEvent.change(field, { target: { value: 'Conflicting edit' } });
    await editMessage(api, message(1), 'Changed elsewhere');
    pressEnter(field);
    await screen.findByRole('button', { name: 'Keep my text' });
    const patchesBefore = server.chat.history(channel.id)[0]?.version;
    pressEnter(field);
    const busyAfterConflictEnter = field.readOnly;
    await act(async () => {
      await Promise.resolve();
    });

    expect(busyAfterConflictEnter).toBe(false);
    expect(server.chat.sends).toHaveLength(0);
    expect(server.chat.history(channel.id)[0]?.version).toBe(patchesBefore);
    expect(field.value).toBe('Conflicting edit');
  });

  test('a phone layout keeps Enter for a new line', async () => {
    const { server } = await fixture(1, '/messages/channel-1', false);
    const field = await typedComposer('Typed on a phone');

    const lineBreakKept = pressEnter(field);
    const busyAfterEnter = field.readOnly;
    await act(async () => {
      await Promise.resolve();
    });

    expect(lineBreakKept).toBe(true);
    expect(busyAfterEnter).toBe(false);
    expect(field.value).toBe('Typed on a phone');
    expect(server.chat.sends).toHaveLength(0);
  });

  test('a student may only reply to or react to another member’s message, and only in a writable chat', async () => {
    await fixture(0, '/messages/channel-1', true, {
      role: 'student',
      seeds: [fromOther(1, 'Teacher', '2026-11-01T00:00:00Z')],
    });
    await screen.findByText('Other 1');

    fireEvent.click(within(rowOf('Other 1')).getByRole('button', { name: 'Message actions' }));

    expect(
      within(rowOf('Other 1'))
        .getAllByRole('button')
        .map((button) => button.textContent),
    ).toEqual(['', 'Reply', 'React']);
  });

  test('a student sees no message actions on another member’s message in a read-only chat', async () => {
    const { server, cache } = await fixture(0, '/messages/channel-1', true, {
      role: 'student',
      seeds: [fromOther(1, 'Teacher', '2026-11-01T00:00:00Z')],
    });
    await screen.findByText('Other 1');

    server.chat.setWritable(channel.id, false);
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelKey(channel.id) });
    });

    await waitFor(() => {
      expect(within(rowOf('Other 1')).queryByRole('button', { name: 'Message actions' })).toBeNull();
    });
  });
});

describe('replies and the pinned message', () => {
  test('Reply shows the quoted message above the composer, focuses it and sends the reply', async () => {
    const { server } = await fixture(2);
    await screen.findByText('Message 1');
    const field = screen.getByRole('textbox', { name: 'Message' });

    fireEvent.click(within(rowOf('Message 1')).getByRole('button', { name: 'Message actions' }));
    fireEvent.click(screen.getByRole('button', { name: 'Reply' }));
    const focusAfterReply = document.activeElement;
    const bar = screen.getByRole('button', { name: 'Cancel reply' }).parentElement?.textContent;
    fireEvent.change(field, { target: { value: 'The answer' } });
    fireEvent.click(screen.getByRole('button', { name: 'Send' }));
    await screen.findByText('The answer');

    expect(focusAfterReply).toBe(field);
    expect(bar).toContain('Message 1');
    expect(server.chat.sends.map((send) => send.replyTo)).toEqual(['message-1']);
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Cancel reply' })).toBeNull();
    });
  });

  test('Escape cancels a reply and editing hides the reply bar until the edit ends', async () => {
    await fixture(2);
    await screen.findByText('Message 1');
    const field = screen.getByRole('textbox', { name: 'Message' });
    const startReply = (): void => {
      fireEvent.click(within(rowOf('Message 1')).getByRole('button', { name: 'Message actions' }));
      fireEvent.click(screen.getByRole('button', { name: 'Reply' }));
    };

    startReply();
    fireEvent.keyDown(field, { key: 'Escape' });
    const afterEscape = screen.queryByRole('button', { name: 'Cancel reply' });
    const focusAfterEscape = document.activeElement;
    startReply();
    fireEvent.click(within(rowOf('Message 2')).getByRole('button', { name: 'Message actions' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    const whileEditing = screen.queryByRole('button', { name: 'Cancel reply' });
    fireEvent.keyDown(field, { key: 'Escape' });
    const afterEdit = screen.queryByRole('button', { name: 'Cancel reply' });

    expect(afterEscape).toBeNull();
    expect(focusAfterEscape).toBe(field);
    expect(whileEditing).toBeNull();
    expect(afterEdit).not.toBeNull();
  });

  test('a reply whose message was deleted meanwhile drops the bar with its notice', async () => {
    const { api, cache, clock } = await fixture(2);
    await screen.findByText('Message 1');
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);
    fireEvent.click(within(rowOf('Message 1')).getByRole('button', { name: 'Message actions' }));
    fireEvent.click(screen.getByRole('button', { name: 'Reply' }));

    await deleteMessage(api, 'message-1');
    act(() => {
      receive({ kind: 'message', change: 'deleted', channelId: channel.id, messageId: 'message-1' });
      clock.pass(1000);
    });

    await screen.findByText('The message you replied to is no longer available.');
    expect(screen.queryByRole('button', { name: 'Cancel reply' })).toBeNull();
  });

  test('a send that replies to a message gone on the server drops the bar with its notice', async () => {
    const { api } = await fixture(2);
    await screen.findByText('Message 1');
    const field = screen.getByRole('textbox', { name: 'Message' });
    fireEvent.click(within(rowOf('Message 1')).getByRole('button', { name: 'Message actions' }));
    fireEvent.click(screen.getByRole('button', { name: 'Reply' }));
    await deleteMessage(api, 'message-1');

    fireEvent.change(field, { target: { value: 'Too late' } });
    fireEvent.click(screen.getByRole('button', { name: 'Send' }));
    await screen.findByText('The message you replied to is no longer available.');

    expect(screen.queryByRole('button', { name: 'Cancel reply' })).toBeNull();
    expect((field as HTMLTextAreaElement).value).toBe('Too late');
  });

  test('Show older inside a window moves to the older messages and focuses one of them', async () => {
    await fixture(120, '/messages/channel-1', true, {
      seeds: [{ ...message(121), body: 'Reply from far ahead', replyTo: 'message-40' }],
    });
    await screen.findByText('Reply from far ahead');
    fireEvent.click(within(rowOf('Reply from far ahead')).getByRole('button', { name: /Message 40$/ }));
    await waitFor(() => {
      expect(document.activeElement?.getAttribute('data-row')).toBe('message-40');
    });

    fireEvent.click(screen.getByRole('button', { name: 'Show older' }));

    await waitFor(() => {
      expect(document.activeElement?.getAttribute('data-row')).toBe('message-16');
    });
    expect(screen.getByText('Message 1')).toBeTruthy();
  });

  test('a quote renders markup and addresses as text and jumps to its loaded message', async () => {
    const quoted = { ...message(2), body: '<b>Bold</b> https://example.org/a' };
    const replying = { ...message(3), body: 'Replying here', replyTo: 'message-2' };
    await fixture(1, '/messages/channel-1', true, { seeds: [quoted, replying] });
    await screen.findByText('Replying here');
    const quote = within(rowOf('Replying here')).getByRole('button', { name: /Bold/ });

    fireEvent.click(quote);

    expect(quote.querySelector('b, a')).toBeNull();
    expect(quote.textContent).toContain('<b>Bold</b> https://example.org/a');
    expect(document.activeElement?.getAttribute('data-row')).toBe('message-2');
  });

  test('a quote of an unloaded message opens a window around it until Jump to latest', async () => {
    const { api, cache, clock } = await fixture(60, '/messages/channel-1', true, {
      seeds: [{ ...message(61), body: 'Reply far back', replyTo: 'message-1' }],
    });
    await screen.findByText('Reply far back');
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);

    fireEvent.click(within(rowOf('Reply far back')).getByRole('button', { name: /Message 1$/ }));
    await waitFor(() => {
      expect(document.activeElement?.getAttribute('data-row')).toBe('message-1');
    });
    const listsWhileDetached = screen.getAllByRole('list', { name: 'Messages' }).length;
    const trackedWhileDetached = Array.from(document.querySelectorAll('[data-seq]')).filter(
      (row) => row.closest('[hidden]') === null,
    ).length;
    await deleteMessage(api, 'message-1');
    act(() => {
      receive({ kind: 'message', change: 'deleted', channelId: channel.id, messageId: 'message-1' });
      clock.pass(1000);
    });
    await waitFor(() => {
      expect(screen.queryByText('Message 1')).toBeNull();
    });
    const windowRow = Array.from(document.querySelectorAll<HTMLElement>('[data-row]')).find(
      (row) => row.dataset.row === 'message-1' && row.closest('[hidden]') === null,
    );
    fireEvent.click(screen.getByRole('button', { name: /^Jump to latest/ }));

    expect(listsWhileDetached).toBe(1);
    expect(windowRow?.textContent).toContain('Message deleted');
    expect(trackedWhileDetached).toBe(0);
    expect(screen.getByRole('textbox', { name: 'Message' })).toBe(document.activeElement);
    expect(rowOf('Reply far back').closest('[hidden]')).toBeNull();
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: /^Jump to latest/ })).toBeNull();
    });
    await waitFor(() => {
      expect(within(rowOf('Reply far back')).getByText('Message deleted')).toBeTruthy();
    });
  });

  test('an own message sent from another tab while a window is open neither scrolls nor clears the new count', async () => {
    const { server, cache, clock } = await fixture(60, '/messages/channel-1', true, {
      seeds: [{ ...message(61), body: 'Reply far back', replyTo: 'message-1' }],
    });
    await screen.findByText('Reply far back');
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);
    fireEvent.click(within(rowOf('Reply far back')).getByRole('button', { name: /Message 1$/ }));
    await waitFor(() => {
      expect(document.activeElement?.getAttribute('data-row')).toBe('message-1');
    });
    server.chat.addMessage(channel.id, { ...fromOther(62, 'Student', '2026-11-01T00:01:00Z'), id: 'message-62' });
    act(() => {
      receive({ kind: 'message', change: 'new', channelId: channel.id, messageId: 'message-62', seq: 62 });
      clock.pass(1000);
    });
    await screen.findByRole('button', { name: 'Jump to latest, 1 new' });
    const windowRow = document.activeElement;
    server.chat.addMessage(channel.id, { ...message(63), body: 'Sent from another tab' });

    act(() => {
      receive({ kind: 'message', change: 'new', channelId: channel.id, messageId: 'message-63', seq: 63 });
      clock.pass(1000);
    });

    await waitFor(() => {
      expect(cache.getQueryData<MessagePage>(messagesKey(channel.id))?.items.some((item) => item.seq === 63)).toBe(
        true,
      );
    });
    expect(screen.getByRole('button', { name: 'Jump to latest, 1 new' })).toBeTruthy();
    expect(windowRow?.closest('[hidden]')).toBeNull();
    expect(document.activeElement).toBe(windowRow);
    receive.cancel();
  });

  test('the attachment addresses of a window around an older message are refreshed before they expire', async () => {
    const withPicture = {
      ...message(1),
      attachments: [
        {
          id: 'old-picture',
          filename: 'old-picture.png',
          media_type: 'image/png',
          width: 2,
          height: 2,
          size_bytes: 24,
          url: '/api/v1/files/old-picture?t=test',
          expires_at: new Date(60_000).toISOString(),
        },
      ],
    };
    const { server, clock } = await fixture(0, '/messages/channel-1', true, {
      seeds: [
        withPicture,
        ...Array.from({ length: 59 }, (_, index) => message(index + 2)),
        { ...message(61), body: 'Reply far back', replyTo: 'message-1' },
      ],
    });
    await screen.findByText('Reply far back');
    fireEvent.click(within(rowOf('Reply far back')).getByRole('button', { name: /Message 1$/ }));
    await screen.findByRole('img', { name: 'old-picture.png' });
    await act(async () => {
      await Promise.resolve();
    });
    const aroundReads = (): number =>
      server.sentAddresses.filter(
        (address) =>
          address.pathname === `/api/v1/channels/${channel.id}/messages` && address.searchParams.has('around_seq'),
      ).length;
    const before = aroundReads();

    await act(async () => {
      clock.pass(30_000);
      clock.pass(1000);
      await Promise.resolve();
    });

    await waitFor(() => {
      expect(aroundReads()).toBeGreaterThan(before);
    });
  });

  test('losing the channel drops every window around its messages', async () => {
    const { cache } = await fixture(1);
    act(() => {
      cache.setQueryData(aroundKey(channel.id, 1), {
        items: [message(1)],
        next_cursor: null,
        has_newer: false,
        peer_read_seq: null,
      });
    });

    act(() => {
      evictChannel(cache, channel.id);
    });

    expect(cache.getQueryCache().findAll({ queryKey: aroundPrefix(channel.id) })).toHaveLength(0);
  });

  test('the tutor pins from the menu, sees the pinned bar and unpins onto the heading', async () => {
    const { server } = await fixture(2);
    await screen.findByText('Message 1');
    const menu = within(rowOf('Message 1')).getByRole('button', { name: 'Message actions' });

    fireEvent.click(menu);
    fireEvent.click(screen.getByRole('button', { name: 'Pin' }));
    const bar = await screen.findByRole('region', { name: 'Pinned message' });
    const focusAfterPin = document.activeElement;
    fireEvent.click(within(bar).getByRole('button', { name: 'Unpin' }));
    await waitFor(() => {
      expect(screen.queryByRole('region', { name: 'Pinned message' })).toBeNull();
    });

    expect(bar.textContent).toContain('Message 1');
    expect(focusAfterPin).toBe(menu);
    expect(server.chat.pinOf(channel.id)).toBeNull();
    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByRole('heading', { level: 2 }));
    });
  });

  test('a pin change keeps the pinned chat’s row, its continuation page and the focus on the row', async () => {
    const { server, cache, clock } = await fixture(0, '/messages', true, { followingQueryClient: true });
    seedChannels(server, 31);
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelsKey });
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Show more' }));
    const row = await screen.findByRole('link', { name: 'Student' });
    row.focus();
    const firstKey = channelPageKey('', 'all', 0, null);
    const readsBefore = cache.getQueryState(firstKey)?.dataUpdateCount ?? 0;
    const scopeBefore = channelScopeRevision(cache);
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);

    act(() => {
      receive({ kind: 'entity', entity: 'channel_pin', id: channel.id });
      clock.pass(1000);
    });

    await waitFor(() => {
      expect(cache.getQueryState(firstKey)?.dataUpdateCount ?? 0).toBeGreaterThan(readsBefore);
    });
    expect(screen.getByRole('link', { name: 'Student' })).toBe(row);
    expect(document.activeElement).toBe(row);
    expect(screen.getByRole('link', { name: 'Chat 31' })).toBeDefined();
    expect(channelScopeRevision(cache)).toBe(scopeBefore);
    receive.cancel();
  });

  test('a student sees the pinned message without Unpin and its quote of markup as text', async () => {
    const { server, cache } = await fixture(0, '/messages/channel-1', true, {
      role: 'student',
      seeds: [{ ...fromOther(1, 'Teacher', '2026-11-01T00:00:00Z'), body: '<i>Pinned</i>' }],
    });
    await screen.findByText('<i>Pinned</i>');

    server.chat.pin(channel.id, 'message-1');
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelKey(channel.id) });
    });
    const bar = await screen.findByRole('region', { name: 'Pinned message' });

    expect(within(bar).queryByRole('button', { name: 'Unpin' })).toBeNull();
    expect(bar.querySelector('i')).toBeNull();
    expect(bar.textContent).toContain('<i>Pinned</i>');
  });
});

describe('chat list filters and rows', () => {
  test('choosing a filter requests the first page with that filter and keeps focus on the pressed filter', async () => {
    const { server } = await fixture(0, '/messages');
    const button = await screen.findByRole('button', { name: 'Groups' });

    fireEvent.click(button);
    await screen.findByText('No group chats');

    expect(button.getAttribute('aria-pressed')).toBe('true');
    expect(document.activeElement).toBe(button);
    expect(
      server.sentAddresses
        .filter((url) => url.pathname === '/api/v1/channels')
        .at(-1)
        ?.searchParams.get('filter'),
    ).toBe('group');
  });

  test('the filter is restored from the address', async () => {
    const address = '/messages?filter=direct';

    const { server } = await fixture(0, address);
    const row = await screen.findByRole('link', { name: 'Student' });

    expect(screen.getByRole('button', { name: 'Students' }).getAttribute('aria-pressed')).toBe('true');
    expect(row.getAttribute('href')).toBe('/messages/channel-1?filter=direct');
    expect(
      server.sentAddresses
        .filter((url) => url.pathname === '/api/v1/channels')
        .at(-1)
        ?.searchParams.get('filter'),
    ).toBe('direct');
  });

  test('the search term is never written to the address', async () => {
    const { router, clock, server } = await fixture(0, '/messages?filter=direct');
    const search = await screen.findByRole('searchbox', { name: 'Search chats' });

    fireEvent.change(search, { target: { value: 'private name' } });
    act(() => {
      clock.pass(200);
    });
    await screen.findByText('No chats');

    expect(router.state.location.search).toBe('?filter=direct');
    expect(new URLSearchParams(router.state.location.search).has('q')).toBe(false);
    expect(
      server.sentAddresses
        .filter((url) => url.pathname === '/api/v1/channels')
        .at(-1)
        ?.searchParams.get('q'),
    ).toBe('private name');
  });

  test('an unknown filter value in the address shows All and is not sent', async () => {
    const value = '<script>private</script>';

    const { server } = await fixture(0, `/messages?filter=${encodeURIComponent(value)}`);
    await screen.findByRole('link', { name: 'Student' });

    expect(screen.getByRole('button', { name: 'All' }).getAttribute('aria-pressed')).toBe('true');
    expect(
      server.sentAddresses
        .filter((url) => url.pathname === '/api/v1/channels')
        .every((url) => !url.searchParams.has('filter')),
    ).toBe(true);
    expect(document.body.textContent).not.toContain(value);
  });

  test("the Unread filter's name and badge show 99+ above 99", async () => {
    const { server, cache } = await fixture(0, '/messages');
    seedChannels(server, 100);
    for (let index = 1; index <= 100; index += 1) server.chat.addMessage(`pagination-${String(index)}`, message(1));

    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelsKey });
    });

    const button = await screen.findByRole('button', { name: 'Unread, 99+' });
    expect(button.textContent).toBe('Unread99+');
  });

  test('a new message that moves a chat into the Unread first page detaches the loaded continuation pages', async () => {
    const { server, cache, clock } = await fixture(0, '/messages?filter=unread');
    seedChannels(server, 61);
    for (let index = 60; index >= 1; index -= 1) server.chat.addMessage(`pagination-${String(index)}`, message(1));
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelsKey });
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Show more' }));
    await screen.findByText('Chat 60');
    const receive = liveRefetch(cache, { arrived: (): void => undefined }, clock);

    server.chat.addMessage('pagination-61', message(2));
    act(() => {
      receive({ kind: 'entity', entity: 'channel', id: 'pagination-61' });
      clock.pass(1000);
    });
    await screen.findByText('Chat 61');

    expect(screen.queryByText('Chat 60')).toBeNull();
    expect(within(screen.getByRole('region', { name: 'Chats' })).getAllByRole('link')).toHaveLength(30);
    receive.cancel();
  });

  test("losing a channel's scope removes its row from a Groups page and its continuation", async () => {
    const { server, cache } = await fixture(0, '/messages?filter=group');
    for (let index = 31; index >= 1; index -= 1)
      server.chat.addChannel({
        channel: { ...channel, id: `group-${String(index)}`, label: `Group ${String(index)}`, kind: 'group' },
        members: [{ accountId: tutor.id }],
      });
    await act(async () => {
      await cache.invalidateQueries({ queryKey: channelsKey });
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Show more' }));
    await screen.findByText('Group 31');

    server.chat.removeMember('group-1', tutor.id);
    await act(async () => {
      evictChannel(cache, 'group-1');
      await cache.invalidateQueries({ queryKey: channelPageKey('', 'group', 0, null) });
    });

    await waitFor(() => {
      expect(screen.queryByText('Group 1')).toBeNull();
    });
    expect(screen.queryByText('Group 31')).not.toBeNull();
    for (const [, page] of cache.getQueriesData<import('./api.ts').ChannelPage>({ queryKey: channelsKey }))
      expect(page?.items.some((item) => item.id === 'group-1')).toBe(false);
  });

  test('a history change of only the filter moves focus from the list to the pressed filter', async () => {
    const { router } = await fixture(0, '/messages?filter=direct');
    fireEvent.click(await screen.findByRole('button', { name: 'All' }));
    const row = await screen.findByRole('link', { name: 'Student' });
    row.focus();

    await act(async () => {
      await router.navigate(-1);
      await Promise.resolve();
    });
    await screen.findByRole('link', { name: 'Student' });
    await act(async () => {
      await Promise.resolve();
    });

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Students' }));
  });
});
