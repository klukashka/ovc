import { type InfiniteData, type UseInfiniteQueryResult, useInfiniteQuery } from '@tanstack/react-query';
import { type ReactElement, useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { linkClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { DelayedLoading, LoadFailed, RefetchFailed } from '../../components/ui/LoadState.tsx';
import { loadNextPage, useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatDate } from '../../lib/format/dates.ts';
import { type IssueFilter, type IssueItem, type IssuePage, fetchIssuePage, issueListKey } from './api.ts';
import { DeadlineText, ReclaimCue, SubmissionState, pointsText } from './IssueParts.tsx';

const overviewLimit = 5;

function useCardIssues(relationshipId: string): UseInfiniteQueryResult<InfiniteData<IssuePage>> {
  const client = useApiClient();
  const filter: IssueFilter = { status: 'active', student: relationshipId };
  return useInfiniteQuery({
    queryKey: issueListKey(filter),
    queryFn: ({ pageParam, signal }) => fetchIssuePage(client, filter, pageParam, signal),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    retry: false,
  });
}

function isOpen(item: IssueItem): boolean {
  const status = item.submission?.status;
  return status === 'not_started' || status === 'draft' || status === 'returned' || status === 'submitted';
}

function CardIssueState({ item }: { readonly item: IssueItem }): ReactElement | null {
  const { t } = useTranslation();
  const submission = item.submission;
  if (submission === undefined) {
    return null;
  }
  return (
    <span className="flex flex-wrap items-center gap-x-3">
      <SubmissionState status={submission.status} late={submission.late} manual={item.has_manual_questions} />
      {submission.status === 'checked' && submission.final_points !== null ? (
        <span className="text-small">{pointsText(t, submission.final_points, item.max_points)}</span>
      ) : null}
    </span>
  );
}

export function CardHomeworkTab({ relationshipId }: { readonly relationshipId: string }): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const query = useCardIssues(relationshipId);
  const items = query.data?.pages.flatMap((page) => page.items) ?? [];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(
    items.length,
    ':scope > li [data-row-focus]',
    query.isFetching,
  );
  useFocusAfterRemovedRow(
    listRef,
    items.map((item) => item.id),
    '[data-row-focus]',
  );
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
  const failure = query.isError ? (
    <RefetchFailed
      error={asApiError(query.error)}
      onRetry={() => {
        void (query.isFetchNextPageError ? loadNextPage(query) : query.refetch());
      }}
    />
  ) : null;
  return (
    <div className="flex flex-col items-start gap-4">
      {failure}
      {items.length === 0 ? <p className="text-text-muted">{t('homework.empty.card')}</p> : null}
      <ul
        ref={listRef}
        aria-label={t('homework.sections.homework')}
        className={items.length === 0 ? 'hidden' : 'flex w-full flex-col divide-y divide-border border-y border-border'}
      >
        {items.map((item) => (
          <li key={item.id} data-row={item.id} className="flex flex-col gap-1 py-3">
            <span data-row-focus="" tabIndex={-1} dir="auto" className="font-medium outline-none">
              {item.title}
            </span>
            {item.submission?.issued_before_reclaim === true ? <ReclaimCue /> : null}
            <span className="text-small text-text-muted">
              {formatDate(item.issued_at, me.language, me.timezone)}
              {' · '}
              <DeadlineText deadline={item.deadline_at} />
            </span>
            <CardIssueState item={item} />
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
    </div>
  );
}

export function CardOpenAssignments({
  relationshipId,
  homeworkAddress,
}: {
  readonly relationshipId: string;
  readonly homeworkAddress: string;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const query = useCardIssues(relationshipId);
  const open = (query.data?.pages[0]?.items ?? []).filter(isOpen).slice(0, overviewLimit);
  return (
    <section aria-labelledby={headingId} className="flex flex-col items-start gap-3">
      <h2 id={headingId} className="text-heading">
        {t('homework.sections.open')}
      </h2>
      {query.data === undefined ? (
        query.isError ? (
          <LoadFailed
            error={asApiError(query.error)}
            onRetry={() => {
              void query.refetch();
            }}
          />
        ) : (
          <DelayedLoading />
        )
      ) : open.length === 0 ? (
        <p className="text-text-muted">{t('homework.empty.cardOpen')}</p>
      ) : (
        <ul
          aria-label={t('homework.sections.open')}
          className="flex w-full flex-col divide-y divide-border border-y border-border"
        >
          {open.map((item) => (
            <li key={item.id} data-row={item.id} className="flex flex-col gap-1 py-3">
              <span dir="auto" className="font-medium">
                {item.title}
              </span>
              <span className="text-small text-text-muted">
                <DeadlineText deadline={item.deadline_at} />
              </span>
              <CardIssueState item={item} />
            </li>
          ))}
        </ul>
      )}
      <p>
        <Link to={homeworkAddress} className={linkClass}>
          {t('homework.sections.all')}
        </Link>
      </p>
    </section>
  );
}
