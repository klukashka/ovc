import { type RefObject, useEffect, useLayoutEffect, useRef } from 'react';

import { focusIsLost } from './LoadState.tsx';

export const rowKeyAttribute = 'data-row';

interface FocusedRow {
  readonly key: string;
  readonly keys: readonly string[];
}

function pageHeading(): HTMLElement | null {
  return document.querySelector<HTMLElement>('main h1');
}

function rowKeyOf(target: EventTarget | null, list: HTMLElement | null): string | null {
  if (!(target instanceof Element) || list?.contains(target) !== true) {
    return null;
  }
  return target.closest(`[${rowKeyAttribute}]`)?.getAttribute(rowKeyAttribute) ?? null;
}

export function useFocusAfterRemovedRow(
  listRef: RefObject<HTMLElement | null>,
  keys: readonly string[],
  focusSelector: string | null,
  heading: RefObject<HTMLElement | null> | null = null,
  paused = false,
): void {
  const focused = useRef<FocusedRow | null>(null);
  const currentKeys = useRef<readonly string[]>([]);
  const joinedKeys = keys.join('\n');

  useEffect(() => {
    const follow = (event: FocusEvent): void => {
      const key = rowKeyOf(event.target, listRef.current);
      focused.current = key === null ? null : { key, keys: currentKeys.current };
    };
    document.addEventListener('focusin', follow);
    return (): void => {
      document.removeEventListener('focusin', follow);
      if (focused.current === null) {
        return;
      }
      focused.current = null;
      window.requestAnimationFrame(() => {
        if (focusIsLost()) {
          pageHeading()?.focus();
        }
      });
    };
  }, [listRef]);

  useLayoutEffect(() => {
    const row = focused.current;
    const present = joinedKeys.split('\n');
    currentKeys.current = present;
    if (row === null) {
      return;
    }
    if (present.includes(row.key)) {
      focused.current = { key: row.key, keys: present };
      return;
    }
    focused.current = null;
    if (paused || !focusIsLost()) {
      return;
    }
    const before = row.keys.slice(0, row.keys.indexOf(row.key)).filter((key) => present.includes(key)).length;
    const rows = listRef.current?.querySelectorAll<HTMLElement>(`:scope > [${rowKeyAttribute}]`);
    const replacement = rows?.[before] ?? rows?.[before - 1] ?? null;
    const inRow =
      focusSelector === null ? replacement : (replacement?.querySelector<HTMLElement>(focusSelector) ?? null);
    const target = inRow ?? heading?.current ?? pageHeading();
    target?.focus();
  }, [joinedKeys, listRef, focusSelector, heading, paused]);
}
