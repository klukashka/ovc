import { act, cleanup, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { MediaQueriesContext, fixedMediaQueries } from '../../app/context.ts';
import type { Me } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { Calendar, CalendarItem } from './api.ts';
import { rangeOf } from './calendarTime.ts';
import { CalendarBody } from './SchedulePage.tsx';

afterEach(() => {
  cleanup();
});

const tutor: Me = {
  id: '00000000-0000-4000-8000-000000000001',
  role: 'tutor',
  state: 'active',
  name: 'Мария',
  email: 'maria@example.org',
  email_verified: true,
  language: 'ru',
  timezone: 'Europe/Moscow',
  theme: 'light',
  avatar_id: null,
  reclaim_pending: false,
};

const lesson: CalendarItem = {
  id: '00000000-0000-4000-8000-000000000002',
  title: 'Урок',
  starts_at: '2026-11-03T15:00:00Z',
  ends_at: '2026-11-03T16:00:00Z',
  status: 'scheduled',
  timezone: 'Europe/Moscow',
  audience: { kind: 'one_to_one', label: 'Анна', label_kind: null, colour: 'moss' },
  tutor: null,
};

async function showOnPhone(calendar: Calendar): Promise<void> {
  const i18n = await createI18n('ru');
  const range = rangeOf('month', '2026-11-03', 'Europe/Moscow');
  const router = createMemoryRouter(
    [
      {
        path: '*',
        element: <CalendarBody calendar={calendar} range={range} me={tutor} today="2026-11-03" onSlot={null} />,
      },
    ],
    { initialEntries: ['/schedule'] },
  );
  await act(async () => {
    render(
      <MediaQueriesContext value={fixedMediaQueries([])}>
        <I18nextProvider i18n={i18n}>
          <RouterProvider router={router} />
        </I18nextProvider>
      </MediaQueriesContext>,
    );
    await Promise.resolve();
  });
}

describe('calendar body', () => {
  test('a truncated response shows the notice above the lessons', async () => {
    const calendar: Calendar = { items: [lesson], truncated: true };

    await showOnPhone(calendar);

    const notice = screen.getByText(
      'Показаны только первые 1 000 уроков этого периода. Откройте неделю, чтобы увидеть остальные.',
    );
    const firstLesson = screen.getByRole('link', { name: /Урок/ });
    expect(notice.compareDocumentPosition(firstLesson) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  test('a month agenda without lessons in that month says so', async () => {
    const lessonBeforeTheMonth = { ...lesson, starts_at: '2026-10-29T15:00:00Z', ends_at: '2026-10-29T16:00:00Z' };
    const calendar: Calendar = { items: [lessonBeforeTheMonth], truncated: false };

    await showOnPhone(calendar);

    expect(screen.getByText('В этом периоде нет уроков')).toBeDefined();
    expect(screen.queryByText(/Показаны только первые/)).toBeNull();
    expect(screen.queryAllByRole('link')).toHaveLength(0);
  });
});
