import { type ReactElement, type RefObject, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { ActionMenu, type MenuItem } from '../../components/ui/ActionMenu.tsx';
import { quietButtonClass } from '../../components/ui/Form.tsx';
import { useReturnFocus } from '../../components/ui/formRoute.ts';
import {
  DataTable,
  type TableColumn,
  endCellClass,
  narrowOnlyClass,
  primaryCellClass,
  rowLinkClass,
  secondaryCellClass,
  tableRowClass,
} from '../../components/ui/DataTable.tsx';
import { AttachIcon, MoreIcon } from '../../components/ui/icons.tsx';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { formatDate } from '../../lib/format/dates.ts';
import type { Me } from '../../lib/api/session.ts';
import { formatStorage } from '../settings/storageAmount.ts';
import type { LibraryFolder, LibraryItem, LibraryPathEntry } from './api.ts';
import { type LibraryTarget, type RowControl, libraryRowOpener, rowKey } from './LibraryDialogs.tsx';

export type RowCommand = 'open' | 'issue' | 'rename' | 'move' | 'duplicate' | 'delete';

type VisibleCommand = 'open' | 'issue';

export interface RowHandlers {
  readonly busyRow: string | null;
  readonly issuable: boolean;
  readonly onCommand: (command: RowCommand, target: LibraryTarget, control: RowControl) => void;
}

export function folderAddress(folderId: string | null): string {
  return folderId === null ? '/homework/library' : `/homework/library?folder=${encodeURIComponent(folderId)}`;
}

export function templateAddress(templateId: string): string {
  return `/homework/templates/${encodeURIComponent(templateId)}`;
}

function Names({ names }: { readonly names: readonly string[] }): ReactElement {
  return (
    <>
      {names.map((name, index) => (
        <span key={`${String(index)}-${name}`}>
          {index === 0 ? null : ' / '}
          <span dir="auto">{name}</span>
        </span>
      ))}
    </>
  );
}

function RowMenu({
  target,
  label,
  commands,
  handlers,
}: {
  readonly target: LibraryTarget;
  readonly label: string;
  readonly commands: readonly RowCommand[];
  readonly handlers: RowHandlers;
}): ReactElement {
  const { t } = useTranslation();
  const key = rowKey(target);
  const menuButton = useRef<HTMLButtonElement>(null);
  useReturnFocus(libraryRowOpener(key, 'menu'), menuButton);
  const items: readonly MenuItem[] = commands.map((command) => ({
    label: command === 'issue' ? t('homework.issue') : t(`library.${command}`),
    danger: command === 'delete',
    onChoose: (): void => {
      handlers.onCommand(command, target, 'menu');
    },
  }));
  return (
    <span className="flex justify-end">
      <ActionMenu
        label={t('library.rowActions', { title: label })}
        busy={handlers.busyRow === key}
        unavailable={handlers.busyRow !== null}
        items={items}
        buttonRef={menuButton}
      >
        <MoreIcon />
      </ActionMenu>
    </span>
  );
}

function VisibleAction({
  target,
  label,
  command,
  handlers,
}: {
  readonly target: LibraryTarget;
  readonly label: string;
  readonly command: VisibleCommand;
  readonly handlers: RowHandlers;
}): ReactElement {
  const { t } = useTranslation();
  const key = rowKey(target);
  const button = useRef<HTMLButtonElement>(null);
  useReturnFocus(libraryRowOpener(key, 'visible'), button);
  const unavailable = handlers.busyRow !== null;
  return (
    <span className="hidden tablet:inline-flex">
      <button
        ref={button}
        type="button"
        data-row-action={command}
        aria-disabled={unavailable ? true : undefined}
        onClick={() => {
          if (!unavailable) {
            handlers.onCommand(command, target, 'visible');
          }
        }}
        className={quietButtonClass}
      >
        {command === 'issue' ? t('library.issueRow') : t('library.open')}
        <span className="sr-only">
          {' '}
          <span dir="auto">{label}</span>
        </span>
      </button>
    </span>
  );
}

function visibleCommand(item: LibraryItem, issuable: boolean): VisibleCommand | null {
  if (item.kind === 'material') {
    return null;
  }
  if (item.ready !== true) {
    return 'open';
  }
  return issuable ? 'issue' : null;
}

function FolderRow({
  folder,
  handlers,
}: {
  readonly folder: LibraryFolder;
  readonly handlers: RowHandlers;
}): ReactElement {
  const target: LibraryTarget = { kind: 'folder', folder };
  return (
    <tr data-row={rowKey(target)} className={tableRowClass}>
      <th scope="row" className={primaryCellClass}>
        <Link data-row-link="" to={folderAddress(folder.id)} dir="auto" className={rowLinkClass}>
          {folder.name}
        </Link>
      </th>
      <td className={secondaryCellClass} />
      <td className={secondaryCellClass} />
      <td className={secondaryCellClass} />
      <td className={endCellClass}>
        <RowMenu target={target} label={folder.name} commands={['rename', 'move', 'delete']} handlers={handlers} />
      </td>
    </tr>
  );
}

function ItemPath({ path }: { readonly path: readonly LibraryPathEntry[] | null }): ReactElement | null {
  const { t } = useTranslation();
  if (path === null) {
    return null;
  }
  return (
    <span className="text-small text-text-muted">
      <Names names={[t('library.root'), ...path.map((entry) => entry.name)]} />
    </span>
  );
}

function commandsOf(item: LibraryItem, issuable: boolean): readonly RowCommand[] {
  if (item.kind === 'material') {
    return ['open', 'rename', 'move', 'delete'];
  }
  return issuable && item.ready === true
    ? ['open', 'issue', 'duplicate', 'move', 'delete']
    : ['open', 'duplicate', 'move', 'delete'];
}

function ItemDetails({ item, me }: { readonly item: LibraryItem; readonly me: Me }): ReactElement {
  const { t } = useTranslation();
  return item.kind === 'material' ? (
    <span className="flex flex-col text-small text-text-muted">
      <span dir="auto">{item.filename}</span>
      <span>{formatStorage(item.size_bytes ?? 0, me.language)}</span>
    </span>
  ) : (
    <span className="flex flex-col text-small text-text-muted">
      <span>{t('library.question_count', { count: item.question_count ?? 0 })}</span>
      <span>{t('library.maxPoints', { count: item.max_points ?? 0 })}</span>
    </span>
  );
}

function ItemState({ item }: { readonly item: LibraryItem }): ReactElement | null {
  const { t } = useTranslation();
  if (item.kind === 'material') {
    return null;
  }
  return item.ready === true ? (
    <StatusWord tone="success">{t('library.ready')}</StatusWord>
  ) : (
    <StatusWord tone="warning">{t('library.incomplete')}</StatusWord>
  );
}

function ItemDate({ item, me }: { readonly item: LibraryItem; readonly me: Me }): ReactElement {
  return <span className="text-small text-text-muted">{formatDate(item.updated_at, me.language, me.timezone)}</span>;
}

function ItemRow({
  item,
  me,
  handlers,
}: {
  readonly item: LibraryItem;
  readonly me: Me;
  readonly handlers: RowHandlers;
}): ReactElement {
  const target: LibraryTarget = { kind: 'item', item };
  const material = item.kind === 'material';
  const visible = visibleCommand(item, handlers.issuable);
  return (
    <tr data-row={rowKey(target)} className={tableRowClass}>
      <th scope="row" className={primaryCellClass}>
        <span className="flex min-w-0 flex-col gap-0.5">
          {material ? (
            <span className="flex min-w-0 items-center gap-2">
              <span aria-hidden="true" className="shrink-0 text-icon">
                <AttachIcon />
              </span>
              <button
                type="button"
                data-row-link=""
                dir="auto"
                onClick={() => {
                  handlers.onCommand('open', target, 'link');
                }}
                className={`text-start ${rowLinkClass}`}
              >
                {item.title}
              </button>
            </span>
          ) : (
            <Link data-row-link="" to={templateAddress(item.id)} dir="auto" className={rowLinkClass}>
              {item.title}
            </Link>
          )}
          <ItemPath path={item.path} />
          <span className={`flex flex-col gap-0.5 ${narrowOnlyClass}`}>
            <ItemDetails item={item} me={me} />
            <ItemState item={item} />
            <ItemDate item={item} me={me} />
          </span>
        </span>
      </th>
      <td className={secondaryCellClass}>
        <ItemDetails item={item} me={me} />
      </td>
      <td className={secondaryCellClass}>
        <ItemState item={item} />
      </td>
      <td className={secondaryCellClass}>
        <ItemDate item={item} me={me} />
      </td>
      <td className={endCellClass}>
        <span className="flex items-center justify-end gap-2">
          {visible === null ? null : (
            <VisibleAction target={target} label={item.title} command={visible} handlers={handlers} />
          )}
          <RowMenu
            target={target}
            label={item.title}
            commands={commandsOf(item, handlers.issuable)}
            handlers={handlers}
          />
        </span>
      </td>
    </tr>
  );
}

export function LibraryTable({
  label,
  folders,
  items,
  me,
  bodyRef,
  handlers,
}: {
  readonly label: string;
  readonly folders: readonly LibraryFolder[];
  readonly items: readonly LibraryItem[];
  readonly me: Me;
  readonly bodyRef: RefObject<HTMLTableSectionElement | null>;
  readonly handlers: RowHandlers;
}): ReactElement {
  const { t } = useTranslation();
  const columns: readonly TableColumn[] = [
    { key: 'name', label: t('library.columns.name'), width: 'w-9/12 tablet:w-3/12' },
    { key: 'details', label: t('library.columns.details'), secondary: true, width: 'w-3/12' },
    { key: 'state', label: t('library.columns.state'), secondary: true, width: 'w-2/12' },
    { key: 'edited', label: t('library.columns.edited'), secondary: true, width: 'w-2/12' },
    { key: 'actions', label: t('library.columns.actions'), visuallyHidden: true, width: 'w-3/12 tablet:w-2/12' },
  ];
  return (
    <DataTable label={label} columns={columns} bodyRef={bodyRef}>
      {folders.map((folder) => (
        <FolderRow key={folder.id} folder={folder} handlers={handlers} />
      ))}
      {items.map((item) => (
        <ItemRow key={`${item.kind}:${item.id}`} item={item} me={me} handlers={handlers} />
      ))}
    </DataTable>
  );
}
