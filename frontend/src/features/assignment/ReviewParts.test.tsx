import { cleanup, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { createI18n } from '../../lib/i18n/i18n.ts';
import type { AnswerFile, QuestionBlock } from './api.ts';
import { QuestionReview, RevealedPart } from './ReviewParts.tsx';
import { assignmentTexts } from './texts.en.ts';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const attachmentId = '0b5e2c8a-3f41-4d7e-9a6b-2c1d8e7f4a90';
const otherAttachmentId = '7c9d1e2f-4a5b-4c6d-8e7f-9a0b1c2d3e4f';

function correctedFile(url: string): AnswerFile {
  return {
    attachmentId,
    filename: 'Corrections.pdf',
    mediaType: 'application/pdf',
    sizeBytes: 2048,
    url,
    expiresAt: '2026-11-15T12:00:00Z',
  };
}

const numberBlock: QuestionBlock = { type: 'short_number', id: 'question-1', prompt: 'Half of 5?', images: [] };

async function renderWithTexts(element: React.ReactElement): Promise<void> {
  const i18n = await createI18n('en');
  i18n.addResourceBundle('en', 'translation', { assignment: assignmentTexts }, true, true);
  render(<I18nextProvider i18n={i18n}>{element}</I18nextProvider>);
}

describe('a corrected file of the published review', () => {
  test('downloads through its own signed address without a referrer', async () => {
    const url = `${origin}/api/v1/files/${attachmentId}?t=signed`;

    await renderWithTexts(
      <QuestionReview
        item={{ comment: '', correctedFiles: [correctedFile(url)] }}
        highlighted={false}
        origin={origin}
      />,
    );

    const link = screen.getByRole('link', { name: 'Corrections.pdf' });
    expect(link.getAttribute('href')).toBe(url);
    expect(link.getAttribute('referrerpolicy')).toBe('no-referrer');
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
    expect(link.getAttribute('download')).toBe('Corrections.pdf');
  });

  test.each([
    ['another attachment', `${origin}/api/v1/files/${otherAttachmentId}?t=signed`],
    ['another origin', `https://elsewhere.example/api/v1/files/${attachmentId}?t=signed`],
    ['an extra query key', `${origin}/api/v1/files/${attachmentId}?t=signed&next=x`],
    ['no address', ''],
  ])('shows only its name when the address names %s', async (_case, url) => {
    await renderWithTexts(
      <QuestionReview
        item={{ comment: '', correctedFiles: [correctedFile(url)] }}
        highlighted={false}
        origin={origin}
      />,
    );

    expect(screen.queryByRole('link')).toBeNull();
    expect(screen.getByText('Corrections.pdf')).not.toBeNull();
  });
});

describe('a revealed number answer', () => {
  test.each([
    ['a tolerance', '0.1', 'Correct value: 2.5 ± 0.1'],
    ['a zero tolerance', '0', 'Correct value: 2.5'],
    ['no tolerance', null, 'Correct value: 2.5'],
  ])('with %s states the value accordingly', async (_case, tolerance, expected) => {
    await renderWithTexts(
      <RevealedPart
        block={numberBlock}
        revealed={{ correctOptionIds: [], acceptedAnswers: [], correctValue: '2.5', tolerance, explanation: '' }}
      />,
    );

    expect(screen.getByText(expected)).not.toBeNull();
  });
});
