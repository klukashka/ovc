import { act, cleanup, render, screen, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { createI18n } from '../../lib/i18n/i18n.ts';
import type { CalendarItem } from './api.ts';
import { rangeOf } from './calendarTime.ts';
import { WeekGrid } from './WeekGrid.tsx';

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

async function showWeek(items: readonly CalendarItem[]): Promise<void> {
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
      <I18nextProvider i18n={i18n}>
        <RouterProvider router={router} />
      </I18nextProvider>,
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

  test('a 15-minute lesson keeps a 24 px link with its time and status icon that never covers the next lesson', async () => {
    const short = lesson('c', 'Короткий урок', '2026-10-21T07:00:00Z', '2026-10-21T07:15:00Z');
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
    const link = within(shortBlock).getByRole('link', { name: /Короткий урок.*Запланирован/ });
    expect(link.getAttribute('data-layout')).toBe('compact');
    expect(link.getAttribute('title')).toBe('09:00–09:15 Короткий урок · Запланирован');
    expect(link.className.split(' ')).toContain('h-full');
    expect([...shortBlock.classList, ...link.classList].filter((name) => /^(p|py|pt|pb)-/.test(name))).toEqual([]);
    expect(link.querySelector('[data-part="time"]')?.textContent).toBe('09:00');
    expect(link.querySelector('[data-part="status"] svg')).not.toBeNull();
  });
});
