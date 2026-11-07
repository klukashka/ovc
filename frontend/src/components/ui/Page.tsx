import type { ParseKeys } from 'i18next';
import { type ReactElement, type ReactNode, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { useRouteTitle } from '../../app/handles.ts';

export function Page({
  title: titleKey,
  headingText,
  headingDir,
  wide = false,
  focusTitle = false,
  action,
  children,
}: {
  readonly title?: ParseKeys;
  readonly headingText?: string;
  readonly headingDir?: 'auto';
  readonly wide?: boolean;
  readonly focusTitle?: boolean;
  readonly action?: ReactNode;
  readonly children?: ReactNode;
}): ReactElement {
  const { t } = useTranslation();
  const routeTitle = useRouteTitle();
  const title = headingText ?? t(titleKey ?? routeTitle);
  const heading = useRef<HTMLHeadingElement>(null);

  useEffect(() => {
    document.title = `${title} · ${t('app.name')}`;
  }, [title, t]);

  useEffect(() => {
    if (focusTitle) {
      heading.current?.focus();
    }
  }, [focusTitle]);

  const headingElement = (
    <h1
      ref={heading}
      tabIndex={-1}
      dir={headingDir}
      className={
        headingDir === undefined
          ? 'font-serif text-title outline-none desktop:text-display'
          : 'font-serif text-title break-words outline-none desktop:text-display'
      }
    >
      {title}
    </h1>
  );

  return (
    <article className={wide ? 'flex flex-col gap-6' : 'flex max-w-reading flex-col gap-6'}>
      {action === undefined ? (
        headingElement
      ) : (
        <header className="flex flex-wrap items-center justify-between gap-3">
          {headingElement}
          {action}
        </header>
      )}
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
