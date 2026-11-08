import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { linkClass } from '../components/ui/Form.tsx';
import { Page } from '../components/ui/Page.tsx';

export function NotFoundContent(): ReactElement {
  const { t } = useTranslation();
  return (
    <>
      <p className="text-text-muted">{t('notFound.body')}</p>
      <p>
        <Link to="/" className={linkClass}>
          {t('notFound.back')}
        </Link>
      </p>
    </>
  );
}

export function NotFoundPage(): ReactElement {
  return (
    <Page width="reading" title="page.notFound">
      <NotFoundContent />
    </Page>
  );
}
