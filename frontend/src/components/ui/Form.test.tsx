import { act, cleanup, render, screen } from '@testing-library/react';
import { type ReactElement, useState } from 'react';
import { afterEach, describe, expect, test } from 'vitest';

import { FormNotice } from './Form.tsx';

function SavingForm(): ReactElement {
  const [saved, setSaved] = useState(false);
  return (
    <main>
      <section>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            setSaved(true);
          }}
        >
          {saved ? <FormNotice takeFocus>Saved.</FormNotice> : null}
          <label htmlFor="notes">Notes</label>
          <textarea id="notes" />
          <button type="submit">Save</button>
        </form>
      </section>
      <section>
        <label htmlFor="elsewhere">Elsewhere</label>
        <input id="elsewhere" />
      </section>
    </main>
  );
}

async function saveWithFocusOn(label: string | null): Promise<void> {
  render(<SavingForm />);
  const button = screen.getByRole('button', { name: 'Save' });
  const form = button.closest('form');
  if (form === null) {
    throw new Error('the button is outside its form');
  }
  await act(async () => {
    if (label === null) {
      button.focus();
    } else {
      screen.getByLabelText(label).focus();
    }
    form.requestSubmit();
    await Promise.resolve();
  });
}

afterEach(() => {
  cleanup();
});

describe('a notice that takes focus', () => {
  test('takes focus from the control that triggered it', async () => {
    await saveWithFocusOn(null);

    const notice = screen.getByRole('status');

    expect(document.activeElement).toBe(notice);
  });

  test('takes focus when the focused control disappeared', async () => {
    render(<SavingForm />);
    const button = screen.getByRole('button', { name: 'Save' });
    await act(async () => {
      button.focus();
      button.remove();
      screen.getByLabelText('Notes').closest('form')?.requestSubmit();
      await Promise.resolve();
    });

    const notice = screen.getByRole('status');

    expect(document.activeElement).toBe(notice);
  });

  test('leaves focus in a text field of the same form', async () => {
    await saveWithFocusOn('Notes');

    const notes = screen.getByLabelText('Notes');

    expect(document.activeElement).toBe(notes);
  });

  test('leaves focus in another section', async () => {
    await saveWithFocusOn('Elsewhere');

    const elsewhere = screen.getByLabelText('Elsewhere');

    expect(document.activeElement).toBe(elsewhere);
  });
});
