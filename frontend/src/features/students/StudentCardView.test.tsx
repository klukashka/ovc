import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider, createMemoryRouter } from 'react-router';
import { afterEach, describe, expect, test } from 'vitest';

import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { SchedulerContext } from '../../app/clock.ts';
import { FakeAuthServer, type ManualScheduler, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
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
  return screen.getAllByRole('button').map((button) => button.getAttribute('aria-label') ?? button.textContent);
}

function cardActionNames(): string[] {
  const menuButton = screen.getByRole('button', { name: 'Действия с карточкой' });
  fireEvent.click(menuButton);
  const menu = document.getElementById(menuButton.getAttribute('aria-controls') ?? '');
  return [...(menu?.querySelectorAll('button') ?? [])].map((button) => button.textContent);
}

const refetchFailedText = 'Не удалось обновить данные. На странице показано то, что загрузилось раньше.';

interface LoadedCard {
  readonly server: FakeAuthServer;
  readonly queryClient: QueryClient;
  readonly card: StudentCard;
  readonly notes: HTMLElement;
  readonly scheduler: ManualScheduler;
  readonly leave: () => Promise<void>;
  readonly open: (other: StudentCard) => Promise<void>;
}

async function createCard(session: Session, displayName: string): Promise<StudentCard> {
  const created = await session.authorizedFetch(`${origin}/api/v1/students`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ display_name: displayName }),
  });
  return (await created.json()) as StudentCard;
}

async function storeNotes(session: Session, card: StudentCard, notes: string): Promise<StudentCard> {
  const written = await session.authorizedFetch(`${origin}/api/v1/students/${card.id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ notes }),
  });
  return (await written.json()) as StudentCard;
}

async function renderCardPage(
  server: FakeAuthServer,
  session: Session,
  card: StudentCard,
  queryClient: QueryClient,
): Promise<LoadedCard> {
  const scheduler = manualScheduler();
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
      { path: '/students', element: <p>Список</p> },
    ],
    { initialEntries: [`/students/${card.id}`] },
  );
  render(
    <SessionContext value={session}>
      <SchedulerContext value={scheduler}>
        <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
          <QueryClientProvider client={queryClient}>
            <I18nextProvider i18n={await createI18n('ru')}>
              <RouterProvider router={router} />
            </I18nextProvider>
          </QueryClientProvider>
        </ApiClientContext>
      </SchedulerContext>
    </SessionContext>,
  );
  await screen.findByRole('heading', { level: 1, name: card.display_name });
  const notes = screen.getByRole('textbox', { name: 'Заметки' });
  notes.focus();
  const leave = async (): Promise<void> => {
    await act(async () => {
      await router.navigate('/students');
    });
  };
  const open = async (other: StudentCard): Promise<void> => {
    await act(async () => {
      await router.navigate(`/students/${other.id}`);
    });
    await screen.findByRole('heading', { level: 1, name: other.display_name });
  };
  return { server, queryClient, card, notes, scheduler, leave, open };
}

async function showLoadedCard(): Promise<LoadedCard> {
  const server = new FakeAuthServer();
  const session = await signedInTutor(server);
  const card = await createCard(session, 'Борис');
  return renderCardPage(server, session, card, new QueryClient());
}

const otherNotes = 'Заметки Веры';

async function showLoadedCardBesideCachedCard(): Promise<LoadedCard & { readonly other: StudentCard }> {
  const server = new FakeAuthServer();
  const session = await signedInTutor(server);
  const card = await createCard(session, 'Борис');
  const other = await storeNotes(session, await createCard(session, 'Вера'), otherNotes);
  const queryClient = new QueryClient();
  queryClient.setQueryData(studentCardKey(other.id), { ...other, next_lessons: [] });
  return { ...(await renderCardPage(server, session, card, queryClient)), other };
}

async function showLoadedCardWithTyping(): Promise<LoadedCard> {
  const shown = await showLoadedCard();
  fireEvent.change(shown.notes, { target: { value: 'Unsaved typing' } });
  return shown;
}

function type(notes: HTMLElement, value: string): void {
  fireEvent.change(notes, { target: { value } });
}

function notesStatus(): string | null {
  return within(screen.getByRole('form', { name: 'Заметки' })).getByRole('status').textContent;
}

function pass(scheduler: ManualScheduler, milliseconds: number): void {
  act(() => {
    scheduler.pass(milliseconds);
  });
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
    const actions = cardActionNames();

    expect(screen.getByText('Приглашение отправлено')).toBeDefined();
    expect(screen.getByText(link)).toBeDefined();
    expect(screen.getByText('Действует до 7 нояб., 18:00')).toBeDefined();
    expect(names).toEqual(['Действия с карточкой', 'Скопировать ссылку', 'Отправить по email', 'Создать новую ссылку']);
    expect(actions).toEqual(['Имя и email…', 'Отозвать приглашение']);
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
    const actions = cardActionNames();

    expect(screen.getByText('Анна Петрова')).toBeDefined();
    expect(screen.getByText('B1 вечер')).toBeDefined();
    expect(screen.queryByRole('heading', { name: 'Ссылка-приглашение' })).toBeNull();
    expect(screen.queryByLabelText('Email (необязательно)')).toBeNull();
    expect(names).toEqual(['Действия с карточкой']);
    expect(actions).toEqual(['Имя и email…', 'Завершить занятия']);
    for (const word of ['Приглашение отправлено', 'Email не подтверждён', 'Ждём ответа ученика', 'Занятия завершены']) {
      expect(screen.queryByText(word)).toBeNull();
    }
  });

  test('a provisional card shows its state word, the account and the action that ends it', async () => {
    await show({ ...activeCard, state: 'provisional', groups: [] });

    const names = buttonNames();
    const actions = cardActionNames();

    expect(screen.getByText('Email не подтверждён')).toBeDefined();
    expect(screen.getByText('Анна Петрова')).toBeDefined();
    expect(names).toEqual(['Действия с карточкой']);
    expect(actions).toEqual(['Имя и email…', 'Завершить занятия']);
  });

  test('a reclaim-pending card shows its state word, no account and the action that ends it', async () => {
    await show({ ...activeCard, state: 'reclaim_pending', account: null, groups: [] });

    const names = buttonNames();
    const actions = cardActionNames();

    expect(screen.getByText('Ждём ответа ученика')).toBeDefined();
    expect(screen.getByText('Владелец этого адреса решает, продолжать ли занятия с вами.')).toBeDefined();
    expect(screen.queryByText('Анна Петрова')).toBeNull();
    expect(screen.queryByRole('heading', { name: 'Аккаунт ученика' })).toBeNull();
    expect(names).toEqual(['Действия с карточкой']);
    expect(actions).toEqual(['Имя и email…', 'Завершить занятия']);
  });

  test('an ended card without a link shows its state word and offers to invite again, not to end', async () => {
    await show({ ...activeCard, state: 'ended', groups: [] });

    const names = buttonNames();
    const actions = cardActionNames();

    expect(screen.getByText('Занятия завершены')).toBeDefined();
    expect(screen.queryByRole('heading', { name: 'Ссылка-приглашение' })).toBeNull();
    expect(names).toEqual(['Действия с карточкой', 'Пригласить снова']);
    expect(actions).toEqual(['Имя и email…']);
  });

  test('an ended card with an open link offers the link instead of inviting again', async () => {
    await show({ ...activeCard, state: 'ended', groups: [], invitation: invitedCard.invitation });

    const names = buttonNames();
    const actions = cardActionNames();

    expect(screen.getByText(link)).toBeDefined();
    expect(names).toEqual(['Действия с карточкой', 'Скопировать ссылку', 'Отправить по email', 'Создать новую ссылку']);
    expect(actions).toEqual(['Имя и email…']);
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

describe('the notes of a student card', () => {
  test('are saved one second after typing stops, with only the notes, and say Saving then Saved', async () => {
    const { server, notes, scheduler } = await showLoadedCard();
    const paused = server.pauseCardWrites();
    type(notes, 'Пред');
    pass(scheduler, 999);
    type(notes, 'Предпочитает вечер');
    pass(scheduler, 999);
    const writesBeforeTheDelay = [...server.cardWrites];

    pass(scheduler, 1);
    await paused.received;
    const writesWhileSaving = [...server.cardWrites];
    const statusWhileSaving = notesStatus();
    paused.resume();
    await screen.findByText('Сохранено.');

    expect(writesBeforeTheDelay).toEqual([]);
    expect(writesWhileSaving).toEqual(['{"notes":"Предпочитает вечер"}']);
    expect(statusWhileSaving).toBe('Сохранение…');
    expect(server.cardWrites).toEqual(['{"notes":"Предпочитает вечер"}']);
    expect(notesStatus()).toBe('Сохранено.');
    expect(screen.getByText('Сохранено.').closest('[role="status"]')).not.toBeNull();
    expect(document.activeElement).toBe(notes);
  });

  test('are saved at once when the field is left, and the pending delay sends nothing more', async () => {
    const { server, notes, scheduler } = await showLoadedCard();
    type(notes, 'Ушёл из поля');

    fireEvent.blur(notes);
    await screen.findByText('Сохранено.');
    pass(scheduler, 1000);

    expect(server.cardWrites).toEqual(['{"notes":"Ушёл из поля"}']);
  });

  test('send one request at a time and save typing made during a save after it without overwriting it', async () => {
    const { server, notes, scheduler } = await showLoadedCard();
    const paused = server.pauseCardWrites();
    type(notes, 'Первое');
    pass(scheduler, 1000);
    await paused.received;
    type(notes, 'Первое и второе');
    pass(scheduler, 1000);
    const writesWhileTheFirstIsInFlight = [...server.cardWrites];

    paused.resume();
    await waitFor(() => {
      expect(server.cardWrites).toHaveLength(2);
    });
    await screen.findByText('Сохранено.');

    expect(writesWhileTheFirstIsInFlight).toEqual(['{"notes":"Первое"}']);
    expect(server.cardWrites).toEqual(['{"notes":"Первое"}', '{"notes":"Первое и второе"}']);
    expect(notes).toHaveProperty('value', 'Первое и второе');
  });

  test('that failed to save keep the text and say so, and Retry saves them', async () => {
    const { server, notes, scheduler } = await showLoadedCard();
    server.failNextCardWrite();
    type(notes, 'Не потерять');
    pass(scheduler, 1000);
    await screen.findByText('Заметки не сохранены.');
    const statusAfterTheFailure = notesStatus();
    const valueAfterTheFailure = (notes as HTMLTextAreaElement).value;
    const focusAfterTheFailure = document.activeElement;

    await act(async () => {
      screen.getByRole('button', { name: 'Попробовать ещё раз' }).click();
      await Promise.resolve();
    });
    await screen.findByText('Сохранено.');

    expect(statusAfterTheFailure).toBe('Заметки не сохранены.');
    expect(valueAfterTheFailure).toBe('Не потерять');
    expect(focusAfterTheFailure).toBe(notes);
    expect(server.cardWrites).toEqual(['{"notes":"Не потерять"}', '{"notes":"Не потерять"}']);
    expect(screen.queryByText('Заметки не сохранены.')).toBeNull();
  });

  test('that were refused are not resent on leaving the field, but a correction typed after the refusal is', async () => {
    const { server, notes, scheduler } = await showLoadedCard();
    server.failNextCardWrite();
    type(notes, 'Отклонено');
    pass(scheduler, 1000);
    await screen.findByText('Заметки не сохранены.');
    fireEvent.blur(notes);
    const writesAfterLeavingRefused = [...server.cardWrites];

    type(notes, 'Исправлено');
    fireEvent.blur(notes);
    await screen.findByText('Сохранено.');

    expect(writesAfterLeavingRefused).toEqual(['{"notes":"Отклонено"}']);
    expect(server.cardWrites).toEqual(['{"notes":"Отклонено"}', '{"notes":"Исправлено"}']);
  });

  test('typed just before leaving the card are not sent once the page is gone', async () => {
    const { server, notes, scheduler, leave } = await showLoadedCard();
    type(notes, 'Не отправлять');

    await leave();
    pass(scheduler, 1000);

    expect(server.cardWrites).toEqual([]);
    expect(scheduler.pendingDelays()).not.toContain(1000);
  });

  test('refused on one card are neither shown nor offered again nor sent once a cached card is opened', async () => {
    const { server, card, notes, scheduler, other, open } = await showLoadedCardBesideCachedCard();
    server.failNextCardWrite();
    type(notes, 'Заметки Бориса');
    pass(scheduler, 1000);
    await screen.findByText('Заметки не сохранены.');

    await open(other);
    const otherField = screen.getByRole('textbox', { name: 'Заметки' });
    fireEvent.blur(otherField);
    pass(scheduler, 1000);

    expect(otherField).toHaveProperty('value', otherNotes);
    expect(notesStatus()).toBe('');
    expect(screen.queryByText('Заметки не сохранены.')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Попробовать ещё раз' })).toBeNull();
    expect(server.cardWritesTo(other.id)).toEqual([`{"notes":"${otherNotes}"}`]);
    expect(server.cardWritesTo(card.id)).toEqual(['{"notes":"Заметки Бориса"}']);
  });

  test('saved on one card while a cached card is opened leave the other card its own text and status', async () => {
    const { server, card, notes, scheduler, other, open } = await showLoadedCardBesideCachedCard();
    const paused = server.pauseCardWrites();
    type(notes, 'Заметки Бориса');
    pass(scheduler, 1000);
    await paused.received;

    await open(other);
    const otherField = screen.getByRole('textbox', { name: 'Заметки' });
    const statusAfterOpening = notesStatus();
    paused.resume();
    type(otherField, `${(otherField as HTMLTextAreaElement).value}!`);
    fireEvent.blur(otherField);
    await waitFor(() => {
      if (server.cardWritesTo(other.id).length < 2) {
        throw new Error('the other card has not been saved yet');
      }
    });
    await screen.findByText('Сохранено.');

    expect(statusAfterOpening).toBe('');
    expect(otherField).toHaveProperty('value', `${otherNotes}!`);
    expect(notesStatus()).toBe('Сохранено.');
    expect(server.cardWritesTo(card.id)).toEqual(['{"notes":"Заметки Бориса"}']);
    expect(server.cardWritesTo(other.id)).toEqual([`{"notes":"${otherNotes}"}`, `{"notes":"${otherNotes}!"}`]);
  });
});
