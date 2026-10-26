import { type RealtimeContractSubject, realtimeContract } from '../src/lib/realtime/realtimeContract.ts';
import type { TestAccount } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { expect, seedVerifiedUser, test } from './fixtures.ts';
import { accountId, closeSockets, nodeSocketFactory } from './realtime.ts';
import { accessToken, callApi, createCard, seedJoinRequests } from './tutoring.ts';

function realSubject(deployment: Deployment, tutor: TestAccount): RealtimeContractSubject {
  return {
    sockets: nodeSocketFactory(deployment),
    token: () => accessToken(deployment, tutor),
    otherAccountToken: async (): Promise<string> =>
      accessToken(deployment, await seedVerifiedUser(deployment, 'Другой репетитор')),
    revoke: async (token): Promise<void> => {
      const answer = await callApi(deployment, token, 'POST', '/auth/logout');
      expect(answer.status).toBe(204);
    },
    closeSockets: async (code): Promise<void> => {
      await closeSockets(deployment, await accountId(deployment, tutor), code);
    },
    changeRelationship: async (): Promise<void> => {
      await createCard(deployment, tutor, 'Карточка для контракта', null);
    },
    notify: async (): Promise<void> => {
      await seedJoinRequests(deployment, tutor, 1);
    },
    readNotifications: async (): Promise<void> => {
      const token = await accessToken(deployment, tutor);
      const listed = await callApi(deployment, token, 'GET', '/notifications');
      const body = listed.body;
      const items = typeof body === 'object' && body !== null && 'items' in body ? body.items : undefined;
      const first: unknown = Array.isArray(items) ? items[0] : undefined;
      const id = typeof first === 'object' && first !== null && 'id' in first ? first.id : undefined;
      expect(typeof id).toBe('string');
      const read = await callApi(deployment, token, 'POST', '/notifications/read', { through: id });
      expect(read.status).toBe(200);
    },
  };
}

test.describe('the realtime gateway against the client parser and the fake socket', () => {
  for (const [name, scenario] of Object.entries(realtimeContract)) {
    test(`keeps the contract: ${name}`, async ({ deployment }) => {
      const subject = realSubject(deployment, await seedVerifiedUser(deployment, 'Репетитор для контракта'));

      const run = scenario(subject);

      await expect(run).resolves.toBeUndefined();
    });
  }
});
