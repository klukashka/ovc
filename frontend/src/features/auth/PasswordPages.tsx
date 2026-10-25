import { type ReactElement, type SyntheticEvent, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import {
  FormAlert,
  FormNotice,
  SubmitButton,
  TextField,
  errorMessage,
  fieldMessage,
  linkClass,
  retryAfterSeconds,
  stringField,
} from '../../components/ui/Form.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSession } from '../../lib/api/session.ts';
import { useAddressToken } from './AddressToken.tsx';

type Request =
  | { readonly kind: 'editing'; readonly error: ApiError | null; readonly retryAfter: number | null }
  | { readonly kind: 'sending' }
  | { readonly kind: 'done'; readonly email: string };

export function ForgotPasswordPage(): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const [request, setRequest] = useState<Request>({ kind: 'editing', error: null, retryAfter: null });

  async function submit(email: string): Promise<void> {
    setRequest({ kind: 'sending' });
    const { error, response } = await client.POST('/api/v1/auth/password/forgot', { body: { email } });
    if (response.ok) {
      setRequest({ kind: 'done', email });
      return;
    }
    setRequest({
      kind: 'editing',
      error: apiErrorFrom(response.status, error),
      retryAfter: retryAfterSeconds(response),
    });
  }

  if (request.kind === 'done') {
    return (
      <Page>
        <FormNotice takeFocus>{t('forgotPassword.sent', { email: request.email })}</FormNotice>
        <p>
          <Link to="/" className={linkClass}>
            {t('forgotPassword.backToSignIn')}
          </Link>
        </p>
      </Page>
    );
  }

  const error = request.kind === 'editing' ? request.error : null;
  return (
    <Page>
      <form
        onSubmit={(event: SyntheticEvent<HTMLFormElement, SubmitEvent>) => {
          event.preventDefault();
          submit(stringField(new FormData(event.currentTarget), 'email')).catch((thrown: unknown) => {
            setRequest({ kind: 'editing', error: asApiError(thrown), retryAfter: null });
          });
        }}
        className="flex max-w-sm flex-col gap-4"
      >
        <p>{t('forgotPassword.body')}</p>
        {error === null ? null : (
          <FormAlert>{errorMessage(t, error, request.kind === 'editing' ? request.retryAfter : null)}</FormAlert>
        )}
        <TextField
          label={t('signIn.email')}
          name="email"
          type="email"
          autoComplete="email"
          error={fieldMessage(t, error, 'email')}
        />
        <SubmitButton busy={request.kind === 'sending'}>{t('forgotPassword.submit')}</SubmitButton>
        <p>
          <Link to="/" className={linkClass}>
            {t('forgotPassword.backToSignIn')}
          </Link>
        </p>
      </form>
    </Page>
  );
}

type Reset =
  | { readonly kind: 'editing'; readonly error: ApiError | null }
  | { readonly kind: 'sending' }
  | { readonly kind: 'done'; readonly choiceRequired: boolean }
  | { readonly kind: 'invalid' };

export function ResetPasswordPage(): ReactElement {
  const { t } = useTranslation();
  const token = useAddressToken();
  const client = useApiClient();
  const session = useSession();
  const [reset, setReset] = useState<Reset>({ kind: 'editing', error: null });

  async function submit(value: string, newPassword: string): Promise<void> {
    setReset({ kind: 'sending' });
    const { data, error, response } = await client.POST('/api/v1/auth/password/reset', {
      body: { token: value, new_password: newPassword },
    });
    if (data !== undefined) {
      if (session.snapshot().kind === 'signedIn') {
        await session.reloadAccount();
      }
      setReset({ kind: 'done', choiceRequired: data.outcome === 'choice_required' });
      return;
    }
    const refusal = apiErrorFrom(response.status, error);
    setReset(
      refusal.code === 'token_invalid' || refusal.fieldCode('token') !== undefined
        ? { kind: 'invalid' }
        : { kind: 'editing', error: refusal },
    );
  }

  if (token === null) {
    return (
      <Page>
        <p>{t('resetPassword.openLinkAgain')}</p>
        <p>
          <Link to="/forgot-password" className={linkClass}>
            {t('resetPassword.requestAgain')}
          </Link>
        </p>
      </Page>
    );
  }
  if (reset.kind === 'done' || reset.kind === 'invalid') {
    return (
      <Page>
        {reset.kind === 'done' ? (
          <FormNotice takeFocus>{t('resetPassword.done')}</FormNotice>
        ) : (
          <FormAlert>{t('errors.token_invalid')}</FormAlert>
        )}
        {reset.kind === 'done' && reset.choiceRequired ? <p>{t('resetPassword.choiceAfterSignIn')}</p> : null}
        <p>
          <Link to={reset.kind === 'done' ? '/' : '/forgot-password'} className={linkClass}>
            {reset.kind === 'done' ? t('resetPassword.signIn') : t('resetPassword.requestAgain')}
          </Link>
        </p>
      </Page>
    );
  }

  const error = reset.kind === 'editing' ? reset.error : null;
  const passwordError = fieldMessage(t, error, 'new_password');
  return (
    <Page>
      <form
        onSubmit={(event: SyntheticEvent<HTMLFormElement, SubmitEvent>) => {
          event.preventDefault();
          submit(token, stringField(new FormData(event.currentTarget), 'new_password')).catch((thrown: unknown) => {
            setReset({ kind: 'editing', error: asApiError(thrown) });
          });
        }}
        className="flex max-w-sm flex-col gap-4"
      >
        {error !== null && passwordError === undefined ? <FormAlert>{errorMessage(t, error, null)}</FormAlert> : null}
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
        <SubmitButton busy={reset.kind === 'sending'}>{t('resetPassword.submit')}</SubmitButton>
      </form>
    </Page>
  );
}

export function PrivacyPage(): ReactElement {
  const { t } = useTranslation();
  return (
    <Page>
      <p className="text-text-muted">{t('privacy.placeholder')}</p>
    </Page>
  );
}
