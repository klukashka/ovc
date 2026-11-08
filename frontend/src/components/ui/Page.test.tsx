import { act, cleanup, render, screen } from '@testing-library/react';
import { type ReactElement, useSyncExternalStore } from 'react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { createI18n } from '../../lib/i18n/i18n.ts';
import { Page } from './Page.tsx';

afterEach(() => {
  cleanup();
});

interface ActionSwitch {
  readonly subscribe: (listener: () => void) => () => void;
  readonly shown: () => boolean;
  readonly show: () => void;
}

function actionSwitch(): ActionSwitch {
  let shown = false;
  const listeners = new Set<() => void>();
  return {
    subscribe: (listener): (() => void) => {
      listeners.add(listener);
      return (): void => {
        listeners.delete(listener);
      };
    },
    shown: () => shown,
    show: (): void => {
      shown = true;
      listeners.forEach((listener) => {
        listener();
      });
    },
  };
}

function PageWhoseActionArrives({ action }: { readonly action: ActionSwitch }): ReactElement {
  const shown = useSyncExternalStore(action.subscribe, action.shown);
  return (
    <Page headingText="Уведомления" action={shown ? <button type="button">Отметить все прочитанными</button> : null}>
      <p>Строки</p>
    </Page>
  );
}

async function renderPage(action: ActionSwitch): Promise<HTMLElement> {
  const i18n = await createI18n('ru');
  const router = createMemoryRouter([{ path: '*', element: <PageWhoseActionArrives action={action} /> }]);
  render(
    <I18nextProvider i18n={i18n}>
      <RouterProvider router={router} />
    </I18nextProvider>,
  );
  return screen.findByRole('heading', { level: 1, name: 'Уведомления' });
}

describe('page header', () => {
  test('an action slot that fills after the data keeps the focused heading in place', async () => {
    const action = actionSwitch();
    const heading = await renderPage(action);
    heading.focus();

    act(() => {
      action.show();
    });

    expect(screen.getByRole('button', { name: 'Отметить все прочитанными' })).toBeDefined();
    expect(document.activeElement).toBe(heading);
  });
});
