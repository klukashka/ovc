import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { MediaQueriesContext, PreferencesContext, fixedMediaQueries, wideLayoutQuery } from '../../app/context.ts';
import { type Me, Session, SessionContext } from '../../lib/api/session.ts';
import { httpAuthTransport } from '../../lib/api/transport.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import { memoryPreferenceStore } from '../../lib/preferences/store.ts';
import { sharedPages, studentPages } from '../../routes/router.tsx';
import { RouteFocus } from './RouteFocus.tsx';
import { SignedInFrame } from './SignedInFrame.tsx';

function account(role: Me['role'], state: Me['state'] = 'active'): Me {
  return {
    id: '00000000-0000-4000-8000-000000000002',
    name: 'A student of languages',
    email: 'student@example.org',
    email_verified: state === 'active',
    timezone: 'Europe/Moscow',
    avatar_id: null,
    role,
    state,
    language: 'ru',
    theme: 'system',
  };
}

function unusedSession(): Session {
  return new Session({
    origin: 'https://ovc.example',
    transport: httpAuthTransport('https://ovc.example', globalThis.fetch),
    locks: { request: (_name, callback) => callback() },
    scheduler: { schedule: () => (): void => undefined },
  });
}

async function renderFrame(
  me: Me,
  address: string,
  wide: boolean,
  language: Language = 'ru',
  title?: 'page.notFound',
): Promise<void> {
  const i18n = await createI18n(language);
  const router = createMemoryRouter(
    [
      {
        element: <RouteFocus />,
        children: [
          {
            element: <SignedInFrame me={me} {...(title === undefined ? {} : { title })} />,
            children: [...studentPages, ...sharedPages],
          },
        ],
      },
    ],
    { initialEntries: [address] },
  );
  render(
    <MediaQueriesContext value={fixedMediaQueries(wide ? [wideLayoutQuery] : [])}>
      <PreferencesContext value={memoryPreferenceStore()}>
        <SessionContext value={unusedSession()}>
          <QueryClientProvider client={new QueryClient()}>
            <I18nextProvider i18n={i18n}>
              <RouterProvider router={router} />
            </I18nextProvider>
          </QueryClientProvider>
        </SessionContext>
      </PreferencesContext>
    </MediaQueriesContext>,
  );
}

async function click(element: HTMLElement): Promise<void> {
  await act(async () => {
    element.click();
    await Promise.resolve();
  });
}

afterEach(() => {
  cleanup();
});

describe('the signed-in frame of a student', () => {
  test('shows the student sections in English and no tutor sections', async () => {
    await renderFrame(account('student'), '/tutors', true, 'en');

    const navigation = screen.getByRole('navigation', { name: 'Main navigation' });

    const labels = within(navigation)
      .getAllByRole('link')
      .map((link) => link.textContent);
    expect(labels).toEqual(['Home', 'Schedule', 'Homework', 'Messages', 'My tutors', 'Notifications', 'Settings']);
    expect(screen.getByRole('button', { name: 'Account menu: A student of languages' })).toBeDefined();
  });

  test('a shared page opened from the sidebar keeps the student frame and focuses its heading', async () => {
    await renderFrame(account('student'), '/tutors', true);

    await click(within(screen.getByRole('navigation')).getByRole('link', { name: 'Расписание' }));

    const heading = screen.getByRole('heading', { level: 1, name: 'Расписание' });
    expect(document.activeElement).toBe(heading);
    expect(within(screen.getByRole('navigation')).getByRole('link', { name: 'Мои репетиторы' })).toBeDefined();
    expect(within(screen.getByRole('navigation')).queryByRole('link', { name: 'Ученики' })).toBeNull();
  });

  test('below 1024 px the bell keeps the student frame', async () => {
    await renderFrame(account('student'), '/tutors', false);

    await click(within(screen.getByRole('banner')).getByRole('link', { name: 'Уведомления' }));

    expect(screen.getByRole('heading', { level: 1, name: 'Уведомления' })).toBeDefined();
    const overlay = screen.getByRole('dialog', { hidden: true });
    expect(within(overlay).getByRole('link', { name: 'Мои репетиторы', hidden: true })).toBeDefined();
    expect(within(overlay).queryByRole('link', { name: 'Ученики', hidden: true })).toBeNull();
  });

  test('below 1024 px a title override replaces the route title in the top bar', async () => {
    await renderFrame(account('student'), '/tutors', false, 'ru', 'page.notFound');

    const banner = screen.getByRole('banner');

    expect(within(banner).getByText('Страница не найдена')).toBeDefined();
    expect(within(banner).queryByText('Мои репетиторы')).toBeNull();
  });
});

describe('the confirmation banner', () => {
  test('an unverified account sees the confirmation banner on every page', async () => {
    await renderFrame(account('tutor', 'unverified'), '/schedule', true);

    const banner = screen.getByRole('region', { name: 'Подтверждение email' });

    expect(banner.textContent).toContain('student@example.org');
    expect(within(banner).getByRole('link', { name: 'Подтвердить email' }).getAttribute('href')).toBe('/verify-email');
  });

  test('an active account sees no confirmation banner', async () => {
    await renderFrame(account('student'), '/schedule', true);

    const banner = screen.queryByRole('region', { name: 'Подтверждение email' });

    expect(banner).toBeNull();
  });
});
