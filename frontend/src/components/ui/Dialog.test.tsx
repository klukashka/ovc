import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { returnFocusTo } from './Dialog.tsx';

afterEach(() => {
  cleanup();
});

function pageWithOpener(): HTMLElement {
  render(
    <main>
      <h1 tabIndex={-1}>Урок</h1>
      <button type="button">Изменить урок</button>
    </main>,
  );
  return screen.getByRole('button');
}

describe('dialog focus return', () => {
  test('a closed dialog returns focus to its opener while the opener is shown', () => {
    const opener = pageWithOpener();

    returnFocusTo(opener);

    expect(document.activeElement).toBe(opener);
  });

  test('a closed dialog whose opener is gone moves focus to the page heading', () => {
    const opener = pageWithOpener();
    opener.focus();
    opener.remove();

    returnFocusTo(opener);

    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });

  test('a closed dialog without an opener moves focus to the page heading', () => {
    pageWithOpener();

    returnFocusTo(null);

    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });
});
