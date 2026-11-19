import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
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
import { tutorHomeKey } from './api.ts';
import { TodayPage } from './TodayPage.tsx';

afterEach(() => {
  cleanup();
});

type TutorJoinRequest = components['schemas']['TutorJoinRequest'];
type ReviewQueueItem = components['schemas']['ReviewQueueItem'];

const origin = 'https://ovc.example';
const password = 'a long password';
const tutorEmail = 'maria@example.org';
const minute = 60_000;
const refetchFailedText = 'The latest data could not be loaded. The page shows what was loaded before.';
const loadFailedText = 'Something went wrong.';
const checklistHeading = 'Getting started';
const lessonsHeading = 'Today’s lessons';
const replacedReads = [
  '/api/v1/calendar',
  '/api/v1/students',
  '/api/v1/library',
  '/api/v1/review-queue',
  '/api/v1/issues',
  '/api/v1/channels',
  '/api/v1/join-requests',
  '/api/v1/tutor/code',
  '/api/v1/groups',
];

const lessonDetail: LessonDetail = {
  id: '00000000-0000-4000-8000-000000000021',
  title: 'Conversation practice',
  description: null,
  starts_at: '2026-11-01T09:00:00Z',
  ends_at: '2026-11-01T10:00:00Z',
  duration_minutes: 60,
  status: 'scheduled',
  took_place: false,
  timezone: 'Europe/Moscow',
  series_id: null,
  series: null,
  version: 1,
  cancelled_at: null,
  other_timezone: null,
  audience: { kind: 'one_to_one', label: 'Anna', label_kind: null, colour: 'moss', members: null },
  tutor: null,
  channel_id: null,
  room_open: false,
  embed_allowed: false,
  materials: [],
};

const lessonItem: CalendarItem = {
  id: lessonDetail.id,
  title: lessonDetail.title,
  starts_at: lessonDetail.starts_at,
  ends_at: lessonDetail.ends_at,
  status: lessonDetail.status,
  took_place: false,
  timezone: lessonDetail.timezone,
  audience: { kind: 'one_to_one', label: 'Anna', label_kind: null, colour: 'moss' },
  tutor: null,
};

const queueItem: ReviewQueueItem = {
  submission_id: '00000000-0000-4000-8000-000000000051',
  issue_id: '00000000-0000-4000-8000-000000000052',
  title: 'Past simple',
  student: { display_name: 'Anna', relationship_id: '00000000-0000-4000-8000-000000000031' },
  first_submitted_at: '2026-10-31T10:00:00Z',
  submitted_at: '2026-10-31T10:00:00Z',
  late: false,
  has_draft: false,
  issued_before_reclaim: false,
};

function joinRequest(index: number): TutorJoinRequest {
  const suffix = String(100 + index).padStart(12, '0');
  return {
    id: `00000000-0000-4000-8000-${suffix}`,
    created_at: new Date(Date.UTC(2026, 9, 20, 10, index)).toISOString(),
    expires_at: new Date(Date.UTC(2026, 10, 20, 10, index)).toISOString(),
    student: { name: `Requester ${String(index)}`, email: `requester${String(index)}@example.org`, avatar_id: null },
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

interface TodaySetup {
  readonly state?: 'active' | 'unverified';
  readonly joinRequests?: number;
  readonly firstLoadFails?: boolean;
}

interface ShownToday {
  readonly server: FakeAuthServer;
  readonly queryClient: QueryClient;
  readonly tabs: RoomTabs;
}

async function showToday({
  state = 'active',
  joinRequests = 0,
  firstLoadFails = false,
}: TodaySetup = {}): Promise<ShownToday> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Maria', email: tutorEmail, password, role: 'tutor', state });
  server.addLesson(lessonDetail, [tutorEmail]);
  server.addCalendarItem(tutorEmail, lessonItem);
  server.addQueueItem(tutorEmail, queueItem);
  for (let index = 1; index <= joinRequests; index += 1) {
    server.addJoinRequest(tutorEmail, joinRequest(index));
  }
  if (firstLoadFails) {
    server.failNextHomeRead();
  }
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(tutorEmail, password);
  const scheduler = manualScheduler();
  scheduler.suspend(Date.parse(lessonDetail.starts_at) - 5 * minute);
  const i18n = await createI18n('en');
  const tabs = new RoomTabs();
  const queryClient = new FollowingQueryClient();
  const router = createMemoryRouter(
    [{ path: '/today', element: <TodayPage />, handle: { title: 'page.today' } satisfies RouteHandle }],
    { initialEntries: ['/today'] },
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
  if (firstLoadFails) {
    await screen.findByText(loadFailedText);
  } else {
    await screen.findByRole('heading', { name: lessonsHeading });
  }
  if (!firstLoadFails && state === 'active') {
    await screen.findByRole('link', { name: 'Anna' });
  }
  return { server, queryClient, tabs };
}

function requestedPaths(server: FakeAuthServer): readonly string[] {
  return server.sentAddresses.map((address) => address.pathname);
}

async function refetchHomeFailing(shown: ShownToday): Promise<void> {
  shown.server.failNextHomeRead();
  await act(async () => {
    await shown.queryClient.refetchQueries({ queryKey: tutorHomeKey });
  });
  await screen.findByText(refetchFailedText);
}

function checklist(): HTMLElement {
  const section = screen.getByRole('heading', { name: checklistHeading }).closest('section');
  if (section === null) {
    throw new Error('the checklist has no section');
  }
  return section;
}

describe('Today on GET /home', () => {
  test('sends one GET /home on load and none of the reads it replaced', async () => {
    const shown = await showToday();

    const lessonLink = await screen.findByRole('link', { name: /Conversation practice/ });

    expect(shown.server.homeReads()).toBe(1);
    expect(requestedPaths(shown.server).filter((path) => replacedReads.includes(path))).toEqual([]);
    expect(lessonLink).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Anna' })).toBeTruthy();
  });

  test('a failed first load shows the load failure with Retry under the page title and no panel', async () => {
    await showToday({ firstLoadFails: true });

    const retry = screen.getByRole('button', { name: 'Try again' });

    expect(screen.getByRole('heading', { level: 1 })).toBeTruthy();
    expect(screen.queryByRole('heading', { name: lessonsHeading })).toBeNull();
    expect(screen.queryByRole('heading', { name: checklistHeading })).toBeNull();
    expect(retry).toBeTruthy();
  });

  test('a failed refetch keeps every panel, a held Join control with its tab and focus, and shows one notice first in the main column', async () => {
    const shown = await showToday();
    const pressed = await screen.findByRole('button', { name: 'Join lesson' });
    const paused = shown.server.pauseLessonWrites();
    pressed.focus();
    fireEvent.click(pressed);
    await paused.received;

    await refetchHomeFailing(shown);
    const notice = screen.getByText(refetchFailedText);
    const afterRefetch = {
      closed: shown.tabs.opened.map((tab) => tab.closed),
      notices: screen.getAllByText(refetchFailedText).length,
      loadFailures: screen.queryAllByText(loadFailedText).length,
      checklistShown: screen.queryByRole('heading', { name: checklistHeading }) !== null,
      queueRow: screen.queryByRole('link', { name: 'Anna' }) !== null,
      noticeBeforeChecklist: (notice.compareDocumentPosition(checklist()) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0,
      focused: document.activeElement,
    };
    paused.resume();

    expect(afterRefetch).toEqual({
      closed: [false],
      notices: 1,
      loadFailures: 0,
      checklistShown: true,
      queueRow: true,
      noticeBeforeChecklist: true,
      focused: pressed,
    });
    expect(await screen.findByRole('button', { name: 'Open again' })).toBe(pressed);
  });

  test('Dismiss sends one PATCH, hides the checklist and moves focus to the heading of today’s lessons', async () => {
    const shown = await showToday();
    const dismiss = within(checklist()).getByRole('button', { name: 'Dismiss' });
    dismiss.focus();

    fireEvent.click(dismiss);
    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: checklistHeading })).toBeNull();
    });

    expect(shown.server.settingsWrites().map((body): unknown => JSON.parse(body))).toEqual([
      { onboarding_dismissed: true },
    ]);
    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByRole('heading', { name: lessonsHeading }));
    });
  });

  test('a dismissal racing a held refetch never brings the checklist back', async () => {
    const shown = await showToday();
    const held = shown.server.pauseHomeReads();
    const refetching = shown.queryClient.invalidateQueries({ queryKey: tutorHomeKey });
    await held.received;

    fireEvent.click(within(checklist()).getByRole('button', { name: 'Dismiss' }));
    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: checklistHeading })).toBeNull();
    });
    held.resume();
    await act(async () => {
      await refetching;
    });
    await waitFor(() => {
      expect(shown.queryClient.isFetching()).toBe(0);
    });

    expect(screen.queryByRole('heading', { name: checklistHeading })).toBeNull();
    expect(shown.server.settingsWrites()).toHaveLength(1);
  });

  test('a refused dismissal shows its message in an alert inside the checklist that takes focus', async () => {
    const shown = await showToday();
    shown.server.refuseNextSettingsWrite(429, 'rate_limited');

    fireEvent.click(within(checklist()).getByRole('button', { name: 'Dismiss' }));
    const alert = await within(checklist()).findByRole('alert');

    expect(alert.textContent).not.toBe('');
    expect(document.activeElement).toBe(alert);
    expect(screen.getByRole('heading', { name: checklistHeading })).toBeTruthy();
  });

  test('an unconfirmed tutor sees the confirmation prompt and no homework panels, requests no closed route and can dismiss', async () => {
    const shown = await showToday({ state: 'unverified' });
    const prompts = {
      invite: within(checklist()).queryByText('Confirm your email to invite students.') !== null,
      inviteLink: within(checklist()).queryByRole('link', { name: 'Open Students' }) !== null,
    };

    fireEvent.click(within(checklist()).getByRole('button', { name: 'Dismiss' }));
    await waitFor(() => {
      expect(screen.queryByRole('heading', { name: checklistHeading })).toBeNull();
    });

    expect(prompts).toEqual({ invite: true, inviteLink: false });
    expect(screen.queryByRole('heading', { name: /^Needs checking/ })).toBeNull();
    expect(screen.queryByRole('heading', { name: 'Upcoming deadlines' })).toBeNull();
    expect(requestedPaths(shown.server).filter((path) => replacedReads.includes(path))).toEqual([]);
  });

  test('Show more continues the join requests from the /home cursor and a /home refetch drops the extra rows', async () => {
    const shown = await showToday({ joinRequests: 7 });
    const region = screen.getByRole('region', { name: 'Join requests' });
    const firstRows = within(region).getAllByRole('listitem').length;

    fireEvent.click(within(region).getByRole('button', { name: 'Show more' }));
    const sixth = await within(region).findByRole('button', { name: 'Accept Requester 6' });
    await waitFor(() => {
      expect(document.activeElement).toBe(sixth);
    });
    const afterMore = within(region).getAllByRole('listitem').length;
    await act(async () => {
      await shown.queryClient.refetchQueries({ queryKey: tutorHomeKey });
    });
    await waitFor(() => {
      expect(within(region).getAllByRole('listitem')).toHaveLength(5);
    });

    expect(firstRows).toBe(5);
    expect(afterMore).toBe(7);
    expect(
      new Set(
        within(region)
          .getAllByRole('listitem')
          .map((row) => row.textContent),
      ).size,
    ).toBe(5);
    expect(requestedPaths(shown.server).filter((path) => path === '/api/v1/join-requests')).toHaveLength(1);
  });

  test('a /home refetch that started before Show more and answers after it drops the extra rows and keeps focus in the list', async () => {
    const shown = await showToday({ joinRequests: 7 });
    const region = screen.getByRole('region', { name: 'Join requests' });
    const held = shown.server.pauseHomeReads();
    const refetching = shown.queryClient.invalidateQueries({ queryKey: tutorHomeKey });
    await held.received;
    fireEvent.click(within(region).getByRole('button', { name: 'Show more' }));
    const sixth = await within(region).findByRole('button', { name: 'Accept Requester 6' });
    await waitFor(() => {
      expect(document.activeElement).toBe(sixth);
    });

    held.resume();
    await act(async () => {
      await refetching;
    });
    await waitFor(() => {
      expect(within(region).getAllByRole('listitem')).toHaveLength(5);
    });

    const rows = within(region).getAllByRole('listitem');
    expect(new Set(rows.map((row) => row.textContent)).size).toBe(5);
    expect(within(region).getByRole('button', { name: 'Show more' })).toBeTruthy();
    expect(document.activeElement).toBe(within(region).getByRole('button', { name: 'Accept Requester 5' }));
  });
});
