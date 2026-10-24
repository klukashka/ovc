import { act, cleanup, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { apiErrorFrom } from '../../lib/api/client.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { ActionState } from './action.ts';
import { Refusal } from './Refusal.tsx';

function unconfirmedRefusal(): ActionState {
  return {
    kind: 'refused',
    error: apiErrorFrom(403, { error: { code: 'email_not_verified', message: '', request_id: 'request-1' } }),
    retryAfter: null,
  };
}

async function renderRefusalAfterAction(): Promise<void> {
  const i18n = await createI18n('ru');
  const router = createMemoryRouter(
    [
      {
        path: '/',
        element: (
          <main>
            <button type="button">Пригласить</button>
            <Refusal state={unconfirmedRefusal()} confirmation="invite" />
          </main>
        ),
      },
    ],
    { initialEntries: ['/'] },
  );
  await act(async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <RouterProvider router={router} />
      </I18nextProvider>,
    );
    await Promise.resolve();
  });
}

afterEach(() => {
  cleanup();
});

describe('a refusal because the email is not confirmed', () => {
  test('shows the confirmation prompt inside an alert that takes focus', async () => {
    await renderRefusalAfterAction();

    const alert = screen.getByRole('alert');

    expect(document.activeElement).toBe(alert);
    expect(alert.textContent).toContain('Подтвердите email, чтобы приглашать учеников.');
    expect(screen.getByRole('link', { name: 'Подтвердить email' }).getAttribute('href')).toBe('/verify-email');
  });
});
