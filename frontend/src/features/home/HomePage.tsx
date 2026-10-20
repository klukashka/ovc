import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';

import { Page } from '../../components/ui/Page.tsx';
import { Readiness } from './Readiness.tsx';

export function HomePage(): ReactElement {
  const { t } = useTranslation();
  return (
    <Page>
      <p className="text-text-muted">{t('app.placeholder')}</p>
      <Readiness />
    </Page>
  );
}
