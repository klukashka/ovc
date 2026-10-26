import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { NotificationsBell } from './Bell.tsx';

const origin = 'https://ovc.example';
const tutor = { id: 'tutor-1', name: 'Мария', email: 'maria@example.org', password: 'a long password' };

async function loadThePanelModuleTheBellLoadsOnDemand(): Promise<void> {
  await import('./NotificationsPanel.tsx');
}

async function showBellWithFailingReads(): Promise<FakeAuthServer> {
  await loadThePanelModuleTheBellLoadsOnDemand();
  const server = new FakeAuthServer();
  server.addAccount(tutor);
  server.failNotificationReads(true);
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
        path: '/today',
        element: (
          <>
            <nav>
              <NotificationsBell placement="sidebar" />
            </nav>
            <main>
              <h1 tabIndex={-1}>Сегодня</h1>
            </main>
          </>
        ),
      },
    ],
    { initialEntries: ['/today'] },
  );
  const i18n = await createI18n('ru');
  render(
    <SessionContext value={session}>
      <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
        <QueryClientProvider client={new QueryClient()}>
          <I18nextProvider i18n={i18n}>
            <RouterProvider router={router} />
          </I18nextProvider>
        </QueryClientProvider>
      </ApiClientContext>
    </SessionContext>,
  );
  return server;
}

function bell(): HTMLElement {
  return within(screen.getByRole('navigation')).getByRole('button', { name: /^Уведомления/ });
}

function panelHeading(): HTMLElement {
  return screen.getByRole('heading', { level: 2, name: 'Уведомления' });
}

async function openFailedPanel(): Promise<HTMLElement> {
  await act(async () => {
    bell().click();
    await Promise.resolve();
  });
  const retry = await screen.findByRole('button', { name: 'Повторить' });
  retry.focus();
  return retry;
}

async function retryUntilLoaded(server: FakeAuthServer, retry: HTMLElement): Promise<void> {
  server.failNotificationReads(false);
  await act(async () => {
    retry.click();
    await Promise.resolve();
  });
  await screen.findByText('Уведомлений пока нет');
}

afterEach(() => {
  cleanup();
});

describe('the notifications panel', () => {
  test('Retry after a failed load moves focus to the panel heading once the failure is gone', async () => {
    const server = await showBellWithFailingReads();
    const retry = await openFailedPanel();

    await retryUntilLoaded(server, retry);

    expect(document.activeElement).toBe(panelHeading());
  });

  test('Escape after a successful Retry closes the panel onto the bell', async () => {
    const server = await showBellWithFailingReads();
    await retryUntilLoaded(server, await openFailedPanel());

    fireEvent.keyDown(document.activeElement ?? document.body, { key: 'Escape' });

    expect(screen.queryByRole('region', { name: 'Уведомления' })).toBeNull();
    expect(document.activeElement).toBe(bell());
  });

  test('Escape closes the panel onto the bell while focus is on no element', async () => {
    await showBellWithFailingReads();
    const retry = await openFailedPanel();
    retry.blur();

    fireEvent.keyDown(document.body, { key: 'Escape' });

    expect(screen.queryByRole('region', { name: 'Уведомления' })).toBeNull();
    expect(document.activeElement).toBe(bell());
  });
});
