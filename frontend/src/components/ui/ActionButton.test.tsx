import { act, cleanup, render, screen } from '@testing-library/react';
import { type ReactElement, useState } from 'react';
import { afterEach, describe, expect, test } from 'vitest';

import { FakeAuthServer, type PausedAnswers } from '../../lib/api/fakeTransport.ts';
import { SubmitButton } from './Form.tsx';

const origin = 'https://ovc.example';

function ReadingForm({ server }: { readonly server: FakeAuthServer }): ReactElement {
  const [busy, setBusy] = useState(false);
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        setBusy(true);
        void server
          .transport()
          .send(new Request(`${origin}/api/v1/users/me`))
          .finally(() => {
            setBusy(false);
          });
      }}
    >
      <label htmlFor="name">Name</label>
      <input id="name" name="name" />
      <SubmitButton busy={busy}>Save</SubmitButton>
    </form>
  );
}

async function submitWhilePaused(): Promise<{ readonly server: FakeAuthServer; readonly paused: PausedAnswers }> {
  const server = new FakeAuthServer();
  const paused = server.pauseAccountReads();
  render(<ReadingForm server={server} />);
  await act(async () => {
    screen.getByRole('button', { name: 'Save' }).click();
    await paused.answered;
  });
  return { server, paused };
}

afterEach(() => {
  cleanup();
});

function defaultButtonOf(field: HTMLElement): HTMLButtonElement {
  const button = field.closest('form')?.querySelector<HTMLButtonElement>('button[type="submit"]');
  if (button === null || button === undefined) {
    throw new Error('the form has no default button');
  }
  return button;
}

describe('a busy submit button', () => {
  test('ignores a click while its request runs', async () => {
    const { server, paused } = await submitWhilePaused();
    const button = screen.getByRole('button', { name: 'Save' });

    await act(async () => {
      button.click();
      await Promise.resolve();
    });
    const sentWhileBusy = server.sentTokens.length;
    await act(async () => {
      paused.resume();
      await Promise.resolve();
    });

    expect(sentWhileBusy).toBe(1);
    expect(button.hasAttribute('disabled')).toBe(false);
  });

  test('ignores an implicit submission from a field while its request runs', async () => {
    const { server, paused } = await submitWhilePaused();
    const field = screen.getByLabelText('Name');

    await act(async () => {
      field.focus();
      defaultButtonOf(field).click();
      await Promise.resolve();
    });
    const sentWhileBusy = server.sentTokens.length;
    await act(async () => {
      paused.resume();
      await Promise.resolve();
    });

    expect(sentWhileBusy).toBe(1);
    expect(document.activeElement).toBe(field);
  });
});
