import { useQuery, useQueryClient } from '@tanstack/react-query';
import { type ReactElement, type SyntheticEvent, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate, useParams } from 'react-router';

import { type ActionState, done, refusedBy, showsStaleData, useAction } from '../../components/ui/action.ts';
import { Avatar } from '../../components/ui/Avatar.tsx';
import { ConfirmationPrompt } from '../../components/ui/ConfirmationPrompt.tsx';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  FormNotice,
  SubmitButton,
  TextField,
  fieldClass,
  fieldMessage,
  linkClass,
  stringField,
} from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import { NotFoundContent } from '../../routes/NotFoundPage.tsx';
import { type StudentCard, fetchStudentCard, studentCardKey, studentsKey, withdrawnNotice } from './api.ts';
import { GroupDots } from './GroupDots.tsx';
import { InvitationLink } from './InvitationLink.tsx';
import { StateBadge } from './StateBadge.tsx';

type StudentCardPatch = components['schemas']['StudentCardPatch'];

const dangerButtonClass =
  'inline-flex min-h-11 items-center justify-center rounded-md border border-danger px-4 text-strong text-danger hover:bg-surface disabled:opacity-60';
const textButtonClass =
  'inline-flex min-h-11 items-center px-3 text-small text-accent-text underline disabled:opacity-60';

export interface CardHandlers {
  readonly onChanged: (card: StudentCard) => void;
  readonly onStale: () => void;
  readonly onWithdrawn: () => void;
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
  handlers,
}: {
  readonly card: StudentCard;
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

function NotesForm({ card, handlers }: { readonly card: StudentCard; readonly handlers: CardHandlers }): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const notesId = useId();
  const hintId = useId();
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
        id={notesId}
        name="notes"
        rows={6}
        maxLength={10000}
        defaultValue={card.notes}
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
  const action = useStaleAwareAction(handlers.onStale);
  const opener = useRef<HTMLButtonElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  const headingId = useId();
  const consequenceId = useId();
  const [open, setOpen] = useState(false);

  function withdraw(): void {
    action.run(async (): Promise<ActionState> => {
      const { error, response } = await client.DELETE('/api/v1/students/{id}', { params: { path: { id: cardId } } });
      if (!response.ok) {
        return refusedBy(response, error);
      }
      handlers.onWithdrawn();
      return done;
    });
  }

  return (
    <section className="flex flex-col items-start gap-3">
      <button
        ref={opener}
        type="button"
        onClick={() => {
          action.reset();
          setOpen(true);
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
          setOpen(false);
        }}
      >
        <div className="flex flex-col gap-4">
          <h2 id={headingId} className="text-heading">
            {t('studentCard.withdrawTitle')}
          </h2>
          <p id={consequenceId}>{t('studentCard.withdrawConsequence')}</p>
          <Refusal state={action.state} />
          <div className="flex flex-wrap gap-3">
            <button
              type="button"
              disabled={action.busy}
              aria-busy={action.busy}
              onClick={withdraw}
              className={dangerButtonClass}
            >
              {t('studentCard.withdrawSubmit')}
            </button>
            <button
              ref={cancel}
              type="button"
              disabled={action.busy}
              onClick={() => {
                setOpen(false);
              }}
              className={textButtonClass}
            >
              {t('studentCard.cancel')}
            </button>
          </div>
        </div>
      </ModalDialog>
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

export function StudentCardView({
  card,
  handlers,
}: {
  readonly card: StudentCard;
  readonly handlers: CardHandlers;
}): ReactElement {
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
      {card.state === 'invited' && card.invitation !== null ? (
        <InvitationLink
          cardId={card.id}
          invitation={card.invitation}
          canSend={card.email !== null}
          onRegenerated={(invitation) => {
            handlers.onChanged({ ...card, invitation });
          }}
          onSent={handlers.onStale}
          onStale={handlers.onStale}
        />
      ) : null}
      <DetailsForm card={card} handlers={handlers} />
      <NotesForm card={card} handlers={handlers} />
      {card.state === 'invited' ? <WithdrawInvitation cardId={card.id} handlers={handlers} /> : null}
    </>
  );
}

export function StudentCardPage(): ReactElement {
  const { studentId = '' } = useParams();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const key = studentCardKey(studentId);
  const query = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => fetchStudentCard(client, studentId, signal),
    gcTime: 0,
    retry: false,
  });

  const error = query.isError ? asApiError(query.error) : null;
  const notFound = error?.code === 'not_found';

  function body(): ReactElement {
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
    return (
      <StudentCardView
        card={query.data}
        handlers={{
          onChanged: (card) => {
            queryClient.setQueryData(key, card);
            void queryClient.invalidateQueries({ queryKey: studentsKey, refetchType: 'none' });
          },
          onStale: () => {
            void queryClient.invalidateQueries({ queryKey: key });
          },
          onWithdrawn: () => {
            void navigate('/students?state=invited', { state: { notice: withdrawnNotice } });
            void queryClient.invalidateQueries({ queryKey: studentsKey, refetchType: 'none' });
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
      {body()}
    </Page>
  );
}
