import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session, SessionContext } from '../../lib/api/session.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { type StudentCard, studentCardKey } from './api.ts';
import { StudentCardPage, StudentCardView } from './StudentCardPage.tsx';

const origin = 'https://ovc.example';
const link = 'https://ovc.example/invite/an-open-invitation-token';

const invitedCard: StudentCard = {
  id: '00000000-0000-4000-8000-000000000010',
  state: 'invited',
  display_name: 'Анна',
  email: 'anna@example.org',
  account: null,
  groups: [],
  notes: '',
  invitation: { url: link, expires_at: '2026-11-07T15:00:00Z', status: 'open' },
  linked_at: null,
  ended_at: null,
  ended_by: null,
  created_at: '2026-10-24T15:00:00Z',
};

const activeCard: StudentCard = {
  ...invitedCard,
  state: 'active',
  invitation: null,
  account: { name: 'Анна Петрова', email: 'anna@example.org', avatar_id: null, email_confirmed: true },
  groups: [{ id: '00000000-0000-4000-8000-000000000020', name: 'B1 вечер', colour: 'moss' }],
  linked_at: '2026-10-25T15:00:00Z',
};

async function signedInTutor(server: FakeAuthServer = new FakeAuthServer()): Promise<Session> {
  server.addAccount({ id: 'tutor-1', name: 'Мария', email: 'maria@example.org', password: 'a long password' });
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn('maria@example.org', 'a long password');
  return session;
}

async function show(card: StudentCard): Promise<void> {
  const session = await signedInTutor();
  const i18n = await createI18n('ru');
  const handlers = {
    onChanged: (): void => undefined,
    onStale: (): void => undefined,
    onSent: (): void => undefined,
    onWithdrawn: (): void => undefined,
    onRemoved: (): void => undefined,
    onEnded: (): void => undefined,
    onChangedElsewhere: (): void => undefined,
    onDialog: (): void => undefined,
    onLeaving: (): void => undefined,
  };
  const router = createMemoryRouter([{ path: '*', element: <StudentCardView card={card} handlers={handlers} /> }], {
    initialEntries: [`/students/${card.id}`],
  });
  await act(async () => {
    render(
      <SessionContext value={session}>
        <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
          <QueryClientProvider client={new QueryClient()}>
            <I18nextProvider i18n={i18n}>
              <RouterProvider router={router} />
            </I18nextProvider>
          </QueryClientProvider>
        </ApiClientContext>
      </SessionContext>,
    );
    await Promise.resolve();
  });
}

function buttonNames(): string[] {
  return screen.getAllByRole('button').map((button) => button.textContent);
}

const refetchFailedText = 'Не удалось обновить данные. На странице показано то, что загрузилось раньше.';

interface LoadedCard {
  readonly server: FakeAuthServer;
  readonly queryClient: QueryClient;
  readonly card: StudentCard;
  readonly notes: HTMLElement;
}

async function showLoadedCardWithTyping(): Promise<LoadedCard> {
  const server = new FakeAuthServer();
  const session = await signedInTutor(server);
  const created = await session.authorizedFetch(`${origin}/api/v1/students`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ display_name: 'Борис' }),
  });
  const card = (await created.json()) as StudentCard;
  const queryClient = new QueryClient();
  const router = createMemoryRouter(
    [
      {
        path: '/students/:studentId',
        element: (
          <main>
            <StudentCardPage />
          </main>
        ),
      },
    ],
    { initialEntries: [`/students/${card.id}`] },
  );
  render(
    <SessionContext value={session}>
      <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={await createI18n('ru')}>
            <RouterProvider router={router} />
          </I18nextProvider>
        </QueryClientProvider>
      </ApiClientContext>
    </SessionContext>,
  );
  await screen.findByRole('heading', { level: 1, name: 'Борис' });
  const notes = screen.getByRole('textbox', { name: 'Заметки' });
  notes.focus();
  fireEvent.change(notes, { target: { value: 'Unsaved typing' } });
  return { server, queryClient, card, notes };
}

async function failOneRefetch({ server, queryClient, card }: LoadedCard): Promise<void> {
  server.failNextCardRead();
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: studentCardKey(card.id) });
  });
  await screen.findByText(refetchFailedText);
}

afterEach(() => {
  cleanup();
});

describe('the student card', () => {
  test('an invited card with an email offers its link, sending, a new link, editing the email and withdrawal', async () => {
    await show(invitedCard);

    const names = buttonNames();

    expect(screen.getByText('Приглашение отправлено')).toBeDefined();
    expect(screen.getByText(link)).toBeDefined();
    expect(screen.getByText('Действует до 7 нояб., 18:00')).toBeDefined();
    expect(screen.getByLabelText('Email (необязательно)')).toBeDefined();
    expect(names).toEqual([
      'Скопировать ссылку',
      'Отправить по email',
      'Создать новую ссылку',
      'Сохранить',
      'Сохранить',
      'Отозвать приглашение',
    ]);
  });

  test('an invited card without an email cannot be sent by email', async () => {
    await show({ ...invitedCard, email: null });

    const names = buttonNames();

    expect(names).not.toContain('Отправить по email');
    expect(names).toContain('Скопировать ссылку');
  });

  test('an expired invitation shows its state and offers only a new link', async () => {
    await show({ ...invitedCard, invitation: { url: null, expires_at: '2026-10-31T15:00:00Z', status: 'expired' } });

    const names = buttonNames();

    expect(screen.getByText('Приглашение истекло')).toBeDefined();
    expect(screen.queryByText(/\/invite\//)).toBeNull();
    expect(names).not.toContain('Скопировать ссылку');
    expect(names).toContain('Создать новую ссылку');
  });

  test('a link revoked after repeated wrong emails says so', async () => {
    await show({
      ...invitedCard,
      invitation: { url: null, expires_at: '2026-11-07T15:00:00Z', status: 'revoked_after_mismatches' },
    });

    const revoked = screen.getByText('Ссылка отозвана после нескольких попыток с другим email');

    expect(revoked).toBeDefined();
    expect(screen.queryByText(/\/invite\//)).toBeNull();
  });

  test('an active card shows the account and groups with no state word, link or withdrawal, and can be ended', async () => {
    await show(activeCard);

    const names = buttonNames();

    expect(screen.getByText('Анна Петрова')).toBeDefined();
    expect(screen.getByText('B1 вечер')).toBeDefined();
    expect(screen.queryByRole('heading', { name: 'Ссылка-приглашение' })).toBeNull();
    expect(screen.queryByLabelText('Email (необязательно)')).toBeNull();
    expect(names).toEqual(['Сохранить', 'Сохранить', 'Завершить занятия']);
    for (const word of ['Приглашение отправлено', 'Email не подтверждён', 'Ждём ответа ученика', 'Занятия завершены']) {
      expect(screen.queryByText(word)).toBeNull();
    }
  });

  test('a provisional card shows its state word, the account and the action that ends it', async () => {
    await show({ ...activeCard, state: 'provisional', groups: [] });

    const names = buttonNames();

    expect(screen.getByText('Email не подтверждён')).toBeDefined();
    expect(screen.getByText('Анна Петрова')).toBeDefined();
    expect(names).toEqual(['Сохранить', 'Сохранить', 'Завершить занятия']);
  });

  test('a reclaim-pending card shows its state word, no account and the action that ends it', async () => {
    await show({ ...activeCard, state: 'reclaim_pending', account: null, groups: [] });

    const names = buttonNames();

    expect(screen.getByText('Ждём ответа ученика')).toBeDefined();
    expect(screen.getByText('Владелец этого адреса решает, продолжать ли занятия с вами.')).toBeDefined();
    expect(screen.queryByText('Анна Петрова')).toBeNull();
    expect(screen.queryByRole('heading', { name: 'Аккаунт ученика' })).toBeNull();
    expect(names).toEqual(['Сохранить', 'Сохранить', 'Завершить занятия']);
  });

  test('an ended card without a link shows its state word and offers to invite again, not to end', async () => {
    await show({ ...activeCard, state: 'ended', groups: [] });

    const names = buttonNames();

    expect(screen.getByText('Занятия завершены')).toBeDefined();
    expect(screen.queryByRole('heading', { name: 'Ссылка-приглашение' })).toBeNull();
    expect(names).toEqual(['Пригласить снова', 'Сохранить', 'Сохранить']);
  });

  test('an ended card with an open link offers the link instead of inviting again', async () => {
    await show({ ...activeCard, state: 'ended', groups: [], invitation: invitedCard.invitation });

    const names = buttonNames();

    expect(screen.getByText(link)).toBeDefined();
    expect(names).toEqual([
      'Скопировать ссылку',
      'Отправить по email',
      'Создать новую ссылку',
      'Сохранить',
      'Сохранить',
    ]);
  });
});

describe('the student card page', () => {
  test('drops the card and its link from the query cache once the page is left', async () => {
    const session = await signedInTutor();
    const created = await session.authorizedFetch(`${origin}/api/v1/students`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ display_name: 'Борис' }),
    });
    const card = (await created.json()) as StudentCard;
    const queryClient = new QueryClient();
    const router = createMemoryRouter(
      [
        { path: '/students/:studentId', element: <StudentCardPage /> },
        { path: '/students', element: <p>Список</p> },
      ],
      { initialEntries: [`/students/${card.id}`] },
    );
    render(
      <SessionContext value={session}>
        <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
          <QueryClientProvider client={queryClient}>
            <I18nextProvider i18n={await createI18n('ru')}>
              <RouterProvider router={router} />
            </I18nextProvider>
          </QueryClientProvider>
        </ApiClientContext>
      </SessionContext>,
    );
    await screen.findByRole('heading', { level: 1, name: 'Борис' });
    expect(queryClient.getQueryData(studentCardKey(card.id))).toEqual({ ...card, next_lessons: [] });

    await act(async () => {
      await router.navigate('/students');
    });

    await waitFor(() => {
      expect(queryClient.getQueryData(studentCardKey(card.id))).toBeUndefined();
    });
    expect(screen.queryByText(card.invitation?.url ?? '')).toBeNull();
  });

  test('keeps unsaved typing and the loaded card when a background refetch fails, with a polite notice', async () => {
    const shown = await showLoadedCardWithTyping();

    await failOneRefetch(shown);

    const notice = screen.getByText(refetchFailedText);
    expect(notice.closest('[role="status"]')).not.toBeNull();
    expect(document.activeElement).toBe(shown.notes);
    expect(screen.getByRole('heading', { level: 1, name: 'Борис' })).toBeDefined();
    expect(screen.queryByText('Что-то пошло не так.')).toBeNull();
    expect(screen.getByRole('textbox', { name: 'Заметки' })).toBe(shown.notes);
    expect(shown.notes).toHaveProperty('value', 'Unsaved typing');
  });

  test('Retry on the refetch notice clears it, keeps the typing and moves focus to the page heading', async () => {
    const shown = await showLoadedCardWithTyping();
    await failOneRefetch(shown);
    const retry = screen.getByRole('button', { name: 'Повторить' });
    retry.focus();

    await act(async () => {
      retry.click();
      await Promise.resolve();
    });
    await waitFor(() => {
      expect(screen.queryByText(refetchFailedText)).toBeNull();
    });

    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Борис' }));
    expect(screen.getByRole('textbox', { name: 'Заметки' })).toBe(shown.notes);
    expect(shown.notes).toHaveProperty('value', 'Unsaved typing');
  });
});
