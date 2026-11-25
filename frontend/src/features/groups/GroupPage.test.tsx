import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { GroupPage } from './GroupPage.tsx';
import { groupsTexts } from './texts.en.ts';

const origin = 'https://ovc.example';
const tutorEmail = 'pavel@example.org';
const password = 'a long password';

interface ShownGroup {
  readonly server: FakeAuthServer;
  readonly groupId: string;
}

async function showGroup(seed: { readonly members: readonly string[] }): Promise<ShownGroup> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Pavel', email: tutorEmail, password });
  const { groupId } = server.seedGroup(tutorEmail, { name: 'Evening algebra', members: seed.members });
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(tutorEmail, password);
  const i18n = await createI18n('en');
  i18n.addResourceBundle('en', 'translation', { groups: groupsTexts }, true, true);
  const router = createMemoryRouter([{ path: '/groups/:groupId', element: <GroupPage /> }], {
    initialEntries: [`/groups/${groupId}`],
  });
  await act(async () => {
    render(
      <SessionContext value={session}>
        <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
          <QueryClientProvider client={new QueryClient()}>
            <SchedulerContext value={manualScheduler()}>
              <I18nextProvider i18n={i18n}>
                <RouterProvider router={router} />
              </I18nextProvider>
            </SchedulerContext>
          </QueryClientProvider>
        </ApiClientContext>
      </SessionContext>,
    );
    await Promise.resolve();
  });
  await screen.findByRole('heading', { level: 1, name: 'Evening algebra' });
  return { server, groupId };
}

async function removeBoris(): Promise<void> {
  fireEvent.click(await screen.findByRole('button', { name: 'Remove from the group: Boris' }));
}

afterEach(() => {
  cleanup();
});

describe('the reversal of a group change whose answer is unknown', () => {
  test('a removal whose answer is lost offers no Add back', async () => {
    const { server, groupId } = await showGroup({ members: ['Anna', 'Boris'] });
    server.loseNextGroupWrite();

    await removeBoris();

    await waitFor(() => {
      expect(screen.queryByRole('link', { name: 'Boris' })).toBeNull();
    });
    expect(screen.queryByRole('button', { name: 'Add back' })).toBeNull();
    expect(server.groupMemberIds(groupId)).toHaveLength(1);
  });

  test('a removal refused with 503 offers no Add back', async () => {
    const { server, groupId } = await showGroup({ members: ['Anna', 'Boris'] });
    server.failNextGroupWrite();

    await removeBoris();

    await screen.findByRole('alert');
    expect(screen.queryByRole('button', { name: 'Add back' })).toBeNull();
    expect(screen.getByRole('link', { name: 'Boris' })).toBeTruthy();
    expect(server.groupMemberIds(groupId)).toHaveLength(2);
  });

  test('an archive whose answer is lost offers no Undo', async () => {
    const { server } = await showGroup({ members: ['Anna'] });
    server.loseNextGroupWrite();

    fireEvent.click(screen.getByRole('button', { name: 'Group actions' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Archive group' }));

    await screen.findByText('Archived', { exact: true });
    expect(screen.queryByRole('button', { name: 'Undo' })).toBeNull();
  });

  test('an Add back whose answer is lost confirms after the refetch', async () => {
    const { server, groupId } = await showGroup({ members: ['Anna', 'Boris'] });
    await removeBoris();
    const addBack = await screen.findByRole('button', { name: 'Add back' });
    server.loseNextGroupWrite();

    fireEvent.click(addBack);

    await screen.findByText(/^Back in the group:/);
    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull();
    expect(server.groupMemberIds(groupId)).toHaveLength(2);
  });
});
