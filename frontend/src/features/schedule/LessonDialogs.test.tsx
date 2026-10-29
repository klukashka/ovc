import { act, cleanup, render, screen } from '@testing-library/react';
import { type ReactElement, useRef } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { useAction } from '../../components/ui/action.ts';
import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import type { Me } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { LessonDetail } from './api.ts';
import { EditForm } from './LessonDialogs.tsx';

afterEach(() => {
  cleanup();
});

const tutorInBerlin: Me = {
  id: '00000000-0000-4000-8000-000000000001',
  role: 'tutor',
  state: 'active',
  name: 'Мария',
  email: 'maria@example.org',
  email_verified: true,
  language: 'ru',
  timezone: 'Europe/Berlin',
  theme: 'light',
  avatar_id: null,
  reclaim_pending: false,
};

const moscowLesson: LessonDetail = {
  id: '00000000-0000-4000-8000-000000000004',
  title: 'Урок',
  description: null,
  starts_at: '2026-11-03T15:00:00Z',
  ends_at: '2026-11-03T16:00:00Z',
  duration_minutes: 60,
  status: 'scheduled',
  timezone: 'Europe/Moscow',
  series_id: null,
  series: null,
  version: 1,
  cancelled_at: null,
  other_timezone: null,
  audience: { kind: 'one_to_one', label: 'Анна', label_kind: null, colour: 'moss', members: [] },
  tutor: null,
  channel_id: null,
};

function Form(): ReactElement {
  const action = useAction();
  const firstField = useRef<HTMLInputElement>(null);
  return (
    <EditForm
      lesson={moscowLesson}
      me={tutorInBerlin}
      headingId="edit-heading"
      firstField={firstField}
      action={action}
      handlers={{ onDismiss: () => undefined, onSaved: () => undefined, onStale: () => undefined }}
    />
  );
}

describe('edit lesson dialog', () => {
  test('the hint naming the lesson timezone describes the date and start fields', async () => {
    const i18n = await createI18n('ru');

    await act(async () => {
      render(
        <ApiClientContext value={createApiClient('https://ovc.example', fetch)}>
          <I18nextProvider i18n={i18n}>
            <Form />
          </I18nextProvider>
        </ApiClientContext>,
      );
      await Promise.resolve();
    });

    const hint = screen.getByText('Дата и время указаны по времени Москвы.');
    const describedBy = (label: string): readonly string[] =>
      (screen.getByLabelText(label, { exact: true }).getAttribute('aria-describedby') ?? '').split(' ');
    expect(hint.id).not.toBe('');
    expect(describedBy('Дата')).toContain(hint.id);
    expect(describedBy('Время начала')).toContain(hint.id);
    expect(screen.getByLabelText('Время начала')).toHaveProperty('value', '18:00');
  });
});
