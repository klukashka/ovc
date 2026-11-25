import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { ConnectionContext, ConnectionState } from '../../app/connectionState.ts';
import {
  MediaQueriesContext,
  PreferencesContext,
  fixedMediaQueries,
  phoneLayoutQuery,
  wideLayoutQuery,
} from '../../app/context.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { type FakeAccountSeed, FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { type Me, Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import { memoryPreferenceStore } from '../../lib/preferences/store.ts';
import { sharedPages, studentPages } from '../../routes/router.tsx';
import { RouteFocus } from './RouteFocus.tsx';
import { SignedInFrame } from './SignedInFrame.tsx';

function account(role: Me['role'], state: Me['state'] = 'active'): FakeAccountSeed {
  return {
    id: '00000000-0000-4000-8000-000000000002',
    name: 'A student of languages',
    email: 'student@example.org',
    password: 'a long password',
    role,
    state,
  };
}

async function signedInSession(seed: FakeAccountSeed): Promise<{ session: Session; me: Me; server: FakeAuthServer }> {
  const server = new FakeAuthServer();
  server.addAccount(seed);
  const session = new Session({
    origin: 'https://ovc.example',
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(seed.email, seed.password);
  const state = session.snapshot();
  if (state.kind !== 'signedIn') {
    throw new Error('the frame test account did not sign in');
  }
  return { session, me: state.me, server };
}

async function renderFrame(
  seed: FakeAccountSeed,
  address: string,
  wide: boolean | 'phone',
  language: Language = 'ru',
  title?: 'page.notFound',
  connection = new ConnectionState(manualScheduler()),
  chatUnread = false,
): Promise<FakeAuthServer> {
  const i18n = await createI18n(language);
  const { session, me, server } = await signedInSession(seed);
  if (chatUnread) {
    server.chat.addChannel({
      channel: {
        id: 'chat-a',
        kind: 'direct',
        label: 'A tutor',
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
      members: [{ accountId: me.id }],
    });
    server.chat.addMessage('chat-a', {
      id: 'message-a',
      seq: 1,
      body: 'A new message',
      attachments: [],
      created_at: '2026-10-31T10:00:00Z',
      deleted: false,
      erased: false,
      edited_at: null,
      own: false,
      sender_label: 'A tutor',
      sender_label_kind: null,
      sent_before_reclaim: false,
      reply_to: null,
      version: 1,
      senderId: 'tutor-a',
    });
  }
  const router = createMemoryRouter(
    [
      {
        element: <RouteFocus />,
        children: [
          {
            element: <SignedInFrame me={me} {...(title === undefined ? {} : { title })} />,
            children: [...studentPages, ...sharedPages],
          },
        ],
      },
    ],
    { initialEntries: [address] },
  );
  render(
    <ConnectionContext value={connection}>
      <MediaQueriesContext
        value={fixedMediaQueries(wide === 'phone' ? [phoneLayoutQuery] : wide ? [wideLayoutQuery] : [])}
      >
        <PreferencesContext value={memoryPreferenceStore()}>
          <SessionContext value={session}>
            <ApiClientContext value={createApiClient('https://ovc.example', session.authorizedFetch)}>
              <QueryClientProvider client={new QueryClient()}>
                <I18nextProvider i18n={i18n}>
                  <RouterProvider router={router} />
                </I18nextProvider>
              </QueryClientProvider>
            </ApiClientContext>
          </SessionContext>
        </PreferencesContext>
      </MediaQueriesContext>
    </ConnectionContext>,
  );
  return server;
}

async function click(element: HTMLElement): Promise<void> {
  await act(async () => {
    element.click();
    await Promise.resolve();
  });
}

afterEach(() => {
  cleanup();
});

function topBar(): HTMLElement {
  const bar = screen.getAllByRole('banner').find((element) => element.closest('main') === null);
  if (bar === undefined) {
    throw new Error('the frame shows no top bar');
  }
  return bar;
}

describe('the signed-in frame of a student', () => {
  test('shows the student sections in English and no tutor sections', async () => {
    await renderFrame(account('student'), '/homework', true, 'en');

    const navigation = screen.getByRole('navigation', { name: 'Main navigation' });

    const labels = within(navigation)
      .getAllByRole('link')
      .map((link) => link.textContent);
    expect(labels).toEqual(['Home', 'Schedule', 'Homework', 'Messages', 'My tutors', 'Settings']);
    expect(within(navigation).getByRole('button', { name: 'Notifications' })).toBeDefined();
    expect(screen.getByRole('button', { name: 'A student of languages, Account menu' })).toBeDefined();
  });

  test('a shared page opened from the sidebar keeps the student frame and focuses its heading', async () => {
    await renderFrame(account('student'), '/homework', true);

    await click(within(screen.getByRole('navigation')).getByRole('link', { name: 'Расписание' }));

    const heading = screen.getByRole('heading', { level: 1, name: 'Расписание' });
    expect(document.activeElement).toBe(heading);
    expect(within(screen.getByRole('navigation')).getByRole('link', { name: 'Мои репетиторы' })).toBeDefined();
    expect(within(screen.getByRole('navigation')).queryByRole('link', { name: 'Ученики' })).toBeNull();
  });

  test('below 1024 px the notifications page keeps the student frame, the top bar keeps the bell and the overlay offers the panel', async () => {
    await renderFrame(account('student'), '/notifications', false);

    const heading = await screen.findByRole('heading', { level: 1, name: 'Уведомления' });

    expect(heading).toBeDefined();
    expect(within(topBar()).getByRole('button', { name: 'Уведомления' })).toBeDefined();
    const overlay = screen.getByRole('dialog', { hidden: true });
    const overlayItem = within(overlay).getByRole('button', { name: 'Уведомления', hidden: true });
    expect(overlayItem.getAttribute('aria-expanded')).toBe('false');
    expect(within(overlay).queryByRole('link', { name: 'Уведомления', hidden: true })).toBeNull();
    expect(within(overlay).getByRole('link', { name: 'Мои репетиторы', hidden: true })).toBeDefined();
    expect(within(overlay).queryByRole('link', { name: 'Ученики', hidden: true })).toBeNull();
  });

  test('below 1024 px a title override replaces the route title in the top bar', async () => {
    await renderFrame(account('student'), '/homework', false, 'ru', 'page.notFound');

    const banner = topBar();

    expect(within(banner).getByText('Страница не найдена')).toBeDefined();
    expect(within(banner).queryByText('Задания')).toBeNull();
  });
});

describe('the signed-in frame of an unconfirmed student', () => {
  test('lists only Home, My tutors and Settings in the sidebar', async () => {
    await renderFrame(account('student', 'unverified'), '/homework', true);

    const navigation = screen.getByRole('navigation', { name: 'Основная навигация' });

    const labels = within(navigation)
      .getAllByRole('link')
      .map((link) => link.textContent);
    expect(labels).toEqual(['Главная', 'Мои репетиторы', 'Настройки']);
  });

  test('below 1024 px the top bar has no notifications bell', async () => {
    await renderFrame(account('student', 'unverified'), '/homework', false);

    const banner = topBar();

    expect(within(banner).queryByRole('button', { name: /Уведомления/ })).toBeNull();
    expect(within(banner).queryByRole('link', { name: /Уведомления/ })).toBeNull();
  });

  test('below 1024 px the sidebar overlay offers no notifications panel', async () => {
    await renderFrame(account('student', 'unverified'), '/homework', false);

    const overlay = screen.getByRole('dialog', { hidden: true });

    expect(within(overlay).getByRole('link', { name: 'Мои репетиторы', hidden: true })).toBeDefined();
    expect(within(overlay).queryByRole('button', { name: /Уведомления/, hidden: true })).toBeNull();
    expect(within(overlay).queryByRole('link', { name: /Уведомления/, hidden: true })).toBeNull();
  });

  test('the confirmation banner asks to confirm to keep studying and offers to send the link again', async () => {
    await renderFrame(account('student', 'unverified'), '/homework', true);

    const banner = screen.getByRole('region', { name: 'Подтверждение email' });

    expect(banner.textContent).toContain('Подтвердите email, чтобы продолжить занятия с репетитором.');
    expect(banner.textContent).toContain('student@example.org');
    expect(within(banner).getByRole('button', { name: 'Отправить ссылку ещё раз' })).toBeDefined();
  });
});

describe('the confirmation banner', () => {
  test('an unverified account sees the confirmation banner on every page', async () => {
    await renderFrame(account('tutor', 'unverified'), '/schedule', true);

    const banner = screen.getByRole('region', { name: 'Подтверждение email' });

    expect(banner.textContent).toContain('student@example.org');
    expect(within(banner).getByRole('link', { name: 'Подтвердить email' }).getAttribute('href')).toBe('/verify-email');
  });

  test('an active account sees no confirmation banner', async () => {
    await renderFrame(account('student'), '/schedule', true);

    const banner = screen.queryByRole('region', { name: 'Подтверждение email' });

    expect(banner).toBeNull();
  });
});

async function openedPanelHeading(bell: HTMLElement): Promise<HTMLElement> {
  await click(bell);
  const heading = await screen.findByRole('heading', { level: 2, name: 'Уведомления' });
  await waitFor(() => {
    expect(document.activeElement).toBe(heading);
  });
  return heading;
}

async function nextFrame(): Promise<void> {
  await new Promise((resolve) => requestAnimationFrame(resolve));
}

describe('the notifications bell', () => {
  test('a click outside the panel closes it and returns focus to the bell', async () => {
    await renderFrame(account('tutor'), '/schedule', true);
    const bell = within(screen.getByRole('navigation')).getByRole('button', { name: 'Уведомления' });
    await openedPanelHeading(bell);

    await act(async () => {
      document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }));
      await nextFrame();
    });

    expect(screen.queryByRole('region', { name: 'Уведомления' })).toBeNull();
    expect(bell.getAttribute('aria-expanded')).toBe('false');
    expect(document.activeElement).toBe(bell);
  });

  test('below 1024 px the top bar bell opens the panel on its heading and Escape returns focus to the bell', async () => {
    await renderFrame(account('student'), '/homework', false);
    const bell = within(topBar()).getByRole('button', { name: 'Уведомления' });
    const heading = await openedPanelHeading(bell);

    await act(async () => {
      heading.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
      await Promise.resolve();
    });

    expect(screen.queryByRole('region', { name: 'Уведомления' })).toBeNull();
    expect(document.activeElement).toBe(bell);
  });
});

test('navigation retains the offline live region without repeating its announcement', async () => {
  const clock = manualScheduler();
  const connection = new ConnectionState(clock);
  connection.account(1);
  await renderFrame(account('student'), '/homework', true, 'en', undefined, connection);
  act(() => {
    clock.pass(5000);
  });
  const banner = screen.getByText('No connection — reconnecting…');
  let changes = 0;
  const observer = new MutationObserver((records) => {
    changes += records.length;
  });
  observer.observe(banner, { childList: true, characterData: true, subtree: true });

  await click(within(screen.getByRole('navigation')).getByRole('link', { name: 'Schedule' }));
  observer.disconnect();

  expect(screen.getByText('No connection — reconnecting…')).toBe(banner);
  expect(changes).toBe(0);
  expect(banner.closest('main')).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Schedule' }));
});

test('the Messages unread channel count also marks the hidden sidebar', async () => {
  await renderFrame(
    account('student'),
    '/homework',
    false,
    'en',
    undefined,
    new ConnectionState(manualScheduler()),
    true,
  );

  const showSidebar = await screen.findByRole('button', { name: /Show sidebar.*new items/i });

  expect(showSidebar.querySelector('[aria-hidden="true"]')).not.toBeNull();
  const messages = screen.getByRole('link', { name: /Messages.*1/i, hidden: true });
  expect(messages.getAttribute('href')).toBe('/messages');
  expect(within(messages).getByText('1')).toBeDefined();
});

function bottomBar(): HTMLElement {
  return screen.getByRole('navigation', { name: /Main navigation|Основная навигация/ });
}

function sheet(): HTMLElement {
  const dialog = document.querySelector<HTMLElement>('dialog[aria-label="More"], dialog[aria-label="Ещё"]');
  if (dialog === null) {
    throw new Error('the frame shows no More sheet');
  }
  return dialog;
}

function linkLabels(container: HTMLElement): (string | null)[] {
  return within(container)
    .queryAllByRole('link', { hidden: true })
    .map((link) => link.textContent);
}

describe('the phone frame', () => {
  test('a student’s bottom bar holds Home, Schedule, Homework, Chats and More, and names the Chats count from its label', async () => {
    await renderFrame(account('student'), '/homework', 'phone', 'en', undefined, undefined, true);

    const chats = await within(bottomBar()).findByRole('link', { name: 'Chats, 1 unread' });

    expect(linkLabels(bottomBar())).toEqual(['Home', 'Schedule', 'Homework', '1Chats']);
    expect(chats.getAttribute('href')).toBe('/messages');
    expect(within(bottomBar()).getByRole('button', { name: 'More' }).getAttribute('aria-expanded')).toBe('false');
    expect(screen.queryByRole('button', { name: /Show sidebar/ })).toBeNull();
    expect(screen.queryByRole('button', { name: /Account menu/ })).toBeNull();
  });

  test('an unconfirmed student’s bar shows Home and More, and the sheet lists My tutors, Settings and Sign out only', async () => {
    await renderFrame(account('student', 'unverified'), '/home', 'phone');

    const bar = bottomBar();

    expect(linkLabels(bar)).toEqual(['Главная']);
    expect(within(bar).getByRole('button', { name: 'Ещё' })).toBeDefined();
    expect(linkLabels(sheet())).toEqual(['Мои репетиторы', 'Настройки']);
    expect(within(sheet()).getByRole('button', { name: 'Выйти', hidden: true })).toBeDefined();
    expect(within(topBar()).queryByRole('button', { name: /Уведомления/ })).toBeNull();
  });

  test('an unconfirmed tutor sees the full bar without a Homework count', async () => {
    await renderFrame(account('tutor', 'unverified'), '/schedule', 'phone', 'en');

    const bar = bottomBar();

    expect(linkLabels(bar)).toEqual(['Today', 'Schedule', 'Homework', 'Chats']);
    expect(within(bar).getByRole('link', { name: 'Homework' }).getAttribute('aria-label')).toBeNull();
    expect(linkLabels(sheet())).toEqual(['Students', 'Groups', 'Notifications', 'Settings']);
  });

  test('a page the sheet lists marks its sheet row current and no bar link', async () => {
    await renderFrame(account('student'), '/tutors', 'phone', 'en');

    const current = within(sheet()).getByRole('link', { name: 'My tutors', hidden: true });

    expect(current.getAttribute('aria-current')).toBe('page');
    expect(bottomBar().querySelector('[aria-current]')).toBeNull();
    expect(within(bottomBar()).getByRole('button', { name: 'More' }).hasAttribute('aria-current')).toBe(false);
  });

  test('a sign-out that fails while the sheet is closed shows an alert above the page and focuses More', async () => {
    const server = await renderFrame(account('student'), '/home', 'phone', 'en');
    server.failNextSignOut();
    const signOut = within(sheet()).getByRole('button', { name: 'Sign out', hidden: true });

    await click(signOut);

    const alert = await screen.findByRole('alert');
    expect(sheet().contains(alert)).toBe(false);
    expect(alert.textContent).toBe('Could not sign out. Check your connection and try again.');
    expect(document.activeElement).toBe(within(bottomBar()).getByRole('button', { name: 'More' }));
  });
});
