import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { type Notification, notificationPagesKey } from './api.ts';
import { NotificationsPage } from './NotificationsPage.tsx';
import { notificationRowSelector } from './NotificationRows.tsx';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const tutor = { id: 'tutor-1', name: 'Мария', email: 'maria@example.org', password: 'a long password' };
const refetchFailedText = 'Не удалось обновить данные. На странице показано то, что загрузилось раньше.';
const loadFailedText = 'Что-то пошло не так.';
const firstPageRows = 30;

function notificationAt(index: number): Notification {
  return {
    id: `00000000-0000-4000-8000-${String(index + 1).padStart(12, '0')}`,
    event: 'join_request.created',
    created_at: new Date(Date.UTC(2026, 10, 1, 12, 0) - index * 60_000).toISOString(),
    read_at: null,
    label: `Ученик ${String(index + 1)}`,
    label_deleted: false,
    label_kind: null,
    colour: null,
    link: null,
  };
}

interface ShownPage {
  readonly server: FakeAuthServer;
  readonly queryClient: QueryClient;
}

async function showNotifications(rows: number): Promise<ShownPage> {
  const server = new FakeAuthServer();
  server.addAccount(tutor);
  for (let index = 0; index < rows; index += 1) {
    server.addNotification(tutor.email, notificationAt(index));
  }
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(tutor.email, tutor.password);
  const router = createMemoryRouter(
    [
      {
        path: '/notifications',
        element: (
          <main>
            <NotificationsPage />
          </main>
        ),
      },
    ],
    { initialEntries: ['/notifications'] },
  );
  const i18n = await createI18n('ru');
  const queryClient = new QueryClient();
  render(
    <SessionContext value={session}>
      <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={i18n}>
            <RouterProvider router={router} />
          </I18nextProvider>
        </QueryClientProvider>
      </ApiClientContext>
    </SessionContext>,
  );
  await screen.findAllByText('Ученик 1');
  return { server, queryClient };
}

function shownRows(): HTMLElement[] {
  const body = screen.getByRole('table').querySelector('tbody');
  return body === null ? [] : [...body.querySelectorAll<HTMLElement>(notificationRowSelector)];
}

function markAllRead(): HTMLElement {
  return screen.getByRole('button', { name: 'Отметить все как прочитанные' });
}

function showMore(): HTMLElement {
  return screen.getByRole('button', { name: 'Показать ещё' });
}

async function pressShowMoreFailing(shown: ShownPage): Promise<HTMLElement> {
  const button = showMore();
  button.focus();
  shown.server.failNotificationReads(true);
  fireEvent.click(button);
  await screen.findByText(refetchFailedText);
  return button;
}

describe('the notifications page after a failed read', () => {
  test('a failed refetch keeps the rows and Mark all as read, keeps focus and shows the refetch notice', async () => {
    const shown = await showNotifications(firstPageRows + 1);
    markAllRead().focus();
    shown.server.failNotificationReads(true);

    await act(async () => {
      await shown.queryClient.refetchQueries({ queryKey: notificationPagesKey });
    });
    await screen.findByText(refetchFailedText);

    expect(shownRows()).toHaveLength(firstPageRows);
    expect(document.activeElement).toBe(markAllRead());
    expect(showMore()).toBeTruthy();
    expect(screen.queryByText(loadFailedText)).toBeNull();
  });

  test('a failed Show more keeps the rows, Mark all as read and the focused button, with the notice', async () => {
    const shown = await showNotifications(firstPageRows + 1);

    const button = await pressShowMoreFailing(shown);

    expect(shownRows()).toHaveLength(firstPageRows);
    expect(markAllRead()).toBeTruthy();
    expect(showMore()).toBe(button);
    expect(document.activeElement).toBe(button);
    expect(screen.queryByText(loadFailedText)).toBeNull();
  });

  test('Retry after a failed Show more loads the next rows and moves focus to the first new row', async () => {
    const shown = await showNotifications(firstPageRows + 1);
    await pressShowMoreFailing(shown);
    shown.server.failNotificationReads(false);

    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }));
    await screen.findAllByText(`Ученик ${String(firstPageRows + 1)}`);

    expect(shownRows()).toHaveLength(firstPageRows + 1);
    expect(document.activeElement).toBe(shownRows()[firstPageRows]);
    expect(screen.queryByText(refetchFailedText)).toBeNull();
  });

  test('a later successful refetch that removes the notice holding focus moves focus to the page heading', async () => {
    const shown = await showNotifications(firstPageRows + 1);
    shown.server.failNotificationReads(true);
    await act(async () => {
      await shown.queryClient.refetchQueries({ queryKey: notificationPagesKey });
    });
    await screen.findByText(refetchFailedText);
    screen.getByRole('button', { name: 'Повторить' }).focus();
    shown.server.failNotificationReads(false);

    await act(async () => {
      await shown.queryClient.refetchQueries({ queryKey: notificationPagesKey });
    });
    await waitFor(() => {
      expect(screen.queryByText(refetchFailedText)).toBeNull();
    });

    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });
});
