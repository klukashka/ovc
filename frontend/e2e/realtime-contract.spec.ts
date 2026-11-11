import { authFrame, closeCodes, parseServerFrame } from '../src/lib/realtime/frames.ts';
import { type RealtimeContractSubject, realtimeContract } from '../src/lib/realtime/realtimeContract.ts';
import type { TestAccount } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { expect, seedVerifiedUser, test } from './fixtures.ts';
import { accountId, closeSockets, nodeSocketFactory } from './realtime.ts';
import { accessToken, callApi, createCard, createGroup, seedActiveStudent, seedJoinRequests } from './tutoring.ts';

function textField(answer: { readonly body: unknown }, name: string): string {
  const body = answer.body;
  const value =
    typeof body === 'object' && body !== null && name in body ? (body as Record<string, unknown>)[name] : undefined;
  if (typeof value !== 'string') {
    throw new Error(`the answer has no ${name}`);
  }
  return value;
}

async function connectAndType(deployment: Deployment, token: string, channel: string): Promise<() => void> {
  const sockets = nodeSocketFactory(deployment);
  return new Promise((resolve, reject) => {
    let ready = false;
    const socket = sockets({
      onOpen: () => {
        socket.send(authFrame(token));
      },
      onText: (text) => {
        if (ready || parseServerFrame(text)?.type !== 'ready') {
          return;
        }
        ready = true;
        socket.send(JSON.stringify({ type: 'typing.start', payload: { channel_id: channel } }));
        resolve(() => {
          socket.close(closeCodes.client);
        });
      },
      onBinary: () => undefined,
      onClose: (code) => {
        if (!ready) {
          reject(new Error(`the member's socket closed with ${String(code)}`));
        }
      },
    });
  });
}

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
    changeChannel: async (): Promise<void> => {
      await createGroup(deployment, tutor, 'Группа для контракта', []);
    },
    issueAssignment: async (): Promise<void> => {
      const token = await accessToken(deployment, tutor);
      const { cardId } = await seedActiveStudent(deployment, tutor, 'Ученик для задания');
      const template = await callApi(deployment, token, 'POST', '/library/templates', {
        title: 'Задание для контракта',
        folder_id: null,
        blocks: [{ id: '5f0c1a2b-3c4d-4e5f-8a6b-7c8d9e0f1a2b', type: 'instruction', text: 'Прочитайте текст' }],
      });
      expect(template.status).toBe(201);
      const issued = await callApi(deployment, token, 'POST', '/issues', {
        template_id: textField(template, 'id'),
        audience: { relationship_id: cardId },
        lesson_id: null,
        deadline_at: null,
        answer_policy: 'never',
      });
      expect(issued.status).toBe(201);
    },
    exchangeMessage: async (): Promise<void> => {
      const token = await accessToken(deployment, tutor);
      const group = await createGroup(deployment, tutor, 'Группа для сообщений', []);
      const channel = textField(await callApi(deployment, token, 'GET', `/groups/${group}`), 'channel_id');
      const sent = await callApi(deployment, token, 'POST', `/channels/${channel}/messages`, {
        body: 'Привет',
        attachment_ids: [],
      });
      expect(sent.status).toBe(201);
      const message = textField(sent, 'id');
      const edited = await callApi(deployment, token, 'PATCH', `/messages/${message}`, {
        body: 'Привет всем',
        version: 1,
      });
      expect(edited.status).toBe(200);
      const deleted = await callApi(deployment, token, 'DELETE', `/messages/${message}`);
      expect(deleted.status).toBe(204);
    },
    readChannel: async (): Promise<void> => {
      const token = await accessToken(deployment, tutor);
      const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Ученица для контракта');
      const channel = textField(await callApi(deployment, token, 'GET', `/students/${cardId}`), 'channel_id');
      const sent = await callApi(
        deployment,
        await accessToken(deployment, student),
        'POST',
        `/channels/${channel}/messages`,
        {
          body: 'Здравствуйте',
          attachment_ids: [],
        },
      );
      expect(sent.status).toBe(201);
      const read = await callApi(deployment, token, 'POST', `/channels/${channel}/read`, { seq: 1 });
      expect(read.status).toBe(200);
    },
    memberConnectsAndTypes: async (): Promise<() => void> => {
      const token = await accessToken(deployment, tutor);
      const { student, cardId } = await seedActiveStudent(deployment, tutor, 'Ученик для присутствия');
      const channel = textField(await callApi(deployment, token, 'GET', `/students/${cardId}`), 'channel_id');
      return connectAndType(deployment, await accessToken(deployment, student), channel);
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
