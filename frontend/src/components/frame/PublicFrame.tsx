import { type ReactElement, type ReactNode, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, Outlet } from 'react-router';

import { useSession, useSessionState } from '../../lib/api/session.ts';
import { LanguageSwitch } from './LanguageSwitch.tsx';

export function PublicFrame({ children }: { readonly children?: ReactNode }): ReactElement {
  const { t } = useTranslation();
  const session = useSession();
  const signedIn = useSessionState().kind === 'signedIn';
  const [signOutFailed, setSignOutFailed] = useState(false);
  return (
    <div className="min-h-dvh bg-bg text-text">
      <header className="mx-auto flex w-full max-w-page flex-wrap items-center gap-4 px-4 py-4 tablet:px-8 desktop:px-14">
        <Link to="/" className="mr-auto inline-flex min-h-11 items-center font-serif text-heading">
          {t('app.name')}
        </Link>
        <LanguageSwitch />
        {signedIn ? (
          <button
            type="button"
            onClick={() => {
              setSignOutFailed(false);
              session.signOut().catch(() => {
                setSignOutFailed(true);
              });
            }}
            className="inline-flex min-h-11 items-center px-2 text-small text-accent-text hover:underline"
          >
            {t('frame.signOut')}
          </button>
        ) : null}
        {signedIn ? null : (
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
        )}
      </header>
      {signedIn && signOutFailed ? (
        <p role="alert" className="mx-auto w-full max-w-page px-4 text-small text-danger tablet:px-8 desktop:px-14">
          {t('frame.signOutFailed')}
        </p>
      ) : null}
      <main className="mx-auto w-full max-w-page px-4 py-10 tablet:px-8 desktop:px-14">{children ?? <Outlet />}</main>
    </div>
  );
}
