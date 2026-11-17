import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { liveRefetch } from '../../app/liveRefetch.ts';
import { NotificationArrivals } from '../../app/notificationArrivals.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { pastSimpleSubmission } from '../../lib/api/fakeSubmissions.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { RealtimeEvent } from '../../lib/realtime/client.ts';
import { AssignmentPage } from './AssignmentPage.tsx';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const openSubmission = '4c3b2a19-0f8e-4d7c-9b6a-5f4e3d2c1b0a';
const otherSubmission = '5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a';
const tutor = { id: 'tutor-1', name: 'Maria', email: 'maria@example.org', password: 'a long password' };
const student = { id: 'student-1', name: 'Anna', email: 'anna@example.org', password: 'another long password' };

interface ShownAssignment {
  readonly server: FakeAuthServer;
  readonly deliver: (event: RealtimeEvent) => Promise<void>;
}

async function showAssignment(): Promise<ShownAssignment> {
  const server = new FakeAuthServer();
  server.addAccount(tutor);
  server.addAccount({ ...student, role: 'student' });
  server.addSubmission(student.email, tutor.email, pastSimpleSubmission(openSubmission));
  server.addSubmission(student.email, tutor.email, {
    ...pastSimpleSubmission(otherSubmission),
    title: 'Irregular verbs',
  });
  const scheduler = manualScheduler();
  const session = new Session({ origin, transport: server.transport(), locks: memoryLocks(), scheduler });
  await session.signIn(student.email, student.password);
  const queryClient = new QueryClient();
  const refetch = liveRefetch(queryClient, new NotificationArrivals(), scheduler);
  const router = createMemoryRouter(
    [
      {
        path: '/homework/:submissionId',
        element: (
          <main>
            <AssignmentPage />
          </main>
        ),
      },
    ],
    { initialEntries: [`/homework/${openSubmission}`] },
  );
  const i18n = await createI18n('en');
  render(
    <SchedulerContext value={scheduler}>
      <SessionContext value={session}>
        <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
          <QueryClientProvider client={queryClient}>
            <I18nextProvider i18n={i18n}>
              <RouterProvider router={router} />
            </I18nextProvider>
          </QueryClientProvider>
        </ApiClientContext>
      </SessionContext>
    </SchedulerContext>,
  );
  await screen.findByRole('heading', { level: 1, name: 'Past simple' });
  return {
    server,
    deliver: async (event): Promise<void> => {
      act(() => {
        refetch(event);
      });
      await waitFor(() => {
        expect(queryClient.isFetching()).toBe(0);
      });
    },
  };
}

describe('the assignment page under submission frames', () => {
  test('a frame for another submission sends no request for the open one', async () => {
    const shown = await showAssignment();
    const readsBefore = shown.server.submissionReads(openSubmission);

    await shown.deliver({ kind: 'entity', entity: 'submission', id: otherSubmission, lessonIds: ['lesson-1'] });

    expect(shown.server.submissionReads(openSubmission)).toBe(readsBefore);
    expect(shown.server.submissionReads(otherSubmission)).toBe(0);
  });

  test('a frame for the open submission refetches it once', async () => {
    const shown = await showAssignment();
    const readsBefore = shown.server.submissionReads(openSubmission);

    await shown.deliver({ kind: 'entity', entity: 'submission', id: openSubmission });

    expect(shown.server.submissionReads(openSubmission)).toBe(readsBefore + 1);
  });
});
