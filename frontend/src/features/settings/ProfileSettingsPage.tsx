import { type ChangeEvent, type ReactElement, type SyntheticEvent, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';

import {
  FormAlert,
  FormNotice,
  SubmitButton,
  TextField,
  errorMessage,
  fieldClass,
  fieldMessage,
  quietButtonClass,
  retryAfterSeconds,
  stringField,
} from '../../components/ui/Form.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import { type components } from '../../lib/api/schema.ts';
import { type Me, useSession, useSignedInAccount } from '../../lib/api/session.ts';
import { SettingsNav } from './SettingsNav.tsx';

type Saving =
  | { readonly kind: 'idle' }
  | { readonly kind: 'saving' }
  | { readonly kind: 'saved' }
  | { readonly kind: 'refused'; readonly error: ApiError; readonly retryAfter: number | null };

type ProfilePatch = components['schemas']['ProfilePatch'];

interface PendingWork {
  readonly busy: boolean;
  readonly track: <Result>(work: Promise<Result>) => Promise<Result>;
}

const acceptedAvatarFiles = 'image/jpeg,image/png,image/webp,.jpg,.jpeg,.png,.webp';
const maxAvatarBytes = 5 * 1000 * 1000;
const maxSubjects = 10;

function subjectsOf(value: string): string[] {
  return value
    .split(',')
    .map((subject) => subject.trim())
    .filter((subject) => subject !== '');
}

function avatarAddress(me: Me): string | null {
  return me.avatar_id === null ? null : `/api/v1/avatars/${me.avatar_id}`;
}

function usePendingWork(): PendingWork {
  const [pending, setPending] = useState(0);
  return {
    busy: pending > 0,
    track: async <Result,>(work: Promise<Result>): Promise<Result> => {
      setPending((count) => count + 1);
      try {
        return await work;
      } finally {
        setPending((count) => count - 1);
      }
    },
  };
}

function useProfileUpdate(work: PendingWork): {
  readonly saving: Saving;
  readonly save: (patch: ProfilePatch) => void;
  readonly refuse: (error: ApiError, retryAfter: number | null) => void;
  readonly settle: () => void;
} {
  const client = useApiClient();
  const session = useSession();
  const [saving, setSaving] = useState<Saving>({ kind: 'idle' });

  async function send(patch: ProfilePatch): Promise<void> {
    setSaving({ kind: 'saving' });
    const { data, error, response } = await client.PATCH('/api/v1/users/me', { body: patch });
    if (data === undefined) {
      setSaving({
        kind: 'refused',
        error: apiErrorFrom(response.status, error),
        retryAfter: retryAfterSeconds(response),
      });
      return;
    }
    session.updateAccount(data);
    setSaving({ kind: 'saved' });
  }

  return {
    saving,
    save: (patch): void => {
      work.track(send(patch)).catch((thrown: unknown) => {
        setSaving({ kind: 'refused', error: asApiError(thrown), retryAfter: null });
      });
    },
    refuse: (error, retryAfter): void => {
      setSaving({ kind: 'refused', error, retryAfter });
    },
    settle: (): void => {
      if (saving.kind === 'saved' || saving.kind === 'refused') {
        setSaving({ kind: 'idle' });
      }
    },
  };
}

function AvatarSection({ me, work }: { readonly me: Me; readonly work: PendingWork }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const inputId = useId();
  const headingId = useId();
  const { saving, save, refuse, settle } = useProfileUpdate(work);
  const address = avatarAddress(me);

  async function upload(file: File): Promise<void> {
    const body = new FormData();
    body.append('file', file);
    const { data, error, response } = await client.POST('/api/v1/uploads', {
      params: { query: { context: 'avatar' } },
      body: { file: '' },
      bodySerializer: () => body,
    });
    if (data === undefined) {
      refuse(apiErrorFrom(response.status, error), retryAfterSeconds(response));
      return;
    }
    save({ avatar_id: data.id });
  }

  function onChange(event: ChangeEvent<HTMLInputElement>): void {
    const file = event.currentTarget.files?.[0];
    event.currentTarget.value = '';
    settle();
    if (file === undefined) {
      return;
    }
    if (file.size > maxAvatarBytes) {
      refuse(apiErrorFrom(413, { error: { code: 'payload_too_large', message: '', request_id: '' } }), null);
      return;
    }
    work.track(upload(file)).catch((thrown: unknown) => {
      refuse(asApiError(thrown), null);
    });
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 id={headingId} className="text-heading">
        {t('settings.avatar')}
      </h2>
      {address === null ? (
        <p className="text-text-muted">{t('settings.noAvatar')}</p>
      ) : (
        <img
          src={address}
          alt={t('settings.avatarAlt', { name: me.name })}
          className="size-24 rounded-full object-cover"
        />
      )}
      {saving.kind === 'refused' ? <FormAlert>{errorMessage(t, saving.error, saving.retryAfter)}</FormAlert> : null}
      {saving.kind === 'saved' ? <FormNotice takeFocus>{t('settings.avatarSaved')}</FormNotice> : null}
      <div className="flex flex-wrap items-center gap-3">
        <input
          id={inputId}
          type="file"
          accept={acceptedAvatarFiles}
          onChange={onChange}
          disabled={work.busy}
          className="peer sr-only"
        />
        <label
          htmlFor={inputId}
          className={`${quietButtonClass} cursor-pointer peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-focus peer-disabled:cursor-default peer-disabled:opacity-60`}
        >
          {t('settings.chooseAvatar')}
        </label>
        {address === null ? null : (
          <button
            type="button"
            disabled={work.busy}
            onClick={() => {
              settle();
              save({ avatar_id: null });
            }}
            className="inline-flex min-h-11 items-center px-2 text-small text-accent-text underline disabled:opacity-60"
          >
            {t('settings.removeAvatar')}
          </button>
        )}
      </div>
      <p className="text-small text-text-muted">{t('settings.avatarHint')}</p>
    </section>
  );
}

function DetailsSection({ me, work }: { readonly me: Me; readonly work: PendingWork }): ReactElement {
  const { t } = useTranslation();
  const bioId = useId();
  const bioErrorId = useId();
  const headingId = useId();
  const { saving, save, settle } = useProfileUpdate(work);
  const error = saving.kind === 'refused' ? saving.error : null;
  const bioError = fieldMessage(t, error, 'bio');

  function onSubmit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const name = stringField(data, 'name');
    if (me.role !== 'tutor') {
      save({ name });
      return;
    }
    const subjects = subjectsOf(stringField(data, 'subjects'));
    const subjectsInput = form.elements.namedItem('subjects');
    if (subjectsInput instanceof HTMLInputElement) {
      subjectsInput.value = subjects.join(', ');
    }
    save({ name, bio: stringField(data, 'bio'), subjects });
  }

  return (
    <form
      onSubmit={onSubmit}
      onChange={settle}
      aria-labelledby={headingId}
      className="flex max-w-reading flex-col gap-4"
    >
      <h2 id={headingId} className="text-heading">
        {t('settings.details')}
      </h2>
      {error === null ? null : (
        <FormAlert>
          {error.fields.length > 0
            ? t('errors.validation_failed')
            : errorMessage(t, error, saving.kind === 'refused' ? saving.retryAfter : null)}
        </FormAlert>
      )}
      {saving.kind === 'saved' ? <FormNotice>{t('settings.saved')}</FormNotice> : null}
      <TextField
        label={t('signUp.name')}
        name="name"
        autoComplete="name"
        maxLength={100}
        defaultValue={me.name}
        error={fieldMessage(t, error, 'name')}
      />
      {me.role === 'tutor' ? (
        <>
          <div className="flex flex-col gap-1">
            <label htmlFor={bioId} className="text-label">
              {t('settings.bio')}
            </label>
            <textarea
              id={bioId}
              name="bio"
              maxLength={1000}
              rows={5}
              defaultValue={me.bio ?? ''}
              aria-invalid={bioError === undefined ? undefined : true}
              aria-describedby={bioError === undefined ? undefined : bioErrorId}
              className={`${fieldClass} py-2`}
            />
            {bioError === undefined ? null : (
              <p id={bioErrorId} className="text-small text-danger">
                {bioError}
              </p>
            )}
          </div>
          <TextField
            label={t('settings.subjects')}
            name="subjects"
            autoComplete="off"
            required={false}
            defaultValue={(me.subjects ?? []).join(', ')}
            hint={t('settings.subjectsHint', { count: maxSubjects })}
            error={fieldMessage(t, error, 'subjects')}
          />
        </>
      ) : null}
      <SubmitButton busy={work.busy}>{t('settings.save')}</SubmitButton>
    </form>
  );
}

export function ProfileSettingsPage(): ReactElement {
  const me = useSignedInAccount();
  const work = usePendingWork();
  return (
    <Page>
      <SettingsNav />
      <AvatarSection me={me} work={work} />
      <DetailsSection key={me.id} me={me} work={work} />
    </Page>
  );
}
