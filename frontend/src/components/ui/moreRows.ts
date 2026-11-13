import { type RefObject, useEffect, useRef } from 'react';

import { focusIsLost } from './LoadState.tsx';

export interface MoreRows<Element extends HTMLElement> {
  readonly listRef: RefObject<Element | null>;
  readonly loadingMore: () => void;
}

function pageHeading(): HTMLElement | null {
  return document.querySelector<HTMLElement>('main h1');
}

interface LoadedPages<Page> {
  readonly pages: readonly Page[];
}

interface PageLoad<Page> {
  readonly data: LoadedPages<Page> | undefined;
  readonly hasNextPage: boolean;
  readonly isError: boolean;
}

export interface PagedQuery<Page> {
  readonly data: LoadedPages<Page> | undefined;
  readonly fetchNextPage: (options: { readonly cancelRefetch: boolean }) => Promise<PageLoad<Page>>;
}

export async function loadNextPage<Page>(query: PagedQuery<Page>): Promise<void> {
  const loadedBefore = query.data?.pages.length ?? 0;
  const first = await query.fetchNextPage({ cancelRefetch: false });
  const joinedRefetch = (first.data?.pages.length ?? 0) <= loadedBefore;
  if (joinedRefetch && first.hasNextPage && !first.isError) {
    await query.fetchNextPage({ cancelRefetch: false });
  }
}

export function useFocusOnFirstNewRow<Element extends HTMLElement>(
  rowCount: number,
  selector: string,
  fetching: boolean,
  fallback: RefObject<HTMLElement | null> | null = null,
): MoreRows<Element> {
  const listRef = useRef<Element>(null);
  const firstNewRow = useRef<number | null>(null);

  useEffect(() => {
    const index = firstNewRow.current;
    if (index === null) {
      return;
    }
    const rows = listRef.current?.querySelectorAll<HTMLElement>(selector);
    if (rowCount > index) {
      firstNewRow.current = null;
      rows?.[index]?.focus();
      return;
    }
    if (fetching) {
      return;
    }
    firstNewRow.current = null;
    if (focusIsLost()) {
      (rows?.[rows.length - 1] ?? fallback?.current ?? pageHeading())?.focus();
    }
  }, [rowCount, selector, fetching, fallback]);

  return {
    listRef,
    loadingMore: (): void => {
      firstNewRow.current = rowCount;
    },
  };
}
