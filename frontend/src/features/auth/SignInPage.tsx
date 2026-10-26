import { type ReactElement, type SyntheticEvent, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { ActionButton } from '../../components/ui/Form.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { type ApiError, asApiError } from '../../lib/api/client.ts';
import { type SignInResult, useSession, useSessionState } from '../../lib/api/session.ts';

export type SignInRefusal =
  | { readonly kind: 'credentials' }
  | { readonly kind: 'blocked' }
  | { readonly kind: 'throttled'; readonly minutes: number }
  | { readonly kind: 'failed'; readonly error: ApiError | null };

const fieldClass =
  'min-h-11 w-full rounded-md border border-border-strong bg-field px-3 text-body-phone text-text tablet:text-body';
const linkClass = 'inline-flex min-h-11 items-center text-accent-text underline';

export function refusalOf(result: Extract<SignInResult, { kind: 'refused' }>): SignInRefusal {
  switch (result.error.code) {
    case 'invalid_credentials':
    case 'validation_failed':
      return { kind: 'credentials' };
    case 'account_blocked':
      return { kind: 'blocked' };
    case 'rate_limited':
      return result.retryAfterSeconds === null
        ? { kind: 'failed', error: result.error }
        : { kind: 'throttled', minutes: Math.max(1, Math.ceil(result.retryAfterSeconds / 60)) };
    default:
      return { kind: 'failed', error: result.error };
  }
}

export function RefusalMessage({
  id,
  refusal,
}: {
  readonly id: string;
  readonly refusal: SignInRefusal;
}): ReactElement {
  const { t } = useTranslation();
  const element = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    element.current?.focus();
  }, [refusal]);

  let message: string;
  switch (refusal.kind) {
    case 'credentials':
      message = t('signIn.wrongCredentials');
      break;
    case 'blocked':
      message = t('signIn.blocked');
      break;
    case 'throttled':
      message = t('signIn.throttled', { minutes: refusal.minutes });
      break;
    case 'failed':
      message =
        refusal.error?.code === undefined
          ? t('signIn.failed', { requestId: refusal.error?.requestId ?? '—' })
          : t(`errors.${refusal.error.code}`);
      break;
  }
  return (
    <p ref={element} id={id} role="alert" tabIndex={-1} className="text-danger outline-none">
      {message}
    </p>
  );
}

export function SignInPage(): ReactElement {
  const { t } = useTranslation();
  const session = useSession();
  const state = useSessionState();
  const endedNotice =
    state.kind === 'signedOut' && state.reason === 'accountDeleted'
      ? 'signIn.accountDeleted'
      : state.kind === 'signedOut' && state.reason === 'sessionEnded'
        ? 'signIn.sessionEnded'
        : null;
  const emailId = useId();
  const passwordId = useId();
  const showId = useId();
  const refusalId = useId();
  const [showPassword, setShowPassword] = useState(false);
  const [busy, setBusy] = useState(false);
  const [refusal, setRefusal] = useState<SignInRefusal | null>(
    state.kind === 'signedOut' && state.reason === 'blocked' ? { kind: 'blocked' } : null,
  );

  async function submit(form: HTMLFormElement): Promise<void> {
    const data = new FormData(form);
    const email = data.get('email');
    const password = data.get('password');
    if (typeof email !== 'string' || typeof password !== 'string') {
      throw new Error('the sign-in form is missing its fields');
    }
    setBusy(true);
    setRefusal(null);
    try {
      const result = await session.signIn(email, password);
      if (result.kind === 'refused') {
        setRefusal(refusalOf(result));
      }
    } catch (thrown: unknown) {
      setRefusal({ kind: 'failed', error: asApiError(thrown) });
    } finally {
      setBusy(false);
    }
  }

  function onSubmit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    void submit(event.currentTarget);
  }

  return (
    <Page>
      {endedNotice === null ? null : (
        <p role="status" className="text-text">
          {t(endedNotice)}
        </p>
      )}
      <form onSubmit={onSubmit} className="flex max-w-sm flex-col gap-4">
        {refusal === null ? null : <RefusalMessage id={refusalId} refusal={refusal} />}
        <div className="flex flex-col gap-1">
          <label htmlFor={emailId} className="text-label">
            {t('signIn.email')}
          </label>
          <input
            id={emailId}
            aria-describedby={refusal === null ? undefined : refusalId}
            name="email"
            type="email"
            autoComplete="username"
            required
            className={fieldClass}
          />
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor={passwordId} className="text-label">
            {t('signIn.password')}
          </label>
          <input
            id={passwordId}
            name="password"
            aria-describedby={refusal === null ? undefined : refusalId}
            type={showPassword ? 'text' : 'password'}
            autoComplete="current-password"
            required
            className={fieldClass}
          />
          <label htmlFor={showId} className="inline-flex min-h-11 items-center gap-2 text-small">
            <input
              id={showId}
              type="checkbox"
              checked={showPassword}
              onChange={(event) => {
                setShowPassword(event.currentTarget.checked);
              }}
              className="size-4"
            />
            {t('signIn.showPassword')}
          </label>
        </div>
        <ActionButton
          type="submit"
          busy={busy}
          className="inline-flex min-h-11 items-center justify-center rounded-md bg-accent px-4 text-strong text-on-accent hover:bg-accent-hover aria-disabled:opacity-60"
        >
          {t('signIn.submit')}
        </ActionButton>
        <p className="flex flex-wrap gap-x-4">
          <Link to="/forgot-password" className={linkClass}>
            {t('signIn.forgotPassword')}
          </Link>
          <Link to="/signup" className={linkClass}>
            {t('signIn.signUp')}
          </Link>
        </p>
      </form>
    </Page>
  );
}
