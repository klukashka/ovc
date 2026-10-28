import { describe, expect, test } from 'vitest';

import { FakeAuthServer } from './fakeTransport.ts';
import type { components } from './schema.ts';
import { type ContractSubject, type SeededLessons, authTransportContract } from './transportContract.ts';

type CalendarItem = components['schemas']['CalendarItem'];

const origin = 'https://ovc.example';

function fakeSubject(): ContractSubject {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'account-a', name: 'A tutor', email: 'a@example.org', password: 'a long enough password' });
  server.addAccount({ id: 'account-b', name: 'B tutor', email: 'b@example.org', password: 'another long password' });
  server.addAccount({
    id: 'student-a',
    name: 'A student',
    email: 's@example.org',
    password: 'a student password',
    role: 'student',
  });
  return {
    transport: server.transport(),
    origin,
    email: 'a@example.org',
    password: 'a long enough password',
    otherTutorEmail: 'b@example.org',
    otherTutorPassword: 'another long password',
    student: async (): Promise<{ readonly email: string; readonly password: string }> => {
      await Promise.resolve();
      return { email: 's@example.org', password: 'a student password' };
    },
    expireAccessTokens: async (): Promise<void> => {
      server.advance(16 * 60);
      await Promise.resolve();
    },
    blockAccount: async (): Promise<void> => {
      server.block('a@example.org');
      await Promise.resolve();
    },
    unsubscribeToken: async (): Promise<string> => {
      await Promise.resolve();
      return server.unsubscribeToken('a@example.org', 'lesson_reminders');
    },
    seedTwoLessons: async (): Promise<SeededLessons> => {
      await Promise.resolve();
      const lesson = (startsAt: string, endsAt: string): CalendarItem => ({
        id: `00000000-0000-4000-8000-${String(Date.parse(startsAt)).slice(-12)}`,
        title: 'Lesson',
        starts_at: startsAt,
        ends_at: endsAt,
        status: 'scheduled',
        timezone: 'Europe/Moscow',
        audience: { kind: 'one_to_one', label: 'A student', label_kind: null, colour: 'moss' },
        tutor: null,
      });
      server.addCalendarItem('a@example.org', lesson('2026-11-03T09:00:00Z', '2026-11-03T10:00:00Z'));
      server.addCalendarItem('a@example.org', lesson('2026-11-03T06:00:00Z', '2026-11-03T07:00:00Z'));
      return {
        from: '2026-11-03T05:00:00Z',
        to: '2026-11-03T12:00:00Z',
        laterStart: '2026-11-03T09:00:00Z',
        earlierStart: '2026-11-03T06:00:00Z',
      };
    },
  };
}

describe('the fake authentication transport', () => {
  test.each(Object.entries(authTransportContract))('keeps the contract: %s', async (_name, scenario) => {
    const subject = fakeSubject();

    const run = scenario(subject);

    await expect(run).resolves.toBeUndefined();
  });
});
