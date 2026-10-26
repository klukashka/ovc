import { QueryObserver } from '@tanstack/react-query';
import { describe, expect, test } from 'vitest';

import { FollowingQueryClient } from './queryClient.ts';

async function heldFetch(held: readonly HeldFetch[], index: number): Promise<HeldFetch> {
  const deadline = Date.now() + 2_000;
  for (let found = held[index]; ; found = held[index]) {
    if (found !== undefined) {
      return found;
    }
    if (Date.now() >= deadline) {
      throw new Error(`fetch ${String(index)} never started`);
    }
    await new Promise<void>((resolve) => {
      setTimeout(resolve, 5);
    });
  }
}

interface HeldFetch {
  readonly signal: AbortSignal;
  readonly answer: (value: number) => void;
}

describe('the following query client', () => {
  test('an invalidation during a fetch lets it finish and fetches once more instead of aborting it', async () => {
    const queryClient = new FollowingQueryClient();
    const held: HeldFetch[] = [];
    const observer = new QueryObserver(queryClient, {
      queryKey: ['students'],
      queryFn: ({ signal }): Promise<number> =>
        new Promise((resolve) => {
          held.push({ signal, answer: resolve });
        }),
    });
    const unsubscribe = observer.subscribe(() => undefined);
    const first = await heldFetch(held, 0);

    const invalidation = queryClient.invalidateQueries({ queryKey: ['students'] });
    first.answer(1);
    (await heldFetch(held, 1)).answer(2);
    await invalidation;

    expect(first.signal.aborted).toBe(false);
    expect(held).toHaveLength(2);
    expect(queryClient.isFetching()).toBe(0);
    expect(queryClient.getQueryData(['students'])).toBe(2);
    unsubscribe();
  });
});
