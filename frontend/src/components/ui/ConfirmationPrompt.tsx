import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { linkClass } from './Form.tsx';

export type ConfirmationReason = 'invite' | 'regenerateCode' | 'blockedStudents' | 'joinRequests';

export function ConfirmationPrompt({ reason }: { readonly reason: ConfirmationReason }): ReactElement {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col items-start gap-1">
      <p>{t(`confirmationPrompt.${reason}`)}</p>
      <Link to="/verify-email" className={linkClass}>
        {t('confirmationPrompt.action')}
      </Link>
    </div>
  );
}
