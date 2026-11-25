import { type ReactElement, useEffect, useRef, useState } from 'react';
import { useScheduler } from '../../app/clock.ts';
import { downloadAddress } from '../../lib/files/downloadAddress.ts';
import { extensionTile } from '../../lib/files/extension.ts';
import { localeOf } from '../../lib/format/dates.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import type { components } from '../../lib/api/schema.ts';
import type { Message } from './api.ts';
import { FileIcon } from './icons.tsx';
import type { ChatTexts } from './translations.ts';

type Attachment = components['schemas']['MessageAttachment'];
export function attachmentAddress(file: Attachment, origin: string): string | null {
  return downloadAddress(file.id, file.url, origin);
}
export function useAttachmentExpiry(
  messages: readonly Message[],
  refresh: () => Promise<boolean>,
  current: () => boolean,
): void {
  const scheduler = useScheduler();
  const refreshRef = useRef(refresh);
  const currentRef = useRef(current);
  useEffect(() => {
    refreshRef.current = refresh;
    currentRef.current = current;
  });
  const expiry = Math.min(
    ...messages.flatMap((message) => message.attachments.map((file) => Date.parse(file.expires_at))),
  );
  useEffect(() => {
    if (!Number.isFinite(expiry)) return;
    let active = true;
    let inFlight = false;
    let cancel = (): void => undefined;
    const arm = (): void => {
      cancel();
      if (!active || document.visibilityState === 'hidden' || !currentRef.current()) return;
      cancel = scheduler.schedule(Math.max(30_000, expiry - scheduler.wallClock() - 30_000), () => {
        if (!active || inFlight || !currentRef.current()) return;
        inFlight = true;
        void refreshRef.current().finally(() => {
          inFlight = false;
        });
      });
    };
    const hide = (): void => {
      cancel();
    };
    window.addEventListener('pagehide', hide);
    window.addEventListener('pageshow', arm);
    document.addEventListener('visibilitychange', arm);
    arm();
    return (): void => {
      active = false;
      cancel();
      window.removeEventListener('pagehide', hide);
      window.removeEventListener('pageshow', arm);
      document.removeEventListener('visibilitychange', arm);
    };
  }, [expiry, scheduler]);
}
type PictureAttempt = 'initial' | 'refreshing' | 'retried' | 'failed';

function usePictureRecovery(refresh: () => Promise<boolean>): {
  readonly attempt: PictureAttempt;
  readonly recover: () => void;
} {
  const [attempt, setAttempt] = useState<PictureAttempt>('initial');
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return (): void => {
      mounted.current = false;
    };
  }, []);
  const recover = (): void => {
    if (attempt !== 'initial') {
      setAttempt('failed');
      return;
    }
    setAttempt('refreshing');
    void refresh().then(
      (refreshed) => {
        if (mounted.current) setAttempt(refreshed ? 'retried' : 'failed');
      },
      () => {
        if (mounted.current) setAttempt('failed');
      },
    );
  };
  return { attempt, recover };
}

interface PictureFile extends Attachment {
  readonly width: number;
  readonly height: number;
}

function isPicture(file: Attachment): file is PictureFile {
  return (
    file.width !== null &&
    file.height !== null &&
    file.width > 0 &&
    file.height > 0 &&
    file.media_type.startsWith('image/')
  );
}

function PictureView({
  file,
  texts,
  refresh,
  fill,
}: {
  readonly file: PictureFile;
  readonly texts: ChatTexts;
  readonly refresh: () => Promise<boolean>;
  readonly fill: boolean;
}): ReactElement {
  const { attempt, recover } = usePictureRecovery(refresh);
  const address = attachmentAddress(file, window.location.origin);
  const notice = 'flex size-full items-center justify-center p-2 text-center text-small text-text-muted';
  if (address === null) return <p className={notice}>{texts.fileUnavailable}</p>;
  if (attempt === 'failed')
    return (
      <p role="status" className={notice}>
        {texts.pictureFailed}
      </p>
    );
  if (attempt === 'refreshing') return <p className={notice}>{texts.refreshingFile}</p>;
  return (
    <img
      key={attempt}
      src={address}
      alt={file.filename}
      width={file.width}
      height={file.height}
      loading="lazy"
      referrerPolicy="no-referrer"
      className={fill ? 'size-full object-cover' : 'block h-auto max-w-full'}
      onError={recover}
    />
  );
}

const albumColumnsUpToFour = 2;
const albumColumnsAboveFour = 3;

export function albumLayout(count: number): { readonly columns: number; readonly leadSpan: number } {
  const columns = count <= 4 ? albumColumnsUpToFour : albumColumnsAboveFour;
  const leftover = count % columns;
  return { columns, leadSpan: leftover === 0 ? 1 : columns - leftover + 1 };
}

function Album({
  pictures,
  texts,
  refresh,
}: {
  readonly pictures: readonly PictureFile[];
  readonly texts: ChatTexts;
  readonly refresh: () => Promise<boolean>;
}): ReactElement {
  const { columns, leadSpan } = albumLayout(pictures.length);
  return (
    <div
      role="group"
      aria-label={texts.album.replace('{count}', String(pictures.length))}
      className={`grid w-80 max-w-full gap-0.5 overflow-hidden rounded-md ${columns === albumColumnsUpToFour ? 'grid-cols-2' : 'grid-cols-3'}`}
    >
      {pictures.map((file, index) => {
        const span = index === 0 ? leadSpan : 1;
        return (
          <div
            key={file.id}
            className="min-w-0 overflow-hidden bg-hover"
            style={{ gridColumn: `span ${String(span)}`, aspectRatio: span }}
          >
            <PictureView file={file} texts={texts} refresh={refresh} fill />
          </div>
        );
      })}
    </div>
  );
}

const bytesPerKilobyte = 1_000;
const bytesPerMegabyte = 1_000_000;
const smallestRoundedMegabyte = 999_500;

export function fileSize(bytes: number, language: Language): string {
  const [value, unit] =
    bytes >= smallestRoundedMegabyte
      ? [bytes / bytesPerMegabyte, 'megabyte']
      : bytes >= bytesPerKilobyte
        ? [bytes / bytesPerKilobyte, 'kilobyte']
        : [bytes, 'byte'];
  return new Intl.NumberFormat(localeOf(language), {
    style: 'unit',
    unit,
    unitDisplay: 'short',
    maximumFractionDigits: unit === 'megabyte' ? 1 : 0,
  }).format(value);
}

function FileCard({
  file,
  texts,
  language,
}: {
  readonly file: Attachment;
  readonly texts: ChatTexts;
  readonly language: Language;
}): ReactElement {
  const address = attachmentAddress(file, window.location.origin);
  const tile = extensionTile(file.filename);
  return (
    <div className="relative flex min-w-0 items-center gap-3 rounded-md border border-border bg-bg p-2 hover:bg-surface">
      <span
        aria-hidden="true"
        className="flex size-10 shrink-0 items-center justify-center rounded-md bg-accent text-label text-on-accent"
      >
        {tile ?? <FileIcon size={20} />}
      </span>
      <span className="flex min-w-0 flex-col">
        {address === null ? (
          <span>{texts.fileUnavailable}</span>
        ) : (
          <a
            href={address}
            referrerPolicy="no-referrer"
            dir="auto"
            className="text-strong text-accent-text [overflow-wrap:anywhere] after:absolute after:inset-0 after:rounded-md"
          >
            {file.filename}
          </a>
        )}
        <span className="text-small text-text-muted">{fileSize(file.size_bytes, language)}</span>
      </span>
    </div>
  );
}

export function Attachments({
  message,
  texts,
  language,
  refresh,
}: {
  readonly message: Message;
  readonly texts: ChatTexts;
  readonly language: Language;
  readonly refresh: () => Promise<boolean>;
}): ReactElement | null {
  if (message.deleted || message.erased || message.attachments.length === 0) return null;
  const pictures = message.attachments.filter(isPicture);
  const files = message.attachments.filter((file) => !isPicture(file));
  const [onlyPicture] = pictures;
  return (
    <div className="flex min-w-0 flex-col gap-2">
      {pictures.length > 1 ? (
        <Album pictures={pictures} texts={texts} refresh={refresh} />
      ) : onlyPicture === undefined ? null : (
        <div className="w-fit max-w-[min(100%,320px)] overflow-hidden rounded-md">
          <PictureView file={onlyPicture} texts={texts} refresh={refresh} fill={false} />
        </div>
      )}
      {files.map((file) => (
        <FileCard key={file.id} file={file} texts={texts} language={language} />
      ))}
    </div>
  );
}
