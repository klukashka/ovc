import { type ReactElement, type ReactNode, useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import { useRouteTitle } from '../../app/handles.ts';

export function Page({ children }: { readonly children?: ReactNode }): ReactElement {
  const { t } = useTranslation();
  const title = t(useRouteTitle());

  useEffect(() => {
    document.title = `${title} · ${t('app.name')}`;
  }, [title, t]);

  return (
    <article className="flex max-w-reading flex-col gap-6">
      <h1 tabIndex={-1} className="font-serif text-title outline-none desktop:text-display">
        {title}
      </h1>
      {children}
    </article>
  );
}

export function PlaceholderPage(): ReactElement {
  const { t } = useTranslation();
  return (
    <Page>
      <p className="text-text-muted">{t('app.placeholder')}</p>
    </Page>
  );
}
