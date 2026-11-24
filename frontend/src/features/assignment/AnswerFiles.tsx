import { type DragEvent, type ReactElement, useEffect, useId, useRef, useState, useSyncExternalStore } from 'react';
import { useTranslation } from 'react-i18next';

import { FormAlert, linkClass, quietButtonClass, textButtonClass } from '../../components/ui/Form.tsx';
import { downloadAddress } from '../../lib/files/downloadAddress.ts';
import { type ApiError, apiErrorFrom, useApiClient } from '../../lib/api/client.ts';
import { useSession } from '../../lib/api/session.ts';
import { formatStorage } from '../settings/storageAmount.ts';
import { UploadQueue } from '../messages/UploadQueue.ts';
import type { AnswerFile, QuestionBlock } from './api.ts';
import { type FileRefusal, acceptList, fileRefusal, pictureAcceptList } from './fileKinds.ts';

export type FileQuestion = Extract<QuestionBlock, { readonly type: 'file' }>;

type AlertReason = FileRefusal | 'tooMany' | 'storageFull' | 'busy' | 'failed';

function uploadRefusal(error: ApiError): AlertReason {
  switch (error.status) {
    case 507:
      return 'storageFull';
    case 503:
      return 'busy';
    case 413:
      return 'tooLarge';
    case 415:
      return 'wrongType';
    default:
      return error.fieldCode('file') === 'webp_too_large' ? 'webpTooLarge' : 'failed';
  }
}

function useAnswerUploads(submissionId: string, questionId: string): UploadQueue {
  const client = useApiClient();
  const session = useSession();
  const [epoch] = useState(() => session.currentEpoch());
  const [queue] = useState(
    () =>
      new UploadQueue(
        async (file, signal, progress) => {
          const body = new FormData();
          body.append('file', file);
          const { data, error, response } = await client.POST('/api/v1/uploads', {
            params: { query: { context: 'answer', scope: `submission:${submissionId}:${questionId}` } },
            signal,
            body: { file: '' },
            bodySerializer: () => null,
            fetch: (request) => session.authorizedUpload(request, { body, progress }),
          });
          if (data === undefined) throw apiErrorFrom(response.status, error);
          return data;
        },
        () => session.currentEpoch() === epoch,
      ),
  );
  useEffect(() => {
    const unsubscribe = session.subscribe(() => {
      if (session.currentEpoch() !== epoch) queue.stop();
    });
    return (): void => {
      unsubscribe();
      queue.clear();
    };
  }, [queue, session, epoch]);
  return queue;
}

function carriesFiles(event: { readonly dataTransfer: DataTransfer | null }): boolean {
  return event.dataTransfer?.types.includes('Files') ?? false;
}

function holdsFolder(transfer: DataTransfer): boolean {
  return Array.from(transfer.items).some((item) => item.webkitGetAsEntry()?.isDirectory === true);
}

function outsideDropZone(event: globalThis.DragEvent): boolean {
  const target = event.target;
  const element = target instanceof Element ? target : target instanceof Node ? target.parentElement : null;
  return element?.closest('[data-drop-zone]') === null || element === null;
}

export function useFileDropGuard(): void {
  useEffect(() => {
    const guard = (event: globalThis.DragEvent): void => {
      if (!carriesFiles(event)) {
        return;
      }
      event.preventDefault();
      if (event.type === 'dragover' && event.dataTransfer !== null && outsideDropZone(event)) {
        event.dataTransfer.dropEffect = 'none';
      }
    };
    window.addEventListener('dragover', guard);
    window.addEventListener('drop', guard);
    return (): void => {
      window.removeEventListener('dragover', guard);
      window.removeEventListener('drop', guard);
    };
  }, []);
}

function focusFirst(candidates: readonly (HTMLElement | null | undefined)[]): void {
  for (const candidate of candidates) {
    if (candidate === null || candidate === undefined) {
      continue;
    }
    candidate.focus();
    if (document.activeElement === candidate) {
      return;
    }
  }
}

function FileName({ file }: { readonly file: AnswerFile }): ReactElement {
  const address = file.url === '' ? null : downloadAddress(file.attachmentId, file.url, window.location.origin);
  return address === null ? (
    <span dir="auto" className="min-w-0 break-words">
      {file.filename}
    </span>
  ) : (
    <a
      href={address}
      download={file.filename}
      rel="noopener noreferrer"
      referrerPolicy="no-referrer"
      dir="auto"
      className={`${linkClass} min-h-0 min-w-0 break-words`}
    >
      {file.filename}
    </a>
  );
}

export function AnswerFiles({
  submissionId,
  block,
  files,
  editable,
  confirmed,
  onChange,
  onUploading,
}: {
  readonly submissionId: string;
  readonly block: FileQuestion;
  readonly files: readonly AnswerFile[];
  readonly editable: boolean;
  readonly confirmed: boolean;
  readonly onChange: (files: readonly AnswerFile[]) => void;
  readonly onUploading: (uploading: boolean) => void;
}): ReactElement {
  const { t, i18n } = useTranslation();
  const hintId = useId();
  const dropHintId = useId();
  const dropButton = useRef<HTMLButtonElement>(null);
  const queue = useAnswerUploads(submissionId, block.id);
  const items = useSyncExternalStore(queue.subscribe, queue.snapshot);
  const [alert, setAlert] = useState<{ readonly reason: AlertReason; readonly attempt: number } | null>(null);
  const attachButton = useRef<HTMLButtonElement>(null);
  const attachInput = useRef<HTMLInputElement>(null);
  const photoInput = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const filesValue = useRef(files);
  const onChangeValue = useRef(onChange);
  useEffect(() => {
    filesValue.current = files;
    onChangeValue.current = onChange;
  }, [files, onChange]);

  useEffect(() => {
    function settle(): void {
      const added: AnswerFile[] = [];
      for (const item of queue.snapshot()) {
        if (item.state === 'ready' && item.uploaded !== null) {
          const uploaded = item.uploaded;
          queue.remove(item.key);
          added.push({
            attachmentId: uploaded.id,
            filename: uploaded.filename,
            mediaType: uploaded.media_type,
            sizeBytes: uploaded.size_bytes,
            url: '',
            expiresAt: '',
          });
        } else if (item.state === 'failed' && item.error !== null) {
          const reason = uploadRefusal(item.error);
          queue.remove(item.key);
          setAlert((previous) => ({ reason, attempt: (previous?.attempt ?? 0) + 1 }));
        }
      }
      if (added.length > 0) {
        const next = [...filesValue.current, ...added];
        filesValue.current = next;
        onChangeValue.current(next);
      }
    }
    return queue.subscribe(settle);
  }, [queue]);

  const [removed, setRemoved] = useState<{ readonly index: number; readonly count: number } | null>(null);
  useEffect(() => {
    if (removed === null) {
      return;
    }
    const buttons = list.current?.querySelectorAll<HTMLButtonElement>('[data-remove-file]');
    const neighbour = buttons?.[removed.index] ?? buttons?.[removed.index - 1];
    focusFirst(neighbour === undefined ? [dropButton.current, attachButton.current] : [neighbour]);
  }, [removed]);
  const uploading = items.length > 0;
  const onUploadingValue = useRef(onUploading);
  useEffect(() => {
    onUploadingValue.current = onUploading;
  }, [onUploading]);
  useEffect(() => {
    onUploadingValue.current(uploading);
  }, [uploading]);
  useEffect(
    () => (): void => {
      onUploadingValue.current(false);
    },
    [],
  );

  function refuse(reason: AlertReason): void {
    setAlert((previous) => ({ reason, attempt: (previous?.attempt ?? 0) + 1 }));
  }

  function choose(chosen: readonly File[]): void {
    if (chosen.length === 0) {
      return;
    }
    if (files.length + items.length + chosen.length > block.maxFiles) {
      refuse('tooMany');
      return;
    }
    for (const file of chosen) {
      const refusal = fileRefusal(file, block.allowedKinds);
      if (refusal !== null) {
        refuse(refusal);
        return;
      }
    }
    setAlert(null);
    queue.add(chosen);
  }

  const remaining = Math.max(0, block.maxFiles - files.length - items.length);

  function dropped(event: DragEvent<HTMLButtonElement>): void {
    if (!carriesFiles(event)) {
      return;
    }
    event.preventDefault();
    if (remaining === 0) {
      refuse('tooMany');
      return;
    }
    if (holdsFolder(event.dataTransfer)) {
      refuse('wrongType');
      return;
    }
    choose(Array.from(event.dataTransfer.files));
  }

  function remove(index: number): void {
    onChange(files.filter((_, position) => position !== index));
    setRemoved((previous) => ({ index, count: (previous?.count ?? 0) + 1 }));
  }

  const alertText =
    alert === null
      ? null
      : alert.reason === 'tooMany'
        ? t('assignment.files.tooMany', { count: block.maxFiles })
        : t(`assignment.files.${alert.reason}`);

  return (
    <div className="flex flex-col gap-3">
      {files.length > 0 ? (
        <ul ref={list} className="flex flex-col gap-2">
          {files.map((file, index) => (
            <li key={file.attachmentId} className="flex flex-wrap items-center gap-x-3 gap-y-1">
              <FileName file={file} />
              <span className="text-small text-text-muted">{formatStorage(file.sizeBytes, i18n.language)}</span>
              {editable ? (
                <button
                  type="button"
                  data-remove-file=""
                  aria-label={t('assignment.files.removeNamed', { name: file.filename })}
                  onClick={() => {
                    remove(index);
                  }}
                  className={textButtonClass}
                >
                  {t('assignment.files.remove')}
                </button>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}
      {items.length > 0 ? (
        <ul className="flex flex-col gap-1">
          {items.map((item) => (
            <li key={item.key} className="text-small text-text-muted">
              <span dir="auto">{item.name}</span>{' '}
              {t('assignment.files.uploading', { percent: `${String(item.progress ?? 0)}%` })}
            </li>
          ))}
        </ul>
      ) : null}
      {alertText === null ? null : <FormAlert key={alert?.attempt}>{alertText}</FormAlert>}
      {editable ? (
        confirmed ? (
          <>
            <div className="hidden flex-col gap-1 desktop:flex">
              <button
                ref={dropButton}
                type="button"
                data-drop-zone=""
                aria-disabled={remaining === 0 ? true : undefined}
                aria-describedby={dropHintId}
                onClick={() => {
                  if (remaining === 0) {
                    refuse('tooMany');
                    return;
                  }
                  attachInput.current?.click();
                }}
                onDragOver={(event) => {
                  if (carriesFiles(event)) {
                    event.preventDefault();
                  }
                }}
                onDrop={dropped}
                className="flex min-h-24 w-full items-center justify-center rounded-lg border border-dashed border-border-strong p-6 text-strong hover:bg-surface aria-disabled:text-text-muted"
              >
                {t('assignment.choose')}
              </button>
              <p id={dropHintId} className="text-small text-text-muted">
                {remaining === 0
                  ? t('assignment.full', { count: block.maxFiles })
                  : t('assignment.remaining', {
                      kinds: block.allowedKinds.map((kind) => t(`assignment.kinds.${kind}`)).join(', '),
                      count: remaining,
                    })}
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2 desktop:hidden max-tablet:flex-col max-tablet:items-stretch">
              <button
                ref={attachButton}
                type="button"
                aria-describedby={hintId}
                onClick={() => attachInput.current?.click()}
                className={quietButtonClass}
              >
                {t('assignment.attach')}
              </button>
              {block.allowedKinds.includes('images') ? (
                <button type="button" onClick={() => photoInput.current?.click()} className={quietButtonClass}>
                  {t('assignment.files.takePhoto')}
                </button>
              ) : null}
              <input
                ref={attachInput}
                type="file"
                hidden
                multiple={block.maxFiles > 1}
                accept={acceptList(block.allowedKinds)}
                onChange={(event) => {
                  choose(Array.from(event.currentTarget.files ?? []));
                  event.currentTarget.value = '';
                }}
              />
              {block.allowedKinds.includes('images') ? (
                <input
                  ref={photoInput}
                  type="file"
                  hidden
                  accept={pictureAcceptList()}
                  capture="environment"
                  onChange={(event) => {
                    choose(Array.from(event.currentTarget.files ?? []));
                    event.currentTarget.value = '';
                  }}
                />
              ) : null}
            </div>
          </>
        ) : (
          <p className="text-text-muted">{t('assignment.files.confirmEmail')}</p>
        )
      ) : null}
      {editable && confirmed ? (
        <p id={hintId} className="text-small text-text-muted desktop:hidden">
          {t('assignment.fileHint', {
            kinds: block.allowedKinds.map((kind) => t(`assignment.kinds.${kind}`)).join(', '),
            count: block.maxFiles,
          })}
        </p>
      ) : null}
    </div>
  );
}
