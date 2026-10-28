import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import { type ReactElement, type SyntheticEvent, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router';

import { type ActionState, done, refusedBy, showsStaleData, useAction } from '../../components/ui/action.ts';
import { Avatar } from '../../components/ui/Avatar.tsx';
import { ConfirmationPrompt } from '../../components/ui/ConfirmationPrompt.tsx';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormNotice,
  SubmitButton,
  TextField,
  fieldClass,
  fieldMessage,
  dangerButtonClass,
  linkClass,
  quietButtonClass,
  stringField,
  textButtonClass,
} from '../../components/ui/Form.tsx';
import { FocusedNotice, LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed, RefetchFailed } from '../../components/ui/LoadState.tsx';
import { useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Page } from '../../components/ui/Page.tsx';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { useStoredValue, valueAccess } from '../../components/ui/serverValue.ts';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import type { components } from '../../lib/api/schema.ts';
import { NotFoundContent } from '../../routes/NotFoundPage.tsx';
import { groupsKey } from '../groups/api.ts';
import type { CalendarItem } from '../schedule/api.ts';
import { LessonList } from '../schedule/LessonList.tsx';
import { ScheduleLessonAction } from '../schedule/ScheduleLessonAction.tsx';
import {
  type CardInvitation,
  type StudentCard,
  type StudentCardDetail,
  cardLessonsKey,
  fetchCardLessons,
  fetchStudentCard,
  hasNotice,
  removedNotice,
  studentCardKey,
  studentsKey,
  withdrawnNotice,
} from './api.ts';
import { GroupDots } from './GroupDots.tsx';
import { InvitationLink } from './InvitationLink.tsx';
import { StateBadge } from './StateBadge.tsx';

type StudentCardPatch = components['schemas']['StudentCardPatch'];

const tabClass = 'inline-flex min-h-11 items-center border-b-2 px-1 text-small';
const currentTabClass = `${tabClass} border-accent font-medium text-text`;
const otherTabClass = `${tabClass} border-transparent text-text-muted hover:text-text`;

export interface CardHandlers {
  readonly onChanged: (card: StudentCard) => void;
  readonly onStale: () => void;
  readonly onSent: () => void;
  readonly onWithdrawn: () => void;
  readonly onRemoved: (newCard: StudentCard) => void;
  readonly onEnded: (card: StudentCard) => void;
  readonly onChangedElsewhere: () => void;
  readonly onDialog: (open: boolean) => void;
  readonly onLeaving: (leaving: boolean) => void;
}

async function withoutLiveRefetch(
  handlers: CardHandlers,
  cardStays: boolean,
  work: () => Promise<ActionState>,
): Promise<ActionState> {
  handlers.onLeaving(true);
  let settled: ActionState | null = null;
  try {
    settled = await work();
    return settled;
  } finally {
    if (cardStays || settled?.kind !== 'done') {
      handlers.onLeaving(false);
    }
  }
}

function useCardDialog(handlers: CardHandlers): {
  readonly open: boolean;
  readonly show: (open: boolean) => void;
  readonly action: ReturnType<typeof useAction>;
} {
  const [open, setOpen] = useState(false);
  const show = (next: boolean): void => {
    setOpen(next);
    handlers.onDialog(next);
  };
  const action = useAction((state) => {
    if (showsStaleData(state)) {
      show(false);
      handlers.onChangedElsewhere();
    }
  });
  return { open, show, action };
}

function useStaleAwareAction(onStale: () => void): ReturnType<typeof useAction> {
  return useAction((state) => {
    if (showsStaleData(state)) {
      onStale();
    }
  });
}

function usePatch(
  cardId: string,
  handlers: CardHandlers,
): {
  readonly action: ReturnType<typeof useAction>;
  readonly save: (patch: StudentCardPatch) => void;
} {
  const client = useApiClient();
  const action = useStaleAwareAction(handlers.onStale);
  return {
    action,
    save: (patch): void => {
      action.run(async (): Promise<ActionState> => {
        const { data, error, response } = await client.PATCH('/api/v1/students/{id}', {
          params: { path: { id: cardId } },
          body: patch,
        });
        if (data === undefined) {
          return refusedBy(response, error);
        }
        handlers.onChanged(data);
        return done;
      });
    },
  };
}

function DetailsForm({
  card,
  reloads,
  handlers,
}: {
  readonly card: StudentCard;
  readonly reloads: number;
  readonly handlers: CardHandlers;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const { action, save } = usePatch(card.id, handlers);
  const error = action.state.kind === 'refused' ? action.state.error : null;
  const emailEditable = card.state === 'invited';

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const displayName = stringField(data, 'display_name');
    if (!emailEditable) {
      save({ display_name: displayName });
      return;
    }
    const email = stringField(data, 'email').trim();
    const emailPatch: StudentCardPatch = email === (card.email ?? '') ? {} : { email: email === '' ? null : email };
    save({ display_name: displayName, ...emailPatch });
  }

  return (
    <form
      aria-labelledby={headingId}
      onSubmit={submit}
      onChange={action.reset}
      className="flex max-w-sm flex-col gap-4"
    >
      <h2 id={headingId} className="text-heading">
        {t('studentCard.details')}
      </h2>
      <Refusal state={action.state} dailyLimit="emailChanges" />
      {action.state.kind === 'done' ? <FormNotice takeFocus>{t('studentCard.saved')}</FormNotice> : null}
      <TextField
        label={t('studentCard.displayName')}
        name="display_name"
        autoComplete="off"
        maxLength={100}
        defaultValue={card.display_name}
        reloads={reloads}
        hint={t('addStudent.displayNameHint')}
        error={fieldMessage(t, error, 'display_name')}
      />
      {emailEditable ? (
        <TextField
          label={t('addStudent.email')}
          name="email"
          type="email"
          autoComplete="off"
          required={false}
          maxLength={254}
          defaultValue={card.email ?? ''}
          reloads={reloads}
          hint={t('studentCard.emailChangeHint')}
          error={fieldMessage(t, error, 'email')}
        />
      ) : null}
      <p>
        <SubmitButton busy={action.busy} quiet>
          {t('studentCard.save')}
        </SubmitButton>
      </p>
    </form>
  );
}

function NotesForm({
  card,
  reloads,
  handlers,
}: {
  readonly card: StudentCard;
  readonly reloads: number;
  readonly handlers: CardHandlers;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const notesId = useId();
  const hintId = useId();
  const notes = useStoredValue<HTMLTextAreaElement>(card.notes, reloads, valueAccess);
  const [initialNotes] = useState(card.notes);
  const { action, save } = usePatch(card.id, handlers);

  return (
    <form
      aria-labelledby={headingId}
      onChange={action.reset}
      onSubmit={(event: SyntheticEvent<HTMLFormElement, SubmitEvent>) => {
        event.preventDefault();
        save({ notes: stringField(new FormData(event.currentTarget), 'notes') });
      }}
      className="flex flex-col gap-3"
    >
      <h2 id={headingId} className="text-heading">
        {t('studentCard.notes')}
      </h2>
      <Refusal state={action.state} />
      {action.state.kind === 'done' ? <FormNotice takeFocus>{t('studentCard.saved')}</FormNotice> : null}
      <label htmlFor={notesId} className="sr-only">
        {t('studentCard.notes')}
      </label>
      <textarea
        ref={notes}
        id={notesId}
        name="notes"
        rows={6}
        maxLength={10000}
        defaultValue={initialNotes}
        aria-describedby={hintId}
        className={`${fieldClass} py-2`}
      />
      <p id={hintId} className="text-small text-text-muted">
        {t('studentCard.notesHint')}
      </p>
      <p>
        <SubmitButton busy={action.busy} quiet>
          {t('studentCard.save')}
        </SubmitButton>
      </p>
    </form>
  );
}

function WithdrawInvitation({
  cardId,
  handlers,
}: {
  readonly cardId: string;
  readonly handlers: CardHandlers;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const { open, show, action } = useCardDialog(handlers);
  const opener = useRef<HTMLButtonElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  const headingId = useId();
  const consequenceId = useId();

  function withdraw(): void {
    action.run(() =>
      withoutLiveRefetch(handlers, false, async (): Promise<ActionState> => {
        const { error, response } = await client.DELETE('/api/v1/students/{id}', {
          params: { path: { id: cardId } },
        });
        if (!response.ok) {
          return refusedBy(response, error);
        }
        handlers.onWithdrawn();
        return done;
      }),
    );
  }

  return (
    <section className="flex flex-col items-start gap-3">
      <button
        ref={opener}
        type="button"
        onClick={() => {
          action.reset();
          show(true);
        }}
        className={dangerButtonClass}
      >
        {t('studentCard.withdraw')}
      </button>
      <ModalDialog
        open={open}
        busy={action.busy}
        labelledBy={headingId}
        describedBy={consequenceId}
        returnFocus={opener}
        initialFocus={cancel}
        onClose={() => {
          show(false);
        }}
      >
        <div className="flex flex-col gap-4">
          <h2 id={headingId} className="text-heading">
            {t('studentCard.withdrawTitle')}
          </h2>
          <p id={consequenceId}>{t('studentCard.withdrawConsequence')}</p>
          <Refusal state={action.state} />
          <div className="flex flex-wrap gap-3">
            <ActionButton busy={action.busy} onClick={withdraw} className={dangerButtonClass}>
              {t('studentCard.withdrawSubmit')}
            </ActionButton>
            <ActionButton
              ref={cancel}
              busy={false}
              unavailable={action.busy}
              onClick={() => {
                show(false);
              }}
              className={textButtonClass}
            >
              {t('studentCard.cancel')}
            </ActionButton>
          </div>
        </div>
      </ModalDialog>
    </section>
  );
}

function EndWorkingTogether({
  consequence,
  handlers,
  end,
}: {
  readonly consequence: string;
  readonly handlers: CardHandlers;
  readonly end: () => Promise<ActionState>;
}): ReactElement {
  const { t } = useTranslation();
  const { open, show, action } = useCardDialog(handlers);
  const opener = useRef<HTMLButtonElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  const headingId = useId();
  const consequenceId = useId();

  return (
    <section className="flex flex-col items-start gap-3">
      <button
        ref={opener}
        type="button"
        onClick={() => {
          action.reset();
          show(true);
        }}
        className={dangerButtonClass}
      >
        {t('studentCard.end')}
      </button>
      <ModalDialog
        open={open}
        busy={action.busy}
        labelledBy={headingId}
        describedBy={consequenceId}
        returnFocus={opener}
        initialFocus={cancel}
        onClose={() => {
          show(false);
        }}
      >
        <div className="flex flex-col gap-4">
          <h2 id={headingId} className="text-heading">
            {t('studentCard.removeTitle')}
          </h2>
          <p id={consequenceId}>{consequence}</p>
          <Refusal state={action.state} />
          <div className="flex flex-wrap gap-3">
            <ActionButton
              busy={action.busy}
              onClick={() => {
                action.run(end);
              }}
              className={dangerButtonClass}
            >
              {t('studentCard.removeSubmit')}
            </ActionButton>
            <ActionButton
              ref={cancel}
              busy={false}
              unavailable={action.busy}
              onClick={() => {
                show(false);
              }}
              className={textButtonClass}
            >
              {t('studentCard.cancel')}
            </ActionButton>
          </div>
        </div>
      </ModalDialog>
    </section>
  );
}

function RemoveStudent({
  cardId,
  state,
  handlers,
}: {
  readonly cardId: string;
  readonly state: 'provisional' | 'reclaim_pending';
  readonly handlers: CardHandlers;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  return (
    <EndWorkingTogether
      consequence={t(
        state === 'provisional' ? 'studentCard.removeConsequence' : 'studentCard.removeReclaimConsequence',
      )}
      handlers={handlers}
      end={() =>
        withoutLiveRefetch(handlers, false, async (): Promise<ActionState> => {
          const { data, error, response } = await client.POST('/api/v1/students/{id}/remove', {
            params: { path: { id: cardId } },
          });
          if (data === undefined) {
            return refusedBy(response, error);
          }
          handlers.onRemoved(data.new_card);
          return done;
        })
      }
    />
  );
}

function EndRelationship({
  cardId,
  handlers,
}: {
  readonly cardId: string;
  readonly handlers: CardHandlers;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  return (
    <EndWorkingTogether
      consequence={t('studentCard.endConsequence')}
      handlers={handlers}
      end={() =>
        withoutLiveRefetch(handlers, true, async (): Promise<ActionState> => {
          const { data, error, response } = await client.POST('/api/v1/students/{id}/end', {
            params: { path: { id: cardId } },
          });
          if (data === undefined) {
            return refusedBy(response, error);
          }
          handlers.onEnded(data);
          return done;
        })
      }
    />
  );
}

function InviteAgain({
  cardId,
  handlers,
  onInvited,
}: {
  readonly cardId: string;
  readonly handlers: CardHandlers;
  readonly onInvited: (invitation: CardInvitation) => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const headingId = useId();
  const action = useAction((state) => {
    if (showsStaleData(state)) {
      handlers.onChangedElsewhere();
    }
  });

  function invite(): void {
    action.run(async (): Promise<ActionState> => {
      const { data, error, response } = await client.POST('/api/v1/students/{id}/invitation', {
        params: { path: { id: cardId } },
      });
      if (data === undefined) {
        return refusedBy(response, error);
      }
      onInvited(data.invitation);
      return done;
    });
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col items-start gap-3">
      <h2 id={headingId} className="text-heading">
        {t('studentCard.inviteAgainTitle')}
      </h2>
      <p className="text-small text-text-muted">{t('studentCard.inviteAgainHint')}</p>
      {showsStaleData(action.state) ? null : (
        <Refusal state={action.state} dailyLimit="invitations" confirmation="invite" />
      )}
      <ActionButton busy={action.busy} onClick={invite} className={quietButtonClass}>
        {t('studentCard.inviteAgain')}
      </ActionButton>
    </section>
  );
}

function AccountDetails({ account }: { readonly account: NonNullable<StudentCard['account']> }): ReactElement {
  const { t } = useTranslation();
  return (
    <section className="flex flex-col gap-1">
      <h2 className="text-label text-text-muted">{t('studentCard.account')}</h2>
      <p className="text-strong">{account.name}</p>
      <p className="text-small text-text-muted">{account.email}</p>
    </section>
  );
}

function CardSummary({ card }: { readonly card: StudentCard }): ReactElement {
  const { t } = useTranslation();
  const email = card.email ?? card.account?.email ?? null;
  return (
    <>
      <p>
        <Link to="/students" className={linkClass}>
          {t('studentCard.back')}
        </Link>
      </p>
      <div className="flex items-center gap-4">
        <Avatar name={card.display_name} avatarId={card.account?.avatar_id ?? null} size="large" />
        <div className="flex flex-col gap-1">
          <p className="text-text-muted">{email ?? t('students.noEmail')}</p>
          <StateBadge state={card.state} />
          <GroupDots groups={card.groups} />
        </div>
      </div>
      {card.account === null ? null : <AccountDetails account={card.account} />}
      {card.state === 'reclaim_pending' ? <p className="text-text-muted">{t('studentCard.waitingForOwner')}</p> : null}
    </>
  );
}

function CardOverview({
  card,
  reloads,
  handlers,
}: {
  readonly card: StudentCard;
  readonly reloads: number;
  readonly handlers: CardHandlers;
}): ReactElement {
  const [invitedAgain, setInvitedAgain] = useState<string | null>(null);
  const linkState = card.state === 'invited' || card.state === 'ended';
  return (
    <>
      {linkState && card.invitation !== null ? (
        <InvitationLink
          cardId={card.id}
          invitation={card.invitation}
          canSend={card.email !== null}
          initialNotice={invitedAgain === card.id ? 'created' : 'none'}
          onRegenerated={(invitation) => {
            handlers.onChanged({ ...card, invitation });
          }}
          onSent={handlers.onSent}
          onStale={handlers.onStale}
        />
      ) : null}
      {card.state === 'ended' && card.invitation === null ? (
        <InviteAgain
          cardId={card.id}
          handlers={handlers}
          onInvited={(invitation) => {
            setInvitedAgain(card.id);
            handlers.onChanged({ ...card, invitation });
          }}
        />
      ) : null}
      <DetailsForm card={card} reloads={reloads} handlers={handlers} />
      <NotesForm card={card} reloads={reloads} handlers={handlers} />
      {card.state === 'invited' ? <WithdrawInvitation cardId={card.id} handlers={handlers} /> : null}
      {card.state === 'provisional' || card.state === 'reclaim_pending' ? (
        <RemoveStudent key={card.state} cardId={card.id} state={card.state} handlers={handlers} />
      ) : null}
      {card.state === 'active' ? <EndRelationship cardId={card.id} handlers={handlers} /> : null}
    </>
  );
}

export function StudentCardView({
  card,
  reloads = 0,
  handlers,
}: {
  readonly card: StudentCard;
  readonly reloads?: number;
  readonly handlers: CardHandlers;
}): ReactElement {
  return (
    <>
      <CardSummary card={card} />
      <CardOverview card={card} reloads={reloads} handlers={handlers} />
    </>
  );
}

type CardTab = 'overview' | 'lessons';

function cardTabOf(value: string | null): CardTab {
  return value === 'lessons' ? 'lessons' : 'overview';
}

function CardTabs({ cardId, tab }: { readonly cardId: string; readonly tab: CardTab }): ReactElement {
  const { t } = useTranslation();
  const tabs: readonly { readonly key: CardTab; readonly to: string }[] = [
    { key: 'overview', to: `/students/${encodeURIComponent(cardId)}` },
    { key: 'lessons', to: `/students/${encodeURIComponent(cardId)}?tab=lessons` },
  ];
  return (
    <nav aria-label={t('studentCard.sections')} className="border-b border-border">
      <ul className="flex flex-wrap gap-6">
        {tabs.map((entry) => (
          <li key={entry.key}>
            <Link
              to={entry.to}
              aria-current={entry.key === tab ? 'page' : undefined}
              className={entry.key === tab ? currentTabClass : otherTabClass}
            >
              {t(`studentCard.${entry.key}`)}
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}

function NextLessons({ items }: { readonly items: readonly CalendarItem[] }): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const headingId = useId();
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 id={headingId} className="text-heading">
        {t('lessons.next')}
      </h2>
      {items.length === 0 ? (
        <p className="text-text-muted">{t('lessons.noNext')}</p>
      ) : (
        <LessonList items={items} label={t('lessons.next')} me={me} showAudience />
      )}
    </section>
  );
}

function CardLessonHistory({ cardId }: { readonly cardId: string }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const me = useSignedInAccount();
  const query = useInfiniteQuery({
    queryKey: cardLessonsKey(cardId),
    queryFn: ({ pageParam }) => fetchCardLessons(client, cardId, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    retry: false,
  });
  const items = query.data?.pages.flatMap((page) => page.items) ?? [];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(
    items.length,
    ':scope > li > a',
    query.isFetchingNextPage,
  );
  const cardGone = query.isError && asApiError(query.error).code === 'not_found';

  useEffect(() => {
    if (cardGone) {
      void queryClient.invalidateQueries({ queryKey: studentCardKey(cardId), exact: true });
    }
  }, [cardGone, cardId, queryClient]);
  if (query.data === undefined) {
    return query.isError ? (
      <LoadFailed
        error={asApiError(query.error)}
        onRetry={() => {
          void query.refetch();
        }}
      />
    ) : (
      <DelayedLoading />
    );
  }
  const failure =
    query.isError && !cardGone ? (
      <RefetchFailed
        error={asApiError(query.error)}
        onRetry={() => {
          void (query.isFetchNextPageError ? query.fetchNextPage() : query.refetch());
        }}
      />
    ) : null;
  if (items.length === 0) {
    return (
      <div className="flex flex-col items-start gap-4">
        {failure}
        <p className="text-text-muted">{t('lessons.historyEmpty')}</p>
      </div>
    );
  }
  return (
    <div className="flex flex-col items-start gap-4">
      {failure}
      <LessonList items={items} label={t('lessons.history')} me={me} showAudience listRef={listRef} />
      {query.hasNextPage ? (
        <button
          type="button"
          aria-disabled={query.isFetchingNextPage}
          aria-busy={query.isFetchingNextPage}
          onClick={() => {
            if (!query.isFetchingNextPage) {
              loadingMore();
              void query.fetchNextPage();
            }
          }}
          className={quietButtonClass}
        >
          {t('lessons.showMore')}
        </button>
      ) : null}
    </div>
  );
}

function schedulable(card: StudentCard): boolean {
  return card.state === 'active' || card.state === 'provisional';
}

function CardWithLessons({
  card,
  tab,
  reloads,
  handlers,
  onScheduled,
}: {
  readonly card: StudentCardDetail;
  readonly tab: CardTab;
  readonly reloads: number;
  readonly handlers: CardHandlers;
  readonly onScheduled: () => void;
}): ReactElement {
  const me = useSignedInAccount();
  return (
    <>
      <CardSummary card={card} />
      {me.state === 'active' && schedulable(card) ? (
        <ScheduleLessonAction
          audience={{ kind: 'one_to_one', id: card.id, name: card.display_name }}
          onScheduled={onScheduled}
        />
      ) : null}
      <CardTabs cardId={card.id} tab={tab} />
      {tab === 'lessons' ? (
        <CardLessonHistory cardId={card.id} />
      ) : (
        <>
          <NextLessons items={card.next_lessons} />
          <CardOverview card={card} reloads={reloads} handlers={handlers} />
        </>
      )}
    </>
  );
}

export function StudentCardPage(): ReactElement {
  const { studentId = '' } = useParams();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const tab = cardTabOf(searchParams.get('tab'));
  const { t } = useTranslation();
  const [removedCard, setRemovedCard] = useState<string | null>(null);
  const removalNoticed = hasNotice(location.state, removedNotice);
  if (removalNoticed && removedCard !== studentId) {
    setRemovedCard(studentId);
  } else if (!removalNoticed && removedCard !== null && removedCard !== studentId) {
    setRemovedCard(null);
  }
  const [changedCard, setChangedCard] = useState<{ readonly id: string; readonly count: number } | null>(null);
  if (changedCard !== null && changedCard.id !== studentId) {
    setChangedCard(null);
  }
  const [endedCard, setEndedCard] = useState<string | null>(null);
  if (endedCard !== null && endedCard !== studentId) {
    setEndedCard(null);
  }
  const key = studentCardKey(studentId);
  const [reloads, setReloads] = useState(0);

  function keepLessons(card: StudentCard): void {
    queryClient.setQueryData<StudentCardDetail>(key, (previous) =>
      previous === undefined ? previous : { ...card, next_lessons: previous.next_lessons },
    );
  }

  function reloadAfterStaleEdit(): void {
    void queryClient.invalidateQueries({ queryKey: key }).then(() => {
      setReloads((count) => count + 1);
    });
  }

  useEffect(() => {
    if (hasNotice(location.state, removedNotice)) {
      void navigate(`${location.pathname}${location.search}`, { replace: true, state: null });
    }
  }, [location, navigate]);
  const [leavingCard, setLeavingCard] = useState<string | null>(null);
  const query = useQuery({
    queryKey: key,
    queryFn: () => fetchStudentCard(client, studentId),
    enabled: leavingCard !== studentId,
    gcTime: 0,
    retry: false,
  });

  const queryError = query.isError ? asApiError(query.error) : null;
  const refetchError =
    queryError !== null && queryError.code !== 'not_found' && query.data !== undefined ? queryError : null;
  const error = refetchError === null ? queryError : null;
  const gone = error?.code === 'not_found' && query.data !== undefined;
  const notFound = error?.code === 'not_found' && !gone;
  const [dialogSubject, setDialogSubject] = useState<{ readonly id: string; readonly state: string } | null>(null);
  if (
    dialogSubject !== null &&
    (dialogSubject.id !== studentId || gone || (query.data !== undefined && query.data.state !== dialogSubject.state))
  ) {
    setDialogSubject(null);
    if (dialogSubject.id === studentId && !gone) {
      setEndedCard(null);
      setChangedCard((previous) => ({ id: studentId, count: (previous?.count ?? 0) + 1 }));
    }
  }

  function body(): ReactElement {
    if (gone) {
      return (
        <>
          <FocusedNotice message={t('studentCard.gone')} whenFocusIsLost />
          <p>
            <Link to="/students" className={linkClass}>
              {t('studentCard.back')}
            </Link>
          </p>
        </>
      );
    }
    if (error !== null) {
      if (notFound) {
        return <NotFoundContent />;
      }
      return error.code === 'email_not_verified' ? (
        <ConfirmationPrompt reason="invite" />
      ) : (
        <LoadFailed
          error={error}
          onRetry={() => {
            void query.refetch();
          }}
        />
      );
    }
    if (query.data === undefined) {
      return <DelayedLoading />;
    }
    const shown = query.data;
    return (
      <CardWithLessons
        card={shown}
        tab={tab}
        reloads={reloads}
        onScheduled={() => {
          void queryClient.invalidateQueries({ queryKey: key });
          void queryClient.invalidateQueries({ queryKey: studentsKey, refetchType: 'none' });
        }}
        handlers={{
          onChanged: (card) => {
            keepLessons(card);
            void queryClient.invalidateQueries({ queryKey: key });
            void queryClient.invalidateQueries({ queryKey: studentsKey, refetchType: 'none' });
          },
          onStale: reloadAfterStaleEdit,
          onSent: () => {
            void queryClient.invalidateQueries({ queryKey: key });
          },
          onWithdrawn: () => {
            void navigate('/students?state=invited', { state: { notice: withdrawnNotice } });
            void queryClient.invalidateQueries({ queryKey: studentsKey, refetchType: 'none' });
          },
          onEnded: (card) => {
            setDialogSubject(null);
            setChangedCard(null);
            setEndedCard(studentId);
            keepLessons(card);
            void queryClient.invalidateQueries({ queryKey: key });
            void queryClient.invalidateQueries({ queryKey: studentsKey, refetchType: 'none' });
            void queryClient.invalidateQueries({ queryKey: groupsKey, refetchType: 'none' });
          },
          onChangedElsewhere: () => {
            setDialogSubject(null);
            setEndedCard(null);
            setChangedCard((previous) => ({ id: studentId, count: (previous?.count ?? 0) + 1 }));
            reloadAfterStaleEdit();
          },
          onLeaving: (leaving) => {
            setLeavingCard(leaving ? studentId : null);
          },
          onDialog: (open) => {
            setDialogSubject(open ? { id: studentId, state: shown.state } : null);
          },
          onRemoved: (newCard) => {
            void queryClient.invalidateQueries({ queryKey: studentsKey, refetchType: 'none' });
            void navigate(`/students/${encodeURIComponent(newCard.id)}`, {
              replace: true,
              state: { notice: removedNotice },
            });
          },
        }}
      />
    );
  }

  return (
    <Page
      {...(notFound ? { title: 'page.notFound' as const } : {})}
      {...(error === null && query.data !== undefined ? { headingText: query.data.display_name } : {})}
      focusTitle={notFound}
    >
      {removedCard === null ? null : <LiveNotice message={t('studentCard.removed')} />}
      {changedCard === null || notFound || gone ? null : (
        <FocusedNotice key={changedCard.count} message={t('studentCard.changedElsewhere')} />
      )}
      {endedCard === null || notFound || gone ? null : <FocusedNotice message={t('studentCard.ended')} />}
      {refetchError === null ? null : (
        <RefetchFailed
          error={refetchError}
          onRetry={() => {
            void query.refetch();
          }}
        />
      )}
      {body()}
    </Page>
  );
}
