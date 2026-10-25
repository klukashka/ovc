import { useQueryClient } from '@tanstack/react-query';
import { type ReactElement, type RefObject, type SyntheticEvent, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router';

import { type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import { Avatar } from '../../components/ui/Avatar.tsx';
import {
  ActionButton,
  FormAlert,
  FormNotice,
  SubmitButton,
  TextField,
  errorMessage,
  fieldClass,
  fieldMessage,
  linkClass,
  primaryButtonClass,
  quietButtonClass,
  stringField,
} from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import { type Me, useSession, useSessionState } from '../../lib/api/session.ts';
import { isLanguage } from '../../lib/i18n/resources.ts';
import { reclaimAddress } from '../../routes/access.ts';
import { myTutorsKey } from '../tutors/api.ts';
import { CheckEmail } from './CheckEmail.tsx';
import { useInvitationToken } from './InvitationToken.tsx';
import { type SignInRefusal, RefusalMessage, refusalOf } from './SignInPage.tsx';
import { ConsentField, LanguageChoice, browserTimezone } from './SignUpPages.tsx';

type PublicTutor = components['schemas']['PublicTutor'];

type Invitation =
  | { readonly kind: 'loading' }
  | { readonly kind: 'ready'; readonly tutor: PublicTutor }
  | { readonly kind: 'invalid' }
  | { readonly kind: 'failed'; readonly error: ApiError };

type Mode = 'choosing' | 'signUp' | 'signIn';

const shownSignUpFields = ['name', 'email', 'password', 'consent'];
const textButtonClass =
  'inline-flex min-h-11 items-center text-small text-accent-text underline aria-disabled:opacity-60 aria-disabled:no-underline';

function useFocusWhenLost(target: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    const element = target.current;
    const focused = document.activeElement;
    if (element !== null && (focused === null || !focused.isConnected || focused.contains(element))) {
      element.focus();
    }
  }, [target]);
}

function TutorProfile({ tutor }: { readonly tutor: PublicTutor }): ReactElement {
  const { t } = useTranslation();
  return (
    <section className="flex flex-col gap-4">
      <div className="flex items-center gap-4">
        <Avatar name={tutor.name} avatarId={tutor.avatar_id} size="large" />
        <div className="flex min-w-0 flex-col gap-1">
          <h2 className="text-heading">{tutor.name}</h2>
          {tutor.subjects.length === 0 ? null : (
            <p className="text-small text-text-muted">{tutor.subjects.join(', ')}</p>
          )}
        </div>
      </div>
      <p>{t('invitationPage.invites', { name: tutor.name })}</p>
    </section>
  );
}

function SignOutInPlace({ label }: { readonly label: string }): ReactElement {
  const { t } = useTranslation();
  const session = useSession();
  const [state, setState] = useState<'idle' | 'busy' | 'failed'>('idle');
  return (
    <>
      <ActionButton
        busy={state === 'busy'}
        onClick={() => {
          setState('busy');
          session.signOut().catch(() => {
            setState('failed');
          });
        }}
        className={textButtonClass}
      >
        {label}
      </ActionButton>
      {state === 'failed' ? <FormAlert>{t('frame.signOutFailed')}</FormAlert> : null}
    </>
  );
}

function signUpError(
  t: ReturnType<typeof useTranslation>['t'],
  state: ActionState,
): { readonly form: string | null; readonly error: ApiError | null } {
  if (state.kind !== 'refused') {
    return { form: null, error: null };
  }
  const error = state.error;
  if (error.code === 'invitation_email_mismatch') {
    return { form: t('invitationPage.emailMismatch'), error };
  }
  if (shownSignUpFields.some((field) => error.fieldCode(field) !== undefined)) {
    return { form: t('errors.validation_failed'), error };
  }
  if (error.code === 'validation_failed') {
    return { form: t('form.failed', { requestId: error.requestId ?? '—' }), error };
  }
  return { form: errorMessage(t, error, state.retryAfter), error };
}

function SignUpForm({
  id,
  token,
  onInvalid,
  onCreated,
}: {
  readonly id: string;
  readonly token: string;
  readonly onInvalid: () => void;
  readonly onCreated: (email: string, password: string) => Promise<void>;
}): ReactElement {
  const { t, i18n } = useTranslation();
  const client = useApiClient();
  const action = useAction();
  const { form: formError, error } = signUpError(t, action.state);

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const email = stringField(data, 'email');
    const password = stringField(data, 'password');
    const language = stringField(data, 'language');
    action.run(async (): Promise<ActionState> => {
      const { error: refusal, response } = await client.POST('/api/v1/invitations/{token}/accept', {
        params: { path: { token } },
        body: {
          name: stringField(data, 'name'),
          email,
          password,
          language: isLanguage(language) ? language : 'ru',
          timezone: browserTimezone(),
          consent: data.get('consent') === 'on',
        },
      });
      if (response.ok) {
        await onCreated(email, password);
        return done;
      }
      const refused = refusedBy(response, refusal);
      if (refused.kind === 'refused' && refused.error.code === 'invitation_invalid') {
        onInvalid();
      }
      return refused;
    });
  }

  return (
    <form
      id={id}
      aria-label={t('invitationPage.createAccount')}
      onSubmit={submit}
      className="flex max-w-sm flex-col gap-4"
    >
      {formError === null ? null : <FormAlert>{formError}</FormAlert>}
      <TextField
        label={t('signUp.name')}
        name="name"
        autoComplete="name"
        maxLength={100}
        error={fieldMessage(t, error, 'name')}
      />
      <TextField
        label={t('signUp.email')}
        name="email"
        type="email"
        autoComplete="email"
        error={fieldMessage(t, error, 'email')}
      />
      <TextField
        label={t('signUp.password')}
        name="password"
        type="password"
        autoComplete="new-password"
        minLength={10}
        maxLength={128}
        hint={t('signUp.passwordHint')}
        error={fieldMessage(t, error, 'password')}
      />
      <LanguageChoice interfaceLanguage={isLanguage(i18n.language) ? i18n.language : 'ru'} />
      <ConsentField error={fieldMessage(t, error, 'consent')} />
      <SubmitButton busy={action.busy}>{t('invitationPage.signUpSubmit')}</SubmitButton>
    </form>
  );
}

function SignInForm({ id }: { readonly id: string }): ReactElement {
  const { t } = useTranslation();
  const session = useSession();
  const emailId = useId();
  const passwordId = useId();
  const showId = useId();
  const refusalId = useId();
  const [showPassword, setShowPassword] = useState(false);
  const [busy, setBusy] = useState(false);
  const [refusal, setRefusal] = useState<SignInRefusal | null>(null);

  async function signIn(email: string, password: string): Promise<void> {
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

  const described = refusal === null ? undefined : refusalId;
  return (
    <form
      id={id}
      aria-label={t('invitationPage.signInToAccept')}
      onSubmit={(event: SyntheticEvent<HTMLFormElement, SubmitEvent>) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget);
        void signIn(stringField(data, 'email'), stringField(data, 'password'));
      }}
      className="flex max-w-sm flex-col gap-4"
    >
      {refusal === null ? null : <RefusalMessage id={refusalId} refusal={refusal} />}
      <div className="flex flex-col gap-1">
        <label htmlFor={emailId} className="text-label">
          {t('signIn.email')}
        </label>
        <input
          id={emailId}
          name="email"
          type="email"
          autoComplete="username"
          required
          aria-describedby={described}
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
          type={showPassword ? 'text' : 'password'}
          autoComplete="current-password"
          required
          aria-describedby={described}
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
      <SubmitButton busy={busy}>{t('invitationPage.signInSubmit')}</SubmitButton>
    </form>
  );
}

function SignedOutPanel({
  token,
  onInvalid,
  onSignedUp,
}: {
  readonly token: string;
  readonly onInvalid: () => void;
  readonly onSignedUp: (email: string, password: string) => Promise<boolean>;
}): ReactElement {
  const { t } = useTranslation();
  const [mode, setMode] = useState<Mode>('choosing');
  const [askedToSignIn, setAskedToSignIn] = useState(false);
  const signUpId = useId();
  const signInId = useId();
  const firstChoice = useRef<HTMLButtonElement>(null);
  useFocusWhenLost(firstChoice);

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap gap-3">
        <button
          ref={firstChoice}
          type="button"
          aria-expanded={mode === 'signUp'}
          aria-controls={mode === 'signUp' ? signUpId : undefined}
          onClick={() => {
            setAskedToSignIn(false);
            setMode('signUp');
          }}
          className={quietButtonClass}
        >
          {t('invitationPage.createAccount')}
        </button>
        <button
          type="button"
          aria-expanded={mode === 'signIn'}
          aria-controls={mode === 'signIn' ? signInId : undefined}
          onClick={() => {
            setMode('signIn');
          }}
          className={quietButtonClass}
        >
          {t('invitationPage.signInToAccept')}
        </button>
      </div>
      {askedToSignIn ? <FormNotice takeFocus>{t('invitationPage.checkEmailOrSignIn')}</FormNotice> : null}
      {mode === 'signUp' ? (
        <SignUpForm
          id={signUpId}
          token={token}
          onInvalid={onInvalid}
          onCreated={async (email, password) => {
            if (!(await onSignedUp(email, password))) {
              setAskedToSignIn(true);
              setMode('signIn');
            }
          }}
        />
      ) : null}
      {mode === 'signIn' ? <SignInForm id={signInId} /> : null}
    </div>
  );
}

function TutorExplanation(): ReactElement {
  const { t } = useTranslation();
  return (
    <section className="flex flex-col items-start gap-3">
      <FormNotice takeFocus>{t('invitationPage.tutorAccount')}</FormNotice>
      <SignOutInPlace label={t('invitationPage.signOutForStudent')} />
    </section>
  );
}

function AcceptPanel({
  token,
  me,
  onInvalid,
}: {
  readonly token: string;
  readonly me: Me;
  readonly onInvalid: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const action = useAction();
  const refusal = action.state.kind === 'refused' ? action.state.error.code : undefined;

  function accept(): void {
    action.run(async (): Promise<ActionState> => {
      const { data, error, response } = await client.POST('/api/v1/invitations/{token}/accept', {
        params: { path: { token } },
        body: {},
      });
      if (data !== undefined && response.status === 200) {
        queryClient.removeQueries({ queryKey: myTutorsKey });
        void navigate('/tutors', { replace: true });
        return done;
      }
      const refused = refusedBy(response, error);
      if (refused.kind === 'refused' && refused.error.code === 'invitation_invalid') {
        onInvalid();
      }
      return refused;
    });
  }

  function refusalMessage(): ReactElement | null {
    switch (refusal) {
      case undefined:
        return null;
      case 'email_not_verified':
        return (
          <>
            <FormAlert>{t('errors.email_not_verified')}</FormAlert>
            <CheckEmail email={me.email} audience={{ kind: 'signedIn' }} />
          </>
        );
      case 'invitation_email_mismatch':
        return <FormAlert>{t('invitationPage.emailMismatch')}</FormAlert>;
      default:
        return <Refusal state={action.state} />;
    }
  }

  return (
    <section className="flex flex-col items-start gap-4">
      <FormNotice takeFocus>{t('invitationPage.signedInAs', { email: me.email })}</FormNotice>
      {me.reclaim_pending ? (
        <p>
          <Link to={reclaimAddress} className={linkClass}>
            {t('invitationPage.reclaimWaiting')}
          </Link>
        </p>
      ) : null}
      {refusalMessage()}
      <div className="flex flex-wrap items-center gap-4">
        {refusal === 'email_not_verified' || refusal === 'invitation_email_mismatch' ? null : (
          <ActionButton busy={action.busy} onClick={accept} className={primaryButtonClass}>
            {t('invitationPage.accept')}
          </ActionButton>
        )}
        <SignOutInPlace label={t('invitationPage.useAnotherAccount')} />
      </div>
    </section>
  );
}

export function InvitationPage(): ReactElement {
  const { t } = useTranslation();
  const { token } = useInvitationToken();
  const client = useApiClient();
  const session = useSession();
  const sessionState = useSessionState();
  const navigate = useNavigate();
  const [invitation, setInvitation] = useState<Invitation>({ kind: 'loading' });
  const [attempt, setAttempt] = useState(0);
  const [keepSignedOutPanel, setKeepSignedOutPanel] = useState(false);
  const settled = sessionState.kind === 'signedIn' || sessionState.kind === 'signedOut';

  useEffect(() => {
    if (token === null || !settled) {
      return undefined;
    }
    const controller = new AbortController();
    client.GET('/api/v1/invitations/{token}', { params: { path: { token } }, signal: controller.signal }).then(
      ({ data, error, response }) => {
        if (data !== undefined) {
          setInvitation({ kind: 'ready', tutor: data.tutor });
        } else if (response.status === 404) {
          setInvitation({ kind: 'invalid' });
        } else {
          setInvitation({ kind: 'failed', error: apiErrorFrom(response.status, error) });
        }
      },
      (thrown: unknown) => {
        if (!controller.signal.aborted) {
          setInvitation({ kind: 'failed', error: asApiError(thrown) });
        }
      },
    );
    return (): void => {
      controller.abort();
    };
  }, [token, settled, attempt, client]);

  function markInvalid(): void {
    setInvitation({ kind: 'invalid' });
  }

  async function signInAfterSignUp(email: string, password: string): Promise<boolean> {
    setKeepSignedOutPanel(true);
    const signedIn = await session.signIn(email, password).then(
      (result) => result.kind === 'signedIn',
      () => false,
    );
    const signedInAs = session.snapshot();
    if (signedIn && signedInAs.kind === 'signedIn' && signedInAs.me.state === 'active') {
      setKeepSignedOutPanel(false);
      return true;
    }
    if (signedIn) {
      void navigate('/', { replace: true });
      return true;
    }
    setKeepSignedOutPanel(false);
    return false;
  }

  function panel(heldToken: string): ReactElement | null {
    if (sessionState.kind === 'signedOut' || keepSignedOutPanel) {
      return <SignedOutPanel token={heldToken} onInvalid={markInvalid} onSignedUp={signInAfterSignUp} />;
    }
    if (sessionState.kind !== 'signedIn') {
      return null;
    }
    if (sessionState.me.role === 'tutor') {
      return <TutorExplanation />;
    }
    return <AcceptPanel token={heldToken} me={sessionState.me} onInvalid={markInvalid} />;
  }

  function body(): ReactElement | null {
    if (token === null) {
      return <p>{t('invitationPage.openLinkAgain')}</p>;
    }
    switch (invitation.kind) {
      case 'loading':
        return <DelayedLoading />;
      case 'invalid':
        return <FormAlert>{t('invitationPage.invalid')}</FormAlert>;
      case 'failed':
        return (
          <LoadFailed
            error={invitation.error}
            onRetry={() => {
              setAttempt((count) => count + 1);
            }}
          />
        );
      case 'ready':
        return (
          <>
            <TutorProfile tutor={invitation.tutor} />
            {panel(token)}
          </>
        );
    }
  }

  return <Page>{body()}</Page>;
}
