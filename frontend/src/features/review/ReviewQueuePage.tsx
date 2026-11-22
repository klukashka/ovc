import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { type ReactElement, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate, useSearchParams } from 'react-router';

import {
  DataTable,
  type TableColumn,
  narrowOnlyClass,
  primaryCellClass,
  rowLinkClass,
  secondaryCellClass,
  tableRowClass,
} from '../../components/ui/DataTable.tsx';
import {
  FormAlert,
  SelectChevron,
  linkClass,
  quietButtonClass,
  selectClass,
  textButtonClass,
} from '../../components/ui/Form.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { DelayedLoading, LoadFailed, RefetchFailed } from '../../components/ui/LoadState.tsx';
import { loadNextPage, useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Page } from '../../components/ui/Page.tsx';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatDate, formatDateTime } from '../../lib/format/dates.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { fetchGroups, groupsKey } from '../groups/api.ts';
import { fetchIssue, issueKey } from '../homework/api.ts';
import { ConfirmEmailPrompt, HomeworkTabs, homeworkTextLoaders } from '../homework/IssueAssignment.tsx';
import { ReclaimCue } from '../homework/IssueParts.tsx';
import { fetchStudentCard, studentCardKey } from '../students/api.ts';
import { type QueueItem, fetchQueuePage, queueAddress, queuePageSize, reviewAddress } from './api.ts';
import { type QueueFilter, queueKey } from './keys.ts';

function countText(count: number): string {
  return count > 99 ? '99+' : String(count);
}

function GroupChoice({
  filter,
  focusRequest,
}: {
  readonly filter: QueueFilter;
  readonly focusRequest: number;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const navigate = useNavigate();
  const fieldId = useId();
  const select = useRef<HTMLSelectElement>(null);
  const groups = useQuery({ queryKey: groupsKey, queryFn: () => fetchGroups(client), retry: false });
  useEffect(() => {
    if (focusRequest > 0) {
      select.current?.focus();
    }
  }, [focusRequest]);
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={fieldId} className="text-label text-text-muted">
        {t('homework.queue.group')}
      </label>
      <span className="relative w-64 max-w-full">
        <select
          ref={select}
          id={fieldId}
          value={filter.group ?? ''}
          onChange={(event) => {
            const group = event.currentTarget.value;
            void navigate(queueAddress({ ...filter, group: group === '' ? null : group }));
          }}
          className={selectClass}
        >
          <option value="">{t('homework.queue.allGroups')}</option>
          {(groups.data ?? []).map((group) => (
            <option key={group.id} value={group.id}>
              {group.name}
            </option>
          ))}
        </select>
        <SelectChevron />
      </span>
    </div>
  );
}

function FilterLabel({ filter }: { readonly filter: QueueFilter }): ReactElement | null {
  const { t } = useTranslation();
  const client = useApiClient();
  const issue = useQuery({
    queryKey: issueKey(filter.issue ?? ''),
    queryFn: ({ signal }) => fetchIssue(client, filter.issue ?? '', signal),
    enabled: filter.issue !== null,
    retry: false,
  });
  const card = useQuery({
    queryKey: studentCardKey(filter.student ?? ''),
    queryFn: () => fetchStudentCard(client, filter.student ?? ''),
    enabled: filter.student !== null,
    retry: false,
  });
  return (
    <span className="flex flex-col gap-0.5">
      {filter.issue === null || issue.data === undefined ? null : (
        <span>{t('homework.queue.assignment', { title: issue.data.title })}</span>
      )}
      {filter.student === null || card.data === undefined ? null : (
        <span>{t('homework.queue.student', { name: card.data.display_name })}</span>
      )}
    </span>
  );
}

function SubmittedCell({ item }: { readonly item: QueueItem }): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  return (
    <span className="flex flex-col text-small">
      {item.submitted_at === null ? null : <span>{formatDateTime(item.submitted_at, me.language, me.timezone)}</span>}
      {item.first_submitted_at === null || item.first_submitted_at === item.submitted_at ? null : (
        <span className="text-text-muted">
          {t('homework.queue.first', { date: formatDate(item.first_submitted_at, me.language, me.timezone) })}
        </span>
      )}
    </span>
  );
}

function QueueBadges({ item }: { readonly item: QueueItem }): ReactElement {
  const { t } = useTranslation();
  return (
    <span className="inline-flex flex-wrap gap-x-3 gap-y-1">
      {item.late ? <StatusWord tone="warning">{t('homework.states.late')}</StatusWord> : null}
      {item.has_draft ? <StatusWord tone="info">{t('homework.states.reviewInProgress')}</StatusWord> : null}
    </span>
  );
}

function QueueTable({ filter }: { readonly filter: QueueFilter }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const list = useInfiniteQuery({
    queryKey: queueKey(filter),
    queryFn: ({ pageParam, signal }) => fetchQueuePage(client, filter, queuePageSize, pageParam, signal),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    retry: false,
  });
  const items = list.data?.pages.flatMap((page) => page.items) ?? [];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLTableSectionElement>(
    items.length,
    ':scope > tr [data-row-link]',
    list.isFetching,
  );
  useFocusAfterRemovedRow(
    listRef,
    items.map((item) => item.submission_id),
    '[data-row-link]',
    heading,
  );
  const error = list.isError ? asApiError(list.error) : null;
  if (error !== null && list.data === undefined && (error.status === 404 || error.status === 422)) {
    return (
      <div className="flex flex-col items-start gap-2">
        <FormAlert>{t('homework.queue.refused')}</FormAlert>
        <Link to="/homework/review" className={linkClass}>
          {t('homework.queue.allWork')}
        </Link>
      </div>
    );
  }
  if (list.data === undefined) {
    return error !== null ? (
      <LoadFailed
        error={error}
        onRetry={() => {
          void list.refetch();
        }}
      />
    ) : (
      <DelayedLoading />
    );
  }
  const count = list.data.pages[0]?.count ?? 0;
  const columns: readonly TableColumn[] = [
    { key: 'student', label: t('homework.issuePage.student'), width: 'w-full tablet:w-3/12' },
    { key: 'assignment', label: t('homework.list.assignment'), secondary: true, width: 'w-4/12' },
    { key: 'submitted', label: t('homework.queue.submitted'), secondary: true, width: 'w-3/12' },
    { key: 'state', label: t('homework.issuePage.state'), secondary: true, width: 'w-2/12' },
  ];
  return (
    <section aria-labelledby={headingId} className="flex flex-col items-start gap-4">
      <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {t('homework.queue.heading')} <span className="tabular text-text-muted">{countText(count)}</span>
      </h2>
      {error === null ? null : (
        <RefetchFailed
          error={error}
          onRetry={() => {
            void (list.isFetchNextPageError ? loadNextPage(list) : list.refetch());
          }}
        />
      )}
      {items.length === 0 ? <p className="text-strong">{t('homework.queue.empty')}</p> : null}
      <div className={items.length === 0 ? 'hidden' : 'w-full'}>
        <DataTable label={t('homework.queue.label')} columns={columns} bodyRef={listRef}>
          {items.map((item) => (
            <tr key={item.submission_id} data-row={item.submission_id} className={tableRowClass}>
              <th scope="row" className={primaryCellClass}>
                <span className="flex min-w-0 flex-col gap-0.5">
                  <Link
                    data-row-link=""
                    to={reviewAddress(item.submission_id, filter)}
                    dir="auto"
                    className={rowLinkClass}
                  >
                    {item.student.display_name}
                  </Link>
                  {item.issued_before_reclaim ? <ReclaimCue /> : null}
                  <span className={`flex flex-col gap-0.5 text-small text-text-muted ${narrowOnlyClass}`}>
                    <span dir="auto">{item.title}</span>
                    <SubmittedCell item={item} />
                    <QueueBadges item={item} />
                  </span>
                </span>
              </th>
              <td className={secondaryCellClass}>
                <span dir="auto">{item.title}</span>
              </td>
              <td className={secondaryCellClass}>
                <SubmittedCell item={item} />
              </td>
              <td className={secondaryCellClass}>
                <QueueBadges item={item} />
              </td>
            </tr>
          ))}
        </DataTable>
      </div>
      {list.hasNextPage ? (
        <button
          type="button"
          aria-disabled={list.isFetchingNextPage}
          aria-busy={list.isFetchingNextPage}
          onClick={() => {
            if (!list.isFetchingNextPage) {
              loadingMore();
              void loadNextPage(list);
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

function ReviewQueue(): ReactElement {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const filter: QueueFilter = {
    issue: params.get('issue'),
    group: params.get('group'),
    student: params.get('student'),
  };
  const [focusGroup, setFocusGroup] = useState(0);
  const filtered = filter.issue !== null || filter.student !== null;
  return (
    <>
      <div className="flex flex-wrap items-end gap-x-6 gap-y-3">
        <GroupChoice filter={filter} focusRequest={focusGroup} />
        {filtered ? (
          <div className="flex flex-wrap items-center gap-3">
            <FilterLabel filter={filter} />
            <button
              type="button"
              onClick={() => {
                void navigate(queueAddress({ issue: null, student: null, group: filter.group }));
                setFocusGroup((previous) => previous + 1);
              }}
              className={textButtonClass}
            >
              {t('homework.queue.clear')}
            </button>
          </div>
        ) : null}
      </div>
      <QueueTable key={`${filter.issue ?? ''}:${filter.group ?? ''}:${filter.student ?? ''}`} filter={filter} />
    </>
  );
}

export function ReviewQueuePage(): ReactElement {
  const texts = useFeatureTexts('homework', homeworkTextLoaders);
  const me = useSignedInAccount();
  return (
    <Page>
      {texts.ready ? (
        <>
          <HomeworkTabs current="review" />
          {me.email_verified ? <ReviewQueue /> : <ConfirmEmailPrompt />}
        </>
      ) : texts.failed ? (
        <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} />
      ) : (
        <DelayedLoading />
      )}
    </Page>
  );
}
