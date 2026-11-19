import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import type { RouteHandle } from '../../app/handles.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { StudentsPage } from './StudentsPage.tsx';

const origin = 'https://ovc.example';

async function showFailingJoinRequests(address: string): Promise<FakeAuthServer> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Maria', email: 'maria@example.org', password: 'a long password' });
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  server.failJoinRequestReads(2);
  await session.signIn('maria@example.org', 'a long password');
  const i18n = await createI18n('en');
  const router = createMemoryRouter(
    [{ path: '/students', element: <StudentsPage />, handle: { title: 'page.students' } satisfies RouteHandle }],
    { initialEntries: [address] },
  );
  const page: ReactElement = (
    <SessionContext value={session}>
      <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
        <QueryClientProvider client={new QueryClient()}>
          <I18nextProvider i18n={i18n}>
            <main>
              <RouterProvider router={router} />
            </main>
          </I18nextProvider>
        </QueryClientProvider>
      </ApiClientContext>
    </SessionContext>
  );
  render(page);
  await waitFor(() => {
    expect(screen.getByRole('region', { name: 'Join requests' }).querySelector('[role="alert"]')).not.toBeNull();
  });
  return server;
}

function sectionHiddenWhileWaiting(): void {
  expect(screen.queryByRole('region', { name: 'Join requests' })).toBeNull();
}

async function retryJoinRequestsWhileItWaits(server: FakeAuthServer, waiting: () => void): Promise<() => void> {
  const paused = server.pauseJoinRequestReads();
  const section = screen.getByRole('region', { name: 'Join requests' });
  const [retry] = [...section.querySelectorAll('button')].filter((button) => button.textContent === 'Try again');
  await act(async () => {
    retry?.click();
    await paused.answered;
  });
  await waitFor(waiting);
  return paused.resume;
}

afterEach(() => {
  cleanup();
});

describe('content shown once the join requests have settled', () => {
  test('stays shown and keeps focus on the Students page while a failed request list is retried', async () => {
    const server = await showFailingJoinRequests('/students');
    const field = screen.getByLabelText('Name or email');
    field.focus();

    const resume = await retryJoinRequestsWhileItWaits(server, sectionHiddenWhileWaiting);

    expect(field.closest('[hidden]')).toBeNull();
    expect(document.activeElement).toBe(field);
    resume();
  });
});
