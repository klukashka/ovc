import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen } from '@testing-library/react';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { ResetPasswordPage } from './PasswordPages.tsx';
import { VerifyEmailPage } from './VerifyEmailPage.tsx';

const origin = 'https://ovc.example';

function unstartedSession(): Session {
  return new Session({
    origin,
    fetch: globalThis.fetch,
    locks: { request: (_name, callback) => callback() },
    scheduler: { schedule: () => (): void => undefined },
  });
}

function routerAt(element: ReactElement, address: string): ReturnType<typeof createMemoryRouter> {
  return createMemoryRouter([{ path: '*', element }], { initialEntries: [address] });
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

  test('the reset page without a token asks to open the link again', async () => {
    const router = routerAt(<ResetPasswordPage />, '/reset-password');

    await show(router);

    expect(screen.getByText('Откройте ссылку из письма ещё раз.')).toBeDefined();
  });
});
