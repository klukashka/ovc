import { act, cleanup, render, screen } from '@testing-library/react';
import { type ReactElement, useRef } from 'react';
import { afterEach, describe, expect, test } from 'vitest';

import { useFocusOnFirstNewRow } from './moreRows.ts';

interface Shown {
  readonly rows: readonly string[];
  readonly fetching: boolean;
  readonly more: boolean;
}

function Rows({ shown }: { readonly shown: Shown }): ReactElement {
  const heading = useRef<HTMLHeadingElement>(null);
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(
    shown.rows.length,
    ':scope > li button',
    shown.fetching,
    heading,
  );
  return (
    <section>
      <h2 ref={heading} tabIndex={-1}>
        Rows
      </h2>
      {shown.rows.length === 0 ? null : (
        <ul ref={listRef}>
          {shown.rows.map((row) => (
            <li key={row}>
              <button type="button">{row}</button>
            </li>
          ))}
        </ul>
      )}
      {shown.more ? (
        <button type="button" onClick={loadingMore}>
          Show more
        </button>
      ) : null}
    </section>
  );
}

function showMoreThatAddsNothing(rows: readonly string[]): void {
  const { rerender } = render(<Rows shown={{ rows, fetching: false, more: true }} />);
  const showMore = screen.getByRole('button', { name: 'Show more' });
  showMore.focus();
  showMore.click();
  rerender(<Rows shown={{ rows, fetching: true, more: true }} />);
  act(() => {
    rerender(<Rows shown={{ rows, fetching: false, more: false }} />);
  });
}

afterEach(() => {
  cleanup();
});

describe('show more whose page adds no visible row', () => {
  test('moves focus to the last row shown', () => {
    const rows = ['Anna', 'Boris'];

    showMoreThatAddsNothing(rows);

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Boris' }));
  });

  test('moves focus to the fallback heading when no row is shown', () => {
    const rows: readonly string[] = [];

    showMoreThatAddsNothing(rows);

    expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Rows' }));
  });
});
