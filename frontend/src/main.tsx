import './styles.css';

import { QueryClientProvider } from '@tanstack/react-query';
import { StrictMode } from 'react';
import { flushSync } from 'react-dom';
import { createRoot } from 'react-dom/client';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider } from 'react-router';

import { applyAccountPreferences, darkSchemeQuery, followSystemTheme } from './app/accountPreferences.ts';
import { ConnectionContext, ConnectionState } from './app/connectionState.ts';
import { PreferencesContext } from './app/context.ts';
import { DocumentLifecycle } from './app/documentLifecycle.ts';
import { FollowingQueryClient, accountCacheOwnership } from './app/queryClient.ts';
import { ApiClientContext, createApiClient } from './lib/api/client.ts';
import { type Me, Session, SessionContext, browserLocks, browserScheduler } from './lib/api/session.ts';
import { httpAuthTransport } from './lib/api/transport.ts';
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
const origin = window.location.origin;
const session = new Session({
  origin,
  transport: httpAuthTransport(origin, window.fetch.bind(window)),
  locks: browserLocks(navigator.locks),
  scheduler: browserScheduler,
});
const apiClient = createApiClient(origin, session.authorizedFetch);
const queryClient = new FollowingQueryClient();
const documentLifecycle = new DocumentLifecycle(
  { events: window, loaded: (): boolean => document.readyState === 'complete' },
  browserScheduler,
  queryClient,
);
const connection = new ConnectionState(browserScheduler);
connection.follow(session);

const ownAccountCache = accountCacheOwnership(queryClient);
let appliedAccount: Me | null = null;
let realtimeRequested = false;
session.subscribe(() => {
  const state = session.snapshot();
  const account = state.kind === 'signedIn' ? state.me : null;
  if (account !== null && !realtimeRequested) {
    realtimeRequested = true;
    import('./app/realtime.ts')
      .then(({ startRealtime }) => {
        startRealtime(session, queryClient, origin, connection, documentLifecycle);
      })
      .catch(reportError);
  }
  ownAccountCache(account);
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
followSystemTheme(document.documentElement, window.matchMedia(darkSchemeQuery));
void session.start();

createRoot(rootElement).render(
  <StrictMode>
    <ConnectionContext value={connection}>
      <PreferencesContext value={preferences}>
        <SessionContext value={session}>
          <ApiClientContext value={apiClient}>
            <QueryClientProvider client={queryClient}>
              <I18nextProvider i18n={i18n}>
                <RouterProvider router={createRouter()} flushSync={flushSync} />
              </I18nextProvider>
            </QueryClientProvider>
          </ApiClientContext>
        </SessionContext>
      </PreferencesContext>
    </ConnectionContext>
  </StrictMode>,
);
