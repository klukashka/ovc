import { act, cleanup, render, screen, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { type ManualScheduler, manualScheduler } from '../../lib/api/fakeTransport.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { CalendarItem } from './api.ts';
import { rangeOf } from './calendarTime.ts';
import { WeekGrid, openingHour } from './WeekGrid.tsx';

afterEach(() => {
  cleanup();
});

function lesson(id: string, title: string, startsAt: string, endsAt: string): CalendarItem {
  return {
    id,
    title,
    starts_at: startsAt,
    ends_at: endsAt,
    status: 'scheduled',
    took_place: false,
    timezone: 'Europe/Berlin',
    audience: { kind: 'one_to_one', label: 'Анна', label_kind: null, colour: 'moss' },
    tutor: null,
  };
}

function shownWeek(): ReturnType<typeof rangeOf> {
  return rangeOf('week', '2026-10-21', 'Europe/Berlin');
}

async function showWeek(items: readonly CalendarItem[], scheduler: ManualScheduler = manualScheduler()): Promise<void> {
  const i18n = await createI18n('ru');
  const range = rangeOf('week', '2026-10-21', 'Europe/Berlin');
  const router = createMemoryRouter(
    [
      {
        path: '*',
        element: (
          <WeekGrid
            range={range}
            items={items}
            timeZone="Europe/Berlin"
            language="ru"
            today="2026-10-21"
            onSlot={null}
          />
        ),
      },
    ],
    { initialEntries: ['/schedule'] },
  );
  await act(async () => {
    render(
      <SchedulerContext value={scheduler}>
        <I18nextProvider i18n={i18n}>
          <RouterProvider router={router} />
        </I18nextProvider>
      </SchedulerContext>,
    );
    await Promise.resolve();
  });
}

function blockOf(title: string): HTMLElement {
  const link = screen.getByRole('link', { name: new RegExp(title) });
  const block = link.closest<HTMLElement>('[data-lesson]');
  if (block === null) {
    throw new Error(`${title} is not placed in the grid`);
  }
  return block;
}

describe('week grid', () => {
  test('opens one hour before the next lesson of the week, not at an earlier one that has ended', () => {
    const early = lesson('e', 'Ранний урок', '2026-10-21T05:00:00Z', '2026-10-21T06:00:00Z');
    const late = lesson('l', 'Поздний урок', '2026-10-21T19:00:00Z', '2026-10-21T20:00:00Z');

    const hour = openingHour(shownWeek(), [late, early], 'Europe/Berlin', Date.parse('2026-10-21T10:00:00Z'));

    expect(hour).toBe(20);
  });

  test('opens one hour before the earliest lesson when every lesson of the week has ended', () => {
    const early = lesson('e', 'Ранний урок', '2026-10-19T05:30:00Z', '2026-10-19T06:30:00Z');
    const late = lesson('l', 'Поздний урок', '2026-10-21T19:00:00Z', '2026-10-21T20:00:00Z');

    const hour = openingHour(shownWeek(), [late, early], 'Europe/Berlin', Date.parse('2026-10-26T10:00:00Z'));

    expect(hour).toBe(6.5);
  });

  test('opens at 08:00 in a week without lessons, and at midnight for a lesson that starts the day before', () => {
    const overnight = lesson('o', 'Ночной урок', '2026-10-18T21:30:00Z', '2026-10-18T23:30:00Z');

    const empty = openingHour(shownWeek(), [], 'Europe/Berlin', Date.parse('2026-10-21T10:00:00Z'));
    const crossing = openingHour(shownWeek(), [overnight], 'Europe/Berlin', Date.parse('2026-10-18T10:00:00Z'));

    expect(empty).toBe(8);
    expect(crossing).toBe(0);
  });

  test("places lessons in the viewer's timezone across a daylight-saving change", async () => {
    const beforeChange = lesson('a', 'Перед переводом', '2026-10-24T16:00:00Z', '2026-10-24T17:00:00Z');
    const afterChange = lesson('b', 'После перевода', '2026-10-25T17:00:00Z', '2026-10-25T18:00:00Z');

    await showWeek([beforeChange, afterChange]);

    const saturday = blockOf('Перед переводом');
    const sunday = blockOf('После перевода');
    expect(saturday.closest('[data-day]')?.getAttribute('data-day')).toBe('2026-10-24');
    expect(sunday.closest('[data-day]')?.getAttribute('data-day')).toBe('2026-10-25');
    expect(saturday.style.top).toBe(sunday.style.top);
    expect(saturday.style.top).toBe(`${String((18 / 24) * 100)}%`);
    expect(saturday.style.height).toBe(`${String((1 / 24) * 100)}%`);
    expect(within(saturday).getByText('18:00–19:00')).toBeDefined();
    expect(within(sunday).getByText('18:00–19:00')).toBeDefined();
  });

  test('a scheduled 15-minute lesson shows its time without a status icon or word', async () => {
    const short = lesson('c', 'Короткий урок', '2026-10-21T07:00:00Z', '2026-10-21T07:15:00Z');

    await showWeek([short]);

    const link = within(blockOf('Короткий урок')).getByRole('link', { name: /Короткий урок/ });
    expect(link.getAttribute('title')).toBe('09:00–09:15 Короткий урок');
    expect(link.querySelector('[data-part="time"]')?.textContent).toBe('09:00');
    expect(link.querySelector('[data-part="status"]')).toBeNull();
    expect(link.textContent).not.toContain('Запланирован');
  });

  test('a 15-minute lesson keeps a 24 px link with its time and status icon that never covers the next lesson', async () => {
    const short = {
      ...lesson('c', 'Короткий урок', '2026-10-21T07:00:00Z', '2026-10-21T07:15:00Z'),
      status: 'cancelled' as const,
    };
    const next = lesson('d', 'Следующий урок', '2026-10-21T07:15:00Z', '2026-10-21T08:15:00Z');

    await showWeek([short, next]);

    const columnHeight = 24 * 48;
    const shortBlock = blockOf('Короткий урок');
    const nextBlock = blockOf('Следующий урок');
    const [shortBox, nextBox] = [shortBlock, nextBlock].map((block) => ({
      top: parseFloat(block.style.top),
      bottom: parseFloat(block.style.top) + parseFloat(block.style.height),
      left: parseFloat(block.style.left),
      right: parseFloat(block.style.left) + parseFloat(block.style.width),
    }));
    const heightInPixels = Math.round((parseFloat(shortBlock.style.height) / 100) * columnHeight * 1000) / 1000;
    const coversVertically = shortBox !== undefined && nextBox !== undefined && shortBox.bottom > nextBox.top;
    const coversHorizontally =
      shortBox !== undefined && nextBox !== undefined && shortBox.right > nextBox.left && nextBox.right > shortBox.left;
    expect(heightInPixels).toBeGreaterThanOrEqual(24);
    expect(coversVertically && coversHorizontally).toBe(false);
    const link = within(shortBlock).getByRole('link', { name: /Короткий урок.*Отменён/ });
    expect(link.getAttribute('data-layout')).toBe('compact');
    expect(link.getAttribute('title')).toBe('09:00–09:15 Короткий урок · Отменён');
    expect(link.className.split(' ')).toContain('h-full');
    expect([...shortBlock.classList, ...link.classList].filter((name) => /^(p|py|pt|pb)-/.test(name))).toEqual([]);
    expect(link.querySelector('[data-part="time"]')?.textContent).toBe('09:00');
    expect(link.querySelector('[data-part="status"] svg')).not.toBeNull();
  });
});
