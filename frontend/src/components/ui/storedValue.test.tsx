import { cleanup, render, screen } from '@testing-library/react';
import type { ReactElement } from 'react';
import { afterEach, describe, expect, test } from 'vitest';

import { TextField } from './Form.tsx';

interface Stored {
  readonly name: string;
  readonly reloads: number;
}

function field(stored: Stored): ReactElement {
  return <TextField label="Name" name="name" autoComplete="off" defaultValue={stored.name} reloads={stored.reloads} />;
}

function nameAfter(typed: string | null, changes: readonly Stored[]): string {
  const { rerender } = render(field({ name: 'Anna', reloads: 0 }));
  const input = screen.getByLabelText<HTMLInputElement>('Name');
  if (typed !== null) {
    input.value = typed;
  }
  for (const change of changes) {
    rerender(field(change));
  }
  return input.value;
}

afterEach(() => {
  cleanup();
});

describe('a field showing a stored value', () => {
  test('follows a changed stored value while the user has not edited it', () => {
    const changes = [{ name: 'Boris', reloads: 0 }];

    const shown = nameAfter(null, changes);

    expect(shown).toBe('Boris');
  });

  test('keeps the user’s typing when the stored value changes in the background', () => {
    const changes = [{ name: 'Boris', reloads: 0 }];

    const shown = nameAfter('Typed', changes);

    expect(shown).toBe('Typed');
  });

  test('shows the changed stored value after a reload because the record changed meanwhile', () => {
    const changes = [
      { name: 'Boris', reloads: 0 },
      { name: 'Boris', reloads: 1 },
    ];

    const shown = nameAfter('Typed', changes);

    expect(shown).toBe('Boris');
  });

  test('keeps the user’s typing after such a reload when its stored value did not change', () => {
    const changes = [{ name: 'Anna', reloads: 1 }];

    const shown = nameAfter('Typed', changes);

    expect(shown).toBe('Typed');
  });
});
