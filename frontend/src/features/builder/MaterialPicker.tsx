import { useInfiniteQuery } from '@tanstack/react-query';
import { type ReactElement, type RefObject, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useDebouncedValue } from '../../app/clock.ts';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import { ActionButton, fieldClass, quietButtonClass, textButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { type LibraryItem, fetchLibraryPage, searchTerm } from '../library/api.ts';

const searchDelayMilliseconds = 300;

export interface ChosenMaterial {
  readonly materialId: string;
  readonly attachmentId: string;
  readonly title: string;
  readonly filename: string;
}

function chosen(item: LibraryItem): ChosenMaterial | null {
  if (item.kind !== 'material' || item.attachment_id === undefined) {
    return null;
  }
  return { materialId: item.id, attachmentId: item.attachment_id, title: item.title, filename: item.filename ?? '' };
}

function PickerContent({
  headingId,
  searchField,
  onChoose,
  onCancel,
}: {
  readonly headingId: string;
  readonly searchField: RefObject<HTMLInputElement | null>;
  readonly onChoose: (material: ChosenMaterial) => void;
  readonly onCancel: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const heading = useRef<HTMLHeadingElement>(null);
  const [folder, setFolder] = useState<string | null>(null);
  const [typed, setTyped] = useState('');
  const settled = useDebouncedValue(typed, searchDelayMilliseconds);
  const search = searchTerm(settled);
  const [focusHeading, setFocusHeading] = useState(0);
  const pendingMore = useRef<number | null>(null);

  const list = useInfiniteQuery({
    queryKey: ['builder', 'picker', search === null ? folder : null, search],
    queryFn: ({ pageParam, signal }) =>
      fetchLibraryPage(client, { folder: search === null ? folder : null, search }, pageParam, signal),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.next_cursor,
    retry: false,
  });

  useEffect(() => {
    if (focusHeading > 0) {
      heading.current?.focus();
    }
  }, [focusHeading]);

  function open(folderId: string | null): void {
    setFolder(folderId);
    setFocusHeading((value) => value + 1);
  }

  useEffect(() => {
    const shown = pendingMore.current;
    if (shown === null || list.isFetchingNextPage) {
      return;
    }
    pendingMore.current = null;
    const firstNew = document.getElementById(`${headingId}-material-${String(shown)}`);
    if (firstNew !== null) {
      firstNew.focus();
      return;
    }
    if (document.activeElement === null || document.activeElement === document.body) {
      (document.getElementById(`${headingId}-material-${String(shown - 1)}`) ?? searchField.current)?.focus();
    }
  }, [list.data, list.isFetchingNextPage, headingId, searchField]);

  function rowId(index: number): string {
    return `${headingId}-material-${String(index)}`;
  }

  const first = list.data?.pages[0];
  const materials = (list.data?.pages ?? []).flatMap((page) => page.items).filter((item) => item.kind === 'material');
  const folders = search === null ? (first?.folders ?? []) : [];
  const path = search === null ? (first?.path ?? []) : [];

  return (
    <div className="flex flex-col gap-4">
      <h2 ref={heading} id={headingId} tabIndex={-1} className="font-serif text-title outline-none">
        {t('builder.pickerTitle')}
      </h2>
      <div className="flex flex-col gap-1">
        <label htmlFor={`${headingId}-search`}>{t('builder.pickerSearch')}</label>
        <input
          ref={searchField}
          id={`${headingId}-search`}
          type="search"
          value={typed}
          dir="auto"
          onChange={(event) => {
            setTyped(event.target.value);
          }}
          className={fieldClass}
        />
      </div>
      {search === null ? (
        <nav aria-label={t('builder.pickerPath')}>
          <ol className="flex flex-wrap items-center gap-1">
            <li>
              {folder === null ? (
                <span className="text-strong">{t('builder.root')}</span>
              ) : (
                <button
                  type="button"
                  onClick={() => {
                    open(null);
                  }}
                  className={textButtonClass}
                >
                  {t('builder.root')}
                </button>
              )}
            </li>
            {path.map((entry, index) => (
              <li key={entry.id} className="flex items-center gap-1">
                <span aria-hidden="true">/</span>
                {index === path.length - 1 ? (
                  <span dir="auto" className="text-strong break-words">
                    {entry.name}
                  </span>
                ) : (
                  <button
                    type="button"
                    dir="auto"
                    onClick={() => {
                      open(entry.id);
                    }}
                    className={textButtonClass}
                  >
                    {entry.name}
                  </button>
                )}
              </li>
            ))}
          </ol>
        </nav>
      ) : null}
      {list.isPending ? <DelayedLoading /> : null}
      {list.isError ? (
        <LoadFailed
          error={asApiError(list.error)}
          onRetry={() => {
            void list.refetch();
          }}
          focusIfLost={() => {
            searchField.current?.focus();
          }}
        />
      ) : null}
      {folders.length === 0 ? null : (
        <ul aria-label={t('builder.pickerFolders')} className="flex flex-col">
          {folders.map((entry) => (
            <li key={entry.id}>
              <button
                type="button"
                dir="auto"
                onClick={() => {
                  open(entry.id);
                }}
                className={`${textButtonClass} text-body`}
              >
                {entry.name}
              </button>
            </li>
          ))}
        </ul>
      )}
      {list.isSuccess && materials.length === 0 && !list.hasNextPage ? (
        <p className="text-text-muted">
          {search === null ? t('builder.pickerEmpty') : t('builder.pickerNothingFound')}
        </p>
      ) : null}
      {materials.length === 0 ? null : (
        <ul aria-label={t('builder.pickerMaterials')} className="flex max-h-96 flex-col gap-1 overflow-y-auto">
          {materials.map((item, index) => {
            const material = chosen(item);
            if (material === null) {
              return null;
            }
            const folderPath = (item.path ?? []).map((entry) => entry.name).join(' / ');
            return (
              <li key={item.id}>
                <button
                  id={rowId(index)}
                  type="button"
                  onClick={() => {
                    onChoose(material);
                  }}
                  className="flex min-h-11 w-full flex-col items-start rounded-md px-3 py-2 text-start hover:bg-hover"
                >
                  <span dir="auto" className="text-strong break-words">
                    {item.title}
                  </span>
                  <span dir="auto" className="text-small break-words text-text-muted">
                    {search === null || folderPath === '' ? material.filename : `${material.filename} · ${folderPath}`}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
      <div className="flex flex-wrap gap-2">
        {list.hasNextPage ? (
          <ActionButton
            busy={list.isFetchingNextPage}
            onClick={() => {
              pendingMore.current = materials.length;
              void list.fetchNextPage();
            }}
            className={quietButtonClass}
          >
            {t('builder.showMore')}
          </ActionButton>
        ) : null}
        <ActionButton busy={false} onClick={onCancel} className={quietButtonClass}>
          {t('builder.cancel')}
        </ActionButton>
      </div>
    </div>
  );
}

export function MaterialPicker({
  open,
  returnFocus,
  onChoose,
  onClose,
}: {
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onChoose: (material: ChosenMaterial) => void;
  readonly onClose: () => void;
}): ReactElement {
  const headingId = useId();
  const searchField = useRef<HTMLInputElement>(null);
  return (
    <ModalDialog
      open={open}
      busy={false}
      labelledBy={headingId}
      returnFocus={returnFocus}
      initialFocus={searchField}
      onClose={onClose}
    >
      <PickerContent headingId={headingId} searchField={searchField} onChoose={onChoose} onCancel={onClose} />
    </ModalDialog>
  );
}
