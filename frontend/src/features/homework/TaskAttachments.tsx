import type { TFunction } from 'i18next';
import { type DragEvent, type ReactElement, useEffect, useId, useRef, useState, useSyncExternalStore } from 'react';
import { useTranslation } from 'react-i18next';

import { FormAlert, textButtonClass } from '../../components/ui/Form.tsx';
import { CrossIcon, DocumentIcon, FileKindIcon, ImageIcon } from '../../components/ui/icons.tsx';
import { refusalText } from '../../components/ui/Refusal.tsx';
import { extensionTile } from '../../lib/files/extension.ts';
import type { Attachment, AttachmentRefusal, TaskAttachments } from './attachmentQueue.ts';

function refusalOf(t: TFunction, refusal: AttachmentRefusal): string {
  if (refusal.kind === 'tooLarge' || refusal.error.code === 'payload_too_large') {
    return t('homework.newTask.fileTooLarge');
  }
  if (refusal.error.code === 'email_not_verified') {
    return t('homework.confirmEmail');
  }
  if (refusal.error.fieldCode('file') === 'webp_too_large') {
    return t('fieldErrors.webp_too_large');
  }
  return refusalText(t, refusal.error, refusal.retryAfter, undefined);
}

function removeButtonId(key: string): string {
  return `task-attachment-remove-${key}`;
}

function FileKind({ file }: { readonly file: File }): ReactElement {
  if (file.type.startsWith('image/')) {
    return <ImageIcon size={28} />;
  }
  const extension = extensionTile(file.name);
  if (extension === null) {
    return <FileKindIcon size={28} />;
  }
  return (
    <>
      <DocumentIcon size={24} />
      <span className="text-label">{extension}</span>
    </>
  );
}

function AttachmentTile({
  attachment,
  locked,
  onRetry,
  onRemove,
}: {
  readonly attachment: Attachment;
  readonly locked: boolean;
  readonly onRetry: () => void;
  readonly onRemove: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const nameId = useId();
  const { state } = attachment;
  const { name } = attachment.file;
  const label =
    state.kind === 'ready'
      ? t('homework.newTask.uploaded')
      : state.kind === 'waiting'
        ? t('homework.newTask.waiting')
        : t('homework.newTask.uploading');
  return (
    <li aria-labelledby={nameId} data-attachment-tile="" className="relative flex w-24 flex-col gap-1">
      <span
        aria-hidden="true"
        className="flex aspect-[4/3] w-full flex-col items-center justify-center gap-0.5 rounded-md border border-border bg-surface text-icon"
      >
        <FileKind file={attachment.file} />
      </span>
      <span id={nameId} dir="auto" title={name} className="truncate text-small">
        {name}
      </span>
      {state.kind === 'refused' ? (
        <div className="text-small">
          <FormAlert>{refusalOf(t, state.refusal)}</FormAlert>
        </div>
      ) : (
        <progress
          aria-label={`${label} ${name}`}
          value={state.kind === 'uploading' ? state.loaded : state.kind === 'ready' ? 1 : 0}
          max={state.kind === 'uploading' ? (state.total ?? attachment.file.size) : 1}
          className="w-full"
        />
      )}
      {state.kind === 'refused' && state.retryable && !locked ? (
        <button type="button" aria-describedby={nameId} onClick={onRetry} className={textButtonClass}>
          {t('homework.newTask.retry')}
        </button>
      ) : null}
      {locked ? null : (
        <button
          id={removeButtonId(attachment.key)}
          type="button"
          aria-label={t('homework.newTask.removeFile', { name })}
          onClick={onRemove}
          className="absolute top-1 right-1 flex size-7 items-center justify-center rounded-full border border-border-strong bg-bg text-text hover:bg-surface"
        >
          <CrossIcon size={14} />
        </button>
      )}
    </li>
  );
}

export function TaskAttachmentsField({
  attachments: store,
  locked,
}: {
  readonly attachments: TaskAttachments;
  readonly locked: boolean;
}): ReactElement {
  const { t } = useTranslation();
  const labelId = useId();
  const hintId = useId();
  const input = useRef<HTMLInputElement>(null);
  const attachments = useSyncExternalStore(store.subscribe, store.snapshot);
  const [dragging, setDragging] = useState(false);

  const chooseButton = useRef<HTMLButtonElement>(null);
  const pendingFocus = useRef<string | null>(null);

  useEffect(() => {
    const target = pendingFocus.current;
    pendingFocus.current = null;
    if (target === null) {
      return;
    }
    const element = target === '' ? chooseButton.current : document.getElementById(target);
    element?.focus();
  });

  function dropped(event: DragEvent<HTMLButtonElement>): void {
    event.preventDefault();
    setDragging(false);
    if (!locked) {
      store.add(Array.from(event.dataTransfer.files));
    }
  }

  function remove(index: number): void {
    const removed = attachments[index];
    if (removed === undefined) {
      return;
    }
    const next = attachments[index + 1] ?? attachments[index - 1];
    pendingFocus.current = next === undefined ? '' : removeButtonId(next.key);
    store.remove(removed.key);
  }

  return (
    <section aria-labelledby={labelId} className="flex flex-col gap-2">
      <h2 id={labelId} className="text-label">
        {t('homework.newTask.attachments')} <span className="font-normal text-text-muted">{t('form.optional')}</span>
      </h2>
      {locked ? null : (
        <>
          <button
            ref={chooseButton}
            type="button"
            data-drop-zone=""
            aria-describedby={hintId}
            onClick={() => {
              input.current?.click();
            }}
            onDragOver={(event) => {
              if (event.dataTransfer.types.includes('Files')) {
                event.preventDefault();
                setDragging(true);
              }
            }}
            onDragLeave={() => {
              setDragging(false);
            }}
            onDrop={dropped}
            className={`flex min-h-24 w-full flex-col items-center justify-center gap-1 rounded-lg border border-dashed border-border-strong p-6 hover:bg-surface ${dragging ? 'bg-surface' : ''}`}
          >
            <span className="text-strong">{t('homework.newTask.choose')}</span>
          </button>
          <p id={hintId} className="text-small text-text-muted">
            {t('homework.newTask.attachHint')}
          </p>
          <input
            ref={input}
            type="file"
            multiple
            tabIndex={-1}
            aria-hidden="true"
            className="sr-only"
            onChange={(event) => {
              store.add(Array.from(event.target.files ?? []));
              event.target.value = '';
            }}
          />
        </>
      )}
      {attachments.length === 0 ? null : (
        <ul aria-label={t('homework.newTask.attachedFiles')} className="flex flex-wrap gap-3">
          {attachments.map((attachment, index) => (
            <AttachmentTile
              key={attachment.key}
              attachment={attachment}
              locked={locked}
              onRetry={() => {
                pendingFocus.current = removeButtonId(attachment.key);
                store.retry(attachment.key);
              }}
              onRemove={() => {
                remove(index);
              }}
            />
          ))}
        </ul>
      )}
      <p className="text-small text-text-muted">{t('homework.newTask.folderHint')}</p>
    </section>
  );
}
