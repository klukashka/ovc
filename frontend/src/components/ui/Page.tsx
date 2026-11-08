import type { ParseKeys } from 'i18next';
import { type ReactElement, type ReactNode, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { useRouteTitle } from '../../app/handles.ts';

export type PageWidth = 'page' | 'reading';

export type TitleSize = 'title' | 'display';

const widthClass: Record<PageWidth, string> = {
  page: 'flex flex-col gap-6',
  reading: 'flex max-w-reading flex-col gap-6',
};

const titleClass: Record<TitleSize, string> = {
  title: 'font-serif text-title outline-none',
  display: 'font-serif text-title outline-none desktop:text-display',
};

export function Page({
  title: titleKey,
  headingText,
  headingDir,
  width = 'page',
  titleSize = 'title',
  titleHiddenOnDesktop = false,
  focusTitle = false,
  action,
  menu,
  children,
}: {
  readonly title?: ParseKeys;
  readonly headingText?: string;
  readonly headingDir?: 'auto';
  readonly width?: PageWidth;
  readonly titleSize?: TitleSize;
  readonly titleHiddenOnDesktop?: boolean;
  readonly focusTitle?: boolean;
  readonly action?: ReactNode;
  readonly menu?: ReactNode;
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
      className={`${titleClass[titleSize]}${headingDir === undefined ? '' : ' break-words'}${titleHiddenOnDesktop ? ' desktop:sr-only' : ''}`}
    >
      {title}
    </h1>
  );

  return (
    <article className={widthClass[width]}>
      {action === undefined && menu === undefined ? (
        headingElement
      ) : (
        <header className="flex flex-wrap items-center justify-between gap-3">
          {headingElement}
          <div className="flex flex-wrap items-center gap-2">
            {action}
            {menu}
          </div>
        </header>
      )}
      {children}
    </article>
  );
}

export function PlaceholderPage(): ReactElement {
  const { t } = useTranslation();
  return (
    <Page width="reading">
      <p className="text-text-muted">{t('app.placeholder')}</p>
    </Page>
  );
}
