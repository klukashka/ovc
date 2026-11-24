import type { TFunction } from 'i18next';
import { type DragEvent, type ReactElement, useEffect, useId, useRef, useState, useSyncExternalStore } from 'react';
import { useTranslation } from 'react-i18next';

import { FormAlert, quietButtonClass, textButtonClass } from '../../components/ui/Form.tsx';
import { refusalText } from '../../components/ui/Refusal.tsx';
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

function AttachmentRow({
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
  const label =
    state.kind === 'ready'
      ? t('homework.newTask.uploaded')
      : state.kind === 'waiting'
        ? t('homework.newTask.waiting')
        : t('homework.newTask.uploading');
  return (
    <li className="flex flex-col gap-2 rounded-lg border border-border bg-surface p-3">
      <div className="flex items-center gap-3">
        <span id={nameId} dir="auto" className="min-w-0 flex-1 break-words">
          {attachment.file.name}
        </span>
        {state.kind === 'refused' && state.retryable && !locked ? (
          <button type="button" aria-describedby={nameId} onClick={onRetry} className={quietButtonClass}>
            {t('homework.newTask.retry')}
          </button>
        ) : null}
        {locked ? null : (
          <button
            id={removeButtonId(attachment.key)}
            type="button"
            aria-describedby={nameId}
            onClick={onRemove}
            className={textButtonClass}
          >
            {t('homework.newTask.remove')}
          </button>
        )}
      </div>
      {state.kind === 'refused' ? (
        <FormAlert>{refusalOf(t, state.refusal)}</FormAlert>
      ) : (
        <progress
          aria-label={`${label} ${attachment.file.name}`}
          value={state.kind === 'uploading' ? state.loaded : state.kind === 'ready' ? 1 : 0}
          max={state.kind === 'uploading' ? (state.total ?? attachment.file.size) : 1}
          className="w-full"
        />
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
        <ul className="flex flex-col gap-2">
          {attachments.map((attachment, index) => (
            <AttachmentRow
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
