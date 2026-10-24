import { type RefObject, useEffect, useRef } from 'react';

export interface MoreRows<Element extends HTMLElement> {
  readonly listRef: RefObject<Element | null>;
  readonly loadingMore: () => void;
}

export function useFocusOnFirstNewRow<Element extends HTMLElement>(
  rowCount: number,
  selector: string,
  fetching: boolean,
): MoreRows<Element> {
  const listRef = useRef<Element>(null);
  const firstNewRow = useRef<number | null>(null);

  useEffect(() => {
    const index = firstNewRow.current;
    if (index === null) {
      return;
    }
    if (rowCount <= index) {
      if (!fetching) {
        firstNewRow.current = null;
      }
      return;
    }
    firstNewRow.current = null;
    listRef.current?.querySelectorAll<HTMLElement>(selector)[index]?.focus();
  }, [rowCount, selector, fetching]);

  return {
    listRef,
    loadingMore: (): void => {
      firstNewRow.current = rowCount;
    },
  };
}
