import { type ComponentType, type ReactElement, useEffect, useLayoutEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { quietButtonClass } from '../components/ui/Form.tsx';
import { DelayedLoading, focusHeadingIfLost } from '../components/ui/LoadState.tsx';
import { Page } from '../components/ui/Page.tsx';

function headingHasFocus(): boolean {
  return document.activeElement?.matches('main h1') === true;
}

function PageLoading(): ReactElement {
  useLayoutEffect(
    () => (): void => {
      if (headingHasFocus()) {
        queueMicrotask(focusHeadingIfLost);
      }
    },
    [],
  );

  return (
    <Page>
      <DelayedLoading />
    </Page>
  );
}

function PageLoadFailed(): ReactElement {
  const { t } = useTranslation();
  return (
    <Page>
      <div className="flex flex-col items-start gap-3">
        <p role="alert" className="text-strong">
          {t('load.pageFailed')}
        </p>
        <button
          type="button"
          onClick={() => {
            window.location.reload();
          }}
          className={quietButtonClass}
        >
          {t('load.reload')}
        </button>
      </div>
    </Page>
  );
}

class PageModule {
  private page: ComponentType | null = null;
  private loading: Promise<ComponentType> | null = null;
  private readonly load: () => Promise<ComponentType>;

  constructor(load: () => Promise<ComponentType>) {
    this.load = load;
  }

  loaded(): ComponentType | null {
    return this.page;
  }

  start(): Promise<ComponentType> {
    this.loading ??= this.load().then(
      (page) => this.keep(page),
      (error: unknown) => {
        console.error('page_load_failed', error instanceof Error ? error.name : typeof error);
        return this.keep(PageLoadFailed);
      },
    );
    return this.loading;
  }

  private keep(page: ComponentType): ComponentType {
    this.page = page;
    return page;
  }
}

function LazyRoute({ module }: { readonly module: PageModule }): ReactElement {
  const [LoadedPage, setLoadedPage] = useState<ComponentType | null>(() => module.loaded());

  useEffect(() => {
    if (LoadedPage !== null) {
      return undefined;
    }
    let mounted = true;
    void module.start().then((page) => {
      if (mounted) {
        setLoadedPage(() => page);
      }
    });
    return (): void => {
      mounted = false;
    };
  }, [LoadedPage, module]);

  return LoadedPage === null ? <PageLoading /> : <LoadedPage />;
}

let pageModules = 0;

export function lazyPage(load: () => Promise<ComponentType>): ReactElement {
  pageModules += 1;
  return <LazyRoute key={`page-module-${String(pageModules)}`} module={new PageModule(load)} />;
}
