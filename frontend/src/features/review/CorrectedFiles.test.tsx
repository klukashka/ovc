import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { homeworkTexts } from '../homework/texts.en.ts';
import { CorrectedFiles } from './CorrectedFiles.tsx';

const origin = 'https://ovc.example';
const email = 'pavel@example.org';
const password = 'a long password';

afterEach(() => {
  cleanup();
});

async function showCorrectedFilesWithFullStorage(): Promise<HTMLInputElement> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Pavel', email, password, role: 'tutor' });
  server.refuseNextHomeworkUpload(507, 'storage_full');
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(email, password);
  const i18n = await createI18n('en');
  i18n.addResourceBundle('en', 'translation', { homework: homeworkTexts }, true, true);
  render(
    <SessionContext value={session}>
      <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
        <I18nextProvider i18n={i18n}>
          <main>
            <h1 tabIndex={-1}>Past simple</h1>
            <CorrectedFiles
              submissionId="submission-1"
              questionId="question-1"
              files={[]}
              stored={[]}
              external={null}
              onChange={() => undefined}
            />
          </main>
        </I18nextProvider>
      </ApiClientContext>
    </SessionContext>,
  );
  const input = document.querySelector<HTMLInputElement>('input[type="file"]');
  if (input === null) {
    throw new Error('the corrected files show no file input');
  }
  return input;
}

describe('corrected file upload refusals', () => {
  test('an upload the server refuses for full storage shows an alert that takes focus', async () => {
    const input = await showCorrectedFilesWithFullStorage();

    await act(async () => {
      fireEvent.change(input, {
        target: { files: [new File(['%PDF-1.7'], 'Corrections.pdf', { type: 'application/pdf' })] },
      });
      await Promise.resolve();
    });

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe(homeworkTexts.review.files.storageFull);
    expect(document.activeElement).toBe(alert);
  });
});
