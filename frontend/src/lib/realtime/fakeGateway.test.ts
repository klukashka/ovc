import { describe, expect, test } from 'vitest';

import { FakeAuthServer } from '../api/fakeTransport.ts';
import { type RealtimeContractSubject, realtimeContract } from './realtimeContract.ts';
import { FakeGateway } from './fakeGateway.ts';

const tutor = { id: 'tutor-a', name: 'A tutor', email: 'a@example.org', password: 'a long enough password' };
const other = { id: 'tutor-b', name: 'B tutor', email: 'b@example.org', password: 'another long password' };

async function tokenOf(server: FakeAuthServer, account: typeof tutor): Promise<string> {
  const response = await server.transport().login(account.email, account.password);
  const body: unknown = await response.json();
  if (typeof body !== 'object' || body === null || !('access_token' in body) || typeof body.access_token !== 'string') {
    throw new Error('the fake server answered no token');
  }
  return body.access_token;
}

function fakeSubject(): RealtimeContractSubject {
  const server = new FakeAuthServer();
  server.addAccount(tutor);
  server.addAccount(other);
  const gateway = new FakeGateway(server);
  return {
    sockets: gateway.sockets,
    token: () => tokenOf(server, tutor),
    otherAccountToken: () => tokenOf(server, other),
    revoke: async (): Promise<void> => {
      server.revokeSessionsOf(tutor.email);
      gateway.recheck();
      await Promise.resolve();
    },
    closeSockets: async (code): Promise<void> => {
      gateway.closeAccount(tutor.id, code);
      await Promise.resolve();
    },
    changeRelationship: async (): Promise<void> => {
      gateway.changed(tutor.id, 'relationship', 'relationship-1');
      await Promise.resolve();
    },
    changeChannel: async (): Promise<void> => {
      gateway.changed(tutor.id, 'channel', 'channel-1');
      await Promise.resolve();
    },
    exchangeMessage: async (): Promise<void> => {
      gateway.messageExchanged(tutor.id, 'channel-1', 'message-1', 1);
      await Promise.resolve();
    },
    readChannel: async (): Promise<void> => {
      gateway.channelRead(tutor.id, 'channel-1');
      await Promise.resolve();
    },
    notify: async (): Promise<void> => {
      gateway.changed(tutor.id, 'join_request', 'request-1');
      gateway.notify(tutor.id, 'notification-1');
      await Promise.resolve();
    },
    readNotifications: async (): Promise<void> => {
      gateway.changed(tutor.id, 'notification', 'notification-1');
      await Promise.resolve();
    },
  };
}

describe('the fake realtime gateway', () => {
  test.each(Object.entries(realtimeContract))('keeps the contract: %s', async (_name, scenario) => {
    const subject = fakeSubject();

    const run = scenario(subject);

    await expect(run).resolves.toBeUndefined();
  });
});
