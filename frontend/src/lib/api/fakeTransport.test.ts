import { describe, expect, test } from 'vitest';

import { FakeAuthServer } from './fakeTransport.ts';
import { type ContractSubject, authTransportContract } from './transportContract.ts';

const origin = 'https://ovc.example';

function fakeSubject(): ContractSubject {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'account-a', name: 'A tutor', email: 'a@example.org', password: 'a long enough password' });
  return {
    transport: server.transport(),
    origin,
    email: 'a@example.org',
    password: 'a long enough password',
    expireAccessTokens: async (): Promise<void> => {
      server.advance(16 * 60);
      await Promise.resolve();
    },
    blockAccount: async (): Promise<void> => {
      server.block('a@example.org');
      await Promise.resolve();
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
