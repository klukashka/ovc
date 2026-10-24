import { type ReactElement, type SyntheticEvent, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import {
  FormAlert,
  SelectChevron,
  SubmitButton,
  TextField,
  errorMessage,
  fieldMessage,
  linkClass,
  retryAfterSeconds,
  selectClass,
  stringField,
} from '../../components/ui/Form.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import { type Language, isLanguage, languages } from '../../lib/i18n/resources.ts';
import { CheckEmail } from './CheckEmail.tsx';

const choiceClass =
  'flex min-h-24 flex-1 flex-col justify-center gap-1 rounded-lg border border-border-strong bg-surface px-5 py-4 hover:bg-hover';

export function SignUpChoicePage(): ReactElement {
  const { t } = useTranslation();
  return (
    <Page>
      <nav aria-label={t('signUp.choiceLabel')} className="flex flex-col gap-4 tablet:flex-row">
        <Link to="/signup/tutor" className={choiceClass}>
          <span className="text-heading">{t('signUp.teach')}</span>
          <span className="text-small text-text-muted">{t('signUp.teachHint')}</span>
        </Link>
        <Link to="/signup/student" className={choiceClass}>
          <span className="text-heading">{t('signUp.study')}</span>
          <span className="text-small text-text-muted">{t('signUp.studyHint')}</span>
        </Link>
      </nav>
      <p>
        <Link to="/" className={linkClass}>
          {t('signUp.haveAccount')}
        </Link>
      </p>
    </Page>
  );
}

export function StudentSignUpPage(): ReactElement {
  const { t } = useTranslation();
  return (
    <Page>
      <p>{t('signUp.studentBody')}</p>
      <p>
        <Link to="/" className={linkClass}>
          {t('signUp.haveAccount')}
        </Link>
      </p>
    </Page>
  );
}

export function browserTimezone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone;
}

type SignUpState =
  | { readonly kind: 'editing'; readonly error: ApiError | null; readonly retryAfter: number | null }
  | { readonly kind: 'sending' }
  | { readonly kind: 'sent'; readonly email: string };

export function LanguageChoice({ interfaceLanguage }: { readonly interfaceLanguage: Language }): ReactElement {
  const { t } = useTranslation();
  const selectId = useId();
  const [chosen, setChosen] = useState<Language | null>(null);
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={selectId} className="text-label">
        {t('signUp.language')}
      </label>
      <div className="relative">
        <select
          id={selectId}
          name="language"
          value={chosen ?? interfaceLanguage}
          onChange={(event) => {
            const value = event.currentTarget.value;
            if (isLanguage(value)) {
              setChosen(value);
            }
          }}
          className={selectClass}
        >
          {languages.map((language) => (
            <option key={language} value={language} lang={language}>
              {t(`language.${language}`)}
            </option>
          ))}
        </select>
        <SelectChevron />
      </div>
    </div>
  );
}

export function ConsentField({ error }: { readonly error: string | undefined }): ReactElement {
  const { t } = useTranslation();
  const checkboxId = useId();
  const errorId = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={checkboxId} className="flex min-h-11 items-start gap-2 text-small">
        <input
          id={checkboxId}
          name="consent"
          type="checkbox"
          required
          aria-invalid={error === undefined ? undefined : true}
          aria-describedby={error === undefined ? undefined : errorId}
          className="mt-0.5 size-4"
        />
        <span>
          {t('signUp.consent')}{' '}
          <Link to="/privacy" target="_blank" rel="noreferrer" className="text-accent-text underline">
            {t('signUp.privacyPolicy')}
            <span className="sr-only"> {t('signUp.opensInNewTab')}</span>
          </Link>
        </span>
      </label>
      {error === undefined ? null : (
        <p id={errorId} className="text-small text-danger">
          {error}
        </p>
      )}
    </div>
  );
}

const shownFields = ['name', 'email', 'password', 'consent'];

export function TutorSignUpPage(): ReactElement {
  const { t, i18n } = useTranslation();
  const client = useApiClient();
  const [state, setState] = useState<SignUpState>({ kind: 'editing', error: null, retryAfter: null });

  async function submit(form: HTMLFormElement): Promise<void> {
    const data = new FormData(form);
    const email = stringField(data, 'email');
    const language = stringField(data, 'language');
    setState({ kind: 'sending' });
    const { error, response } = await client.POST('/api/v1/auth/register', {
      body: {
        role: 'tutor',
        name: stringField(data, 'name'),
        email,
        password: stringField(data, 'password'),
        language: isLanguage(language) ? language : 'ru',
        timezone: browserTimezone(),
        consent: data.get('consent') === 'on',
      },
    });
    if (response.ok) {
      setState({ kind: 'sent', email });
      return;
    }
    setState({ kind: 'editing', error: apiErrorFrom(response.status, error), retryAfter: retryAfterSeconds(response) });
  }

  function onSubmit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    submit(event.currentTarget).catch((thrown: unknown) => {
      setState({ kind: 'editing', error: asApiError(thrown), retryAfter: null });
    });
  }

  if (state.kind === 'sent') {
    return (
      <Page title="page.checkEmail" focusTitle>
        <CheckEmail email={state.email} audience={{ kind: 'email', email: state.email }} />
      </Page>
    );
  }

  const error = state.kind === 'editing' ? state.error : null;
  const fieldError = (field: string): string | undefined => fieldMessage(t, error, field);
  const shownProblem = error !== null && shownFields.some((field) => error.fieldCode(field) !== undefined);
  const formError =
    error === null
      ? null
      : shownProblem
        ? t('errors.validation_failed')
        : error.code === 'validation_failed'
          ? t('form.failed', { requestId: error.requestId ?? '—' })
          : errorMessage(t, error, state.kind === 'editing' ? state.retryAfter : null);
  return (
    <Page>
      <form onSubmit={onSubmit} className="flex max-w-sm flex-col gap-4">
        {formError === null ? null : <FormAlert>{formError}</FormAlert>}
        <TextField
          label={t('signUp.name')}
          name="name"
          autoComplete="name"
          maxLength={100}
          error={fieldError('name')}
        />
        <TextField
          label={t('signUp.email')}
          name="email"
          type="email"
          autoComplete="email"
          error={fieldError('email')}
        />
        <TextField
          label={t('signUp.password')}
          name="password"
          type="password"
          autoComplete="new-password"
          minLength={10}
          maxLength={128}
          hint={t('signUp.passwordHint')}
          error={fieldError('password')}
        />
        <LanguageChoice interfaceLanguage={isLanguage(i18n.language) ? i18n.language : 'ru'} />
        <ConsentField error={fieldError('consent')} />
        <SubmitButton busy={state.kind === 'sending'}>{t('signUp.submit')}</SubmitButton>
        <p>
          <Link to="/" className={linkClass}>
            {t('signUp.haveAccount')}
          </Link>
        </p>
      </form>
    </Page>
  );
}
