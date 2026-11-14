import { type ReactElement, useEffect, useRef, useState, useSyncExternalStore } from 'react';
import { useTranslation } from 'react-i18next';

import { FormAlert, quietButtonClass, textButtonClass } from '../../components/ui/Form.tsx';
import { type ApiError, apiErrorFrom, useApiClient } from '../../lib/api/client.ts';
import { useSession } from '../../lib/api/session.ts';
import { acceptList, fileRefusal } from '../assignment/fileKinds.ts';
import { UploadQueue } from '../messages/UploadQueue.ts';
import { formatStorage } from '../settings/storageAmount.ts';
import type { ReviewFile } from './api.ts';
import type { CorrectedEntry } from './draft.ts';
import { FileLink } from './ReviewAnswer.tsx';

export const correctedFilesPerAnswer = 5;

const correctedKinds = ['images', 'pdf', 'office', 'audio'] as const;

export type CorrectedAlert = 'tooLarge' | 'webpTooLarge' | 'wrongType' | 'storageFull' | 'busy' | 'failed' | 'tooMany';

function uploadRefusal(error: ApiError): CorrectedAlert {
  switch (error.status) {
    case 507:
      return 'storageFull';
    case 503:
    case 429:
      return 'busy';
    case 413:
      return 'tooLarge';
    case 415:
      return 'wrongType';
    default:
      return error.fieldCode('file') === 'webp_too_large' ? 'webpTooLarge' : 'failed';
  }
}

function useCorrectedUploads(submissionId: string, questionId: string): UploadQueue {
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
            params: { query: { context: 'corrected', scope: `submission:${submissionId}:${questionId}` } },
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

export function CorrectedFiles({
  submissionId,
  questionId,
  files,
  stored,
  external,
  onChange,
}: {
  readonly submissionId: string;
  readonly questionId: string;
  readonly files: readonly CorrectedEntry[];
  readonly stored: readonly ReviewFile[];
  readonly external: { readonly reason: CorrectedAlert | 'fileGone'; readonly count: number } | null;
  readonly onChange: (files: readonly CorrectedEntry[]) => void;
}): ReactElement {
  const { t, i18n } = useTranslation();
  const queue = useCorrectedUploads(submissionId, questionId);
  const items = useSyncExternalStore(queue.subscribe, queue.snapshot);
  const [alert, setAlert] = useState<{ readonly reason: CorrectedAlert | 'fileGone'; readonly count: number } | null>(
    null,
  );
  const [seenExternal, setSeenExternal] = useState(0);
  if (external !== null && external.count !== seenExternal) {
    setSeenExternal(external.count);
    setAlert((previous) => ({ reason: external.reason, count: (previous?.count ?? 0) + 1 }));
  }
  const attachButton = useRef<HTMLButtonElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const filesValue = useRef(files);
  const onChangeValue = useRef(onChange);
  useEffect(() => {
    filesValue.current = files;
    onChangeValue.current = onChange;
  }, [files, onChange]);

  useEffect(() => {
    function settle(): void {
      const added: CorrectedEntry[] = [];
      for (const item of queue.snapshot()) {
        if (item.state === 'ready' && item.uploaded !== null) {
          queue.remove(item.key);
          added.push({
            attachmentId: item.uploaded.id,
            filename: item.uploaded.filename,
            sizeBytes: item.uploaded.size_bytes,
          });
        } else if (item.state === 'failed' && item.error !== null) {
          const reason = uploadRefusal(item.error);
          queue.remove(item.key);
          setAlert((previous) => ({ reason, count: (previous?.count ?? 0) + 1 }));
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
    const buttons = list.current?.querySelectorAll<HTMLButtonElement>('[data-remove-corrected]');
    (buttons?.[removed.index] ?? buttons?.[removed.index - 1] ?? attachButton.current)?.focus();
  }, [removed]);

  function refuse(reason: CorrectedAlert): void {
    setAlert((previous) => ({ reason, count: (previous?.count ?? 0) + 1 }));
  }

  function choose(chosen: readonly File[]): void {
    if (chosen.length === 0) {
      return;
    }
    if (files.length + items.length + chosen.length > correctedFilesPerAnswer) {
      refuse('tooMany');
      return;
    }
    for (const file of chosen) {
      const refusal = fileRefusal(file, correctedKinds);
      if (refusal !== null) {
        refuse(refusal);
        return;
      }
    }
    setAlert(null);
    queue.add(chosen);
  }

  return (
    <div className="flex flex-col gap-2">
      {files.length > 0 ? (
        <ul ref={list} className="flex flex-col gap-2">
          {files.map((file, index) => {
            const storedFile = stored.find((entry) => entry.attachmentId === file.attachmentId);
            return (
              <li key={file.attachmentId} className="flex flex-wrap items-center gap-x-3 gap-y-1">
                {storedFile === undefined ? (
                  <span dir="auto" className="min-w-0 break-words">
                    {file.filename}
                  </span>
                ) : (
                  <FileLink file={storedFile} />
                )}
                <span className="text-small text-text-muted">{formatStorage(file.sizeBytes, i18n.language)}</span>
                <button
                  type="button"
                  data-remove-corrected=""
                  aria-label={t('homework.review.removeNamed', { name: file.filename })}
                  onClick={() => {
                    onChange(files.filter((_, position) => position !== index));
                    setRemoved((previous) => ({ index, count: (previous?.count ?? 0) + 1 }));
                  }}
                  className={textButtonClass}
                >
                  {t('homework.review.remove')}
                </button>
              </li>
            );
          })}
        </ul>
      ) : null}
      {items.length > 0 ? (
        <ul className="flex flex-col gap-1">
          {items.map((item) => (
            <li key={item.key} className="text-small text-text-muted">
              <span dir="auto">{item.name}</span>{' '}
              {t('homework.review.uploading', { percent: `${String(item.progress ?? 0)}%` })}
            </li>
          ))}
        </ul>
      ) : null}
      {alert === null ? null : (
        <FormAlert key={alert.count}>
          {alert.reason === 'fileGone' ? t('homework.review.fileGone') : t(`homework.review.files.${alert.reason}`)}
        </FormAlert>
      )}
      <div>
        <button
          ref={attachButton}
          type="button"
          data-attach-corrected=""
          onClick={() => input.current?.click()}
          className={quietButtonClass}
        >
          {t('homework.review.attach')}
        </button>
        <input
          ref={input}
          type="file"
          hidden
          multiple
          accept={acceptList([...correctedKinds])}
          onChange={(event) => {
            choose(Array.from(event.currentTarget.files ?? []));
            event.currentTarget.value = '';
          }}
        />
      </div>
    </div>
  );
}
