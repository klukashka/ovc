import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { type ReactElement, useState } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { ConnectionContext, ConnectionState } from '../../app/connectionState.ts';
import { type ManualScheduler, manualScheduler } from '../../lib/api/fakeTransport.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { ActionButton, primaryButtonClass } from '../ui/Form.tsx';
import { useChatWrite } from '../ui/chatWrite.ts';
import { OfflineBanner } from './OfflineBanner.tsx';

function Composer({
  busy = false,
  unavailable = false,
}: {
  readonly busy?: boolean;
  readonly unavailable?: boolean;
}): ReactElement {
  const write = useChatWrite(busy, unavailable);
  const [draft, setDraft] = useState('Unsent text');
  const [sent, setSent] = useState(0);
  const [reads, setReads] = useState(0);
  return (
    <>
      <form
        aria-label="Composer"
        onSubmit={(event) => {
          event.preventDefault();
          write.run(() => {
            setSent(sent + 1);
            setDraft('');
          });
        }}
      >
        <label>
          Message
          <textarea
            value={draft}
            onChange={(event) => {
              setDraft(event.target.value);
            }}
          />
        </label>
        <ActionButton
          type="submit"
          busy={busy}
          unavailable={write.unavailable}
          unavailableReason={write.reason}
          className={primaryButtonClass}
        >
          Send
        </ActionButton>
      </form>
      <output aria-label="Sent">{sent}</output>
      <ActionButton
        busy={false}
        onClick={() => {
          setReads(reads + 1);
        }}
        className={primaryButtonClass}
      >
        Read
      </ActionButton>
      <output aria-label="Reads">{reads}</output>
      <a href="#other">Other page</a>
    </>
  );
}

async function fixture(
  busy = false,
  unavailable = false,
): Promise<{ clock: ManualScheduler; connection: ConnectionState }> {
  const clock = manualScheduler();
  const connection = new ConnectionState(clock);
  connection.account(1);
  const i18n = await createI18n('en');
  render(
    <I18nextProvider i18n={i18n}>
      <ConnectionContext value={connection}>
        <OfflineBanner />
        <Composer busy={busy} unavailable={unavailable} />
      </ConnectionContext>
    </I18nextProvider>,
  );
  return { clock, connection };
}

afterEach(cleanup);

describe('offline banner and chat writes', () => {
  test('appears after five seconds and announces recovery without moving focus', async () => {
    const { clock, connection } = await fixture();
    const field = screen.getByRole('textbox');
    field.focus();

    act(() => {
      clock.pass(4999);
    });
    const before = document.querySelector('[aria-live="polite"]')?.textContent;
    act(() => {
      clock.pass(1);
    });
    const offline = document.querySelector('[aria-live="polite"]')?.textContent;
    act(() => {
      connection.readiness(1, true);
    });

    expect(before).toBe('');
    expect(offline).toBe('No connection — reconnecting…');
    expect(document.querySelector('[aria-live="polite"]')?.textContent).toBe('Connection restored');
    expect(document.activeElement).toBe(field);
    expect(field).toBeInstanceOf(HTMLTextAreaElement);
    expect(field instanceof HTMLTextAreaElement ? field.value : null).toBe('Unsent text');
  });

  test('blocks chat clicks and form submission while ordinary controls remain usable', async () => {
    const { clock } = await fixture();
    const send = screen.getByRole('button', { name: 'Send' });
    send.focus();

    act(() => {
      clock.pass(5000);
    });
    fireEvent.click(send);
    fireEvent.submit(screen.getByRole('form', { name: 'Composer' }));
    fireEvent.click(screen.getByRole('button', { name: 'Read' }));

    expect(send.getAttribute('aria-disabled')).toBe('true');
    expect(send.hasAttribute('disabled')).toBe(false);
    expect(send.title).toBe('No connection');
    expect(document.getElementById(send.getAttribute('aria-describedby') ?? '')?.textContent).toBe('No connection');
    expect(document.activeElement).toBe(send);
    expect(screen.getByDisplayValue('Unsent text')).toBe(screen.getByRole('textbox'));
    expect(screen.getByLabelText('Sent').textContent).toBe('0');
    expect(screen.getByLabelText('Reads').textContent).toBe('1');
    expect(screen.getByRole('link', { name: 'Other page' }).getAttribute('href')).toBe('#other');
  });

  test.each([
    { busy: true, unavailable: false },
    { busy: false, unavailable: true },
  ])('recovery preserves independent refusal $busy/$unavailable', async ({ busy, unavailable }) => {
    const { clock, connection } = await fixture(busy, unavailable);
    act(() => {
      clock.pass(5000);
    });

    act(() => {
      connection.readiness(1, true);
    });
    fireEvent.click(screen.getByRole('button', { name: 'Send' }));
    fireEvent.submit(screen.getByRole('form', { name: 'Composer' }));

    expect(screen.getByRole('button', { name: 'Send' }).getAttribute('aria-disabled')).toBe('true');
    expect(screen.getByRole('button', { name: 'Send' }).hasAttribute('title')).toBe(false);
    expect(screen.getByLabelText('Sent').textContent).toBe('0');
    expect(screen.getByDisplayValue('Unsent text')).toBe(screen.getByRole('textbox'));
  });
});
