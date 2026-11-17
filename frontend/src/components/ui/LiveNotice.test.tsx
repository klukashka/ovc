import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { type ReactElement, useState } from 'react';
import { afterEach, describe, expect, test } from 'vitest';

import { FocusedNotice } from './LiveNotice.tsx';

afterEach(() => {
  cleanup();
});

function NoticeBesideAControl({ whenFocusIsLost }: { readonly whenFocusIsLost: boolean }): ReactElement {
  const [shown, setShown] = useState(false);
  return (
    <main>
      <button
        type="button"
        onClick={() => {
          setShown(true);
        }}
      >
        Save
      </button>
      {shown ? <FocusedNotice message="Saved." whenFocusIsLost={whenFocusIsLost} /> : null}
    </main>
  );
}

function pressSave(): HTMLElement {
  const button = screen.getByRole('button', { name: 'Save' });
  button.focus();
  fireEvent.click(button);
  return button;
}

describe('the focused notice', () => {
  test('takes focus in the commit that shows it, before any animation frame', () => {
    render(<FocusedNotice message="Saved." />);

    const focused = document.activeElement;

    expect(focused).toBe(screen.getByRole('status'));
    expect(focused?.textContent).toBe('Saved.');
  });

  test('takes focus from the control that showed it', () => {
    render(<NoticeBesideAControl whenFocusIsLost={false} />);

    pressSave();

    expect(document.activeElement).toBe(screen.getByRole('status'));
  });

  test('shown only when focus is lost, leaves focus on a focused control', () => {
    render(<NoticeBesideAControl whenFocusIsLost />);

    const button = pressSave();

    expect(screen.getByRole('status').textContent).toBe('Saved.');
    expect(document.activeElement).toBe(button);
  });

  test('shown only when focus is lost, takes focus from the page body', () => {
    render(<FocusedNotice message="Gone." whenFocusIsLost />);

    const focused = document.activeElement;

    expect(focused).toBe(screen.getByRole('status'));
  });
});
