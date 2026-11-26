import { type ReactElement, type ReactNode, type RefObject, type SyntheticEvent, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { ActionMenu } from '../../components/ui/ActionMenu.tsx';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormActions,
  FormAlert,
  SelectChevron,
  SubmitButton,
  TextField,
  dangerFilledButtonClass,
  errorMessage,
  fieldClass,
  fieldMessage,
  quietButtonClass,
  retryAfterSeconds,
  selectClass,
  stringField,
} from '../../components/ui/Form.tsx';
import { MoreIcon } from '../../components/ui/icons.tsx';
import { FocusedNotice, LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import { type Me, useSession, useSignedInAccount } from '../../lib/api/session.ts';
import { isLanguage, languages } from '../../lib/i18n/resources.ts';
import { SettingsNav } from './SettingsNav.tsx';
import { SettingsRow, SettingsRows } from './SettingsRow.tsx';
import { TimezoneField } from './TimezoneField.tsx';

type Outcome =
  | { readonly kind: 'idle' }
  | { readonly kind: 'sending' }
  | { readonly kind: 'done' }
  | { readonly kind: 'refused'; readonly error: ApiError; readonly retryAfter: number | null };

const themes = ['system', 'light', 'dark'] as const;
type Theme = (typeof themes)[number];

function isTheme(value: string): value is Theme {
  return themes.some((theme) => theme === value);
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

function SelectRow({
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
    <SettingsRow label={label} labelFor={selectId}>
      <div className="relative w-full max-w-sm">
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
    </SettingsRow>
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
      className="flex flex-col gap-4"
    >
      <h2 id={headingId} className="text-heading">
        {t('settings.preferences')}
      </h2>
      <RefusalMessage outcome={outcome} />
      <LiveNotice message={outcome.kind === 'done' ? t('settings.saved') : null} />
      <SettingsRows>
        <SelectRow
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
        </SelectRow>
        <SettingsRow label={t('settings.timezone')}>
          <TimezoneField
            name="timezone"
            defaultValue={me.timezone}
            language={me.language}
            error={fieldMessage(t, error, 'timezone')}
            legendHidden
          />
        </SettingsRow>
        <SelectRow
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
        </SelectRow>
      </SettingsRows>
      <div>
        <SubmitButton busy={outcome.kind === 'sending'}>{t('settings.save')}</SubmitButton>
      </div>
    </form>
  );
}

function PasswordDialog({
  open,
  returnFocus,
  onClose,
  onChanged,
}: {
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
  readonly onChanged: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const headingId = useId();
  const firstField = useRef<HTMLInputElement>(null);
  const [outcome, setOutcome] = useState<Outcome>({ kind: 'idle' });
  const sending = outcome.kind === 'sending';
  const error = outcome.kind === 'refused' ? outcome.error : null;

  async function change(form: HTMLFormElement): Promise<void> {
    const data = new FormData(form);
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
    setOutcome({ kind: 'idle' });
    onChanged();
  }

  return (
    <ModalDialog
      open={open}
      busy={sending}
      labelledBy={headingId}
      returnFocus={returnFocus}
      initialFocus={firstField}
      onClose={() => {
        setOutcome({ kind: 'idle' });
        onClose();
      }}
    >
      <form
        aria-labelledby={headingId}
        onChange={() => {
          setOutcome(settled);
        }}
        onSubmit={(event: SyntheticEvent<HTMLFormElement, SubmitEvent>) => {
          event.preventDefault();
          change(event.currentTarget).catch((thrown: unknown) => {
            setOutcome(failed(thrown));
          });
        }}
        className="flex flex-col gap-4"
      >
        <h2 id={headingId} className="text-heading">
          {t('settings.changePassword')}
        </h2>
        <RefusalMessage outcome={outcome} />
        <PasswordInput
          inputRef={firstField}
          label={t('settings.currentPassword')}
          name="current_password"
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
        <FormActions>
          <SubmitButton busy={sending}>{t('settings.changePassword')}</SubmitButton>
          <ActionButton busy={false} unavailable={sending} onClick={onClose} className={quietButtonClass}>
            {t('settings.cancel')}
          </ActionButton>
        </FormActions>
      </form>
    </ModalDialog>
  );
}

function PasswordInput({
  inputRef,
  label,
  name,
  autoComplete,
  error,
}: {
  readonly inputRef: RefObject<HTMLInputElement | null>;
  readonly label: string;
  readonly name: string;
  readonly autoComplete: string;
  readonly error: string | undefined;
}): ReactElement {
  const inputId = useId();
  const errorId = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={inputId} className="text-label">
        {label}
      </label>
      <input
        ref={inputRef}
        id={inputId}
        name={name}
        type="password"
        autoComplete={autoComplete}
        required
        aria-invalid={error === undefined ? undefined : true}
        aria-describedby={error === undefined ? undefined : errorId}
        className={fieldClass}
      />
      {error === undefined ? null : (
        <p id={errorId} className="text-small text-danger">
          {error}
        </p>
      )}
    </div>
  );
}

function SignOutEverywhereDialog({
  open,
  returnFocus,
  onBusy,
  onClose,
}: {
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onBusy: (busy: boolean) => void;
  readonly onClose: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const session = useSession();
  const headingId = useId();
  const consequenceId = useId();
  const cancelButton = useRef<HTMLButtonElement>(null);
  const [outcome, setOutcome] = useState<Outcome>({ kind: 'idle' });
  const sending = outcome.kind === 'sending';

  async function signOutEverywhere(): Promise<void> {
    setOutcome({ kind: 'sending' });
    onBusy(true);
    const { error, response } = await session.whileEnding(
      (send) => client.POST('/api/v1/auth/logout-all', { fetch: send }),
      (answer) => (answer.response.ok ? 'signedOut' : null),
    );
    if (!response.ok) {
      onBusy(false);
      setOutcome(refusedFrom(response, error));
    }
  }

  return (
    <ModalDialog
      open={open}
      busy={sending}
      labelledBy={headingId}
      describedBy={consequenceId}
      returnFocus={returnFocus}
      initialFocus={cancelButton}
      onClose={() => {
        setOutcome({ kind: 'idle' });
        onClose();
      }}
    >
      <div className="flex flex-col gap-4">
        <h2 id={headingId} className="text-heading">
          {t('settings.signOutEverywhereConfirm')}
        </h2>
        <p id={consequenceId}>{t('settings.signOutEverywhereBody')}</p>
        <RefusalMessage outcome={outcome} />
        <FormActions>
          <ActionButton
            busy={sending}
            onClick={() => {
              signOutEverywhere().catch((thrown: unknown) => {
                onBusy(false);
                setOutcome(failed(thrown));
              });
            }}
            className={dangerFilledButtonClass}
          >
            {t('settings.signOutEverywhere')}
          </ActionButton>
          <ActionButton
            ref={cancelButton}
            busy={false}
            unavailable={sending}
            onClick={onClose}
            className={quietButtonClass}
          >
            {t('settings.cancel')}
          </ActionButton>
        </FormActions>
      </div>
    </ModalDialog>
  );
}

function DeleteAccountDialog({
  role,
  open,
  returnFocus,
  onBusy,
  onClose,
}: {
  readonly role: Me['role'];
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onBusy: (busy: boolean) => void;
  readonly onClose: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const session = useSession();
  const headingId = useId();
  const consequenceId = useId();
  const passwordField = useRef<HTMLInputElement>(null);
  const [outcome, setOutcome] = useState<Outcome>({ kind: 'idle' });
  const sending = outcome.kind === 'sending';
  const error = outcome.kind === 'refused' ? outcome.error : null;

  async function remove(data: FormData): Promise<void> {
    setOutcome({ kind: 'sending' });
    onBusy(true);
    const { error: refusal, response } = await session.whileEnding(
      (send) => client.DELETE('/api/v1/users/me', { body: { password: stringField(data, 'password') }, fetch: send }),
      (answer) => (answer.response.ok ? 'accountDeleted' : null),
    );
    if (!response.ok) {
      onBusy(false);
      setOutcome(refusedFrom(response, refusal));
    }
  }

  return (
    <ModalDialog
      open={open}
      busy={sending}
      labelledBy={headingId}
      describedBy={consequenceId}
      returnFocus={returnFocus}
      initialFocus={passwordField}
      onClose={() => {
        setOutcome({ kind: 'idle' });
        onClose();
      }}
    >
      <form
        onChange={() => {
          setOutcome(settled);
        }}
        onSubmit={(event: SyntheticEvent<HTMLFormElement, SubmitEvent>) => {
          event.preventDefault();
          remove(new FormData(event.currentTarget)).catch((thrown: unknown) => {
            onBusy(false);
            setOutcome(failed(thrown));
          });
        }}
        className="flex flex-col gap-4"
      >
        <h2 id={headingId} className="text-heading">
          {t('settings.deleteAccountConfirm')}
        </h2>
        <p id={consequenceId}>{t(`settings.deleteAccountConsequence.${role}`)}</p>
        <RefusalMessage outcome={outcome} />
        <PasswordInput
          inputRef={passwordField}
          label={t('settings.currentPassword')}
          name="password"
          autoComplete="current-password"
          error={fieldMessage(t, error, 'password')}
        />
        <FormActions>
          <ActionButton type="submit" busy={sending} className={dangerFilledButtonClass}>
            {t('settings.deleteAccountSubmit')}
          </ActionButton>
          <ActionButton busy={false} unavailable={sending} onClick={onClose} className={quietButtonClass}>
            {t('settings.cancel')}
          </ActionButton>
        </FormActions>
      </form>
    </ModalDialog>
  );
}

type AccountDialog = 'password' | 'signOutEverywhere' | 'delete' | null;

export function AccountSettingsPage(): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const passwordButton = useRef<HTMLButtonElement>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  const [dialog, setDialog] = useState<AccountDialog>(null);
  const [ending, setEnding] = useState(false);
  const [passwordNotice, setPasswordNotice] = useState(0);
  const securityHeading = useId();
  const close = (): void => {
    setDialog(null);
  };

  return (
    <Page
      menu={
        <ActionMenu
          label={t('settings.accountActions')}
          unavailable={ending}
          busy={ending}
          buttonRef={menuButton}
          items={[
            {
              label: t('settings.signOutEverywhere'),
              danger: true,
              onChoose: (): void => {
                setDialog('signOutEverywhere');
              },
            },
            {
              label: t('settings.deleteAccount'),
              danger: true,
              onChoose: (): void => {
                setDialog('delete');
              },
            },
          ]}
        >
          <MoreIcon />
        </ActionMenu>
      }
    >
      <SettingsNav />
      <PreferencesSection key={me.id} me={me} />
      <section aria-labelledby={securityHeading} className="flex flex-col gap-4">
        <h2 id={securityHeading} className="text-heading">
          {t('settings.security')}
        </h2>
        {passwordNotice === 0 ? null : <FocusedNotice key={passwordNotice} message={t('settings.passwordChanged')} />}
        <SettingsRows>
          <SettingsRow label={t('settings.password')}>
            <button
              ref={passwordButton}
              type="button"
              onClick={() => {
                setPasswordNotice(0);
                setDialog('password');
              }}
              className={quietButtonClass}
            >
              {t('settings.changePassword')}
            </button>
          </SettingsRow>
        </SettingsRows>
      </section>
      <PasswordDialog
        open={dialog === 'password'}
        returnFocus={passwordButton}
        onClose={close}
        onChanged={() => {
          setDialog(null);
          setPasswordNotice((count) => count + 1);
        }}
      />
      <SignOutEverywhereDialog
        open={dialog === 'signOutEverywhere'}
        returnFocus={menuButton}
        onBusy={setEnding}
        onClose={close}
      />
      <DeleteAccountDialog
        role={me.role}
        open={dialog === 'delete'}
        returnFocus={menuButton}
        onBusy={setEnding}
        onClose={close}
      />
    </Page>
  );
}
