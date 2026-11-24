import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { type ReactElement, useState } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import type { AnswerPolicy, IssueTemplate } from './api.ts';
import { PolicyField } from './IssueAssignmentPage.tsx';
import { homeworkTexts as englishTexts } from './texts.en.ts';
import { homeworkTexts as russianTexts } from './texts.ru.ts';

afterEach(() => {
  cleanup();
});

const origin = 'https://ovc.example';
const password = 'a long password';
const tutorEmail = 'maria@example.org';
const englishHint =
  'This assignment has no questions you check by hand, so students see the answers right after submitting.';
const russianHint =
  'В этом задании нет вопросов, которые вы проверяете вручную, поэтому ученики увидят ответы сразу после отправки.';

type QuestionType = IssueTemplate['blocks'][number]['type'];

function templateWith(id: string, types: readonly QuestionType[]): IssueTemplate {
  return {
    id,
    title: 'Present Perfect',
    folder_id: null,
    created_at: '2026-11-01T10:00:00Z',
    updated_at: '2026-11-01T10:00:00Z',
    version: 1,
    ready: true,
    problems: [],
    max_points: types.length,
    question_count: types.length,
    blocks: types.map((type, index) => ({
      id: `00000000-0000-4000-8000-0000000001${String(index).padStart(2, '0')}`,
      type,
      prompt: 'Question',
      points: 1,
    })),
  };
}

function PolicyChoice({ templateId }: { readonly templateId: string }): ReactElement {
  const [policy, setPolicy] = useState<AnswerPolicy>('after_check');
  return <PolicyField template={{ id: templateId, title: 'Present Perfect' }} policy={policy} onChange={setPolicy} />;
}

interface ShownField {
  readonly server: FakeAuthServer;
}

async function showPolicy(
  language: 'en' | 'ru',
  template: IssueTemplate,
  arrange: (server: FakeAuthServer) => void = () => undefined,
): Promise<ShownField> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'Мария', email: tutorEmail, password, role: 'tutor' });
  server.addTemplate(tutorEmail, template);
  arrange(server);
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(tutorEmail, password);
  const client = createApiClient(origin, session.authorizedFetch);
  const i18n = await createI18n(language);
  i18n.addResourceBundle(
    language,
    'translation',
    { homework: language === 'en' ? englishTexts : russianTexts },
    true,
    true,
  );
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <ApiClientContext value={client}>
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <PolicyChoice templateId={template.id} />
        </I18nextProvider>
      </QueryClientProvider>
    </ApiClientContext>,
  );
  await waitFor(() => {
    expect(queryClient.isFetching()).toBe(0);
  });
  return { server };
}

function descriptionOf(radio: HTMLElement): string {
  return (radio.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter((id) => id !== '')
    .map((id) => document.getElementById(id)?.textContent ?? '')
    .join(' ');
}

function shownText(text: string): HTMLElement | null {
  const element = screen.queryByText(text);
  return element !== null && element.closest('[aria-hidden="true"]') === null ? element : null;
}

const automaticTemplateId = '00000000-0000-4000-8000-000000000051';

describe('answer visibility in the Issue dialog', () => {
  test('describes "After checking" with the hint for a template with no manual question in English', async () => {
    const template = templateWith(automaticTemplateId, ['single_choice', 'short_text', 'short_number']);

    await showPolicy('en', template);

    expect(shownText(englishHint)).not.toBeNull();
    expect(descriptionOf(screen.getByRole('radio', { name: 'After checking' }))).toBe(englishHint);
  });

  test('describes "После проверки" with the hint for a template with no manual question in Russian', async () => {
    const template = templateWith(automaticTemplateId, ['multiple_choice']);

    await showPolicy('ru', template);

    expect(shownText(russianHint)).not.toBeNull();
    expect(descriptionOf(screen.getByRole('radio', { checked: true }))).toBe(russianHint);
  });

  test.each<QuestionType>(['long_text', 'file'])('shows no hint for a template with a %s question', async (type) => {
    const template = templateWith('00000000-0000-4000-8000-000000000052', ['single_choice', type]);

    await showPolicy('en', template);

    expect(shownText(englishHint)).toBeNull();
    expect(screen.getByRole('radio', { name: 'After checking' }).getAttribute('aria-describedby')).toBeNull();
  });

  test('shows no hint when the template read fails', async () => {
    const template = templateWith(automaticTemplateId, ['single_choice']);

    await showPolicy('en', template, (server) => {
      server.failNextTemplateRead();
    });

    expect(shownText(englishHint)).toBeNull();
  });

  test('choosing "Never" hides the hint and keeps focus on that radio', async () => {
    await showPolicy('en', templateWith(automaticTemplateId, ['short_text']));
    const never = screen.getByRole('radio', { name: 'Never' });
    never.focus();

    fireEvent.click(never);

    expect(shownText(englishHint)).toBeNull();
    expect(document.activeElement).toBe(never);
  });

  test('choosing "After checking" again shows the hint and keeps focus on that radio', async () => {
    await showPolicy('en', templateWith(automaticTemplateId, ['short_text']));
    fireEvent.click(screen.getByRole('radio', { name: 'After submission' }));
    const afterCheck = screen.getByRole('radio', { name: 'After checking' });
    afterCheck.focus();

    fireEvent.click(afterCheck);

    expect(descriptionOf(afterCheck)).toBe(englishHint);
    expect(document.activeElement).toBe(afterCheck);
  });
});
