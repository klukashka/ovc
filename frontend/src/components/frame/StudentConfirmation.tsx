import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';

import { useApiClient } from '../../lib/api/client.ts';
import { done, refusedBy, useAction } from '../ui/action.ts';
import { FormNotice } from '../ui/Form.tsx';
import { Refusal } from '../ui/Refusal.tsx';

const resendButtonClass =
  'inline-flex min-h-11 items-center text-accent-text underline disabled:opacity-60 disabled:no-underline';

export function StudentConfirmation({ email }: { readonly email: string }): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const action = useAction();

  function resend(): void {
    action.run(async () => {
      const { error, response } = await client.POST('/api/v1/auth/verify-email/resend', { body: {} });
      return response.ok ? done : refusedBy(response, error);
    });
  }

  return (
    <>
      <p>{t('frame.studentConfirmationBody', { email })}</p>
      <button
        type="button"
        disabled={action.busy}
        aria-busy={action.busy}
        onClick={resend}
        className={resendButtonClass}
      >
        {t('frame.resendConfirmation')}
      </button>
      {action.state.kind === 'done' ? <FormNotice takeFocus>{t('checkEmail.sent')}</FormNotice> : null}
      <Refusal state={action.state} />
    </>
  );
}
