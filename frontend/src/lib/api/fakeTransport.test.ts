import { describe, expect, test } from 'vitest';

import { FakeAuthServer } from './fakeTransport.ts';
import { type ContractSubject, authTransportContract } from './transportContract.ts';

const origin = 'https://ovc.example';

function fakeSubject(): ContractSubject {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'account-a', name: 'A tutor', email: 'a@example.org', password: 'a long enough password' });
  server.addAccount({ id: 'account-b', name: 'B tutor', email: 'b@example.org', password: 'another long password' });
  return {
    transport: server.transport(),
    origin,
    email: 'a@example.org',
    password: 'a long enough password',
    otherTutorEmail: 'b@example.org',
    otherTutorPassword: 'another long password',
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

describe('the fake card routes', () => {
  test('refuse a student as the backend refuses a tutor route to the other role', async () => {
    const server = new FakeAuthServer();
    server.addAccount({
      id: 'student-a',
      name: 'A student',
      email: 's@example.org',
      password: 'a long enough password',
      role: 'student',
    });
    const transport = server.transport();
    const login: unknown = await (await transport.login('s@example.org', 'a long enough password')).json();
    const token =
      typeof login === 'object' && login !== null && 'access_token' in login ? String(login.access_token) : '';

    const created = await transport.send(
      new Request(`${origin}/api/v1/students`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
        body: JSON.stringify({ display_name: 'Someone' }),
      }),
    );

    expect(created.status).toBe(404);
  });
});
