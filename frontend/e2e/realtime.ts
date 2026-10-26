import { expect } from '@playwright/test';

import { authFrame, closeCodes, parseServerFrame } from '../src/lib/realtime/frames.ts';
import {
  type RealtimeSocket,
  type SocketFactory,
  type WebSocketLike,
  realtimeAddress,
  webSocketFactory,
} from '../src/lib/realtime/socket.ts';
import { type TestAccount, testNetAddress } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { accessToken, callApi } from './tutoring.ts';

export interface Presence {
  readonly online: boolean;
  readonly sockets: number;
}

const presenceDeadlineMilliseconds = 10_000;

async function control(deployment: Deployment, path: string, body: unknown): Promise<unknown> {
  const response = await fetch(`${deployment.apiUrl}/__test__/${path}`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    throw new Error(`/__test__/${path} answered ${String(response.status)}`);
  }
  return response.json();
}

function isPresence(value: unknown): value is Presence {
  return (
    typeof value === 'object' &&
    value !== null &&
    'online' in value &&
    typeof value.online === 'boolean' &&
    'sockets' in value &&
    typeof value.sockets === 'number'
  );
}

export async function accountId(deployment: Deployment, account: TestAccount): Promise<string> {
  const answer = await callApi(deployment, await accessToken(deployment, account), 'GET', '/users/me');
  const body = answer.body;
  if (answer.status !== 200 || typeof body !== 'object' || body === null || !('id' in body)) {
    throw new Error(`reading the account answered ${String(answer.status)}`);
  }
  const id = body.id;
  if (typeof id !== 'string') {
    throw new Error('the account has no identifier');
  }
  return id;
}

export async function presence(deployment: Deployment, user: string): Promise<Presence> {
  const answer = await control(deployment, 'presence', { user });
  if (!isPresence(answer)) {
    throw new Error('the presence route answered an unexpected shape');
  }
  return answer;
}

export async function waitForSockets(deployment: Deployment, user: string, sockets: number): Promise<void> {
  await expect
    .poll(async () => (await presence(deployment, user)).sockets, { timeout: presenceDeadlineMilliseconds })
    .toBe(sockets);
}

export async function closeSockets(deployment: Deployment, user: string, code: 4429 | 1012): Promise<number> {
  const answer = await control(deployment, 'realtime/close', { user, code });
  if (typeof answer !== 'object' || answer === null || !('closed' in answer) || typeof answer.closed !== 'number') {
    throw new Error('the socket-closing route answered an unexpected shape');
  }
  return answer.closed;
}

type NodeWebSocketConstructor = new (
  address: string,
  init: { readonly headers: Readonly<Record<string, string>> },
) => WebSocketLike;

const NodeWebSocket = WebSocket as unknown as NodeWebSocketConstructor;
const socketsPerAccount = 20;

export function nodeSocketFactory(deployment: Deployment): SocketFactory {
  const address = realtimeAddress(deployment.apiUrl);
  const headers = { Origin: deployment.origin, 'X-Forwarded-For': testNetAddress() };
  return webSocketFactory(() => new NodeWebSocket(address, { headers }));
}

function authenticatedSocket(sockets: SocketFactory, token: string): Promise<RealtimeSocket> {
  return new Promise((resolve, reject) => {
    let ready = false;
    const socket = sockets({
      onOpen: () => {
        socket.send(authFrame(token));
      },
      onText: (text) => {
        if (!ready && parseServerFrame(text)?.type === 'ready') {
          ready = true;
          resolve(socket);
        }
      },
      onBinary: () => undefined,
      onClose: (code) => {
        if (!ready) {
          reject(new Error(`a socket holding a slot closed with ${String(code)} before it was ready`));
        }
      },
    });
  });
}

export interface SocketSlotHolder {
  readonly keepPageSocketsOff: (account: TestAccount) => Promise<void>;
  readonly release: () => void;
}

export function socketSlotHolder(deployment: Deployment): SocketSlotHolder {
  const held: RealtimeSocket[] = [];
  return {
    keepPageSocketsOff: async (account): Promise<void> => {
      const sockets = nodeSocketFactory(deployment);
      const token = await accessToken(deployment, account);
      for (let slot = 0; slot < socketsPerAccount; slot += 1) {
        held.push(await authenticatedSocket(sockets, token));
      }
    },
    release: (): void => {
      for (const socket of held) {
        socket.close(closeCodes.client);
      }
      held.splice(0);
    },
  };
}
