import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import {
  type ReactElement,
  type ReactNode,
  type RefObject,
  type SyntheticEvent,
  useEffect,
  useId,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router';

import { type ActionState, done, refusedBy, showsStaleData, useAction } from '../../components/ui/action.ts';
import { ActionMenu, type MenuItem } from '../../components/ui/ActionMenu.tsx';
import { Avatar } from '../../components/ui/Avatar.tsx';
import { ConfirmationPrompt } from '../../components/ui/ConfirmationPrompt.tsx';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormActions,
  SubmitButton,
  TextField,
  fieldClass,
  fieldMessage,
  dangerFilledButtonClass,
  linkClass,
  quietButtonClass,
  stringField,
} from '../../components/ui/Form.tsx';
import { MoreIcon } from '../../components/ui/icons.tsx';
import { FocusedNotice, LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed, RefetchFailed } from '../../components/ui/LoadState.tsx';
import { loadNextPage, useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Page } from '../../components/ui/Page.tsx';
import { RouteTabs } from '../../components/ui/Tabs.tsx';
import { PageColumns } from '../../components/ui/PageColumns.tsx';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { useStoredValue, valueAccess } from '../../components/ui/serverValue.ts';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import type { components } from '../../lib/api/schema.ts';
import { NotFoundContent } from '../../routes/NotFoundPage.tsx';
import { groupsKey } from '../groups/api.ts';
import { LessonList } from '../schedule/LessonList.tsx';
import { NewLessonButton, ScheduleLessonAction, useFocusHeadingWhenFilled } from '../schedule/ScheduleLessonAction.tsx';
import { CardHomeworkTab, CardOpenAssignments } from '../homework/CardHomework.tsx';
import {
  IssueAssignmentButton,
  IssuedNotice,
  homeworkTextLoaders,
  useIssuedResult,
} from '../homework/IssueAssignment.tsx';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { studentCardTextLoaders } from './cardTexts.ts';
import {
  type CardInvitation,
  type RelationshipState,
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
import { type AutosaveStatus, useAutosave } from './notesAutosave.ts';
import { StateBadge } from './StateBadge.tsx';

const cardNewLessonOpener = 'card-new-lesson';
const cardEmptyNewLessonOpener = 'card-empty-new-lesson';
const cardIssueOpener = 'card-issue';

type StudentCardPatch = components['schemas']['StudentCardPatch'];

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
  readonly onEnterEmail: () => void;
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

function useDialogAction(handlers: CardHandlers, close: () => void): ReturnType<typeof useAction> {
  return useAction((state) => {
    if (showsStaleData(state)) {
      close();
      handlers.onChangedElsewhere();
    }
  });
}

function useStaleAwareAction(onStale: () => void): ReturnType<typeof useAction> {
  return useAction((state) => {
    if (showsStaleData(state)) {
      onStale();
    }
  });
}

function DetailsDialog({
  card,
  reloads,
  handlers,
  open,
  returnFocus,
  onClose,
  onSaved,
}: {
  readonly card: StudentCard;
  readonly reloads: number;
  readonly handlers: CardHandlers;
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
  readonly onSaved: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const client = useApiClient();
  const action = useStaleAwareAction(handlers.onStale);
  const error = action.state.kind === 'refused' ? action.state.error : null;
  const emailEditable = card.state === 'invited';

  function save(patch: StudentCardPatch): void {
    action.run(async (): Promise<ActionState> => {
      const {
        data,
        error: refusal,
        response,
      } = await client.PATCH('/api/v1/students/{id}', {
        params: { path: { id: card.id } },
        body: patch,
      });
      if (data === undefined) {
        return refusedBy(response, refusal);
      }
      handlers.onChanged(data);
      onSaved();
      return { kind: 'idle' };
    });
  }

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
    <ModalDialog
      open={open}
      busy={action.busy}
      labelledBy={headingId}
      returnFocus={returnFocus}
      onClose={() => {
        action.reset();
        onClose();
      }}
    >
      <form aria-labelledby={headingId} onSubmit={submit} onChange={action.reset} className="flex flex-col gap-4">
        <h2 id={headingId} className="text-heading">
          {t('studentCard.details')}
        </h2>
        <Refusal state={action.state} dailyLimit="emailChanges" />
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
        <FormActions>
          <SubmitButton busy={action.busy}>{t('studentCard.save')}</SubmitButton>
          <ActionButton
            busy={false}
            unavailable={action.busy}
            onClick={() => {
              action.reset();
              onClose();
            }}
            className={quietButtonClass}
          >
            {t('studentCard.cancel')}
          </ActionButton>
        </FormActions>
      </form>
    </ModalDialog>
  );
}

function autosaveMessage(t: TFunction, status: AutosaveStatus): string | null {
  if (status === 'saving') {
    return t('studentCard.notesSaving');
  }
  if (status === 'saved') {
    return t('studentCard.saved');
  }
  return status === 'failed' ? t('studentCard.notesNotSaved') : null;
}

const notesStatusClass: Readonly<Record<'failed' | 'quiet', string>> = {
  failed: 'text-small text-danger',
  quiet: 'text-small text-text-muted',
};

function NotesAutosave({
  card,
  reloads,
  handlers,
}: {
  readonly card: StudentCard;
  readonly reloads: number;
  readonly handlers: CardHandlers;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const headingId = useId();
  const notesId = useId();
  const hintId = useId();
  const autosave = useAutosave(async (notes): Promise<boolean> => {
    const { data, error, response } = await client.PATCH('/api/v1/students/{id}', {
      params: { path: { id: card.id } },
      body: { notes },
    });
    if (data === undefined) {
      if (showsStaleData(refusedBy(response, error))) {
        handlers.onStale();
      }
      return false;
    }
    handlers.onChanged(data);
    return true;
  });
  const notes = useStoredValue<HTMLTextAreaElement>(autosave.unsaved ? undefined : card.notes, reloads, valueAccess);
  const [initialNotes] = useState(card.notes);
  const message = autosaveMessage(t, autosave.status);

  return (
    <form
      aria-labelledby={headingId}
      onSubmit={(event: SyntheticEvent<HTMLFormElement, SubmitEvent>) => {
        event.preventDefault();
        autosave.left();
      }}
      className="flex flex-col gap-3"
    >
      <h2 id={headingId} className="text-heading">
        {t('studentCard.notes')}
      </h2>
      <label htmlFor={notesId} className="sr-only">
        {t('studentCard.notes')}
      </label>
      <textarea
        ref={notes}
        id={notesId}
        name="notes"
        rows={8}
        maxLength={10000}
        defaultValue={initialNotes}
        aria-describedby={hintId}
        onChange={(event) => {
          autosave.changed(event.currentTarget.value);
        }}
        onBlur={autosave.left}
        className={`${fieldClass} py-2`}
      />
      <p id={hintId} className="text-small text-text-muted">
        {t('studentCard.notesHint')}
      </p>
      <div className="flex min-h-11 flex-wrap items-center gap-3">
        <p role="status" className={notesStatusClass[autosave.status === 'failed' ? 'failed' : 'quiet']}>
          {message}
        </p>
        {autosave.status === 'failed' ? (
          <button
            type="button"
            onClick={() => {
              notes.current?.focus();
              autosave.retry();
            }}
            className={quietButtonClass}
          >
            {t('studentCard.notesRetry')}
          </button>
        ) : null}
      </div>
    </form>
  );
}

function WithdrawInvitation({
  cardId,
  handlers,
  open,
  returnFocus,
  onClose,
}: {
  readonly cardId: string;
  readonly handlers: CardHandlers;
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const action = useDialogAction(handlers, onClose);
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
    <ModalDialog
      open={open}
      busy={action.busy}
      labelledBy={headingId}
      describedBy={consequenceId}
      returnFocus={returnFocus}
      initialFocus={cancel}
      onClose={onClose}
    >
      <div className="flex flex-col gap-4">
        <h2 id={headingId} className="text-heading">
          {t('studentCard.withdrawTitle')}
        </h2>
        <p id={consequenceId}>{t('studentCard.withdrawConsequence')}</p>
        <Refusal state={action.state} />
        <FormActions>
          <ActionButton busy={action.busy} onClick={withdraw} className={dangerFilledButtonClass}>
            {t('studentCard.withdrawSubmit')}
          </ActionButton>
          <ActionButton
            ref={cancel}
            busy={false}
            unavailable={action.busy}
            onClick={onClose}
            className={quietButtonClass}
          >
            {t('studentCard.cancel')}
          </ActionButton>
        </FormActions>
      </div>
    </ModalDialog>
  );
}

function EndWorkingTogether({
  consequence,
  handlers,
  open,
  returnFocus,
  onClose,
  end,
}: {
  readonly consequence: string;
  readonly handlers: CardHandlers;
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
  readonly end: () => Promise<ActionState>;
}): ReactElement {
  const { t } = useTranslation();
  const action = useDialogAction(handlers, onClose);
  const cancel = useRef<HTMLButtonElement>(null);
  const headingId = useId();
  const consequenceId = useId();

  return (
    <ModalDialog
      open={open}
      busy={action.busy}
      labelledBy={headingId}
      describedBy={consequenceId}
      returnFocus={returnFocus}
      initialFocus={cancel}
      onClose={onClose}
    >
      <div className="flex flex-col gap-4">
        <h2 id={headingId} className="text-heading">
          {t('studentCard.removeTitle')}
        </h2>
        <p id={consequenceId}>{consequence}</p>
        <Refusal state={action.state} />
        <FormActions>
          <ActionButton
            busy={action.busy}
            onClick={() => {
              action.run(end);
            }}
            className={dangerFilledButtonClass}
          >
            {t('studentCard.removeSubmit')}
          </ActionButton>
          <ActionButton
            ref={cancel}
            busy={false}
            unavailable={action.busy}
            onClick={onClose}
            className={quietButtonClass}
          >
            {t('studentCard.cancel')}
          </ActionButton>
        </FormActions>
      </div>
    </ModalDialog>
  );
}

function RemoveStudent({
  cardId,
  state,
  handlers,
  open,
  returnFocus,
  onClose,
}: {
  readonly cardId: string;
  readonly state: 'provisional' | 'reclaim_pending';
  readonly handlers: CardHandlers;
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  return (
    <EndWorkingTogether
      consequence={t(
        state === 'provisional' ? 'studentCard.removeConsequence' : 'studentCard.removeReclaimConsequence',
      )}
      handlers={handlers}
      open={open}
      returnFocus={returnFocus}
      onClose={onClose}
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
  open,
  returnFocus,
  onClose,
}: {
  readonly cardId: string;
  readonly handlers: CardHandlers;
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  return (
    <EndWorkingTogether
      consequence={t('studentCard.endConsequence')}
      handlers={handlers}
      open={open}
      returnFocus={returnFocus}
      onClose={onClose}
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
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <Avatar name={card.display_name} avatarId={card.account?.avatar_id ?? null} size="large" />
        <p className="min-w-0 text-text-muted [overflow-wrap:anywhere]">{email ?? t('students.noEmail')}</p>
        <StateBadge state={card.state} />
        <GroupDots groups={card.groups} />
      </div>
    </>
  );
}

function ErasedEmail({ onEnterEmail }: { readonly onEnterEmail: () => void }): ReactElement {
  const { t } = useTranslation();
  const labelId = useId();
  return (
    <section aria-labelledby={labelId} className="flex flex-col gap-3">
      <h2 id={labelId} className="text-heading">
        {t('invitation.link')}
      </h2>
      <p>{t('invitation.emailErased')}</p>
      <div className="flex flex-wrap gap-3">
        <button type="button" onClick={onEnterEmail} className={`${quietButtonClass} w-full tablet:w-auto`}>
          {t('invitation.enterEmail')}
        </button>
      </div>
    </section>
  );
}

function CardInvitation({
  card,
  handlers,
}: {
  readonly card: StudentCard;
  readonly handlers: CardHandlers;
}): ReactElement | null {
  const { t } = useTranslation();
  const [invitedAgain, setInvitedAgain] = useState<string | null>(null);
  const linkState = card.state === 'invited' || card.state === 'ended';
  if (card.state === 'invited' && card.email_erased) {
    return <ErasedEmail onEnterEmail={handlers.onEnterEmail} />;
  }
  if (linkState && card.invitation !== null) {
    return (
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
        onErased={handlers.onChangedElsewhere}
        newLinkTexts={{
          menu: t('studentCard.invitationActions'),
          title: t('studentCard.regenerateTitle'),
          consequence: t('studentCard.regenerateConsequence'),
          cancel: t('studentCard.cancel'),
        }}
        onDialog={handlers.onDialog}
        onChangedMeanwhile={handlers.onChangedElsewhere}
      />
    );
  }
  if (card.state === 'ended' && card.invitation === null) {
    return (
      <InviteAgain
        cardId={card.id}
        handlers={handlers}
        onInvited={(invitation) => {
          setInvitedAgain(card.id);
          handlers.onChanged({ ...card, invitation });
        }}
      />
    );
  }
  return null;
}

function CardColumns({
  card,
  reloads,
  handlers,
  children,
}: {
  readonly card: StudentCard;
  readonly reloads: number;
  readonly handlers: CardHandlers;
  readonly children?: ReactNode;
}): ReactElement {
  const { t } = useTranslation();
  return (
    <PageColumns
      asideLabel={t('studentCard.notes')}
      main={
        <>
          {card.account === null ? null : <AccountDetails account={card.account} />}
          {card.state === 'reclaim_pending' ? (
            <p className="text-text-muted">{t('studentCard.waitingForOwner')}</p>
          ) : null}
          <CardInvitation card={card} handlers={handlers} />
          {children}
        </>
      }
      aside={<NotesAutosave key={card.id} card={card} reloads={reloads} handlers={handlers} />}
    />
  );
}

type CardDialogKind = 'details' | 'danger';

interface OpenCardDialog {
  readonly kind: CardDialogKind;
  readonly state: RelationshipState;
}

function dangerousActionLabel(t: TFunction, state: RelationshipState): string | null {
  if (state === 'invited') {
    return t('studentCard.withdraw');
  }
  return state === 'ended' ? null : t('studentCard.end');
}

export function CardActions({
  card,
  reloads,
  handlers,
  detailsRequests = 0,
  onDetails,
  onSaved,
}: {
  readonly card: StudentCard;
  readonly reloads: number;
  readonly handlers: CardHandlers;
  readonly detailsRequests?: number;
  readonly onDetails: () => void;
  readonly onSaved: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const menuButton = useRef<HTMLButtonElement>(null);
  const [dialog, setDialog] = useState<OpenCardDialog | null>(null);
  const [openings, setOpenings] = useState(0);
  const [answeredRequests, setAnsweredRequests] = useState(detailsRequests);
  if (detailsRequests !== answeredRequests) {
    setAnsweredRequests(detailsRequests);
    setOpenings((count) => count + 1);
    setDialog({ kind: 'details', state: card.state });
  }
  if (dialog !== null && dialog.state !== card.state) {
    setDialog(null);
  }
  const dangerous = dangerousActionLabel(t, card.state);

  function open(kind: CardDialogKind): void {
    setOpenings((count) => count + 1);
    setDialog({ kind, state: card.state });
    handlers.onDialog(true);
    if (kind === 'details') {
      onDetails();
    }
  }

  function close(): void {
    setDialog(null);
    handlers.onDialog(false);
  }

  const detailsOpen = dialog?.kind === 'details' && dialog.state === card.state;
  const dangerOpen = dialog?.kind === 'danger' && dialog.state === card.state;

  const items: MenuItem[] = [
    {
      label: t('studentCard.editDetails'),
      onChoose: (): void => {
        open('details');
      },
    },
    ...(dangerous === null
      ? []
      : [
          {
            label: dangerous,
            danger: true,
            onChoose: (): void => {
              open('danger');
            },
          },
        ]),
  ];

  return (
    <>
      <ActionMenu label={t('studentCard.actions')} unavailable={false} items={items} buttonRef={menuButton}>
        <MoreIcon />
      </ActionMenu>
      <DetailsDialog
        card={card}
        reloads={reloads}
        handlers={handlers}
        open={detailsOpen}
        returnFocus={menuButton}
        onClose={close}
        onSaved={() => {
          close();
          onSaved();
        }}
      />
      {card.state === 'invited' ? (
        <WithdrawInvitation
          key={openings}
          cardId={card.id}
          handlers={handlers}
          open={dangerOpen}
          returnFocus={menuButton}
          onClose={close}
        />
      ) : null}
      {card.state === 'provisional' || card.state === 'reclaim_pending' ? (
        <RemoveStudent
          key={`${card.state}-${String(openings)}`}
          cardId={card.id}
          state={card.state}
          handlers={handlers}
          open={dangerOpen}
          returnFocus={menuButton}
          onClose={close}
        />
      ) : null}
      {card.state === 'active' ? (
        <EndRelationship
          key={openings}
          cardId={card.id}
          handlers={handlers}
          open={dangerOpen}
          returnFocus={menuButton}
          onClose={close}
        />
      ) : null}
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
  const [detailsRequests, setDetailsRequests] = useState(0);
  const viewHandlers: CardHandlers = {
    ...handlers,
    onEnterEmail: () => {
      handlers.onEnterEmail();
      setDetailsRequests((count) => count + 1);
    },
  };
  return (
    <>
      <CardActions
        key={card.id}
        card={card}
        reloads={reloads}
        handlers={viewHandlers}
        detailsRequests={detailsRequests}
        onDetails={() => undefined}
        onSaved={() => undefined}
      />
      <CardSummary card={card} />
      <CardColumns card={card} reloads={reloads} handlers={viewHandlers} />
    </>
  );
}

type CardTab = 'overview' | 'lessons' | 'homework';

function cardTabOf(value: string | null): CardTab {
  return value === 'lessons' || value === 'homework' ? value : 'overview';
}

function cardTabAddress(cardId: string, tab: CardTab): string {
  const address = `/students/${encodeURIComponent(cardId)}`;
  return tab === 'overview' ? address : `${address}?tab=${tab}`;
}

function cardTabLabel(t: TFunction, tab: CardTab): string {
  return tab === 'homework' ? t('homework.sections.homework') : t(`studentCard.${tab}`);
}

function CardTabs({
  cardId,
  tab,
  homework,
  homeworkPending,
}: {
  readonly cardId: string;
  readonly tab: CardTab;
  readonly homework: boolean;
  readonly homeworkPending: boolean;
}): ReactElement {
  const { t } = useTranslation();
  const keys: readonly CardTab[] = homework ? ['overview', 'lessons', 'homework'] : ['overview', 'lessons'];
  return (
    <RouteTabs
      label={t('studentCard.sections')}
      current={tab}
      busy={homeworkPending}
      tabs={keys.map((key) => ({ key, to: cardTabAddress(cardId, key), label: cardTabLabel(t, key) }))}
    />
  );
}

function NextLessons({ card }: { readonly card: StudentCardDetail }): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const empty = card.next_lessons.length === 0;
  useFocusHeadingWhenFilled(empty, heading);
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {t('lessons.next')}
      </h2>
      {empty ? (
        <div className="flex flex-col items-start gap-3">
          <p className="text-text-muted">{t('lessons.noNext')}</p>
          {me.state === 'active' && schedulable(card) ? (
            <NewLessonButton
              audience={{ kind: 'one_to_one', id: card.id, name: card.display_name }}
              opener={cardEmptyNewLessonOpener}
            >
              {t('studentCard.newLesson')}
            </NewLessonButton>
          ) : null}
        </div>
      ) : (
        <LessonList items={card.next_lessons} label={t('lessons.next')} me={me} showAudience />
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
    query.isFetching,
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
          void (query.isFetchNextPageError ? loadNextPage(query) : query.refetch());
        }}
      />
    ) : null;
  if (items.length === 0) {
    return (
      <div className="flex flex-col items-start gap-4">
        {failure}
        <p className="text-text-muted">
          {t('lessons.historyEmpty')} {t('studentCard.historyEmptyBody')}
        </p>
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
              void loadNextPage(query);
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

function CardTabContent({
  card,
  tab,
  homework,
}: {
  readonly card: StudentCardDetail;
  readonly tab: CardTab;
  readonly homework: boolean;
}): ReactElement {
  if (tab === 'lessons') {
    return <CardLessonHistory cardId={card.id} />;
  }
  if (tab === 'homework') {
    return homework ? <CardHomeworkTab relationshipId={card.id} /> : <DelayedLoading />;
  }
  return (
    <>
      <NextLessons card={card} />
      {homework ? (
        <CardOpenAssignments relationshipId={card.id} homeworkAddress={cardTabAddress(card.id, 'homework')} />
      ) : null}
    </>
  );
}

function CardWithLessons({
  card,
  tab,
  homework,
  homeworkPending,
  reloads,
  handlers,
}: {
  readonly card: StudentCardDetail;
  readonly tab: CardTab;
  readonly homework: boolean;
  readonly homeworkPending: boolean;
  readonly reloads: number;
  readonly handlers: CardHandlers;
}): ReactElement {
  return (
    <>
      <CardSummary card={card} />
      <CardColumns card={card} reloads={reloads} handlers={handlers}>
        <CardTabs cardId={card.id} tab={tab} homework={homework} homeworkPending={homeworkPending} />
        <CardTabContent card={card} tab={tab} homework={homework} />
      </CardColumns>
    </>
  );
}

interface DialogSubject {
  readonly id: string;
  readonly state: StudentCardDetail['state'];
  readonly emailErased: boolean;
}

function subjectOf(id: string, card: StudentCardDetail): DialogSubject {
  return { id, state: card.state, emailErased: card.email_erased };
}

function subjectChanged(subject: DialogSubject, card: StudentCardDetail): boolean {
  return card.state !== subject.state || card.email_erased !== subject.emailErased;
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
  const [savedCard, setSavedCard] = useState<{ readonly id: string; readonly count: number } | null>(null);
  const [detailsRequests, setDetailsRequests] = useState(0);
  if (savedCard !== null && savedCard.id !== studentId) {
    setSavedCard(null);
  }
  const me = useSignedInAccount();
  const homeworkTexts = useFeatureTexts('homework', homeworkTextLoaders);
  const cardTexts = useFeatureTexts('studentCard', studentCardTextLoaders);
  const homework = me.email_verified && homeworkTexts.ready;
  const issuedId = useIssuedResult();
  const [issued, setIssued] = useState<{ readonly id: string; readonly count: number } | null>(() =>
    issuedId === null ? null : { id: studentId, count: 1 },
  );
  if (issued !== null && issued.id !== studentId) {
    setIssued(null);
  }
  const key = studentCardKey(studentId);
  const [reloads, setReloads] = useState(0);

  function keepLessons(card: StudentCard): void {
    queryClient.setQueryData<StudentCardDetail>(key, (previous) =>
      previous === undefined
        ? previous
        : { ...card, next_lessons: previous.next_lessons, channel_id: previous.channel_id },
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
  const [dialogSubject, setDialogSubject] = useState<DialogSubject | null>(null);
  if (
    dialogSubject !== null &&
    (dialogSubject.id !== studentId || gone || (query.data !== undefined && subjectChanged(dialogSubject, query.data)))
  ) {
    setDialogSubject(null);
    if (dialogSubject.id === studentId && !gone) {
      setEndedCard(null);
      setChangedCard((previous) => ({ id: studentId, count: (previous?.count ?? 0) + 1 }));
    }
  }

  const handlers: CardHandlers = {
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
      setDialogSubject(open && query.data !== undefined ? subjectOf(studentId, query.data) : null);
    },
    onEnterEmail: () => {
      setSavedCard(null);
      setDialogSubject(query.data === undefined ? null : subjectOf(studentId, query.data));
      setDetailsRequests((count) => count + 1);
    },
    onRemoved: (newCard) => {
      void queryClient.invalidateQueries({ queryKey: studentsKey, refetchType: 'none' });
      void navigate(`/students/${encodeURIComponent(newCard.id)}`, {
        replace: true,
        state: { notice: removedNotice },
      });
    },
  };
  const shown = error === null && cardTexts.ready ? (query.data ?? null) : null;
  const noticesShown = cardTexts.ready && !notFound && !gone;

  function body(): ReactElement {
    if (!cardTexts.ready) {
      return cardTexts.failed ? (
        <LoadFailed error={asApiError(undefined)} onRetry={cardTexts.retry} />
      ) : (
        <DelayedLoading />
      );
    }
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
    if (shown === null) {
      return <DelayedLoading />;
    }
    return (
      <CardWithLessons
        card={shown}
        tab={tab}
        homework={homework}
        homeworkPending={homeworkTexts.pending}
        reloads={reloads}
        handlers={handlers}
      />
    );
  }

  return (
    <Page
      {...(notFound ? { title: 'page.notFound' as const } : {})}
      {...(shown === null ? {} : { headingText: shown.display_name })}
      focusTitle={notFound}
      action={
        shown === null ? null : (
          <>
            {shown.channel_id == null ? null : (
              <Link to={`/messages/${encodeURIComponent(shown.channel_id)}`} className={linkClass}>
                {t('section.messages')}
              </Link>
            )}
            {me.state === 'active' && schedulable(shown) ? (
              <ScheduleLessonAction
                audience={{ kind: 'one_to_one', id: shown.id, name: shown.display_name }}
                opener={cardNewLessonOpener}
                onScheduled={() => {
                  void queryClient.invalidateQueries({ queryKey: key });
                  void queryClient.invalidateQueries({ queryKey: studentsKey, refetchType: 'none' });
                }}
              />
            ) : null}
            {homework && schedulable(shown) ? (
              <IssueAssignmentButton
                opener={cardIssueOpener}
                preset={{ audience: { kind: 'one_to_one', id: shown.id, name: shown.display_name } }}
              />
            ) : null}
          </>
        )
      }
      menu={
        shown === null ? null : (
          <CardActions
            key={shown.id}
            card={shown}
            reloads={reloads}
            handlers={handlers}
            detailsRequests={detailsRequests}
            onDetails={() => {
              setSavedCard(null);
            }}
            onSaved={() => {
              setSavedCard((previous) => ({ id: studentId, count: (previous?.count ?? 0) + 1 }));
            }}
          />
        )
      }
    >
      {removedCard === null || !cardTexts.ready ? null : <LiveNotice message={t('studentCard.removed')} />}
      {issued === null || notFound || gone ? null : <IssuedNotice key={issued.count} issueId={null} />}
      {changedCard === null || !noticesShown ? null : (
        <FocusedNotice key={changedCard.count} message={t('studentCard.changedElsewhere')} />
      )}
      {endedCard === null || !noticesShown ? null : <FocusedNotice message={t('studentCard.ended')} />}
      {savedCard === null || !noticesShown ? null : (
        <FocusedNotice key={savedCard.count} message={t('studentCard.saved')} />
      )}
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
