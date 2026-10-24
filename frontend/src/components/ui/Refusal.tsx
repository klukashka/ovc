import type { TFunction } from 'i18next';
import { type ReactElement, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import type { ApiError } from '../../lib/api/client.ts';
import type { ActionState } from './action.ts';
import { type ConfirmationReason, ConfirmationPrompt } from './ConfirmationPrompt.tsx';
import { FormAlert, errorMessage } from './Form.tsx';

export type DailyLimit = 'cards' | 'emailChanges' | 'invitations' | 'code';

export function refusalText(
  t: TFunction,
  error: ApiError,
  retryAfter: number | null,
  dailyLimit: DailyLimit | undefined,
): string {
  if (error.code === 'rate_limited' && dailyLimit !== undefined) {
    return t(`limits.${dailyLimit}`);
  }
  if (error.fields.length > 0) {
    return t('errors.validation_failed');
  }
  return errorMessage(t, error, retryAfter);
}

function ConfirmationAlert({
  refusal,
  reason,
}: {
  readonly refusal: ActionState;
  readonly reason: ConfirmationReason;
}): ReactElement {
  const element = useRef<HTMLDivElement>(null);

  useEffect(() => {
    element.current?.focus();
  }, [refusal]);

  return (
    <div ref={element} role="alert" tabIndex={-1} className="outline-none">
      <ConfirmationPrompt reason={reason} />
    </div>
  );
}

export function Refusal({
  state,
  dailyLimit,
  confirmation,
}: {
  readonly state: ActionState;
  readonly dailyLimit?: DailyLimit;
  readonly confirmation?: ConfirmationReason;
}): ReactElement | null {
  const { t } = useTranslation();
  if (state.kind !== 'refused') {
    return null;
  }
  if (state.error.code === 'email_not_verified' && confirmation !== undefined) {
    return <ConfirmationAlert refusal={state} reason={confirmation} />;
  }
  return <FormAlert>{refusalText(t, state.error, state.retryAfter, dailyLimit)}</FormAlert>;
}
