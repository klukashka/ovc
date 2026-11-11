import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { PreferencesContext } from '../../app/context.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, type PausedAnswers, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { type Me, Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { memoryPreferenceStore } from '../../lib/preferences/store.ts';
import { PublicFrame } from './PublicFrame.tsx';
import { Sidebar } from './Sidebar.tsx';

const origin = 'https://ovc.example';
const seed = {
  id: '00000000-0000-4000-8000-000000000001',
  name: 'Мария',
  email: 'maria@example.org',
  password: 'a long password',
  role: 'tutor',
} as const;

const pausedReads: PausedAnswers[] = [];

afterEach(() => {
  for (const paused of pausedReads.splice(0)) {
    paused.resume();
  }
  cleanup();
});

async function signedInWithReadInFlight(): Promise<{
  readonly session: Session;
  readonly me: Me;
  readonly server: FakeAuthServer;
  readonly paused: PausedAnswers;
}> {
  const server = new FakeAuthServer();
  server.addAccount(seed);
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(seed.email, seed.password);
  const state = session.snapshot();
  if (state.kind !== 'signedIn') {
    throw new Error('the sign-out test account did not sign in');
  }
  const paused = server.pauseAccountReads();
  pausedReads.push(paused);
  void session.authorizedFetch(`${origin}/api/v1/users/me`).catch(() => undefined);
  await paused.answered;
  return { session, me: state.me, server, paused };
}

async function renderWithin(session: Session, content: ReactElement): Promise<void> {
  const i18n = await createI18n('ru');
  render(
    <PreferencesContext value={memoryPreferenceStore()}>
      <SessionContext value={session}>
        <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
          <QueryClientProvider client={new QueryClient()}>
            <I18nextProvider i18n={i18n}>
              <MemoryRouter>{content}</MemoryRouter>
            </I18nextProvider>
          </QueryClientProvider>
        </ApiClientContext>
      </SessionContext>
    </PreferencesContext>,
  );
}

function expectBusySignOut(): void {
  const button = screen.getByRole('button', { name: 'Выход…' });
  expect(button.getAttribute('aria-busy')).toBe('true');
  expect(button.getAttribute('aria-disabled')).toBe('true');
  expect(button.hasAttribute('disabled')).toBe(false);
}

describe('signing out while a read is in flight', () => {
  test('the public frame shows its sign-out button busy', async () => {
    const { session } = await signedInWithReadInFlight();
    await renderWithin(session, <PublicFrame>{null}</PublicFrame>);

    act(() => {
      screen.getByRole('button', { name: 'Выйти' }).click();
    });

    expectBusySignOut();
  });

  test('the account menu shows its sign-out button busy', async () => {
    const { session, me } = await signedInWithReadInFlight();
    await renderWithin(
      session,
      <Sidebar me={me} counts={{}} bell={null} headerAction={null} onNavigate={() => undefined} />,
    );
    act(() => {
      screen.getByRole('button', { name: 'Мария, Меню аккаунта' }).click();
    });

    act(() => {
      screen.getByRole('button', { name: 'Выйти' }).click();
    });

    expectBusySignOut();
  });

  test('a second press while signing out sends no second sign-out', async () => {
    const { session, server, paused } = await signedInWithReadInFlight();
    await renderWithin(session, <PublicFrame>{null}</PublicFrame>);

    act(() => {
      screen.getByRole('button', { name: 'Выйти' }).click();
    });
    act(() => {
      screen.getByRole('button', { name: 'Выход…' }).click();
    });
    paused.resume();
    await waitFor(() => {
      expect(session.snapshot().kind).toBe('signedOut');
    });

    expect(server.signOuts).toBe(1);
  });
});
