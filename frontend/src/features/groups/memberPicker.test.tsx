import { QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { FollowingQueryClient } from '../../app/queryClient.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { MemberPicker } from './MemberPicker.tsx';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const password = 'a long password';
const tutorEmail = 'maria@example.org';
const activeStudents = 31;

function studentName(index: number): string {
  return `Student ${String(index).padStart(2, '0')}`;
}

async function showPicker(): Promise<FakeAuthServer> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Maria', email: tutorEmail, password, role: 'tutor' });
  for (let index = 1; index <= activeStudents; index += 1) {
    server.addActiveStudent(tutorEmail, studentName(index), `student${String(index)}@example.org`);
  }
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(tutorEmail, password);
  const i18n = await createI18n('en');
  render(
    <SessionContext value={session}>
      <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
        <QueryClientProvider client={new FollowingQueryClient()}>
          <I18nextProvider i18n={i18n}>
            <main>
              <MemberPicker excluded={new Set()} chosen={new Map()} slots={30} onToggle={() => undefined} />
            </main>
          </I18nextProvider>
        </QueryClientProvider>
      </ApiClientContext>
    </SessionContext>,
  );
  await screen.findByRole('checkbox', { name: studentName(activeStudents) });
  return server;
}

function studentList(): HTMLElement {
  return screen.getByRole('list', { name: 'Active students' });
}

describe('the group member picker', () => {
  test('a search submitted while a Show more page loads keeps focus in the search field', async () => {
    const server = await showPicker();
    const held = server.pauseStudentListReads();
    fireEvent.click(screen.getByRole('button', { name: 'Show more' }));
    await held.received;
    const searchField = screen.getByRole('searchbox', { name: 'Search students' });
    searchField.focus();

    fireEvent.change(searchField, { target: { value: 'Student 0' } });
    fireEvent.submit(screen.getByRole('search', { name: 'Search students' }));
    held.resume();
    await waitFor(() => {
      expect(within(studentList()).getAllByRole('checkbox')).toHaveLength(9);
    });

    expect(within(studentList()).getByRole('checkbox', { name: studentName(1) })).toBeTruthy();
    expect(document.activeElement).toBe(searchField);
  });
});
