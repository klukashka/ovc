import { keepPreviousData, useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { type ReactElement, type SyntheticEvent, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router';

import { Avatar } from '../../components/ui/Avatar.tsx';
import { ConfirmationPrompt } from '../../components/ui/ConfirmationPrompt.tsx';
import {
  DataTable,
  type TableColumn,
  cellClass,
  narrowOnlyClass,
  primaryCellClass,
  rowLinkClass,
  rowLinkSelector,
  secondaryCellClass,
  tableRowClass,
} from '../../components/ui/DataTable.tsx';
import {
  SelectChevron,
  fieldClass,
  linkClass,
  primaryButtonClass,
  quietButtonClass,
  selectClass,
  stringField,
} from '../../components/ui/Form.tsx';
import { LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { loadNextPage, useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Page } from '../../components/ui/Page.tsx';
import { RouteTabs } from '../../components/ui/Tabs.tsx';
import { useAddressValue } from '../../components/ui/serverValue.ts';
import { type ApiError, asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { fetchGroups, groupsKey } from '../groups/api.ts';
import { NextLessonLine } from '../schedule/LessonList.tsx';
import { fetchTutorCode, tutorCodeKey } from '../settings/tutorCode.ts';
import { AddStudentDialog } from './AddStudentDialog.tsx';
import {
  type StudentCardSummary,
  type StudentListState,
  fetchStudentsPage,
  hasNotice,
  isStudentListState,
  studentListKey,
  studentListStates,
  withdrawnNotice,
} from './api.ts';
import { GroupDots } from './GroupDots.tsx';
import { JoinRequests, useJoinRequestsSettled } from './JoinRequests.tsx';
import { StateBadge } from './StateBadge.tsx';

interface ListView {
  readonly state: StudentListState;
  readonly search: string;
  readonly group: string;
}

function listQuery(view: ListView): Record<string, string> {
  return {
    state: view.state,
    ...(view.group === '' ? {} : { group: view.group }),
    ...(view.search === '' ? {} : { q: view.search }),
  };
}

function listAddress(view: ListView): string {
  return `/students?${new URLSearchParams(listQuery(view)).toString()}`;
}

function Tabs({ view }: { readonly view: ListView }): ReactElement {
  const { t } = useTranslation();
  return (
    <RouteTabs
      label={t('students.lists')}
      inFilterRow
      current={view.state}
      tabs={studentListStates.map((state) => ({
        key: state,
        to: listAddress({ ...view, state }),
        label: t(`students.tabs.${state}`),
      }))}
    />
  );
}

function GroupFilter({ group }: { readonly group: string }): ReactElement | null {
  const { t } = useTranslation();
  const client = useApiClient();
  const me = useSignedInAccount();
  const selectId = useId();
  const select = useAddressValue<HTMLSelectElement>(group);
  const groups = useQuery({
    queryKey: groupsKey,
    queryFn: () => fetchGroups(client),
    enabled: me.state === 'active',
    retry: false,
  });
  if (!groups.isSuccess || (groups.data.length === 0 && group === '')) {
    return null;
  }
  return (
    <div className="flex w-full flex-col gap-1 tablet:w-auto tablet:min-w-48">
      <label htmlFor={selectId} className="text-label">
        {t('students.group')}
      </label>
      <div className="relative">
        <select ref={select} id={selectId} name="group" defaultValue={group} className={selectClass}>
          <option value="">{t('students.allGroups')}</option>
          {groups.data.map((option) => (
            <option key={option.id} value={option.id}>
              {option.archived_at === null ? option.name : t('students.archivedGroup', { name: option.name })}
            </option>
          ))}
        </select>
        <SelectChevron />
      </div>
    </div>
  );
}

function SearchForm({ view }: { readonly view: ListView }): ReactElement {
  const { t } = useTranslation();
  const [, setSearchParams] = useSearchParams();
  const inputId = useId();
  const searchField = useAddressValue<HTMLInputElement>(view.search);
  const filtered = view.search !== '' || view.group !== '';

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const group = data.get('group');
    setSearchParams(
      listQuery({
        state: view.state,
        search: stringField(data, 'q').trim(),
        group: typeof group === 'string' ? group : view.group,
      }),
    );
  }

  return (
    <form
      role="search"
      aria-label={t('students.search')}
      onSubmit={submit}
      className="flex flex-1 flex-wrap items-end gap-3"
    >
      <div className="flex w-full flex-col gap-1 tablet:w-auto tablet:min-w-60 tablet:flex-1">
        <label htmlFor={inputId} className="text-label">
          {t('students.searchLabel')}
        </label>
        <input
          ref={searchField}
          id={inputId}
          name="q"
          type="search"
          maxLength={254}
          defaultValue={view.search}
          className={fieldClass}
        />
      </div>
      <GroupFilter group={view.group} />
      <button type="submit" className={`${quietButtonClass} w-full tablet:w-auto`}>
        {t('students.searchSubmit')}
      </button>
      {filtered ? (
        <Link
          to={listAddress({ ...view, search: '', group: '' })}
          onClick={() => {
            searchField.current?.focus();
          }}
          className={linkClass}
        >
          {t('students.clearSearch')}
        </Link>
      ) : null}
    </form>
  );
}

function lessonShown(card: StudentCardSummary): boolean {
  return card.next_lesson !== null || card.state === 'active' || card.state === 'provisional';
}

function StudentRow({ card }: { readonly card: StudentCardSummary }): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const email = card.email ?? card.account?.email ?? null;
  const lesson = lessonShown(card) ? <NextLessonLine item={card.next_lesson} me={me} /> : null;
  return (
    <tr data-row={card.id} className={tableRowClass}>
      <th scope="row" className={primaryCellClass}>
        <span className="flex min-w-0 items-center gap-3">
          <Avatar name={card.display_name} avatarId={card.account?.avatar_id ?? null} />
          <span className="flex min-w-0 flex-col">
            <Link
              data-row-link=""
              to={`/students/${encodeURIComponent(card.id)}`}
              dir="auto"
              className={`truncate ${rowLinkClass}`}
            >
              {card.display_name}
            </Link>
            <span className={`text-small text-text-muted ${narrowOnlyClass}`}>{email ?? t('students.noEmail')}</span>
            {card.groups.length === 0 ? null : (
              <span className={narrowOnlyClass}>
                <GroupDots groups={card.groups} />
              </span>
            )}
            {lesson === null ? null : <span className={narrowOnlyClass}>{lesson}</span>}
            <span className={`text-small text-text-muted ${narrowOnlyClass}`}>
              {t('students.openAssignments', { count: card.open_assignments })}
            </span>
          </span>
        </span>
      </th>
      <td className={secondaryCellClass}>
        <span className="block text-small text-text-muted">{email ?? t('students.noEmail')}</span>
      </td>
      <td className={cellClass}>
        <StateBadge state={card.state} />
      </td>
      <td className={secondaryCellClass}>
        <GroupDots groups={card.groups} />
      </td>
      <td className={secondaryCellClass}>{lesson}</td>
      <td className={secondaryCellClass}>
        <span className="tabular">{card.open_assignments}</span>
      </td>
    </tr>
  );
}

function EmptyList({ view }: { readonly view: ListView }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const { state } = view;
  const filtered = view.search !== '' || view.group !== '';
  const firstStudent = state === 'active' && !filtered;
  const code = useQuery({
    queryKey: tutorCodeKey,
    queryFn: () => fetchTutorCode(client),
    enabled: firstStudent,
    retry: false,
  });
  if (!firstStudent) {
    return (
      <p className="text-text-muted">
        {t(filtered ? 'students.noMatches' : state === 'invited' ? 'students.emptyInvited' : 'students.emptyEnded')}
      </p>
    );
  }
  return (
    <section className="flex flex-col items-start gap-3">
      <h2 className="text-heading">{t('students.emptyTitle')}</h2>
      <p className="text-text-muted">{t('students.emptyBody')}</p>
      {code.isSuccess ? (
        <p className="text-text-muted">
          {t('students.emptyCode')} <span className="font-mono text-text">{code.data.code}</span>
        </p>
      ) : null}
      {code.isError ? (
        <LoadFailed
          error={asApiError(code.error)}
          onRetry={() => {
            void code.refetch();
          }}
        />
      ) : null}
    </section>
  );
}

function unusableFilter(error: ApiError, view: ListView): 'group' | 'search' | null {
  if (view.group !== '' && (error.code === 'not_found' || error.fieldCode('query.group') !== undefined)) {
    return 'group';
  }
  if (view.search !== '' && error.fieldCode('query.q') !== undefined) {
    return 'search';
  }
  return null;
}

function StudentList({ view }: { readonly view: ListView }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const { state, search, group } = view;
  const query = useInfiniteQuery({
    queryKey: studentListKey(state, search, group),
    queryFn: ({ pageParam }) => fetchStudentsPage(client, { state, search, group, cursor: pageParam }),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    placeholderData: keepPreviousData,
    refetchOnWindowFocus: false,
    retry: false,
  });
  const cards = query.data?.pages.flatMap((page) => page.items) ?? [];
  const columns: readonly TableColumn[] = [
    { key: 'name', label: t('students.columns.name'), width: 'w-2/3 tablet:w-3/12' },
    { key: 'email', label: t('students.columns.email'), secondary: true, width: 'w-2/12' },
    { key: 'state', label: t('students.columns.state'), width: 'w-1/3 tablet:w-2/12' },
    { key: 'groups', label: t('students.columns.groups'), secondary: true, width: 'w-2/12' },
    { key: 'lesson', label: t('students.columns.nextLesson'), secondary: true, width: 'w-2/12' },
    { key: 'open', label: t('students.columns.openAssignments'), secondary: true, width: 'w-1/12' },
  ];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLTableSectionElement>(
    cards.length,
    `:scope > tr ${rowLinkSelector}`,
    query.isFetching,
  );
  useFocusAfterRemovedRow(
    listRef,
    cards.map((card) => card.id),
    rowLinkSelector,
  );
  if (query.isPending) {
    return <DelayedLoading />;
  }
  if (query.isError) {
    const error = asApiError(query.error);
    if (error.code === 'email_not_verified') {
      return <ConfirmationPrompt reason="invite" />;
    }
    const unusable = unusableFilter(error, view);
    if (unusable !== null) {
      return (
        <div className="flex flex-col items-start gap-1">
          <p>{t(unusable === 'group' ? 'students.groupNotFound' : 'students.searchInvalid')}</p>
          <Link
            to={listAddress(unusable === 'group' ? { ...view, group: '' } : { ...view, search: '' })}
            className={linkClass}
          >
            {t(unusable === 'group' ? 'students.allGroups' : 'students.withoutSearch')}
          </Link>
        </div>
      );
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
  if (cards.length === 0) {
    return <EmptyList view={view} />;
  }
  return (
    <div aria-busy={query.isPlaceholderData} className="flex flex-col items-start gap-4">
      <DataTable label={t(`students.tabs.${state}`)} columns={columns} bodyRef={listRef}>
        {cards.map((card) => (
          <StudentRow key={card.id} card={card} />
        ))}
      </DataTable>
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
          {t('students.showMore')}
        </button>
      ) : null}
    </div>
  );
}

export function StudentsPage(): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const location = useLocation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const requestedState = searchParams.get('state');
  const state: StudentListState = isStudentListState(requestedState) ? requestedState : 'active';
  const view: ListView = {
    state,
    search: (searchParams.get('q') ?? '').trim(),
    group: searchParams.get('group') ?? '',
  };
  const address = listAddress(view);
  const [withdrawnView, setWithdrawnView] = useState(() =>
    hasNotice(location.state, withdrawnNotice) ? address : null,
  );
  if (withdrawnView !== null && withdrawnView !== address) {
    setWithdrawnView(null);
  }
  const [dialogOpen, setDialogOpen] = useState(false);
  const opener = useRef<HTMLButtonElement>(null);
  const requestsSettled = useJoinRequestsSettled();

  useEffect(() => {
    if (hasNotice(location.state, withdrawnNotice)) {
      void navigate(`${location.pathname}${location.search}`, { replace: true, state: null });
    }
  }, [location, navigate]);

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
        {t('students.add')}
      </button>
    );

  return (
    <Page action={addButton ?? undefined}>
      <LiveNotice message={withdrawnView === null ? null : t('students.withdrawn')} />
      <JoinRequests hideWhenEmpty />
      {requestsSettled ? null : <DelayedLoading />}
      <div hidden={!requestsSettled} className="flex flex-col gap-6">
        <div className="flex flex-col gap-4 desktop:flex-row desktop:items-end desktop:gap-8">
          <Tabs view={view} />
          <SearchForm view={view} />
        </div>
        <StudentList view={view} />
      </div>
      <AddStudentDialog
        open={dialogOpen}
        returnFocus={opener}
        onClose={() => {
          setDialogOpen(false);
        }}
      />
    </Page>
  );
}
