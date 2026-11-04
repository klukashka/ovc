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
import { MediaQueriesContext, PreferencesContext, fixedMediaQueries, wideLayoutQuery } from '../../app/context.ts';
import { type ApiClient, ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { type Channel, type Message, deleteMessage, editMessage } from './api.ts';
import { channelsScopeRevisionKey, channelScopeRevision, evictChannel } from './cache.ts';
import { channelKey, channelsKey, messagesKey } from './keys.ts';
import { MessageKeyContext } from './Conversation.tsx';
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
  server.chat.addChannel({ channel, members: [{ accountId: tutor.id }, { accountId: 'student-1' }] });
  for (let seq = 1; seq <= count; seq += 1)
    server.chat.addMessage(channel.id, { ...message(seq), sent_before_reclaim: options.reclaimed ?? false });
  const clock = manualScheduler();
  const session = new Session({ origin, transport: server.transport(), locks: memoryLocks(), scheduler: clock });
  await session.signIn(tutor.email, tutor.password);
  const connection = new ConnectionState(clock);
  connection.account(1);
  connection.readiness(1, true);
  const queryOptions = { defaultOptions: { queries: { retry: false } } };
  const cache =
    options.initialReadMode === undefined ? new QueryClient(queryOptions) : new FollowingQueryClient(queryOptions);
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
                    value={fixedMediaQueries(
                      wide
                        ? options.initialReadMode === undefined
                          ? ['(min-width: 768px)']
                          : ['(min-width: 768px)', wideLayoutQuery]
                        : [],
                    )}
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
  await screen.findByRole('heading', { level: address === '/messages' ? 1 : 2 });
  return { server, initialReads, api, cache, clock, connection, router, session };
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
    const edit = screen.getByRole('button', { name: 'Edit' });
    const focusedFirst = document.activeElement;
    fireEvent.keyDown(edit, { key: 'Escape' });

    expect(focusedFirst).toBe(edit);
    expect(document.activeElement).toBe(menu);
    expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull();
  });

  test('editing focuses the composer and Escape restores the draft and message focus', async () => {
    await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    fireEvent.change(field, { target: { value: 'Unsent draft' } });

    fireEvent.click(screen.getByRole('button', { name: 'Message actions' }));
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

  test('touch long press keeps the menu open through pointerup and its generated click', async () => {
    const { clock } = await fixture();
    const menu = await screen.findByRole('button', { name: 'Message actions' });

    fireEvent.pointerDown(menu, { pointerType: 'touch' });
    act(() => {
      clock.pass(500);
    });
    fireEvent.pointerUp(menu, { pointerType: 'touch' });
    fireEvent.click(menu);

    expect(screen.getByRole('button', { name: 'Edit' })).toBe(document.activeElement);
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
    expect(document.activeElement).toBe(field);
  });

  test.each(['Keep my text', 'Use latest text'] as const)(
    'a stale edit adopts a fresh version only after %s and saves explicitly',
    async (choice) => {
      const { server, api } = await fixture();
      const field = await screen.findByRole('textbox', { name: 'Message' });
      fireEvent.change(field, { target: { value: 'Original draft' } });
      fireEvent.click(screen.getByRole('button', { name: 'Message actions' }));
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
    fireEvent.click(screen.getByRole('button', { name: 'Message actions' }));
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
    fireEvent.click(screen.getByRole('button', { name: 'Message actions' }));
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
    fireEvent.click(screen.getByRole('button', { name: 'Message actions' }));
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
    fireEvent.click(screen.getByRole('button', { name: 'Message actions' }));
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
      fireEvent.click(screen.getByRole('button', { name: 'Message actions' }));
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
      await cache.invalidateQueries({ queryKey: [...channelsKey, '', null], exact: true });
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

      const first = screen.getByText('Message 1').closest('li');
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

  test('a failed deletion returns focus to its surviving message', async () => {
    const { server } = await fixture();
    const row = (await screen.findByText('Message 1')).closest('li');
    server.chat.failNextDelete();
    fireEvent.click(screen.getByRole('button', { name: 'Message actions' }));

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    await screen.findByRole('alert');

    await waitFor(() => {
      expect(document.activeElement).toBe(row);
    });
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
      expect(document.activeElement).toBe(field);
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
  fireEvent.click(screen.getByRole('button', { name: 'Message actions' }));
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
