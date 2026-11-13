import { InfiniteQueryObserver } from '@tanstack/react-query';
import { afterEach, describe, expect, test } from 'vitest';

import { FollowingQueryClient } from '../../app/queryClient.ts';
import { eventually } from '../../lib/eventually.ts';
import { loadNextPage } from './moreRows.ts';

interface Page {
  readonly items: readonly string[];
  readonly next: number | null;
}

interface HeldPage {
  readonly cursor: number;
  readonly signal: AbortSignal;
  readonly answer: (page: Page) => void;
  readonly fail: (error: Error) => void;
}

interface PagedList {
  readonly observer: InfiniteQueryObserver<
    Page,
    Error,
    { pages: Page[]; pageParams: number[] },
    readonly string[],
    number
  >;
  readonly held: HeldPage[];
}

const subscriptions: (() => void)[] = [];

function pagedList(): PagedList {
  const held: HeldPage[] = [];
  const observer = new InfiniteQueryObserver<
    Page,
    Error,
    { pages: Page[]; pageParams: number[] },
    readonly string[],
    number
  >(new FollowingQueryClient(), {
    queryKey: ['issues'],
    queryFn: ({ pageParam, signal }): Promise<Page> =>
      new Promise((resolve, reject) => {
        held.push({ cursor: pageParam, signal, answer: resolve, fail: reject });
      }),
    initialPageParam: 0,
    getNextPageParam: (page): number | null => page.next,
    retry: false,
  });
  subscriptions.push(observer.subscribe(() => undefined));
  return { observer, held };
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

async function firstPageShown(list: PagedList, next: number): Promise<void> {
  (await heldPage(list.held, 0)).answer({ items: ['Fractions'], next });
  await eventually(() => {
    expect(list.observer.getCurrentResult().data?.pages).toHaveLength(1);
  });
}

async function refetchInFlight(list: PagedList): Promise<HeldPage> {
  void list.observer.refetch();
  return heldPage(list.held, 1);
}

afterEach(() => {
  for (const stop of subscriptions.splice(0)) {
    stop();
  }
});

describe('loading the next page', () => {
  test('a Show more during a refetch lets the refetch finish and then loads the page after its refreshed cursor', async () => {
    const list = pagedList();
    await firstPageShown(list, 1);
    const refetch = await refetchInFlight(list);

    const loading = loadNextPage(list.observer.getCurrentResult());
    refetch.answer({ items: ['Decimals'], next: 7 });
    const nextPage = await heldPage(list.held, 2);
    nextPage.answer({ items: ['Fractions'], next: null });
    await loading;

    expect(refetch.signal.aborted).toBe(false);
    expect(nextPage.cursor).toBe(7);
    expect(list.held).toHaveLength(3);
    expect(list.observer.getCurrentResult().data?.pages.map((page) => page.items)).toEqual([
      ['Decimals'],
      ['Fractions'],
    ]);
  });

  test('a Show more with no refetch in flight loads exactly one page', async () => {
    const list = pagedList();
    await firstPageShown(list, 1);

    const loading = loadNextPage(list.observer.getCurrentResult());
    const nextPage = await heldPage(list.held, 1);
    nextPage.answer({ items: ['Decimals'], next: 2 });
    await loading;

    expect(nextPage.cursor).toBe(1);
    expect(list.held).toHaveLength(2);
    expect(list.observer.getCurrentResult().data?.pages).toHaveLength(2);
  });

  test('a Show more during a refetch that fails loads no further page', async () => {
    const list = pagedList();
    await firstPageShown(list, 1);
    const refetch = await refetchInFlight(list);

    const loading = loadNextPage(list.observer.getCurrentResult());
    refetch.fail(new Error('refetch failed'));
    await loading;

    expect(list.held).toHaveLength(2);
    expect(list.observer.getCurrentResult().isError).toBe(true);
    expect(list.observer.getCurrentResult().data?.pages).toHaveLength(1);
  });

  test('a Show more during a refetch that leaves no next page loads no further page', async () => {
    const list = pagedList();
    await firstPageShown(list, 1);
    const refetch = await refetchInFlight(list);

    const loading = loadNextPage(list.observer.getCurrentResult());
    refetch.answer({ items: ['Decimals'], next: null });
    await loading;

    expect(list.held).toHaveLength(2);
    expect(list.observer.getCurrentResult().hasNextPage).toBe(false);
    expect(list.observer.getCurrentResult().data?.pages).toHaveLength(1);
  });

  test('two Show more presses during a refetch load exactly one page', async () => {
    const list = pagedList();
    await firstPageShown(list, 1);
    const refetch = await refetchInFlight(list);

    const first = loadNextPage(list.observer.getCurrentResult());
    const second = loadNextPage(list.observer.getCurrentResult());
    refetch.answer({ items: ['Decimals'], next: 3 });
    (await heldPage(list.held, 2)).answer({ items: ['Ratios'], next: null });
    await Promise.all([first, second]);

    expect(list.held).toHaveLength(3);
    expect(list.observer.getCurrentResult().data?.pages).toHaveLength(2);
  });
});
