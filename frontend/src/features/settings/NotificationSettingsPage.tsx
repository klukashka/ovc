import { useQuery, useQueryClient } from '@tanstack/react-query';
import { type ReactElement, type Ref, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { errorMessage, quietButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { type ApiClient, type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import { type SessionState, useSession } from '../../lib/api/session.ts';
import type { components } from '../../lib/api/schema.ts';
import { SettingsNav } from './SettingsNav.tsx';

type EmailPreferences = components['schemas']['EmailPreferences'];
type EmailPreference = components['schemas']['EmailPreference'];
type EmailGroup = EmailPreference['group'];

interface FailedSave {
  readonly preference: EmailPreference;
  readonly error: ApiError;
}

const emailPreferencesKey = ['users', 'me', 'email-preferences'] as const;

function accountOf(state: SessionState): string | null {
  return state.kind === 'signedIn' ? state.me.id : null;
}

async function fetchEmailPreferences(client: ApiClient): Promise<EmailPreferences> {
  const { data, error, response } = await client.GET('/api/v1/users/me/email-preferences');
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

async function saveEmailPreference(client: ApiClient, preference: EmailPreference): Promise<EmailPreferences> {
  const { data, error, response } = await client.PUT('/api/v1/users/me/email-preferences', {
    body: { groups: [preference] },
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

function withSavedGroup(
  current: EmailPreferences | undefined,
  saved: EmailPreferences,
  group: EmailGroup,
): EmailPreferences {
  if (current === undefined) {
    return saved;
  }
  const answer = saved.groups.find((preference) => preference.group === group);
  return {
    groups: current.groups.map((preference) =>
      preference.group === group && answer !== undefined ? answer : preference,
    ),
  };
}

function EmailSwitch({
  preference,
  saving,
  onToggle,
  ref,
}: {
  readonly preference: EmailPreference;
  readonly saving: boolean;
  readonly onToggle: () => void;
  readonly ref: Ref<HTMLButtonElement>;
}): ReactElement {
  const { t } = useTranslation();
  const labelId = useId();
  const hintId = useId();
  const { group, enabled } = preference;
  return (
    <li className="flex items-center justify-between gap-4 py-3">
      <div className="flex flex-col gap-1">
        <span id={labelId} className="text-strong">
          {t(`emailSettings.groups.${group}`)}
        </span>
        <span id={hintId} className="text-small text-text-muted">
          {t(`emailSettings.hints.${group}`)}
        </span>
      </div>
      <button
        ref={ref}
        type="button"
        role="switch"
        aria-checked={enabled}
        aria-labelledby={labelId}
        aria-describedby={hintId}
        aria-busy={saving}
        aria-disabled={saving}
        onClick={() => {
          if (!saving) {
            onToggle();
          }
        }}
        className="inline-flex min-h-11 min-w-11 shrink-0 items-center justify-center rounded-md aria-disabled:opacity-60"
      >
        <span
          aria-hidden="true"
          className={`flex h-6 w-11 items-center rounded-full border p-0.5 ${
            enabled ? 'justify-end border-accent bg-accent' : 'justify-start border-border-strong bg-hover'
          }`}
        >
          <span className="size-4.5 rounded-full bg-field" />
        </span>
      </button>
    </li>
  );
}

function SaveFailed({
  failure,
  onRetry,
}: {
  readonly failure: FailedSave;
  readonly onRetry: () => void;
}): ReactElement {
  const { t } = useTranslation();
  return (
    <div role="alert" className="flex flex-wrap items-center gap-3">
      <p className="text-danger">
        {t('emailSettings.saveFailed', { group: t(`emailSettings.groups.${failure.preference.group}`) })}{' '}
        {errorMessage(t, failure.error, null)}
      </p>
      <button type="button" onClick={onRetry} className={quietButtonClass}>
        {t('emailSettings.retry')}
      </button>
    </div>
  );
}

function EmailSwitches({ preferences }: { readonly preferences: EmailPreferences }): ReactElement {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const session = useSession();
  const [pending, setPending] = useState<ReadonlyMap<EmailGroup, boolean>>(new Map());
  const [failed, setFailed] = useState<ReadonlyMap<EmailGroup, FailedSave>>(new Map());
  const switches = useRef(new Map<EmailGroup, HTMLButtonElement>());

  function settle(group: EmailGroup): void {
    setPending((current) => new Map([...current].filter(([pendingGroup]) => pendingGroup !== group)));
  }

  function save(preference: EmailPreference): void {
    setFailed((current) => new Map([...current].filter(([failedGroup]) => failedGroup !== preference.group)));
    setPending((current) => new Map(current).set(preference.group, preference.enabled));
    const account = accountOf(session.snapshot());
    saveEmailPreference(client, preference).then(
      (saved) => {
        if (account !== null && accountOf(session.snapshot()) === account) {
          queryClient.setQueryData<EmailPreferences>(emailPreferencesKey, (current) =>
            withSavedGroup(current, saved, preference.group),
          );
        }
        settle(preference.group);
      },
      (thrown: unknown) => {
        settle(preference.group);
        setFailed((current) => new Map(current).set(preference.group, { preference, error: asApiError(thrown) }));
      },
    );
  }

  function retry(failure: FailedSave): void {
    switches.current.get(failure.preference.group)?.focus();
    save(failure.preference);
  }

  return (
    <>
      <ul className="flex flex-col divide-y divide-border">
        {preferences.groups.map((stored) => {
          const wanted = pending.get(stored.group);
          return (
            <EmailSwitch
              key={stored.group}
              preference={{ group: stored.group, enabled: wanted ?? stored.enabled }}
              saving={wanted !== undefined}
              ref={(element) => {
                if (element === null) {
                  switches.current.delete(stored.group);
                } else {
                  switches.current.set(stored.group, element);
                }
              }}
              onToggle={() => {
                save({ group: stored.group, enabled: !stored.enabled });
              }}
            />
          );
        })}
      </ul>
      {preferences.groups.map((stored) => {
        const failure = failed.get(stored.group);
        return failure === undefined ? null : (
          <SaveFailed
            key={stored.group}
            failure={failure}
            onRetry={() => {
              retry(failure);
            }}
          />
        );
      })}
    </>
  );
}

export function NotificationSettingsPage(): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const headingId = useId();
  const query = useQuery({
    queryKey: emailPreferencesKey,
    queryFn: () => fetchEmailPreferences(client),
    retry: false,
  });

  function body(): ReactElement {
    if (query.isPending) {
      return <DelayedLoading />;
    }
    if (query.isError) {
      return (
        <LoadFailed
          error={asApiError(query.error)}
          onRetry={() => {
            void query.refetch();
          }}
        />
      );
    }
    return <EmailSwitches preferences={query.data} />;
  }

  return (
    <Page width="reading">
      <SettingsNav />
      <section aria-labelledby={headingId} className="flex flex-col gap-3">
        <h2 id={headingId} className="text-heading">
          {t('emailSettings.heading')}
        </h2>
        {body()}
        <p className="text-small text-text-muted">{t('emailSettings.required')}</p>
      </section>
    </Page>
  );
}
