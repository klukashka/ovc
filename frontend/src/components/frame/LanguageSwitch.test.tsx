import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import type { i18n } from 'i18next';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { PreferencesContext } from '../../app/context.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { type PreferenceStore, memoryPreferenceStore, preferenceKeys } from '../../lib/preferences/store.ts';
import { LanguageSwitch } from './LanguageSwitch.tsx';

async function renderSwitch(preferences: PreferenceStore): Promise<i18n> {
  const instance = await createI18n('ru');
  render(
    <PreferencesContext value={preferences}>
      <I18nextProvider i18n={instance}>
        <LanguageSwitch />
      </I18nextProvider>
    </PreferencesContext>,
  );
  return instance;
}

async function choose(name: string): Promise<void> {
  act(() => {
    screen.getByRole('button', { name }).click();
  });
  await waitFor(() => {
    expect(screen.getByRole('button', { name }).getAttribute('aria-pressed')).toBe('true');
  });
}

afterEach(() => {
  cleanup();
});

describe('LanguageSwitch', () => {
  test('choosing a language switches the interface, presses its button and remembers the choice', async () => {
    const preferences = memoryPreferenceStore();
    const instance = await renderSwitch(preferences);

    await choose('English');

    expect(instance.language).toBe('en');
    expect(preferences.read(preferenceKeys.language)).toBe('en');
    expect(screen.getByRole('group', { name: 'Language' })).toBeDefined();
    expect(screen.getByRole('button', { name: 'English' }).getAttribute('aria-pressed')).toBe('true');
    expect(screen.getByRole('button', { name: 'Русский' }).getAttribute('aria-pressed')).toBe('false');
  });
});
