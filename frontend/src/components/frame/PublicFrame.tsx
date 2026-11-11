import { type ReactElement, type ReactNode, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, Outlet, useNavigate } from 'react-router';

import { useApiClient } from '../../lib/api/client.ts';
import { useSession, useSessionState, useSignOut } from '../../lib/api/session.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import { ActionButton } from '../ui/Form.tsx';
import { LanguageSwitch } from './LanguageSwitch.tsx';
import { SkipLink, mainContentId } from './SkipLink.tsx';

export function PublicFrame({ children }: { readonly children?: ReactNode }): ReactElement {
  const { t } = useTranslation();
  const session = useSession();
  const client = useApiClient();
  const navigate = useNavigate();
  const signedIn = useSessionState().kind === 'signedIn';
  const signing = useSignOut(() => {
    void navigate('/', { replace: true });
  });
  const [languageNotSaved, setLanguageNotSaved] = useState(false);
  const latestLanguageSave = useRef(0);

  async function saveLanguage(language: Language): Promise<void> {
    latestLanguageSave.current += 1;
    const attempt = latestLanguageSave.current;
    const saved = await client.PATCH('/api/v1/users/me', { body: { language } }).then(
      ({ data }) => data,
      () => undefined,
    );
    if (attempt !== latestLanguageSave.current) {
      return;
    }
    if (saved === undefined) {
      setLanguageNotSaved(true);
      return;
    }
    session.updateAccount(saved);
  }

  return (
    <div className="min-h-dvh bg-bg text-text">
      <SkipLink />
      <header className="mx-auto flex w-full max-w-page flex-wrap items-center gap-4 px-4 py-4 tablet:px-8 desktop:px-14">
        <Link to="/" className="mr-auto inline-flex min-h-11 min-w-11 items-center font-serif text-heading">
          {t('app.name')}
        </Link>
        <LanguageSwitch
          onChosen={
            signedIn
              ? (language): void => {
                  setLanguageNotSaved(false);
                  saveLanguage(language).catch(() => {
                    setLanguageNotSaved(true);
                  });
                }
              : undefined
          }
        />
        {signedIn ? (
          <ActionButton
            busy={signing.state === 'busy'}
            onClick={signing.signOut}
            className="inline-flex min-h-11 items-center px-2 text-small text-accent-text hover:underline"
          >
            {signing.state === 'busy' ? t('frame.signingOut') : t('frame.signOut')}
          </ActionButton>
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
      {signedIn && languageNotSaved ? (
        <p role="alert" className="mx-auto w-full max-w-page px-4 text-small text-danger tablet:px-8 desktop:px-14">
          {t('frame.languageNotSaved')}
        </p>
      ) : null}
      {signedIn && signing.state === 'failed' ? (
        <p role="alert" className="mx-auto w-full max-w-page px-4 text-small text-danger tablet:px-8 desktop:px-14">
          {t('frame.signOutFailed')}
        </p>
      ) : null}
      <main
        id={mainContentId}
        tabIndex={-1}
        className="mx-auto w-full max-w-page px-4 py-10 outline-none tablet:px-8 desktop:px-14"
      >
        {children ?? <Outlet />}
      </main>
    </div>
  );
}
