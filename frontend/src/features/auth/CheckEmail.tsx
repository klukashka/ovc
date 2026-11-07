import { type ReactElement, useEffect, useState } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';

import {
  ActionButton,
  FormAlert,
  errorMessage,
  linkClass,
  primaryButtonClass,
  retryAfterSeconds,
  retryMinutes,
} from '../../components/ui/Form.tsx';
import { LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { type ApiError, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';

const resendCooldownSeconds = 60;

type ResendAudience = { readonly kind: 'email'; readonly email: string } | { readonly kind: 'signedIn' };

type ResendState =
  | { readonly kind: 'idle' }
  | { readonly kind: 'sending' }
  | { readonly kind: 'sent' }
  | { readonly kind: 'failed'; readonly error: ApiError; readonly retryAfter: number | null };

function millisecondsToNextSecond(until: number, current: number): number {
  return ((until - current - 1) % 1000) + 1;
}

function useCooldown(initialSeconds: number): readonly [number, (seconds: number) => void] {
  const [until, setUntil] = useState(() => Date.now() + initialSeconds * 1000);
  const [now, setNow] = useState(() => Date.now());
  const remaining = Math.max(0, Math.ceil((until - now) / 1000));

  useEffect(() => {
    let timer: number | undefined;
    function tick(): void {
      const current = Date.now();
      setNow(current);
      if (current < until) {
        timer = window.setTimeout(tick, millisecondsToNextSecond(until, current));
      }
    }
    tick();
    return (): void => {
      window.clearTimeout(timer);
    };
  }, [until]);

  function restart(seconds: number): void {
    const started = Date.now();
    setNow(started);
    setUntil(started + seconds * 1000);
  }

  return [remaining, restart];
}

export function CheckEmail({
  email,
  audience,
}: {
  readonly email: string;
  readonly audience: ResendAudience;
}): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const [remaining, restart] = useCooldown(audience.kind === 'email' ? resendCooldownSeconds : 0);
  const [state, setState] = useState<ResendState>({ kind: 'idle' });

  async function resend(): Promise<void> {
    setState({ kind: 'sending' });
    const body = audience.kind === 'email' ? { email: audience.email } : {};
    const { error, response } = await client.POST('/api/v1/auth/verify-email/resend', { body });
    if (response.ok) {
      setState({ kind: 'sent' });
      restart(resendCooldownSeconds);
      return;
    }
    const retryAfter = retryAfterSeconds(response);
    setState({ kind: 'failed', error: apiErrorFrom(response.status, error), retryAfter });
    if (retryAfter !== null) {
      restart(retryAfter);
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <p>{t('checkEmail.body', { email })}</p>
      {audience.kind === 'email' ? (
        <p className="text-text-muted">
          {t('checkEmail.signInMeanwhile')}{' '}
          <Link to="/" className={linkClass}>
            {t('checkEmail.signIn')}
          </Link>
        </p>
      ) : null}
      <LiveNotice message={state.kind === 'sent' ? t('checkEmail.sent') : null} />
      {state.kind === 'failed' ? <FormAlert>{errorMessage(t, state.error, state.retryAfter)}</FormAlert> : null}
      <p>
        <ActionButton
          busy={state.kind === 'sending'}
          unavailable={remaining > 0}
          onClick={() => {
            resend().catch((thrown: unknown) => {
              setState({ kind: 'failed', error: asApiError(thrown), retryAfter: null });
            });
          }}
          className={primaryButtonClass}
        >
          {t('checkEmail.resend')}
        </ActionButton>
      </p>
      {remaining > 0 ? (
        <p className="text-small text-text-muted">
          {remaining > 60
            ? t('checkEmail.availableInMinutes', { minutes: retryMinutes(remaining) })
            : t('checkEmail.availableInSeconds', { seconds: remaining })}
        </p>
      ) : null}
    </div>
  );
}
