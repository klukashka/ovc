import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { pastSimpleSubmission } from '../../lib/api/fakeSubmissions.ts';
import { FakeAuthServer, type ManualScheduler, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { AssignmentPage } from './AssignmentPage.tsx';
import { autosaveDelayMilliseconds } from './autosave.ts';
import { assignmentTexts } from './texts.en.ts';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const submissionId = '4c3b2a19-0f8e-4d7c-9b6a-5f4e3d2c1b0a';
const tutor = { id: 'tutor-1', name: 'Maria', email: 'maria@example.org', password: 'a long password' };
const student = { id: 'student-1', name: 'Anna', email: 'anna@example.org', password: 'another long password' };

interface ShownAssignment {
  readonly server: FakeAuthServer;
  readonly scheduler: ManualScheduler;
}

async function showAssignment(): Promise<ShownAssignment> {
  const server = new FakeAuthServer();
  server.addAccount(tutor);
  server.addAccount({ ...student, role: 'student' });
  server.addSubmission(student.email, tutor.email, pastSimpleSubmission(submissionId));
  const scheduler = manualScheduler();
  const session = new Session({ origin, transport: server.transport(), locks: memoryLocks(), scheduler });
  await session.signIn(student.email, student.password);
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
    <SchedulerContext value={scheduler}>
      <SessionContext value={session}>
        <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
          <QueryClientProvider client={new QueryClient()}>
            <I18nextProvider i18n={i18n}>
              <RouterProvider router={router} />
            </I18nextProvider>
          </QueryClientProvider>
        </ApiClientContext>
      </SessionContext>
    </SchedulerContext>,
  );
  await screen.findByRole('heading', { level: 1, name: 'Past simple' });
  return { server, scheduler };
}

function question(number: number): HTMLElement {
  return screen.getByRole('group', { name: `Question ${String(number)}` });
}

function leaveTheQuestion(): void {
  screen.getByRole('heading', { level: 1 }).focus();
}

async function letTheAutosaveRun(scheduler: ManualScheduler): Promise<void> {
  await act(async () => {
    scheduler.pass(autosaveDelayMilliseconds);
    await Promise.resolve();
  });
}

describe('the assignment page after a refused save', () => {
  test('a save refused as validation_failed moves focus to the first named question', async () => {
    const { server, scheduler } = await showAssignment();
    server.refuseNextAnswerSave(422, 'validation_failed', [{ path: 'body.answers[0].value.text', code: 'too_long' }]);
    const field = within(question(2)).getByRole('textbox');
    fireEvent.change(field, { target: { value: 'went' } });
    leaveTheQuestion();

    await letTheAutosaveRun(scheduler);

    await waitFor(() => {
      expect(document.activeElement).toBe(field);
    });
    expect(field.getAttribute('aria-invalid')).toBe('true');
  });

  test('a file binding refused over the tutor’s full quota focuses the alert under the file question', async () => {
    const { server, scheduler } = await showAssignment();
    server.fillFileQuota(tutor.email);
    const input = question(3).querySelector<HTMLInputElement>('input[type="file"]');
    if (input === null) {
      throw new Error('the file question shows no file input');
    }
    await act(async () => {
      fireEvent.change(input, {
        target: { files: [new File(['%PDF-1.7'], 'answer.pdf', { type: 'application/pdf' })] },
      });
      await Promise.resolve();
    });
    await within(question(3)).findByText('answer.pdf');
    leaveTheQuestion();

    await letTheAutosaveRun(scheduler);

    const alert = await within(question(3)).findByRole('alert');
    expect(alert.textContent).toBe(assignmentTexts.files.storageFull);
    expect(document.activeElement).toBe(alert);
  });
});
