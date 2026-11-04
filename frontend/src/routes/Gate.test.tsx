import { ConnectionContext, ConnectionState } from '../app/connectionState.ts';
import { FollowingQueryClient, accountCacheOwnership } from '../app/queryClient.ts';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { MediaQueriesContext, PreferencesContext, fixedMediaQueries } from '../app/context.ts';
import { ApiClientContext, createApiClient } from '../lib/api/client.ts';
import { FakeAuthServer, SignalAwareRefreshBodies, manualScheduler, memoryLocks } from '../lib/api/fakeTransport.ts';
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

test.each([
  { outcome: 'release', operation: 'refresh' },
  { outcome: 'fail', operation: 'refresh' },
  { outcome: 'fail', operation: 'start' },
] as const)(
  'real session/cache/Gate ownership survives $operation with body $outcome after account replacement',
  async ({ outcome, operation }) => {
    const body = new SignalAwareRefreshBodies({ hold: true });
    const server = new FakeAuthServer({ refreshBodies: body });
    const first = {
      id: 'lifecycle-first',
      name: 'First owner',
      email: 'first@example.org',
      password: 'first lifecycle password',
    };
    const second = {
      id: 'lifecycle-second',
      name: 'Second owner',
      email: 'second@example.org',
      password: 'second lifecycle password',
    };
    server.addAccount(first);
    server.addAccount(second);
    const clock = manualScheduler();
    const session = new Session({ origin, transport: server.transport(), locks: memoryLocks(), scheduler: clock });
    const cache = new FollowingQueryClient({ defaultOptions: { queries: { retry: false } } });
    const ownCache = accountCacheOwnership(cache);
    const followOwner = (): void => {
      const state = session.snapshot();
      ownCache(state.kind === 'signedIn' ? state.me : null);
    };
    const stopOwner = session.subscribe(followOwner);
    await session.signIn(first.email, first.password);
    const connection = new ConnectionState(clock);
    const stopConnection = connection.follow(session);
    const router = createMemoryRouter(
      [
        { element: <Gate audience="tutor" />, children: [{ path: '/today', element: <h1>Private owner view</h1> }] },
        { path: '/', element: <h1>Signed out view</h1> },
      ],
      { initialEntries: ['/today'] },
    );
    const i18n = await createI18n('en');
    render(
      <MediaQueriesContext value={fixedMediaQueries([])}>
        <PreferencesContext value={memoryPreferenceStore()}>
          <ConnectionContext value={connection}>
            <SessionContext value={session}>
              <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
                <QueryClientProvider client={cache}>
                  <I18nextProvider i18n={i18n}>
                    <RouterProvider router={router} />
                  </I18nextProvider>
                </QueryClientProvider>
              </ApiClientContext>
            </SessionContext>
          </ConnectionContext>
        </PreferencesContext>
      </MediaQueriesContext>,
    );
    const ownedOperations: Promise<unknown>[] = [];
    try {
      await screen.findByRole('heading', { name: 'Private owner view' });
      await waitFor(() => {
        expect(cache.isFetching()).toBe(0);
      });
      cache.setQueryData(['private-owner'], { owner: first.id });
      server.revokeSessionsOf(first.email);

      let firstResult: Promise<'resolved' | 'rejected'> = Promise.resolve('resolved');
      await act(async () => {
        firstResult = (operation === 'start' ? session.start() : session.refresh()).then(
          () => 'resolved' as const,
          () => 'rejected' as const,
        );
        ownedOperations.push(firstResult);
        await body.headers;
        await body.reading;
      });
      await screen.findByRole('heading', { name: 'Signed out view' });
      const cacheWhileHeld = cache.getQueryData(['private-owner']);
      const stateWhileHeld = session.snapshot();
      const queued = session.refresh();
      ownedOperations.push(queued);
      const login = await server.transport().login(second.email, second.password);
      const token: unknown = await login.json();
      await act(async () => {
        await session.adopt(token);
        await router.navigate('/today');
      });
      await screen.findByRole('heading', { name: 'Private owner view' });
      cache.setQueryData(['private-owner'], { owner: second.id });
      await act(async () => {
        if (outcome === 'release') body.release();
        else body.fail();
        await firstResult;
        await queued;
      });
      const result = await firstResult;
      const next = await session.authorizedFetch(`${origin}/api/v1/users/me`);

      expect(cacheWhileHeld).toBeUndefined();
      expect(stateWhileHeld.kind).toBe('signedOut');
      expect(cache.getQueryData(['private-owner'])).toEqual({ owner: second.id });
      expect(router.state.location.pathname).toBe('/today');
      expect(await next.json()).toMatchObject({ id: second.id });
      expect((await queued).kind).toBe('superseded');
      expect(result).toBe(outcome === 'release' || operation === 'start' ? 'resolved' : 'rejected');
      expect(server.refreshes).toBe(1);
      expect(body.snapshot()).toMatchObject({
        active: 0,
        readerLocked: false,
        readFailures: outcome === 'fail' ? 1 : 0,
      });
    } finally {
      body.release();
      try {
        await Promise.all(ownedOperations);
      } finally {
        stopOwner();
        stopConnection();
        cache.clear();
        router.dispose();
      }
    }
  },
);
