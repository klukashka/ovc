import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import type { ReviewFile } from './api.ts';
import { FileLink } from './ReviewAnswer.tsx';

afterEach(() => {
  cleanup();
});

const attachmentId = '0b5f2c1e-3c3a-4b8e-9d55-7f1a2b3c4d5e';
const otherAttachmentId = '9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d';

function fileWithAddress(url: string): ReviewFile {
  return {
    attachmentId,
    filename: 'essay.pdf',
    mediaType: 'application/pdf',
    sizeBytes: 1200,
    url,
    expiresAt: '2026-11-14T10:00:00Z',
    width: null,
    height: null,
  };
}

const refusedAddresses: readonly [string, string][] = [
  ['another origin', `https://elsewhere.example/api/v1/files/${attachmentId}?t=token`],
  ['another attachment', `/api/v1/files/${otherAttachmentId}?t=token`],
  ['an extra query key', `/api/v1/files/${attachmentId}?t=token&download=1`],
  ['no address yet', ''],
];

describe('a file link on the review screen', () => {
  test('links to its own same-origin signed address without a referrer', () => {
    const address = `${window.location.origin}/api/v1/files/${attachmentId}?t=token`;

    render(<FileLink file={fileWithAddress(address)} />);

    const link = screen.getByRole('link', { name: 'essay.pdf' });
    expect(link.getAttribute('href')).toBe(address);
    expect(link.getAttribute('referrerpolicy')).toBe('no-referrer');
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
  });

  test.each(refusedAddresses)('shows only the file name for %s', (_name, url) => {
    render(<FileLink file={fileWithAddress(url)} />);

    expect(screen.queryByRole('link')).toBeNull();
    expect(screen.getByText('essay.pdf')).not.toBeNull();
  });
});
