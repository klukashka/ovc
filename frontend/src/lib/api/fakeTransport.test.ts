import { describe, expect, test } from 'vitest';

import { FakeAuthServer } from './fakeTransport.ts';
import { type ContractSubject, authTransportContract } from './transportContract.ts';

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
    unsubscribeToken: async (group): Promise<string> => {
      await Promise.resolve();
      return server.unsubscribeToken('a@example.org', group);
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
