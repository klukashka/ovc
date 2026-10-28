import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { createRef } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, type ManualScheduler, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { AudiencePicker, searchDelayMilliseconds } from './AudiencePicker.tsx';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';

function studentSearches(server: FakeAuthServer): (string | null)[] {
  return server.sentAddresses
    .filter((address) => address.pathname === '/api/v1/students')
    .map((address) => address.searchParams.get('q'));
}

async function showPicker(server: FakeAuthServer, scheduler: ManualScheduler): Promise<void> {
  server.addAccount({ id: 'tutor-1', name: 'Мария', email: 'maria@example.org', password: 'a long password' });
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn('maria@example.org', 'a long password');
  const i18n = await createI18n('ru');
  render(
    <SchedulerContext value={scheduler}>
      <SessionContext value={session}>
        <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
          <QueryClientProvider client={new QueryClient()}>
            <I18nextProvider i18n={i18n}>
              <AudiencePicker
                chosen={null}
                preselected={null}
                searchField={createRef<HTMLInputElement>()}
                error={undefined}
                onChoose={() => undefined}
              />
            </I18nextProvider>
          </QueryClientProvider>
        </ApiClientContext>
      </SessionContext>
    </SchedulerContext>,
  );
  await waitFor(() => {
    expect(studentSearches(server)).toEqual([null]);
  });
}

describe('the audience picker search', () => {
  test('a burst of typing sends one search for the settled text', async () => {
    const server = new FakeAuthServer();
    const scheduler = manualScheduler();
    await showPicker(server, scheduler);
    const search = screen.getByLabelText('Поиск ученика или группы');

    for (const typed of ['А', 'Ан', 'Анн', 'Анна']) {
      act(() => {
        fireEvent.change(search, { target: { value: typed } });
      });
    }
    act(() => {
      scheduler.pass(searchDelayMilliseconds);
    });

    await waitFor(() => {
      expect(studentSearches(server)).toContain('Анна');
    });
    expect(studentSearches(server)).toEqual([null, 'Анна']);
  });
});
