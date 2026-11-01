import { type ReactElement, useEffect, useRef, useState } from 'react';
import { useScheduler } from '../../app/clock.ts';
import type { components } from '../../lib/api/schema.ts';
import type { Message } from './api.ts';
import type { ChatTexts } from './translations.ts';

type Attachment = components['schemas']['MessageAttachment'];
export function attachmentAddress(file: Attachment, origin: string): string | null {
  try {
    const url = new URL(file.url, origin);
    if (
      url.origin !== origin ||
      url.username !== '' ||
      url.password !== '' ||
      url.hash !== '' ||
      url.pathname !== `/api/v1/files/${file.id}` ||
      url.searchParams.getAll('t').length !== 1 ||
      [...url.searchParams.keys()].some((key) => key !== 't')
    )
      return null;
    return url.href;
  } catch {
    return null;
  }
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
function AttachmentView({
  file,
  texts,
  refresh,
}: {
  readonly file: Attachment;
  readonly texts: ChatTexts;
  readonly refresh: () => Promise<boolean>;
}): ReactElement {
  const [attempt, setAttempt] = useState<'initial' | 'refreshing' | 'retried' | 'failed'>('initial');
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return (): void => {
      mounted.current = false;
    };
  }, []);
  const address = attachmentAddress(file, window.location.origin);
  const picture =
    file.width !== null &&
    file.height !== null &&
    file.width > 0 &&
    file.height > 0 &&
    file.media_type.startsWith('image/');
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
  return (
    <div className="flex flex-col gap-1">
      {address === null ? (
        <p>{texts.fileUnavailable}</p>
      ) : picture ? (
        <>
          {attempt === 'failed' ? (
            <p role="status">{texts.pictureFailed}</p>
          ) : attempt === 'refreshing' ? (
            <p>{texts.refreshingFile}</p>
          ) : (
            <img
              key={attempt}
              src={address}
              alt={file.filename}
              width={file.width ?? undefined}
              height={file.height ?? undefined}
              loading="lazy"
              referrerPolicy="no-referrer"
              className="h-auto max-w-full"
              onError={recover}
            />
          )}
          <span dir="auto">{file.filename}</span>
        </>
      ) : (
        <a href={address} referrerPolicy="no-referrer" className="text-accent-text underline" dir="auto">
          {file.filename}
        </a>
      )}
    </div>
  );
}
export function Attachments({
  message,
  texts,
  refresh,
}: {
  readonly message: Message;
  readonly texts: ChatTexts;
  readonly refresh: () => Promise<boolean>;
}): ReactElement | null {
  if (message.deleted || message.erased) return null;
  return (
    <div className="flex flex-col gap-2">
      {message.attachments.map((file) => (
        <AttachmentView key={file.id} file={file} texts={texts} refresh={refresh} />
      ))}
    </div>
  );
}
