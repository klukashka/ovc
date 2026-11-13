import { type ReactElement, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { textButtonClass } from '../../components/ui/Form.tsx';
import type { AutosaveStatus } from './autosave.ts';

export function SaveStatus({
  status,
  onRetry,
}: {
  readonly status: AutosaveStatus;
  readonly onRetry: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const [retrying, setRetrying] = useState(false);
  const sending = status.kind === 'pending' || status.kind === 'saving';
  if (retrying && !sending && status.kind !== 'failed') {
    setRetrying(false);
  }
  const text = sending
    ? t('assignment.save.saving')
    : status.kind === 'saved'
      ? t('assignment.save.saved')
      : status.kind === 'failed'
        ? t('assignment.save.failed')
        : '';
  return (
    <div className="flex flex-wrap items-center gap-3">
      <p role="status" className="text-small text-text-muted">
        {text}
      </p>
      {status.kind === 'failed' || (retrying && sending) ? (
        <button
          type="button"
          aria-disabled={sending}
          aria-busy={sending}
          onClick={() => {
            if (!sending) {
              setRetrying(true);
              onRetry();
            }
          }}
          className={textButtonClass}
        >
          {t('assignment.save.tryNow')}
        </button>
      ) : null}
    </div>
  );
}
