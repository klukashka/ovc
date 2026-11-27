import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen } from '@testing-library/react';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import type { RouteHandle } from '../../app/handles.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { IssueAssignmentPage } from './IssueAssignmentPage.tsx';
import { IssuePage } from './IssuePage.tsx';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const email = 'maria@example.org';
const password = 'a long password';
const confirmEmailText = 'Confirm your email to issue and check assignments.';

async function showPage(path: string, address: string, title: RouteHandle['title'], page: ReactElement): Promise<void> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Maria', email, password, role: 'tutor', state: 'unverified' });
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(email, password);
  const i18n = await createI18n('en');
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createMemoryRouter([{ path, element: page, handle: { title } satisfies RouteHandle }], {
    initialEntries: [address],
  });
  render(
    <SessionContext value={session}>
      <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={i18n}>
            <main>
              <RouterProvider router={router} />
            </main>
          </I18nextProvider>
        </QueryClientProvider>
      </ApiClientContext>
    </SessionContext>,
  );
}

describe('the heading of the homework issue pages while their texts load', () => {
  test('the Issue page keeps the same heading element when the homework texts arrive', async () => {
    await showPage('/homework/issue', '/homework/issue', 'page.issueAssignment', <IssueAssignmentPage />);
    const headingBefore = screen.getByRole('heading', { level: 1 });

    await screen.findByText(confirmEmailText);

    expect(screen.getByRole('heading', { level: 1 })).toBe(headingBefore);
    expect(screen.getByRole('link', { name: 'Back to the library' })).toBeTruthy();
  });

  test('the issued assignment page keeps the same heading element when the homework texts arrive', async () => {
    await showPage(
      '/homework/issued/:issueId',
      '/homework/issued/00000000-0000-4000-8000-000000000031',
      'page.issue',
      <IssuePage />,
    );
    const headingBefore = screen.getByRole('heading', { level: 1 });

    await screen.findByText(confirmEmailText);

    expect(screen.getByRole('heading', { level: 1 })).toBe(headingBefore);
  });
});
