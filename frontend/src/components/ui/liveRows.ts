import { type RefObject, useEffect, useLayoutEffect, useRef } from 'react';

import { focusIsLost } from './LoadState.tsx';

export const rowKeyAttribute = 'data-row';

interface FocusedRow {
  readonly key: string;
  readonly keys: readonly string[];
  readonly element: Element;
}

const replacementsOfRemovedRows = new WeakMap<Element, () => HTMLElement | null>();

export function replacementOfRemovedRow(target: HTMLElement): HTMLElement | null {
  const row = target.closest(`[${rowKeyAttribute}]`);
  if (row === null || row.isConnected) {
    return null;
  }
  return replacementsOfRemovedRows.get(row)?.() ?? null;
}

function inDialog(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest('dialog') !== null;
}

function pageHeading(): HTMLElement | null {
  return document.querySelector<HTMLElement>('main h1');
}

function rowOf(target: EventTarget | null, list: HTMLElement | null): Element | null {
  if (!(target instanceof Element) || list?.contains(target) !== true) {
    return null;
  }
  return target.closest(`[${rowKeyAttribute}]`);
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
      const row = rowOf(event.target, listRef.current);
      const key = row?.getAttribute(rowKeyAttribute) ?? null;
      if (row !== null && key !== null) {
        focused.current = { key, keys: currentKeys.current, element: row };
        return;
      }
      if (!inDialog(event.target)) {
        focused.current = null;
      }
    };
    document.addEventListener('focusin', follow);
    return (): void => {
      document.removeEventListener('focusin', follow);
      if (focused.current === null) {
        return;
      }
      focused.current = null;
      queueMicrotask(() => {
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
      focused.current = { key: row.key, keys: present, element: row.element };
      return;
    }
    focused.current = null;
    const before = row.keys.slice(0, row.keys.indexOf(row.key)).filter((key) => present.includes(key)).length;
    const replacementInRow = (): HTMLElement | null => {
      const rows = listRef.current?.querySelectorAll<HTMLElement>(`:scope > [${rowKeyAttribute}]`);
      const replacement = rows?.[before] ?? rows?.[before - 1] ?? null;
      return focusSelector === null ? replacement : (replacement?.querySelector<HTMLElement>(focusSelector) ?? null);
    };
    if (inDialog(document.activeElement)) {
      replacementsOfRemovedRows.set(row.element, replacementInRow);
      return;
    }
    if (paused || !focusIsLost()) {
      return;
    }
    const target = replacementInRow() ?? heading?.current ?? pageHeading();
    target?.focus();
  }, [joinedKeys, listRef, focusSelector, heading, paused]);
}
