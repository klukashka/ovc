import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { MediaQueriesContext, fixedMediaQueries, wideLayoutQuery } from '../../app/context.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, type ManualScheduler, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { LessonView } from './api.ts';
import { lessonKey } from './keys.ts';
import { LessonPage } from './LessonPage.tsx';
import { embedLoadHintMilliseconds } from './LessonRoom.tsx';
import { type OpenRoomTab, RoomTabContext } from './roomTab.ts';

async function focusMovesTo(target: () => Element): Promise<void> {
  await waitFor(() => {
    expect(document.activeElement).toBe(target());
  });
}

async function focusMovesToTheHeading(): Promise<void> {
  await focusMovesTo(() => screen.getByRole('heading', { level: 1 }));
}

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const password = 'a long password';
const tutorEmail = 'maria@example.org';
const studentEmail = 'anna@example.org';
const minute = 60_000;

const scheduledLesson: LessonView = {
  id: '00000000-0000-4000-8000-000000000004',
  title: 'Разговорная практика',
  description: null,
  starts_at: '2026-11-03T15:00:00Z',
  ends_at: '2026-11-03T16:00:00Z',
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
  materials: [],
  issues: [],
};

const liveLesson: LessonView = { ...scheduledLesson, status: 'live', took_place: true, room_open: true };
const secondLesson: LessonView = { ...scheduledLesson, id: '00000000-0000-4000-8000-000000000005', title: 'Второй' };

class RecordedTab {
  readonly visited: string[] = [];
  closed = false;
}

class RoomTabs {
  readonly opened: RecordedTab[] = [];
  refuses = false;

  readonly open: OpenRoomTab = () => {
    if (this.refuses) {
      return null;
    }
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

  only(): RecordedTab {
    const [tab] = this.opened;
    if (tab === undefined || this.opened.length !== 1) {
      throw new Error(`expected one opened tab, found ${String(this.opened.length)}`);
    }
    return tab;
  }
}

interface ShownRoom {
  readonly server: FakeAuthServer;
  readonly tabs: RoomTabs;
  readonly scheduler: ManualScheduler;
  readonly queryClient: QueryClient;
  readonly router: ReturnType<typeof createMemoryRouter>;
}

interface RoomSetup {
  readonly role?: 'tutor' | 'student';
  readonly lesson?: LessonView;
  readonly wide?: boolean;
  readonly minutesBeforeStart?: number;
}

async function showRoom({
  role = 'tutor',
  lesson = scheduledLesson,
  wide = true,
  minutesBeforeStart = 5,
}: RoomSetup = {}): Promise<ShownRoom> {
  const server = new FakeAuthServer();
  const email = role === 'tutor' ? tutorEmail : studentEmail;
  server.addAccount({ id: `${role}-1`, name: 'Участник', email, password, role });
  server.addLesson(lesson, [email]);
  server.addLesson(secondLesson, [email]);
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(email, password);
  const scheduler = manualScheduler();
  scheduler.suspend(Date.parse(lesson.starts_at) - minutesBeforeStart * minute);
  const i18n = await createI18n('ru');
  const router = createMemoryRouter(
    [
      {
        path: '/lessons/:lessonId',
        element: (
          <main>
            <LessonPage />
          </main>
        ),
      },
      { path: '/elsewhere', element: <main>Elsewhere</main> },
    ],
    { initialEntries: [`/lessons/${lesson.id}`] },
  );
  const tabs = new RoomTabs();
  const queryClient = new QueryClient();
  render(
    <SchedulerContext value={scheduler}>
      <RoomTabContext value={tabs.open}>
        <MediaQueriesContext value={fixedMediaQueries(wide ? [wideLayoutQuery] : [])}>
          <SessionContext value={session}>
            <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
              <QueryClientProvider client={queryClient}>
                <I18nextProvider i18n={i18n}>
                  <RouterProvider router={router} />
                </I18nextProvider>
              </QueryClientProvider>
            </ApiClientContext>
          </SessionContext>
        </MediaQueriesContext>
      </RoomTabContext>
    </SchedulerContext>,
  );
  await screen.findByRole('heading', { level: 1, name: lesson.title });
  act(() => {
    scheduler.pass(0);
  });
  return { server, tabs, scheduler, queryClient, router };
}

function roomAddressesCached(queryClient: QueryClient): boolean {
  const cached = JSON.stringify(
    queryClient
      .getQueryCache()
      .getAll()
      .map((query) => query.state.data),
  );
  return cached.includes('room.example');
}

function lessonWrites(server: FakeAuthServer): readonly string[] {
  return server.sentAddresses
    .map((address) => address.pathname)
    .filter((path) => /\/(?:join|end|restart-room)$/.test(path));
}

async function refetched(shown: ShownRoom): Promise<void> {
  await act(async () => {
    await shown.queryClient.invalidateQueries();
  });
}

function joinButton(name: string): HTMLElement {
  return screen.getByRole('button', { name });
}

describe('joining through a new tab', () => {
  test('the tab opens in the click before the answer and receives the link afterwards', async () => {
    const shown = await showRoom();
    const paused = shown.server.pauseLessonWrites();

    fireEvent.click(joinButton('Подключиться к уроку'));
    const openedInTheClick = shown.tabs.only().visited.length;
    await paused.received;
    paused.resume();

    expect(openedInTheClick).toBe(0);
    await waitFor(() => {
      expect(shown.tabs.only().visited).toHaveLength(1);
    });
    expect(shown.tabs.only().closed).toBe(false);
    expect(await screen.findByText('Урок открыт в другой вкладке')).toBeTruthy();
  });

  test('the pressed button keeps focus through joining and becomes Open again', async () => {
    const shown = await showRoom();
    const paused = shown.server.pauseLessonWrites();
    const button = joinButton('Подключиться к уроку');
    button.focus();

    fireEvent.click(button);
    const whileJoining = button.getAttribute('aria-busy');
    fireEvent.click(button);
    await paused.received;
    paused.resume();

    expect(whileJoining).toBe('true');
    expect(await screen.findByRole('button', { name: 'Открыть снова' })).toBe(button);
    expect(document.activeElement).toBe(button);
    expect(lessonWrites(shown.server)).toHaveLength(1);
  });

  test('Open again asks for a new link and opens a new tab', async () => {
    const shown = await showRoom();
    fireEvent.click(joinButton('Подключиться к уроку'));
    const again = await screen.findByRole('button', { name: 'Открыть снова' });

    fireEvent.click(again);
    await waitFor(() => {
      expect(shown.tabs.opened[1]?.visited).toHaveLength(1);
    });

    const [first, second] = shown.tabs.opened;
    expect(second?.visited[0]).not.toBe(first?.visited[0]);
    expect(lessonWrites(shown.server)).toHaveLength(2);
  });

  test('the link stays out of the query cache', async () => {
    const shown = await showRoom();

    fireEvent.click(joinButton('Подключиться к уроку'));
    await screen.findByText('Урок открыт в другой вкладке');
    await refetched(shown);

    expect(shown.tabs.only().visited[0]).toContain('room.example');
    expect(roomAddressesCached(shown.queryClient)).toBe(false);
  });

  test('a provider refusal closes the tab, says so and keeps focus on the button', async () => {
    const shown = await showRoom();
    shown.server.refuseNextLessonWrite(503, 'provider_unavailable');
    const button = joinButton('Подключиться к уроку');
    button.focus();

    fireEvent.click(button);
    const alert = await screen.findByRole('alert');

    expect(alert.textContent).toBe('Видеосвязь недоступна, попробуйте ещё раз');
    expect(shown.tabs.only().closed).toBe(true);
    expect(shown.tabs.only().visited).toEqual([]);
    expect(document.activeElement).toBe(button);
  });

  test('a refused pop-up sends no request and explains what to do', async () => {
    const shown = await showRoom();
    shown.tabs.refuses = true;

    fireEvent.click(joinButton('Подключиться к уроку'));
    const alert = await screen.findByRole('alert');

    expect(alert.textContent).toContain('Браузер заблокировал новую вкладку');
    expect(lessonWrites(shown.server)).toEqual([]);
  });

  test('a tab the user closed before the answer returns to the join button without a message', async () => {
    const shown = await showRoom();
    const paused = shown.server.pauseLessonWrites();
    const button = joinButton('Подключиться к уроку');
    button.focus();
    fireEvent.click(button);
    await paused.received;
    shown.tabs.only().closed = true;

    paused.resume();

    expect(await screen.findByRole('button', { name: 'Подключиться снова' })).toBe(button);
    expect(document.activeElement).toBe(button);
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByText('Урок открыт в другой вкладке')).toBeNull();
    expect(shown.tabs.only().visited).toEqual([]);
  });

  test('an answer with a plain http address outside loopback never reaches the tab', async () => {
    const shown = await showRoom();
    shown.server.nextRoomAddress = 'http://room.example/join';

    fireEvent.click(joinButton('Подключиться к уроку'));
    const alert = await screen.findByRole('alert');

    expect(alert.textContent).toBe('Видеосвязь недоступна, попробуйте ещё раз');
    expect(shown.tabs.only().visited).toEqual([]);
    expect(shown.tabs.only().closed).toBe(true);
  });

  test('an answer with an address that is not a web address never reaches the tab', async () => {
    const shown = await showRoom();
    shown.server.nextRoomAddress = 'javascript:alert(1)';

    fireEvent.click(joinButton('Подключиться к уроку'));
    await screen.findByRole('alert');

    expect(shown.tabs.only().visited).toEqual([]);
    expect(shown.tabs.only().closed).toBe(true);
  });
});

describe('leaving while the join is in flight', () => {
  test('leaving the page closes the tab and drops the late answer', async () => {
    const shown = await showRoom();
    const paused = shown.server.pauseLessonWrites();
    fireEvent.click(joinButton('Подключиться к уроку'));
    await paused.received;

    await act(async () => {
      await shown.router.navigate('/elsewhere');
    });
    const closedOnLeaving = shown.tabs.only().closed;
    await act(async () => {
      paused.resume();
      await Promise.resolve();
    });

    expect(closedOnLeaving).toBe(true);
    expect(shown.tabs.only().visited).toEqual([]);
  });

  test('moving to another lesson closes the tab and starts that lesson outside the room', async () => {
    const shown = await showRoom();
    const paused = shown.server.pauseLessonWrites();
    fireEvent.click(joinButton('Подключиться к уроку'));
    await paused.received;

    await act(async () => {
      await shown.router.navigate(`/lessons/${secondLesson.id}`);
    });
    await screen.findByRole('heading', { level: 1, name: secondLesson.title });
    await act(async () => {
      paused.resume();
      await Promise.resolve();
    });

    expect(shown.tabs.only().closed).toBe(true);
    expect(shown.tabs.only().visited).toEqual([]);
    expect(screen.queryByText('Урок открыт в другой вкладке')).toBeNull();
  });

  test('unmounting, as an account change does, closes the tab and drops the late answer', async () => {
    const shown = await showRoom();
    const paused = shown.server.pauseLessonWrites();
    fireEvent.click(joinButton('Подключиться к уроку'));
    await paused.received;

    cleanup();
    const closedOnUnmount = shown.tabs.only().closed;
    await act(async () => {
      paused.resume();
      await Promise.resolve();
    });

    expect(closedOnUnmount).toBe(true);
    expect(shown.tabs.only().visited).toEqual([]);
  });

  test('an unloaded page (pagehide) closes the tab and returns to the join button', async () => {
    const shown = await showRoom();
    const paused = shown.server.pauseLessonWrites();
    fireEvent.click(joinButton('Подключиться к уроку'));
    await paused.received;

    act(() => {
      window.dispatchEvent(new Event('pagehide'));
    });
    paused.resume();

    expect(shown.tabs.only().closed).toBe(true);
    expect(await screen.findByRole('button', { name: 'Подключиться к уроку' })).toBeTruthy();
    expect(shown.tabs.only().visited).toEqual([]);
  });
});

describe('losing the lesson while the join is in flight', () => {
  test('a lesson that answers not found closes the tab and drops the late answer', async () => {
    const shown = await showRoom({ lesson: liveLesson });
    const paused = shown.server.pauseLessonWrites();
    fireEvent.click(joinButton('Подключиться снова'));
    await paused.received;
    shown.server.removeLessonMember(liveLesson.id, tutorEmail);

    await act(async () => {
      await shown.queryClient.invalidateQueries({ queryKey: lessonKey(liveLesson.id) });
    });
    await waitFor(() => {
      expect(shown.tabs.only().closed).toBe(true);
    });
    await act(async () => {
      paused.resume();
      await Promise.resolve();
    });

    expect(shown.tabs.only().visited).toEqual([]);
  });
});

describe('the lesson controls when the lesson goes live early', () => {
  test('focus on Edit moves to the heading when the lesson goes live before its start', async () => {
    const shown = await showRoom();
    joinButton('Изменить урок').focus();
    shown.server.changeLesson(scheduledLesson.id, { status: 'live', took_place: true, room_open: true });

    await act(async () => {
      await shown.queryClient.invalidateQueries({ queryKey: lessonKey(scheduledLesson.id) });
    });

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Изменить урок' })).toBeNull();
    });
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });

  test('focus on Lesson actions stays on it when the lesson goes live before its start', async () => {
    const shown = await showRoom();
    const menu = joinButton('Действия с уроком');
    menu.focus();
    shown.server.changeLesson(scheduledLesson.id, { status: 'live', took_place: true, room_open: true });

    await act(async () => {
      await shown.queryClient.invalidateQueries({ queryKey: lessonKey(scheduledLesson.id) });
    });

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Изменить урок' })).toBeNull();
    });
    expect(document.activeElement).toBe(joinButton('Действия с уроком'));
    expect(joinButton('Действия с уроком')).toBe(menu);
  });
});

describe('the live lesson actions menu', () => {
  test('focus on Lesson actions moves to the heading when the lesson is ended elsewhere', async () => {
    const shown = await showRoom({ lesson: liveLesson });
    joinButton('Действия с уроком').focus();
    shown.server.changeLesson(liveLesson.id, { status: 'ended', room_open: false });

    await act(async () => {
      await shown.queryClient.invalidateQueries({ queryKey: lessonKey(liveLesson.id) });
    });

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Действия с уроком' })).toBeNull();
    });
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });
});

describe('what each role sees', () => {
  test('a student before the tutor joins waits and the Join button appears on the live change', async () => {
    const shown = await showRoom({ role: 'student' });
    const waiting = screen.getByText('Комната откроется, когда подключится преподаватель');
    const noButton = screen.queryByRole('button', { name: 'Подключиться' });

    shown.server.changeLesson(scheduledLesson.id, { status: 'live', room_open: true, took_place: true });
    await refetched(shown);

    expect(waiting).toBeTruthy();
    expect(noButton).toBeNull();
    expect(await screen.findByRole('button', { name: 'Подключиться' })).toBeTruthy();
    expect(screen.queryByText('Комната откроется, когда подключится преподаватель')).toBeNull();
  });

  test('a live lesson whose room closed offers the tutor Rejoin and tells the student to wait', async () => {
    const closed = { ...liveLesson, room_open: false };

    await showRoom({ lesson: closed });
    const rejoin = joinButton('Подключиться снова');
    cleanup();
    await showRoom({ role: 'student', lesson: closed });

    expect(rejoin).toBeTruthy();
    expect(screen.getByText('Комната откроется, когда подключится преподаватель')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Подключиться' })).toBeNull();
  });

  test('before the window both roles see when the room opens', async () => {
    await showRoom({ minutesBeforeStart: 30 });
    const forTutor = screen.getByText(/^Комната откроется в /);
    cleanup();
    await showRoom({ role: 'student', minutesBeforeStart: 30 });

    expect(forTutor).toBeTruthy();
    expect(screen.getByText(/^Комната откроется в /)).toBeTruthy();
  });

  test('an ended lesson that ran shows Ended, and one nobody joined shows Did not take place', async () => {
    await showRoom({ lesson: { ...liveLesson, status: 'ended', room_open: false } });
    const ran = screen.getByText('Завершён');
    cleanup();
    await showRoom({ lesson: { ...scheduledLesson, status: 'ended' } });

    expect(ran).toBeTruthy();
    expect(screen.getByText('Не состоялся')).toBeTruthy();
    expect(screen.queryByRole('region', { name: 'Видеокомната' })).toBeNull();
  });

  test('a student refused as out of scope loses the button and focus moves to the heading', async () => {
    const shown = await showRoom({ role: 'student', lesson: liveLesson });
    shown.server.refuseNextLessonWrite(404, 'not_found');
    const button = joinButton('Подключиться');
    button.focus();

    fireEvent.click(button);
    const alert = await screen.findByRole('alert');

    expect(alert.textContent).toBe('Вы не можете подключиться к этому уроку');
    expect(screen.queryByRole('button', { name: 'Подключиться' })).toBeNull();
    await focusMovesToTheHeading();
  });

  test('a student told the room is not live sees the waiting state without an alert', async () => {
    const shown = await showRoom({ role: 'student', lesson: liveLesson });
    shown.server.refuseNextLessonWrite(409, 'lesson_not_live');
    shown.server.changeLesson(liveLesson.id, { room_open: false });
    const button = joinButton('Подключиться');
    button.focus();

    fireEvent.click(button);

    expect(await screen.findByText('Комната откроется, когда подключится преподаватель')).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
    await focusMovesToTheHeading();
    expect(shown.tabs.only().closed).toBe(true);
  });

  test('the in-tab state ends when the room closes', async () => {
    const shown = await showRoom({ role: 'student', lesson: liveLesson });
    fireEvent.click(joinButton('Подключиться'));
    await screen.findByText('Урок открыт в другой вкладке');
    await refetched(shown);

    shown.server.changeLesson(liveLesson.id, { room_open: false });
    await refetched(shown);

    expect(await screen.findByText('Комната откроется, когда подключится преподаватель')).toBeTruthy();
    expect(screen.queryByText('Урок открыт в другой вкладке')).toBeNull();
  });
});

describe('the embedded room', () => {
  const embeddable: LessonView = { ...liveLesson, embed_allowed: true };

  async function embeddedRoom(): Promise<ShownRoom & { readonly frame: HTMLElement }> {
    const shown = await showRoom({ lesson: embeddable });
    fireEvent.click(joinButton('Подключиться снова'));
    const frame = await screen.findByTitle(embeddable.title);
    await waitFor(() => {
      expect(document.activeElement).toBe(frame);
    });
    return { ...shown, frame };
  }

  test('opens without a tab, titled by the lesson, with its permissions, and takes focus', async () => {
    const shown = await embeddedRoom();

    expect(shown.tabs.opened).toEqual([]);
    expect(shown.frame.tagName).toBe('IFRAME');
    expect(shown.frame.getAttribute('allow')).toBe('camera; microphone; display-capture; fullscreen; autoplay');
    expect(shown.frame.getAttribute('referrerpolicy')).toBe('no-referrer');
    await focusMovesTo(() => shown.frame);
    expect(joinButton('Открыть в новой вкладке')).toBeTruthy();
    expect(roomAddressesCached(shown.queryClient)).toBe(false);
  });

  test('a narrow window takes the new-tab path even when embedding is allowed', async () => {
    const shown = await showRoom({ lesson: embeddable, wide: false });

    fireEvent.click(joinButton('Подключиться снова'));
    await screen.findByText('Урок открыт в другой вкладке');

    expect(shown.tabs.only().visited).toHaveLength(1);
    expect(screen.queryByTitle(embeddable.title)).toBeNull();
  });

  test('a frame without load after 15 seconds highlights the new-tab button, and load never hides it', async () => {
    const { scheduler, frame } = await embeddedRoom();

    act(() => {
      scheduler.pass(embedLoadHintMilliseconds);
    });
    const hint = screen.getByRole('button', { name: 'Возникли проблемы? Откройте в новой вкладке' });
    const highlighted = hint.className;
    fireEvent.load(frame);

    expect(highlighted).toContain('bg-accent');
    expect(joinButton('Открыть в новой вкладке')).toBe(hint);
  });

  test('a frame that loads in time never shows the hint', async () => {
    const { scheduler, frame } = await embeddedRoom();

    fireEvent.load(frame);
    act(() => {
      scheduler.pass(embedLoadHintMilliseconds);
    });

    expect(screen.queryByRole('button', { name: 'Возникли проблемы? Откройте в новой вкладке' })).toBeNull();
  });

  test('switching to a new tab removes the frame first and focus lands on the room button', async () => {
    const shown = await embeddedRoom();
    const paused = shown.server.pauseLessonWrites();
    const button = joinButton('Открыть в новой вкладке');
    button.focus();

    fireEvent.click(button);
    const frameWhileJoining = screen.queryByTitle(embeddable.title);
    const focusedWhileJoining = document.activeElement;
    await paused.received;
    paused.resume();

    expect(frameWhileJoining).toBeNull();
    expect(focusedWhileJoining).toBe(screen.getByRole('button', { name: /Подключение|Открыть снова/ }));
    await waitFor(() => {
      expect(shown.tabs.only().visited).toHaveLength(1);
    });
  });

  test('a blocked pop-up from Open in new tab keeps the frame, explains it and keeps focus on the button', async () => {
    const shown = await embeddedRoom();
    shown.tabs.refuses = true;
    const button = joinButton('Открыть в новой вкладке');
    button.focus();

    fireEvent.click(button);
    const alert = await screen.findByRole('alert');

    expect(alert.textContent).toContain('Браузер заблокировал новую вкладку');
    expect(screen.getByTitle(embeddable.title)).toBe(shown.frame);
    expect(document.activeElement).toBe(button);
    expect(lessonWrites(shown.server)).toEqual([`/api/v1/lessons/${embeddable.id}/join`]);
  });

  test('a lesson change to ended removes the frame and focus moves to the heading', async () => {
    const shown = await embeddedRoom();
    await refetched(shown);
    const toggle = joinButton('Режим фокуса');
    toggle.focus();

    shown.server.changeLesson(embeddable.id, { status: 'ended', room_open: false });
    await refetched(shown);

    await waitFor(() => {
      expect(screen.queryByTitle(embeddable.title)).toBeNull();
    });
    await focusMovesToTheHeading();
  });

  test('leaving the page removes the frame and cancels the hint timer', async () => {
    const shown = await embeddedRoom();
    const pendingWithRoom = shown.scheduler.pendingDelays().length;

    await act(async () => {
      await shown.router.navigate('/elsewhere');
    });

    expect(screen.queryByTitle(embeddable.title)).toBeNull();
    expect(shown.scheduler.pendingDelays().length).toBeLessThan(pendingWithRoom);
  });

  test('a join answer that no longer allows embedding asks to try again', async () => {
    const shown = await showRoom({ lesson: embeddable });
    shown.server.refuseEmbedding(embeddable.id);

    fireEvent.click(joinButton('Подключиться снова'));
    const alert = await screen.findByRole('alert');

    expect(alert.textContent).toBe('Видеосвязь недоступна, попробуйте ещё раз');
    expect(screen.queryByTitle(embeddable.title)).toBeNull();
    expect(shown.tabs.opened).toEqual([]);
  });
});

describe('ending the lesson and restarting the room', () => {
  test('a student has no Lesson actions menu and sees neither End lesson nor Restart the room', async () => {
    await showRoom({ role: 'student', lesson: liveLesson });

    const menu = screen.queryByRole('button', { name: 'Действия с уроком' });
    const end = screen.queryByRole('button', { name: 'Завершить урок' });
    const restart = screen.queryByRole('button', { name: 'Перезапустить комнату' });

    expect(menu).toBeNull();
    expect(end).toBeNull();
    expect(restart).toBeNull();
  });
  test('while a join is pending Lesson actions is unavailable and opens no menu', async () => {
    const shown = await showRoom({ lesson: liveLesson });
    const paused = shown.server.pauseLessonWrites();
    fireEvent.click(joinButton('Подключиться снова'));
    await paused.received;

    fireEvent.click(joinButton('Действия с уроком'));

    expect(joinButton('Действия с уроком').getAttribute('aria-disabled')).toBe('true');
    expect(joinButton('Действия с уроком').getAttribute('aria-expanded')).toBe('false');
    expect(screen.queryByRole('button', { name: 'Завершить урок' })).toBeNull();
    expect(lessonWrites(shown.server)).toEqual([`/api/v1/lessons/${liveLesson.id}/join`]);
    await act(async () => {
      paused.resume();
      await Promise.resolve();
    });
  });

  test('a refusal disappears when a live change takes the join button away', async () => {
    const shown = await showRoom({ role: 'student', lesson: liveLesson });
    shown.server.refuseNextLessonWrite(503, 'provider_unavailable');
    fireEvent.click(joinButton('Подключиться'));
    await screen.findByText('Видеосвязь недоступна, попробуйте ещё раз');

    shown.server.changeLesson(liveLesson.id, { room_open: false });
    await refetched(shown);

    expect(await screen.findByText('Комната откроется, когда подключится преподаватель')).toBeTruthy();
    expect(screen.queryByText('Видеосвязь недоступна, попробуйте ещё раз')).toBeNull();
  });
});
