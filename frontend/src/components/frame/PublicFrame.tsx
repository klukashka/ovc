import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, Outlet } from 'react-router';

import { LanguageSwitch } from './LanguageSwitch.tsx';

export function PublicFrame(): ReactElement {
  const { t } = useTranslation();
  return (
    <div className="min-h-dvh bg-bg text-text">
      <header className="mx-auto flex w-full max-w-page flex-wrap items-center gap-4 px-4 py-4 tablet:px-8 desktop:px-14">
        <Link to="/" className="mr-auto font-serif text-heading">
          {t('app.name')}
        </Link>
        <LanguageSwitch />
        <nav aria-label={t('frame.navigation')} className="flex items-center gap-1">
          <Link to="/" className="inline-flex min-h-11 items-center px-2 text-small text-accent-text hover:underline">
            {t('public.signIn')}
          </Link>
          <Link
            to="/signup"
            className="inline-flex min-h-11 items-center px-2 text-small text-accent-text hover:underline"
          >
            {t('public.signUp')}
          </Link>
        </nav>
      </header>
      <main className="mx-auto w-full max-w-page px-4 py-10 tablet:px-8 desktop:px-14">
        <Outlet />
      </main>
    </div>
  );
}
