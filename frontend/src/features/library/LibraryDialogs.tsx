import { useQueryClient } from '@tanstack/react-query';
import type { TFunction } from 'i18next';
import {
  type ReactElement,
  type RefObject,
  type SyntheticEvent,
  useEffect,
  useId,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react';
import { useTranslation } from 'react-i18next';

import { type Action, type ActionState, done, refusedBy, useAction } from '../../components/ui/action.ts';
import { ConfirmationPrompt } from '../../components/ui/ConfirmationPrompt.tsx';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormAlert,
  SelectChevron,
  FormActions,
  dangerFilledButtonClass,
  fieldClass,
  primaryButtonClass,
  quietButtonClass,
  selectClass,
} from '../../components/ui/Form.tsx';
import { refusalText } from '../../components/ui/Refusal.tsx';
import { hasUnknownOutcome } from '../../components/ui/settled.ts';
import { type ApiError, useApiClient } from '../../lib/api/client.ts';
import {
  type FolderNode,
  type LibraryFolder,
  type LibraryItem,
  cutToCodePoints,
  libraryKey,
  subtreeOf,
  titleLimit,
  withoutExtension,
} from './api.ts';
import { type Moved, sendMove } from './moveUndo.ts';
import type { LibraryUpload, UploadState } from './upload.ts';

export type LibraryTarget =
  { readonly kind: 'folder'; readonly folder: LibraryFolder } | { readonly kind: 'item'; readonly item: LibraryItem };

export function targetName(target: LibraryTarget): string {
  return target.kind === 'folder' ? target.folder.name : target.item.title;
}

export type RowControl = 'menu' | 'visible' | 'link';

export function libraryRowOpener(key: string, control: RowControl): string {
  return control === 'menu' ? `library-row:${key}` : `library-row:${key}:${control}`;
}

export function rowKey(target: LibraryTarget): string {
  return target.kind === 'folder' ? `folder:${target.folder.id}` : `${target.item.kind}:${target.item.id}`;
}

interface DialogFrame {
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
  readonly onGone: () => void;
}

function isGone(state: ActionState): boolean {
  return state.kind === 'refused' && state.error.code === 'not_found';
}

function nameProblem(t: TFunction, error: ApiError | null, field: 'name' | 'title'): string | undefined {
  if (error === null) {
    return undefined;
  }
  const own = error.fieldCode(field);
  if (own === 'taken') {
    return t('library.taken');
  }
  if (own !== undefined) {
    return t(`fieldErrors.${own}`);
  }
  if (error.fieldCode('parent_id') === 'invalid') {
    return t('library.depth');
  }
  if (error.fieldCode('body') === 'too_many') {
    return t('library.tooMany');
  }
  return undefined;
}

function DialogField({
  label,
  name,
  defaultValue,
  error,
  input,
}: {
  readonly label: string;
  readonly name: string;
  readonly defaultValue: string;
  readonly error: string | undefined;
  readonly input: RefObject<HTMLInputElement | null>;
}): ReactElement {
  const inputId = useId();
  const errorId = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={inputId} className="text-label">
        {label}
      </label>
      <input
        ref={input}
        id={inputId}
        name={name}
        type="text"
        autoComplete="off"
        required
        defaultValue={defaultValue}
        dir="auto"
        aria-invalid={error === undefined ? undefined : true}
        aria-describedby={error === undefined ? undefined : errorId}
        className={fieldClass}
      />
      {error === undefined ? null : (
        <p id={errorId} className="text-small text-danger">
          {error}
        </p>
      )}
    </div>
  );
}

function fieldValue(form: HTMLFormElement, name: string): string {
  const value = new FormData(form).get(name);
  return typeof value === 'string' ? value : '';
}

export type NameSubject =
  | { readonly kind: 'newFolder'; readonly parentId: string | null }
  | { readonly kind: 'rename'; readonly target: LibraryTarget };

export function NameDialog({
  subject,
  returnFocus,
  onClose,
  onGone,
  onSaved,
}: DialogFrame & {
  readonly subject: NameSubject | null;
  readonly onSaved: (rowKey: string) => void;
}): ReactElement {
  const input = useRef<HTMLInputElement>(null);
  const saved = useRef<string | null>(null);
  const [refusals, setRefusals] = useState(0);
  const action = useAction((state) => {
    if (state.kind === 'done' && saved.current !== null) {
      onSaved(saved.current);
      return;
    }
    if (isGone(state)) {
      onGone();
      return;
    }
    setRefusals((count) => count + 1);
  });
  const headingId = useId();

  function close(): void {
    action.reset();
    onClose();
  }

  return (
    <ModalDialog
      open={subject !== null}
      busy={action.busy}
      labelledBy={headingId}
      returnFocus={returnFocus}
      initialFocus={input}
      onClose={close}
    >
      {subject === null ? null : (
        <NameForm
          subject={subject}
          headingId={headingId}
          input={input}
          saved={saved}
          refusals={refusals}
          action={action}
          onCancel={close}
        />
      )}
    </ModalDialog>
  );
}

function NameForm({
  subject,
  headingId,
  input,
  saved,
  refusals,
  action,
  onCancel,
}: {
  readonly subject: NameSubject;
  readonly headingId: string;
  readonly input: RefObject<HTMLInputElement | null>;
  readonly saved: RefObject<string | null>;
  readonly refusals: number;
  readonly action: Action;
  readonly onCancel: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const formId = useId();
  const folderField = subject.kind === 'newFolder' || subject.target.kind === 'folder';
  const field = folderField ? 'name' : 'title';
  const error = action.state.kind === 'refused' ? action.state.error : null;
  const problem = nameProblem(t, error, field);
  const otherRefusal =
    action.state.kind === 'refused' && problem === undefined
      ? refusalText(t, action.state.error, action.state.retryAfter, undefined)
      : null;
  const heading =
    subject.kind === 'newFolder'
      ? t('library.newFolder')
      : subject.target.kind === 'folder'
        ? t('library.renameFolder')
        : t('library.renameMaterial');

  useEffect(() => {
    if (refusals > 0 && problem !== undefined) {
      input.current?.focus();
    }
  }, [refusals, problem, input]);

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    const value = fieldValue(event.currentTarget, field);
    action.run(async (): Promise<ActionState> => {
      if (subject.kind === 'newFolder') {
        const {
          data,
          error: problemBody,
          response,
        } = await client.POST('/api/v1/library/folders', {
          body: { name: value, parent_id: subject.parentId },
        });
        if (data === undefined) {
          return refusedBy(response, problemBody);
        }
        saved.current = `folder:${data.id}`;
      } else if (subject.target.kind === 'folder') {
        const {
          data,
          error: problemBody,
          response,
        } = await client.PATCH('/api/v1/library/folders/{id}', {
          params: { path: { id: subject.target.folder.id } },
          body: { name: value },
        });
        if (data === undefined) {
          return refusedBy(response, problemBody);
        }
        saved.current = `folder:${data.id}`;
      } else {
        const {
          data,
          error: problemBody,
          response,
        } = await client.PATCH('/api/v1/library/materials/{id}', {
          params: { path: { id: subject.target.item.id } },
          body: { title: value },
        });
        if (data === undefined) {
          return refusedBy(response, problemBody);
        }
        saved.current = `material:${data.id}`;
      }
      await queryClient.invalidateQueries({ queryKey: libraryKey });
      return done;
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <h2 id={headingId} className="font-serif text-title">
        {heading}
      </h2>
      {otherRefusal === null ? null : <FormAlert key={refusals}>{otherRefusal}</FormAlert>}
      <form id={formId} onSubmit={submit} className="flex flex-col gap-4">
        <DialogField
          label={folderField ? t('library.folderName') : t('library.title')}
          name={field}
          defaultValue={subject.kind === 'rename' ? targetName(subject.target) : ''}
          error={problem}
          input={input}
        />
      </form>
      <FormActions>
        <ActionButton type="submit" form={formId} busy={action.busy} className={primaryButtonClass}>
          {subject.kind === 'newFolder' ? t('library.create') : t('library.save')}
        </ActionButton>
        <ActionButton busy={false} unavailable={action.busy} onClick={onCancel} className={quietButtonClass}>
          {t('library.cancel')}
        </ActionButton>
      </FormActions>
    </div>
  );
}

function destinationLabel(node: FolderNode): string {
  return node.path.map((folder) => folder.name).join(' / ');
}

function currentParent(target: LibraryTarget): string | null {
  return target.kind === 'folder' ? target.folder.parent_id : target.item.folder_id;
}

export function MoveDialog({
  target,
  nodes,
  returnFocus,
  onClose,
  onGone,
  onMoved,
}: DialogFrame & {
  readonly target: LibraryTarget | null;
  readonly nodes: readonly FolderNode[];
  readonly onMoved: (moved: Moved) => void;
}): ReactElement {
  const headingId = useId();
  const picker = useRef<HTMLSelectElement>(null);
  const [refusals, setRefusals] = useState(0);
  const lastMoved = useRef<Moved | null>(null);
  const action = useAction((state) => {
    if (state.kind === 'done') {
      if (lastMoved.current !== null) {
        onMoved(lastMoved.current);
      }
      return;
    }
    if (isGone(state)) {
      onGone();
      return;
    }
    setRefusals((count) => count + 1);
  });

  function close(): void {
    action.reset();
    onClose();
  }

  return (
    <ModalDialog
      open={target !== null}
      busy={action.busy}
      labelledBy={headingId}
      returnFocus={returnFocus}
      initialFocus={picker}
      onClose={close}
    >
      {target === null ? null : (
        <MoveForm
          target={target}
          nodes={nodes}
          headingId={headingId}
          picker={picker}
          refusals={refusals}
          action={action}
          onCancel={close}
          onMovedTo={(moved) => {
            lastMoved.current = moved;
          }}
        />
      )}
    </ModalDialog>
  );
}

export function MoveForm({
  target,
  nodes,
  headingId,
  picker,
  refusals,
  action,
  onCancel,
  onMovedTo,
}: {
  readonly target: LibraryTarget;
  readonly nodes: readonly FolderNode[];
  readonly headingId: string;
  readonly picker: RefObject<HTMLSelectElement | null>;
  readonly refusals: number;
  readonly action: Action;
  readonly onCancel: () => void;
  readonly onMovedTo: (moved: Moved) => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const formId = useId();
  const selectId = useId();
  const unavailable = target.kind === 'folder' ? subtreeOf(nodes, target.folder.id) : new Set<string>();
  const refusal =
    action.state.kind !== 'refused'
      ? null
      : action.state.error.fieldCode('parent_id') === 'invalid'
        ? t('library.depth')
        : refusalText(t, action.state.error, action.state.retryAfter, undefined);

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    const chosen = fieldValue(event.currentTarget, 'destination');
    const destination = chosen === '' ? null : chosen;
    action.run(async (): Promise<ActionState> => {
      const version = target.kind === 'item' ? (target.item.version ?? 0) : null;
      const answer = await sendMove(client, target, destination, version);
      if (typeof answer !== 'number' && answer !== null) {
        if (hasUnknownOutcome(answer) || (answer.kind === 'refused' && answer.error.status === 409)) {
          await queryClient.invalidateQueries({ queryKey: libraryKey });
        }
        return answer;
      }
      onMovedTo({ target, from: currentParent(target), to: destination, version: answer });
      await queryClient.invalidateQueries({ queryKey: libraryKey });
      return done;
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <h2 id={headingId} className="font-serif text-title">
        {t('library.moveTitle')}
      </h2>
      <p dir="auto" className="break-words text-text-muted">
        {targetName(target)}
      </p>
      {refusal === null ? null : <FormAlert key={refusals}>{refusal}</FormAlert>}
      <form id={formId} onSubmit={submit} className="flex flex-col gap-1">
        <label htmlFor={selectId} className="text-label">
          {t('library.destination')}
        </label>
        <span className="relative">
          <select
            ref={picker}
            id={selectId}
            name="destination"
            defaultValue={currentParent(target) ?? ''}
            className={selectClass}
          >
            <option value="">{t('library.root')}</option>
            {nodes.map((node) => (
              <option key={node.folder.id} value={node.folder.id} disabled={unavailable.has(node.folder.id)}>
                {destinationLabel(node)}
              </option>
            ))}
          </select>
          <SelectChevron />
        </span>
      </form>
      <FormActions>
        <ActionButton type="submit" form={formId} busy={action.busy} className={primaryButtonClass}>
          {t('library.move')}
        </ActionButton>
        <ActionButton busy={false} unavailable={action.busy} onClick={onCancel} className={quietButtonClass}>
          {t('library.cancel')}
        </ActionButton>
      </FormActions>
    </div>
  );
}

function deleteQuestion(t: TFunction, target: LibraryTarget): string {
  if (target.kind === 'folder') {
    return t('library.deleteFolder');
  }
  return target.item.kind === 'material' ? t('library.deleteMaterial') : t('library.deleteTemplate');
}

function deleteLabel(t: TFunction, target: LibraryTarget): string {
  if (target.kind === 'folder') {
    return t('library.confirmDeleteFolder');
  }
  return target.item.kind === 'material' ? t('library.confirmDeleteMaterial') : t('library.confirmDeleteTemplate');
}

export function DeleteDialog({
  target,
  returnFocus,
  onClose,
  onGone,
  onDeleted,
}: DialogFrame & {
  readonly target: LibraryTarget | null;
  readonly onDeleted: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const headingId = useId();
  const textId = useId();
  const cancel = useRef<HTMLButtonElement>(null);
  const [refusals, setRefusals] = useState(0);
  const action = useAction((state) => {
    if (state.kind === 'done') {
      onDeleted();
      return;
    }
    if (isGone(state)) {
      onGone();
      return;
    }
    setRefusals((count) => count + 1);
  });
  const refusal =
    action.state.kind !== 'refused'
      ? null
      : action.state.error.code === 'not_editable'
        ? t('library.notEmpty')
        : refusalText(t, action.state.error, action.state.retryAfter, undefined);

  function close(): void {
    action.reset();
    onClose();
  }

  function remove(chosen: LibraryTarget): void {
    action.run(async (): Promise<ActionState> => {
      const answer =
        chosen.kind === 'folder'
          ? await client.DELETE('/api/v1/library/folders/{id}', { params: { path: { id: chosen.folder.id } } })
          : chosen.item.kind === 'material'
            ? await client.DELETE('/api/v1/library/materials/{id}', { params: { path: { id: chosen.item.id } } })
            : await client.DELETE('/api/v1/library/templates/{id}', { params: { path: { id: chosen.item.id } } });
      return answer.response.ok ? done : refusedBy(answer.response, answer.error);
    });
  }

  return (
    <ModalDialog
      open={target !== null}
      busy={action.busy}
      labelledBy={headingId}
      describedBy={textId}
      returnFocus={returnFocus}
      initialFocus={cancel}
      onClose={close}
    >
      {target === null ? null : (
        <div className="flex flex-col gap-4">
          <h2 id={headingId} dir="auto" className="font-serif text-title break-words">
            {targetName(target)}
          </h2>
          <p id={textId}>{deleteQuestion(t, target)}</p>
          {refusal === null ? null : <FormAlert key={refusals}>{refusal}</FormAlert>}
          <FormActions>
            <ActionButton
              busy={action.busy}
              onClick={() => {
                remove(target);
              }}
              className={dangerFilledButtonClass}
            >
              {deleteLabel(t, target)}
            </ActionButton>
            <ActionButton
              ref={cancel}
              busy={false}
              unavailable={action.busy}
              onClick={close}
              className={quietButtonClass}
            >
              {t('library.cancel')}
            </ActionButton>
          </FormActions>
        </div>
      )}
    </ModalDialog>
  );
}

function uploadRefusal(t: TFunction, state: UploadState): string | null {
  if (state.kind === 'tooLarge') {
    return state.limit === 'file' ? t('library.fileTooLarge') : t('fieldErrors.webp_too_large');
  }
  if (state.kind !== 'refused') {
    return null;
  }
  if (state.error.fieldCode('file') === 'webp_too_large') {
    return t('fieldErrors.webp_too_large');
  }
  if (state.error.code === 'payload_too_large') {
    return t('library.fileTooLarge');
  }
  return refusalText(t, state.error, state.retryAfter, undefined);
}

function materialRefusal(t: TFunction, state: ActionState): string | null {
  if (state.kind !== 'refused' || state.error.fieldCode('title') !== undefined) {
    return null;
  }
  if (state.error.fieldCode('body') === 'too_many') {
    return t('library.tooMany');
  }
  return refusalText(t, state.error, state.retryAfter, undefined);
}

export function UploadDialog({
  upload,
  folderId,
  returnFocus,
  onClose,
  onGone,
  onSaved,
}: DialogFrame & {
  readonly upload: LibraryUpload | null;
  readonly folderId: string | null;
  readonly onSaved: (rowKey: string) => void;
}): ReactElement {
  const headingId = useId();
  const input = useRef<HTMLInputElement>(null);
  const refusedBeforeSending = useRef<HTMLParagraphElement>(null);
  const saved = useRef<string | null>(null);
  const [refusals, setRefusals] = useState(0);
  const openedRefused = upload?.snapshot().kind === 'tooLarge';
  const action = useAction((settled) => {
    if (settled.kind === 'done' && saved.current !== null) {
      onSaved(saved.current);
      return;
    }
    if (isGone(settled)) {
      onGone();
      return;
    }
    setRefusals((count) => count + 1);
  });

  function close(): void {
    action.reset();
    onClose();
  }

  return (
    <ModalDialog
      open={upload !== null}
      busy={action.busy}
      labelledBy={headingId}
      returnFocus={returnFocus}
      initialFocus={openedRefused ? refusedBeforeSending : input}
      onClose={close}
    >
      {upload === null ? null : (
        <UploadForm
          refusedBeforeSending={refusedBeforeSending}
          upload={upload}
          folderId={folderId}
          headingId={headingId}
          input={input}
          saved={saved}
          refusals={refusals}
          action={action}
          onCancel={close}
        />
      )}
    </ModalDialog>
  );
}

function UploadForm({
  refusedBeforeSending,
  upload,
  folderId,
  headingId,
  input,
  saved,
  refusals,
  action,
  onCancel,
}: {
  readonly refusedBeforeSending: RefObject<HTMLParagraphElement | null>;
  readonly upload: LibraryUpload;
  readonly folderId: string | null;
  readonly headingId: string;
  readonly input: RefObject<HTMLInputElement | null>;
  readonly saved: RefObject<string | null>;
  readonly refusals: number;
  readonly action: Action;
  readonly onCancel: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const formId = useId();
  const state = useSyncExternalStore(upload.subscribe, upload.snapshot);
  const titleError =
    action.state.kind === 'refused' && action.state.error.fieldCode('title') !== undefined
      ? nameProblem(t, action.state.error, 'title')
      : undefined;
  const saveRefusal = materialRefusal(t, action.state);

  useEffect(() => {
    if (refusals > 0 && titleError !== undefined) {
      input.current?.focus();
    }
  }, [refusals, titleError, input]);

  function submit(event: SyntheticEvent<HTMLFormElement, SubmitEvent>): void {
    event.preventDefault();
    if (state.kind !== 'uploaded') {
      return;
    }
    const title = fieldValue(event.currentTarget, 'title');
    action.run(async (): Promise<ActionState> => {
      const { data, error, response } = await client.POST('/api/v1/library/materials', {
        body: { attachment_id: state.file.id, title, folder_id: folderId },
      });
      if (data === undefined) {
        return refusedBy(response, error);
      }
      saved.current = `material:${data.id}`;
      await queryClient.invalidateQueries({ queryKey: libraryKey });
      return done;
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <h2 id={headingId} className="font-serif text-title">
        {t('library.upload')}
      </h2>
      <UploadStatus upload={upload} refusedBeforeSending={refusedBeforeSending} />
      {saveRefusal === null ? null : <FormAlert key={refusals}>{saveRefusal}</FormAlert>}
      <form id={formId} onSubmit={submit} className="flex flex-col gap-4">
        <DialogField
          label={t('library.title')}
          name="title"
          defaultValue={cutToCodePoints(withoutExtension(upload.file.name), titleLimit)}
          error={titleError}
          input={input}
        />
      </form>
      <FormActions>
        <ActionButton
          type="submit"
          form={formId}
          busy={action.busy}
          unavailable={state.kind !== 'uploaded'}
          className={primaryButtonClass}
        >
          {t('library.save')}
        </ActionButton>
        <ActionButton busy={false} unavailable={action.busy} onClick={onCancel} className={quietButtonClass}>
          {t('library.cancel')}
        </ActionButton>
      </FormActions>
    </div>
  );
}

export function UploadStatus({
  upload,
  refusedBeforeSending,
}: {
  readonly upload: LibraryUpload;
  readonly refusedBeforeSending: RefObject<HTMLParagraphElement | null>;
}): ReactElement {
  const { t } = useTranslation();
  const state = useSyncExternalStore(upload.subscribe, upload.snapshot);
  const refusal = uploadRefusal(t, state);
  if (refusal !== null && state.kind === 'tooLarge') {
    return (
      <p ref={refusedBeforeSending} role="alert" tabIndex={-1} className="text-danger outline-none">
        {refusal}
      </p>
    );
  }
  if (refusal !== null) {
    return <FormAlert>{refusal}</FormAlert>;
  }
  const label = state.kind === 'uploaded' ? t('library.uploaded') : t('library.uploading');
  return (
    <div className="flex flex-col gap-1">
      <p className="text-small break-words text-text-muted">
        {label} <span dir="auto">{upload.file.name}</span>
      </p>
      <progress
        aria-label={`${label} ${upload.file.name}`}
        value={state.kind === 'uploading' ? state.loaded : 1}
        max={state.kind === 'uploading' ? (state.total ?? upload.file.size) : 1}
        className="w-full"
      />
    </div>
  );
}

export function UploadPromptDialog({
  open,
  returnFocus,
  onClose,
}: {
  readonly open: boolean;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly onClose: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  return (
    <ModalDialog open={open} busy={false} labelledBy={headingId} returnFocus={returnFocus} onClose={onClose}>
      <div className="flex flex-col gap-4">
        <h2 id={headingId} className="font-serif text-title">
          {t('library.upload')}
        </h2>
        <ConfirmationPrompt reason="upload" />
        <FormActions>
          <ActionButton busy={false} onClick={onClose} className={quietButtonClass}>
            {t('library.close')}
          </ActionButton>
        </FormActions>
      </div>
    </ModalDialog>
  );
}
