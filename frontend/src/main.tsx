import './styles.css';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider } from 'react-router';

import { applyAccountPreferences, darkSchemeQuery } from './app/accountPreferences.ts';
import { PreferencesContext } from './app/context.ts';
import { ApiClientContext, createApiClient } from './lib/api/client.ts';
import { type Me, Session, SessionContext, browserLocks, browserScheduler } from './lib/api/session.ts';
import { createI18n } from './lib/i18n/i18n.ts';
import { isLanguage } from './lib/i18n/resources.ts';
import { browserPreferenceStoreOrMemory } from './lib/preferences/store.ts';
import { createRouter } from './routes/router.tsx';

const rootElement = document.getElementById('root');
if (rootElement === null) {
  throw new Error('The #root element is missing from index.html');
}

const documentLanguage = document.documentElement.lang;
const i18n = await createI18n(isLanguage(documentLanguage) ? documentLanguage : 'ru');
i18n.on('languageChanged', (language: string) => {
  document.documentElement.lang = language;
});
const preferences = browserPreferenceStoreOrMemory();
const session = new Session({
  origin: window.location.origin,
  fetch: window.fetch.bind(window),
  locks: browserLocks(navigator.locks),
  scheduler: browserScheduler,
});
const apiClient = createApiClient(window.location.origin, session.authorizedFetch);
const queryClient = new QueryClient();

let appliedAccount: Me | null = null;
session.subscribe(() => {
  const state = session.snapshot();
  const account = state.kind === 'signedIn' ? state.me : null;
  if (account?.id !== appliedAccount?.id) {
    queryClient.clear();
  }
  if (account === null || account === appliedAccount) {
    appliedAccount = account;
    return;
  }
  appliedAccount = account;
  applyAccountPreferences(
    account,
    preferences,
    i18n,
    document.documentElement,
    window.matchMedia(darkSchemeQuery).matches,
  ).catch(reportError);
});
void session.start();

createRoot(rootElement).render(
  <StrictMode>
    <PreferencesContext value={preferences}>
      <SessionContext value={session}>
        <ApiClientContext value={apiClient}>
          <QueryClientProvider client={queryClient}>
            <I18nextProvider i18n={i18n}>
              <RouterProvider router={createRouter()} />
            </I18nextProvider>
          </QueryClientProvider>
        </ApiClientContext>
      </SessionContext>
    </PreferencesContext>
  </StrictMode>,
);
