import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { Page } from '../components/ui/Page.tsx';

export function NotFoundPage(): ReactElement {
  const { t } = useTranslation();
  return (
    <Page>
      <p className="text-text-muted">{t('notFound.body')}</p>
      <p>
        <Link to="/" className="text-accent-text underline">
          {t('notFound.back')}
        </Link>
      </p>
    </Page>
  );
}
