import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, test } from 'vitest';

import { ApiClientContext, createApiClient } from '../../lib/api/client.ts';
import { FakeAuthServer, manualScheduler, memoryLocks } from '../../lib/api/fakeTransport.ts';
import { Session } from '../../lib/api/session.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import { createI18n } from '../../lib/i18n/i18n.ts';
import { StorageUsage } from './StorageUsage.tsx';
import { teachingSettingsTexts as englishTeachingSettingsTexts } from './teachingTexts.en.ts';
import { teachingSettingsTexts as russianTeachingSettingsTexts } from './teachingTexts.ru.ts';

const origin = 'https://ovc.example';
const tutorEmail = 'tutor@example.org';
const fiveGigabytes = 5_000_000_000;

async function tutorWithUsage(usedBytes: number): Promise<Session> {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'tutor-1', name: 'A tutor', email: tutorEmail, password: 'a long password', role: 'tutor' });
  server.setTutorSettings(tutorEmail, {
    file_usage_bytes: usedBytes,
    file_quota_bytes: fiveGigabytes,
    onboarding_dismissed: false,
  });
  const session = new Session({
    origin,
    transport: server.transport(),
    locks: memoryLocks(),
    scheduler: manualScheduler(),
  });
  await session.signIn(tutorEmail, 'a long password');
  return session;
}

async function showUsage(session: Session, language: Language): Promise<void> {
  const i18n = await createI18n(language);
  i18n.addResourceBundle('en', 'translation', { teachingSettings: englishTeachingSettingsTexts }, true, true);
  i18n.addResourceBundle('ru', 'translation', { teachingSettings: russianTeachingSettingsTexts }, true, true);
  await act(async () => {
    render(
      <ApiClientContext value={createApiClient(origin, session.authorizedFetch)}>
        <QueryClientProvider client={new QueryClient()}>
          <I18nextProvider i18n={i18n}>
            <StorageUsage />
          </I18nextProvider>
        </QueryClientProvider>
      </ApiClientContext>,
    );
    await Promise.resolve();
  });
}

afterEach(() => {
  cleanup();
});

describe('the storage usage', () => {
  test('an empty storage of 5 GB reads in megabytes and gigabytes', async () => {
    const session = await tutorWithUsage(0);

    await showUsage(session, 'ru');

    expect(await screen.findByText('0 МБ из 5 ГБ')).toBeDefined();
  });

  test('a storage filled past one gigabyte reads in gigabytes', async () => {
    const session = await tutorWithUsage(1_500_000_000);

    await showUsage(session, 'ru');

    expect(await screen.findByText('1,5 ГБ из 5 ГБ')).toBeDefined();
  });

  test('a few kilobytes read as the smallest shown amount instead of zero', async () => {
    const session = await tutorWithUsage(40_000);

    await showUsage(session, 'en');

    expect(await screen.findByText('0.1 MB of 5 GB')).toBeDefined();
  });

  test('an amount just short of one gigabyte rounds into gigabytes', async () => {
    const session = await tutorWithUsage(999_960_000);

    await showUsage(session, 'en');

    expect(await screen.findByText('1 GB of 5 GB')).toBeDefined();
  });
});
