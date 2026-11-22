import { type ReactElement, type RefObject, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ApiError } from '../../lib/api/client.ts';
import { quietButtonClass } from './Form.tsx';
import { LiveNotice } from './LiveNotice.tsx';

export const loadingDelayMilliseconds = 300;

export function DelayedLoading(): ReactElement {
  const { t } = useTranslation();
  const [shown, setShown] = useState(false);

  useEffect(() => {
    const handle = window.setTimeout(() => {
      setShown(true);
    }, loadingDelayMilliseconds);
    return (): void => {
      window.clearTimeout(handle);
    };
  }, []);

  return (
    <div role="status" aria-busy="true" className="flex flex-col gap-3">
      {shown ? (
        <>
          <span className="sr-only">{t('session.loading')}</span>
          <span aria-hidden="true" className="h-11 w-full rounded-md bg-hover" />
          <span aria-hidden="true" className="h-11 w-full rounded-md bg-hover" />
          <span aria-hidden="true" className="h-11 w-2/3 rounded-md bg-hover" />
        </>
      ) : null}
    </div>
  );
}

export function focusIsLost(): boolean {
  const focused = document.activeElement;
  return focused === null || focused === document.body || !focused.isConnected || focused.matches('main');
}

export function focusHeadingIfLost(): void {
  if (focusIsLost()) {
    document.querySelector<HTMLElement>('main h1')?.focus();
  }
}

function useFocusAfterRetry(retried: { readonly current: boolean }, focusIfLost: () => void): void {
  const latest = useRef(focusIfLost);
  useEffect(() => {
    latest.current = focusIfLost;
  }, [focusIfLost]);
  useEffect(
    () => (): void => {
      if (!retried.current) {
        return;
      }
      latest.current();
    },
    [retried],
  );
}

function useFocusWhenRemovedHoldingFocus(
  container: RefObject<HTMLElement | null>,
  focusIfLost: () => void = focusHeadingIfLost,
): void {
  const heldFocus = useRef(false);
  const latest = useRef(focusIfLost);
  useEffect(() => {
    latest.current = focusIfLost;
  }, [focusIfLost]);
  useLayoutEffect(() => {
    const element = container.current;
    return (): void => {
      heldFocus.current = element?.contains(document.activeElement) ?? false;
    };
  }, [container]);
  useEffect(
    () => (): void => {
      if (heldFocus.current) {
        latest.current();
      }
    },
    [],
  );
}

export function LoadFailed({
  error,
  onRetry,
  focusIfLost = focusHeadingIfLost,
}: {
  readonly error: ApiError;
  readonly onRetry: () => void;
  readonly focusIfLost?: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const retried = useRef(false);
  const container = useRef<HTMLDivElement>(null);
  useFocusAfterRetry(retried, focusIfLost);
  useFocusWhenRemovedHoldingFocus(container, focusIfLost);
  return (
    <div ref={container} className="flex flex-col items-start gap-3">
      <p role="alert" className="text-strong">
        {t('load.failed')}
      </p>
      <button
        type="button"
        onClick={() => {
          retried.current = true;
          onRetry();
        }}
        className={quietButtonClass}
      >
        {t('load.retry')}
      </button>
      <p className="text-caption text-text-muted">{t('load.requestId', { requestId: error.requestId ?? '—' })}</p>
    </div>
  );
}

export function RefetchFailed({
  error,
  onRetry,
}: {
  readonly error: ApiError;
  readonly onRetry: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const retried = useRef(false);
  const container = useRef<HTMLDivElement>(null);
  useFocusAfterRetry(retried, focusHeadingIfLost);
  useFocusWhenRemovedHoldingFocus(container);
  return (
    <div ref={container} className="flex flex-col items-start gap-2">
      <LiveNotice message={t('load.refetchFailed')} />
      <button
        type="button"
        onClick={() => {
          retried.current = true;
          onRetry();
        }}
        className={quietButtonClass}
      >
        {t('load.retry')}
      </button>
      <p className="text-caption text-text-muted">{t('load.requestId', { requestId: error.requestId ?? '—' })}</p>
    </div>
  );
}

export function ModuleLoadFailed(): ReactElement {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col items-start gap-3">
      <p role="alert" className="text-strong">
        {t('load.pageFailed')}
      </p>
      <button
        type="button"
        onClick={() => {
          window.location.reload();
        }}
        className={quietButtonClass}
      >
        {t('load.reload')}
      </button>
    </div>
  );
}
