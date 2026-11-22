import { act, cleanup, render, screen } from '@testing-library/react';
import { type ReactElement, useSyncExternalStore } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { ApiError } from '../../lib/api/client.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { LoadFailed } from './LoadState.tsx';

afterEach(() => {
  cleanup();
});

interface ReadState {
  readonly subscribe: (listener: () => void) => () => void;
  readonly failed: () => boolean;
  readonly refetch: () => void;
}

function readState(): ReadState {
  let failed = true;
  const listeners = new Set<() => void>();
  return {
    subscribe: (listener): (() => void) => {
      listeners.add(listener);
      return (): void => {
        listeners.delete(listener);
      };
    },
    failed: () => failed,
    refetch: (): void => {
      failed = false;
      listeners.forEach((listener) => {
        listener();
      });
    },
  };
}

const failure = new ApiError(500, { code: 'internal', message: 'internal', request_id: 'request-1' });

function focusPanelHeading(): void {
  document.querySelector<HTMLElement>('#panel-heading')?.focus();
}

function PageWithFailedRead({ read }: { readonly read: ReadState }): ReactElement {
  const failed = useSyncExternalStore(read.subscribe, read.failed);
  return (
    <main>
      <h1 tabIndex={-1}>Сегодня</h1>
      <button type="button">Другое действие</button>
      <section>
        <h2 id="panel-heading" tabIndex={-1}>
          Сообщения
        </h2>
        {failed ? <LoadFailed error={failure} onRetry={() => undefined} focusIfLost={focusPanelHeading} /> : null}
      </section>
      {failed ? <LoadFailed error={failure} onRetry={() => undefined} /> : <p>Загружается</p>}
    </main>
  );
}

async function renderPage(read: ReadState): Promise<void> {
  const i18n = await createI18n('ru');
  render(
    <I18nextProvider i18n={i18n}>
      <PageWithFailedRead read={read} />
    </I18nextProvider>,
  );
}

function retryButtons(): HTMLElement[] {
  return screen.getAllByRole('button', { name: 'Повторить' });
}

describe('the load failure', () => {
  test('removed by a live refetch while its Retry holds focus, moves focus to the page heading', async () => {
    const read = readState();
    await renderPage(read);
    retryButtons()[1]?.focus();

    act(() => {
      read.refetch();
    });

    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Сегодня' }));
  });

  test('inside a panel, removed while its Retry holds focus, moves focus to the panel’s heading', async () => {
    const read = readState();
    await renderPage(read);
    retryButtons()[0]?.focus();

    act(() => {
      read.refetch();
    });

    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 2, name: 'Сообщения' }));
  });

  test('removed while focus is elsewhere on the page, leaves focus where it is', async () => {
    const read = readState();
    await renderPage(read);
    const other = screen.getByRole('button', { name: 'Другое действие' });
    other.focus();

    act(() => {
      read.refetch();
    });

    expect(document.activeElement).toBe(other);
  });
});
