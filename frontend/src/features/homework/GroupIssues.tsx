import {
  type InfiniteData,
  type UseInfiniteQueryResult,
  useInfiniteQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { type ReactElement, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { quietButtonClass } from '../../components/ui/Form.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { DelayedLoading, LoadFailed, RefetchFailed } from '../../components/ui/LoadState.tsx';
import { loadNextPage, useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { type GroupDetail, groupKey } from '../groups/api.ts';
import { type IssueFilter, type IssuePage, fetchIssuePage, issueAddress, issueListKey } from './api.ts';
import { IssueAssignmentButton } from './IssueAssignment.tsx';
import { DeadlineText, ProgressText, ToCheckBadge } from './IssueParts.tsx';

const groupEmptyIssueOpener = 'group-issues-empty';

function useGroupIssues(groupId: string): UseInfiniteQueryResult<InfiniteData<IssuePage>> {
  const client = useApiClient();
  const filter: IssueFilter = { status: 'active', group: groupId };
  return useInfiniteQuery({
    queryKey: issueListKey(filter),
    queryFn: ({ pageParam, signal }) => fetchIssuePage(client, filter, pageParam, signal),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    retry: false,
  });
}

export function GroupMemberNote({ groupId }: { readonly groupId: string }): ReactElement | null {
  const { t } = useTranslation();
  const query = useGroupIssues(groupId);
  if ((query.data?.pages[0]?.items.length ?? 0) === 0) {
    return null;
  }
  return <p className="text-small text-text-muted">{t('homework.groupMemberNote')}</p>;
}

export function GroupIssues({ groupId }: { readonly groupId: string }): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const query = useGroupIssues(groupId);
  const group = useQueryClient().getQueryData<GroupDetail>(groupKey(groupId));
  const items = query.data?.pages.flatMap((page) => page.items) ?? [];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(
    items.length,
    ':scope > li a',
    query.isFetching,
    heading,
  );
  useFocusAfterRemovedRow(
    listRef,
    items.map((item) => item.id),
    'a',
    heading,
  );

  function content(): ReactElement {
    if (query.data === undefined) {
      return query.isError ? (
        <LoadFailed
          error={asApiError(query.error)}
          onRetry={() => {
            void query.refetch();
          }}
        />
      ) : (
        <DelayedLoading />
      );
    }
    return (
      <>
        {query.isError ? (
          <RefetchFailed
            error={asApiError(query.error)}
            onRetry={() => {
              void (query.isFetchNextPageError ? loadNextPage(query) : query.refetch());
            }}
          />
        ) : null}
        {items.length === 0 ? (
          <>
            <p className="text-text-muted">{t('homework.empty.group')}</p>
            {group?.archived_at !== null || group.members.length === 0 ? null : (
              <IssueAssignmentButton
                opener={groupEmptyIssueOpener}
                preset={{ audience: { kind: 'group', id: group.id, name: group.name } }}
              >
                {t('homework.empty.issueToGroup')}
              </IssueAssignmentButton>
            )}
          </>
        ) : null}
      </>
    );
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col items-start gap-3">
      <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {t('homework.sections.assignments')}
      </h2>
      {content()}
      <ul
        ref={listRef}
        aria-label={t('homework.sections.assignments')}
        className={items.length === 0 ? 'hidden' : 'flex w-full flex-col divide-y divide-border border-y border-border'}
      >
        {items.map((item) => (
          <li key={item.id} data-row={item.id} className="flex flex-col gap-1 py-3">
            <Link
              to={issueAddress(item.id)}
              dir="auto"
              className="font-medium text-text underline-offset-2 hover:underline"
            >
              {item.title}
            </Link>
            <span className="text-small text-text-muted">
              <DeadlineText deadline={item.deadline_at} />
            </span>
            <ProgressText progress={item.progress} />
            <ToCheckBadge count={item.progress.needs_checking} />
          </li>
        ))}
      </ul>
      {query.hasNextPage ? (
        <button
          type="button"
          aria-disabled={query.isFetchingNextPage}
          aria-busy={query.isFetchingNextPage}
          onClick={() => {
            if (!query.isFetchingNextPage) {
              loadingMore();
              void loadNextPage(query);
            }
          }}
          className={quietButtonClass}
        >
          {t('homework.showMore')}
        </button>
      ) : null}
    </section>
  );
}
