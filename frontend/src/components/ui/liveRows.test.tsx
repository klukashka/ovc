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

function RowsWithDialog(): ReactElement {
  const list = useRef<HTMLUListElement>(null);
  const opener = useRef<HTMLElement | null>(null);
  const [keys, setKeys] = useState<readonly string[]>(['first', 'second', 'third']);
  const [open, setOpen] = useState(false);
  const [closed, setClosed] = useState(false);
  useFocusAfterRemovedRow(list, keys, 'button');
  useEffect(() => {
    if (closed) {
      returnFocusTo(opener.current);
    }
  }, [closed]);
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
                setOpen(true);
              }}
            >
              {key}
            </button>
          </li>
        ))}
      </ul>
      {open ? (
        <dialog open aria-label="Delete second">
          <button
            type="button"
            onClick={() => {
              setKeys((shown) => shown.filter((key) => key !== 'second'));
            }}
          >
            Refetch without second
          </button>
          <button
            type="button"
            onClick={() => {
              setOpen(false);
              setClosed(true);
            }}
          >
            Cancel
          </button>
        </dialog>
      ) : null}
    </main>
  );
}

function openTheDialogFromTheSecondRowAndRemoveTheRow(): void {
  const second = screen.getByRole('button', { name: 'second' });
  second.focus();
  fireEvent.click(second);
  const refetch = screen.getByRole('button', { name: 'Refetch without second' });
  refetch.focus();
  fireEvent.click(refetch);
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
    render(<RowsWithDialog />);
    openTheDialogFromTheSecondRowAndRemoveTheRow();

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'third' }));
  });
});
