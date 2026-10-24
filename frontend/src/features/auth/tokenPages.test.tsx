import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { httpAuthTransport } from '../../lib/api/transport.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { MediaQueriesContext, PreferencesContext, fixedMediaQueries } from '../../app/context.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { memoryPreferenceStore } from '../../lib/preferences/store.ts';
import { routes } from '../../routes/router.tsx';
import { InvitationPage } from './InvitationPage.tsx';
import { InvitationTokenHolder } from './InvitationToken.tsx';
import { ResetPasswordPage } from './PasswordPages.tsx';
import { VerifyEmailPage } from './VerifyEmailPage.tsx';

const origin = 'https://ovc.example';

function unstartedSession(): Session {
  return new Session({
    origin,
    transport: httpAuthTransport('https://ovc.example', globalThis.fetch),
    locks: { request: (_name, callback) => callback() },
    scheduler: { schedule: () => (): void => undefined },
  });
}

function routerAt(element: ReactElement, address: string): ReturnType<typeof createMemoryRouter> {
  return createMemoryRouter([{ path: '*', element }], { initialEntries: [address] });
}

function invitationRouterAt(address: string): ReturnType<typeof createMemoryRouter> {
  return createMemoryRouter(
    [
      {
        path: '/invite/:token?',
        element: <InvitationTokenHolder />,
        children: [{ index: true, element: <InvitationPage /> }],
      },
    ],
    { initialEntries: [address] },
  );
}

async function show(router: ReturnType<typeof createMemoryRouter>): Promise<void> {
  const i18n = await createI18n('ru');
  await act(async () => {
    render(
      <SessionContext value={unstartedSession()}>
        <ApiClientContext value={createApiClient(origin, globalThis.fetch)}>
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
}

async function renderApp(session: Session, address: string): Promise<ReturnType<typeof createMemoryRouter>> {
  const router = createMemoryRouter(routes, { initialEntries: [address] });
  const i18n = await createI18n('ru');
  await act(async () => {
    render(
      <MediaQueriesContext value={fixedMediaQueries([])}>
        <PreferencesContext value={memoryPreferenceStore()}>
          <SessionContext value={session}>
            <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
              <QueryClientProvider client={new QueryClient()}>
                <I18nextProvider i18n={i18n}>
                  <RouterProvider router={router} />
                </I18nextProvider>
              </QueryClientProvider>
            </ApiClientContext>
          </SessionContext>
        </PreferencesContext>
      </MediaQueriesContext>,
    );
    await Promise.resolve();
  });
  return router;
}

afterEach(() => {
  cleanup();
});

describe('token pages', () => {
  test('the confirmation page removes the token from the address while the session is still loading', async () => {
    const router = routerAt(<VerifyEmailPage />, '/verify-email?token=secret-token-value');

    await show(router);

    const location = router.state.location;
    expect(`${location.pathname}${location.search}`).toBe('/verify-email');
    expect(screen.getByRole('status').textContent).toBe('Подтверждаем адрес');
  });

  test('the reset page removes the token from the address and keeps the form', async () => {
    const router = routerAt(<ResetPasswordPage />, '/reset-password?token=secret-token-value');

    await show(router);

    const location = router.state.location;
    expect(`${location.pathname}${location.search}`).toBe('/reset-password');
    expect(screen.getByLabelText('Новый пароль')).toBeDefined();
  });

  test('the invitation page removes the token from the address and keeps it out of the history state', async () => {
    const router = invitationRouterAt('/invite/secret-token-value');

    await show(router);

    const location = router.state.location;
    expect(`${location.pathname}${location.search}${location.hash}`).toBe('/invite');
    expect(JSON.stringify(location.state ?? null)).not.toContain('secret-token-value');
    expect(document.body.textContent).not.toContain('secret-token-value');
  });

  test('the invitation page without a token asks to open the link again', async () => {
    const router = invitationRouterAt('/invite');

    await show(router);

    expect(screen.getByText('Откройте ссылку от репетитора ещё раз.')).toBeDefined();
  });

  test('the invitation token leaves the address while the server is unavailable and is kept for the retry', async () => {
    const server = new FakeAuthServer();
    server.failNextRefresh();
    const session = new Session({
      origin,
      transport: server.transport(),
      locks: memoryLocks(),
      scheduler: manualScheduler(),
    });
    const router = await renderApp(session, '/invite/secret-token-value');
    const addressWhilePending = `${router.state.location.pathname}${router.state.location.search}`;
    await act(async () => {
      await session.start();
    });

    const unavailable = screen.getByRole('heading', { level: 1, name: 'Сервер недоступен' });
    await act(async () => {
      screen.getByRole('button', { name: 'Попробовать ещё раз' }).click();
      await Promise.resolve();
    });

    expect(addressWhilePending).toBe('/invite');
    expect(unavailable).toBeDefined();
    expect(await screen.findByRole('alert')).toHaveProperty(
      'textContent',
      'Это приглашение больше не действует — попросите у репетитора новую ссылку.',
    );
    expect(router.state.location.pathname).toBe('/invite');
  });

  test('after a successful retry of a failed invitation load the page heading takes focus', async () => {
    const server = new FakeAuthServer();
    server.addAccount({ id: 'tutor-1', name: 'Мария', email: 'maria@example.org', password: 'a long password' });
    const login: unknown = await (await server.transport().login('maria@example.org', 'a long password')).json();
    const access =
      typeof login === 'object' && login !== null && 'access_token' in login ? String(login.access_token) : '';
    const created: unknown = await (
      await server.transport().send(
        new Request(`${origin}/api/v1/students`, {
          method: 'POST',
          headers: { Authorization: `Bearer ${access}`, 'Content-Type': 'application/json' },
          body: JSON.stringify({ display_name: 'Анна' }),
        }),
      )
    ).json();
    const link =
      typeof created === 'object' && created !== null && 'invitation' in created
        ? JSON.stringify(created.invitation)
        : '';
    const token = /invite\/([^"]+)"/.exec(link)?.[1] ?? '';
    server.failNextInvitationRead();
    const session = new Session({
      origin,
      transport: server.transport(),
      locks: memoryLocks(),
      scheduler: manualScheduler(),
    });
    await renderApp(session, `/invite/${token}`);
    await act(async () => {
      await session.start();
    });
    const retry = await screen.findByRole('button', { name: 'Повторить' });

    await act(async () => {
      retry.click();
      await Promise.resolve();
    });

    await screen.findByRole('heading', { level: 2, name: 'Мария' });
    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Приглашение' }));
    });
  });

  test('the reset page without a token asks to open the link again', async () => {
    const router = routerAt(<ResetPasswordPage />, '/reset-password');

    await show(router);

    expect(screen.getByText('Откройте ссылку из письма ещё раз.')).toBeDefined();
  });
});
