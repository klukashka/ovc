import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext, useSignedInAccount } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { CalendarItem, LessonDetail } from '../schedule/api.ts';
import { calendarKey } from '../schedule/keys.ts';
import { firstStudentCardKey } from '../students/api.ts';
import { type OpenRoomTab, RoomTabContext } from '../schedule/roomTab.ts';
import { FirstRunChecklist } from './FirstRunChecklist.tsx';
import { NextLesson, NextSevenDays } from './HomeLessons.tsx';
import { TodayLessons } from './TodayLessons.tsx';
import { useUpcomingLessons } from './upcoming.ts';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const password = 'a long password';
const tutorEmail = 'maria@example.org';
const studentEmail = 'anna@example.org';
const minute = 60_000;
const refetchFailedText = 'Не удалось обновить данные. На странице показано то, что загрузилось раньше.';
const loadFailedText = 'Что-то пошло не так.';
const checklistHeading = 'С чего начать';

const lessonDetail: LessonDetail = {
  id: '00000000-0000-4000-8000-000000000021',
  title: 'Разговорная практика',
  description: null,
  starts_at: '2026-11-09T15:00:00Z',
  ends_at: '2026-11-09T16:00:00Z',
  duration_minutes: 60,
  status: 'scheduled',
  took_place: false,
  timezone: 'Europe/Moscow',
  series_id: null,
  series: null,
  version: 1,
  cancelled_at: null,
  other_timezone: null,
  audience: { kind: 'one_to_one', label: 'Анна', label_kind: null, colour: 'moss', members: null },
  tutor: null,
  channel_id: null,
  room_open: false,
  embed_allowed: false,
};

const liveLessonDetail: LessonDetail = { ...lessonDetail, status: 'live', room_open: true, took_place: true };

function calendarItemOf(detail: LessonDetail, tutor: CalendarItem['tutor']): CalendarItem {
  return {
    id: detail.id,
    title: detail.title,
    starts_at: detail.starts_at,
    ends_at: detail.ends_at,
    status: detail.status,
    took_place: detail.took_place,
    timezone: detail.timezone,
    audience: { kind: 'one_to_one', label: 'Анна', label_kind: null, colour: 'moss' },
    tutor,
  };
}

class RecordedTab {
  readonly visited: string[] = [];
  closed = false;
}

class RoomTabs {
  readonly opened: RecordedTab[] = [];

  readonly open: OpenRoomTab = () => {
    const tab = new RecordedTab();
    this.opened.push(tab);
    return {
      navigate: (url): boolean => {
        if (tab.closed) {
          return false;
        }
        tab.visited.push(url);
        return true;
      },
      close: (): void => {
        tab.closed = true;
      },
    };
  };
}

function TodayHost(): ReactElement {
  const me = useSignedInAccount();
  const { today, query } = useUpcomingLessons(me, null);
  return (
    <>
      <FirstRunChecklist me={me} lessons={query} />
      <TodayLessons me={me} today={today} query={query} />
    </>
  );
}

function HomeHost(): ReactElement {
  const me = useSignedInAccount();
  const { query } = useUpcomingLessons(me, null);
  return (
    <>
      <NextLesson me={me} query={query} />
      <NextSevenDays me={me} query={query} scheduleAddress="/schedule" />
    </>
  );
}

type Shown = 'today' | 'home';

interface ShownPage {
  readonly server: FakeAuthServer;
  readonly tabs: RoomTabs;
  readonly queryClient: QueryClient;
}

interface PageSetup {
  readonly page: Shown;
  readonly bio?: string;
  readonly withStudentCard?: boolean;
}

async function showPage({ page, bio, withStudentCard = false }: PageSetup): Promise<ShownPage> {
  const server = new FakeAuthServer();
  const role = page === 'today' ? 'tutor' : 'student';
  const email = page === 'today' ? tutorEmail : studentEmail;
  const detail = page === 'today' ? lessonDetail : liveLessonDetail;
  server.addAccount({
    id: `${role}-1`,
    name: 'Участник',
    email,
    password,
    role,
    ...(bio === undefined ? {} : { bio }),
  });
  server.addLesson(detail, [email]);
  server.addCalendarItem(email, calendarItemOf(detail, page === 'today' ? null : { name: 'Мария', colour: 'moss' }));
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(email, password);
  const client = createApiClient(origin, session.authorizedFetch);
  if (withStudentCard) {
    await client.POST('/api/v1/students', { body: { display_name: 'Анна' } });
  }
  const scheduler = manualScheduler();
  scheduler.suspend(Date.parse(lessonDetail.starts_at) - 5 * minute);
  const i18n = await createI18n('ru');
  const tabs = new RoomTabs();
  const queryClient = new QueryClient();
  const router = createMemoryRouter(
    [
      {
        path: '/',
        element: (
          <main>
            <h1 tabIndex={-1}>{page === 'today' ? 'Сегодня' : 'Главная'}</h1>
            {page === 'today' ? <TodayHost /> : <HomeHost />}
          </main>
        ),
      },
    ],
    { initialEntries: ['/'] },
  );
  render(
    <SchedulerContext value={scheduler}>
      <RoomTabContext value={tabs.open}>
        <SessionContext value={session}>
          <ApiClientContext value={client}>
            <QueryClientProvider client={queryClient}>
              <I18nextProvider i18n={i18n}>
                <RouterProvider router={router} />
              </I18nextProvider>
            </QueryClientProvider>
          </ApiClientContext>
        </SessionContext>
      </RoomTabContext>
    </SchedulerContext>,
  );
  await screen.findAllByRole('link', { name: /Разговорная практика/ });
  return { server, tabs, queryClient };
}

async function joinAndHoldTheTab(
  shown: ShownPage,
  name: string,
): Promise<{ pressed: HTMLElement; resume: () => void }> {
  const pressed = await screen.findByRole('button', { name });
  const paused = shown.server.pauseLessonWrites();
  pressed.focus();
  fireEvent.click(pressed);
  await paused.received;
  return { pressed, resume: paused.resume };
}

async function refetchCalendarFailing(shown: ShownPage): Promise<void> {
  shown.server.failNextCalendarRead();
  await act(async () => {
    await shown.queryClient.refetchQueries({ queryKey: calendarKey });
  });
  await screen.findByText(refetchFailedText);
}

describe('a failed calendar refetch on Today', () => {
  test('keeps the lessons and a held Join control with its tab and focus, and shows the refetch notice', async () => {
    const shown = await showPage({ page: 'today' });
    const held = await joinAndHoldTheTab(shown, 'Подключиться к уроку');

    await refetchCalendarFailing(shown);
    const afterRefetch = {
      closed: shown.tabs.opened.map((tab) => tab.closed),
      notices: screen.getAllByText(refetchFailedText).length,
      lessonLinks: screen.getAllByRole('link', { name: /Разговорная практика/ }).length,
      loadFailures: screen.queryAllByText(loadFailedText).length,
      focused: document.activeElement,
    };
    held.resume();
    const openAgain = await screen.findByRole('button', { name: 'Открыть снова' });

    expect(afterRefetch).toEqual({
      closed: [false],
      notices: 1,
      lessonLinks: 1,
      loadFailures: 0,
      focused: held.pressed,
    });
    expect(openAgain).toBe(held.pressed);
    expect(document.activeElement).toBe(held.pressed);
    expect(shown.tabs.opened[0]?.visited).toHaveLength(1);
  });

  test('keeps a completed checklist hidden instead of showing a load failure', async () => {
    const shown = await showPage({ page: 'today', bio: 'Преподаю английский', withStudentCard: true });
    const hiddenBefore = screen.queryByRole('heading', { name: checklistHeading });

    await refetchCalendarFailing(shown);

    expect(hiddenBefore).toBeNull();
    expect(screen.queryByRole('heading', { name: checklistHeading })).toBeNull();
    expect(screen.queryByText(loadFailedText)).toBeNull();
    expect(screen.getByRole('link', { name: /Разговорная практика/ })).toBeTruthy();
  });
});

describe('a failed student list refetch on Today', () => {
  test('keeps the shown checklist steps and puts the refetch notice inside the checklist', async () => {
    const shown = await showPage({ page: 'today', withStudentCard: true });
    const checklist = (await screen.findByRole('heading', { name: checklistHeading })).closest('section');
    await waitFor(() => {
      expect(checklist?.querySelector('[data-step="invite"]')?.getAttribute('data-done')).toBe('true');
    });
    shown.server.failNextStudentListRead();

    await act(async () => {
      await shown.queryClient.refetchQueries({ queryKey: firstStudentCardKey });
    });
    const notice = await screen.findByText(refetchFailedText);

    expect(checklist?.contains(notice)).toBe(true);
    expect(screen.getAllByText(refetchFailedText)).toHaveLength(1);
    expect(checklist?.querySelector('[data-step="invite"]')?.getAttribute('data-done')).toBe('true');
    expect(checklist?.querySelectorAll('[data-step]')).toHaveLength(3);
    expect(screen.queryByText(loadFailedText)).toBeNull();
  });
});

describe('a failed calendar refetch on Home', () => {
  test('keeps the next lesson, the week and a held Join control with its tab and focus, with one notice', async () => {
    const shown = await showPage({ page: 'home' });
    const held = await joinAndHoldTheTab(shown, 'Подключиться');

    await refetchCalendarFailing(shown);
    const afterRefetch = {
      closed: shown.tabs.opened.map((tab) => tab.closed),
      notices: screen.getAllByText(refetchFailedText).length,
      lessonLinks: screen.getAllByRole('link', { name: /Разговорная практика/ }).length,
      loadFailures: screen.queryAllByText(loadFailedText).length,
      focused: document.activeElement,
    };
    held.resume();
    const openAgain = await screen.findByRole('button', { name: 'Открыть снова' });

    expect(afterRefetch).toEqual({
      closed: [false],
      notices: 1,
      lessonLinks: 2,
      loadFailures: 0,
      focused: held.pressed,
    });
    expect(openAgain).toBe(held.pressed);
    expect(document.activeElement).toBe(held.pressed);
    expect(shown.tabs.opened[0]?.visited).toHaveLength(1);
  });
});
