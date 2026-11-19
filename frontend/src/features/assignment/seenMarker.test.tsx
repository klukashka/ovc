import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { StrictMode } from 'react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { pastSimpleSubmission } from '../../lib/api/fakeSubmissions.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import type { components } from '../../lib/api/schema.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { AssignmentPage } from './AssignmentPage.tsx';
import { submissionKey } from './keys.ts';

afterEach(() => {
  cleanup();
});

type StudentSubmission = components['schemas']['StudentSubmission'];

const origin = 'https://ovc.example';
const submissionId = '4c3b2a19-0f8e-4d7c-9b6a-5f4e3d2c1b0a';
const tutor = { id: 'tutor-1', name: 'Maria', email: 'maria@example.org', password: 'a long password' };
const student = { id: 'student-1', name: 'Anna', email: 'anna@example.org', password: 'another long password' };

function checked(version: number): StudentSubmission {
  return {
    ...pastSimpleSubmission(submissionId),
    status: 'checked',
    version,
    editable_question_ids: [],
    review: { feedback: 'Well done.', items: [] },
  };
}

interface ShownResults {
  readonly server: FakeAuthServer;
  readonly queryClient: QueryClient;
}

async function showResultsOn(server: FakeAuthServer, submission: StudentSubmission): Promise<ShownResults> {
  server.addAccount(tutor);
  server.addAccount({ ...student, role: 'student' });
  server.addSubmission(student.email, tutor.email, submission);
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(student.email, student.password);
  const queryClient = new QueryClient();
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
    { initialEntries: [`/homework/${submissionId}`] },
  );
  const i18n = await createI18n('en');
  render(
    <StrictMode>
      <SchedulerContext value={manualScheduler()}>
        <SessionContext value={session}>
          <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
            <QueryClientProvider client={queryClient}>
              <I18nextProvider i18n={i18n}>
                <RouterProvider router={router} />
              </I18nextProvider>
            </QueryClientProvider>
          </ApiClientContext>
        </SessionContext>
      </SchedulerContext>
    </StrictMode>,
  );
  await screen.findByRole('heading', { level: 1, name: 'Past simple' });
  return { server, queryClient };
}

function showResults(submission: StudentSubmission): Promise<ShownResults> {
  return showResultsOn(new FakeAuthServer(), submission);
}

async function refetchSubmission(shown: ShownResults): Promise<void> {
  await act(async () => {
    await shown.queryClient.refetchQueries({ queryKey: submissionKey(submissionId) });
  });
}

describe('the seen marker on the assignment page', () => {
  test('a page showing a published review sends one feedback-seen with the shown version', async () => {
    const shown = await showResults(checked(4));

    await waitFor(() => {
      expect(shown.server.feedbackSeenRequests()).toHaveLength(1);
    });

    expect(shown.server.feedbackSeenRequests()).toEqual([{ id: submissionId, version: 4 }]);
  });

  test('a page without a published review sends none', async () => {
    const shown = await showResults(pastSimpleSubmission(submissionId));

    await refetchSubmission(shown);

    expect(shown.server.feedbackSeenRequests()).toEqual([]);
  });

  test('a refused marker shows nothing and is not retried on a refetch of the same version', async () => {
    const server = new FakeAuthServer();
    server.refuseNextFeedbackSeen(409, 'conflict');
    const shown = await showResultsOn(server, checked(4));
    await waitFor(() => {
      expect(shown.server.feedbackSeenRequests()).toHaveLength(1);
    });

    await refetchSubmission(shown);

    expect(shown.server.feedbackSeenRequests()).toHaveLength(1);
    expect(screen.queryByRole('alert')).toBeNull();
  });

  test('a refetch showing a newer version with a review sends the marker again', async () => {
    const shown = await showResults(checked(4));
    await waitFor(() => {
      expect(shown.server.feedbackSeenRequests()).toHaveLength(1);
    });
    shown.server.addSubmission(student.email, tutor.email, checked(6));

    await refetchSubmission(shown);
    await waitFor(() => {
      expect(shown.server.feedbackSeenRequests()).toHaveLength(2);
    });

    expect(shown.server.feedbackSeenRequests()).toEqual([
      { id: submissionId, version: 4 },
      { id: submissionId, version: 6 },
    ]);
  });
});
