import { useInfiniteQuery } from '@tanstack/react-query';
import { type ReactElement, type RefObject, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useDebouncedValue } from '../../app/clock.ts';
import { fieldClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { loadNextPage, useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { type LibraryItem, fetchLibraryPage, libraryKey, searchLimit, searchTerm } from '../library/api.ts';
import { searchDelayMilliseconds } from '../schedule/AudiencePicker.tsx';

export interface ChosenTemplate {
  readonly id: string;
  readonly title: string;
}

interface FolderStep {
  readonly id: string;
  readonly name: string;
}

export function templatePickerKey(folder: string | null, search: string | null): readonly unknown[] {
  return [...libraryKey, 'issue-picker', folder, search];
}

function TemplateChoice({
  item,
  notReadyHintId,
  onChoose,
}: {
  readonly item: LibraryItem;
  readonly notReadyHintId: string;
  readonly onChoose: (template: ChosenTemplate) => void;
}): ReactElement {
  const { t } = useTranslation();
  const titleId = useId();
  const stateId = useId();
  const ready = item.ready === true;
  return (
    <li className="border-b border-border">
      <button
        type="button"
        aria-labelledby={titleId}
        aria-describedby={ready ? stateId : `${stateId} ${notReadyHintId}`}
        aria-disabled={ready ? undefined : true}
        onClick={() => {
          if (ready) {
            onChoose({ id: item.id, title: item.title });
          }
        }}
        className="flex min-h-11 w-full items-center justify-between gap-3 px-1 text-left text-small hover:bg-hover focus-visible:-outline-offset-2 aria-disabled:opacity-60"
      >
        <span id={titleId} dir="auto" className="min-w-0 truncate text-strong">
          {item.title}
        </span>
        <span id={stateId} className="shrink-0">
          <StatusWord tone={ready ? 'success' : 'warning'}>
            {ready ? t('homework.picker.ready') : t('homework.picker.incomplete')}
          </StatusWord>
        </span>
      </button>
    </li>
  );
}

function FolderPath({
  trail,
  currentStep,
  onOpen,
}: {
  readonly trail: readonly FolderStep[];
  readonly currentStep: RefObject<HTMLElement | null>;
  readonly onOpen: (depth: number) => void;
}): ReactElement {
  const { t } = useTranslation();
  const steps = [{ id: null, name: t('homework.picker.root') }, ...trail];
  return (
    <ol aria-label={t('homework.picker.path')} className="flex flex-wrap items-center gap-x-2 text-small">
      {steps.map((step, depth) => (
        <li key={step.id ?? 'root'} className="inline-flex items-center gap-2">
          {depth === 0 ? null : <span aria-hidden="true">/</span>}
          {depth === steps.length - 1 ? (
            <span
              ref={currentStep}
              tabIndex={-1}
              dir="auto"
              aria-current="location"
              className="inline-flex min-h-11 items-center text-strong outline-none"
            >
              {step.name}
            </span>
          ) : (
            <button
              type="button"
              dir="auto"
              onClick={() => {
                onOpen(depth);
              }}
              className="inline-flex min-h-11 items-center text-accent-text underline"
            >
              {step.name}
            </button>
          )}
        </li>
      ))}
    </ol>
  );
}

function Listing({
  folder,
  search,
  searchField,
  onOpenFolder,
  onChoose,
}: {
  readonly folder: string | null;
  readonly search: string | null;
  readonly searchField: RefObject<HTMLInputElement | null>;
  readonly onOpenFolder: (step: FolderStep) => void;
  readonly onChoose: (template: ChosenTemplate) => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const notReadyHintId = useId();
  const listing = useInfiniteQuery({
    queryKey: templatePickerKey(folder, search),
    queryFn: ({ pageParam, signal }) => fetchLibraryPage(client, { folder, search }, pageParam, signal),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    retry: false,
  });
  const templates = (listing.data?.pages.flatMap((page) => page.items) ?? []).filter(
    (item) => item.kind === 'template',
  );
  const folders = search === null ? (listing.data?.pages[0]?.folders ?? []) : [];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(
    templates.length,
    ':scope > li button',
    listing.isFetching,
    searchField,
  );
  if (listing.isPending) {
    return <DelayedLoading />;
  }
  if (listing.isError) {
    return (
      <LoadFailed
        error={asApiError(listing.error)}
        onRetry={() => {
          void listing.refetch();
        }}
      />
    );
  }
  const empty = folders.length === 0 && templates.length === 0 && !listing.hasNextPage;
  return (
    <div className="flex flex-col gap-2">
      {empty ? (
        <p className="text-small text-text-muted">
          {search === null ? t('homework.picker.empty') : t('homework.picker.nothingFound')}
        </p>
      ) : null}
      {folders.length === 0 ? null : (
        <ul aria-label={t('homework.picker.folders')} className="flex flex-col border-t border-border">
          {folders.map((child) => (
            <li key={child.id} className="border-b border-border">
              <button
                type="button"
                dir="auto"
                onClick={() => {
                  onOpenFolder({ id: child.id, name: child.name });
                }}
                className="flex min-h-11 w-full items-center px-1 text-start text-small text-strong hover:bg-hover focus-visible:-outline-offset-2"
              >
                {child.name}
              </button>
            </li>
          ))}
        </ul>
      )}
      {templates.length === 0 ? null : (
        <ul ref={listRef} aria-label={t('homework.picker.templates')} className="flex flex-col border-t border-border">
          {templates.map((item) => (
            <TemplateChoice key={item.id} item={item} notReadyHintId={notReadyHintId} onChoose={onChoose} />
          ))}
        </ul>
      )}
      {templates.some((item) => item.ready !== true) ? (
        <p id={notReadyHintId} className="text-small text-text-muted">
          {t('homework.notReady')}
        </p>
      ) : null}
      {listing.hasNextPage ? (
        <p>
          <button
            type="button"
            aria-disabled={listing.isFetchingNextPage}
            aria-busy={listing.isFetchingNextPage}
            onClick={() => {
              if (!listing.isFetchingNextPage) {
                loadingMore();
                void loadNextPage(listing);
              }
            }}
            className={quietButtonClass}
          >
            {t('homework.showMore')}
          </button>
        </p>
      ) : null}
    </div>
  );
}

export function TemplatePicker({
  searchField,
  onChoose,
}: {
  readonly searchField: RefObject<HTMLInputElement | null>;
  readonly onChoose: (template: ChosenTemplate) => void;
}): ReactElement {
  const { t } = useTranslation();
  const inputId = useId();
  const currentStep = useRef<HTMLElement>(null);
  const focusCurrentStep = useRef(false);
  const [typed, setTyped] = useState('');
  const [trail, setTrail] = useState<readonly FolderStep[]>([]);
  const search = useDebouncedValue(searchTerm(typed), searchDelayMilliseconds);
  const folder = search === null ? (trail.at(-1)?.id ?? null) : null;

  useEffect(() => {
    if (focusCurrentStep.current) {
      currentStep.current?.focus();
    }
    focusCurrentStep.current = false;
  });

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <label htmlFor={inputId} className="text-small text-text-muted">
          {t('homework.picker.search')}
        </label>
        <input
          ref={searchField}
          id={inputId}
          type="search"
          dir="auto"
          autoComplete="off"
          maxLength={searchLimit}
          value={typed}
          onChange={(event) => {
            setTyped(event.target.value);
          }}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault();
            }
          }}
          className={fieldClass}
        />
      </div>
      <div className="flex max-h-64 flex-col gap-3 overflow-y-auto">
        {search === null ? (
          <FolderPath
            trail={trail}
            currentStep={currentStep}
            onOpen={(depth) => {
              focusCurrentStep.current = true;
              setTrail(trail.slice(0, depth));
            }}
          />
        ) : null}
        <Listing
          key={`${folder ?? ''}\n${search ?? ''}`}
          folder={folder}
          search={search}
          searchField={searchField}
          onOpenFolder={(step) => {
            focusCurrentStep.current = true;
            setTrail([...trail, step]);
          }}
          onChoose={onChoose}
        />
      </div>
    </div>
  );
}
