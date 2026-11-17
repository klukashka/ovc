import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { MediaQueriesContext, fixedMediaQueries } from '../../app/context.ts';
import { liveRefetch } from '../../app/liveRefetch.ts';
import { NotificationArrivals } from '../../app/notificationArrivals.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { RealtimeEvent } from '../../lib/realtime/client.ts';
import type { LessonView } from './api.ts';
import { LessonPage } from './LessonPage.tsx';
import { RoomTabContext } from './roomTab.ts';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const password = 'a long password';
const tutorEmail = 'maria@example.org';
const otherLesson = '00000000-0000-4000-8000-000000000099';

const lessonDetail: LessonView = {
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
  audience: { kind: 'one_to_one', label: 'Анна', label_kind: null, colour: 'moss', members: [] },
  tutor: null,
  channel_id: null,
  room_open: false,
  embed_allowed: false,
  materials: [],
  issues: [],
};

interface ShownLesson {
  readonly server: FakeAuthServer;
  readonly queryClient: QueryClient;
  readonly deliver: (event: RealtimeEvent) => Promise<void>;
}

async function showLesson(): Promise<ShownLesson> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Мария', email: tutorEmail, password, role: 'tutor' });
  server.addLesson(lessonDetail, [tutorEmail]);
  const scheduler = manualScheduler();
  scheduler.suspend(Date.parse(lessonDetail.starts_at) - 86_400_000);
  const session = new Session({ origin, transport: server.transport(), locks: memoryLocks(), scheduler });
  await session.signIn(tutorEmail, password);
  const client = createApiClient(origin, session.authorizedFetch);
  const i18n = await createI18n('ru');
  const queryClient = new QueryClient();
  const refetch = liveRefetch(queryClient, new NotificationArrivals(), scheduler);
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
    ],
    { initialEntries: [`/lessons/${lessonDetail.id}`] },
  );
  render(
    <MediaQueriesContext value={fixedMediaQueries(['(min-width: 768px)'])}>
      <SchedulerContext value={scheduler}>
        <RoomTabContext value={() => null}>
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
      </SchedulerContext>
    </MediaQueriesContext>,
  );
  await screen.findByRole('heading', { level: 1, name: 'Разговорная практика' });
  return {
    server,
    queryClient,
    deliver: async (event): Promise<void> => {
      act(() => {
        refetch(event);
      });
      await waitFor(() => {
        expect(queryClient.isFetching()).toBe(0);
      });
    },
  };
}

describe('the lesson page under homework frames', () => {
  test('refetches for a frame that names its lesson', async () => {
    const shown = await showLesson();
    const readsBefore = shown.server.lessonReads(lessonDetail.id);

    await shown.deliver({ kind: 'entity', entity: 'issue', id: 'issue-1', lessonIds: [otherLesson, lessonDetail.id] });

    expect(shown.server.lessonReads(lessonDetail.id)).toBe(readsBefore + 1);
  });

  test('ignores homework frames that name another lesson or none', async () => {
    const shown = await showLesson();
    const readsBefore = shown.server.lessonReads(lessonDetail.id);

    await shown.deliver({ kind: 'entity', entity: 'issue', id: 'issue-1', lessonIds: [otherLesson] });
    await shown.deliver({ kind: 'entity', entity: 'submission', id: 'submission-1', issueId: 'issue-1' });
    await shown.deliver({ kind: 'entity', entity: 'submission', id: 'submission-2' });

    expect(shown.server.lessonReads(lessonDetail.id)).toBe(readsBefore);
  });
});
