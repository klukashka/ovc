import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { webpUploadLimitBytes } from '../library/api.ts';
import { TaskAttachments } from './attachmentQueue.ts';
import { TaskAttachmentsField } from './TaskAttachments.tsx';
import { homeworkTexts } from './texts.en.ts';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const email = 'maria@example.org';
const password = 'a long password';

interface ShownAttachments {
  readonly store: TaskAttachments;
}

async function showAttachments(): Promise<ShownAttachments> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Maria', email, password, role: 'tutor' });
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(email, password);
  const client = createApiClient(origin, session.authorizedFetch);
  const store = new TaskAttachments(client, session, { folder: 'Task attachments', fallbackTitle: 'Attachment' });
  const i18n = await createI18n('en');
  i18n.addResourceBundle('en', 'translation', { homework: homeworkTexts }, true, true);
  render(
    <I18nextProvider i18n={i18n}>
      <TaskAttachmentsField attachments={store} locked={false} />
    </I18nextProvider>,
  );
  return { store };
}

interface DroppedItem {
  readonly webkitGetAsEntry: () => { readonly isDirectory: boolean } | null;
}

function dropFolder(target: HTMLElement): void {
  const files = [new File([], 'scans', { type: '' })];
  const items: readonly DroppedItem[] = [{ webkitGetAsEntry: () => ({ isDirectory: true }) }];
  fireEvent.drop(target, { dataTransfer: { files, items, types: ['Files'] } });
}

describe('the attachment area of New task', () => {
  test('a dropped folder is refused with its own alert, which takes focus, and attaches nothing', async () => {
    const { store } = await showAttachments();

    dropFolder(screen.getByRole('button', { name: 'Choose files' }));

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe(homeworkTexts.newTask.folderRefused);
    expect(document.activeElement).toBe(alert);
    expect(store.snapshot()).toEqual([]);
  });

  test('a WebP picture over 8 MB is refused before it is sent with the WebP limit, not the general one', async () => {
    const { store } = await showAttachments();

    store.add([new File([new Uint8Array(webpUploadLimitBytes + 1)], 'scan.webp', { type: 'image/webp' })]);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe('WebP pictures can be at most 8 MB');
  });
});
