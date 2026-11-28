import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { type ReactElement, useEffect, useRef, useState } from 'react';
import { afterEach, describe, expect, test } from 'vitest';

import { returnFocusTo } from './Dialog.tsx';
import { rowKeyAttribute, useFocusAfterRemovedRow } from './liveRows.ts';

afterEach(() => {
  cleanup();
});

function Rows({ keys }: { readonly keys: readonly string[] }): ReactElement {
  const list = useRef<HTMLUListElement>(null);
  useFocusAfterRemovedRow(list, keys, 'button');
  return (
    <ul ref={list}>
      {keys.map((key) => (
        <li key={key} {...{ [rowKeyAttribute]: key }}>
          <button type="button">{key}</button>
        </li>
      ))}
    </ul>
  );
}

function PageWithRows({ focusAfterRemoval }: { readonly focusAfterRemoval: boolean }): ReactElement {
  const [shown, setShown] = useState(true);
  return (
    <main>
      <h1 tabIndex={-1}>Lessons</h1>
      <button
        type="button"
        onClick={() => {
          setShown(false);
        }}
      >
        Remove the list
      </button>
      {shown ? (
        <Rows keys={['first', 'second']} />
      ) : (
        <p
          ref={(element) => {
            if (focusAfterRemoval) {
              element?.focus();
            }
          }}
          tabIndex={-1}
        >
          No lessons
        </p>
      )}
    </main>
  );
}

async function removeTheListAndLetTheCommitSettle(): Promise<void> {
  fireEvent.click(screen.getByRole('button', { name: 'Remove the list' }));
  await Promise.resolve();
}

function RowsWithDialog({
  keys: initialKeys,
  opener: openerKey,
}: {
  readonly keys: readonly string[];
  readonly opener: string;
}): ReactElement {
  const list = useRef<HTMLUListElement>(null);
  const dialog = useRef<HTMLDialogElement>(null);
  const opener = useRef<HTMLElement | null>(null);
  const [keys, setKeys] = useState<readonly string[]>(initialKeys);
  const [opened, setOpened] = useState(false);
  const [open, setOpen] = useState(false);
  useFocusAfterRemovedRow(list, keys, 'button');
  useEffect(() => {
    if (opened && !open) {
      dialog.current?.removeAttribute('open');
      returnFocusTo(opener.current);
    }
  }, [opened, open]);
  const removeTheOpenerRow = (): void => {
    setKeys((shown) => shown.filter((key) => key !== openerKey));
  };
  const close = (): void => {
    setOpen(false);
  };
  return (
    <main>
      <h1 tabIndex={-1}>Library</h1>
      <ul ref={list}>
        {keys.map((key) => (
          <li key={key} {...{ [rowKeyAttribute]: key }}>
            <button
              type="button"
              onClick={(event) => {
                opener.current = event.currentTarget;
                setOpened(true);
                setOpen(true);
              }}
            >
              {key}
            </button>
          </li>
        ))}
      </ul>
      {opened ? (
        <dialog ref={dialog} open aria-label="Delete">
          {open ? (
            <>
              <button type="button" onClick={removeTheOpenerRow}>
                Refetch without the row
              </button>
              <button type="button" onClick={close}>
                Cancel
              </button>
              <button
                type="button"
                onClick={() => {
                  removeTheOpenerRow();
                  close();
                }}
              >
                Delete
              </button>
            </>
          ) : null}
        </dialog>
      ) : null}
    </main>
  );
}

function openTheDialogFrom(key: string): void {
  const row = screen.getByRole('button', { name: key });
  row.focus();
  fireEvent.click(row);
  screen.getByRole('button', { name: 'Cancel' }).focus();
}

function openTheDialogFromAndRemoveTheRow(key: string): void {
  openTheDialogFrom(key);
  fireEvent.click(screen.getByRole('button', { name: 'Refetch without the row' }));
}

describe('the focus after a removed row', () => {
  test('a list that unmounts with focus in a row moves focus to the page heading after the commit, before any animation frame', async () => {
    render(<PageWithRows focusAfterRemoval={false} />);
    screen.getByRole('button', { name: 'second' }).focus();

    await removeTheListAndLetTheCommitSettle();

    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });

  test('a list that unmounts leaves focus where the same commit moved it', async () => {
    render(<PageWithRows focusAfterRemoval />);
    screen.getByRole('button', { name: 'second' }).focus();

    await removeTheListAndLetTheCommitSettle();

    expect(document.activeElement).toBe(screen.getByText('No lessons'));
  });

  test('a row removed while the dialog it opened is open hands focus to the row that took its place when the dialog closes', () => {
    render(<RowsWithDialog keys={['first', 'second', 'third']} opener="second" />);
    openTheDialogFromAndRemoveTheRow('second');

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'third' }));
  });

  test('a last row removed while the dialog it opened is open hands focus to the previous row when the dialog closes', () => {
    render(<RowsWithDialog keys={['first', 'second', 'third']} opener="third" />);
    openTheDialogFromAndRemoveTheRow('third');

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'second' }));
  });

  test('the only row removed while the dialog it opened is open hands focus to the page heading when the dialog closes', () => {
    render(<RowsWithDialog keys={['only']} opener="only" />);
    openTheDialogFromAndRemoveTheRow('only');

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });

  test('a row removed in the commit that closes the dialog it opened hands focus to the row that took its place', () => {
    render(<RowsWithDialog keys={['first', 'second', 'third']} opener="second" />);
    openTheDialogFrom('second');

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'third' }));
  });
});
