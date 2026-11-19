import { cleanup, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { createI18n } from '../../lib/i18n/i18n.ts';
import { IssuedNotice } from './IssueAssignment.tsx';
import { homeworkTexts } from './texts.en.ts';

afterEach(() => {
  cleanup();
});

async function renderIssuedNotice(issueId: string | null): Promise<void> {
  const i18n = await createI18n('en');
  i18n.addResourceBundle('en', 'translation', { homework: homeworkTexts }, true, true);
  render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter>
        <main>
          <h1 tabIndex={-1}>Library</h1>
          <IssuedNotice issueId={issueId} />
        </main>
      </MemoryRouter>
    </I18nextProvider>,
  );
}

describe('the issued notice', () => {
  test('takes focus in the commit that shows it, before any animation frame', async () => {
    await renderIssuedNotice('00000000-0000-4000-8000-000000000031');

    const focused = document.activeElement;

    expect(focused).toBe(screen.getByRole('status'));
    expect(focused?.textContent).toBe('Assignment issued.');
  });
});
