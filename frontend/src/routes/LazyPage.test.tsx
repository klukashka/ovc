import { act, cleanup, render, screen } from '@testing-library/react';
import type { ComponentType, ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { Outlet, RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import type { RouteHandle } from '../app/handles.ts';
import { RouteFocus } from '../components/frame/RouteFocus.tsx';
import { Page } from '../components/ui/Page.tsx';
import { createI18n } from '../lib/i18n/i18n.ts';
import { lazyPage } from './LazyPage.tsx';

interface PendingModule {
  readonly load: () => Promise<ComponentType>;
  readonly resolve: (page: ComponentType) => void;
  readonly reject: (reason: Error) => void;
}

function pendingModule(): PendingModule {
  let resolve: (page: ComponentType) => void = () => undefined;
  let reject: (reason: Error) => void = () => undefined;
  const promise = new Promise<ComponentType>((settle, refuse) => {
    resolve = settle;
    reject = refuse;
  });
  return { load: () => promise, resolve, reject };
}

function Frame(): ReactElement {
  return (
    <main>
      <Outlet />
    </main>
  );
}

function LoadedTodayPage(): ReactElement {
  return (
    <Page>
      <p>Loaded today</p>
    </Page>
  );
}

function LoadedGroupsPage(): ReactElement {
  return (
    <Page>
      <p>Loaded groups</p>
    </Page>
  );
}

async function showAt(
  element: ReactElement,
  address: string,
  todayElement: ReactElement = <Page />,
): Promise<ReturnType<typeof createMemoryRouter>> {
  const router = createMemoryRouter(
    [
      {
        element: <RouteFocus />,
        children: [
          {
            element: <Frame />,
            children: [
              { path: '/today', element: todayElement, handle: { title: 'page.today' } satisfies RouteHandle },
              { path: '/groups', element, handle: { title: 'page.groups' } satisfies RouteHandle },
            ],
          },
        ],
      },
    ],
    { initialEntries: [address] },
  );
  const i18n = await createI18n('en');
  await act(async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <RouterProvider router={router} />
      </I18nextProvider>,
    );
    await Promise.resolve();
  });
  return router;
}

afterEach(() => {
  cleanup();
});

describe('lazily loaded pages', () => {
  test('a page still loading shows its heading, which keeps focus once the page has loaded', async () => {
    const groups = pendingModule();
    const router = await showAt(lazyPage(groups.load), '/today');
    await act(async () => {
      await router.navigate('/groups');
    });
    const loadingHeading = screen.getByRole('heading', { level: 1, name: 'Groups' });
    const focusedWhileLoading = document.activeElement;

    await act(async () => {
      groups.resolve(LoadedGroupsPage);
      await Promise.resolve();
    });

    expect(focusedWhileLoading).toBe(loadingHeading);
    expect(screen.getByText('Loaded groups')).toBeDefined();
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Groups' }));
    expect(loadingHeading.isConnected).toBe(false);
  });

  test('a page loaded on the first visit leaves focus where it was', async () => {
    const groups = pendingModule();
    await showAt(lazyPage(groups.load), '/groups');

    await act(async () => {
      groups.resolve(LoadedGroupsPage);
      await Promise.resolve();
    });

    expect(screen.getByText('Loaded groups')).toBeDefined();
    expect(document.activeElement).toBe(document.body);
  });

  test('a page whose code fails to load says so inside its page, whose heading keeps focus', async () => {
    const groups = pendingModule();
    const router = await showAt(lazyPage(groups.load), '/today');
    await act(async () => {
      await router.navigate('/groups');
    });

    await act(async () => {
      groups.reject(new Error('chunk unavailable'));
      await Promise.resolve();
    });

    const main = screen.getByRole('main');
    expect(main.contains(screen.getByRole('alert'))).toBe(true);
    expect(screen.getByRole('alert').textContent).toBe(
      'This page could not be loaded. Check your connection and reload the page.',
    );
    expect(screen.getByRole('button', { name: 'Reload the page' })).toBeDefined();
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Groups' }));
  });

  test('moving between two lazily loaded pages of one frame shows the page of the new route', async () => {
    const today = lazyPage(() => Promise.resolve(LoadedTodayPage));
    const groups = lazyPage(() => Promise.resolve(LoadedGroupsPage));
    const router = await showAt(groups, '/today', today);
    await screen.findByText('Loaded today');

    await act(async () => {
      await router.navigate('/groups');
    });

    expect(await screen.findByText('Loaded groups')).toBeDefined();
    expect(screen.queryByText('Loaded today')).toBeNull();
  });

  test('a page left while its code loads shows at once without a loading state on the next visit', async () => {
    const groups = pendingModule();
    const router = await showAt(lazyPage(groups.load), '/today');
    await act(async () => {
      await router.navigate('/groups');
      await router.navigate('/today');
      groups.resolve(LoadedGroupsPage);
      await Promise.resolve();
    });

    await act(async () => {
      await router.navigate('/groups');
    });

    expect(screen.getByText('Loaded groups')).toBeDefined();
    expect(screen.queryByRole('status')).toBeNull();
  });
});
