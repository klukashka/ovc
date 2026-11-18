import { type UseQueryResult, useQuery } from '@tanstack/react-query';
import { type ReactElement, type ReactNode, type RefObject, useEffect, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { useScheduler } from '../../app/clock.ts';
import type { Scheduler } from '../../lib/api/session.ts';
import { linkClass } from '../../components/ui/Form.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { DelayedLoading, LoadFailed, RefetchFailed } from '../../components/ui/LoadState.tsx';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatDateTime } from '../../lib/format/dates.ts';
import { relativeHint } from '../../lib/format/relative.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { useNow } from '../assignment/parts.tsx';
import { fetchGroups, groupsKey } from '../groups/api.ts';
import { type IssueItem, type IssuePage, issueAddress, issuesKey } from '../homework/api.ts';
import { AudienceLabel } from '../homework/IssueParts.tsx';
import { type QueuePage, fetchQueuePage, reviewAddress } from '../review/api.ts';
import { queuePanelKey } from '../review/keys.ts';
import { homeworkPanelTextLoaders } from './panelTexts.ts';

const panelSize = 5;
const deadlineWindowMilliseconds = 3 * 86_400_000;
const noFilter = { issue: null, group: null, student: null };
const deadlineMarginMilliseconds = 1_000;
const passedDeadlineRetryMilliseconds = 30_000;
const passedDeadlineRetryCapMilliseconds = 600_000;

function earliestDeadline(items: readonly IssueItem[]): string | null {
  let earliest: string | null = null;
  for (const item of items) {
    if (item.deadline_at !== null && (earliest === null || Date.parse(item.deadline_at) < Date.parse(earliest))) {
      earliest = item.deadline_at;
    }
  }
  return earliest;
}

interface DeadlineRefetch {
  readonly deadline: string;
  readonly delay: number;
  readonly afterDeadline: boolean;
}

function nextDeadlineRefetch(deadline: string, now: number, previous: DeadlineRefetch | null): DeadlineRefetch {
  const due = Date.parse(deadline) - now;
  if (due > 0) {
    return { deadline, delay: due + deadlineMarginMilliseconds, afterDeadline: false };
  }
  if (previous?.deadline !== deadline || !previous.afterDeadline) {
    return { deadline, delay: deadlineMarginMilliseconds, afterDeadline: true };
  }
  const delay = Math.min(
    Math.max(previous.delay * 2, passedDeadlineRetryMilliseconds),
    passedDeadlineRetryCapMilliseconds,
  );
  return { deadline, delay, afterDeadline: true };
}

function useRefetchAtEarliestDeadline(query: UseQueryResult<IssuePage>, scheduler: Scheduler): void {
  const deadline = earliestDeadline(query.data?.items ?? []);
  const previous = useRef<DeadlineRefetch | null>(null);
  const { refetch, dataUpdatedAt: loadedAt, errorUpdatedAt: failedAt } = query;
  useEffect(() => {
    if (deadline === null) {
      previous.current = null;
      return undefined;
    }
    const next = nextDeadlineRefetch(deadline, scheduler.wallClock(), previous.current);
    previous.current = next;
    return scheduler.schedule(next.delay, () => {
      void refetch();
    });
  }, [deadline, loadedAt, failedAt, refetch, scheduler]);
}

function Panel<Data>({
  title,
  count,
  query,
  rowKeys,
  children,
}: {
  readonly title: string;
  readonly count: ReactNode;
  readonly query: UseQueryResult<Data>;
  readonly rowKeys: readonly string[];
  readonly children: (listRef: RefObject<HTMLUListElement | null>) => ReactNode;
}): ReactElement {
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  useFocusAfterRemovedRow(listRef, rowKeys, '[data-row-link]', heading);
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {title} {count}
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
      ) : (
        <>
          {query.isError ? (
            <RefetchFailed
              error={asApiError(query.error)}
              onRetry={() => {
                void query.refetch();
              }}
            />
          ) : null}
          {children(listRef)}
        </>
      )}
    </section>
  );
}

function NeedsChecking(): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const me = useSignedInAccount();
  const query = useQuery<QueuePage>({
    queryKey: queuePanelKey(panelSize),
    queryFn: ({ signal }) => fetchQueuePage(client, noFilter, panelSize, null, signal),
    retry: false,
  });
  const items = query.data?.items ?? [];
  const count = query.data?.count ?? 0;
  return (
    <Panel
      title={t('homeworkPanels.needsChecking')}
      count={
        query.data === undefined ? null : (
          <span className="tabular text-text-muted">{count > 99 ? '99+' : String(count)}</span>
        )
      }
      query={query}
      rowKeys={items.map((item) => item.submission_id)}
    >
      {(listRef) => (
        <>
          {items.length === 0 ? <p className="text-text-muted">{t('homeworkPanels.nothingToCheck')}</p> : null}
          <ul ref={listRef} className={items.length === 0 ? 'hidden' : 'flex flex-col border-b border-border'}>
            {items.map((item) => (
              <li
                key={item.submission_id}
                data-row={item.submission_id}
                className="flex flex-col gap-1 border-t border-border py-3"
              >
                <Link
                  data-row-link=""
                  to={reviewAddress(item.submission_id, noFilter)}
                  dir="auto"
                  className={`${linkClass} min-h-0 break-words`}
                >
                  {item.student.display_name}
                </Link>
                <span dir="auto" className="break-words">
                  {item.title}
                </span>
                {item.issued_before_reclaim ? (
                  <span className="text-small text-text-muted">{t('homeworkPanels.cue')}</span>
                ) : null}
                <span className="flex flex-wrap items-center gap-x-3 gap-y-1 text-small">
                  {item.submitted_at === null ? null : (
                    <span className="text-text-muted">
                      {formatDateTime(item.submitted_at, me.language, me.timezone)}
                    </span>
                  )}
                  {item.late ? <StatusWord tone="warning">{t('homeworkPanels.late')}</StatusWord> : null}
                  {item.has_draft ? <StatusWord tone="info">{t('homeworkPanels.reviewInProgress')}</StatusWord> : null}
                </span>
              </li>
            ))}
          </ul>
          <p>
            <Link to="/homework/review" className={linkClass}>
              {t('homeworkPanels.openQueue')}
            </Link>
          </p>
        </>
      )}
    </Panel>
  );
}

function UpcomingDeadlines(): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const me = useSignedInAccount();
  const scheduler = useScheduler();
  const query = useQuery<IssuePage>({
    queryKey: [...issuesKey, 'deadlines', panelSize],
    queryFn: async ({ signal }) => {
      const bound = new Date(scheduler.wallClock() + deadlineWindowMilliseconds).toISOString();
      const { data, error, response } = await client.GET('/api/v1/issues', {
        params: { query: { status: 'active', deadline_before: bound, limit: panelSize } },
        signal,
      });
      if (data === undefined) {
        throw apiErrorFrom(response.status, error);
      }
      return data;
    },
    retry: false,
  });
  useRefetchAtEarliestDeadline(query, scheduler);
  const groups = useQuery({ queryKey: groupsKey, queryFn: () => fetchGroups(client), retry: false });
  const items = query.data?.items ?? [];
  const now = useNow();
  return (
    <Panel title={t('homeworkPanels.deadlines')} count={null} query={query} rowKeys={items.map((item) => item.id)}>
      {(listRef) => (
        <>
          {items.length === 0 ? <p className="text-text-muted">{t('homeworkPanels.noDeadlines')}</p> : null}
          <ul ref={listRef} className={items.length === 0 ? 'hidden' : 'flex flex-col border-b border-border'}>
            {items.map((item) => (
              <li key={item.id} data-row={item.id} className="flex flex-col gap-1 border-t border-border py-3">
                <Link
                  data-row-link=""
                  to={issueAddress(item.id)}
                  dir="auto"
                  className={`${linkClass} min-h-0 break-words`}
                >
                  {item.title}
                </Link>
                <span className="text-small">
                  <AudienceLabel audience={item.audience} groups={groups.data ?? []} />
                </span>
                {item.deadline_at === null ? null : (
                  <span className="text-small text-text-muted">
                    {formatDateTime(item.deadline_at, me.language, me.timezone)} (
                    {relativeHint(item.deadline_at, me.language, now)})
                  </span>
                )}
                <span className="text-small">
                  {t('homeworkPanels.submittedProgress', {
                    submitted: item.progress.submitted,
                    total: item.progress.students,
                  })}
                </span>
              </li>
            ))}
          </ul>
          <p>
            <Link to="/homework/issued" className={linkClass}>
              {t('homeworkPanels.allIssued')}
            </Link>
          </p>
        </>
      )}
    </Panel>
  );
}

function PanelTexts({ children }: { readonly children: ReactNode }): ReactElement {
  const texts = useFeatureTexts('homeworkPanels', homeworkPanelTextLoaders);
  if (!texts.ready) {
    return texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : <DelayedLoading />;
  }
  return <>{children}</>;
}

export function NeedsCheckingPanel(): ReactElement {
  return (
    <PanelTexts>
      <NeedsChecking />
    </PanelTexts>
  );
}

export function UpcomingDeadlinesPanel(): ReactElement {
  return (
    <PanelTexts>
      <UpcomingDeadlines />
    </PanelTexts>
  );
}
