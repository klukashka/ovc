import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { MediaQueriesContext, fixedMediaQueries } from '../../app/context.ts';
import { loadingDelayMilliseconds } from '../../components/ui/LoadState.tsx';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, type ManualScheduler, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { CalendarItem } from './api.ts';
import { SchedulePage } from './SchedulePage.tsx';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const email = 'maria@example.org';
const password = 'a long password';

const sundayLesson: CalendarItem = {
  id: '00000000-0000-4000-8000-000000000002',
  title: 'Воскресный урок',
  starts_at: '2026-11-08T15:00:00Z',
  ends_at: '2026-11-08T16:00:00Z',
  status: 'scheduled',
  took_place: false,
  timezone: 'Europe/Moscow',
  audience: { kind: 'one_to_one', label: 'Анна', label_kind: null, colour: 'moss' },
  tutor: null,
};

const shownWeekTitle = /2.+8 нояб/;

interface ShownSchedule {
  readonly server: FakeAuthServer;
  readonly scheduler: ManualScheduler;
  readonly router: ReturnType<typeof createMemoryRouter>;
}

function calendarReads(server: FakeAuthServer): number {
  return server.sentAddresses.filter((address) => address.pathname === '/api/v1/calendar').length;
}

async function showScheduleOnSundayNight(address = '/schedule'): Promise<ShownSchedule> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Мария', email, password });
  server.addCalendarItem(email, sundayLesson);
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(email, password);
  const scheduler = manualScheduler();
  scheduler.suspend(Date.parse('2026-11-08T20:59:30Z'));
  const i18n = await createI18n('ru');
  const router = createMemoryRouter(
    [
      {
        path: '/schedule',
        element: (
          <main>
            <SchedulePage />
          </main>
        ),
      },
    ],
    {
      initialEntries: [address],
    },
  );
  render(
    <SchedulerContext value={scheduler}>
      <MediaQueriesContext value={fixedMediaQueries(['(min-width: 768px)', '(min-width: 1024px) and (pointer: fine)'])}>
        <SessionContext value={session}>
          <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
            <QueryClientProvider client={new QueryClient()}>
              <I18nextProvider i18n={i18n}>
                <RouterProvider router={router} />
              </I18nextProvider>
            </QueryClientProvider>
          </ApiClientContext>
        </SessionContext>
      </MediaQueriesContext>
    </SchedulerContext>,
  );
  await screen.findByRole('link', { name: /Воскресный урок/ });
  return { server, scheduler, router };
}

function busyHolder(): HTMLElement {
  const grid = screen.getByRole('region', { name: 'Уроки недели' });
  const holder = grid.closest<HTMLElement>('[aria-busy]');
  if (holder === null) {
    throw new Error('the week grid is not inside the busy holder');
  }
  return holder;
}

describe('the Schedule page', () => {
  test('while the next week loads the shown week stays busy, says it is loading after the delay and keeps focus', async () => {
    const { server, scheduler } = await showScheduleOnSundayNight();
    const paused = server.pauseCalendarReads();
    const next = screen.getByRole('button', { name: 'Следующая неделя' });
    next.focus();

    act(() => {
      fireEvent.click(next);
    });
    await paused.received;
    act(() => {
      scheduler.pass(loadingDelayMilliseconds);
    });

    await waitFor(() => {
      expect(screen.getByText('Загружаем')).toBeDefined();
    });
    expect(busyHolder().getAttribute('aria-busy')).toBe('true');
    expect(busyHolder().classList.contains('aria-busy:opacity-60')).toBe(true);
    expect(busyHolder().contains(screen.getByText('Загружаем'))).toBe(false);
    expect(screen.getByRole('link', { name: /Воскресный урок/ })).toBeDefined();
    expect(document.activeElement).toBe(next);
    paused.resume();
  });

  test('a lesson link keeps focus when today turns into the next week at local midnight', async () => {
    const { server, scheduler } = await showScheduleOnSundayNight();
    const lesson = screen.getByRole('link', { name: /Воскресный урок/ });
    lesson.focus();

    await act(async () => {
      scheduler.pass(30_000);
      await Promise.resolve();
    });

    expect(document.activeElement).toBe(lesson);
    expect(lesson.isConnected).toBe(true);
    expect(screen.getByRole('heading', { level: 2 }).textContent).toMatch(shownWeekTitle);
    expect(calendarReads(server)).toBe(1);
  });

  test('leaving a dated address after midnight shows the period of the new today', async () => {
    const { scheduler, router } = await showScheduleOnSundayNight('/schedule?date=2026-11-04');
    await act(async () => {
      scheduler.pass(30_000);
      await Promise.resolve();
    });

    await act(async () => {
      await router.navigate('/schedule');
    });

    await waitFor(() => {
      expect(screen.getByRole('heading', { level: 2 }).textContent).toMatch(/9.+15 нояб/);
    });
  });
});
