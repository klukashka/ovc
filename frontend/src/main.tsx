import './styles.css';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { I18nextProvider } from 'react-i18next';
import { RouterProvider } from 'react-router';

import { PreferencesContext } from './app/context.ts';
import { ApiClientContext, createApiClient } from './lib/api/client.ts';
import { createI18n } from './lib/i18n/i18n.ts';
import { isLanguage } from './lib/i18n/resources.ts';
import { availablePreferenceStore } from './lib/preferences/store.ts';
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
const preferences = availablePreferenceStore(() => window.localStorage);
const apiClient = createApiClient(window.location.origin, window.fetch.bind(window));
const queryClient = new QueryClient();

createRoot(rootElement).render(
  <StrictMode>
    <PreferencesContext value={preferences}>
      <ApiClientContext value={apiClient}>
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={i18n}>
            <RouterProvider router={createRouter()} />
          </I18nextProvider>
        </QueryClientProvider>
      </ApiClientContext>
    </PreferencesContext>
  </StrictMode>,
);
