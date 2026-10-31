import { useContext } from 'react';
import { useTranslation } from 'react-i18next';

import { ConnectionContext, useConnectionStatus } from '../../app/connectionState.ts';

export function useChatWrite(
  busy = false,
  unavailable = false,
): {
  readonly unavailable: boolean;
  readonly reason: string | undefined;
  readonly run: (action: () => void) => void;
} {
  const { t } = useTranslation();
  const connection = useContext(ConnectionContext);
  const offline = useConnectionStatus() === 'offline';
  return {
    unavailable: unavailable || offline,
    reason: offline ? t('connection.reason') : undefined,
    run: (action): void => {
      if (!busy && !unavailable && connection.snapshot() !== 'offline') {
        action();
      }
    },
  };
}
