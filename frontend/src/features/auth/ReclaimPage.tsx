import { useQuery } from '@tanstack/react-query';
import { type ReactElement, type SyntheticEvent, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import { Avatar } from '../../components/ui/Avatar.tsx';
import { FormAlert, FormNotice, SubmitButton, linkClass, phoneFullWidthClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { type ApiClient, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import { type Me, useSession, useSignedInAccount } from '../../lib/api/session.ts';
import { formatDateTime } from '../../lib/format/dates.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { homeOf } from '../../routes/access.ts';
import { type Decision, type ReclaimList, decisions, linkField, reclaimChoice, requestField } from './reclaimChoice.ts';
import { reclaimTextLoaders } from './reclaimTexts.ts';

type ReclaimTutor = components['schemas']['ReclaimTutor'];
type Outcome = components['schemas']['ReclaimDecided']['outcome'];

const reclaimKey = ['reclaim'] as const;

async function fetchReclaimList(client: ApiClient): Promise<ReclaimList> {
  const { data, error, response } = await client.GET('/api/v1/users/me/reclaim');
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

function ContinueLink({ me }: { readonly me: Me }): ReactElement {
  const { t } = useTranslation();
  return (
    <p>
      <Link to={homeOf(me)} className={linkClass}>
        {t('reclaim.continue')}
      </Link>
    </p>
  );
}

function AccountCheck({ me }: { readonly me: Me }): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  return (
    <section aria-labelledby={headingId} className="flex flex-col items-start gap-2">
      <h2 id={headingId} className="text-heading">
        {t('reclaim.account')}
      </h2>
      <div className="flex items-center gap-3">
        <Avatar name={me.name} avatarId={me.avatar_id} size="large" />
        <p className="text-strong">{me.name}</p>
      </div>
      <p className="text-small text-text-muted">{t('reclaim.accountHint')}</p>
      <Link to="/settings/profile" className={linkClass}>
        {t('reclaim.editAccount')}
      </Link>
    </section>
  );
}

function ItemChoice({ field, tutor }: { readonly field: string; readonly tutor: ReclaimTutor }): ReactElement {
  const { t } = useTranslation();
  return (
    <li className="border-t border-border py-3">
      <fieldset className="grid grid-cols-[auto_1fr] items-center gap-x-3 gap-y-2 tablet:grid-cols-[auto_1fr_auto]">
        <legend className="sr-only">{tutor.name}</legend>
        <Avatar name={tutor.name} avatarId={tutor.avatar_id} />
        <div className="flex min-w-0 flex-col">
          <span aria-hidden="true" dir="auto" className="text-strong wrap-anywhere">
            {tutor.name}
          </span>
          {tutor.subjects.length === 0 ? null : (
            <span className="text-small text-text-muted">{tutor.subjects.join(', ')}</span>
          )}
        </div>
        <div className="col-span-2 flex flex-col gap-2 tablet:col-span-1 tablet:flex-row tablet:gap-4">
          {decisions.map((decision: Decision) => (
            <label
              key={decision}
              className="flex min-h-11 w-full items-center gap-2 rounded-md border border-border px-3 tablet:w-auto tablet:border-0 tablet:px-0"
            >
              <input type="radio" name={field} value={decision} required className="size-4 accent-accent" />
              {t(`reclaim.${decision}`)}
            </label>
          ))}
        </div>
      </fieldset>
    </li>
  );
}

function ChoiceGroup({
  title,
  items,
}: {
  readonly title: string;
  readonly items: readonly { readonly field: string; readonly tutor: ReclaimTutor }[];
}): ReactElement | null {
  const headingId = useId();
  if (items.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-2">
      <h2 id={headingId} className="text-heading">
        {title}
      </h2>
      <ul className="border-b border-border">
        {items.map((item) => (
          <ItemChoice key={item.field} field={item.field} tutor={item.tutor} />
        ))}
      </ul>
    </section>
  );
}

function ChoiceForm({
  list,
  me,
  onSaved,
  onChanged,
}: {
  readonly list: ReclaimList;
  readonly me: Me;
  readonly onSaved: (outcome: Outcome) => void;
  readonly onChanged: () => Promise<unknown>;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const action = useAction();
  const [listChanged, setListChanged] = useState(false);

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const choice = reclaimChoice(list, (field) => {
      const value = data.get(field);
      return typeof value === 'string' ? value : null;
    });
    if (choice === null) {
      return;
    }
    setListChanged(false);
    action.run(async (): Promise<ActionState> => {
      const { data: decided, error, response } = await client.POST('/api/v1/users/me/reclaim', { body: choice });
      if (decided !== undefined) {
        onSaved(decided.outcome);
        return done;
      }
      if (response.status === 404 || response.status === 422) {
        setListChanged(true);
        await onChanged();
      }
      return refusedBy(response, error);
    });
  }

  return (
    <form aria-label={t('page.reclaim')} onSubmit={submit} className="flex flex-col gap-6">
      {listChanged ? <FormAlert>{t('reclaim.changed')}</FormAlert> : <Refusal state={action.state} />}
      {list.choose_by === null ? null : (
        <p>{t('reclaim.deadline', { date: formatDateTime(list.choose_by, me.language, me.timezone) })}</p>
      )}
      <ChoiceGroup
        title={t('reclaim.links')}
        items={list.links.map((link) => ({ field: linkField(link.relationship_id), tutor: link.tutor }))}
      />
      <ChoiceGroup
        title={t('reclaim.requests')}
        items={list.requests.map((request) => ({ field: requestField(request.request_id), tutor: request.tutor }))}
      />
      <p className="text-small text-text-muted">{t('reclaim.dropHint')}</p>
      <p className={phoneFullWidthClass}>
        <SubmitButton busy={action.busy}>{t('reclaim.save')}</SubmitButton>
      </p>
    </form>
  );
}

function ReclaimChoice(): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const client = useApiClient();
  const session = useSession();
  const [saved, setSaved] = useState<{ readonly outcome: Outcome; readonly reloadFailed: boolean } | null>(null);
  const query = useQuery({
    queryKey: reclaimKey,
    queryFn: () => fetchReclaimList(client),
    enabled: saved === null,
    gcTime: 0,
    retry: false,
  });

  function body(): ReactElement {
    if (saved !== null) {
      return (
        <>
          <FormNotice takeFocus>
            {saved.outcome === null ? t('reclaim.saved') : t(`outcome.${saved.outcome}`)}
          </FormNotice>
          {saved.reloadFailed ? <FormAlert>{t('reclaim.reloadFailed')}</FormAlert> : null}
          <ContinueLink me={me} />
        </>
      );
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
    if (query.data === undefined) {
      return <DelayedLoading />;
    }
    if (query.data.links.length === 0 && query.data.requests.length === 0) {
      return (
        <>
          <FormNotice takeFocus>{t('reclaim.nothing')}</FormNotice>
          <ContinueLink me={me} />
        </>
      );
    }
    return (
      <>
        <p>{t('reclaim.explanation')}</p>
        <AccountCheck me={me} />
        <ChoiceForm
          list={query.data}
          me={me}
          onSaved={(outcome) => {
            setSaved({ outcome, reloadFailed: false });
            session.reloadAccount().catch(() => {
              setSaved({ outcome, reloadFailed: true });
            });
          }}
          onChanged={() => query.refetch()}
        />
      </>
    );
  }

  return body();
}

export function ReclaimPage(): ReactElement {
  const texts = useFeatureTexts('reclaim', reclaimTextLoaders);
  return (
    <Page width="reading">
      {texts.ready ? (
        <ReclaimChoice />
      ) : texts.failed ? (
        <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} />
      ) : (
        <DelayedLoading />
      )}
    </Page>
  );
}
