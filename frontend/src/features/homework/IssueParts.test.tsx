import { cleanup, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { createI18n } from '../../lib/i18n/i18n.ts';
import { AudienceLabel } from './IssueParts.tsx';

afterEach(() => {
  cleanup();
});

describe('issue audience label', () => {
  test("an issue whose card name a student's purge erased shows the fixed former student text", async () => {
    const i18n = await createI18n('en');

    render(
      <I18nextProvider i18n={i18n}>
        <AudienceLabel
          audience={{ relationship_id: '00000000-0000-4000-8000-000000000001', label_kind: 'former_student' }}
          groups={[]}
        />
      </I18nextProvider>,
    );

    expect(screen.getByText('Former student')).toBeDefined();
  });
});
