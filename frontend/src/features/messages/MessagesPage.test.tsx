import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { liveRefetch } from '../../app/liveRefetch.ts';
import { SchedulerContext } from '../../app/clock.ts';
import { ConnectionContext, ConnectionState } from '../../app/connectionState.ts';
import { MediaQueriesContext, fixedMediaQueries } from '../../app/context.ts';
import { type ApiClient, ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { type Channel, type Message, editMessage } from './api.ts';
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
  options: { readonly role?: 'tutor' | 'student'; readonly reclaimed?: boolean } = {},
): Promise<{
  readonly server: FakeAuthServer;
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
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createMemoryRouter(
    [
      {
        path: '/messages/:channelId?',
        element: (
          <main>
            <MessagesPage />
          </main>
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
  render(
    <SessionContext value={session}>
      <ApiClientContext value={api}>
        <QueryClientProvider client={cache}>
          <I18nextProvider i18n={i18n}>
            <SchedulerContext value={clock}>
              <ConnectionContext value={connection}>
                <MediaQueriesContext value={fixedMediaQueries(wide ? ['(min-width: 768px)'] : [])}>
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
    </SessionContext>,
  );
  await screen.findByRole('heading', { level: address === '/messages' ? 1 : 2 });
  return { server, api, cache, clock, connection, router, session };
}

afterEach(cleanup);

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

  test('a stale edit refreshes history and preserves the unsaved text', async () => {
    const { server, api } = await fixture();
    const field = await screen.findByRole('textbox', { name: 'Message' });
    fireEvent.click(screen.getByRole('button', { name: 'Message actions' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    fireEvent.change(field, { target: { value: 'My unsaved edit' } });
    await editMessage(api, message(1), 'Changed elsewhere');

    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await screen.findByRole('alert');
    await screen.findByText('Changed elsewhere');

    expect((field as HTMLTextAreaElement).value).toBe('My unsaved edit');
    expect(server.chat.history(channel.id)[0]?.body).toBe('Changed elsewhere');
  });
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
