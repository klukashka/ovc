import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { type ReactElement, useState } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { createI18n } from '../../lib/i18n/i18n.ts';
import type { AutosaveStatus } from './autosave.ts';
import { SaveStatus } from './SaveStatus.tsx';
import { assignmentTexts } from './texts.en.ts';

afterEach(() => {
  cleanup();
});

function FailingSave({ next }: { readonly next: readonly AutosaveStatus[] }): ReactElement {
  const [step, setStep] = useState(0);
  const status = step === 0 ? { kind: 'failed' as const } : (next[step - 1] ?? { kind: 'failed' as const });
  return (
    <SaveStatus
      status={status}
      onRetry={() => {
        setStep((value) => value + 1);
      }}
    />
  );
}

async function renderFailingSave(next: readonly AutosaveStatus[]): Promise<void> {
  const i18n = await createI18n('en');
  i18n.addResourceBundle('en', 'translation', { assignment: assignmentTexts }, true, true);
  render(
    <I18nextProvider i18n={i18n}>
      <FailingSave next={next} />
    </I18nextProvider>,
  );
}

describe('the save status of an assignment', () => {
  test('a failed save says it is trying again and offers Try now', async () => {
    await renderFailingSave([]);

    const status = screen.getByRole('status');

    expect(status.textContent).toBe('Not saved. Trying again…');
    expect(screen.queryByRole('button', { name: 'Try now' })).not.toBeNull();
  });

  test('Try now keeps focus while the save it sent is in flight', async () => {
    await renderFailingSave([{ kind: 'saving' }]);
    const tryNow = screen.getByRole('button', { name: 'Try now' });

    tryNow.focus();
    act(() => {
      fireEvent.click(tryNow);
    });

    expect(screen.getByRole('status').textContent).toBe('Saving…');
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Try now' }));
    expect(screen.getByRole('button', { name: 'Try now' }).getAttribute('aria-busy')).toBe('true');
  });
});
