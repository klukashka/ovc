import { QueryClientProvider, useInfiniteQuery } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import type { ReactElement } from 'react';
import { afterEach, describe, expect, test } from 'vitest';

import { FollowingQueryClient } from '../../app/queryClient.ts';
import { eventually } from '../../lib/eventually.ts';
import { loadNextPage, useFocusOnFirstNewRow } from './moreRows.ts';

interface Page {
  readonly rows: readonly string[];
  readonly next: number | null;
}

interface HeldPage {
  readonly signal: AbortSignal;
  readonly answer: (page: Page) => void;
}

const listKey = ['issued'];

function Rows({ held }: { readonly held: HeldPage[] }): ReactElement {
  const list = useInfiniteQuery({
    queryKey: listKey,
    queryFn: ({ signal }): Promise<Page> =>
      new Promise((resolve) => {
        held.push({ signal, answer: resolve });
      }),
    initialPageParam: 0,
    getNextPageParam: (page): number | null => page.next,
    retry: false,
  });
  const rows = list.data?.pages.flatMap((page) => page.rows) ?? [];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(
    rows.length,
    ':scope > li button',
    list.isFetching,
  );
  return (
    <section>
      <ul ref={listRef}>
        {rows.map((row) => (
          <li key={row}>
            <button type="button">{row}</button>
          </li>
        ))}
      </ul>
      {list.hasNextPage ? (
        <button
          type="button"
          onClick={() => {
            loadingMore();
            void loadNextPage(list);
          }}
        >
          Show more
        </button>
      ) : null}
    </section>
  );
}

async function heldPage(held: readonly HeldPage[], index: number): Promise<HeldPage> {
  await eventually(() => {
    expect(held[index]).toBeDefined();
  });
  const found = held[index];
  if (found === undefined) {
    throw new Error(`page fetch ${String(index)} never started`);
  }
  return found;
}

afterEach(() => {
  cleanup();
});

describe('show more while the loaded page is refetched', () => {
  test('waits for the refetch, keeps it, and moves focus to the first new row', async () => {
    const client = new FollowingQueryClient();
    const held: HeldPage[] = [];
    render(
      <QueryClientProvider client={client}>
        <Rows held={held} />
      </QueryClientProvider>,
    );
    (await heldPage(held, 0)).answer({ rows: ['Fractions', 'Decimals'], next: 1 });
    const showMore = await screen.findByRole('button', { name: 'Show more' });
    showMore.focus();
    void client.refetchQueries({ queryKey: listKey });
    const refetch = await heldPage(held, 1);

    fireEvent.click(showMore);
    await act(async () => {
      refetch.answer({ rows: ['Fractions', 'Percentages'], next: 1 });
      (await heldPage(held, 2)).answer({ rows: ['Ratios'], next: null });
    });

    await eventually(() => {
      expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Ratios' }));
    });
    expect(refetch.signal.aborted).toBe(false);
    expect(held).toHaveLength(3);
    expect(screen.getByRole('button', { name: 'Percentages' })).toBeDefined();
  });
});
