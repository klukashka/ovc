import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { type ReactElement, useRef, useState } from 'react';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { RouteFocus } from '../frame/RouteFocus.tsx';
import { useFormPreset, useFormResult, useLeaveFormRoute, useOpenFormRoute, useReturnFocus } from './formRoute.ts';

function OriginPage({ holdsOpener }: { readonly holdsOpener: boolean }): ReactElement {
  const open = useOpenFormRoute('account-1');
  const issued = useFormResult('assignmentIssued');
  const opener = useRef<HTMLButtonElement>(null);
  useReturnFocus('row-menu', opener, holdsOpener);
  return (
    <main>
      <h1 tabIndex={-1}>Library</h1>
      {issued === null ? null : <p>Issued {issued.issueId}</p>}
      {holdsOpener ? (
        <button
          ref={opener}
          type="button"
          onClick={() => {
            open('/form', 'row-menu', { audience: 'Anna' });
          }}
        >
          Issue
        </button>
      ) : null}
      <button
        type="button"
        onClick={() => {
          open('/form', 'gone-opener');
        }}
      >
        Issue from elsewhere
      </button>
    </main>
  );
}

function FormPage({ account }: { readonly account: string }): ReactElement {
  const { leave } = useLeaveFormRoute('/fallback');
  const preset = useFormPreset(account);
  return (
    <main>
      <h1 tabIndex={-1}>Issue assignment</h1>
      <p>{JSON.stringify(preset ?? null)}</p>
      <button
        type="button"
        onClick={() => {
          leave();
        }}
      >
        Cancel
      </button>
      <button
        type="button"
        onClick={() => {
          leave({ kind: 'assignmentIssued', issueId: 'issue-1' });
        }}
      >
        Issue
      </button>
    </main>
  );
}

function DelayedOrigin(): ReactElement {
  const [loaded, setLoaded] = useState(false);
  return loaded ? (
    <OriginPage holdsOpener />
  ) : (
    <main>
      <h1 tabIndex={-1}>Library</h1>
      <button
        type="button"
        onClick={() => {
          setLoaded(true);
        }}
      >
        Finish loading
      </button>
    </main>
  );
}

let renderedRoutes = 0;

function renderRoutes(
  origin: ReactElement,
  initialEntries: readonly string[] = ['/origin'],
  formAccount = 'account-1',
): void {
  const router = createMemoryRouter(
    [
      {
        element: <RouteFocus />,
        children: [
          { path: '/origin', element: origin },
          { path: '/form', element: <FormPage account={formAccount} /> },
          { path: '/fallback', element: <OriginPage holdsOpener={false} /> },
        ],
      },
    ],
    {
      initialEntries: initialEntries.map((pathname) => ({
        pathname,
        key: `${pathname}-${String((renderedRoutes += 1))}`,
      })),
    },
  );
  render(<RouterProvider router={router} />);
}

async function press(name: string): Promise<void> {
  await act(async () => {
    screen.getByRole('button', { name }).click();
    await Promise.resolve();
  });
}

afterEach(() => {
  cleanup();
});

describe('a form route opened from a control', () => {
  test('arriving from a control focuses the form heading', async () => {
    renderRoutes(<OriginPage holdsOpener />);

    await press('Issue');

    expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Issue assignment' }));
  });

  test('Cancel returns focus to the opening control', async () => {
    renderRoutes(<OriginPage holdsOpener />);
    await press('Issue');

    await press('Cancel');

    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Issue' }));
    });
  });

  test('Cancel leaves focus on the origin heading while the origin page is still loading', async () => {
    renderRoutes(<DelayedOrigin />);
    await press('Finish loading');
    await press('Issue');

    await press('Cancel');

    expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Library' }));
  });

  test('focus returns to the opener once the origin page renders it again', async () => {
    renderRoutes(<DelayedOrigin />);
    await press('Finish loading');
    await press('Issue');
    await press('Cancel');

    await act(async () => {
      screen.getByRole('button', { name: 'Finish loading' }).dispatchEvent(new MouseEvent('click', { bubbles: true }));
      await Promise.resolve();
    });

    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Issue' }));
    });
  });

  test('focus stays on the origin heading when the origin page no longer holds the opener', async () => {
    renderRoutes(<OriginPage holdsOpener />);
    await press('Issue from elsewhere');

    await press('Cancel');

    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Library' }));
    });
  });

  test('a saved result reaches the origin page and leaves focus to its notice instead of the opener', async () => {
    renderRoutes(<OriginPage holdsOpener />);
    await press('Issue');

    await press('Issue');

    await waitFor(() => {
      expect(screen.getByText('Issued issue-1')).toBeDefined();
    });
    expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Library' }));
  });

  test('a form reached directly leaves to its fallback page', async () => {
    renderRoutes(<OriginPage holdsOpener />, ['/form']);

    await press('Cancel');

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Issue from elsewhere' })).toBeDefined();
    });
    expect(screen.queryByRole('button', { name: 'Issue' })).toBeNull();
  });

  test('the form shows the preset of the account that opened it', async () => {
    renderRoutes(<OriginPage holdsOpener />);

    await press('Issue');

    expect(screen.getByText('{"audience":"Anna"}')).toBeDefined();
  });

  test('a preset written by one account is not shown to another account', async () => {
    renderRoutes(<OriginPage holdsOpener />, ['/origin'], 'account-2');

    await press('Issue');

    expect(screen.getByText('null')).toBeDefined();
    expect(screen.queryByText('{"audience":"Anna"}')).toBeNull();
  });
});
