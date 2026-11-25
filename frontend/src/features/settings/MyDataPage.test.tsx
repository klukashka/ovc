import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import type { i18n as I18n } from 'i18next';
import { type ReactElement, type ReactNode, useRef } from 'react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { SchedulerContext } from '../../app/clock.ts';
import { ConnectionState } from '../../app/connectionState.ts';
import { DocumentLifecycle } from '../../app/documentLifecycle.ts';
import { FollowingQueryClient } from '../../app/queryClient.ts';
import { wireRealtime } from '../../app/realtime.ts';
import { type ControlledVisibility, VisibilityContext, controlledVisibility } from '../../app/visibility.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import {
  FakeAuthServer,
  type ManualScheduler,
  fakeStorageOrigin,
  manualScheduler,
  memoryLocks,
} from '../../lib/api/fakeTransport.ts';
import { type Me, Session, SessionContext } from '../../lib/api/session.ts';
import { eventually } from '../../lib/eventually.ts';
import { formatDateTime } from '../../lib/format/dates.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { FakeGateway } from '../../lib/realtime/fakeGateway.ts';
import { type CurrentExport, type Export, type ExportFailure, currentExportKey } from './exportApi.ts';
import { MyDataPage } from './MyDataPage.tsx';
import { myDataTexts as englishTexts } from './myDataTexts.en.ts';
import { myDataTexts as russianTexts } from './myDataTexts.ru.ts';
import { RequestExportForm } from './RequestExportDialog.tsx';

const origin = 'https://ovc.example';
const email = 'person@example.org';
const password = 'a long password';
const accountId = 'account-1';
const hourMilliseconds = 3_600_000;

interface ShownPage {
  readonly server: FakeAuthServer;
  readonly clock: ManualScheduler;
  readonly visibility: ControlledVisibility;
  readonly queryClient: QueryClient;
}

function fakeServer(state: Me['state'] = 'active'): FakeAuthServer {
  const server = new FakeAuthServer();
  server.addAccount({ id: accountId, name: 'A person', email, password, state });
  return server;
}

function signedOutSession(server: FakeAuthServer): Session {
  return new Session({ origin, transport: server.transport(), locks: memoryLocks(), scheduler: manualScheduler() });
}

function pageClock(server: FakeAuthServer): ManualScheduler {
  const clock = manualScheduler();
  clock.suspend(server.serverNow());
  return clock;
}

function instantAfter(server: FakeAuthServer, milliseconds: number): string {
  return new Date(server.serverNow() + milliseconds).toISOString();
}

function anExport(server: FakeAuthServer, change: Partial<Export>): Export {
  return {
    id: '00000000-0000-4000-a000-0000000000ff',
    state: 'queued',
    failure: null,
    requested_at: new Date(server.serverNow()).toISOString(),
    built_at: null,
    expires_at: null,
    size_bytes: null,
    progress: null,
    next_allowed_at: null,
    ...change,
  };
}

function readyExport(server: FakeAuthServer): Export {
  return anExport(server, {
    state: 'ready',
    built_at: new Date(server.serverNow()).toISOString(),
    expires_at: instantAfter(server, 72 * hourMilliseconds),
    size_bytes: 2_500_000,
  });
}

async function english(): Promise<I18n> {
  const i18n = await createI18n('en');
  i18n.addResourceBundle('en', 'translation', { myData: englishTexts }, true, true);
  i18n.addResourceBundle('ru', 'translation', { myData: russianTexts }, true, true);
  return i18n;
}

function providers(
  session: Session,
  queryClient: QueryClient,
  clock: ManualScheduler,
  visibility: ControlledVisibility,
  i18n: I18n,
  children: ReactNode,
): ReactElement {
  return (
    <SessionContext value={session}>
      <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
        <QueryClientProvider client={queryClient}>
          <SchedulerContext value={clock}>
            <VisibilityContext value={visibility}>
              <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
            </VisibilityContext>
          </SchedulerContext>
        </QueryClientProvider>
      </ApiClientContext>
    </SessionContext>
  );
}

async function showPage(
  server: FakeAuthServer,
  queryClient: QueryClient = new QueryClient(),
  session?: Session,
): Promise<ShownPage> {
  const signedIn = session ?? signedOutSession(server);
  if (session === undefined) {
    await signedIn.signIn(email, password);
  }
  const clock = pageClock(server);
  const visibility = controlledVisibility();
  const i18n = await english();
  const router = createMemoryRouter(
    [
      {
        path: '*',
        element: (
          <main>
            <MyDataPage />
          </main>
        ),
      },
    ],
    { initialEntries: ['/settings/data'] },
  );
  await act(async () => {
    render(providers(signedIn, queryClient, clock, visibility, i18n, <RouterProvider router={router} />));
    await Promise.resolve();
  });
  return { server, clock, visibility, queryClient };
}

async function shownExport(
  server: FakeAuthServer,
  current: Export | null,
  nextAllowedAt?: string,
  queryClient: QueryClient = new QueryClient(),
): Promise<ShownPage> {
  server.seedExport(email, current, nextAllowedAt ?? null);
  const shown = await showPage(server, queryClient);
  await screen.findByRole('heading', { level: 2, name: 'Export of your data' });
  await eventually(() => {
    expect(server.exportReads).toBe(1);
    expect(screen.queryByText('Exports are busy right now. Try again in a minute.')).toBeNull();
  });
  await screen.findByText(
    /^(You have no export yet\.|Your export is|The export (was|grew|has)|Preparing the export|Something went wrong while)/,
  );
  return shown;
}

function button(name: string): HTMLElement {
  return screen.getByRole('button', { name });
}

function mintRequests(server: FakeAuthServer): number {
  return server.sentAddresses.filter((address) => address.pathname === '/api/v1/exports/current/download').length;
}

interface RecordedLink {
  readonly href: string;
  readonly download: string | null;
  readonly rel: string;
  readonly element: HTMLAnchorElement;
}

const listenersToRemove: AbortController[] = [];

function recordDownloadLinks(): RecordedLink[] {
  const recorded: RecordedLink[] = [];
  const listening = new AbortController();
  listenersToRemove.push(listening);
  document.addEventListener(
    'click',
    (event) => {
      const target = event.target;
      if (target instanceof HTMLAnchorElement && target.href.startsWith(`${fakeStorageOrigin}/`)) {
        recorded.push({
          href: target.href,
          download: target.getAttribute('download'),
          rel: target.rel,
          element: target,
        });
        event.preventDefault();
      }
    },
    { capture: true, signal: listening.signal },
  );
  return recorded;
}

interface FormCallbacks {
  readonly requested: Export[];
  readonly inProgress: number[];
  readonly limited: number[];
}

function FormInPage({ callbacks }: { readonly callbacks: FormCallbacks }): ReactElement {
  const field = useRef<HTMLInputElement>(null);
  return (
    <main>
      <h1 tabIndex={-1}>My data</h1>
      <RequestExportForm
        headingId="request-heading"
        bodyId="request-body"
        password={field}
        onSending={() => undefined}
        onClose={() => undefined}
        onRequested={(requested) => {
          callbacks.requested.push(requested);
        }}
        onInProgress={() => {
          callbacks.inProgress.push(1);
        }}
        onLimited={() => {
          callbacks.limited.push(1);
        }}
      />
    </main>
  );
}

async function showForm(server: FakeAuthServer): Promise<FormCallbacks> {
  const session = signedOutSession(server);
  await session.signIn(email, password);
  const callbacks: FormCallbacks = { requested: [], inProgress: [], limited: [] };
  const i18n = await english();
  await act(async () => {
    render(
      providers(
        session,
        new QueryClient(),
        pageClock(server),
        controlledVisibility(),
        i18n,
        <FormInPage callbacks={callbacks} />,
      ),
    );
    await Promise.resolve();
  });
  return callbacks;
}

function passwordField(): HTMLInputElement {
  const field = screen.getByLabelText('Current password');
  if (!(field instanceof HTMLInputElement)) {
    throw new Error('the password field is not an input');
  }
  return field;
}

async function submitPassword(typed: string): Promise<void> {
  fireEvent.change(passwordField(), { target: { value: typed } });
  await act(async () => {
    fireEvent.click(button('Request export'));
    await Promise.resolve();
  });
}

afterEach(() => {
  cleanup();
  for (const listening of listenersToRemove.splice(0)) listening.abort();
});

describe('My data states', () => {
  test('no export shows the explanation and Request an export', async () => {
    const server = fakeServer();

    await shownExport(server, null);

    expect(screen.getByText(/ZIP archive with tables, a calendar file, your files and a JSON copy/)).toBeDefined();
    expect(screen.getByText('You have no export yet.')).toBeDefined();
    expect(button('Request an export').getAttribute('aria-disabled')).toBeNull();
  });

  test('a queued export shows that it waits, with Cancel export', async () => {
    const server = fakeServer();

    await shownExport(server, anExport(server, { state: 'queued' }));

    expect(screen.getByText('Your export is waiting to start.')).toBeDefined();
    expect(button('Cancel export')).toBeDefined();
    expect(screen.queryByRole('button', { name: 'Request an export' })).toBeNull();
  });

  test('a building export shows its progress with Cancel export', async () => {
    const server = fakeServer();

    await shownExport(server, anExport(server, { state: 'building', progress: { done: 3, total: 12 } }));

    expect(screen.getByText('Your export is being prepared.')).toBeDefined();
    expect(screen.getByText('3 of 12 items')).toBeDefined();
    expect(screen.getByRole('progressbar').getAttribute('value')).toBe('3');
    expect(button('Cancel export')).toBeDefined();
  });

  test('a ready export shows its size and expiry with Download and Delete export', async () => {
    const server = fakeServer();
    const ready = readyExport(server);

    await shownExport(server, ready);

    expect(screen.getByText('Your export is ready.')).toBeDefined();
    expect(screen.getByText(/^Size: 2[.,]5/)).toBeDefined();
    expect(
      screen.getByText(`Available until ${formatDateTime(ready.expires_at ?? '', 'ru', 'Europe/Moscow')}.`),
    ).toBeDefined();
    expect(button('Download')).toBeDefined();
    expect(button('Delete export')).toBeDefined();
  });

  test.each<[ExportFailure, string]>([
    ['withdrawn', 'The export was withdrawn because some data in it is no longer available to you. Request a new one.'],
    ['too_long', 'Preparing the export took too long.'],
    ['too_large', 'The export grew too large.'],
    ['error', 'Something went wrong while preparing the export.'],
  ])('fault: a failed export with the reason %s says why and offers a new request', async (failure, text) => {
    const server = fakeServer();

    await shownExport(server, anExport(server, { state: 'failed', failure }));

    expect(screen.getByText(text)).toBeDefined();
    expect(button('Request an export').getAttribute('aria-disabled')).toBeNull();
  });

  test('a request inside the 24-hour window is an unavailable button with the time', async () => {
    const server = fakeServer();
    const nextAllowedAt = instantAfter(server, 20 * hourMilliseconds);

    await shownExport(server, anExport(server, { state: 'failed', failure: 'error' }), nextAllowedAt);

    const request = button('Request an export');
    const reason = `You can request a new export after ${formatDateTime(nextAllowedAt, 'ru', 'Europe/Moscow')}.`;
    expect(request.getAttribute('aria-disabled')).toBe('true');
    expect(screen.getByText(reason).id).toBe(request.getAttribute('aria-describedby'));
  });

  test('an expired export says so', async () => {
    const server = fakeServer();

    await shownExport(server, anExport(server, { state: 'expired' }));

    expect(screen.getByText('The export has expired and is no longer available.')).toBeDefined();
    expect(button('Request an export')).toBeDefined();
  });

  test('an unconfirmed account sees the confirmation notice and reads no export', async () => {
    const server = fakeServer('unverified');

    await showPage(server);

    expect(await screen.findByText('Confirm your email to export your data.')).toBeDefined();
    expect(screen.getByRole('link', { name: 'Confirm email' }).getAttribute('href')).toBe('/verify-email');
    expect(screen.queryByRole('button', { name: 'Request an export' })).toBeNull();
    expect(server.exportReads).toBe(0);
  });
});

describe('My data actions', () => {
  test('Cancel export deletes a building export and the notice takes focus', async () => {
    const server = fakeServer();
    await shownExport(server, anExport(server, { state: 'building', progress: { done: 1, total: 4 } }));

    await act(async () => {
      fireEvent.click(button('Cancel export'));
      await Promise.resolve();
    });

    const notice = await screen.findByText('The export was cancelled.');
    expect(document.activeElement).toBe(notice);
    expect(screen.getByText('You have no export yet.')).toBeDefined();
    expect(button('Request an export').getAttribute('aria-disabled')).toBeNull();
  });

  test('Delete export removes a ready export, keeps the 24-hour limit and the notice takes focus', async () => {
    const server = fakeServer();
    const nextAllowedAt = instantAfter(server, 24 * hourMilliseconds);
    await shownExport(server, readyExport(server), nextAllowedAt);

    await act(async () => {
      fireEvent.click(button('Delete export'));
      await Promise.resolve();
    });

    const notice = await screen.findByText('The export was deleted.');
    expect(document.activeElement).toBe(notice);
    expect(button('Request an export').getAttribute('aria-disabled')).toBe('true');
  });

  test('Download starts the download from a temporary link without a download attribute and keeps no address', async () => {
    const server = fakeServer();
    const shown = await shownExport(server, readyExport(server));
    const links = recordDownloadLinks();

    await act(async () => {
      fireEvent.click(button('Download'));
      await Promise.resolve();
    });

    await eventually(() => {
      expect(links).toHaveLength(1);
    });
    const [link] = links;
    expect(link?.href.startsWith(`${fakeStorageOrigin}/exports/`)).toBe(true);
    expect(link?.download).toBeNull();
    expect(link?.rel).toBe('noopener noreferrer');
    expect(link?.element.isConnected).toBe(false);
    expect(document.documentElement.outerHTML).not.toContain('X-Amz-Signature');
    const cached = JSON.stringify(
      shown.queryClient
        .getQueryCache()
        .getAll()
        .map((query) => query.state.data),
    );
    expect(cached).not.toContain('X-Amz-Signature');
    expect(shown.queryClient.getMutationCache().getAll()).toHaveLength(0);
    expect(server.exportDownloadsMinted).toBe(1);
  });

  test('a mint refused as not found reads the export again and shows its state without a retry', async () => {
    const server = fakeServer();
    await shownExport(server, readyExport(server));
    server.seedExport(email, null);

    await act(async () => {
      fireEvent.click(button('Download'));
      await Promise.resolve();
    });

    expect(await screen.findByText('The export is no longer available. The page shows its current state.')).toBe(
      document.activeElement,
    );
    expect(screen.getByText('You have no export yet.')).toBeDefined();
    expect(mintRequests(server)).toBe(1);
    expect(server.exportReads).toBe(2);
  });

  test('fault: a mint refused as rate limited shows the time without a retry', async () => {
    const server = fakeServer();
    await shownExport(server, readyExport(server));
    server.refuseNextExportDownload(429, 'rate_limited', 600);

    await act(async () => {
      fireEvent.click(button('Download'));
      await Promise.resolve();
    });

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe('Too many attempts, try again in 10 minutes.');
    expect(document.activeElement).toBe(alert);
    expect(mintRequests(server)).toBe(1);
  });
});

describe('reading the export again after an action', () => {
  test('a read held from before Cancel export answers afterwards and the cancelled state stays', async () => {
    const server = fakeServer();
    const shown = await shownExport(
      server,
      anExport(server, { state: 'building', progress: { done: 1, total: 4 } }),
      undefined,
      new FollowingQueryClient(),
    );
    const held = server.holdNextExportRead();
    act(() => {
      shown.clock.pass(10_000);
    });
    await held.received;

    await act(async () => {
      fireEvent.click(button('Cancel export'));
      await Promise.resolve();
    });
    await screen.findByText('The export was cancelled.');
    await act(async () => {
      held.resume();
      await held.consumed;
      await new Promise<void>((settled) => {
        setTimeout(settled, 0);
      });
    });

    expect(server.exportReadsAnswered).toBe(3);
    expect(shown.queryClient.getQueryData<CurrentExport>(currentExportKey)?.export).toBeNull();
    expect(screen.getByText('You have no export yet.')).toBeDefined();
    expect(screen.queryByRole('button', { name: 'Cancel export' })).toBeNull();
    expect(document.activeElement).toBe(screen.getByText('The export was cancelled.'));
  });

  test('fault: a failed read after a successful cancel announces the cancel and reports the failed read', async () => {
    const server = fakeServer();
    await shownExport(server, anExport(server, { state: 'building' }));
    server.failNextExportRead();

    await act(async () => {
      fireEvent.click(button('Cancel export'));
      await Promise.resolve();
    });

    const notice = await screen.findByText('The export was cancelled.');
    expect(document.activeElement).toBe(notice);
    expect(
      await screen.findByText('The latest data could not be loaded. The page shows what was loaded before.'),
    ).toBeDefined();
    expect(screen.queryByRole('alert')).toBeNull();
  });
});

describe('focus and refusals when the shown export changes', () => {
  test('a read that replaces the focused Cancel export moves focus to the page heading', async () => {
    const server = fakeServer();
    const shown = await shownExport(server, anExport(server, { state: 'building', progress: { done: 1, total: 4 } }));
    button('Cancel export').focus();
    server.seedExport(email, readyExport(server));

    act(() => {
      shown.clock.pass(10_000);
    });

    await screen.findByText('Your export is ready.');
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });

  test('the request button keeps focus when the 24-hour limit passes', async () => {
    const server = fakeServer();
    const shown = await shownExport(
      server,
      anExport(server, { state: 'failed', failure: 'error' }),
      instantAfter(server, hourMilliseconds),
    );
    const request = button('Request an export');
    request.focus();

    act(() => {
      shown.clock.pass(hourMilliseconds);
    });

    expect(button('Request an export')).toBe(request);
    expect(request.getAttribute('aria-disabled')).toBeNull();
    expect(document.activeElement).toBe(request);
  });

  test('fault: a refused cancel disappears when a read shows the export ready', async () => {
    const server = fakeServer();
    const shown = await shownExport(server, anExport(server, { state: 'building' }));
    server.refuseNextExportDeletion(503, 'service_unavailable', 60);
    await act(async () => {
      fireEvent.click(button('Cancel export'));
      await Promise.resolve();
    });
    await screen.findByRole('alert');
    server.seedExport(email, readyExport(server));

    act(() => {
      shown.clock.pass(10_000);
    });

    await screen.findByText('Your export is ready.');
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByText('Exports are busy right now. Try again in a minute.')).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });

  test('fault: a focused refusal removed by a queued to building read moves focus to the page heading', async () => {
    const server = fakeServer();
    const shown = await shownExport(server, anExport(server, { state: 'queued' }));
    server.refuseNextExportDeletion(503, 'service_unavailable', 60);
    await act(async () => {
      fireEvent.click(button('Cancel export'));
      await Promise.resolve();
    });
    await eventually(() => {
      expect(document.activeElement).toBe(screen.getByRole('alert'));
    });
    server.seedExport(email, anExport(server, { state: 'building', progress: { done: 1, total: 4 } }));

    act(() => {
      shown.clock.pass(10_000);
    });

    await screen.findByText('Your export is being prepared.');
    expect(screen.queryByRole('alert')).toBeNull();
    expect(button('Cancel export')).toBeDefined();
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 }));
  });
});

describe('the export request form', () => {
  test('a wrong password is refused inline with focus kept on the password field', async () => {
    const server = fakeServer();
    const callbacks = await showForm(server);

    await submitPassword('not the password');

    await eventually(() => {
      expect(screen.getByText('This value is incorrect.')).toBeDefined();
    });
    expect(passwordField().getAttribute('aria-invalid')).toBe('true');
    expect(document.activeElement).toBe(passwordField());
    expect(screen.queryByRole('alert')).toBeNull();
    expect(callbacks.requested).toHaveLength(0);
  });

  test('the right password requests the export with the password only in the body', async () => {
    const server = fakeServer();
    const callbacks = await showForm(server);

    await submitPassword(password);

    await eventually(() => {
      expect(callbacks.requested.map((requested) => requested.state)).toEqual(['queued']);
    });
    expect(server.sentAddresses.every((address) => !address.href.includes(encodeURIComponent(password)))).toBe(true);
  });

  test('the sixth wrong password shows the rate-limited text with its time, not the refusal of the password', async () => {
    const server = fakeServer();
    const callbacks = await showForm(server);
    for (let attempt = 0; attempt < 5; attempt += 1) {
      await submitPassword('not the password');
      await screen.findByText('This value is incorrect.');
    }

    await submitPassword('not the password');

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe('Too many attempts, try again in 15 minutes.');
    expect(screen.queryByText('This value is incorrect.')).toBeNull();
    expect(callbacks.limited).toHaveLength(1);
  });

  test('a request inside the 24-hour window shows when a new export may start', async () => {
    const server = fakeServer();
    const nextAllowedAt = instantAfter(server, 20 * hourMilliseconds);
    server.seedExport(email, null, nextAllowedAt);
    const callbacks = await showForm(server);

    await submitPassword(password);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe(
      `You can request a new export after ${formatDateTime(nextAllowedAt, 'ru', 'Europe/Moscow')}.`,
    );
    expect(callbacks.limited).toHaveLength(1);
    expect(server.exportRequests).toBe(1);
  });

  test('fault: a full export capacity shows that exports are busy without a retry', async () => {
    const server = fakeServer();
    server.refuseNextExportRequest(503, 'service_unavailable', 60);
    await showForm(server);

    await submitPassword(password);

    const alert = await screen.findByRole('alert');
    expect(alert.textContent).toBe('Exports are busy right now. Try again in a minute.');
    expect(server.exportRequests).toBe(1);
  });

  test('fault: an export already in progress from another tab hands over to the page', async () => {
    const server = fakeServer();
    server.seedExport(email, anExport(server, { state: 'building' }));
    const callbacks = await showForm(server);

    await submitPassword(password);

    await eventually(() => {
      expect(callbacks.inProgress).toHaveLength(1);
    });
    expect(callbacks.requested).toHaveLength(0);
    expect(server.exportRequests).toBe(1);
  });
});

describe('following an export in progress', () => {
  test('a building export is read again after 10 seconds, not before', async () => {
    const server = fakeServer();
    const shown = await shownExport(server, anExport(server, { state: 'building' }));

    act(() => {
      shown.clock.pass(9_999);
    });
    const fetchingBeforeTenSeconds = shown.queryClient.isFetching();
    act(() => {
      shown.clock.pass(1);
    });

    expect(fetchingBeforeTenSeconds).toBe(0);
    expect(shown.queryClient.isFetching()).toBe(1);
    await eventually(() => {
      expect(server.exportReads).toBe(2);
    });
  });

  test('a hidden page reads nothing until it is shown again', async () => {
    const server = fakeServer();
    const shown = await shownExport(server, anExport(server, { state: 'queued' }));

    act(() => {
      shown.visibility.hide();
    });
    act(() => {
      shown.clock.pass(60_000);
    });
    const fetchingWhileHidden = shown.queryClient.isFetching();
    act(() => {
      shown.visibility.show();
    });
    act(() => {
      shown.clock.pass(10_000);
    });

    expect(fetchingWhileHidden).toBe(0);
    await eventually(() => {
      expect(server.exportReads).toBe(2);
    });
  });

  test('a ready export stops the reads', async () => {
    const server = fakeServer();
    const shown = await shownExport(server, anExport(server, { state: 'building' }));
    server.seedExport(email, readyExport(server));
    act(() => {
      shown.clock.pass(10_000);
    });
    await screen.findByText('Your export is ready.');

    act(() => {
      shown.clock.pass(60_000);
    });

    expect(shown.queryClient.isFetching()).toBe(0);
    expect(server.exportReads).toBe(2);
    expect(shown.clock.pendingDelays()).toEqual([]);
  });

  test('a notification while the page is shown reads the export again', async () => {
    const server = fakeServer();
    const gateway = new FakeGateway(server);
    const session = signedOutSession(server);
    await session.signIn(email, password);
    const queryClient = new FollowingQueryClient();
    const realtimeClock = manualScheduler();
    const connection = new ConnectionState(manualScheduler());
    connection.follow(session);
    const lifecycle = new DocumentLifecycle(
      { events: new EventTarget(), loaded: (): boolean => true },
      realtimeClock,
      queryClient,
    );
    server.seedExport(email, anExport(server, { state: 'building' }));
    await showPage(server, queryClient, session);
    await screen.findByText('Your export is being prepared.');
    const stop = wireRealtime({
      session,
      connection,
      queryClient,
      sockets: gateway.sockets,
      scheduler: realtimeClock,
      lifecycle,
      onFailure: (error) => {
        throw error;
      },
    });
    await eventually(() => {
      expect(gateway.readySockets(accountId)).toBe(1);
      expect(server.exportReads).toBe(2);
      expect(queryClient.isFetching()).toBe(0);
    });
    server.seedExport(email, readyExport(server));

    act(() => {
      gateway.notify(accountId, '00000000-0000-4000-b000-000000000001');
    });

    expect(await screen.findByText('Your export is ready.')).toBeDefined();
    stop();
    lifecycle.dispose();
  });
});
