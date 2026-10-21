import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { type Me, Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { ProfileSettingsPage } from './ProfileSettingsPage.tsx';

const origin = 'https://ovc.example';

async function signedInAs(role: Me['role']): Promise<Session> {
  const server = new FakeAuthServer();
  server.addAccount({
    id: 'account-1',
    name: 'A person',
    email: 'person@example.org',
    password: 'a long password',
    role,
  });
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn('person@example.org', 'a long password');
  return session;
}

async function show(session: Session): Promise<void> {
  const i18n = await createI18n('ru');
  const router = createMemoryRouter([{ path: '*', element: <ProfileSettingsPage /> }], {
    initialEntries: ['/settings/profile'],
  });
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
}

afterEach(() => {
  cleanup();
});

describe('the profile settings', () => {
  test('a tutor edits the name, the bio and the subjects', async () => {
    const session = await signedInAs('tutor');

    await show(session);

    expect(screen.getByLabelText('Имя')).toBeDefined();
    expect(screen.getByLabelText('О себе')).toBeDefined();
    expect(screen.getByLabelText('Предметы')).toBeDefined();
  });

  test('a student edits only the name', async () => {
    const session = await signedInAs('student');

    await show(session);

    expect(screen.getByLabelText('Имя')).toBeDefined();
    expect(screen.queryByLabelText('О себе')).toBeNull();
    expect(screen.queryByLabelText('Предметы')).toBeNull();
  });
});
