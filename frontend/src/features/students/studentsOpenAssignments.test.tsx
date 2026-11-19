import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import type { RouteHandle } from '../../app/handles.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { StudentsPage } from './StudentsPage.tsx';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';

async function showStudents(): Promise<void> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Maria', email: 'maria@example.org', password: 'a long password' });
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn('maria@example.org', 'a long password');
  const client = createApiClient(origin, session.authorizedFetch);
  const created = await client.POST('/api/v1/students', { body: { display_name: 'Vera' } });
  await client.POST('/api/v1/students', { body: { display_name: 'Oleg' } });
  server.setOpenAssignments(created.data?.id ?? '', 3);
  const i18n = await createI18n('en');
  const router = createMemoryRouter(
    [{ path: '/students', element: <StudentsPage />, handle: { title: 'page.students' } satisfies RouteHandle }],
    { initialEntries: ['/students?state=invited'] },
  );
  render(
    <SessionContext value={session}>
      <ApiClientContext value={client}>
        <QueryClientProvider client={new QueryClient()}>
          <I18nextProvider i18n={i18n}>
            <main>
              <RouterProvider router={router} />
            </main>
          </I18nextProvider>
        </QueryClientProvider>
      </ApiClientContext>
    </SessionContext>,
  );
  await screen.findByRole('link', { name: 'Vera' });
}

function rowOf(name: string): HTMLElement {
  const row = screen.getByRole('link', { name }).closest('tr');
  if (row === null) {
    throw new Error(`no row for ${name}`);
  }
  return row;
}

describe('the open assignments column of the Students list', () => {
  test('shows each row’s open assignments as a plain number, 0 included, and under the name for phones', async () => {
    await showStudents();

    const header = screen.getByRole('columnheader', { name: 'Open assignments' });

    expect(header).toBeTruthy();
    expect(within(rowOf('Vera')).getAllByRole('cell').at(-1)?.textContent).toBe('3');
    expect(within(rowOf('Oleg')).getAllByRole('cell').at(-1)?.textContent).toBe('0');
    expect(within(rowOf('Vera')).getByText('Open assignments: 3')).toBeTruthy();
    expect(within(rowOf('Vera')).queryAllByRole('link')).toHaveLength(1);
  });
});
