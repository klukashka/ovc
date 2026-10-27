import { type Page, type WebSocket as PageWebSocket, type Request, expect } from '@playwright/test';

import { authFrame, closeCodes, parseServerFrame } from '../src/lib/realtime/frames.ts';
import {
  type RealtimeSocket,
  type SocketFactory,
  type WebSocketLike,
  realtimeAddress,
  realtimePath,
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

export interface TrafficAfterReady {
  readonly readyReceived: () => boolean;
  readonly unsettled: () => string[];
}

function apiPathOf(request: Request): string | null {
  const path = new URL(request.url()).pathname;
  return path.startsWith('/api/') && path !== realtimePath ? path : null;
}

export function trafficAfterReady(page: Page, queryPaths: readonly string[]): TrafficAfterReady {
  let ready = false;
  const pending = new Set<Request>();
  const answeredAfterReady = new Set<string>();
  page.on('websocket', (socket) => {
    socket.on('framereceived', ({ payload }) => {
      if (typeof payload === 'string' && parseServerFrame(payload)?.type === 'ready') {
        ready = true;
      }
    });
  });
  const startedAfterReady = new Set<Request>();
  page.on('request', (request) => {
    if (apiPathOf(request) !== null) {
      pending.add(request);
      if (ready) {
        startedAfterReady.add(request);
      }
    }
  });
  const settled = (request: Request): void => {
    const path = apiPathOf(request);
    pending.delete(request);
    if (path !== null && startedAfterReady.has(request)) {
      answeredAfterReady.add(path);
    }
  };
  page.on('requestfinished', settled);
  page.on('requestfailed', settled);
  return {
    readyReceived: () => ready,
    unsettled: () => [
      ...[...pending].map((request) => apiPathOf(request) ?? ''),
      ...queryPaths.filter((path) => !answeredAfterReady.has(path)),
    ],
  };
}

export async function expectRefetchedAfterTheSocketIsReady(traffic: TrafficAfterReady): Promise<void> {
  await expect.poll(() => traffic.readyReceived()).toBe(true);
  await expect.poll(() => traffic.unsettled()).toEqual([]);
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
const socketSlotSearchCeiling = 100;

export function nodeSocketFactory(deployment: Deployment): SocketFactory {
  const address = realtimeAddress(deployment.apiUrl);
  const headers = { Origin: deployment.origin, 'X-Forwarded-For': testNetAddress() };
  return webSocketFactory(() => new NodeWebSocket(address, { headers }));
}

type SlotAttempt = { readonly held: RealtimeSocket } | { readonly refused: true };

function slotAttempt(sockets: SocketFactory, token: string): Promise<SlotAttempt> {
  return new Promise((resolve, reject) => {
    let ready = false;
    const socket = sockets({
      onOpen: () => {
        socket.send(authFrame(token));
      },
      onText: (text) => {
        if (!ready && parseServerFrame(text)?.type === 'ready') {
          ready = true;
          resolve({ held: socket });
        }
      },
      onBinary: () => undefined,
      onClose: (code) => {
        if (ready) {
          return;
        }
        if (code === closeCodes.tooMany) {
          resolve({ refused: true });
        } else {
          reject(new Error(`a socket taking a slot closed with ${String(code)} before it was ready`));
        }
      },
    });
  });
}

interface PageSocket {
  readonly socket: PageWebSocket;
  admitted: boolean;
}

function watchPageSockets(page: Page): PageSocket[] {
  const opened: PageSocket[] = [];
  page.on('websocket', (socket) => {
    if (new URL(socket.url()).pathname !== realtimePath) {
      return;
    }
    const watched: PageSocket = { socket, admitted: false };
    opened.push(watched);
    socket.on('framereceived', ({ payload }) => {
      if (typeof payload === 'string' && parseServerFrame(payload)?.type === 'ready') {
        watched.admitted = true;
      }
    });
  });
  return opened;
}

export interface SocketSlotHolder {
  readonly keepPageSocketsOff: (account: TestAccount) => Promise<void>;
  readonly expectPageSocketsRefused: () => Promise<void>;
  readonly release: () => void;
}

export function socketSlotHolder(deployment: Deployment, page: Page): SocketSlotHolder {
  const held: RealtimeSocket[] = [];
  let pageSockets: PageSocket[] | undefined;
  return {
    keepPageSocketsOff: async (account): Promise<void> => {
      const sockets = nodeSocketFactory(deployment);
      const token = await accessToken(deployment, account);
      for (let attempt = 0; attempt < socketSlotSearchCeiling; attempt += 1) {
        const outcome = await slotAttempt(sockets, token);
        if ('refused' in outcome) {
          pageSockets = watchPageSockets(page);
          return;
        }
        held.push(outcome.held);
      }
      throw new Error(`the gateway admitted ${String(socketSlotSearchCeiling)} sockets of one account`);
    },
    expectPageSocketsRefused: async (): Promise<void> => {
      const watched = pageSockets;
      if (watched === undefined) {
        return;
      }
      await expect
        .poll(() => watched.length > 0 && watched.every((entry) => entry.socket.isClosed()), {
          message: `every page socket on ${realtimePath} is closed while the slots are held`,
          timeout: presenceDeadlineMilliseconds,
        })
        .toBe(true);
      expect(watched.filter((entry) => entry.admitted).map((entry) => entry.socket.url())).toEqual([]);
    },
    release: (): void => {
      for (const socket of held) {
        socket.close(closeCodes.client);
      }
      held.splice(0);
    },
  };
}
