import { act, cleanup, render, screen } from '@testing-library/react';
import { type ReactElement, useRef, useSyncExternalStore } from 'react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { Page } from '../../components/ui/Page.tsx';
import type { Me } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { ChangedLessons, LessonDetail, LessonSeries } from './api.ts';
import {
  LessonActionsMenu,
  LessonEditButton,
  LessonView,
  MemberLabel,
  headingOf,
  lessonControlsShown,
  nextSeriesAfterEnding,
  useHeadingWhenControlsLeave,
} from './LessonPage.tsx';

afterEach(() => {
  cleanup();
});

const minute = 60_000;

const student: Me = {
  id: '00000000-0000-4000-8000-000000000003',
  role: 'student',
  state: 'active',
  name: 'Анна',
  email: 'anna@example.org',
  email_verified: true,
  language: 'ru',
  timezone: 'Europe/Berlin',
  theme: 'light',
  avatar_id: null,
  reclaim_pending: false,
};

const tutor: Me = {
  ...student,
  id: '00000000-0000-4000-8000-000000000001',
  role: 'tutor',
  name: 'Мария',
  email: 'maria@example.org',
  timezone: 'Europe/Moscow',
};

const title = 'שיעור <b>1</b>';
const description = 'Первая строка <img src=x>\nשורה שנייה';

const lesson: LessonDetail = {
  id: '00000000-0000-4000-8000-000000000004',
  title,
  description,
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
  other_timezone: 'Europe/Moscow',
  audience: { kind: 'one_to_one', label: null, label_kind: 'individual', colour: 'moss', members: null },
  tutor: { name: 'Мария', colour: 'moss' },
  channel_id: null,
  room_open: false,
  embed_allowed: false,
  materials: [],
};

const start = Date.parse(lesson.starts_at);
const dayBefore = start - 24 * 60 * minute;

const relationshipId = '00000000-0000-4000-8000-000000000010';

const occurrenceSeries: LessonSeries = {
  id: '00000000-0000-4000-8000-000000000005',
  weekdays: [2],
  start: '18:00',
  duration_minutes: 60,
  timezone: 'Europe/Moscow',
  first_date: '2026-10-27',
  end_date: '2026-11-04',
  count: null,
};

const occurrenceWithoutItsDate: LessonDetail = {
  ...lesson,
  series_id: occurrenceSeries.id,
  series: occurrenceSeries,
  other_timezone: null,
  audience: {
    kind: 'one_to_one',
    relationship_id: relationshipId,
    label: 'Анна',
    label_kind: null,
    colour: 'moss',
    members: [],
  },
  tutor: null,
};

const seriesOccurrence: LessonDetail = { ...occurrenceWithoutItsDate, occurrence_date: '2026-11-03' };

function seriesEndingOn(endDate: string): ChangedLessons {
  return { series: { ...occurrenceSeries, end_date: endDate }, lessons: [] };
}

interface Shown {
  readonly lesson: LessonDetail;
  readonly me: Me;
  readonly now: number;
}

class ShownStore {
  private value: Shown;
  private readonly listeners = new Set<() => void>();

  constructor(initial: Shown) {
    this.value = initial;
  }

  readonly get = (): Shown => this.value;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return (): void => {
      this.listeners.delete(listener);
    };
  };

  set(next: Partial<Shown>): void {
    this.value = { ...this.value, ...next };
    for (const listener of this.listeners) {
      listener();
    }
  }
}

const openNoDialog = (): undefined => undefined;

function Shown({ store }: { readonly store: ShownStore }): ReactElement {
  const shown = useSyncExternalStore(store.subscribe, store.get);
  const editButton = useRef<HTMLButtonElement>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  const controlsShown = lessonControlsShown(shown.me, shown.lesson, shown.now);
  useHeadingWhenControlsLeave(controlsShown);
  return (
    <Page
      {...headingOf(shown.lesson, false)}
      action={controlsShown ? <LessonEditButton editButton={editButton} onOpen={openNoDialog} /> : null}
      menu={
        controlsShown ? (
          <LessonActionsMenu menuButton={menuButton} series={shown.lesson.series !== null} onOpen={openNoDialog} />
        ) : null
      }
    >
      <LessonView lesson={shown.lesson} me={shown.me} now={shown.now} />
    </Page>
  );
}

async function show(initial: Partial<Shown> = {}): Promise<ShownStore> {
  const store = new ShownStore({ lesson, me: student, now: dayBefore, ...initial });
  const i18n = await createI18n('ru');
  const router = createMemoryRouter(
    [
      {
        path: '*',
        handle: { title: 'page.lesson' },
        element: (
          <main>
            <Shown store={store} />
          </main>
        ),
      },
    ],
    { initialEntries: [`/lessons/${lesson.id}`] },
  );
  await act(async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <RouterProvider router={router} />
      </I18nextProvider>,
    );
    await Promise.resolve();
  });
  return store;
}

function focusedEditControl(): HTMLElement {
  const control = screen.getByRole('button', { name: 'Изменить урок' });
  control.focus();
  return control;
}

function focusedCancelItemOfOpenMenu(): HTMLElement {
  act(() => {
    screen.getByRole('button', { name: 'Действия с уроком' }).click();
  });
  const item = screen.getByRole('button', { name: 'Отменить урок' });
  item.focus();
  return item;
}

describe('lesson page', () => {
  test('the title and description render in elements isolated with dir="auto"', async () => {
    await show();

    const heading = screen.getByRole('heading', { level: 1 });
    const shownDescription = document.querySelector<HTMLElement>('[data-part="description"]');

    expect(heading.textContent).toBe(title);
    expect(heading.getAttribute('dir')).toBe('auto');
    expect(shownDescription?.getAttribute('dir')).toBe('auto');
    expect(shownDescription?.textContent).toBe(description);
    expect(shownDescription?.className).toContain('whitespace-pre-line');
    expect(document.querySelector('article')?.querySelector('b, img')).toBeNull();
    expect(screen.getAllByText('18:00 по времени Москвы (16:00 по вашему)').length).toBeGreaterThan(0);
  });

  test('inside the join window the status word is Starting soon', async () => {
    await show({ now: start - 5 * minute });

    const status = screen.getByRole('status');

    expect(status.textContent).toBe('Скоро начало');
  });

  test('a scheduled lesson whose end has passed shows Did not take place', async () => {
    await show({ now: Date.parse(lesson.ends_at) + minute });

    const status = screen.getByRole('status');

    expect(status.textContent).toBe('Не состоялся');
  });

  test('focused controls that leave when the start passes move focus to the heading', async () => {
    const store = await show({ me: tutor, now: start - minute });
    const control = focusedEditControl();

    act(() => {
      store.set({ now: start });
    });

    expect(control.isConnected).toBe(false);
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });

  test('focused controls that leave when a refetch shows the lesson cancelled move focus to the heading', async () => {
    const store = await show({ me: tutor });
    const control = focusedEditControl();

    act(() => {
      store.set({ lesson: { ...lesson, status: 'cancelled', cancelled_at: '2026-11-01T10:00:00Z', version: 2 } });
    });

    expect(control.isConnected).toBe(false);
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });

  test('a focused item of the open lesson actions menu that leaves when the start passes moves focus to the heading', async () => {
    const store = await show({ me: tutor, now: start - minute });
    const item = focusedCancelItemOfOpenMenu();

    act(() => {
      store.set({ now: start });
    });

    expect(item.isConnected).toBe(false);
    expect(screen.queryByRole('button', { name: 'Действия с уроком' })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });

  test('the heading stays the same element when the controls leave', async () => {
    const store = await show({ me: tutor, now: start - minute });
    const headingWithControls = screen.getByRole('heading', { level: 1 });
    const editShownBefore = screen.queryByRole('button', { name: 'Изменить урок' });

    act(() => {
      store.set({ now: start });
    });

    expect(editShownBefore).not.toBeNull();
    expect(screen.queryByRole('button', { name: 'Изменить урок' })).toBeNull();
    expect(screen.getByRole('heading', { level: 1 })).toBe(headingWithControls);
  });

  test('a lesson detail without its occurrence date treats a series ending as changed meanwhile', async () => {
    const { t } = await createI18n('ru');
    const prefill = nextSeriesAfterEnding(t, occurrenceWithoutItsDate, seriesEndingOn('2026-11-02'), tutor.timezone);

    expect(prefill).toBeNull();
  });

  test('an occurrence moved to an earlier date prefills the new series on its original date', async () => {
    const { t } = await createI18n('ru');
    const movedEarlier = { ...seriesOccurrence, starts_at: '2026-11-02T15:00:00Z', ends_at: '2026-11-02T16:00:00Z' };

    const prefill = nextSeriesAfterEnding(t, movedEarlier, seriesEndingOn('2026-11-02'), tutor.timezone);

    expect(prefill).toEqual({
      date: '2026-11-03',
      start: '18:00',
      durationMinutes: 60,
      weekdays: [2],
      audience: { kind: 'one_to_one', id: relationshipId, name: 'Анна' },
    });
  });

  test('an occurrence moved beyond the series end whose series did not end opens no new series', async () => {
    const { t } = await createI18n('ru');
    const movedBeyond = { ...seriesOccurrence, starts_at: '2026-11-12T15:00:00Z', ends_at: '2026-11-12T16:00:00Z' };

    const prefill = nextSeriesAfterEnding(t, movedBeyond, seriesEndingOn('2026-11-04'), tutor.timezone);

    expect(prefill).toBeNull();
  });

  test('an answer whose series still has a count opens no new series', async () => {
    const { t } = await createI18n('ru');
    const counted: ChangedLessons = { series: { ...occurrenceSeries, end_date: null, count: 4 }, lessons: [] };

    const prefill = nextSeriesAfterEnding(t, seriesOccurrence, counted, tutor.timezone);

    expect(prefill).toBeNull();
  });

  test('a member without a name shows its fixed label alone', async () => {
    const i18n = await createI18n('ru');

    render(
      <I18nextProvider i18n={i18n}>
        <p data-part="member">
          <MemberLabel label={null} kind="deleted_account" />
        </p>
      </I18nextProvider>,
    );

    expect(document.querySelector('[data-part="member"]')?.textContent).toBe('Удалённый аккаунт');
  });
});
