import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { type ReactElement, useState } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { AnswerFiles, type FileQuestion, useFileDropGuard } from './AnswerFiles.tsx';
import type { AnswerFile } from './api.ts';
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

function answerFile(name: string): AnswerFile {
  return {
    attachmentId: `attachment-${name}`,
    filename: name,
    mediaType: 'application/pdf',
    sizeBytes: 1_000,
    url: '',
    expiresAt: '',
  };
}

function FileAnswer({
  block,
  initial,
  confirmed,
}: {
  readonly block: FileQuestion;
  readonly initial: readonly AnswerFile[];
  readonly confirmed: boolean;
}): ReactElement {
  const [files, setFiles] = useState(initial);
  return (
    <AnswerFiles
      submissionId="submission-1"
      block={block}
      files={files}
      editable
      confirmed={confirmed}
      onChange={setFiles}
      onUploading={() => undefined}
    />
  );
}

async function showFileAnswer(
  block: FileQuestion,
  initial: readonly AnswerFile[] = [],
  confirmed = true,
): Promise<void> {
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
            <FileAnswer block={block} initial={initial} confirmed={confirmed} />
          </main>
        </I18nextProvider>
      </ApiClientContext>
    </SessionContext>,
  );
}

function dropZone(): HTMLElement {
  return screen.getByRole('button', { name: 'Choose files' });
}

interface DroppedItem {
  readonly webkitGetAsEntry: () => { readonly isDirectory: boolean } | null;
}

function dropFiles(target: HTMLElement, files: readonly File[], folder = false): void {
  const items: readonly DroppedItem[] = files.map(() => ({
    webkitGetAsEntry: () => (folder ? { isDirectory: true } : null),
  }));
  fireEvent.drop(target, { dataTransfer: { files, items, types: ['Files'] } });
}

const pdfOnly: FileQuestion = { ...photoQuestion, allowedKinds: ['pdf'], maxFiles: 2 };

describe('the drop zone of a file question', () => {
  test('a dropped file of an allowed kind is sent as an answer upload', async () => {
    await showFileAnswer(pdfOnly);

    dropFiles(dropZone(), [new File(['%PDF-1.7'], 'essay.pdf', { type: 'application/pdf' })]);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe(assignmentTexts.files.busy);
  });

  test('a dropped file of a kind outside the question’s kinds shows the refusal, which takes focus', async () => {
    await showFileAnswer(pdfOnly);

    dropFiles(dropZone(), [new File(['png'], 'photo.png', { type: 'image/png' })]);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe(assignmentTexts.files.wrongType);
    expect(document.activeElement).toBe(alert);
  });

  test('a drop beyond the question’s maximum is refused', async () => {
    await showFileAnswer(pdfOnly, [answerFile('first.pdf')]);

    dropFiles(dropZone(), [
      new File(['%PDF-1.7'], 'second.pdf', { type: 'application/pdf' }),
      new File(['%PDF-1.7'], 'third.pdf', { type: 'application/pdf' }),
    ]);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe('At most 2 files');
  });

  test('a dropped folder is refused, which takes focus', async () => {
    await showFileAnswer(pdfOnly);

    dropFiles(dropZone(), [new File([], 'scans.pdf', { type: '' })], true);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe(assignmentTexts.files.wrongType);
    expect(document.activeElement).toBe(alert);
  });

  test('a full question keeps the drop zone focusable, unavailable and saying why', async () => {
    await showFileAnswer(pdfOnly, [answerFile('first.pdf'), answerFile('second.pdf')]);

    fireEvent.click(dropZone());

    expect(dropZone().getAttribute('aria-disabled')).toBe('true');
    expect(dropZone()).toHaveProperty('disabled', false);
    expect(screen.getByText('You have added the maximum of 2 files', { exact: true })).toBeTruthy();
    expect((await screen.findByRole('alert')).textContent).toBe('At most 2 files');
  });

  test('the hint names the allowed kinds and the remaining count', async () => {
    await showFileAnswer({ ...photoQuestion, maxFiles: 3 }, [answerFile('first.pdf')]);

    const hint = screen.getByText('pictures, PDF; you can add 2 more files', { exact: true });

    expect(dropZone().getAttribute('aria-describedby')).toBe(hint.id);
  });

  test('removing the only file moves focus to the drop zone', async () => {
    await showFileAnswer(pdfOnly, [answerFile('only.pdf')]);

    fireEvent.click(screen.getByRole('button', { name: 'Remove only.pdf' }));

    expect(document.activeElement).toBe(dropZone());
  });

  test('an unconfirmed student gets no drop zone and no attach control', async () => {
    await showFileAnswer(pdfOnly, [], false);

    const controls = [
      screen.queryByRole('button', { name: 'Choose files' }),
      screen.queryByRole('button', { name: 'Attach files' }),
      document.querySelector('[data-drop-zone]'),
    ];

    expect(controls).toEqual([null, null, null]);
    expect(screen.getByText(assignmentTexts.files.confirmEmail, { exact: true })).toBeTruthy();
  });
});

function GuardedPage(): ReactElement {
  useFileDropGuard();
  return (
    <main>
      <textarea aria-label="Your answer" />
      <button type="button" data-drop-zone="">
        Choose files
      </button>
    </main>
  );
}

describe('the work page’s guard against dropped files', () => {
  test('a file dragged or dropped outside a drop zone is held so the page is not replaced', () => {
    render(<GuardedPage />);
    const files = { types: ['Files'], dropEffect: 'copy' };

    const dragged = fireEvent.dragOver(document.body, { dataTransfer: files });
    const dropped = fireEvent.drop(document.body, { dataTransfer: { types: ['Files'] } });

    expect([dragged, dropped]).toEqual([false, false]);
    expect(files.dropEffect).toBe('none');
  });

  test('a file dragged over a drop zone keeps the zone’s own drop effect', () => {
    render(<GuardedPage />);
    const files = { types: ['Files'], dropEffect: 'copy' };

    fireEvent.dragOver(screen.getByRole('button', { name: 'Choose files' }).firstChild ?? document.body, {
      dataTransfer: files,
    });

    expect(files.dropEffect).toBe('copy');
  });

  test('text dragged into an answer field is left to the browser', () => {
    render(<GuardedPage />);

    const dropped = fireEvent.drop(screen.getByRole('textbox', { name: 'Your answer' }), {
      dataTransfer: { types: ['text/plain'] },
    });

    expect(dropped).toBe(true);
  });
});
