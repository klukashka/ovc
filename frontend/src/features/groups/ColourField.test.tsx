import { act, cleanup, render, screen } from '@testing-library/react';
import type { i18n } from 'i18next';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { createI18n } from '../../lib/i18n/i18n.ts';
import type { GroupColour } from './api.ts';
import { ColourField } from './ColourField.tsx';

interface Stored {
  readonly colour: GroupColour;
  readonly reloads: number;
}

function field(translations: i18n, stored: Stored): ReactElement {
  return (
    <I18nextProvider i18n={translations}>
      <ColourField defaultColour={stored.colour} reloads={stored.reloads} />
    </I18nextProvider>
  );
}

async function checkedAfter(picked: string | null, changes: readonly Stored[]): Promise<string[]> {
  const translations = await createI18n('en');
  const { rerender } = render(field(translations, { colour: 'clay', reloads: 0 }));
  if (picked !== null) {
    act(() => {
      screen.getByRole('radio', { name: picked }).click();
    });
  }
  for (const change of changes) {
    rerender(field(translations, change));
  }
  return screen
    .getAllByRole<HTMLInputElement>('radio')
    .filter((radio) => radio.checked)
    .map((radio) => radio.value);
}

afterEach(() => {
  cleanup();
});

describe('the group colour choice showing a stored colour', () => {
  test('follows a changed stored colour while the user has not picked another', async () => {
    const changes = [{ colour: 'teal', reloads: 0 } as const];

    const checked = await checkedAfter(null, changes);

    expect(checked).toEqual(['teal']);
  });

  test('keeps the user’s pick when the stored colour changes in the background', async () => {
    const changes = [{ colour: 'teal', reloads: 0 } as const];

    const checked = await checkedAfter('Green', changes);

    expect(checked).toEqual(['moss']);
  });

  test('shows the changed stored colour after a reload because the group changed meanwhile', async () => {
    const changes = [{ colour: 'teal', reloads: 0 } as const, { colour: 'teal', reloads: 1 } as const];

    const checked = await checkedAfter('Green', changes);

    expect(checked).toEqual(['teal']);
  });
});

async function renderedIn(language: 'ru' | 'en'): Promise<HTMLElement> {
  const translations = await createI18n(language);
  const { container } = render(field(translations, { colour: 'moss', reloads: 0 }));
  return container;
}

function visibleText(container: HTMLElement): string {
  const shown = [...container.querySelectorAll('*')].filter(
    (element) => element.closest('.sr-only') === null && element.closest('[aria-hidden="true"]') === null,
  );
  return shown
    .flatMap((element) => [...element.childNodes].filter((node) => node.nodeType === Node.TEXT_NODE))
    .map((node) => node.textContent ?? '')
    .join(' ');
}

describe('the group colour swatches', () => {
  test('show no visible colour names', async () => {
    const container = await renderedIn('ru');

    const text = visibleText(container);

    expect(text.trim()).toBe('Цвет');
    expect(screen.getByRole<HTMLInputElement>('radio', { name: 'Зелёный' }).checked).toBe(true);
  });

  test('use eight native radios in one named group', async () => {
    await renderedIn('en');

    const group = screen.getByRole('group', { name: 'Colour' });

    expect(group.querySelectorAll('input[type="radio"][name="colour"]')).toHaveLength(8);
  });

  test.each(['ru', 'en'] as const)('give eight distinct accessible names in %s', async (language) => {
    await renderedIn(language);

    const names = screen.getAllByRole('radio').map((radio) => radio.closest('label')?.textContent ?? '');

    expect(new Set(names).size).toBe(8);
    expect(names.every((name) => name.trim() !== '')).toBe(true);
  });
});
