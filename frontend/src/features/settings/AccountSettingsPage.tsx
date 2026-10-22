import { type ReactElement, type ReactNode, type SyntheticEvent, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import {
  FormAlert,
  FormNotice,
  SelectChevron,
  SubmitButton,
  TextField,
  errorMessage,
  fieldMessage,
  quietButtonClass,
  retryAfterSeconds,
  selectClass,
  stringField,
} from '../../components/ui/Form.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import { type Me, useSession, useSignedInAccount } from '../../lib/api/session.ts';
import { isLanguage, languages } from '../../lib/i18n/resources.ts';
import { SettingsNav } from './SettingsNav.tsx';

type Outcome =
  | { readonly kind: 'idle' }
  | { readonly kind: 'sending' }
  | { readonly kind: 'done' }
  | { readonly kind: 'refused'; readonly error: ApiError; readonly retryAfter: number | null };

const themes = ['system', 'light', 'dark'] as const;
type Theme = (typeof themes)[number];

const dangerButtonClass =
  'inline-flex min-h-11 items-center justify-center rounded-md border border-danger px-4 text-strong text-danger hover:bg-surface disabled:opacity-60';

function isTheme(value: string): value is Theme {
  return themes.some((theme) => theme === value);
}

function timezones(current: string): readonly string[] {
  const known = Intl.supportedValuesOf('timeZone');
  return known.includes(current) ? known : [current, ...known];
}

function refusedFrom(response: Response, error: unknown): Outcome {
  return { kind: 'refused', error: apiErrorFrom(response.status, error), retryAfter: retryAfterSeconds(response) };
}

function failed(thrown: unknown): Outcome {
  return { kind: 'refused', error: asApiError(thrown), retryAfter: null };
}

function settled(outcome: Outcome): Outcome {
  return outcome.kind === 'done' || outcome.kind === 'refused' ? { kind: 'idle' } : outcome;
}

function RefusalMessage({ outcome }: { readonly outcome: Outcome }): ReactElement | null {
  const { t } = useTranslation();
  if (outcome.kind !== 'refused') {
    return null;
  }
  return (
    <FormAlert>
      {outcome.error.fields.length > 0
        ? t('errors.validation_failed')
        : errorMessage(t, outcome.error, outcome.retryAfter)}
    </FormAlert>
  );
}

function SelectField({
  label,
  name,
  defaultValue,
  error,
  children,
}: {
  readonly label: string;
  readonly name: string;
  readonly defaultValue: string;
  readonly error: string | undefined;
  readonly children: ReactNode;
}): ReactElement {
  const selectId = useId();
  const errorId = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={selectId} className="text-label">
        {label}
      </label>
      <div className="relative">
        <select
          id={selectId}
          name={name}
          defaultValue={defaultValue}
          aria-invalid={error === undefined ? undefined : true}
          aria-describedby={error === undefined ? undefined : errorId}
          className={selectClass}
        >
          {children}
        </select>
        <SelectChevron />
      </div>
      {error === undefined ? null : (
        <p id={errorId} className="text-small text-danger">
          {error}
        </p>
      )}
    </div>
  );
}

function PreferencesSection({ me }: { readonly me: Me }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const session = useSession();
  const headingId = useId();
  const [outcome, setOutcome] = useState<Outcome>({ kind: 'idle' });
  const error = outcome.kind === 'refused' ? outcome.error : null;

  async function save(data: FormData): Promise<void> {
    const language = stringField(data, 'language');
    const theme = stringField(data, 'theme');
    setOutcome({ kind: 'sending' });
    const {
      data: updated,
      error: refusal,
      response,
    } = await client.PATCH('/api/v1/users/me', {
      body: {
        language: isLanguage(language) ? language : me.language,
        timezone: stringField(data, 'timezone'),
        theme: isTheme(theme) ? theme : me.theme,
      },
    });
    if (updated === undefined) {
      setOutcome(refusedFrom(response, refusal));
      return;
    }
    session.updateAccount(updated);
    setOutcome({ kind: 'done' });
  }

  return (
    <form
      aria-labelledby={headingId}
      onChange={() => {
        setOutcome(settled);
      }}
      onSubmit={(event: SyntheticEvent<HTMLFormElement, SubmitEvent>) => {
        event.preventDefault();
        save(new FormData(event.currentTarget)).catch((thrown: unknown) => {
          setOutcome(failed(thrown));
        });
      }}
      className="flex max-w-sm flex-col gap-4"
    >
      <h2 id={headingId} className="text-heading">
        {t('settings.preferences')}
      </h2>
      <RefusalMessage outcome={outcome} />
      {outcome.kind === 'done' ? <FormNotice>{t('settings.saved')}</FormNotice> : null}
      <SelectField
        label={t('settings.language')}
        name="language"
        defaultValue={me.language}
        error={fieldMessage(t, error, 'language')}
      >
        {languages.map((language) => (
          <option key={language} value={language} lang={language}>
            {t(`language.${language}`)}
          </option>
        ))}
      </SelectField>
      <SelectField
        label={t('settings.timezone')}
        name="timezone"
        defaultValue={me.timezone}
        error={fieldMessage(t, error, 'timezone')}
      >
        {timezones(me.timezone).map((zone) => (
          <option key={zone} value={zone}>
            {zone}
          </option>
        ))}
      </SelectField>
      <SelectField
        label={t('settings.theme')}
        name="theme"
        defaultValue={me.theme}
        error={fieldMessage(t, error, 'theme')}
      >
        {themes.map((theme) => (
          <option key={theme} value={theme}>
            {t(`settings.themes.${theme}`)}
          </option>
        ))}
      </SelectField>
      <SubmitButton busy={outcome.kind === 'sending'}>{t('settings.save')}</SubmitButton>
    </form>
  );
}

function PasswordSection(): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const headingId = useId();
  const [outcome, setOutcome] = useState<Outcome>({ kind: 'idle' });
  const [form, setForm] = useState(0);
  const error = outcome.kind === 'refused' ? outcome.error : null;

  async function change(data: FormData): Promise<void> {
    setOutcome({ kind: 'sending' });
    const { error: refusal, response } = await client.POST('/api/v1/auth/password/change', {
      body: {
        current_password: stringField(data, 'current_password'),
        new_password: stringField(data, 'new_password'),
      },
    });
    if (!response.ok) {
      setOutcome(refusedFrom(response, refusal));
      return;
    }
    setForm((value) => value + 1);
    setOutcome({ kind: 'done' });
  }

  return (
    <form
      key={form}
      aria-labelledby={headingId}
      onChange={() => {
        setOutcome(settled);
      }}
      onSubmit={(event: SyntheticEvent<HTMLFormElement, SubmitEvent>) => {
        event.preventDefault();
        change(new FormData(event.currentTarget)).catch((thrown: unknown) => {
          setOutcome(failed(thrown));
        });
      }}
      className="flex max-w-sm flex-col gap-4"
    >
      <h2 id={headingId} className="text-heading">
        {t('settings.changePassword')}
      </h2>
      <RefusalMessage outcome={outcome} />
      {outcome.kind === 'done' ? <FormNotice>{t('settings.passwordChanged')}</FormNotice> : null}
      <TextField
        label={t('settings.currentPassword')}
        name="current_password"
        type="password"
        autoComplete="current-password"
        error={fieldMessage(t, error, 'current_password')}
      />
      <TextField
        label={t('verifyEmail.newPassword')}
        name="new_password"
        type="password"
        autoComplete="new-password"
        minLength={10}
        maxLength={128}
        hint={t('signUp.passwordHint')}
        error={fieldMessage(t, error, 'new_password')}
      />
      <SubmitButton busy={outcome.kind === 'sending'} quiet>
        {t('settings.changePassword')}
      </SubmitButton>
    </form>
  );
}

function SignOutEverywhereSection(): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const session = useSession();
  const [outcome, setOutcome] = useState<Outcome>({ kind: 'idle' });

  async function signOutEverywhere(): Promise<void> {
    setOutcome({ kind: 'sending' });
    const { error, response } = await client.POST('/api/v1/auth/logout-all');
    if (!response.ok) {
      setOutcome(refusedFrom(response, error));
      return;
    }
    session.endRevoked('signedOut');
  }

  return (
    <section className="flex max-w-reading flex-col gap-3">
      <h2 className="text-heading">{t('settings.signOutEverywhere')}</h2>
      <p className="text-text-muted">{t('settings.signOutEverywhereBody')}</p>
      <RefusalMessage outcome={outcome} />
      <p>
        <button
          type="button"
          disabled={outcome.kind === 'sending'}
          onClick={() => {
            signOutEverywhere().catch((thrown: unknown) => {
              setOutcome(failed(thrown));
            });
          }}
          className={quietButtonClass}
        >
          {t('settings.signOutEverywhere')}
        </button>
      </p>
    </section>
  );
}

function DeleteAccountSection(): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const session = useSession();
  const dialog = useRef<HTMLDialogElement>(null);
  const opener = useRef<HTMLButtonElement>(null);
  const headingId = useId();
  const consequenceId = useId();
  const [opening, setOpening] = useState(0);
  const [open, setOpen] = useState(false);
  const [outcome, setOutcome] = useState<Outcome>({ kind: 'idle' });
  const sending = outcome.kind === 'sending';
  const error = outcome.kind === 'refused' ? outcome.error : null;

  useEffect(() => {
    const element = dialog.current;
    if (element === null) {
      return;
    }
    if (open && !element.open) {
      element.showModal();
    }
    if (!open && element.open) {
      element.close();
      opener.current?.focus();
    }
  }, [open]);

  async function remove(data: FormData): Promise<void> {
    setOutcome({ kind: 'sending' });
    const { error: refusal, response } = await client.DELETE('/api/v1/users/me', {
      body: { password: stringField(data, 'password') },
    });
    if (!response.ok) {
      setOutcome(refusedFrom(response, refusal));
      return;
    }
    session.endRevoked('accountDeleted');
  }

  return (
    <section className="flex max-w-reading flex-col gap-3">
      <h2 className="text-heading">{t('settings.deleteAccount')}</h2>
      <p className="text-text-muted">{t('settings.deleteAccountBody')}</p>
      <p>
        <button
          ref={opener}
          type="button"
          onClick={() => {
            if (sending) {
              return;
            }
            setOutcome({ kind: 'idle' });
            setOpening((count) => count + 1);
            setOpen(true);
          }}
          className={dangerButtonClass}
        >
          {t('settings.deleteAccount')}
        </button>
      </p>
      <dialog
        ref={dialog}
        aria-labelledby={headingId}
        aria-describedby={consequenceId}
        onCancel={(event) => {
          if (sending) {
            event.preventDefault();
          }
        }}
        onClose={() => {
          if (sending) {
            dialog.current?.showModal();
            return;
          }
          setOpen(false);
          opener.current?.focus();
        }}
        className="m-auto w-full max-w-sm rounded-lg border border-border bg-surface p-6 text-text shadow-raised backdrop:bg-scrim dark:border-border-strong dark:shadow-none"
      >
        <form
          key={opening}
          onChange={() => {
            setOutcome(settled);
          }}
          onSubmit={(event: SyntheticEvent<HTMLFormElement, SubmitEvent>) => {
            event.preventDefault();
            remove(new FormData(event.currentTarget)).catch((thrown: unknown) => {
              setOutcome(failed(thrown));
            });
          }}
          className="flex flex-col gap-4"
        >
          <h3 id={headingId} className="text-heading">
            {t('settings.deleteAccountConfirm')}
          </h3>
          <p id={consequenceId}>{t('settings.deleteAccountConsequence')}</p>
          <RefusalMessage outcome={outcome} />
          <TextField
            label={t('settings.currentPassword')}
            name="password"
            type="password"
            autoComplete="current-password"
            error={fieldMessage(t, error, 'password')}
          />
          <div className="flex flex-wrap gap-3">
            <button type="submit" disabled={sending} className={dangerButtonClass}>
              {t('settings.deleteAccountSubmit')}
            </button>
            <button
              type="button"
              disabled={sending}
              onClick={() => {
                setOpen(false);
              }}
              className="inline-flex min-h-11 items-center px-3 text-small text-accent-text underline disabled:opacity-60"
            >
              {t('settings.cancel')}
            </button>
          </div>
        </form>
      </dialog>
    </section>
  );
}

export function AccountSettingsPage(): ReactElement {
  const me = useSignedInAccount();
  return (
    <Page>
      <SettingsNav />
      <PreferencesSection key={me.id} me={me} />
      <PasswordSection />
      <SignOutEverywhereSection />
      <DeleteAccountSection />
    </Page>
  );
}
