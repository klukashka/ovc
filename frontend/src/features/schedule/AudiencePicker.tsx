import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { type ReactElement, type RefObject, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useDebouncedValue } from '../../app/clock.ts';
import { fieldClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { paletteDotClass } from '../../components/ui/palette.ts';
import { type ApiClient, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import { type GroupSummary, fetchGroups, groupsKey } from '../groups/api.ts';
import { type StudentCardPage, type StudentCardSummary, studentsKey } from '../students/api.ts';

export type Audience =
  | { readonly kind: 'one_to_one'; readonly id: string; readonly name: string }
  | { readonly kind: 'group'; readonly id: string; readonly name: string };

export const searchDelayMilliseconds = 300;

function schedulableStudentsKey(search: string): readonly unknown[] {
  return [...studentsKey, 'schedulable', search];
}

async function fetchStudentsPage(client: ApiClient, search: string, cursor: string | null): Promise<StudentCardPage> {
  const { data, error, response } = await client.GET('/api/v1/students', {
    params: {
      query: { state: 'active', ...(search === '' ? {} : { q: search }), ...(cursor === null ? {} : { cursor }) },
    },
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

function schedulable(card: StudentCardSummary): boolean {
  return card.state === 'active' || card.state === 'provisional';
}

function matches(group: GroupSummary, search: string, withMembers: boolean): boolean {
  return (
    group.archived_at === null &&
    (!withMembers || group.member_count > 0) &&
    group.name.toLocaleLowerCase().includes(search.toLocaleLowerCase())
  );
}

function Choice({
  label,
  hint,
  dot,
  onChoose,
}: {
  readonly label: string;
  readonly hint: string | null;
  readonly dot: string | null;
  readonly onChoose: () => void;
}): ReactElement {
  const labelId = useId();
  const hintId = useId();
  return (
    <li className="border-b border-border">
      <button
        type="button"
        aria-labelledby={labelId}
        aria-describedby={hint === null ? undefined : hintId}
        onClick={onChoose}
        className="flex min-h-11 w-full items-center gap-3 px-1 text-left text-small hover:bg-hover focus-visible:-outline-offset-2"
      >
        {dot === null ? null : <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${dot}`} />}
        <span className="flex min-w-0 flex-col">
          <span id={labelId} dir="auto" className="truncate text-strong">
            {label}
          </span>
          {hint === null ? null : (
            <span id={hintId} className="truncate text-text-muted">
              {hint}
            </span>
          )}
        </span>
      </button>
    </li>
  );
}

function isListed(audience: Audience, cards: readonly StudentCardSummary[], groups: readonly GroupSummary[]): boolean {
  return audience.kind === 'one_to_one'
    ? cards.some((card) => card.id === audience.id)
    : groups.some((group) => group.id === audience.id);
}

function Choices({
  search,
  preselected,
  groupsWithMembers,
  searchField,
  onChoose,
}: {
  readonly search: string;
  readonly preselected: Audience | null;
  readonly groupsWithMembers: boolean;
  readonly searchField: RefObject<HTMLInputElement | null>;
  readonly onChoose: (audience: Audience) => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const students = useInfiniteQuery({
    queryKey: schedulableStudentsKey(search),
    queryFn: ({ pageParam }) => fetchStudentsPage(client, search, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    placeholderData: (previous) => previous,
    retry: false,
  });
  const groups = useQuery({ queryKey: groupsKey, queryFn: () => fetchGroups(client), retry: false });
  const cards = (students.data?.pages.flatMap((page) => page.items) ?? []).filter(schedulable);
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(
    cards.length,
    ':scope > li button',
    students.isFetchingNextPage,
    searchField,
  );
  if (students.isPending || groups.isPending) {
    return <DelayedLoading />;
  }
  if (students.isError || groups.isError) {
    return (
      <LoadFailed
        error={asApiError(students.error ?? groups.error)}
        onRetry={() => {
          void students.refetch();
          void groups.refetch();
        }}
      />
    );
  }
  const shownGroups = groups.data.filter((group) => matches(group, search, groupsWithMembers));
  const pinned = preselected !== null && !isListed(preselected, cards, shownGroups) ? preselected : null;
  if (pinned === null && cards.length === 0 && shownGroups.length === 0 && !students.hasNextPage) {
    return <p className="text-small text-text-muted">{t('newLesson.nothingFound')}</p>;
  }
  return (
    <div className="flex max-h-64 flex-col gap-2 overflow-y-auto">
      {pinned === null ? null : (
        <ul aria-label={t('newLesson.preselected')} className="flex flex-col border-t border-border">
          <Choice
            label={pinned.name}
            hint={null}
            dot={null}
            onChoose={() => {
              onChoose(pinned);
            }}
          />
        </ul>
      )}
      {cards.length === 0 ? null : (
        <ul ref={listRef} aria-label={t('newLesson.students')} className="flex flex-col border-t border-border">
          {cards.map((card) => (
            <Choice
              key={card.id}
              label={card.display_name}
              hint={card.state === 'provisional' ? t('newLesson.emailNotConfirmed') : null}
              dot={null}
              onChoose={() => {
                onChoose({ kind: 'one_to_one', id: card.id, name: card.display_name });
              }}
            />
          ))}
        </ul>
      )}
      {students.hasNextPage ? (
        <p>
          <button
            type="button"
            aria-disabled={students.isFetchingNextPage}
            aria-busy={students.isFetchingNextPage}
            onClick={() => {
              if (!students.isFetchingNextPage) {
                loadingMore();
                void students.fetchNextPage();
              }
            }}
            className={quietButtonClass}
          >
            {t('newLesson.showMore')}
          </button>
        </p>
      ) : null}
      {shownGroups.length === 0 ? null : (
        <ul aria-label={t('newLesson.groups')} className="flex flex-col border-t border-border">
          {shownGroups.map((group) => (
            <Choice
              key={group.id}
              label={group.name}
              hint={null}
              dot={paletteDotClass[group.colour]}
              onChoose={() => {
                onChoose({ kind: 'group', id: group.id, name: group.name });
              }}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

type PendingFocus = 'search' | 'change' | null;

export function AudiencePicker({
  chosen,
  preselected,
  groupsWithMembers = false,
  searchField,
  changeButton,
  error,
  onChoose,
  onClear,
}: {
  readonly chosen: Audience | null;
  readonly preselected: Audience | null;
  readonly groupsWithMembers?: boolean;
  readonly searchField: RefObject<HTMLInputElement | null>;
  readonly changeButton: RefObject<HTMLButtonElement | null>;
  readonly error: string | undefined;
  readonly onChoose: (audience: Audience) => void;
  readonly onClear: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const legendId = useId();
  const inputId = useId();
  const errorId = useId();
  const chosenId = useId();
  const [search, setSearch] = useState('');
  const [changing, setChanging] = useState(false);
  const pendingFocus = useRef<PendingFocus>(null);
  const settledSearch = useDebouncedValue(search.trim(), searchDelayMilliseconds);
  const collapsed = chosen !== null && !changing;

  useEffect(() => {
    if (pendingFocus.current === 'search') {
      searchField.current?.focus();
    } else if (pendingFocus.current === 'change') {
      changeButton.current?.focus();
    }
    pendingFocus.current = null;
  });

  if (collapsed) {
    return (
      <fieldset aria-labelledby={legendId} className="flex flex-col gap-2">
        <legend id={legendId} className="text-label">
          {t('newLesson.audience')}
        </legend>
        <p className="flex min-h-11 items-center gap-3">
          <span id={chosenId} dir="auto" className="min-w-0 truncate text-strong">
            {chosen.name}
          </span>
          <button
            ref={changeButton}
            type="button"
            aria-describedby={chosenId}
            onClick={() => {
              pendingFocus.current = 'search';
              setChanging(true);
              onClear();
            }}
            className={quietButtonClass}
          >
            {t('newLesson.changeAudience')}
          </button>
        </p>
      </fieldset>
    );
  }

  return (
    <fieldset aria-labelledby={legendId} className="flex flex-col gap-2">
      <legend id={legendId} className="text-label">
        {t('newLesson.audience')}
      </legend>
      <label htmlFor={inputId} className="text-small text-text-muted">
        {t('newLesson.search')}
      </label>
      <input
        ref={searchField}
        id={inputId}
        type="search"
        maxLength={254}
        value={search}
        aria-invalid={error === undefined ? undefined : true}
        aria-describedby={error === undefined ? undefined : errorId}
        onChange={(event) => {
          setSearch(event.target.value);
        }}
        className={fieldClass}
      />
      {error === undefined ? null : (
        <p id={errorId} className="text-small text-danger">
          {error}
        </p>
      )}
      <Choices
        search={settledSearch}
        preselected={preselected}
        groupsWithMembers={groupsWithMembers}
        searchField={searchField}
        onChoose={(audience) => {
          onChoose(audience);
          pendingFocus.current = 'change';
          setChanging(false);
        }}
      />
    </fieldset>
  );
}
