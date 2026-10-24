import { type RefObject, useEffect, useRef } from 'react';

import { focusIsLost } from './LoadState.tsx';

export interface MoreRows<Element extends HTMLElement> {
  readonly listRef: RefObject<Element | null>;
  readonly loadingMore: () => void;
}

function pageHeading(): HTMLElement | null {
  return document.querySelector<HTMLElement>('main h1');
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
