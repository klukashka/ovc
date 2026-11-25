import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { type ReactElement, useState } from 'react';
import { afterEach, describe, expect, test } from 'vitest';

import { ActionNotice } from './ActionNotice.tsx';

afterEach(() => {
  cleanup();
});

function descriptionOf(element: HTMLElement): string {
  return (element.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .map((id) => document.getElementById(id)?.textContent ?? '')
    .join(' ');
}

function RefusingNotice(): ReactElement {
  const [problem, setProblem] = useState<string | null>(null);
  return (
    <ActionNotice
      action={{
        label: 'Add back',
        busy: false,
        onActivate: () => {
          setProblem('Can’t add back: the group is archived.');
        },
      }}
      problem={problem}
    >
      Removed from the group: <bdi>Boris</bdi>. Their unsent files are not restored.
    </ActionNotice>
  );
}

describe('the action notice', () => {
  test('takes focus on its button, which its result text describes', () => {
    render(<RefusingNotice />);

    const button = screen.getByRole('button', { name: 'Add back' });

    expect(document.activeElement).toBe(button);
    expect(descriptionOf(button)).toBe('Removed from the group: Boris. Their unsent files are not restored.');
  });

  test('keeps focus on its button when the action is refused, and the refusal joins its description', () => {
    render(<RefusingNotice />);
    const button = screen.getByRole('button', { name: 'Add back' });

    fireEvent.click(button);

    expect(document.activeElement).toBe(button);
    expect(screen.getByRole('alert').textContent).toBe('Can’t add back: the group is archived.');
    expect(descriptionOf(button)).toBe(
      'Removed from the group: Boris. Their unsent files are not restored. Can’t add back: the group is archived.',
    );
  });
});
