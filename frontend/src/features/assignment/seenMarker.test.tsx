import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { StrictMode } from 'react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { pastSimpleQuestions, pastSimpleSubmission } from '../../lib/api/fakeSubmissions.ts';
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
type StudentReview = components['schemas']['StudentReview'];

const origin = 'https://ovc.example';
const submissionId = '4c3b2a19-0f8e-4d7c-9b6a-5f4e3d2c1b0a';
const tutor = { id: 'tutor-1', name: 'Maria', email: 'maria@example.org', password: 'a long password' };
const student = { id: 'student-1', name: 'Anna', email: 'anna@example.org', password: 'another long password' };

function checked(version: number, feedback = 'Well done.'): StudentSubmission {
  return {
    ...pastSimpleSubmission(submissionId),
    status: 'checked',
    version,
    editable_question_ids: [],
    review: { feedback, items: [] },
  };
}

const firstAttachment = '6d5c4b3a-2f1e-4d0c-9b8a-7f6e5d4c3b2a';
const secondAttachment = '7e6d5c4b-3a2f-4e1d-8c9b-0a7f6e5d4c3b';

function correctedFile(address: string, attachmentId = firstAttachment): components['schemas']['SubmissionFile'] {
  return {
    attachment_id: attachmentId,
    filename: 'corrected.pdf',
    media_type: 'application/pdf',
    size_bytes: 2048,
    width: null,
    height: null,
    url: `https://storage.example/${address}`,
    expires_at: '2026-11-01T00:05:00Z',
  };
}

function reviewed(version: number, status: StudentSubmission['status'], review: StudentReview): StudentSubmission {
  return { ...checked(version), status, review };
}

function commented(comment: string, address = 'first', attachmentId = firstAttachment): StudentReview {
  return {
    feedback: '',
    items: [
      { question_id: pastSimpleQuestions.file, comment, corrected_files: [correctedFile(address, attachmentId)] },
    ],
  };
}

async function markersAfterRefetchShowing(
  first: StudentSubmission,
  second: StudentSubmission,
): Promise<readonly { readonly id: string; readonly version: number }[]> {
  const shown = await showResults(first);
  await waitFor(() => {
    expect(shown.server.feedbackSeenRequests()).toHaveLength(1);
  });
  shown.server.addSubmission(student.email, tutor.email, second);
  await refetchSubmission(shown);
  await waitFor(() => {
    expect(shown.queryClient.getQueryData<{ version: number }>(submissionKey(submissionId))?.version).toBe(
      second.version,
    );
  });
  await waitFor(() => {
    expect(shown.queryClient.isFetching() + shown.queryClient.isMutating()).toBe(0);
  });
  return shown.server.feedbackSeenRequests();
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

  test('a refetch showing a newly published review sends the marker again with its version', async () => {
    const shown = await showResults(checked(4));
    await waitFor(() => {
      expect(shown.server.feedbackSeenRequests()).toHaveLength(1);
    });
    shown.server.addSubmission(student.email, tutor.email, checked(6, 'Better now.'));

    await refetchSubmission(shown);
    await waitFor(() => {
      expect(shown.server.feedbackSeenRequests()).toHaveLength(2);
    });

    expect(shown.server.feedbackSeenRequests()).toEqual([
      { id: submissionId, version: 4 },
      { id: submissionId, version: 6 },
    ]);
  });

  test('newer versions that keep the shown review, as autosaves do, send no further marker', async () => {
    const shown = await showResults(checked(4));
    await waitFor(() => {
      expect(shown.server.feedbackSeenRequests()).toHaveLength(1);
    });

    shown.server.addSubmission(student.email, tutor.email, checked(5));
    await refetchSubmission(shown);
    shown.server.addSubmission(student.email, tutor.email, checked(6));
    await refetchSubmission(shown);
    await waitFor(() => {
      expect(shown.queryClient.getQueryData<StudentSubmission>(submissionKey(submissionId))?.version).toBe(6);
    });

    expect(shown.server.feedbackSeenRequests()).toEqual([{ id: submissionId, version: 4 }]);
  });

  test('a marker refused with a conflict is sent again at the next version that shows the same review', async () => {
    const server = new FakeAuthServer();
    server.refuseNextFeedbackSeen(409, 'conflict');
    const shown = await showResultsOn(server, checked(4));
    await waitFor(() => {
      expect(shown.server.feedbackSeenRequests()).toHaveLength(1);
    });
    shown.server.addSubmission(student.email, tutor.email, checked(5));

    await refetchSubmission(shown);
    await waitFor(() => {
      expect(shown.server.feedbackSeenRequests()).toHaveLength(2);
    });

    expect(shown.server.feedbackSeenRequests()).toEqual([
      { id: submissionId, version: 4 },
      { id: submissionId, version: 5 },
    ]);
  });
  test('a marker refused with a conflict after an autosave moved the version meanwhile is sent again at once', async () => {
    const server = new FakeAuthServer();
    const paused = server.pauseFeedbackSeen();
    const shown = await showResultsOn(server, checked(4));
    await paused.received;
    shown.server.addSubmission(student.email, tutor.email, checked(5));
    await refetchSubmission(shown);
    await waitFor(() => {
      expect(shown.queryClient.getQueryData<StudentSubmission>(submissionKey(submissionId))?.version).toBe(5);
    });

    act(() => {
      paused.resume();
    });
    await waitFor(() => {
      expect(shown.server.feedbackSeenRequests()).toHaveLength(2);
    });

    expect(shown.server.feedbackSeenRequests()).toEqual([
      { id: submissionId, version: 4 },
      { id: submissionId, version: 5 },
    ]);
  });

  test('a marker refused for another reason is not sent again at the next version that shows the same review', async () => {
    const server = new FakeAuthServer();
    server.refuseNextFeedbackSeen(503, 'service_unavailable');
    const shown = await showResultsOn(server, checked(4));
    await waitFor(() => {
      expect(shown.server.feedbackSeenRequests()).toHaveLength(1);
    });
    shown.server.addSubmission(student.email, tutor.email, checked(5));

    await refetchSubmission(shown);
    await waitFor(() => {
      expect(shown.queryClient.getQueryData<StudentSubmission>(submissionKey(submissionId))?.version).toBe(5);
    });

    expect(shown.server.feedbackSeenRequests()).toEqual([{ id: submissionId, version: 4 }]);
  });

  test('a newly published review that changes only an item comment sends the marker again', async () => {
    const first = reviewed(4, 'checked', commented('Check the spelling.'));
    const second = reviewed(6, 'checked', commented('Better now.'));

    const markers = await markersAfterRefetchShowing(first, second);

    expect(markers).toEqual([
      { id: submissionId, version: 4 },
      { id: submissionId, version: 6 },
    ]);
  });

  test('a check after a return that keeps the returned review sends the marker again', async () => {
    const first = reviewed(4, 'returned', commented('Check the spelling.'));
    const second = reviewed(6, 'checked', commented('Check the spelling.'));

    const markers = await markersAfterRefetchShowing(first, second);

    expect(markers).toEqual([
      { id: submissionId, version: 4 },
      { id: submissionId, version: 6 },
    ]);
  });

  test('a newer version whose corrected file only has a fresh download address sends no further marker', async () => {
    const first = reviewed(4, 'returned', commented('Check the spelling.', 'first'));
    const second = reviewed(5, 'returned', commented('Check the spelling.', 'renewed'));

    const markers = await markersAfterRefetchShowing(first, second);

    expect(markers).toEqual([{ id: submissionId, version: 4 }]);
  });
  test('a newly published review that changes only a corrected file sends the marker again', async () => {
    const first = reviewed(4, 'checked', commented('Check the spelling.', 'first', firstAttachment));
    const second = reviewed(6, 'checked', commented('Check the spelling.', 'second', secondAttachment));

    const markers = await markersAfterRefetchShowing(first, second);

    expect(markers).toEqual([
      { id: submissionId, version: 4 },
      { id: submissionId, version: 6 },
    ]);
  });
});
