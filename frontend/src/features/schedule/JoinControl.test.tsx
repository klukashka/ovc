import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, type ManualScheduler, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext, useSignedInAccount } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { LessonDetail } from './api.ts';
import { type JoinLesson, JoinControl } from './JoinControl.tsx';
import { type OpenRoomTab, RoomTabContext } from './roomTab.ts';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const password = 'a long password';
const tutorEmail = 'maria@example.org';
const studentEmail = 'anna@example.org';
const minute = 60_000;
const waitingText = 'Комната откроется, когда подключится преподаватель';

const firstDetail: LessonDetail = {
  id: '00000000-0000-4000-8000-000000000014',
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
  embed_allowed: true,
};

const secondDetail: LessonDetail = {
  ...firstDetail,
  id: '00000000-0000-4000-8000-000000000015',
  starts_at: '2026-11-03T15:02:00Z',
  ends_at: '2026-11-03T16:02:00Z',
};

function joinLessonOf(detail: LessonDetail, change: Partial<JoinLesson> = {}): JoinLesson {
  return {
    id: detail.id,
    status: detail.status,
    starts_at: detail.starts_at,
    ends_at: detail.ends_at,
    room_open: detail.room_open,
    ...change,
  };
}

const first = joinLessonOf(firstDetail);
const second = joinLessonOf(secondDetail);
const firstLive = joinLessonOf(firstDetail, { status: 'live', room_open: true });

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

  only(): RecordedTab {
    const [tab] = this.opened;
    if (tab === undefined || this.opened.length !== 1) {
      throw new Error(`expected one opened tab, found ${String(this.opened.length)}`);
    }
    return tab;
  }
}

interface ControlRow {
  readonly lesson: JoinLesson;
  readonly showsOpensAt: boolean;
}

function Controls({ rows }: { readonly rows: readonly ControlRow[] }): ReactElement {
  const me = useSignedInAccount();
  return (
    <ul>
      {rows.map((row, index) => (
        <li key={index}>
          <JoinControl key={row.lesson.id} lesson={row.lesson} me={me} showsOpensAt={row.showsOpensAt} />
        </li>
      ))}
    </ul>
  );
}

interface ShownControls {
  readonly server: FakeAuthServer;
  readonly tabs: RoomTabs;
  readonly scheduler: ManualScheduler;
  readonly queryClient: QueryClient;
  readonly show: (rows: readonly ControlRow[]) => void;
  readonly container: HTMLElement;
}

interface ControlsSetup {
  readonly role?: 'tutor' | 'student';
  readonly rows?: readonly ControlRow[];
  readonly minutesBeforeStart?: number;
}

function rowOf(lesson: JoinLesson, showsOpensAt = true): ControlRow {
  return { lesson, showsOpensAt };
}

async function showControls({
  role = 'tutor',
  rows = [rowOf(first)],
  minutesBeforeStart = 5,
}: ControlsSetup = {}): Promise<ShownControls> {
  const server = new FakeAuthServer();
  const email = role === 'tutor' ? tutorEmail : studentEmail;
  server.addAccount({ id: `${role}-1`, name: 'Участник', email, password, role });
  server.addLesson(firstDetail, [email]);
  server.addLesson(secondDetail, [email]);
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(email, password);
  const scheduler = manualScheduler();
  scheduler.suspend(Date.parse(firstDetail.starts_at) - minutesBeforeStart * minute);
  const i18n = await createI18n('ru');
  const tabs = new RoomTabs();
  const queryClient = new QueryClient();
  const tree = (shown: readonly ControlRow[]): ReactElement => (
    <SchedulerContext value={scheduler}>
      <RoomTabContext value={tabs.open}>
        <SessionContext value={session}>
          <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
            <QueryClientProvider client={queryClient}>
              <I18nextProvider i18n={i18n}>
                <main>
                  <h1 tabIndex={-1}>Сегодня</h1>
                  <Controls rows={shown} />
                </main>
              </I18nextProvider>
            </QueryClientProvider>
          </ApiClientContext>
        </SessionContext>
      </RoomTabContext>
    </SchedulerContext>
  );
  const view = render(tree(rows));
  act(() => {
    scheduler.pass(0);
  });
  return {
    server,
    tabs,
    scheduler,
    queryClient,
    container: view.container,
    show: (shown): void => {
      view.rerender(tree(shown));
    },
  };
}

function joinWrites(server: FakeAuthServer): readonly string[] {
  return server.sentAddresses.map((address) => address.pathname).filter((path) => path.endsWith('/join'));
}

function button(name: string): HTMLElement {
  return screen.getByRole('button', { name });
}

async function settled(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
  });
}

describe('what the control offers', () => {
  test('a tutor sees when the room opens before the window, Join lesson in it and Rejoin while live', async () => {
    await showControls({ minutesBeforeStart: 30 });
    const beforeWindow = screen.getByText(/^Комната откроется в /).textContent;
    const buttonBeforeWindow = screen.queryByRole('button');
    cleanup();
    await showControls();
    const inWindow = button('Подключиться к уроку');
    cleanup();
    await showControls({ rows: [rowOf(firstLive)] });

    expect(beforeWindow).toBe('Комната откроется в 17:50');
    expect(buttonBeforeWindow).toBeNull();
    expect(inWindow).toBeTruthy();
    expect(button('Подключиться снова')).toBeTruthy();
  });

  test('a student waits in the window and while the room is closed, and sees Join only while it is open', async () => {
    await showControls({ role: 'student' });
    const inWindow = screen.getByText(waitingText);
    const buttonInWindow = screen.queryByRole('button');
    cleanup();
    await showControls({ role: 'student', rows: [rowOf({ ...firstLive, room_open: false })] });
    const closedRoom = screen.getByText(waitingText);
    const buttonWhileClosed = screen.queryByRole('button');
    cleanup();
    await showControls({ role: 'student', rows: [rowOf(firstLive)] });

    expect(inWindow).toBeTruthy();
    expect(buttonInWindow).toBeNull();
    expect(closedRoom).toBeTruthy();
    expect(buttonWhileClosed).toBeNull();
    expect(button('Подключиться')).toBeTruthy();
    expect(screen.queryByText(waitingText)).toBeNull();
  });

  test('cancelled and ended lessons show nothing, and the opening time shows only where it is asked for', async () => {
    const { container } = await showControls({
      rows: [
        rowOf({ ...first, status: 'cancelled' }),
        rowOf({ ...firstLive, id: secondDetail.id, status: 'ended', room_open: false }),
      ],
    });
    const shownForGoneLessons = container.querySelectorAll('[data-part="join"]').length;
    cleanup();
    await showControls({ minutesBeforeStart: 30, rows: [rowOf(first, false)] });

    expect(shownForGoneLessons).toBe(0);
    expect(screen.queryByText(/^Комната откроется в /)).toBeNull();
  });

  test('the window opening turns the opening time into Join lesson without a new lesson read', async () => {
    const shown = await showControls({ minutesBeforeStart: 11 });
    const beforeWindow = screen.queryByRole('button', { name: 'Подключиться к уроку' });

    act(() => {
      shown.scheduler.pass(minute);
    });

    expect(beforeWindow).toBeNull();
    expect(button('Подключиться к уроку')).toBeTruthy();
    expect(screen.queryByText(/^Комната откроется в /)).toBeNull();
    expect(shown.server.sentAddresses.filter((address) => address.pathname.includes('/lessons/'))).toEqual([]);
  });
});

describe('joining through the control', () => {
  test('one button goes through Joining… to Open again, keeps focus and opens one tab with the link', async () => {
    const shown = await showControls();
    const paused = shown.server.pauseLessonWrites();
    const pressed = button('Подключиться к уроку');
    pressed.focus();

    fireEvent.click(pressed);
    const whileJoining = { busy: pressed.getAttribute('aria-busy'), name: pressed.textContent };
    fireEvent.click(pressed);
    await paused.received;
    paused.resume();

    expect(whileJoining).toEqual({ busy: 'true', name: 'Подключение…' });
    expect(await screen.findByRole('button', { name: 'Открыть снова' })).toBe(pressed);
    expect(document.activeElement).toBe(pressed);
    expect(joinWrites(shown.server)).toHaveLength(1);
    expect(shown.tabs.only().visited).toHaveLength(1);
    expect(shown.tabs.only().visited[0]).toContain('room.example');
    expect(shown.container.querySelector('iframe')).toBeNull();
    expect(JSON.stringify(shown.queryClient.getQueryCache().getAll())).not.toContain('room.example');
  });

  test('a tutor refused outside the join window sees the reason, the tab closes and focus stays', async () => {
    const shown = await showControls();
    shown.server.refuseNextLessonWrite(409, 'outside_join_window');
    const pressed = button('Подключиться к уроку');
    pressed.focus();

    fireEvent.click(pressed);
    const alert = await screen.findByRole('alert');

    expect(alert.textContent).toBe('Войти в урок сейчас нельзя.');
    expect(shown.tabs.only().closed).toBe(true);
    expect(shown.tabs.only().visited).toEqual([]);
    expect(document.activeElement).toBe(pressed);
  });

  test('a student told the lesson is not live waits without an alert once the lesson data says so', async () => {
    const shown = await showControls({ role: 'student', rows: [rowOf(firstLive)] });
    shown.server.refuseNextLessonWrite(409, 'lesson_not_live');

    fireEvent.click(button('Подключиться'));
    await waitFor(() => {
      expect(shown.tabs.only().closed).toBe(true);
    });
    shown.show([rowOf({ ...firstLive, room_open: false })]);

    expect(await screen.findByText(waitingText)).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByRole('button')).toBeNull();
    expect(shown.tabs.only().visited).toEqual([]);
  });

  test('a student out of scope loses the button for that lesson, the tab closes and focus reaches the heading', async () => {
    const shown = await showControls({ role: 'student', rows: [rowOf(firstLive)] });
    shown.server.refuseNextLessonWrite(404, 'not_found');
    const pressed = button('Подключиться');
    pressed.focus();

    fireEvent.click(pressed);
    const alert = await screen.findByRole('alert');
    shown.show([rowOf({ ...firstLive, room_open: true })]);

    expect(alert.textContent).toBe('Вы не можете подключиться к этому уроку');
    expect(screen.queryByRole('button')).toBeNull();
    expect(shown.tabs.only().closed).toBe(true);
    expect(shown.tabs.only().visited).toEqual([]);
    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
    });
  });

  test('a rate-limited join says when to try again and closes the tab', async () => {
    const shown = await showControls();
    shown.server.refuseNextLessonWrite(429, 'rate_limited', 90);

    fireEvent.click(button('Подключиться к уроку'));
    const alert = await screen.findByRole('alert');

    expect(alert.textContent).toBe('Слишком много попыток, попробуйте через 2 минуты.');
    expect(shown.tabs.only().closed).toBe(true);
    expect(shown.tabs.only().visited).toEqual([]);
    expect(button('Подключиться к уроку')).toBeTruthy();
  });
});

describe('a held join that loses its control', () => {
  test('moving the control to another lesson closes the tab, which never receives the link', async () => {
    const shown = await showControls();
    const paused = shown.server.pauseLessonWrites();
    fireEvent.click(button('Подключиться к уроку'));
    await paused.received;

    shown.show([rowOf(second)]);
    const closedOnChange = shown.tabs.only().closed;
    paused.resume();
    await settled();

    expect(closedOnChange).toBe(true);
    expect(shown.tabs.only().visited).toEqual([]);
    expect(button('Подключиться к уроку').getAttribute('aria-busy')).toBe('false');
  });

  test('a cancellation of the lesson closes the tab, which never receives the link', async () => {
    const shown = await showControls();
    const paused = shown.server.pauseLessonWrites();
    fireEvent.click(button('Подключиться к уроку'));
    await paused.received;

    shown.show([rowOf({ ...first, status: 'cancelled' })]);
    const closedOnCancel = shown.tabs.only().closed;
    paused.resume();
    await settled();

    expect(closedOnCancel).toBe(true);
    expect(shown.tabs.only().visited).toEqual([]);
    expect(screen.queryByRole('button')).toBeNull();
  });

  test('an ending of the lesson closes the held tab, which never receives the link', async () => {
    const shown = await showControls();
    const paused = shown.server.pauseLessonWrites();
    fireEvent.click(button('Подключиться к уроку'));
    await paused.received;

    shown.show([rowOf({ ...first, status: 'ended' })]);
    const closedOnEnd = shown.tabs.only().closed;
    paused.resume();
    await settled();

    expect(closedOnEnd).toBe(true);
    expect(shown.tabs.only().visited).toEqual([]);
    expect(screen.queryByRole('button')).toBeNull();
  });

  test('a lesson that ends while its control has focus moves focus to the page heading', async () => {
    const shown = await showControls();
    button('Подключиться к уроку').focus();

    shown.show([rowOf({ ...first, status: 'ended' })]);

    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
    });
  });

  test('an unloaded page closes the held tab of every control', async () => {
    const shown = await showControls({ rows: [rowOf(first), rowOf(joinLessonOf(secondDetail), false)] });
    const paused = shown.server.pauseLessonWrites();
    for (const pressed of screen.getAllByRole('button', { name: 'Подключиться к уроку' })) {
      fireEvent.click(pressed);
    }
    await paused.received;
    await waitFor(() => {
      expect(joinWrites(shown.server)).toHaveLength(2);
    });

    act(() => {
      window.dispatchEvent(new Event('pagehide'));
    });
    const closedOnPageHide = shown.tabs.opened.map((tab) => tab.closed);
    paused.resume();
    await settled();

    expect(closedOnPageHide).toEqual([true, true]);
    expect(shown.tabs.opened.map((tab) => tab.visited)).toEqual([[], []]);
    expect(await screen.findAllByRole('button', { name: 'Подключиться к уроку' })).toHaveLength(2);
  });
});
