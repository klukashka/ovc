import type { ParseKeys } from 'i18next';
import { type ReactElement, type ReactNode, useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import { useRouteTitle } from '../../app/handles.ts';

export function Page({
  title: titleKey,
  children,
}: {
  readonly title?: ParseKeys;
  readonly children?: ReactNode;
}): ReactElement {
  const { t } = useTranslation();
  const routeTitle = useRouteTitle();
  const title = t(titleKey ?? routeTitle);

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
