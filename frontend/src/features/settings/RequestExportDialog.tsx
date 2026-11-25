import type { TFunction } from 'i18next';
import { type ReactElement, type RefObject, type SyntheticEvent, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useScheduler } from '../../app/clock.ts';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormActions,
  FormAlert,
  SubmitButton,
  errorMessage,
  fieldClass,
  fieldMessage,
  quietButtonClass,
  retryAfterSeconds,
  stringField,
} from '../../components/ui/Form.tsx';
import { type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import { type Me, useSignedInAccount } from '../../lib/api/session.ts';
import { formatDateTime } from '../../lib/format/dates.ts';
import type { Export } from './exportApi.ts';

type RequestOutcome =
  | { readonly kind: 'idle' }
  | { readonly kind: 'sending' }
  | { readonly kind: 'refused'; readonly error: ApiError; readonly retryAfter: number | null; readonly at: number };

const longWaitSeconds = 3_600;
const passwordField = 'password';

function refusalText(
  t: TFunction,
  me: Me,
  outcome: Extract<RequestOutcome, { readonly kind: 'refused' }>,
): string | null {
  if (outcome.error.fieldCode(passwordField) !== undefined) {
    return null;
  }
  if (outcome.error.code === 'service_unavailable') {
    return t('myData.busy');
  }
  if (outcome.error.code === 'rate_limited' && outcome.retryAfter !== null && outcome.retryAfter >= longWaitSeconds) {
    const time = new Date(outcome.at + outcome.retryAfter * 1_000).toISOString();
    return t('myData.requestAfter', { time: formatDateTime(time, me.language, me.timezone) });
  }
  return errorMessage(t, outcome.error, outcome.retryAfter);
}

export function RequestExportForm({
  headingId,
  bodyId,
  password,
  onSending,
  onClose,
  onRequested,
  onInProgress,
  onLimited,
}: {
  readonly headingId: string;
  readonly bodyId: string;
  readonly password: RefObject<HTMLInputElement | null>;
  readonly onSending: (sending: boolean) => void;
  readonly onClose: () => void;
  readonly onRequested: (requested: Export) => void;
  readonly onInProgress: () => void;
  readonly onLimited: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const scheduler = useScheduler();
  const me = useSignedInAccount();
  const inputId = useId();
  const errorId = useId();
  const [outcome, setOutcome] = useState<RequestOutcome>({ kind: 'idle' });
  const sending = outcome.kind === 'sending';
  const error = outcome.kind === 'refused' ? outcome.error : null;
  const passwordError = fieldMessage(t, error, passwordField);
  const alert = outcome.kind === 'refused' ? refusalText(t, me, outcome) : null;

  useEffect(() => {
    if (passwordError !== undefined) {
      password.current?.focus();
    }
  }, [outcome, passwordError, password]);

  function settle(next: RequestOutcome): void {
    setOutcome(next);
    onSending(next.kind === 'sending');
  }

  async function request(data: FormData): Promise<void> {
    settle({ kind: 'sending' });
    const {
      data: requested,
      error: refusal,
      response,
    } = await client.POST('/api/v1/exports', { body: { password: stringField(data, passwordField) } });
    if (requested !== undefined) {
      settle({ kind: 'idle' });
      onRequested(requested);
      return;
    }
    const refused = apiErrorFrom(response.status, refusal);
    if (refused.code === 'export_in_progress') {
      settle({ kind: 'idle' });
      onInProgress();
      return;
    }
    settle({ kind: 'refused', error: refused, retryAfter: retryAfterSeconds(response), at: scheduler.wallClock() });
    if (refused.code === 'rate_limited') {
      onLimited();
    }
  }

  return (
    <form
      aria-labelledby={headingId}
      onSubmit={(event: SyntheticEvent<HTMLFormElement, SubmitEvent>) => {
        event.preventDefault();
        request(new FormData(event.currentTarget)).catch((thrown: unknown) => {
          console.error('export_request_failed', thrown instanceof Error ? thrown.name : typeof thrown);
          settle({ kind: 'refused', error: asApiError(thrown), retryAfter: null, at: scheduler.wallClock() });
        });
      }}
      className="flex flex-col gap-4"
    >
      <h2 id={headingId} className="text-heading">
        {t('myData.dialogTitle')}
      </h2>
      <p id={bodyId}>{t('myData.dialogBody')}</p>
      {alert === null ? null : <FormAlert>{alert}</FormAlert>}
      <div className="flex flex-col gap-1">
        <label htmlFor={inputId} className="text-label">
          {t('settings.currentPassword')}
        </label>
        <input
          ref={password}
          id={inputId}
          name={passwordField}
          type="password"
          autoComplete="current-password"
          required
          aria-invalid={passwordError === undefined ? undefined : true}
          aria-describedby={passwordError === undefined ? undefined : errorId}
          className={fieldClass}
        />
        {passwordError === undefined ? null : (
          <p id={errorId} className="text-small text-danger">
            {passwordError}
          </p>
        )}
      </div>
      <FormActions>
        <SubmitButton busy={sending}>{t('myData.submit')}</SubmitButton>
        <ActionButton busy={false} unavailable={sending} onClick={onClose} className={quietButtonClass}>
          {t('settings.cancel')}
        </ActionButton>
      </FormActions>
    </form>
  );
}

export function RequestExportDialog({
  open,
  returnFocus,
  onClose,
  onRequested,
  onInProgress,
  onLimited,
}: {
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
  readonly onRequested: (requested: Export) => void;
  readonly onInProgress: () => void;
  readonly onLimited: () => void;
}): ReactElement {
  const headingId = useId();
  const bodyId = useId();
  const password = useRef<HTMLInputElement>(null);
  const [sending, setSending] = useState(false);

  return (
    <ModalDialog
      open={open}
      busy={sending}
      labelledBy={headingId}
      describedBy={bodyId}
      returnFocus={returnFocus}
      initialFocus={password}
      onClose={() => {
        setSending(false);
        onClose();
      }}
    >
      <RequestExportForm
        headingId={headingId}
        bodyId={bodyId}
        password={password}
        onSending={setSending}
        onClose={onClose}
        onRequested={onRequested}
        onInProgress={onInProgress}
        onLimited={onLimited}
      />
    </ModalDialog>
  );
}
