import { useInfiniteQuery } from '@tanstack/react-query';
import { type ReactElement, type SyntheticEvent, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { fieldClass, quietButtonClass, stringField } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import type { StudentCardSummary } from '../students/api.ts';
import { activeStudentsKey, fetchActiveStudentsPage } from './api.ts';

export type Chosen = ReadonlyMap<string, string>;

export function toggled(chosen: Chosen, id: string, name: string): Chosen {
  const next = new Map(chosen);
  if (next.has(id)) {
    next.delete(id);
  } else {
    next.set(id, name);
  }
  return next;
}

export function without(chosen: Chosen, ids: readonly string[]): Chosen {
  const next = new Map(chosen);
  for (const id of ids) {
    next.delete(id);
  }
  return next;
}

export function trimmed(chosen: Chosen, places: number): Chosen {
  return chosen.size <= places ? chosen : new Map([...chosen].slice(0, Math.max(0, places)));
}

function StudentChoice({
  card,
  checked,
  full,
  onToggle,
}: {
  readonly card: StudentCardSummary;
  readonly checked: boolean;
  readonly full: boolean;
  readonly onToggle: (id: string, name: string) => void;
}): ReactElement {
  const { t } = useTranslation();
  const inputId = useId();
  const emailId = useId();
  return (
    <li className="flex min-h-11 items-center gap-3 text-small">
      <input
        id={inputId}
        type="checkbox"
        checked={checked}
        aria-disabled={!checked && full}
        aria-describedby={emailId}
        onChange={(event) => {
          if (!checked && full) {
            event.preventDefault();
            return;
          }
          onToggle(card.id, card.display_name);
        }}
        className="size-4 accent-accent"
      />
      <span className="flex min-w-0 flex-col">
        <label htmlFor={inputId} className="truncate text-strong">
          {card.display_name}
        </label>
        <span id={emailId} className="truncate text-text-muted">
          {card.email ?? card.account?.email ?? t('students.noEmail')}
        </span>
      </span>
    </li>
  );
}

function StudentChoices({
  search,
  excluded,
  chosen,
  slots,
  onToggle,
}: {
  readonly search: string;
  readonly excluded: ReadonlySet<string>;
  readonly chosen: Chosen;
  readonly slots: number;
  readonly onToggle: (id: string, name: string) => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const query = useInfiniteQuery({
    queryKey: activeStudentsKey(search),
    queryFn: ({ pageParam, signal }) => fetchActiveStudentsPage(client, search, pageParam, signal),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    refetchOnWindowFocus: false,
    retry: false,
  });
  const students = (query.data?.pages.flatMap((page) => page.items) ?? []).filter(
    (card) => card.state === 'active' && !excluded.has(card.id),
  );
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(
    students.length,
    ':scope > li input',
    query.isFetchingNextPage,
  );
  if (query.isPending) {
    return <DelayedLoading />;
  }
  if (query.isError) {
    return (
      <LoadFailed
        error={asApiError(query.error)}
        onRetry={() => {
          void query.refetch();
        }}
      />
    );
  }
  const full = chosen.size >= slots;
  return (
    <>
      {students.length === 0 ? (
        <p className="text-text-muted">{t(search === '' ? 'groups.noActiveStudents' : 'groups.noMatches')}</p>
      ) : (
        <ul ref={listRef} aria-label={t('groups.students')} className="flex max-h-64 flex-col overflow-y-auto">
          {students.map((card) => (
            <StudentChoice key={card.id} card={card} checked={chosen.has(card.id)} full={full} onToggle={onToggle} />
          ))}
        </ul>
      )}
      {query.hasNextPage ? (
        <p>
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
            {t('groups.showMore')}
          </button>
        </p>
      ) : null}
    </>
  );
}

export function MemberPicker({
  excluded,
  chosen,
  slots,
  onToggle,
}: {
  readonly excluded: ReadonlySet<string>;
  readonly chosen: Chosen;
  readonly slots: number;
  readonly onToggle: (id: string, name: string) => void;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const inputId = useId();
  const [search, setSearch] = useState('');

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    setSearch(stringField(new FormData(event.currentTarget), 'q').trim());
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h3 id={headingId} className="text-label">
        {t('groups.chooseStudents')}
      </h3>
      <form role="search" aria-label={t('groups.searchStudents')} onSubmit={submit} className="flex items-end gap-2">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <label htmlFor={inputId} className="text-small text-text-muted">
            {t('groups.searchStudents')}
          </label>
          <input id={inputId} name="q" type="search" maxLength={254} className={fieldClass} />
        </div>
        <button type="submit" className={quietButtonClass}>
          {t('groups.search')}
        </button>
      </form>
      <p aria-live="polite" className="text-small text-text-muted">
        {t('groups.chosen', { count: chosen.size, limit: slots })}
      </p>
      <StudentChoices search={search} excluded={excluded} chosen={chosen} slots={slots} onToggle={onToggle} />
    </section>
  );
}
