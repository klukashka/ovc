import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { MediaQueriesContext, PreferencesContext, fixedMediaQueries } from '../app/context.ts';
import { ApiClientContext, createApiClient } from '../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../lib/api/session.ts';
import { createI18n } from '../lib/i18n/i18n.ts';
import { memoryPreferenceStore } from '../lib/preferences/store.ts';
import { PlaceholderPage } from '../components/ui/Page.tsx';
import { Gate } from './Gate.tsx';
import { lazyPage } from './LazyPage.tsx';
import { routes } from './router.tsx';

const origin = 'https://ovc.example';

async function renderUnavailableStart(address: string): Promise<void> {
  const server = new FakeAuthServer();
  server.failNextRefresh();
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  const i18n = await createI18n('ru');
  await act(async () => {
    render(
      <MediaQueriesContext value={fixedMediaQueries([])}>
        <PreferencesContext value={memoryPreferenceStore()}>
          <SessionContext value={session}>
            <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
              <QueryClientProvider client={new QueryClient()}>
                <I18nextProvider i18n={i18n}>
                  <RouterProvider router={createMemoryRouter(routes, { initialEntries: [address] })} />
                </I18nextProvider>
              </QueryClientProvider>
            </ApiClientContext>
          </SessionContext>
        </PreferencesContext>
      </MediaQueriesContext>,
    );
    await session.start();
  });
}

async function pressRetry(): Promise<void> {
  const retry = screen.getByRole('button', { name: 'Попробовать ещё раз' });
  await act(async () => {
    retry.focus();
    retry.click();
    await Promise.resolve();
  });
}

async function pageLoadsAfterStart(failRefresh: boolean): Promise<{ loads: number; address: string }> {
  const server = new FakeAuthServer();
  if (failRefresh) {
    server.failNextRefresh();
  }
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  let loads = 0;
  const router = createMemoryRouter(
    [
      {
        element: <Gate audience="tutor" />,
        children: [
          {
            path: '/today',
            element: lazyPage(() => {
              loads += 1;
              return Promise.resolve(PlaceholderPage);
            }),
          },
        ],
      },
      { path: '*', element: <p>Elsewhere</p> },
    ],
    { initialEntries: ['/today'] },
  );
  const i18n = await createI18n('ru');
  await act(async () => {
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
    await Promise.resolve();
  });
  await act(async () => {
    await session.start();
  });
  return { loads, address: router.state.location.pathname };
}

afterEach(() => {
  cleanup();
});

describe('the start-up session check', () => {
  test('moves focus to the page heading once a retry reaches the server', async () => {
    await renderUnavailableStart('/');

    await pressRetry();

    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Вход' }));
    });
  });
});

describe('the access rule before lazily loaded pages', () => {
  test('a tutor page module is not loaded while the session is pending or unavailable', async () => {
    const unavailable = true;

    const outcome = await pageLoadsAfterStart(unavailable);

    expect(outcome.loads).toBe(0);
    expect(screen.getByRole('heading', { level: 1, name: 'Сервер недоступен' })).toBeDefined();
  });

  test('a tutor page module is not loaded for a signed-out visitor, who is sent to sign in', async () => {
    const unavailable = false;

    const outcome = await pageLoadsAfterStart(unavailable);

    expect(outcome.loads).toBe(0);
    expect(outcome.address).toBe('/');
  });
});
