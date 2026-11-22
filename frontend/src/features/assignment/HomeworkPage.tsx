import { type UseInfiniteQueryResult, type InfiniteData, useInfiniteQuery } from '@tanstack/react-query';
import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useSearchParams } from 'react-router';

import {
  DataTable,
  type TableColumn,
  narrowOnlyClass,
  primaryCellClass,
  rowLinkClass,
  secondaryCellClass,
  tableRowClass,
} from '../../components/ui/DataTable.tsx';
import { linkClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { DelayedLoading, LoadFailed, RefetchFailed } from '../../components/ui/LoadState.tsx';
import { loadNextPage, useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Page } from '../../components/ui/Page.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatDate } from '../../lib/format/dates.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { TutorFilter } from '../home/TutorFilter.tsx';
import { currentTutors } from '../tutors/api.ts';
import { useMyTutors } from '../tutors/TutorList.tsx';
import {
  type AssignmentItem,
  type AssignmentPage,
  type AssignmentStatus,
  assignmentAddress,
  assignmentListKey,
  assignmentPageSize,
  assignmentStatuses,
  fetchAssignmentPage,
  isAssignmentStatus,
} from './api.ts';
import { DeadlineHint, ReclaimCue, StateLabel, TutorName, useNow } from './parts.tsx';
import { assignmentTextLoaders } from './texts.ts';

const statusTabClass = 'inline-flex min-h-11 items-center rounded-md px-3 text-small';
const currentStatusClass = `${statusTabClass} bg-hover font-medium text-text`;
const otherStatusClass = `${statusTabClass} text-text-muted hover:text-text`;

type ListQuery = UseInfiniteQueryResult<InfiniteData<AssignmentPage>>;

function listAddress(status: AssignmentStatus, tutor: string | null): string {
  const params = new URLSearchParams();
  if (status !== 'todo') {
    params.set('status', status);
  }
  if (tutor !== null) {
    params.set('tutor', tutor);
  }
  const search = params.toString();
  return search === '' ? '/homework' : `/homework?${search}`;
}

function StatusTabs({
  current,
  tutor,
  closedCount,
}: {
  readonly current: AssignmentStatus;
  readonly tutor: string | null;
  readonly closedCount: number;
}): ReactElement {
  const { t } = useTranslation();
  const shown = assignmentStatuses.filter((status) => status !== 'closed' || closedCount > 0 || current === 'closed');
  return (
    <nav aria-label={t('assignment.list.tabs')}>
      <ul className="flex flex-wrap gap-2">
        {shown.map((status) => (
          <li key={status}>
            <Link
              to={listAddress(status, tutor)}
              aria-current={status === current ? 'page' : undefined}
              className={status === current ? currentStatusClass : otherStatusClass}
            >
              {t(`assignment.list.tab.${status}`)}
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}

function WhenCell({
  item,
  status,
}: {
  readonly item: AssignmentItem;
  readonly status: AssignmentStatus;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const now = useNow();
  if (status === 'submitted' && item.submitted_at !== null) {
    return (
      <span>{t('assignment.list.submittedOn', { date: formatDate(item.submitted_at, me.language, me.timezone) })}</span>
    );
  }
  if (status === 'checked' && item.checked_at !== null) {
    return (
      <span>{t('assignment.list.checkedOn', { date: formatDate(item.checked_at, me.language, me.timezone) })}</span>
    );
  }
  return <DeadlineHint deadline={item.deadline_at} now={now} />;
}

function AssignmentRow({
  item,
  status,
}: {
  readonly item: AssignmentItem;
  readonly status: AssignmentStatus;
}): ReactElement {
  const { t } = useTranslation();
  const now = useNow();
  return (
    <tr data-row={item.submission_id} className={tableRowClass}>
      <th scope="row" className={primaryCellClass}>
        <span className="flex min-w-0 flex-col gap-0.5">
          <Link data-row-link="" to={assignmentAddress(item.submission_id)} dir="auto" className={rowLinkClass}>
            {item.title}
          </Link>
          {item.issued_before_reclaim ? <ReclaimCue /> : null}
          {status === 'checked' && item.feedback_excerpt !== null ? (
            <span dir="auto" className="text-small break-words text-text-muted">
              {item.feedback_excerpt}
            </span>
          ) : null}
          {status === 'checked' && item.has_corrected_files ? (
            <span className="text-small text-text-muted">{t('assignment.correctedFile')}</span>
          ) : null}
          <span className={`flex flex-col gap-0.5 text-small text-text-muted ${narrowOnlyClass}`}>
            <TutorName name={item.tutor.name} colour={item.tutor.colour} />
            <WhenCell item={item} status={status} />
            <StateLabel status={item.status} deadline={item.deadline_at} late={item.late} now={now} />
          </span>
        </span>
      </th>
      <td className={secondaryCellClass}>
        <TutorName name={item.tutor.name} colour={item.tutor.colour} />
      </td>
      <td className={secondaryCellClass}>
        <StateLabel status={item.status} deadline={item.deadline_at} late={item.late} now={now} />
      </td>
      <td className={secondaryCellClass}>
        <span className="text-small">
          <WhenCell item={item} status={status} />
        </span>
      </td>
    </tr>
  );
}

function AssignmentList({
  status,
  list,
}: {
  readonly status: AssignmentStatus;
  readonly list: ListQuery;
}): ReactElement {
  const { t } = useTranslation();
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
        void (list.isFetchNextPageError ? loadNextPage(list) : list.refetch());
      }}
    />
  ) : null;
  if (items.length === 0) {
    return (
      <div className="flex flex-col items-start gap-3">
        {failure}
        <p className="text-strong">{t(`assignment.empty.${status}`)}</p>
      </div>
    );
  }
  const columns: readonly TableColumn[] = [
    { key: 'assignment', label: t('assignment.list.assignment'), width: 'w-full tablet:w-5/12' },
    { key: 'tutor', label: t('assignment.list.tutor'), secondary: true, width: 'w-2/12' },
    { key: 'state', label: t('assignment.list.state'), secondary: true, width: 'w-2/12' },
    { key: 'when', label: t('assignment.list.when'), secondary: true, width: 'w-3/12' },
  ];
  return (
    <div className="flex flex-col items-start gap-4">
      {failure}
      <DataTable label={t('assignment.list.label')} columns={columns} bodyRef={listRef}>
        {items.map((item) => (
          <AssignmentRow key={item.submission_id} item={item} status={status} />
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
              void loadNextPage(list);
            }
          }}
          className={quietButtonClass}
        >
          {t('assignment.list.showMore')}
        </button>
      ) : null}
    </div>
  );
}

function refusedTutor(error: unknown): boolean {
  const refusal = asApiError(error);
  return refusal.code === 'not_found' || refusal.code === 'validation_failed';
}

function HomeworkLists({
  status,
  tutor,
}: {
  readonly status: AssignmentStatus;
  readonly tutor: string | null;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const [, setParams] = useSearchParams();
  const tutorsQuery = useMyTutors();
  const current = tutorsQuery.data === undefined ? [] : currentTutors(tutorsQuery.data);
  const known = tutor === null || current.some((entry) => entry.relationship_id === tutor);
  const unknownTutor = tutor !== null && tutorsQuery.data !== undefined && !known;
  const list = useInfiniteQuery({
    queryKey: assignmentListKey(status, tutor, assignmentPageSize),
    queryFn: ({ pageParam }) => fetchAssignmentPage(client, status, tutor, assignmentPageSize, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    enabled: tutor === null || (tutorsQuery.data !== undefined && known),
    retry: false,
  });
  const unavailable = unknownTutor || (tutor !== null && list.isError && refusedTutor(list.error));
  const waitingForTutors = tutor !== null && tutorsQuery.data === undefined;
  const closedCount = list.data?.pages[0]?.counts.closed ?? 0;
  return (
    <>
      <StatusTabs current={status} tutor={unavailable ? null : tutor} closedCount={closedCount} />
      {current.length >= 2 ? (
        <TutorFilter
          tutors={current}
          chosen={tutor}
          onChoose={(choice) => {
            if (choice !== tutor) {
              setParams(
                new URLSearchParams([
                  ...(status === 'todo' ? [] : [['status', status]]),
                  ...(choice === null ? [] : [['tutor', choice]]),
                ]),
              );
            }
          }}
        />
      ) : null}
      {unavailable ? (
        <div className="flex flex-col items-start gap-2">
          <p>{t('assignment.list.tutorUnavailable')}</p>
          <Link to={listAddress(status, null)} className={linkClass}>
            {t('assignment.list.allTutors')}
          </Link>
        </div>
      ) : waitingForTutors ? (
        tutorsQuery.isError ? (
          <LoadFailed
            error={asApiError(tutorsQuery.error)}
            onRetry={() => {
              void tutorsQuery.refetch();
            }}
          />
        ) : (
          <DelayedLoading />
        )
      ) : (
        <AssignmentList key={`${status}:${tutor ?? ''}`} status={status} list={list} />
      )}
    </>
  );
}

export function HomeworkPage(): ReactElement {
  const texts = useFeatureTexts('assignment', assignmentTextLoaders);
  const [params] = useSearchParams();
  const requested = params.get('status');
  const status: AssignmentStatus = isAssignmentStatus(requested) ? requested : 'todo';
  const tutor = params.get('tutor');
  return (
    <Page>
      {texts.ready ? (
        <HomeworkLists status={status} tutor={tutor} />
      ) : texts.failed ? (
        <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} />
      ) : (
        <DelayedLoading />
      )}
    </Page>
  );
}
