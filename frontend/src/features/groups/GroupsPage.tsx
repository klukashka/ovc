import { type UseQueryResult, useQuery } from '@tanstack/react-query';
import { type ReactElement, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { Avatar } from '../../components/ui/Avatar.tsx';
import { ConfirmationPrompt } from '../../components/ui/ConfirmationPrompt.tsx';
import { primaryButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { type GroupSummary, fetchGroups, firstUnusedColour, groupsKey } from './api.ts';
import { GroupDot } from './ColourField.tsx';
import { NewGroupDialog } from './NewGroupDialog.tsx';

function GroupRow({ group }: { readonly group: GroupSummary }): ReactElement {
  const { t } = useTranslation();
  const hidden = group.member_count - group.members.length;
  return (
    <li className="border-t border-border">
      <Link
        to={`/groups/${encodeURIComponent(group.id)}`}
        className="grid min-h-11 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1 px-2 py-3 hover:bg-surface tablet:grid-cols-[minmax(0,2fr)_minmax(0,2fr)_auto]"
      >
        <span className="flex min-w-0 items-center gap-2">
          <GroupDot colour={group.colour} />
          <span className="truncate text-strong">{group.name}</span>
        </span>
        <span className="flex items-center gap-1" aria-hidden="true">
          {group.members.map((member) => (
            <Avatar key={member.relationship_id} name={member.display_name} avatarId={member.avatar_id} />
          ))}
          {hidden > 0 ? <span className="text-small text-text-muted">+{hidden}</span> : null}
        </span>
        <span className="flex flex-col items-start gap-1 tablet:items-end">
          <span className="text-small text-text-muted">{t('groups.memberCount', { count: group.member_count })}</span>
          {group.archived_at === null ? null : <StatusWord tone="muted">{t('groups.archived')}</StatusWord>}
        </span>
      </Link>
    </li>
  );
}

function GroupList({
  query,
  addButton,
}: {
  readonly query: UseQueryResult<readonly GroupSummary[]>;
  readonly addButton: ReactElement | null;
}): ReactElement {
  const { t } = useTranslation();
  if (query.isPending) {
    return <DelayedLoading />;
  }
  if (query.isError) {
    const error = asApiError(query.error);
    if (error.code === 'email_not_verified') {
      return <ConfirmationPrompt reason="groups" />;
    }
    return (
      <LoadFailed
        error={error}
        onRetry={() => {
          void query.refetch();
        }}
      />
    );
  }
  if (query.data.length === 0) {
    return (
      <section className="flex flex-col items-start gap-3">
        <h2 className="text-heading">{t('groups.emptyTitle')}</h2>
        <p className="text-text-muted">{t('groups.emptyBody')}</p>
        {addButton}
      </section>
    );
  }
  return (
    <div className="flex flex-col items-start gap-4">
      {addButton}
      <ul aria-label={t('groups.list')} className="w-full border-b border-border">
        {query.data.map((group) => (
          <GroupRow key={group.id} group={group} />
        ))}
      </ul>
    </div>
  );
}

export function GroupsPage(): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const client = useApiClient();
  const [dialogOpen, setDialogOpen] = useState(false);
  const opener = useRef<HTMLButtonElement>(null);
  const query = useQuery({
    queryKey: groupsKey,
    queryFn: ({ signal }) => fetchGroups(client, signal),
    retry: false,
  });

  const addButton =
    me.state === 'unverified' ? null : (
      <button
        ref={opener}
        type="button"
        onClick={() => {
          setDialogOpen(true);
        }}
        className={primaryButtonClass}
      >
        {t('groups.new')}
      </button>
    );

  return (
    <Page wide>
      <GroupList query={query} addButton={addButton} />
      <NewGroupDialog
        open={dialogOpen}
        defaultColour={firstUnusedColour(query.data ?? [])}
        returnFocus={opener}
        onClose={() => {
          setDialogOpen(false);
        }}
      />
    </Page>
  );
}
