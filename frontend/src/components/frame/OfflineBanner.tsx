import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';

import { type ConnectionStatus, useConnectionStatus } from '../../app/connectionState.ts';

const statusClasses: Record<ConnectionStatus, string> = {
  waiting: 'sr-only',
  recovered: 'sr-only',
  offline: 'border-b border-border bg-surface px-4 py-3 text-small tablet:px-8 desktop:px-14',
};

export function OfflineBanner(): ReactElement {
  const { t } = useTranslation();
  const status = useConnectionStatus();
  return (
    <div aria-live="polite" aria-atomic="true" className={statusClasses[status]}>
      {status === 'waiting' ? '' : t(`connection.${status}`)}
    </div>
  );
}
