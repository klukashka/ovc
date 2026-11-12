import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useSearchParams } from 'react-router';

import {
  DataTable,
  type TableColumn,
  cellClass,
  primaryCellClass,
  rowLinkClass,
  secondaryCellClass,
  tableRowClass,
} from '../../components/ui/DataTable.tsx';
import { linkClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { DelayedLoading, LoadFailed, RefetchFailed } from '../../components/ui/LoadState.tsx';
import { useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Page } from '../../components/ui/Page.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { formatDate } from '../../lib/format/dates.ts';
import { fetchGroups, groupsKey } from '../groups/api.ts';
import {
  type IssueFilter,
  type IssueStatus,
  fetchIssuePage,
  isIssueStatus,
  issueAddress,
  issueListKey,
} from './api.ts';
import { ConfirmEmailPrompt, HomeworkTabs, homeworkTextLoaders } from './IssueAssignment.tsx';
import { AudienceLabel, DeadlineText, ProgressText, ToCheckBadge } from './IssueParts.tsx';

const statusTabClass = 'inline-flex min-h-11 items-center rounded-md px-3 text-small';
const currentStatusClass = `${statusTabClass} bg-hover font-medium text-text`;
const otherStatusClass = `${statusTabClass} text-text-muted hover:text-text`;

const statuses: readonly IssueStatus[] = ['active', 'withdrawn'];

function StatusFilter({ current }: { readonly current: IssueStatus }): ReactElement {
  const { t } = useTranslation();
  return (
    <nav aria-label={t('homework.list.filter')}>
      <ul className="flex flex-wrap gap-2">
        {statuses.map((status) => (
          <li key={status}>
            <Link
              to={status === 'active' ? '/homework/issued' : `/homework/issued?status=${status}`}
              aria-current={status === current ? 'page' : undefined}
              className={status === current ? currentStatusClass : otherStatusClass}
            >
              {t(`homework.list.${status}`)}
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}

function IssuedList({ status }: { readonly status: IssueStatus }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const me = useSignedInAccount();
  const filter: IssueFilter = { status };
  const list = useInfiniteQuery({
    queryKey: issueListKey(filter),
    queryFn: ({ pageParam, signal }) => fetchIssuePage(client, filter, pageParam, signal),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    retry: false,
  });
  const groups = useQuery({ queryKey: groupsKey, queryFn: () => fetchGroups(client), retry: false });
  const items = list.data?.pages.flatMap((page) => page.items) ?? [];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLTableSectionElement>(
    items.length,
    ':scope > tr [data-row-link]',
    list.isFetchingNextPage,
  );
  useFocusAfterRemovedRow(
    listRef,
    items.map((item) => item.id),
    '[data-row-link]',
  );
  if (list.data === undefined) {
    return list.isError ? (
      <LoadFailed
        error={asApiError(list.error)}
        onRetry={() => {
          void list.refetch();
        }}
      />
    ) : (
      <DelayedLoading />
    );
  }
  const failure = list.isError ? (
    <RefetchFailed
      error={asApiError(list.error)}
      onRetry={() => {
        void (list.isFetchNextPageError ? list.fetchNextPage() : list.refetch());
      }}
    />
  ) : null;
  if (items.length === 0) {
    return (
      <div className="flex flex-col items-start gap-3">
        {failure}
        {status === 'active' ? (
          <>
            <p className="text-strong">{t('homework.empty.issued')}</p>
            <Link to="/homework/library" className={linkClass}>
              {t('homework.empty.openLibrary')}
            </Link>
          </>
        ) : (
          <p className="text-text-muted">{t('homework.empty.withdrawn')}</p>
        )}
      </div>
    );
  }
  const columns: readonly TableColumn[] = [
    { key: 'assignment', label: t('homework.list.assignment'), width: 'w-1/2 tablet:w-3/12' },
    { key: 'audience', label: t('homework.list.audience'), width: 'w-1/4 tablet:w-2/12' },
    { key: 'issued', label: t('homework.list.issued'), secondary: true, width: 'w-2/12' },
    { key: 'deadline', label: t('homework.list.deadline'), secondary: true, width: 'w-2/12' },
    { key: 'progress', label: t('homework.list.progress'), width: 'w-1/4 tablet:w-3/12' },
  ];
  return (
    <div className="flex flex-col items-start gap-4">
      {failure}
      <DataTable label={t('homework.list.label')} columns={columns} bodyRef={listRef}>
        {items.map((item) => (
          <tr key={item.id} data-row={item.id} className={tableRowClass}>
            <th scope="row" className={primaryCellClass}>
              <span className="flex min-w-0 flex-col gap-0.5">
                <Link data-row-link="" to={issueAddress(item.id)} dir="auto" className={rowLinkClass}>
                  {item.title}
                </Link>
                <span className="text-small text-text-muted tablet:hidden">
                  <DeadlineText deadline={item.deadline_at} />
                </span>
              </span>
            </th>
            <td className={cellClass}>
              <AudienceLabel audience={item.audience} groups={groups.data ?? []} />
            </td>
            <td className={secondaryCellClass}>
              <span className="text-small text-text-muted">{formatDate(item.issued_at, me.language, me.timezone)}</span>
            </td>
            <td className={secondaryCellClass}>
              <span className="text-small">
                <DeadlineText deadline={item.deadline_at} />
              </span>
            </td>
            <td className={cellClass}>
              <span className="flex flex-col gap-1">
                <ProgressText progress={item.progress} full />
                <ToCheckBadge count={item.progress.needs_checking} />
              </span>
            </td>
          </tr>
        ))}
      </DataTable>
      {list.hasNextPage ? (
        <button
          type="button"
          aria-disabled={list.isFetchingNextPage}
          aria-busy={list.isFetchingNextPage}
          onClick={() => {
            if (!list.isFetchingNextPage) {
              loadingMore();
              void list.fetchNextPage();
            }
          }}
          className={quietButtonClass}
        >
          {t('homework.showMore')}
        </button>
      ) : null}
    </div>
  );
}

export function IssuedPage(): ReactElement {
  const texts = useFeatureTexts('homework', homeworkTextLoaders);
  const me = useSignedInAccount();
  const [params] = useSearchParams();
  const requested = params.get('status');
  const status: IssueStatus = isIssueStatus(requested) ? requested : 'active';

  return (
    <Page>
      {texts.ready ? (
        <>
          <HomeworkTabs current="issued" />
          {me.email_verified ? (
            <>
              <StatusFilter current={status} />
              <IssuedList key={status} status={status} />
            </>
          ) : (
            <ConfirmEmailPrompt />
          )}
        </>
      ) : texts.failed ? (
        <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} />
      ) : (
        <DelayedLoading />
      )}
    </Page>
  );
}
