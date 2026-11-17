import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, type ManualScheduler, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { IssueItem } from '../homework/api.ts';
import { UpcomingDeadlinesPanel } from './TutorHomeworkPanels.tsx';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const password = 'a long password';
const tutorEmail = 'maria@example.org';
const hour = 3_600_000;

function issueDue(id: string, title: string, deadline: number): IssueItem {
  return {
    id,
    title,
    audience: { relationship_id: '00000000-0000-4000-8000-000000000031', display_name: 'Анна' },
    lesson_id: null,
    deadline_at: new Date(deadline).toISOString(),
    answer_policy: 'after_check',
    status: 'active',
    issued_at: '2026-10-30T10:00:00Z',
    withdrawn_at: null,
    has_manual_questions: true,
    max_points: 10,
    version: 1,
    progress: { students: 1, submitted: 0, checked: 0, returned: 0, late: 0, needs_checking: 0 },
  };
}

interface ShownDeadlines {
  readonly server: FakeAuthServer;
  readonly scheduler: ManualScheduler;
  readonly queryClient: QueryClient;
}

async function showDeadlines(clientAheadBy: number): Promise<ShownDeadlines> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Мария', email: tutorEmail, password, role: 'tutor' });
  const now = server.serverNow();
  server.addIssue(tutorEmail, issueDue('00000000-0000-4000-8000-000000000041', 'Эссе о поездке', now + 2 * hour));
  server.addIssue(tutorEmail, issueDue('00000000-0000-4000-8000-000000000042', 'Грамматика', now + 30 * hour));
  const scheduler = manualScheduler();
  scheduler.suspend(now + clientAheadBy);
  const session = new Session({ origin, transport: server.transport(), locks: memoryLocks(), scheduler });
  await session.signIn(tutorEmail, password);
  const client = createApiClient(origin, session.authorizedFetch);
  const i18n = await createI18n('ru');
  const queryClient = new QueryClient();
  const router = createMemoryRouter([{ path: '/', element: <UpcomingDeadlinesPanel /> }], { initialEntries: ['/'] });
  render(
    <SchedulerContext value={scheduler}>
      <SessionContext value={session}>
        <ApiClientContext value={client}>
          <QueryClientProvider client={queryClient}>
            <I18nextProvider i18n={i18n}>
              <RouterProvider router={router} />
            </I18nextProvider>
          </QueryClientProvider>
        </ApiClientContext>
      </SessionContext>
    </SchedulerContext>,
  );
  await screen.findByRole('link', { name: 'Эссе о поездке' });
  return { server, scheduler, queryClient };
}

async function passBoth(shown: ShownDeadlines, milliseconds: number): Promise<void> {
  shown.server.advance(milliseconds / 1_000);
  act(() => {
    shown.scheduler.pass(milliseconds);
  });
  await waitFor(() => {
    expect(shown.queryClient.isFetching()).toBe(0);
  });
}

describe('Upcoming deadlines', () => {
  test('drops an issue whose deadline passed without a reload and moves focus from it to the next row', async () => {
    const shown = await showDeadlines(0);
    screen.getByRole('link', { name: 'Эссе о поездке' }).focus();

    await passBoth(shown, 2 * hour + 1_000);

    const next = await screen.findByRole('link', { name: 'Грамматика' });
    expect(screen.queryByRole('link', { name: 'Эссе о поездке' })).toBeNull();
    expect(document.activeElement).toBe(next);
  });

  test('backs off instead of refetching in a loop while the client clock runs ahead of the server', async () => {
    const shown = await showDeadlines(10 * 60_000);
    const readsBefore = shown.server.deadlineReads();

    await passBoth(shown, 2 * hour - 10 * 60_000 + 1_000);
    for (let step = 0; step < 120; step += 1) {
      await passBoth(shown, 10_000);
    }

    expect(shown.server.deadlineReads() - readsBefore).toBeLessThanOrEqual(8);
    await screen.findByRole('link', { name: 'Грамматика' });
    expect(screen.queryByRole('link', { name: 'Эссе о поездке' })).toBeNull();
  });
});
