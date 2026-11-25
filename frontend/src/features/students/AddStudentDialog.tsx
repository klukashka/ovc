import { useQueryClient } from '@tanstack/react-query';
import { type ReactElement, type RefObject, type SyntheticEvent, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { type Action, type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormActions,
  SubmitButton,
  TextField,
  fieldMessage,
  linkClass,
  primaryButtonClass,
  quietButtonClass,
  stringField,
} from '../../components/ui/Form.tsx';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { useApiClient } from '../../lib/api/client.ts';
import { type StudentCard, studentsKey } from './api.ts';
import { InvitationLink } from './InvitationLink.tsx';

function ExistingStudent({ state }: { readonly state: ActionState }): ReactElement | null {
  const { t } = useTranslation();
  const existing = state.kind === 'refused' ? state.error.existingStudentId : undefined;
  if (state.kind !== 'refused' || state.error.code !== 'student_exists' || existing === undefined) {
    return null;
  }
  return (
    <Link to={`/students/${encodeURIComponent(existing)}`} className={linkClass}>
      {t('addStudent.openExisting')}
    </Link>
  );
}

function NewCardForm({
  headingId,
  action,
  onCreated,
  onCancel,
}: {
  readonly headingId: string;
  readonly action: Action;
  readonly onCreated: (card: StudentCard) => void;
  readonly onCancel: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const [idempotencyKey] = useState(() => crypto.randomUUID());
  const error = action.state.kind === 'refused' ? action.state.error : null;

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const displayName = stringField(data, 'display_name');
    const email = stringField(data, 'email').trim();
    action.run(async (): Promise<ActionState> => {
      const {
        data: card,
        error: refusal,
        response,
      } = await client.POST('/api/v1/students', {
        params: { header: { 'Idempotency-Key': idempotencyKey } },
        body: email === '' ? { display_name: displayName } : { display_name: displayName, email },
      });
      if (card === undefined) {
        return refusedBy(response, refusal);
      }
      onCreated(card);
      return done;
    });
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <h2 id={headingId} className="font-serif text-title">
        {t('addStudent.title')}
      </h2>
      <Refusal state={action.state} dailyLimit="cards" confirmation="invite" />
      <ExistingStudent state={action.state} />
      <TextField
        label={t('addStudent.displayName')}
        name="display_name"
        autoComplete="off"
        maxLength={100}
        hint={t('addStudent.displayNameHint')}
        error={fieldMessage(t, error, 'display_name')}
      />
      <TextField
        label={t('addStudent.email')}
        name="email"
        type="email"
        autoComplete="off"
        required={false}
        maxLength={254}
        hint={t('addStudent.emailHint')}
        error={fieldMessage(t, error, 'email')}
      />
      <FormActions>
        <SubmitButton busy={action.busy}>{t('addStudent.submit')}</SubmitButton>
        <ActionButton busy={false} unavailable={action.busy} onClick={onCancel} className={quietButtonClass}>
          {t('addStudent.cancel')}
        </ActionButton>
      </FormActions>
    </form>
  );
}

function CreatedCard({
  headingId,
  card,
  onDone,
}: {
  readonly headingId: string;
  readonly card: StudentCard;
  readonly onDone: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const heading = useRef<HTMLHeadingElement>(null);

  useEffect(() => {
    heading.current?.focus();
  }, []);

  return (
    <div className="flex flex-col gap-4">
      <h2 ref={heading} id={headingId} tabIndex={-1} className="font-serif text-title outline-none">
        {t('addStudent.createdTitle')}
      </h2>
      <p className="text-strong">{card.display_name}</p>
      <p className="text-text-muted">
        {card.email === null ? t('addStudent.createdNoEmail') : t('addStudent.created')}
      </p>
      {card.invitation === null ? null : (
        <InvitationLink
          cardId={card.id}
          invitation={card.invitation}
          canSend={card.email !== null}
          onSent={() => undefined}
        />
      )}
      <div className="flex flex-wrap items-center gap-3">
        <button type="button" onClick={onDone} className={primaryButtonClass}>
          {t('addStudent.done')}
        </button>
        <Link to={`/students/${encodeURIComponent(card.id)}`} className={linkClass}>
          {t('addStudent.openCard')}
        </Link>
      </div>
    </div>
  );
}

export function AddStudentDialog({
  open,
  returnFocus,
  onClose,
}: {
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
}): ReactElement {
  const queryClient = useQueryClient();
  const headingId = useId();
  const action = useAction();
  const [created, setCreated] = useState<StudentCard | null>(null);

  function close(): void {
    setCreated(null);
    action.reset();
    onClose();
  }

  return (
    <ModalDialog open={open} busy={action.busy} labelledBy={headingId} returnFocus={returnFocus} onClose={close}>
      {created === null ? (
        <NewCardForm
          headingId={headingId}
          action={action}
          onCancel={close}
          onCreated={(card) => {
            setCreated(card);
            void queryClient.invalidateQueries({ queryKey: studentsKey });
          }}
        />
      ) : (
        <CreatedCard headingId={headingId} card={created} onDone={close} />
      )}
    </ModalDialog>
  );
}
