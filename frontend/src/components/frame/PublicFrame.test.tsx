import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { PreferencesContext } from '../../app/context.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { type FakeAccountSeed, FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { memoryPreferenceStore } from '../../lib/preferences/store.ts';
import { PublicFrame } from './PublicFrame.tsx';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';

const firstAccount: FakeAccountSeed = {
  id: '00000000-0000-4000-8000-000000000071',
  name: 'Первая',
  email: 'first@example.org',
  password: 'a long password',
  role: 'student',
};

const secondAccount: FakeAccountSeed = {
  id: '00000000-0000-4000-8000-000000000072',
  name: 'Вторая',
  email: 'second@example.org',
  password: 'another long password',
  role: 'student',
};

interface ShownFrame {
  readonly server: FakeAuthServer;
  readonly session: Session;
}

async function showPublicFrame(): Promise<ShownFrame> {
  const server = new FakeAuthServer();
  server.addAccount(firstAccount);
  server.addAccount(secondAccount);
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(firstAccount.email, firstAccount.password);
  const i18n = await createI18n('ru');
  const router = createMemoryRouter([{ path: '/', element: <PublicFrame /> }], { initialEntries: ['/'] });
  render(
    <PreferencesContext value={memoryPreferenceStore()}>
      <SessionContext value={session}>
        <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
          <I18nextProvider i18n={i18n}>
            <RouterProvider router={router} />
          </I18nextProvider>
        </ApiClientContext>
      </SessionContext>
    </PreferencesContext>,
  );
  await screen.findByRole('button', { name: 'English' });
  return { server, session };
}

async function choose(name: string): Promise<void> {
  act(() => {
    screen.getByRole('button', { name }).click();
  });
  await waitFor(() => {
    expect(screen.getByRole('button', { name }).getAttribute('aria-pressed')).toBe('true');
  });
}

async function queueTwoChoicesWithTheFirstHeld(shown: ShownFrame): Promise<() => void> {
  const writes = shown.server.pauseAccountWrites();
  await choose('English');
  await writes.received;
  await choose('Русский');
  return writes.resume;
}

async function releaseAndAwaitTheAnswer(shown: ShownFrame, releaseFirst: () => void): Promise<void> {
  releaseFirst();
  await waitFor(() => {
    expect(shown.server.accountWritesAnswered).toBe(1);
  });
}

async function chooseAndAwaitTheWrite(shown: ShownFrame, name: string, writes: number): Promise<void> {
  await choose(name);
  await waitFor(() => {
    expect(shown.server.accountWrites).toHaveLength(writes);
  });
}

function signedInAccount(session: Session): string | null {
  const state = session.snapshot();
  return state.kind === 'signedIn' ? state.me.id : null;
}

describe('PublicFrame language saves', () => {
  test('a choice queued before another account is adopted is never saved for either account', async () => {
    const shown = await showPublicFrame();
    const releaseFirst = await queueTwoChoicesWithTheFirstHeld(shown);
    const secondToken: unknown = await (
      await shown.server.transport().login(secondAccount.email, secondAccount.password)
    ).json();

    await act(async () => {
      await shown.session.adopt(secondToken);
    });
    await releaseAndAwaitTheAnswer(shown, releaseFirst);
    await chooseAndAwaitTheWrite(shown, 'English', 2);

    expect(shown.server.accountWrites).toEqual([
      { accountId: firstAccount.id, language: 'en' },
      { accountId: secondAccount.id, language: 'en' },
    ]);
    expect(signedInAccount(shown.session)).toBe(secondAccount.id);
    expect(screen.queryByRole('alert')).toBeNull();
  });

  test('a choice queued before a sign-out is never saved, even after the next sign-in', async () => {
    const shown = await showPublicFrame();
    const releaseFirst = await queueTwoChoicesWithTheFirstHeld(shown);

    await act(async () => {
      await shown.session.signOut();
      await shown.session.signIn(secondAccount.email, secondAccount.password);
    });
    await releaseAndAwaitTheAnswer(shown, releaseFirst);
    await chooseAndAwaitTheWrite(shown, 'English', 2);

    expect(shown.server.accountWrites).toEqual([
      { accountId: firstAccount.id, language: 'en' },
      { accountId: secondAccount.id, language: 'en' },
    ]);
    expect(signedInAccount(shown.session)).toBe(secondAccount.id);
    expect(screen.queryByRole('alert')).toBeNull();
  });
});
