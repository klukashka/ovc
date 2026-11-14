import { act, cleanup, render } from '@testing-library/react';
import { type ReactElement, useEffect } from 'react';
import { flushSync } from 'react-dom';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { withSynchronousNavigation } from './router.tsx';

afterEach(() => {
  cleanup();
});

class LeftPage {
  mounted = false;

  mount(): () => void {
    this.mounted = true;
    return (): void => {
      this.mounted = false;
    };
  }
}

function LeavingPage({ page }: { readonly page: LeftPage }): ReactElement {
  useEffect(() => page.mount(), [page]);
  return <p>Today</p>;
}

function routerLeaving(page: LeftPage): ReturnType<typeof createMemoryRouter> {
  return createMemoryRouter(
    [
      { path: '/today', element: <LeavingPage page={page} /> },
      { path: '/review', element: <p>Review</p> },
    ],
    { initialEntries: ['/today'] },
  );
}

async function mountedWhenTheAddressChanged(
  router: ReturnType<typeof createMemoryRouter>,
  page: LeftPage,
  to: string,
): Promise<boolean | undefined> {
  render(<RouterProvider router={router} flushSync={flushSync} />);
  let mountedAtChange: boolean | undefined;
  router.subscribe((state) => {
    mountedAtChange ??= state.location.pathname === '/today' ? undefined : page.mounted;
  });
  await act(async () => {
    await router.navigate(to);
  });
  return mountedAtChange;
}

describe('client-side navigation', () => {
  test('unmounts the page it leaves in the same task as the address change', async () => {
    const page = new LeftPage();
    const router = withSynchronousNavigation(routerLeaving(page));

    const mountedAtChange = await mountedWhenTheAddressChanged(router, page, '/review');

    expect(mountedAtChange).toBe(false);
  });

  test('guards the reason for synchronous navigation: the router alone keeps the left page mounted at the address change', async () => {
    const page = new LeftPage();
    const router = routerLeaving(page);

    const mountedAtChange = await mountedWhenTheAddressChanged(router, page, '/review');

    expect(mountedAtChange).toBe(true);
  });
});
