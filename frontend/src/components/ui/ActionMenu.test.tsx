import { act, cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { ActionMenu, type MenuItem } from './ActionMenu.tsx';

afterEach(() => {
  cleanup();
});

function openMenu(items: readonly MenuItem[]): HTMLElement {
  render(
    <ActionMenu label="More actions" unavailable={false} items={items}>
      More
    </ActionMenu>,
  );
  const button = screen.getByRole('button', { name: 'More actions' });
  act(() => {
    button.click();
  });
  const menu = document.getElementById(button.getAttribute('aria-controls') ?? '');
  if (menu === null) {
    throw new Error('the menu did not open');
  }
  return menu;
}

function itemChoices(chosen: string[]): readonly MenuItem[] {
  return [
    {
      label: 'Stop studying',
      danger: true,
      onChoose: (): void => {
        chosen.push('Stop studying');
      },
    },
    {
      label: 'Rename',
      onChoose: (): void => {
        chosen.push('Rename');
      },
    },
  ];
}

describe('an action menu with a danger item', () => {
  test('renders the danger item last, after a separator, in danger text with a warning icon', () => {
    const items = itemChoices([]);

    const menu = openMenu(items);

    const children = [...menu.children];
    const last = children.at(-1);
    expect(children.map((child) => child.tagName)).toEqual(['BUTTON', 'HR', 'BUTTON']);
    expect(children.at(-2)).toBe(screen.getByRole('separator'));
    expect(last?.textContent).toBe('Stop studying');
    expect(last?.className).toContain('text-danger');
    expect(last?.querySelector('svg[data-outline-icon]')).not.toBeNull();
    expect(children[0]?.className).not.toContain('text-danger');
    expect(children[0]?.querySelector('svg')).toBeNull();
  });

  test('chooses the danger item and returns focus to the menu button', () => {
    const chosen: string[] = [];
    const menu = openMenu(itemChoices(chosen));
    const dangerItem = screen.getByRole('button', { name: 'Stop studying' });

    act(() => {
      dangerItem.click();
    });

    expect(chosen).toEqual(['Stop studying']);
    expect(menu.isConnected).toBe(false);
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'More actions' }));
  });

  test('renders no separator without a danger item', () => {
    const items: readonly MenuItem[] = [{ label: 'Rename', onChoose: () => undefined }];

    const menu = openMenu(items);

    expect([...menu.children].map((child) => child.tagName)).toEqual(['BUTTON']);
    expect(screen.queryByRole('separator')).toBeNull();
  });

  test('renders no separator when the danger item is the only item', () => {
    const items: readonly MenuItem[] = [{ label: 'Stop studying', danger: true, onChoose: () => undefined }];

    const menu = openMenu(items);

    expect([...menu.children].map((child) => child.tagName)).toEqual(['BUTTON']);
    expect(menu.children[0]?.className).toContain('text-danger');
  });
});
