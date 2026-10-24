import { keepPreviousData, useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { type ReactElement, type SyntheticEvent, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router';

import { Avatar } from '../../components/ui/Avatar.tsx';
import { ConfirmationPrompt } from '../../components/ui/ConfirmationPrompt.tsx';
import { fieldClass, linkClass, primaryButtonClass, quietButtonClass, stringField } from '../../components/ui/Form.tsx';
import { LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Page } from '../../components/ui/Page.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
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
import { StateBadge } from './StateBadge.tsx';

const tabClass = 'inline-flex min-h-11 items-center border-b-2 px-1 text-small';
const currentTabClass = `${tabClass} border-accent font-medium text-text`;
const otherTabClass = `${tabClass} border-transparent text-text-muted hover:text-text`;

function listAddress(state: StudentListState, search: string): string {
  const query = new URLSearchParams({ state });
  if (search !== '') {
    query.set('q', search);
  }
  return `/students?${query.toString()}`;
}

function Tabs({ current, search }: { readonly current: StudentListState; readonly search: string }): ReactElement {
  const { t } = useTranslation();
  return (
    <nav aria-label={t('students.lists')} className="border-b border-border">
      <ul className="flex flex-wrap gap-6">
        {studentListStates.map((state) => (
          <li key={state}>
            <Link
              to={listAddress(state, search)}
              aria-current={state === current ? 'page' : undefined}
              className={state === current ? currentTabClass : otherTabClass}
            >
              {t(`students.tabs.${state}`)}
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );
}

function SearchForm({ state, search }: { readonly state: StudentListState; readonly search: string }): ReactElement {
  const { t } = useTranslation();
  const [, setSearchParams] = useSearchParams();
  const inputId = useId();

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    const value = stringField(new FormData(event.currentTarget), 'q').trim();
    setSearchParams(value === '' ? { state } : { state, q: value });
  }

  return (
    <form
      key={search}
      role="search"
      aria-label={t('students.search')}
      onSubmit={submit}
      className="flex flex-wrap items-end gap-3"
    >
      <div className="flex min-w-60 flex-1 flex-col gap-1">
        <label htmlFor={inputId} className="text-label">
          {t('students.searchLabel')}
        </label>
        <input id={inputId} name="q" type="search" maxLength={254} defaultValue={search} className={fieldClass} />
      </div>
      <button type="submit" className={quietButtonClass}>
        {t('students.searchSubmit')}
      </button>
      {search === '' ? null : (
        <Link to={listAddress(state, '')} className={linkClass}>
          {t('students.clearSearch')}
        </Link>
      )}
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
  state,
  search,
  addButton,
}: {
  readonly state: StudentListState;
  readonly search: string;
  readonly addButton: ReactElement | null;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const firstStudent = state === 'active' && search === '';
  const code = useQuery({
    queryKey: tutorCodeKey,
    queryFn: ({ signal }) => fetchTutorCode(client, signal),
    enabled: firstStudent,
    retry: false,
  });
  if (search !== '') {
    return <p className="text-text-muted">{t('students.noMatches')}</p>;
  }

  if (!firstStudent) {
    return (
      <p className="text-text-muted">{t(state === 'invited' ? 'students.emptyInvited' : 'students.emptyEnded')}</p>
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

function StudentList({
  state,
  search,
  addButton,
}: {
  readonly state: StudentListState;
  readonly search: string;
  readonly addButton: ReactElement | null;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const query = useInfiniteQuery({
    queryKey: studentListKey(state, search),
    queryFn: ({ pageParam, signal }) => fetchStudentsPage(client, { state, search, cursor: pageParam }, signal),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    placeholderData: keepPreviousData,
    refetchOnWindowFocus: false,
    retry: false,
  });
  const cards = query.data?.pages.flatMap((page) => page.items) ?? [];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(cards.length, ':scope > li > a');
  if (query.isPending) {
    return <DelayedLoading />;
  }
  if (query.isError) {
    const error = asApiError(query.error);
    if (error.code === 'email_not_verified') {
      return <ConfirmationPrompt reason="invite" />;
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
    return <EmptyList state={state} search={search} addButton={addButton} />;
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
  const search = (searchParams.get('q') ?? '').trim();
  const view = listAddress(state, search);
  const [withdrawnView, setWithdrawnView] = useState(() => (hasNotice(location.state, withdrawnNotice) ? view : null));
  if (withdrawnView !== null && withdrawnView !== view) {
    setWithdrawnView(null);
  }
  const [dialogOpen, setDialogOpen] = useState(false);
  const opener = useRef<HTMLButtonElement>(null);

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
      <Tabs current={state} search={search} />
      <SearchForm state={state} search={search} />
      <StudentList state={state} search={search} addButton={addButton} />
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
