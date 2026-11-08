import { type ReactElement, type RefObject, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { ActionButton, errorMessage, fieldMessage, quietButtonClass } from '../../components/ui/Form.tsx';
import { AttachIcon } from '../../components/ui/icons.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import type { UploadItem } from './UploadQueue.ts';
import type { ChatTexts } from './translations.ts';

function UploadFailure({ item, texts }: { readonly item: UploadItem; readonly texts: ChatTexts }): ReactElement | null {
  const { t } = useTranslation();
  const notice = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (item.state === 'failed') notice.current?.focus();
  }, [item.state]);
  return item.error === null ? null : (
    <p ref={notice} role="alert" tabIndex={-1}>
      {fieldMessage(t, item.error, 'file') ??
        (item.error.code === 'payload_too_large' ? texts.fileTooLarge : errorMessage(t, item.error, null))}
    </p>
  );
}

const attachButtonClass =
  'inline-flex size-11 items-center justify-center rounded-md border border-border-strong text-icon hover:bg-surface aria-disabled:opacity-60 desktop:pointer-fine:size-10';

export function UploadComposer({
  items,
  texts,
  confirmed,
  busy,
  unavailable,
  reason,
  choose,
  remove,
  focusFallback,
}: {
  readonly focusFallback: RefObject<HTMLTextAreaElement | null>;
  readonly items: readonly UploadItem[];
  readonly texts: ChatTexts;
  readonly confirmed: boolean;
  readonly busy: boolean;
  readonly unavailable: boolean;
  readonly reason: string | undefined;
  readonly choose: (files: readonly File[]) => void;
  readonly remove: (key: number) => void;
}): ReactElement {
  const picker = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLUListElement>(null);
  useFocusAfterRemovedRow(
    list,
    items.map((item) => String(item.key)),
    'button',
    focusFallback,
  );
  return (
    <div className="flex flex-col gap-2">
      {confirmed ? (
        <>
          <input
            ref={picker}
            type="file"
            multiple
            tabIndex={-1}
            className="hidden"
            aria-label={texts.attach}
            onChange={(event) => {
              choose(Array.from(event.target.files ?? []));
              event.target.value = '';
            }}
          />
          <ActionButton
            type="button"
            className={attachButtonClass}
            busy={busy}
            unavailable={unavailable}
            unavailableReason={reason}
            label={texts.attach}
            onClick={() => {
              if (!busy && !unavailable) picker.current?.click();
            }}
          >
            <AttachIcon />
          </ActionButton>
        </>
      ) : (
        <p>{texts.confirmFiles}</p>
      )}
      <ul ref={list} className="flex flex-col gap-2" aria-label={texts.attachments}>
        {items.map((item) => (
          <li key={item.key} data-row={String(item.key)}>
            <span dir="auto">{item.name}</span>
            {item.state === 'uploading' || item.state === 'waiting' ? (
              <progress max={100} value={item.progress ?? undefined} aria-label={item.name} />
            ) : null}
            {item.state === 'ready' ? <span> {texts.uploaded}</span> : null}
            <button
              type="button"
              className={quietButtonClass}
              aria-disabled={busy}
              onClick={() => {
                if (!busy) remove(item.key);
              }}
            >
              {texts.removeFile}
            </button>
            <UploadFailure item={item} texts={texts} />
          </li>
        ))}
      </ul>
    </div>
  );
}
