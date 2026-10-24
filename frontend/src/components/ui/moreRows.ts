import { type RefObject, useEffect, useRef } from 'react';

export interface MoreRows<Element extends HTMLElement> {
  readonly listRef: RefObject<Element | null>;
  readonly loadingMore: () => void;
}

export function useFocusOnFirstNewRow<Element extends HTMLElement>(
  rowCount: number,
  selector: string,
): MoreRows<Element> {
  const listRef = useRef<Element>(null);
  const firstNewRow = useRef<number | null>(null);

  useEffect(() => {
    const index = firstNewRow.current;
    if (index === null || rowCount <= index) {
      return;
    }
    firstNewRow.current = null;
    listRef.current?.querySelectorAll<HTMLElement>(selector)[index]?.focus();
  }, [rowCount, selector]);

  return {
    listRef,
    loadingMore: (): void => {
      firstNewRow.current = rowCount;
    },
  };
}
