import { type ReactElement, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ApiError } from '../../lib/api/client.ts';
import { quietButtonClass } from './Form.tsx';

const loadingDelayMilliseconds = 300;

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

export function LoadFailed({
  error,
  onRetry,
}: {
  readonly error: ApiError;
  readonly onRetry: () => void;
}): ReactElement {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col items-start gap-3">
      <p role="alert" className="text-strong">
        {t('load.failed')}
      </p>
      <button type="button" onClick={onRetry} className={quietButtonClass}>
        {t('load.retry')}
      </button>
      <p className="text-caption text-text-muted">{t('load.requestId', { requestId: error.requestId ?? '—' })}</p>
    </div>
  );
}
