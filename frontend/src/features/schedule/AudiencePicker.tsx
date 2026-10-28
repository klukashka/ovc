import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { type ReactElement, type RefObject, useDeferredValue, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';

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

function matches(group: GroupSummary, search: string): boolean {
  return group.archived_at === null && group.name.toLocaleLowerCase().includes(search.toLocaleLowerCase());
}

function Choice({
  value,
  label,
  hint,
  dot,
  checked,
  onChoose,
}: {
  readonly value: string;
  readonly label: string;
  readonly hint: string | null;
  readonly dot: string | null;
  readonly checked: boolean;
  readonly onChoose: () => void;
}): ReactElement {
  const inputId = useId();
  const hintId = useId();
  return (
    <li className="flex min-h-11 items-center gap-3 text-small">
      <input
        id={inputId}
        type="radio"
        name="audience"
        value={value}
        checked={checked}
        aria-describedby={hint === null ? undefined : hintId}
        onChange={onChoose}
        className="size-4 accent-accent"
      />
      {dot === null ? null : <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${dot}`} />}
      <span className="flex min-w-0 flex-col">
        <label htmlFor={inputId} className="truncate text-strong">
          {label}
        </label>
        {hint === null ? null : (
          <span id={hintId} className="truncate text-text-muted">
            {hint}
          </span>
        )}
      </span>
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
  chosen,
  preselected,
  searchField,
  onChoose,
}: {
  readonly search: string;
  readonly chosen: Audience | null;
  readonly preselected: Audience | null;
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
    ':scope > li input',
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
  const shownGroups = groups.data.filter((group) => matches(group, search));
  const pinned = preselected !== null && !isListed(preselected, cards, shownGroups) ? preselected : null;
  if (pinned === null && cards.length === 0 && shownGroups.length === 0 && !students.hasNextPage) {
    return <p className="text-small text-text-muted">{t('newLesson.nothingFound')}</p>;
  }
  return (
    <div className="flex max-h-64 flex-col gap-2 overflow-y-auto">
      {pinned === null ? null : (
        <ul aria-label={t('newLesson.preselected')} className="flex flex-col">
          <Choice
            value={`${pinned.kind}:${pinned.id}`}
            label={pinned.name}
            hint={null}
            dot={null}
            checked={chosen?.kind === pinned.kind && chosen.id === pinned.id}
            onChoose={() => {
              onChoose(pinned);
            }}
          />
        </ul>
      )}
      {cards.length === 0 ? null : (
        <ul ref={listRef} aria-label={t('newLesson.students')} className="flex flex-col">
          {cards.map((card) => (
            <Choice
              key={card.id}
              value={`relationship:${card.id}`}
              label={card.display_name}
              hint={card.state === 'provisional' ? t('newLesson.emailNotConfirmed') : null}
              dot={null}
              checked={chosen?.kind === 'one_to_one' && chosen.id === card.id}
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
        <ul aria-label={t('newLesson.groups')} className="flex flex-col">
          {shownGroups.map((group) => (
            <Choice
              key={group.id}
              value={`group:${group.id}`}
              label={group.name}
              hint={null}
              dot={paletteDotClass[group.colour]}
              checked={chosen?.kind === 'group' && chosen.id === group.id}
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

export function AudiencePicker({
  chosen,
  preselected,
  searchField,
  error,
  onChoose,
}: {
  readonly chosen: Audience | null;
  readonly preselected: Audience | null;
  readonly searchField: RefObject<HTMLInputElement | null>;
  readonly error: string | undefined;
  readonly onChoose: (audience: Audience) => void;
}): ReactElement {
  const { t } = useTranslation();
  const legendId = useId();
  const inputId = useId();
  const errorId = useId();
  const [search, setSearch] = useState('');
  const deferredSearch = useDeferredValue(search.trim());
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
        search={deferredSearch}
        chosen={chosen}
        preselected={preselected}
        searchField={searchField}
        onChoose={onChoose}
      />
    </fieldset>
  );
}
