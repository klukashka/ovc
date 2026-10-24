import { keepPreviousData, useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { type ReactElement, type SyntheticEvent, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router';

import { Avatar } from '../../components/ui/Avatar.tsx';
import { ConfirmationPrompt } from '../../components/ui/ConfirmationPrompt.tsx';
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
import { useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Page } from '../../components/ui/Page.tsx';
import { useAddressValue } from '../../components/ui/serverValue.ts';
import { type ApiError, asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { fetchGroups, groupsKey } from '../groups/api.ts';
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

const tabClass = 'inline-flex min-h-11 items-center border-b-2 px-1 text-small';
const currentTabClass = `${tabClass} border-accent font-medium text-text`;
const otherTabClass = `${tabClass} border-transparent text-text-muted hover:text-text`;

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
    <nav aria-label={t('students.lists')} className="border-b border-border">
      <ul className="flex flex-wrap gap-6">
        {studentListStates.map((state) => (
          <li key={state}>
            <Link
              to={listAddress({ ...view, state })}
              aria-current={state === view.state ? 'page' : undefined}
              className={state === view.state ? currentTabClass : otherTabClass}
            >
              {t(`students.tabs.${state}`)}
            </Link>
          </li>
        ))}
      </ul>
    </nav>
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
    queryFn: ({ signal }) => fetchGroups(client, signal),
    enabled: me.state === 'active',
    retry: false,
  });
  if (!groups.isSuccess || (groups.data.length === 0 && group === '')) {
    return null;
  }
  return (
    <div className="flex min-w-48 flex-col gap-1">
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
    <form role="search" aria-label={t('students.search')} onSubmit={submit} className="flex flex-wrap items-end gap-3">
      <div className="flex min-w-60 flex-1 flex-col gap-1">
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
      <button type="submit" className={quietButtonClass}>
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

function StudentRow({ card }: { readonly card: StudentCardSummary }): ReactElement {
  const { t } = useTranslation();
  const email = card.email ?? card.account?.email ?? null;
  return (
    <li className="border-t border-border">
      <Link
        to={`/students/${encodeURIComponent(card.id)}`}
        className="grid min-h-11 grid-cols-[auto_1fr] items-center gap-x-3 gap-y-1 px-2 py-3 hover:bg-surface tablet:grid-cols-[auto_minmax(0,2fr)_minmax(0,2fr)_minmax(14rem,1fr)]"
      >
        <Avatar name={card.display_name} avatarId={card.account?.avatar_id ?? null} />
        <span className="flex min-w-0 flex-col">
          <span className="truncate text-strong">{card.display_name}</span>
          <span className="truncate text-small text-text-muted tablet:hidden">{email ?? t('students.noEmail')}</span>
        </span>
        <span className="hidden truncate text-small text-text-muted tablet:block">
          {email ?? t('students.noEmail')}
        </span>
        <span className="col-start-2 flex flex-col gap-1 tablet:col-start-auto">
          <StateBadge state={card.state} />
          <GroupDots groups={card.groups} />
        </span>
      </Link>
    </li>
  );
}

function EmptyList({
  view,
  addButton,
}: {
  readonly view: ListView;
  readonly addButton: ReactElement | null;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const { state } = view;
  const filtered = view.search !== '' || view.group !== '';
  const firstStudent = state === 'active' && !filtered;
  const code = useQuery({
    queryKey: tutorCodeKey,
    queryFn: ({ signal }) => fetchTutorCode(client, signal),
    enabled: firstStudent,
    retry: false,
  });
  if (!firstStudent) {
    return (
      <div className="flex flex-col items-start gap-4">
        {addButton}
        <p className="text-text-muted">
          {t(filtered ? 'students.noMatches' : state === 'invited' ? 'students.emptyInvited' : 'students.emptyEnded')}
        </p>
      </div>
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
      {addButton}
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

function StudentList({
  view,
  addButton,
}: {
  readonly view: ListView;
  readonly addButton: ReactElement | null;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const { state, search, group } = view;
  const query = useInfiniteQuery({
    queryKey: studentListKey(state, search, group),
    queryFn: ({ pageParam, signal }) => fetchStudentsPage(client, { state, search, group, cursor: pageParam }, signal),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    placeholderData: keepPreviousData,
    refetchOnWindowFocus: false,
    retry: false,
  });
  const cards = query.data?.pages.flatMap((page) => page.items) ?? [];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(
    cards.length,
    ':scope > li > a',
    query.isFetchingNextPage,
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
    return <EmptyList view={view} addButton={addButton} />;
  }
  return (
    <div aria-busy={query.isPlaceholderData} className="flex flex-col items-start gap-4">
      {addButton}
      <ul ref={listRef} aria-label={t(`students.tabs.${state}`)} className="w-full border-b border-border">
        {cards.map((card) => (
          <StudentRow key={card.id} card={card} />
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
              void query.fetchNextPage();
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
    <Page wide>
      <LiveNotice message={withdrawnView === null ? null : t('students.withdrawn')} />
      <JoinRequests hideWhenEmpty />
      {requestsSettled ? null : <DelayedLoading />}
      <div hidden={!requestsSettled} className="flex flex-col gap-6">
        <Tabs view={view} />
        <SearchForm view={view} />
        <StudentList view={view} addButton={addButton} />
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
