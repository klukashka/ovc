import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { AddressTokenHolder } from '../auth/AddressToken.tsx';
import { NotificationSettingsPage } from './NotificationSettingsPage.tsx';
import { UnsubscribePage } from './UnsubscribePage.tsx';

const origin = 'https://ovc.example';
const email = 'student@example.org';
const password = 'a long password';

function fakeServer(): FakeAuthServer {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'student-1', name: 'A student', email, password, role: 'student' });
  return server;
}

function sessionOn(server: FakeAuthServer): Session {
  return new Session({ origin, transport: server.transport(), locks: memoryLocks(), scheduler: manualScheduler() });
}

async function show(session: Session, element: ReactElement, address: string): Promise<QueryClient> {
  const i18n = await createI18n('ru');
  const router = createMemoryRouter([{ element: <AddressTokenHolder />, children: [{ path: '*', element }] }], {
    initialEntries: [address],
  });
  const queryClient = new QueryClient();
  await act(async () => {
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
    await Promise.resolve();
  });
  return queryClient;
}

async function press(button: HTMLElement): Promise<void> {
  await act(async () => {
    button.focus();
    button.click();
    await Promise.resolve();
  });
}

afterEach(() => {
  cleanup();
});

describe('the unsubscribe page', () => {
  test('confirming replaces the button with the result, which takes focus, and keeps the token out of the query cache', async () => {
    const server = fakeServer();
    const token = server.unsubscribeToken(email, 'message_digest');
    const queryClient = await show(sessionOn(server), <UnsubscribePage />, `/unsubscribe?token=${token}`);
    const button = await screen.findByRole('button', { name: 'Отключить эти письма' });

    await press(button);

    const result = await screen.findByText('Эти письма отключены');
    await waitFor(() => {
      expect(document.activeElement).toBe(result);
    });
    expect(screen.queryByRole('button', { name: 'Отключить эти письма' })).toBeNull();
    expect(server.emailGroupEnabled(email, 'message_digest')).toBe(false);
    expect(
      JSON.stringify(
        queryClient
          .getQueryCache()
          .getAll()
          .map((query) => [query.queryKey, query.state.data]),
      ),
    ).not.toContain(token);
  });

  test('a refused confirmation shows its error, which takes focus, and keeps the button', async () => {
    const server = fakeServer();
    const token = server.unsubscribeToken(email, 'homework');
    await show(sessionOn(server), <UnsubscribePage />, `/unsubscribe?token=${token}`);
    const button = await screen.findByRole('button', { name: 'Отключить эти письма' });
    server.failNextUnsubscribe();

    await press(button);

    const alert = await screen.findByRole('alert');
    await waitFor(() => {
      expect(document.activeElement).toBe(alert);
    });
    expect(button.getAttribute('aria-busy')).toBe('false');
    expect(button.isConnected).toBe(true);
    expect(server.emailGroupEnabled(email, 'homework')).toBe(true);
  });

  test('a link that became invalid before confirming shows the one invalid-link message, which takes focus', async () => {
    const server = fakeServer();
    const token = server.unsubscribeToken(email, 'results');
    await show(sessionOn(server), <UnsubscribePage />, `/unsubscribe?token=${token}`);
    const button = await screen.findByRole('button', { name: 'Отключить эти письма' });
    server.block(email);

    await press(button);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe('Ссылка больше не действует');
    await waitFor(() => {
      expect(document.activeElement).toBe(alert);
    });
    expect(screen.queryByRole('button', { name: 'Отключить эти письма' })).toBeNull();
    expect(screen.getByRole('heading', { level: 1 })).toBeDefined();
  });
});

describe('the email settings', () => {
  test('a student sees five groups, and a failed save puts the switch back, keeps focus on it and offers a retry', async () => {
    const server = fakeServer();
    const session = sessionOn(server);
    await session.signIn(email, password);
    server.failNextPreferenceSave();
    await show(session, <NotificationSettingsPage />, '/settings/notifications');
    const results = await screen.findByRole('switch', { name: 'Проверенные работы' });

    await press(results);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toContain('Настройка «Проверенные работы» не сохранена.');
    expect(screen.getByRole('button', { name: 'Повторить' })).toBeDefined();
    expect(results.getAttribute('aria-checked')).toBe('true');
    expect(results.getAttribute('aria-busy')).toBe('false');
    expect(document.activeElement).toBe(results);
    expect(server.emailGroupEnabled(email, 'results')).toBe(true);
    expect(
      screen
        .getAllByRole('switch')
        .map((element) => document.getElementById(element.getAttribute('aria-labelledby') ?? '')?.textContent),
    ).toEqual([
      'Напоминания об уроках',
      'Изменения в расписании',
      'Новые задания',
      'Проверенные работы',
      'Непрочитанные сообщения',
    ]);
  });

  test('retrying a failed save moves focus back to its switch and saves it', async () => {
    const server = fakeServer();
    const session = sessionOn(server);
    await session.signIn(email, password);
    server.failNextPreferenceSave();
    await show(session, <NotificationSettingsPage />, '/settings/notifications');
    const homework = await screen.findByRole('switch', { name: 'Новые задания' });
    await press(homework);
    const retry = await screen.findByRole('button', { name: 'Повторить' });

    await press(retry);

    await waitFor(() => {
      expect(homework.getAttribute('aria-checked')).toBe('false');
    });
    expect(homework.getAttribute('aria-busy')).toBe('false');
    expect(document.activeElement).toBe(homework);
    expect(screen.queryByRole('alert')).toBeNull();
    expect(server.emailGroupEnabled(email, 'homework')).toBe(false);
  });
});
