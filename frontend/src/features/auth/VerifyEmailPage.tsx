import { type ReactElement, type SyntheticEvent, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, Navigate } from 'react-router';

import {
  FormAlert,
  FormNotice,
  SubmitButton,
  TextField,
  errorMessage,
  fieldMessage,
  linkClass,
  primaryButtonClass,
  stringField,
} from '../../components/ui/Form.tsx';
import { LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import { type SessionState, useSession, useSessionState } from '../../lib/api/session.ts';
import { reclaimAddress } from '../../routes/access.ts';
import { CheckEmail } from './CheckEmail.tsx';
import { useAddressToken } from './AddressToken.tsx';

type EmailConfirmed = components['schemas']['EmailConfirmed'];

type Confirmation =
  | { readonly kind: 'waiting' }
  | { readonly kind: 'confirming' }
  | { readonly kind: 'needsPassword'; readonly error: ApiError | null }
  | { readonly kind: 'confirmed'; readonly outcome: EmailConfirmed['outcome'] }
  | { readonly kind: 'invalid' }
  | { readonly kind: 'signOutFailed' }
  | { readonly kind: 'failed'; readonly error: ApiError };

function settled(state: SessionState): boolean {
  return state.kind === 'signedIn' || state.kind === 'signedOut';
}

function confirmationOf(refusal: ApiError, sentPassword: boolean): Confirmation {
  if (refusal.code === 'token_invalid' || refusal.fieldCode('token') !== undefined) {
    return { kind: 'invalid' };
  }
  if (refusal.code === 'validation_failed' && refusal.fieldCode('new_password') !== undefined) {
    return { kind: 'needsPassword', error: sentPassword ? refusal : null };
  }
  return { kind: 'failed', error: refusal };
}

function NoToken({ state }: { readonly state: SessionState }): ReactElement {
  const { t } = useTranslation();
  if (state.kind === 'signedIn' && state.me.state === 'unverified') {
    return (
      <Page width="reading" title="page.checkEmail">
        <CheckEmail email={state.me.email} audience={{ kind: 'signedIn' }} />
      </Page>
    );
  }
  if (state.kind === 'signedIn') {
    return (
      <Page width="reading">
        <p>{t('verifyEmail.alreadyConfirmed')}</p>
        <p>
          <Link to="/" className={linkClass}>
            {t('verifyEmail.continue')}
          </Link>
        </p>
      </Page>
    );
  }
  return (
    <Page width="reading">
      <p>{t('verifyEmail.openLinkAgain')}</p>
      <p>
        <Link to="/" className={linkClass}>
          {t('verifyEmail.signIn')}
        </Link>
      </p>
    </Page>
  );
}

export function VerifyEmailPage(): ReactElement {
  const { t } = useTranslation();
  const token = useAddressToken();
  const client = useApiClient();
  const session = useSession();
  const sessionState = useSessionState();
  const [confirmation, setConfirmation] = useState<Confirmation>({ kind: 'waiting' });
  const sent = useRef(false);

  async function confirm(value: string, newPassword: string | null): Promise<void> {
    setConfirmation({ kind: 'confirming' });
    if (newPassword !== null && session.snapshot().kind === 'signedIn') {
      const signedOut = await session.signOut().then(
        () => true,
        () => false,
      );
      if (!signedOut) {
        setConfirmation({ kind: 'signOutFailed' });
        return;
      }
    }
    const body = newPassword === null ? { token: value } : { token: value, new_password: newPassword };
    const { data, error, response } = await client.POST('/api/v1/auth/verify-email', { body });
    if (data === undefined) {
      setConfirmation(confirmationOf(apiErrorFrom(response.status, error), newPassword !== null));
      return;
    }
    if (data.access_token === undefined) {
      await session.reloadAccount();
    } else {
      await session.adopt(data);
    }
    setConfirmation({ kind: 'confirmed', outcome: data.outcome });
  }

  function attempt(value: string, newPassword: string | null): void {
    confirm(value, newPassword).catch((thrown: unknown) => {
      setConfirmation({ kind: 'failed', error: asApiError(thrown) });
    });
  }

  const ready = settled(sessionState);
  useEffect(() => {
    if (token === null || !ready || sent.current) {
      return;
    }
    sent.current = true;
    attempt(token, null);
  });

  if (token === null) {
    return <NoToken state={sessionState} />;
  }

  switch (confirmation.kind) {
    case 'waiting':
    case 'confirming':
      return (
        <Page width="reading">
          <p role="status" className="text-text-muted">
            {t('verifyEmail.confirming')}
          </p>
        </Page>
      );
    case 'confirmed':
      if (confirmation.outcome === null) {
        return <Navigate to="/" replace />;
      }
      if (confirmation.outcome === 'choice_required') {
        return <Navigate to={reclaimAddress} replace />;
      }
      return (
        <Page width="reading">
          <FormNotice takeFocus>{t(`outcome.${confirmation.outcome}`)}</FormNotice>
          <p>
            <Link to="/" className={linkClass}>
              {t('verifyEmail.continue')}
            </Link>
          </p>
        </Page>
      );
    case 'invalid':
      return (
        <Page width="reading">
          <FormAlert>{t('errors.token_invalid')}</FormAlert>
          <p>
            <Link to="/" className={linkClass}>
              {t('verifyEmail.signIn')}
            </Link>
          </p>
        </Page>
      );
    case 'signOutFailed':
    case 'failed':
      return (
        <Page width="reading">
          <FormAlert>
            {confirmation.kind === 'failed' ? errorMessage(t, confirmation.error, null) : t('frame.signOutFailed')}
          </FormAlert>
          <p>
            <button
              type="button"
              onClick={() => {
                attempt(token, null);
              }}
              className={primaryButtonClass}
            >
              {t('verifyEmail.retry')}
            </button>
          </p>
        </Page>
      );
    case 'needsPassword': {
      const passwordError = fieldMessage(t, confirmation.error, 'new_password');
      return (
        <Page width="reading">
          <form
            onSubmit={(event: SyntheticEvent<HTMLFormElement, SubmitEvent>) => {
              event.preventDefault();
              attempt(token, stringField(new FormData(event.currentTarget), 'new_password'));
            }}
            className="flex max-w-sm flex-col gap-4"
          >
            {passwordError === undefined ? null : <FormAlert>{t('errors.validation_failed')}</FormAlert>}
            <LiveNotice
              message={
                sessionState.kind === 'signedIn'
                  ? t('verifyEmail.switchAccount', { email: sessionState.me.email })
                  : null
              }
            />
            <p>{t('verifyEmail.choosePassword')}</p>
            <TextField
              label={t('verifyEmail.newPassword')}
              name="new_password"
              type="password"
              autoComplete="new-password"
              minLength={10}
              maxLength={128}
              hint={t('signUp.passwordHint')}
              error={passwordError}
            />
            <SubmitButton busy={false}>{t('verifyEmail.submit')}</SubmitButton>
          </form>
        </Page>
      );
    }
  }
}
