import { cleanup, render } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { createI18n } from '../../lib/i18n/i18n.ts';
import { legalDocumentNames, legalLanguages } from '../../legal/document.ts';
import { LegalText } from './LegalPage.tsx';

const allowedElements: ReadonlySet<string> = new Set(['DIV', 'P', 'STRONG', 'EM', 'CODE', 'UL', 'OL', 'LI', 'A', 'BR']);

afterEach(() => {
  cleanup();
});

describe('the legal texts', () => {
  test('render only the restricted renderer’s elements and https links, in both languages', async () => {
    const shown: { readonly tags: string[]; readonly hrefs: string[]; readonly paragraphs: number }[] = [];

    for (const language of legalLanguages) {
      const i18n = await createI18n(language);
      for (const name of legalDocumentNames) {
        const { container } = render(
          <I18nextProvider i18n={i18n}>
            <LegalText name={name} />
          </I18nextProvider>,
        );
        const elements = [...container.querySelectorAll('*')];
        shown.push({
          tags: elements.map((element) => element.tagName),
          hrefs: elements.flatMap((element) => (element instanceof HTMLAnchorElement ? [element.href] : [])),
          paragraphs: container.querySelectorAll('p').length,
        });
        cleanup();
      }
    }

    expect(shown).toHaveLength(6);
    expect(shown.flatMap((text) => text.tags).filter((tag) => !allowedElements.has(tag))).toEqual([]);
    expect(shown.flatMap((text) => text.hrefs).filter((href) => !href.startsWith('https:'))).toEqual([]);
    expect(shown.every((text) => text.paragraphs > 3)).toBe(true);
  });
});
