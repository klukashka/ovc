import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { MediaQueriesContext, PreferencesContext, fixedMediaQueries, wideLayoutQuery } from '../../app/context.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { type Me, Session, SessionContext } from '../../lib/api/session.ts';
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
    reclaim_pending: false,
    role,
    state,
    language: 'ru',
    theme: 'system',
  };
}

function unusedSession(): Session {
  return new Session({
    origin: 'https://ovc.example',
    transport: new FakeAuthServer().transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
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
  const session = unusedSession();
  render(
    <MediaQueriesContext value={fixedMediaQueries(wide ? [wideLayoutQuery] : [])}>
      <PreferencesContext value={memoryPreferenceStore()}>
        <SessionContext value={session}>
          <ApiClientContext value={createApiClient('https://ovc.example', session.authorizedFetch)}>
            <QueryClientProvider client={new QueryClient()}>
              <I18nextProvider i18n={i18n}>
                <RouterProvider router={router} />
              </I18nextProvider>
            </QueryClientProvider>
          </ApiClientContext>
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
    await renderFrame(account('student'), '/homework', true, 'en');

    const navigation = screen.getByRole('navigation', { name: 'Main navigation' });

    const labels = within(navigation)
      .getAllByRole('link')
      .map((link) => link.textContent);
    expect(labels).toEqual(['Home', 'Schedule', 'Homework', 'Messages', 'My tutors', 'Settings']);
    expect(within(navigation).getByRole('button', { name: 'Notifications' })).toBeDefined();
    expect(screen.getByRole('button', { name: 'A student of languages, Account menu' })).toBeDefined();
  });

  test('a shared page opened from the sidebar keeps the student frame and focuses its heading', async () => {
    await renderFrame(account('student'), '/homework', true);

    await click(within(screen.getByRole('navigation')).getByRole('link', { name: 'Расписание' }));

    const heading = screen.getByRole('heading', { level: 1, name: 'Расписание' });
    expect(document.activeElement).toBe(heading);
    expect(within(screen.getByRole('navigation')).getByRole('link', { name: 'Мои репетиторы' })).toBeDefined();
    expect(within(screen.getByRole('navigation')).queryByRole('link', { name: 'Ученики' })).toBeNull();
  });

  test('below 1024 px the notifications page keeps the student frame and the top bar keeps the bell', async () => {
    await renderFrame(account('student'), '/notifications', false);

    const heading = await screen.findByRole('heading', { level: 1, name: 'Уведомления' });

    expect(heading).toBeDefined();
    expect(within(screen.getByRole('banner')).getByRole('button', { name: 'Уведомления' })).toBeDefined();
    const overlay = screen.getByRole('dialog', { hidden: true });
    expect(within(overlay).getByRole('link', { name: 'Уведомления', hidden: true }).getAttribute('href')).toBe(
      '/notifications',
    );
    expect(within(overlay).getByRole('link', { name: 'Мои репетиторы', hidden: true })).toBeDefined();
    expect(within(overlay).queryByRole('link', { name: 'Ученики', hidden: true })).toBeNull();
  });

  test('below 1024 px a title override replaces the route title in the top bar', async () => {
    await renderFrame(account('student'), '/homework', false, 'ru', 'page.notFound');

    const banner = screen.getByRole('banner');

    expect(within(banner).getByText('Страница не найдена')).toBeDefined();
    expect(within(banner).queryByText('Задания')).toBeNull();
  });
});

describe('the signed-in frame of an unconfirmed student', () => {
  test('lists only Home, My tutors and Settings in the sidebar', async () => {
    await renderFrame(account('student', 'unverified'), '/homework', true);

    const navigation = screen.getByRole('navigation', { name: 'Основная навигация' });

    const labels = within(navigation)
      .getAllByRole('link')
      .map((link) => link.textContent);
    expect(labels).toEqual(['Главная', 'Мои репетиторы', 'Настройки']);
  });

  test('below 1024 px the top bar has no notifications bell', async () => {
    await renderFrame(account('student', 'unverified'), '/homework', false);

    const banner = screen.getByRole('banner');

    expect(within(banner).queryByRole('button', { name: /Уведомления/ })).toBeNull();
    expect(within(banner).queryByRole('link', { name: /Уведомления/ })).toBeNull();
  });

  test('the confirmation banner asks to confirm to keep studying and offers to send the link again', async () => {
    await renderFrame(account('student', 'unverified'), '/homework', true);

    const banner = screen.getByRole('region', { name: 'Подтверждение email' });

    expect(banner.textContent).toContain('Подтвердите email, чтобы продолжить занятия с репетитором.');
    expect(banner.textContent).toContain('student@example.org');
    expect(within(banner).getByRole('button', { name: 'Отправить ссылку ещё раз' })).toBeDefined();
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

async function openedPanelHeading(bell: HTMLElement): Promise<HTMLElement> {
  await click(bell);
  const heading = await screen.findByRole('heading', { level: 2, name: 'Уведомления' });
  await waitFor(() => {
    expect(document.activeElement).toBe(heading);
  });
  return heading;
}

async function nextFrame(): Promise<void> {
  await new Promise((resolve) => requestAnimationFrame(resolve));
}

describe('the notifications bell', () => {
  test('a click outside the panel closes it and returns focus to the bell', async () => {
    await renderFrame(account('tutor'), '/schedule', true);
    const bell = within(screen.getByRole('navigation')).getByRole('button', { name: 'Уведомления' });
    await openedPanelHeading(bell);

    await act(async () => {
      document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }));
      await nextFrame();
    });

    expect(screen.queryByRole('region', { name: 'Уведомления' })).toBeNull();
    expect(bell.getAttribute('aria-expanded')).toBe('false');
    expect(document.activeElement).toBe(bell);
  });

  test('below 1024 px the top bar bell opens the panel on its heading and Escape returns focus to the bell', async () => {
    await renderFrame(account('student'), '/homework', false);
    const bell = within(screen.getByRole('banner')).getByRole('button', { name: 'Уведомления' });
    const heading = await openedPanelHeading(bell);

    await act(async () => {
      heading.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
      await Promise.resolve();
    });

    expect(screen.queryByRole('region', { name: 'Уведомления' })).toBeNull();
    expect(document.activeElement).toBe(bell);
  });
});
