import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { type ReactElement, useState } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { QuestionBlock } from '../builder/blocks.ts';
import { TaskAttachments } from './attachmentQueue.ts';
import { type QuestionType, newQuestion } from './newTask.ts';
import { TaskAttachmentsField } from './TaskAttachments.tsx';
import { TaskQuestions } from './TaskQuestions.tsx';
import { homeworkTexts } from './texts.en.ts';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const email = 'maria@example.org';
const password = 'a long password';

async function englishTexts(): Promise<Awaited<ReturnType<typeof createI18n>>> {
  const i18n = await createI18n('en');
  i18n.addResourceBundle('en', 'translation', { homework: homeworkTexts }, true, true);
  return i18n;
}

function questionsOf(types: readonly QuestionType[]): readonly QuestionBlock[] {
  let next = 0;
  const newId = (): string => {
    next += 1;
    return `00000000-0000-4000-8000-${String(next).padStart(12, '0')}`;
  };
  return types.map((type) => newQuestion(type, newId));
}

function QuestionList({ initial }: { readonly initial: readonly QuestionBlock[] }): ReactElement {
  const [questions, setQuestions] = useState(initial);
  return (
    <TaskQuestions
      questions={questions}
      problems={() => undefined}
      locked={false}
      full={false}
      onAdd={() => undefined}
      onChange={(changed) => {
        setQuestions((previous) => previous.map((question) => (question.id === changed.id ? changed : question)));
      }}
      onRemove={(questionId) => {
        setQuestions((previous) => previous.filter((question) => question.id !== questionId));
      }}
    />
  );
}

async function showQuestions(types: readonly QuestionType[]): Promise<void> {
  const i18n = await englishTexts();
  render(
    <I18nextProvider i18n={i18n}>
      <QuestionList initial={questionsOf(types)} />
    </I18nextProvider>,
  );
}

async function showRefusedAttachments(names: readonly string[]): Promise<void> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Maria', email, password, role: 'tutor' });
  names.forEach(() => {
    server.refuseNextLibraryUpload(507, 'quota_exceeded');
  });
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(email, password);
  const client = createApiClient(origin, session.authorizedFetch);
  const store = new TaskAttachments(client, session, { folder: 'Task attachments', fallbackTitle: 'Attachment' });
  const i18n = await englishTexts();
  render(
    <I18nextProvider i18n={i18n}>
      <TaskAttachmentsField attachments={store} locked={false} />
    </I18nextProvider>,
  );
  store.add(names.map((name) => new File(['%PDF-1.7'], name, { type: 'application/pdf' })));
  await waitFor(() => {
    expect(screen.getAllByRole('alert')).toHaveLength(names.length);
  });
}

function removeButtonOf(name: string): HTMLElement {
  return screen.getByRole('button', { name: `Remove ${name}` });
}

describe('focus on the New task page', () => {
  test('removing a question moves focus to the heading of the next question', async () => {
    await showQuestions(['single_choice', 'long_text', 'file']);

    fireEvent.click(screen.getByRole('button', { name: 'Remove question 2' }));

    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 3, name: /^Question 2 · File or photo/ }));
  });

  test('removing the last question moves focus to the heading of the previous one', async () => {
    await showQuestions(['single_choice', 'long_text']);

    fireEvent.click(screen.getByRole('button', { name: 'Remove question 2' }));

    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 3, name: /^Question 1 · Single choice/ }));
  });

  test('removing the only question moves focus to the question type choice', async () => {
    await showQuestions(['short_text']);

    fireEvent.click(screen.getByRole('button', { name: 'Remove question 1' }));

    expect(document.activeElement).toBe(screen.getByLabelText('Question type'));
  });

  test('removing an option moves focus to the option that takes its place', async () => {
    await showQuestions(['multiple_choice']);
    fireEvent.click(screen.getByRole('button', { name: 'Add option' }));
    fireEvent.change(screen.getByLabelText('Option 3'), { target: { value: 'third' } });

    fireEvent.click(screen.getByRole('button', { name: 'Remove option 2' }));

    expect(document.activeElement).toBe(screen.getByLabelText('Option 2'));
    expect(screen.getByLabelText<HTMLInputElement>('Option 2').value).toBe('third');
  });

  test('removing a file moves focus to the Remove button of the next file', async () => {
    await showRefusedAttachments(['first.pdf', 'second.pdf']);

    fireEvent.click(removeButtonOf('first.pdf'));

    expect(document.activeElement).toBe(removeButtonOf('second.pdf'));
  });

  test('removing the last of several files moves focus to the Remove button of the previous file', async () => {
    await showRefusedAttachments(['first.pdf', 'second.pdf']);

    fireEvent.click(removeButtonOf('second.pdf'));

    expect(document.activeElement).toBe(removeButtonOf('first.pdf'));
  });

  test('removing the only file moves focus to Choose files', async () => {
    await showRefusedAttachments(['only.pdf']);

    fireEvent.click(removeButtonOf('only.pdf'));

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Choose files' }));
  });

  test('attached files show as tiles named by their files, with the extension and no picture of the file', async () => {
    const names = ['first.pdf', 'second.pdf'];

    await showRefusedAttachments(names);

    expect(screen.getByRole('listitem', { name: 'first.pdf' })).toBeDefined();
    expect(screen.getByRole('listitem', { name: 'second.pdf' })).toBeDefined();
    expect(screen.getAllByText('PDF')).toHaveLength(2);
    expect(screen.getByRole('button', { name: 'Remove first.pdf' })).toBeDefined();
    expect(document.querySelectorAll('[data-attachment-tile] img')).toHaveLength(0);
  });

  test('Retry moves focus to the Remove button of the same file', async () => {
    await showRefusedAttachments(['retried.pdf']);

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));

    expect(document.activeElement).toBe(removeButtonOf('retried.pdf'));
  });
});
