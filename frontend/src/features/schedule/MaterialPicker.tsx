import { useInfiniteQuery } from '@tanstack/react-query';
import { type ReactElement, type RefObject, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useDebouncedValue } from '../../app/clock.ts';
import { fieldClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { loadNextPage, useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { searchDelayMilliseconds } from './AudiencePicker.tsx';
import { type LibraryItem, fetchLibraryPage, searchLimit, searchTerm } from '../library/api.ts';
import { useMaterialTexts } from './materialTexts.ts';
import { type PickedMaterial, lessonMaterialLimit, libraryPickerKey, pickedFromLibrary } from './materials.ts';

interface FolderStep {
  readonly id: string;
  readonly name: string;
}

interface Choice {
  readonly chosen: readonly PickedMaterial[];
  readonly limitHintId: string;
  readonly onToggle: (material: PickedMaterial, checked: boolean) => void;
}

function MaterialChoice({
  material,
  location,
  choice,
}: {
  readonly material: PickedMaterial;
  readonly location: string | null;
  readonly choice: Choice;
}): ReactElement {
  const checked = choice.chosen.some((candidate) => candidate.id === material.id);
  const full = !checked && choice.chosen.length >= lessonMaterialLimit;
  const titleId = useId();
  const detailId = useId();
  return (
    <li className="border-b border-border">
      <label className="flex min-h-11 items-center gap-3 px-1 text-small">
        <input
          type="checkbox"
          checked={checked}
          aria-labelledby={titleId}
          aria-describedby={full ? `${detailId} ${choice.limitHintId}` : detailId}
          aria-disabled={full ? true : undefined}
          onChange={(event) => {
            if (!full) {
              choice.onToggle(material, event.target.checked);
            }
          }}
          className="size-4 shrink-0 accent-accent"
        />
        <span className="flex min-w-0 flex-col">
          <span id={titleId} dir="auto" className="truncate text-strong">
            {material.title}
          </span>
          <span id={detailId} className="truncate text-text-muted">
            <span dir="auto">{material.filename}</span>
            {location === null ? null : (
              <>
                {' · '}
                <span dir="auto">{location}</span>
              </>
            )}
          </span>
        </span>
      </label>
    </li>
  );
}

function locationOf(item: LibraryItem): string | null {
  return item.path === null || item.path.length === 0 ? null : item.path.map((step) => step.name).join(' / ');
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
  const texts = useMaterialTexts();
  const steps = [{ id: null, name: texts.libraryRoot }, ...trail];
  return (
    <ol aria-label={texts.libraryPath} className="flex flex-wrap items-center gap-x-2 text-small">
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

function LibraryListing({
  folder,
  search,
  listedAtOpening,
  searchField,
  choice,
  onOpenFolder,
}: {
  readonly folder: string | null;
  readonly search: string | null;
  readonly listedAtOpening: readonly PickedMaterial[];
  readonly searchField: RefObject<HTMLInputElement | null>;
  readonly choice: Choice;
  readonly onOpenFolder: (step: FolderStep) => void;
}): ReactElement {
  const { t } = useTranslation();
  const texts = useMaterialTexts();
  const client = useApiClient();
  const [shownAbove] = useState(() => new Set(listedAtOpening.map((material) => material.id)));
  const listing = useInfiniteQuery({
    queryKey: libraryPickerKey(folder, search),
    queryFn: ({ pageParam, signal }) => fetchLibraryPage(client, { folder, search }, pageParam, signal),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    retry: false,
  });
  const libraryMaterials = (listing.data?.pages.flatMap((page) => page.items) ?? []).filter(
    (item) => item.kind === 'material',
  );
  const materials = libraryMaterials.filter((item) => !shownAbove.has(item.id));
  const folders = search === null ? (listing.data?.pages[0]?.folders ?? []) : [];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(
    materials.length,
    ':scope > li input',
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
  const empty = folders.length === 0 && libraryMaterials.length === 0 && !listing.hasNextPage;
  return (
    <div className="flex flex-col gap-2">
      {empty ? (
        <p className="text-small text-text-muted">
          {search === null ? texts.noLibraryMaterials : t('newLesson.nothingFound')}
        </p>
      ) : null}
      {folders.length === 0 ? null : (
        <ul aria-label={texts.libraryFolders} className="flex flex-col border-t border-border">
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
      {materials.length === 0 ? null : (
        <ul ref={listRef} aria-label={texts.materials} className="flex flex-col border-t border-border">
          {materials.map((item) => (
            <MaterialChoice
              key={item.id}
              material={pickedFromLibrary(item)}
              location={search === null ? null : locationOf(item)}
              choice={choice}
            />
          ))}
        </ul>
      )}
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
            {t('lessons.showMore')}
          </button>
        </p>
      ) : null}
    </div>
  );
}

export function MaterialPicker({
  chosen,
  searchField,
  onChange,
}: {
  readonly chosen: readonly PickedMaterial[];
  readonly searchField: RefObject<HTMLInputElement | null>;
  readonly onChange: (chosen: readonly PickedMaterial[]) => void;
}): ReactElement {
  const { t } = useTranslation();
  const texts = useMaterialTexts();
  const inputId = useId();
  const countId = useId();
  const limitHintId = useId();
  const currentStep = useRef<HTMLElement>(null);
  const focusCurrentStep = useRef(false);
  const [typed, setTyped] = useState('');
  const [trail, setTrail] = useState<readonly FolderStep[]>([]);
  const [listed, setListed] = useState<readonly PickedMaterial[]>(chosen);
  const search = useDebouncedValue(searchTerm(typed), searchDelayMilliseconds);
  const folder = search === null ? (trail.at(-1)?.id ?? null) : null;

  useEffect(() => {
    if (focusCurrentStep.current) {
      currentStep.current?.focus();
    }
    focusCurrentStep.current = false;
  });

  const choice: Choice = {
    chosen,
    limitHintId,
    onToggle: (material, checked) => {
      if (checked && !listed.some((candidate) => candidate.id === material.id)) {
        setListed([...listed, material]);
      }
      onChange(
        checked
          ? [...chosen.filter((candidate) => candidate.id !== material.id), material]
          : chosen.filter((candidate) => candidate.id !== material.id),
      );
    },
  };

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <label htmlFor={inputId} className="text-label">
          {texts.librarySearch}
        </label>
        <input
          ref={searchField}
          id={inputId}
          type="search"
          dir="auto"
          autoComplete="off"
          maxLength={searchLimit}
          value={typed}
          aria-describedby={countId}
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
      <p id={countId} className="text-small text-text-muted">
        {t('newLesson.materialsChosen', { count: chosen.length, limit: lessonMaterialLimit })}
      </p>
      {chosen.length >= lessonMaterialLimit ? (
        <p id={limitHintId} className="text-small text-text-muted">
          {texts.materialsLimit}
        </p>
      ) : null}
      <div className="flex max-h-64 flex-col gap-3 overflow-y-auto">
        {listed.length === 0 ? null : (
          <ul aria-label={t('newLesson.preselected')} className="flex flex-col border-t border-border">
            {listed.map((material) => (
              <MaterialChoice key={material.id} material={material} location={null} choice={choice} />
            ))}
          </ul>
        )}
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
        <LibraryListing
          key={`${folder ?? ''}\n${search ?? ''}`}
          folder={folder}
          search={search}
          listedAtOpening={listed}
          searchField={searchField}
          choice={choice}
          onOpenFolder={(step) => {
            focusCurrentStep.current = true;
            setTrail([...trail, step]);
          }}
        />
      </div>
    </div>
  );
}

export function MaterialsField({
  chosen,
  onChange,
}: {
  readonly chosen: readonly PickedMaterial[];
  readonly onChange: (chosen: readonly PickedMaterial[]) => void;
}): ReactElement {
  const texts = useMaterialTexts();
  const legendId = useId();
  const searchField = useRef<HTMLInputElement>(null);
  const focusOnOpen = useRef(false);
  const [opened, setOpened] = useState(false);

  useEffect(() => {
    if (focusOnOpen.current) {
      searchField.current?.focus();
    }
    focusOnOpen.current = false;
  });

  return (
    <fieldset aria-labelledby={legendId} className="flex flex-col gap-2">
      <legend id={legendId} className="text-label">
        {texts.materials}
      </legend>
      {opened ? (
        <MaterialPicker chosen={chosen} searchField={searchField} onChange={onChange} />
      ) : (
        <>
          {chosen.length === 0 ? (
            <p className="text-small text-text-muted">{texts.noMaterials}</p>
          ) : (
            <ul className="flex flex-col gap-1 text-small">
              {chosen.map((material) => (
                <li key={material.id} dir="auto" className="break-words">
                  {material.title}
                </li>
              ))}
            </ul>
          )}
          <p>
            <button
              type="button"
              onClick={() => {
                focusOnOpen.current = true;
                setOpened(true);
              }}
              className={quietButtonClass}
            >
              {texts.attachMaterial}
            </button>
          </p>
        </>
      )}
    </fieldset>
  );
}
