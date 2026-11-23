import { type UseQueryResult, useQuery } from '@tanstack/react-query';
import { type ReactElement, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { Avatar } from '../../components/ui/Avatar.tsx';
import { ConfirmationPrompt } from '../../components/ui/ConfirmationPrompt.tsx';
import {
  DataTable,
  type TableColumn,
  narrowOnlyClass,
  primaryCellClass,
  rowLinkClass,
  secondaryCellClass,
  tableRowClass,
} from '../../components/ui/DataTable.tsx';
import { primaryButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { type GroupSummary, fetchGroups, firstUnusedColour, groupsKey } from './api.ts';
import { GroupDot } from './ColourField.tsx';
import { NewGroupDialog } from './NewGroupDialog.tsx';
import { groupsTextLoaders } from './texts.ts';
import { NextLessonLine } from '../schedule/LessonList.tsx';

function GroupRow({ group }: { readonly group: GroupSummary }): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const hidden = group.member_count - group.members.length;
  const memberCount = t('groups.memberCount', { count: group.member_count });
  const lesson = group.archived_at === null ? <NextLessonLine item={group.next_lesson} me={me} /> : null;
  const archived = group.archived_at === null ? null : <StatusWord tone="muted">{t('groups.archived')}</StatusWord>;
  return (
    <tr data-row={group.id} className={tableRowClass}>
      <th scope="row" className={primaryCellClass}>
        <span className="flex min-w-0 items-center gap-2">
          <GroupDot colour={group.colour} />
          <span className="flex min-w-0 flex-col">
            <Link
              data-row-link=""
              to={`/groups/${encodeURIComponent(group.id)}`}
              dir="auto"
              className={`truncate ${rowLinkClass}`}
            >
              {group.name}
            </Link>
            <span className={`text-small text-text-muted ${narrowOnlyClass}`}>{memberCount}</span>
            {lesson === null ? null : <span className={narrowOnlyClass}>{lesson}</span>}
            {archived === null ? null : <span className={narrowOnlyClass}>{archived}</span>}
          </span>
        </span>
      </th>
      <td className={secondaryCellClass}>
        <span className="flex flex-wrap items-center gap-2">
          <span className="flex items-center gap-1" aria-hidden="true">
            {group.members.map((member) => (
              <Avatar key={member.relationship_id} name={member.display_name} avatarId={member.avatar_id} />
            ))}
            {hidden > 0 ? <span className="text-small text-text-muted">+{hidden}</span> : null}
          </span>
          <span className="text-small text-text-muted">{memberCount}</span>
        </span>
      </td>
      <td className={secondaryCellClass}>{lesson}</td>
      <td className={secondaryCellClass}>{archived}</td>
    </tr>
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
  const columns: readonly TableColumn[] = [
    { key: 'name', label: t('groups.columns.name'), width: 'w-full tablet:w-4/12' },
    { key: 'members', label: t('groups.columns.members'), secondary: true, width: 'w-3/12' },
    { key: 'lesson', label: t('groups.columns.nextLesson'), secondary: true, width: 'w-3/12' },
    { key: 'state', label: t('groups.columns.state'), secondary: true, width: 'w-2/12' },
  ];
  return (
    <DataTable label={t('groups.list')} columns={columns}>
      {query.data.map((group) => (
        <GroupRow key={group.id} group={group} />
      ))}
    </DataTable>
  );
}

export function GroupsPage(): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const client = useApiClient();
  const texts = useFeatureTexts('groups', groupsTextLoaders);
  const [dialogOpen, setDialogOpen] = useState(false);
  const opener = useRef<HTMLButtonElement>(null);
  const query = useQuery({
    queryKey: groupsKey,
    queryFn: () => fetchGroups(client),
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

  if (!texts.ready) {
    return (
      <Page>
        {texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : <DelayedLoading />}
      </Page>
    );
  }

  return (
    <Page action={query.isSuccess && query.data.length > 0 ? addButton : null}>
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
