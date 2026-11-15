import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { AnswerFiles, type FileQuestion } from './AnswerFiles.tsx';
import { assignmentTexts } from './texts.en.ts';

const origin = 'https://ovc.example';
const email = 'anna@example.org';
const password = 'a long password';

const photoQuestion: FileQuestion = {
  type: 'file',
  id: 'question-1',
  prompt: 'Photograph your answer',
  images: [],
  allowedKinds: ['images', 'pdf'],
  maxFiles: 3,
};

afterEach(() => {
  cleanup();
});

async function showAnswerFilesWithBusyUploads(): Promise<HTMLInputElement> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'student-1', name: 'Anna', email, password, role: 'student' });
  server.refuseNextHomeworkUpload(503, 'service_unavailable');
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(email, password);
  const i18n = await createI18n('en');
  i18n.addResourceBundle('en', 'translation', { assignment: assignmentTexts }, true, true);
  render(
    <SessionContext value={session}>
      <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
        <I18nextProvider i18n={i18n}>
          <main>
            <h1 tabIndex={-1}>Past simple</h1>
            <AnswerFiles
              submissionId="submission-1"
              block={photoQuestion}
              files={[]}
              editable
              confirmed
              onChange={() => undefined}
              onUploading={() => undefined}
            />
          </main>
        </I18nextProvider>
      </ApiClientContext>
    </SessionContext>,
  );
  const input = document.querySelector<HTMLInputElement>('input[type="file"]');
  if (input === null) {
    throw new Error('the file question shows no file input');
  }
  return input;
}

describe('answer file upload refusals', () => {
  test('uploads refused as busy show an alert that takes focus', async () => {
    const input = await showAnswerFilesWithBusyUploads();

    await act(async () => {
      fireEvent.change(input, {
        target: { files: [new File(['%PDF-1.7'], 'answer.pdf', { type: 'application/pdf' })] },
      });
      await Promise.resolve();
    });

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe(assignmentTexts.files.busy);
    expect(document.activeElement).toBe(alert);
  });
});
