import { cleanup, render, screen } from '@testing-library/react';
import { createRef } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { UploadStatus } from './LibraryDialogs.tsx';
import { libraryTexts } from './texts.ru.ts';
import { LibraryUpload } from './upload.ts';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const email = 'maria@example.org';
const password = 'a long password';

interface Refusal {
  readonly status: number;
  readonly code: string;
  readonly fieldCode?: string;
}

async function showRefusedUpload(refusal: Refusal, file: File): Promise<void> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Мария', email, password, role: 'tutor' });
  if (refusal.fieldCode === undefined) {
    server.refuseNextLibraryUpload(refusal.status, refusal.code);
  } else {
    server.refuseNextLibraryUploadAsTooLarge(refusal.fieldCode);
  }
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(email, password);
  const client = createApiClient(origin, session.authorizedFetch);
  const i18n = await createI18n('ru');
  i18n.addResourceBundle('ru', 'translation', { library: libraryTexts }, true, true);
  render(
    <I18nextProvider i18n={i18n}>
      <main>
        <h1 tabIndex={-1}>Библиотека</h1>
        <UploadStatus upload={new LibraryUpload(client, session, file)} refusedBeforeSending={createRef()} />
      </main>
    </I18nextProvider>,
  );
}

describe('library upload refusals', () => {
  test('a server refusal of the size shows the library limit in an alert that takes focus', async () => {
    await showRefusedUpload(
      { status: 413, code: 'payload_too_large' },
      new File(['%PDF-1.7'], 'lecture.pdf', { type: 'application/pdf' }),
    );

    const alert = await screen.findByRole('alert');

    expect(alert.textContent).toBe('Файл больше 100 МБ');
    expect(document.activeElement).toBe(alert);
  });

  test('a server refusal of a WebP picture shows its own limit in an alert that takes focus', async () => {
    await showRefusedUpload(
      { status: 413, code: 'payload_too_large', fieldCode: 'webp_too_large' },
      new File(['RIFF'], 'photo.webp', { type: 'image/webp' }),
    );

    const alert = await screen.findByRole('alert');

    expect(alert.textContent).toBe('Изображения WebP могут быть не больше 8 МБ');
    expect(document.activeElement).toBe(alert);
  });
});
