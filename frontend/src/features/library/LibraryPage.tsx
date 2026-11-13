import {
  type InfiniteData,
  type UseInfiniteQueryResult,
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import { type DragEvent, type ReactElement, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate, useSearchParams } from 'react-router';

import { useDebouncedValue } from '../../app/clock.ts';
import { ActionMenu } from '../../components/ui/ActionMenu.tsx';
import { type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import {
  ActionButton,
  FormAlert,
  SelectChevron,
  fieldClass,
  linkClass,
  primaryButtonClass,
  quietButtonClass,
  selectClass,
} from '../../components/ui/Form.tsx';
import { MoreIcon } from '../../components/ui/icons.tsx';
import { FocusedNotice } from '../../components/ui/LiveNotice.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { DelayedLoading, LoadFailed, RefetchFailed, focusHeadingIfLost } from '../../components/ui/LoadState.tsx';
import { loadNextPage, useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Page } from '../../components/ui/Page.tsx';
import { refusalText } from '../../components/ui/Refusal.tsx';
import { type ApiError, asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSession, useSignedInAccount } from '../../lib/api/session.ts';
import { downloadAddress } from '../../lib/files/downloadAddress.ts';
import { useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import { NotFoundContent } from '../../routes/NotFoundPage.tsx';
import {
  HomeworkTabs,
  IssueDialogHost,
  type IssuePreset,
  IssuedNotice,
  homeworkTextLoaders,
} from '../homework/IssueAssignment.tsx';
import {
  type FolderNode,
  type Library,
  type LibraryFolder,
  type LibraryItem,
  cutToCodePoints,
  fetchFolders,
  fetchLibraryPage,
  folderTree,
  libraryFoldersKey,
  libraryKey,
  libraryListKey,
  searchTerm,
  titleLimit,
} from './api.ts';
import {
  DeleteDialog,
  type LibraryTarget,
  MoveDialog,
  NameDialog,
  type NameSubject,
  UploadDialog,
  UploadPromptDialog,
  rowKey,
} from './LibraryDialogs.tsx';
import { type RowCommand, type RowHandlers, LibraryTable, folderAddress, templateAddress } from './LibraryTable.tsx';
import { libraryTextLoaders } from './texts.ts';
import { LibraryUpload } from './upload.ts';

const searchDelayMilliseconds = 300;

type OpenDialog =
  | { readonly kind: 'name'; readonly subject: NameSubject }
  | { readonly kind: 'move'; readonly target: LibraryTarget }
  | { readonly kind: 'delete'; readonly target: LibraryTarget }
  | { readonly kind: 'upload'; readonly upload: LibraryUpload }
  | { readonly kind: 'uploadPrompt' };

interface PendingFocus {
  readonly key: string;
  readonly listAt: number;
  readonly foldersAt: number;
}

interface Notice {
  readonly message: 'moved' | 'gone';
  readonly count: number;
  readonly view: string;
}

interface Alert {
  readonly message: string;
  readonly count: number;
  readonly view: string;
}

function libraryNotFound(error: ApiError, searching: boolean): boolean {
  return !searching && (error.code === 'not_found' || error.fieldCode('query.folder') !== undefined);
}

function actionRefusal(t: TFunction, state: ActionState): string | null {
  if (state.kind !== 'refused') {
    return null;
  }
  if (state.error.fieldCode('body') === 'too_many') {
    return t('library.tooMany');
  }
  return refusalText(t, state.error, state.retryAfter, undefined);
}

function startDownload(address: string): void {
  const link = document.createElement('a');
  link.href = address;
  link.download = '';
  link.rel = 'noopener noreferrer';
  document.body.append(link);
  link.click();
  link.remove();
}

function visible(element: HTMLElement | null): element is HTMLElement {
  return element !== null && element.getClientRects().length > 0;
}

function TreeLevel({
  parent,
  children,
  current,
}: {
  readonly parent: string | null;
  readonly children: ReadonlyMap<string | null, readonly LibraryFolder[]>;
  readonly current: string | null;
}): ReactElement | null {
  const level = children.get(parent) ?? [];
  if (level.length === 0) {
    return null;
  }
  return (
    <ul className="flex flex-col gap-1 pl-4">
      {level.map((folder) => (
        <li key={folder.id} className="flex flex-col gap-1">
          <Link
            to={folderAddress(folder.id)}
            data-tree-folder={folder.id}
            aria-current={current === folder.id ? 'page' : undefined}
            dir="auto"
            className="rounded-sm break-words text-text underline-offset-2 hover:underline aria-[current=page]:font-medium aria-[current=page]:text-accent-text"
          >
            {folder.name}
          </Link>
          <TreeLevel parent={folder.id} children={children} current={current} />
        </li>
      ))}
    </ul>
  );
}

function FolderTree({
  folders,
  current,
}: {
  readonly folders: readonly LibraryFolder[];
  readonly current: string | null;
}): ReactElement {
  const { t } = useTranslation();
  const children = new Map<string | null, LibraryFolder[]>();
  for (const folder of folders) {
    children.set(folder.parent_id, [...(children.get(folder.parent_id) ?? []), folder]);
  }
  return (
    <nav aria-label={t('library.folders')} className="hidden flex-col gap-1 desktop:flex">
      <Link
        to={folderAddress(null)}
        aria-current={current === null ? 'page' : undefined}
        className="rounded-sm text-text underline-offset-2 hover:underline aria-[current=page]:font-medium aria-[current=page]:text-accent-text"
      >
        {t('library.root')}
      </Link>
      <TreeLevel parent={null} children={children} current={current} />
    </nav>
  );
}

function FolderChoice({
  nodes,
  current,
}: {
  readonly nodes: readonly FolderNode[];
  readonly current: string | null;
}): ReactElement {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const selectId = useId();
  return (
    <div className="flex flex-col gap-1 desktop:hidden">
      <label htmlFor={selectId} className="text-label">
        {t('library.folderChoice')}
      </label>
      <span className="relative">
        <select
          id={selectId}
          value={current ?? ''}
          onChange={(event) => {
            void navigate(folderAddress(event.target.value === '' ? null : event.target.value));
          }}
          className={selectClass}
        >
          <option value="">{t('library.root')}</option>
          {nodes.map((node) => (
            <option key={node.folder.id} value={node.folder.id}>
              {node.path.map((folder) => folder.name).join(' / ')}
            </option>
          ))}
        </select>
        <SelectChevron />
      </span>
    </div>
  );
}

function Breadcrumbs({ library }: { readonly library: Library }): ReactElement | null {
  const { t } = useTranslation();
  if (library.path.length === 0) {
    return null;
  }
  const ancestors = library.path.slice(0, -1);
  const last = library.path.at(-1);
  return (
    <nav aria-label={t('library.path')}>
      <ol className="flex flex-wrap items-center gap-1 text-small">
        <li>
          <Link to={folderAddress(null)} className={linkClass}>
            {t('library.root')}
          </Link>
        </li>
        {ancestors.map((entry) => (
          <li key={entry.id} className="flex items-center gap-1">
            <span aria-hidden="true">/</span>
            <Link to={folderAddress(entry.id)} dir="auto" className={linkClass}>
              {entry.name}
            </Link>
          </li>
        ))}
        {last === undefined ? null : (
          <li className="flex items-center gap-1">
            <span aria-hidden="true">/</span>
            <span aria-current="page" dir="auto" className="break-words">
              {last.name}
            </span>
          </li>
        )}
      </ol>
    </nav>
  );
}

function SearchField({
  urlQuery,
  onSearch,
}: {
  readonly urlQuery: string | null;
  readonly onSearch: (term: string | null) => void;
}): ReactElement {
  const { t } = useTranslation();
  const inputId = useId();
  const [typed, setTyped] = useState(urlQuery ?? '');
  const [synced, setSynced] = useState(urlQuery);
  const settled = useDebouncedValue(typed, searchDelayMilliseconds);
  const latestQuery = useRef(urlQuery);
  const latestSearch = useRef(onSearch);
  if (synced !== urlQuery) {
    setSynced(urlQuery);
    if (searchTerm(typed) !== urlQuery) {
      setTyped(urlQuery ?? '');
    }
  }

  useEffect(() => {
    latestQuery.current = urlQuery;
    latestSearch.current = onSearch;
  });

  useEffect(() => {
    const term = searchTerm(settled);
    if (term !== latestQuery.current) {
      latestSearch.current(term);
    }
  }, [settled]);

  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={inputId} className="sr-only">
        {t('library.search')}
      </label>
      <input
        id={inputId}
        type="search"
        value={typed}
        placeholder={t('library.search')}
        autoComplete="off"
        onChange={(event) => {
          setTyped(event.target.value);
          if (searchTerm(event.target.value) === null && urlQuery !== null) {
            onSearch(null);
          }
        }}
        className={fieldClass}
      />
    </div>
  );
}

function EmptyLibrary({
  createButton,
  onChoose,
  onFile,
}: {
  readonly createButton: ReactElement;
  readonly onChoose: () => void;
  readonly onFile: (file: File) => void;
}): ReactElement {
  const { t } = useTranslation();
  const [over, setOver] = useState(false);

  function drop(event: DragEvent<HTMLElement>): void {
    event.preventDefault();
    setOver(false);
    const file = event.dataTransfer.files.item(0);
    if (file !== null) {
      onFile(file);
    }
  }

  return (
    <section className="flex flex-col items-start gap-3">
      <h2 className="text-heading">{t('library.emptyTitle')}</h2>
      <p className="text-text-muted">{t('library.emptyBody')}</p>
      {createButton}
      <button
        type="button"
        data-drop-zone=""
        onClick={onChoose}
        onDragOver={(event): void => {
          if (event.dataTransfer.types.includes('Files')) {
            event.preventDefault();
            setOver(true);
          }
        }}
        onDragLeave={(): void => {
          setOver(false);
        }}
        onDrop={drop}
        className={`flex min-h-24 w-full items-center justify-center rounded-lg border border-dashed border-border-strong p-6 text-text-muted hover:bg-surface ${over ? 'bg-surface' : ''}`}
      >
        {t('library.upload')}
      </button>
    </section>
  );
}

function ListBody({
  list,
  searching,
  folders,
  items,
  empty,
  handlers,
}: {
  readonly list: UseInfiniteQueryResult<InfiniteData<Library>>;
  readonly searching: boolean;
  readonly folders: readonly LibraryFolder[];
  readonly items: readonly LibraryItem[];
  readonly empty: ReactElement | null;
  readonly handlers: RowHandlers;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const rowKeys = [
    ...folders.map((child) => rowKey({ kind: 'folder', folder: child })),
    ...items.map((item) => rowKey({ kind: 'item', item })),
  ];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLTableSectionElement>(
    rowKeys.length,
    '[data-row-link]',
    list.isFetching,
  );
  useFocusAfterRemovedRow(listRef, rowKeys, '[data-row-link]');

  if (list.isPending) {
    return <DelayedLoading />;
  }
  if (list.data === undefined) {
    return (
      <LoadFailed
        error={asApiError(list.error)}
        onRetry={() => {
          void list.refetch();
        }}
      />
    );
  }
  if (empty !== null) {
    return empty;
  }
  if (folders.length === 0 && items.length === 0) {
    return <p className="text-text-muted">{searching ? t('library.nothingFound') : t('library.emptyFolder')}</p>;
  }
  return (
    <div className="flex flex-col gap-4">
      <LibraryTable
        label={searching ? t('library.searchResults') : t('library.list')}
        folders={folders}
        items={items}
        me={me}
        bodyRef={listRef}
        handlers={handlers}
      />
      {list.hasNextPage ? (
        <div>
          <ActionButton
            busy={list.isFetchingNextPage}
            onClick={() => {
              loadingMore();
              void loadNextPage(list);
            }}
            className={quietButtonClass}
          >
            {t('library.showMore')}
          </ActionButton>
        </div>
      ) : null}
    </div>
  );
}

export function LibraryPage(): ReactElement {
  const { t } = useTranslation();
  const texts = useFeatureTexts('library', libraryTextLoaders);
  const homeworkTexts = useFeatureTexts('homework', homeworkTextLoaders);
  const me = useSignedInAccount();
  const session = useSession();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const folder = params.get('folder');
  const urlQuery = params.get('q');
  const search = urlQuery === null ? null : searchTerm(urlQuery);
  const searching = search !== null;
  const listFolder = searching ? null : folder;
  const listKey = libraryListKey(listFolder, search);
  const opener = useRef<HTMLElement | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const menuButton = useRef<HTMLButtonElement>(null);
  const [dialog, setDialog] = useState<OpenDialog | null>(null);
  const pendingFocus = useRef<PendingFocus | null>(null);
  const [focusRequest, setFocusRequest] = useState(0);
  const [notice, setNotice] = useState<Notice | null>(null);
  const [alert, setAlert] = useState<Alert | null>(null);
  const [issuing, setIssuing] = useState<IssuePreset | null>(null);
  const [issued, setIssued] = useState<{ readonly issueId: string; readonly count: number } | null>(null);
  const [busyRow, setBusyRow] = useState<string | null>(null);
  const rowAction = useAction();
  const currentUpload = useRef<LibraryUpload | null>(null);
  const shownView = useRef<string | null>(null);
  const createAction = useAction();

  const list = useInfiniteQuery({
    queryKey: listKey,
    queryFn: ({ pageParam, signal }) => fetchLibraryPage(client, { folder: listFolder, search }, pageParam, signal),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.next_cursor,
    retry: false,
  });
  const folders = useQuery({
    queryKey: libraryFoldersKey,
    queryFn: ({ signal }) => fetchFolders(client, signal),
    retry: false,
  });
  const nodes = folderTree(folders.data ?? []);
  const firstPage = list.data?.pages[0];
  const childFolders = searching ? [] : (firstPage?.folders ?? []);
  const items: readonly LibraryItem[] = list.data?.pages.flatMap((page) => page.items) ?? [];
  const listError = list.isError ? asApiError(list.error) : null;
  const notFound = listError !== null && list.data === undefined && libraryNotFound(listError, searching);
  const viewKey = `${listFolder ?? ''}\n${search ?? ''}`;

  useEffect(() => {
    const pending = pendingFocus.current;
    if (pending === null || focusRequest === 0) {
      return;
    }
    if (list.dataUpdatedAt < pending.listAt || folders.dataUpdatedAt < pending.foldersAt) {
      return;
    }
    pendingFocus.current = null;
    const row = document.querySelector<HTMLElement>(
      `main tbody [data-row="${CSS.escape(pending.key)}"] [data-row-link]`,
    );
    const treeItem = pending.key.startsWith('folder:')
      ? document.querySelector<HTMLElement>(`[data-tree-folder="${CSS.escape(pending.key.slice('folder:'.length))}"]`)
      : null;
    const target = row ?? (visible(treeItem) ? treeItem : null);
    if (target === null) {
      focusHeadingIfLost();
    } else {
      target.focus();
    }
  }, [focusRequest, list.dataUpdatedAt, folders.dataUpdatedAt]);

  useEffect(() => {
    if (shownView.current === null || shownView.current === viewKey) {
      shownView.current = viewKey;
      return undefined;
    }
    shownView.current = viewKey;
    const frame = window.requestAnimationFrame(focusHeadingIfLost);
    return (): void => {
      window.cancelAnimationFrame(frame);
    };
  }, [viewKey]);

  useEffect(
    () => (): void => {
      currentUpload.current?.cancel();
    },
    [],
  );

  function rememberOpener(): void {
    opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  }

  function focusAfterRefetch(key: string): void {
    pendingFocus.current = {
      key,
      listAt: queryClient.getQueryState(listKey)?.dataUpdatedAt ?? 0,
      foldersAt: queryClient.getQueryState(libraryFoldersKey)?.dataUpdatedAt ?? 0,
    };
    setFocusRequest((count) => count + 1);
  }

  function showNotice(message: Notice['message']): void {
    setAlert(null);
    setIssued(null);
    setNotice((previous) => ({ message, count: (previous?.count ?? 0) + 1, view: viewKey }));
  }

  function showAlert(message: string): void {
    setNotice(null);
    setIssued(null);
    setAlert((previous) => ({ message, count: (previous?.count ?? 0) + 1, view: viewKey }));
  }

  function closeDialog(): void {
    if (dialog?.kind === 'upload') {
      dialog.upload.cancel();
    }
    setDialog(null);
  }

  function itemGone(): void {
    closeDialog();
    void queryClient.invalidateQueries({ queryKey: libraryKey });
    showNotice('gone');
  }

  function savedTo(key: string): void {
    setDialog(null);
    setNotice(null);
    setAlert(null);
    focusAfterRefetch(key);
  }

  function startUpload(file: File): void {
    rememberOpener();
    if (opener.current === null || !opener.current.isConnected || opener.current === document.body) {
      opener.current = menuButton.current;
    }
    const upload = new LibraryUpload(client, session, file);
    currentUpload.current = upload;
    setDialog({ kind: 'upload', upload });
  }

  function acceptFile(file: File): void {
    if (me.state !== 'active') {
      rememberOpener();
      setDialog({ kind: 'uploadPrompt' });
      return;
    }
    startUpload(file);
  }

  function chooseUpload(): void {
    rememberOpener();
    if (me.state !== 'active') {
      setDialog({ kind: 'uploadPrompt' });
      return;
    }
    fileInput.current?.click();
  }

  function applySearch(term: string | null): void {
    setParams(
      (previous) => {
        const next = new URLSearchParams(previous);
        if (term === null) {
          next.delete('q');
        } else {
          next.set('q', term);
        }
        return next;
      },
      { replace: true },
    );
  }

  function createAssignment(): void {
    const idempotencyKey = crypto.randomUUID();
    createAction.run(async (): Promise<ActionState> => {
      const { data, error, response } = await client.POST('/api/v1/library/templates', {
        params: { header: { 'Idempotency-Key': idempotencyKey } },
        body: { title: t('library.newAssignment'), folder_id: folder, blocks: [] },
      });
      if (data === undefined) {
        const refused = refusedBy(response, error);
        if (response.status === 404) {
          itemGone();
        } else {
          showAlert(actionRefusal(t, refused) ?? '');
        }
        return refused;
      }
      void queryClient.invalidateQueries({ queryKey: libraryKey, refetchType: 'none' });
      void navigate(templateAddress(data.id));
      return done;
    });
  }

  function openMaterial(item: LibraryItem): void {
    const attachmentId = item.attachment_id;
    if (attachmentId === undefined) {
      showAlert(t('errors.internal'));
      return;
    }
    setBusyRow(rowKey({ kind: 'item', item }));
    rowAction.run(async (): Promise<ActionState> => {
      const { data, error, response } = await client.GET('/api/v1/attachments/{id}', {
        params: {
          path: { id: attachmentId },
          query: { ref: `material:${item.id}`, disposition: 'attachment' },
        },
      });
      setBusyRow(null);
      if (data === undefined) {
        const refused = refusedBy(response, error);
        if (response.status === 404) {
          itemGone();
        } else {
          showAlert(actionRefusal(t, refused) ?? '');
        }
        return refused;
      }
      const address = downloadAddress(attachmentId, data.url, window.location.origin);
      if (address === null) {
        showAlert(t('errors.internal'));
        return done;
      }
      startDownload(address);
      return done;
    });
  }

  function duplicateTemplate(item: LibraryItem): void {
    const key = rowKey({ kind: 'item', item });
    setBusyRow(key);
    rowAction.run(async (): Promise<ActionState> => {
      const { data, error, response } = await client.POST('/api/v1/library/templates/{id}/duplicate', {
        params: { path: { id: item.id } },
        body: {
          title: cutToCodePoints(t('library.copyTitle', { title: item.title }), titleLimit),
          folder_id: item.folder_id,
        },
      });
      if (data === undefined) {
        setBusyRow(null);
        const refused = refusedBy(response, error);
        if (response.status === 404) {
          itemGone();
        } else {
          showAlert(actionRefusal(t, refused) ?? '');
        }
        return refused;
      }
      await queryClient.invalidateQueries({ queryKey: libraryKey });
      setBusyRow(null);
      setNotice(null);
      setAlert(null);
      focusAfterRefetch(`template:${data.id}`);
      return done;
    });
  }

  function onCommand(command: RowCommand, target: LibraryTarget): void {
    if (command === 'open' && target.kind === 'item') {
      if (target.item.kind === 'material') {
        openMaterial(target.item);
      } else {
        void navigate(templateAddress(target.item.id));
      }
      return;
    }
    if (command === 'duplicate' && target.kind === 'item') {
      duplicateTemplate(target.item);
      return;
    }
    rememberOpener();
    if (command === 'issue' && target.kind === 'item') {
      setNotice(null);
      setAlert(null);
      setIssued(null);
      setIssuing({ template: { id: target.item.id, title: target.item.title } });
      return;
    }
    if (command === 'rename') {
      setDialog({ kind: 'name', subject: { kind: 'rename', target } });
    } else if (command === 'move') {
      setDialog({ kind: 'move', target });
    } else if (command === 'delete') {
      setDialog({ kind: 'delete', target });
    }
  }

  const createButton = (
    <ActionButton busy={createAction.busy} onClick={createAssignment} className={primaryButtonClass}>
      {t('library.newAssignment')}
    </ActionButton>
  );
  const rootEmpty =
    !searching &&
    folder === null &&
    list.isSuccess &&
    childFolders.length === 0 &&
    items.length === 0 &&
    !list.hasNextPage;

  if (!texts.ready) {
    return (
      <Page>
        {texts.failed ? <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} /> : <DelayedLoading />}
      </Page>
    );
  }

  if (notFound) {
    return (
      <Page title="page.notFound" focusTitle>
        <NotFoundContent />
      </Page>
    );
  }

  return (
    <Page
      action={rootEmpty ? null : createButton}
      menu={
        <ActionMenu
          label={t('library.moreActions')}
          unavailable={false}
          buttonRef={menuButton}
          items={[
            { label: t('library.upload'), onChoose: chooseUpload },
            {
              label: t('library.newFolder'),
              onChoose: (): void => {
                rememberOpener();
                setDialog({ kind: 'name', subject: { kind: 'newFolder', parentId: folder } });
              },
            },
          ]}
        >
          <MoreIcon />
        </ActionMenu>
      }
    >
      <input
        ref={fileInput}
        type="file"
        tabIndex={-1}
        aria-hidden="true"
        className="sr-only"
        onChange={(event) => {
          const file = event.target.files?.item(0) ?? null;
          event.target.value = '';
          if (file !== null) {
            acceptFile(file);
          }
        }}
      />
      {homeworkTexts.ready ? <HomeworkTabs current="library" /> : null}
      {issued === null ? null : <IssuedNotice key={issued.count} issueId={issued.issueId} />}
      {notice?.view !== viewKey ? null : (
        <FocusedNotice
          key={notice.count}
          message={notice.message === 'moved' ? t('library.moved') : t('library.gone')}
        />
      )}
      {alert?.view !== viewKey ? null : <FormAlert key={alert.count}>{alert.message}</FormAlert>}
      <div className="flex flex-col gap-6 wide:grid wide:grid-cols-[var(--container-aside)_minmax(0,1fr)] wide:items-start wide:gap-10">
        <FolderTree folders={folders.data ?? []} current={searching ? null : folder} />
        <div className="flex min-w-0 flex-col gap-4">
          <FolderChoice nodes={nodes} current={searching ? null : folder} />
          <SearchField urlQuery={search} onSearch={applySearch} />
          {searching || firstPage === undefined ? null : <Breadcrumbs library={firstPage} />}
          {list.isError && list.data !== undefined ? (
            <RefetchFailed
              error={asApiError(list.error)}
              onRetry={() => {
                void list.refetch();
              }}
            />
          ) : null}
          <ListBody
            key={viewKey}
            list={list}
            searching={searching}
            folders={childFolders}
            items={items}
            empty={
              rootEmpty ? (
                <EmptyLibrary createButton={createButton} onChoose={chooseUpload} onFile={acceptFile} />
              ) : null
            }
            handlers={{ busyRow, issuable: me.email_verified && homeworkTexts.ready, onCommand }}
          />
        </div>
      </div>
      <NameDialog
        subject={dialog?.kind === 'name' ? dialog.subject : null}
        returnFocus={opener}
        onClose={closeDialog}
        onGone={itemGone}
        onSaved={savedTo}
      />
      <MoveDialog
        target={dialog?.kind === 'move' ? dialog.target : null}
        nodes={nodes}
        returnFocus={opener}
        onClose={closeDialog}
        onGone={itemGone}
        onMoved={() => {
          setDialog(null);
          showNotice('moved');
        }}
      />
      <DeleteDialog
        target={dialog?.kind === 'delete' ? dialog.target : null}
        returnFocus={opener}
        onClose={closeDialog}
        onGone={itemGone}
        onDeleted={() => {
          setDialog(null);
          setNotice(null);
          setAlert(null);
          void queryClient.invalidateQueries({ queryKey: libraryKey });
        }}
      />
      <UploadDialog
        upload={dialog?.kind === 'upload' ? dialog.upload : null}
        folderId={folder}
        returnFocus={opener}
        onClose={closeDialog}
        onGone={itemGone}
        onSaved={savedTo}
      />
      <UploadPromptDialog open={dialog?.kind === 'uploadPrompt'} returnFocus={opener} onClose={closeDialog} />
      {homeworkTexts.ready ? (
        <IssueDialogHost
          open={issuing !== null}
          preset={issuing ?? {}}
          returnFocus={opener}
          onClose={() => {
            setIssuing(null);
          }}
          onIssued={(issue) => {
            setIssuing(null);
            setIssued((previous) => ({ issueId: issue.id, count: (previous?.count ?? 0) + 1 }));
          }}
        />
      ) : null}
    </Page>
  );
}
