import { QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import type { RouteHandle } from '../../app/handles.ts';
import { FollowingQueryClient } from '../../app/queryClient.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import type { components } from '../../lib/api/schema.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { CalendarItem, LessonDetail } from '../schedule/api.ts';
import { type OpenRoomTab, RoomTabContext } from '../schedule/roomTab.ts';
import type { Language } from '../../lib/i18n/languages.ts';
import { studentHomesKey } from './api.ts';
import { HomePage } from './HomePage.tsx';

afterEach(() => {
  cleanup();
});

type MyTutor = components['schemas']['MyTutor'];
type StudentAssignmentItem = components['schemas']['StudentAssignmentItem'];

const origin = 'https://ovc.example';
const password = 'a long password';
const studentEmail = 'anna@example.org';
const mariaLink = '00000000-0000-4000-8000-000000000071';
const pavelLink = '00000000-0000-4000-8000-000000000072';
const otherLink = '00000000-0000-4000-8000-000000000079';
const refetchFailedText = 'The latest data could not be loaded. The page shows what was loaded before.';
const unavailableText = 'This tutor is not available.';

const liveLesson: LessonDetail = {
  id: '00000000-0000-4000-8000-000000000021',
  title: 'Conversation practice',
  description: null,
  starts_at: '2026-10-31T23:50:00Z',
  ends_at: '2026-11-01T00:50:00Z',
  duration_minutes: 60,
  status: 'live',
  took_place: true,
  timezone: 'Europe/Moscow',
  series_id: null,
  series: null,
  version: 1,
  cancelled_at: null,
  other_timezone: null,
  audience: { kind: 'one_to_one', label: 'Anna', label_kind: null, colour: 'moss', members: null },
  tutor: { name: 'Maria', colour: 'moss' },
  channel_id: null,
  room_open: true,
  embed_allowed: false,
  materials: [],
};

const liveItem: CalendarItem = {
  id: liveLesson.id,
  title: liveLesson.title,
  starts_at: liveLesson.starts_at,
  ends_at: liveLesson.ends_at,
  status: 'live',
  took_place: true,
  timezone: liveLesson.timezone,
  audience: { kind: 'one_to_one', label: 'Anna', label_kind: null, colour: 'moss' },
  tutor: { name: 'Maria', colour: 'moss' },
};

class RoomTabs {
  readonly opened: { closed: boolean }[] = [];

  readonly open: OpenRoomTab = () => {
    const tab = { closed: false };
    this.opened.push(tab);
    return {
      navigate: (): boolean => !tab.closed,
      close: (): void => {
        tab.closed = true;
      },
    };
  };
}

function tutor(relationshipId: string, name: string): MyTutor {
  return {
    relationship_id: relationshipId,
    state: 'active',
    colour: 'moss',
    channel_id: null,
    account_closed_until: null,
    ended_at: null,
    ended_by: null,
    tutor: { name, avatar_id: null, subjects: ['English'], bio: '' },
  };
}

function work(
  id: string,
  title: string,
  status: StudentAssignmentItem['status'],
  feedbackNew: boolean,
): StudentAssignmentItem {
  return {
    submission_id: `00000000-0000-4000-8000-${id.padStart(12, '0')}`,
    title,
    tutor: { relationship_id: mariaLink, name: 'Maria', colour: 'moss' },
    status,
    late: false,
    issued_before_reclaim: false,
    deadline_at: null,
    issued_at: '2026-10-28T10:00:00Z',
    submitted_at: status === 'checked' ? '2026-10-29T10:00:00Z' : null,
    checked_at: status === 'checked' ? '2026-10-30T10:00:00Z' : null,
    feedback_excerpt: status === 'checked' ? 'Well done.' : null,
    has_corrected_files: false,
    feedback_new: feedbackNew,
  };
}

interface HomeSetup {
  readonly address?: string;
  readonly language?: Language;
}

interface ShownHome {
  readonly server: FakeAuthServer;
  readonly queryClient: FollowingQueryClient;
  readonly navigate: (address: string) => Promise<void>;
  readonly tabs: RoomTabs;
}

async function showHome({ address = '/home', language = 'en' }: HomeSetup = {}): Promise<ShownHome> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'student-1', name: 'Anna', email: studentEmail, password, role: 'student' });
  server.seedTutors(studentEmail, [tutor(mariaLink, 'Maria'), tutor(pavelLink, 'Pavel')]);
  server.addAssignment(studentEmail, work('81', 'Irregular verbs', 'not_started', false));
  server.addAssignment(studentEmail, work('82', 'Past simple', 'checked', true));
  server.addAssignment(studentEmail, work('83', 'Articles', 'checked', false));
  server.addLesson(liveLesson, [studentEmail]);
  server.addCalendarItem(studentEmail, liveItem, mariaLink);
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(studentEmail, password);
  const i18n = await createI18n(language);
  const queryClient = new FollowingQueryClient();
  const tabs = new RoomTabs();
  const scheduler = manualScheduler();
  scheduler.suspend(server.serverNow());
  const router = createMemoryRouter(
    [{ path: '/home', element: <HomePage />, handle: { title: 'page.home' } satisfies RouteHandle }],
    { initialEntries: [address] },
  );
  render(
    <SchedulerContext value={scheduler}>
      <RoomTabContext value={tabs.open}>
        <SessionContext value={session}>
          <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
            <QueryClientProvider client={queryClient}>
              <I18nextProvider i18n={i18n}>
                <main>
                  <RouterProvider router={router} />
                </main>
              </I18nextProvider>
            </QueryClientProvider>
          </ApiClientContext>
        </SessionContext>
      </RoomTabContext>
    </SchedulerContext>,
  );
  await screen.findAllByRole('listitem');
  return {
    server,
    queryClient,
    tabs,
    navigate: async (to: string): Promise<void> => {
      await act(async () => {
        await router.navigate(to);
      });
    },
  };
}

async function refetchHome(shown: ShownHome): Promise<void> {
  await act(async () => {
    await shown.queryClient.refetchQueries({ queryKey: studentHomesKey, type: 'active' });
  });
  await waitFor(() => {
    expect(shown.queryClient.isFetching()).toBe(0);
  });
}

describe('Home on GET /home', () => {
  test('sends one GET /home on load and none of the reads it replaced', async () => {
    const shown = await showHome();

    const todo = await screen.findByRole('link', { name: 'Irregular verbs' });

    expect(shown.server.homeQueries()).toEqual(['']);
    expect(
      shown.server.sentAddresses
        .map((sent) => sent.pathname)
        .filter((path) => ['/api/v1/calendar', '/api/v1/tutors', '/api/v1/assignments'].includes(path)),
    ).toEqual([]);
    expect(todo).toBeTruthy();
  });

  test('a tutor named by the address goes to /home on the first load', async () => {
    const shown = await showHome({ address: `/home?tutor=${mariaLink}` });

    await screen.findByRole('link', { name: 'Irregular verbs' });

    expect(shown.server.homeQueries()).toEqual([`?tutor=${mariaLink}`]);
  });

  test('a refused tutor value costs one further read without it and is never sent again on a refetch', async () => {
    const shown = await showHome({ address: `/home?tutor=${otherLink}` });
    await screen.findByText('This tutor is not available.');

    await refetchHome(shown);

    expect(shown.server.homeQueries()).toEqual([`?tutor=${otherLink}`, '', '']);
    expect(screen.getByText('This tutor is not available.')).toBeTruthy();
    expect(screen.getByRole('link', { name: 'All tutors' }).getAttribute('href')).toBe('/home');
    expect(screen.queryByRole('link', { name: 'Irregular verbs' })).toBeNull();
    expect(screen.getAllByText('Maria').length).toBeGreaterThan(0);
  });

  test('a failed refetch keeps every panel and a held Join control with its tab and focus, with one notice above the next lesson', async () => {
    const shown = await showHome();
    const pressed = await screen.findByRole('button', { name: 'Join' });
    const paused = shown.server.pauseLessonWrites();
    pressed.focus();
    fireEvent.click(pressed);
    await paused.received;
    shown.server.failNextHomeRead();

    await refetchHome(shown);
    const notice = await screen.findByText(refetchFailedText);
    const nextLesson = screen.getByRole('heading', { name: 'Next lesson' });
    const afterRefetch = {
      closed: shown.tabs.opened.map((tab) => tab.closed),
      notices: screen.getAllByText(refetchFailedText).length,
      noticeFirst: (notice.compareDocumentPosition(nextLesson) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0,
      checkedRow: screen.queryByRole('link', { name: 'Past simple' }) !== null,
      focused: document.activeElement,
    };
    paused.resume();

    expect(afterRefetch).toEqual({
      closed: [false],
      notices: 1,
      noticeFirst: true,
      checkedRow: true,
      focused: pressed,
    });
    expect(await screen.findByRole('button', { name: 'Open again' })).toBe(pressed);
  });

  test('a refusal on a later refetch keeps the page, says the tutor is not available and moves lost focus to the page heading', async () => {
    const shown = await showHome({ address: `/home?tutor=${mariaLink}` });
    (await screen.findByRole('link', { name: 'Irregular verbs' })).focus();
    shown.server.seedTutors(studentEmail, [
      { ...tutor(mariaLink, 'Maria'), state: 'ended' },
      tutor(pavelLink, 'Pavel'),
    ]);

    await refetchHome(shown);
    await screen.findByText(unavailableText);

    expect(shown.server.homeQueries()).toEqual([`?tutor=${mariaLink}`, `?tutor=${mariaLink}`, '']);
    expect(screen.queryByRole('link', { name: 'Irregular verbs' })).toBeNull();
    expect(screen.queryByText('Something went wrong.')).toBeNull();
    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
    });
  });

  test('a failed further read after a later refusal is shown as a failed refetch with the page kept', async () => {
    const shown = await showHome({ address: `/home?tutor=${mariaLink}` });
    shown.server.seedTutors(studentEmail, [
      { ...tutor(mariaLink, 'Maria'), state: 'ended' },
      tutor(pavelLink, 'Pavel'),
    ]);
    const refused = shown.server.pauseHomeReads();
    const refetching = shown.queryClient.refetchQueries({ queryKey: studentHomesKey, type: 'active' });
    await refused.received;

    refused.resume();
    const further = shown.server.pauseHomeReads();
    await act(async () => {
      await refetching;
      await further.received;
    });
    shown.server.failNextHomeRead();
    further.resume();
    await screen.findByText(refetchFailedText);

    expect(screen.getAllByText(refetchFailedText)).toHaveLength(1);
    expect(screen.getByText(unavailableText)).toBeTruthy();
    expect(screen.queryByText('Something went wrong.')).toBeNull();
    expect(screen.getAllByText('Pavel').length).toBeGreaterThan(0);
  });

  test('choosing a tutor keeps the page and focus on the pressed choice while its read is in flight', async () => {
    const shown = await showHome();
    const pavel = screen.getByRole('button', { name: 'Pavel' });
    pavel.focus();
    const held = shown.server.pauseHomeReads();

    fireEvent.click(pavel);
    await held.received;
    const whileLoading = {
      focused: document.activeElement === pavel,
      todo: screen.queryByRole('link', { name: 'Irregular verbs' }) !== null,
    };
    held.resume();
    await waitFor(() => {
      expect(shown.server.homeQueries()).toEqual(['', `?tutor=${pavelLink}`]);
    });

    expect(whileLoading).toEqual({ focused: true, todo: true });
    expect(document.activeElement).toBe(pavel);
  });

  test('a failed first read of a newly chosen tutor shows the load failure in place of the filtered panels and keeps focus on the choice', async () => {
    const shown = await showHome();
    const pavel = screen.getByRole('button', { name: 'Pavel' });
    pavel.focus();
    shown.server.failNextHomeRead();

    fireEvent.click(pavel);
    await screen.findByText('Something went wrong.');

    expect(screen.queryByRole('link', { name: 'Irregular verbs' })).toBeNull();
    expect(screen.queryByRole('heading', { name: 'Next seven days' })).toBeNull();
    expect(screen.queryByText(refetchFailedText)).toBeNull();
    expect(screen.getByRole('button', { name: 'Try again' })).toBeTruthy();
    expect(screen.getByRole('list', { name: 'Tutors' }).querySelectorAll('li')).toHaveLength(2);
    expect(screen.queryByText(/lessons? today/)).toBeNull();
    expect(document.activeElement).toBe(pavel);
  });

  test('a tutor value no loaded current tutor holds is refused without a read', async () => {
    const shown = await showHome();
    const readsBefore = shown.server.homeQueries();

    await shown.navigate(`/home?tutor=${otherLink}`);
    await screen.findByText(unavailableText);

    expect(shown.server.homeQueries()).toEqual(readsBefore);
    expect(screen.queryByRole('link', { name: 'Irregular verbs' })).toBeNull();
  });

  test.each([
    ['en', 'New feedback', 'New'],
    ['ru', 'Новый отзыв', 'Новое'],
  ] as const)('in %s marks only new feedback in Recently checked, with no points', async (language, name, text) => {
    await showHome({ language });

    const badges = await screen.findAllByRole('img', { name });

    expect(badges).toHaveLength(1);
    expect(badges[0]?.textContent).toBe(text);
    expect(badges[0]?.closest('li')?.textContent).toContain('Past simple');
    expect(screen.getByRole('main').textContent).not.toMatch(/points|балл/i);
  });
});
