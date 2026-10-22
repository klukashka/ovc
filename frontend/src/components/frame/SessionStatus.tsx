import { type ReactElement, useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import { useSession } from '../../lib/api/session.ts';

function StatusLayout({ children }: { readonly children: ReactElement }): ReactElement {
  return (
    <div className="flex min-h-dvh items-center justify-center bg-bg px-4 text-text">
      <main className="flex max-w-reading flex-col items-center gap-4 text-center">{children}</main>
    </div>
  );
}

export function SessionPending(): ReactElement {
  const { t } = useTranslation();

  useEffect(() => {
    document.title = `${t('session.loading')} · ${t('app.name')}`;
  }, [t]);
  return (
    <StatusLayout>
      <div role="status">
        <h1 className="text-body text-text-muted">{t('session.loading')}</h1>
      </div>
    </StatusLayout>
  );
}

export function SessionUnavailable(): ReactElement {
  const { t } = useTranslation();
  const session = useSession();

  useEffect(() => {
    document.title = `${t('session.unavailableTitle')} · ${t('app.name')}`;
  }, [t]);

  return (
    <StatusLayout>
      <>
        <h1 className="font-serif text-title">{t('session.unavailableTitle')}</h1>
        <p role="alert" className="text-text-muted">
          {t('session.unavailableBody')}
        </p>
        <button
          type="button"
          onClick={() => {
            void session.start();
          }}
          className="inline-flex min-h-11 items-center rounded-md bg-accent px-4 text-strong text-on-accent hover:bg-accent-hover"
        >
          {t('session.retry')}
        </button>
      </>
    </StatusLayout>
  );
}
