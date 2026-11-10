import { emitBoundedDiagnostic } from './browser-diagnostics.ts';
import { isCancellationText } from './request-failures.ts';
import {
  type Page,
  type WebSocket as PageWebSocket,
  type Request,
  type Response,
  type Frame,
  type JSHandle,
  expect,
} from '@playwright/test';

import {
  NavigationReadiness,
  ownedApiPath,
  type ReadinessPhase,
  type ReadinessRequestId,
} from './navigation-readiness.ts';

import { authFrame, closeCodes, parseServerFrame } from '../src/lib/realtime/frames.ts';
import {
  type RealtimeSocket,
  type SocketFactory,
  type WebSocketLike,
  realtimeAddress,
  realtimePath,
  webSocketFactory,
} from '../src/lib/realtime/socket.ts';
import { type TestAccount, pageSettled, testNetAddress } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { accessToken, callApi } from './tutoring.ts';

export interface Presence {
  readonly online: boolean;
  readonly sockets: number;
}

const presenceDeadlineMilliseconds = 10_000;

class PresencePendingError extends Error {}

function isPendingPresenceError(value: unknown): boolean {
  return (
    typeof value === 'object' &&
    value !== null &&
    'error' in value &&
    typeof value.error === 'object' &&
    value.error !== null &&
    'code' in value.error &&
    value.error.code === 'service_unavailable'
  );
}

async function control(deployment: Deployment, path: string, body: unknown): Promise<unknown> {
  const response = await fetch(`${deployment.apiUrl}/__test__/${path}`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (path === 'presence' && response.status === 503) {
    const body: unknown = await response.json();
    if (isPendingPresenceError(body)) {
      throw new PresencePendingError('The local presence write is pending.');
    }
    throw new Error('The presence route returned an unexpected unavailable response.');
  }
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
    .poll(
      async () => {
        try {
          return (await presence(deployment, user)).sockets;
        } catch (error) {
          if (error instanceof PresencePendingError) {
            return 'presence-pending';
          }
          throw error;
        }
      },
      { timeout: presenceDeadlineMilliseconds },
    )
    .toBe(sockets);
}

export interface TrafficAfterReady {
  readonly readyReceived: () => boolean;
  readonly unsettled: () => string[];
  readonly snapshot: () => { readonly generation: number; readonly pending: number; readonly phase: ReadinessPhase };
  readonly capture: () => {
    readonly owner: number;
    readonly captured: number | null;
    readonly waits: number;
    readonly handles: number;
    readonly publications: readonly number[];
  };
  readonly initialize: () => Promise<void>;
  readonly prepareNavigation: () => Promise<void>;
  readonly coherent: () => boolean;
  readonly dispose: () => Promise<void>;
}

function documentAddress(address: string): string {
  const url = new URL(address);
  url.hash = '';
  return url.href;
}

export function trafficAfterReady(page: Page, queryPaths: readonly string[]): TrafficAfterReady {
  const state = new NavigationReadiness(queryPaths);
  const requests = new Map<Request, ReadinessRequestId>();
  const navigations = new WeakMap<Request, ReadinessRequestId>();
  const sockets = new Map<
    PageWebSocket,
    { readonly frame: (event: { readonly payload: string | Buffer }) => void; readonly close: () => void }
  >();
  const operations = new Set<Promise<void>>();
  let sequence = 0;
  let version = 0;
  let documentCheck = 0;
  let documentCaptured = false;
  let capturedOwner: number | null = null;
  let activeWaits = 0;
  let activeHandles = 0;
  const publications: number[] = [];
  let navigationPrepared = false;
  let navigation: Request | null = null;
  let document: Promise<JSHandle<{ readonly document: Document }> | null> = Promise.resolve(null);
  let disposed = false;
  let lifecycleFailed = false;
  let disposal: Promise<void> | null = null;
  const track = (operation: Promise<void>): void => {
    const observed = operation
      .catch(() => {
        lifecycleFailed = true;
        state.fail('observer_operation_failed');
      })
      .finally(() => {
        operations.delete(observed);
      });
    operations.add(observed);
  };
  const disposeHandle = async (handle: JSHandle<{ readonly document: Document }>): Promise<void> => {
    await handle.dispose();
    activeHandles -= 1;
  };
  const joinOperations = async (): Promise<void> => {
    while (operations.size > 0) await Promise.all([...operations]);
    if (lifecycleFailed) throw new Error('navigation_readiness_lifecycle_failed');
  };
  const singleFrame = (): boolean => {
    if (page.frames().length !== 1) state.fail('ambiguous_frame');
    return page.frames().length === 1;
  };
  const onFrameAttached = (): void => {
    state.fail('ambiguous_frame');
  };
  const releaseDocument = (): void => {
    documentCaptured = false;
    capturedOwner = null;
    const previous = document;
    document = Promise.resolve(null);
    track(
      previous.then(async (handle) => {
        if (handle !== null) await disposeHandle(handle);
      }),
    );
  };
  const captureDocument = (): void => {
    const capturedVersion = version;
    documentCaptured = false;
    capturedOwner = null;
    activeWaits += 1;
    document = page
      .waitForLoadState('domcontentloaded', { timeout: 5000 })
      .finally(() => {
        activeWaits -= 1;
      })
      .then(async () => {
        if (disposed || version !== capturedVersion) return null;
        const handle = await page.evaluateHandle(() => ({ document: window.document }));
        activeHandles += 1;
        return handle;
      })
      .then(
        async (handle) => {
          if (handle === null) return null;
          if (disposed || version !== capturedVersion) {
            await disposeHandle(handle);
            return null;
          }
          if (publications.length >= 128) {
            await disposeHandle(handle);
            state.fail('capture_history_overflow');
            return null;
          }
          publications.push(capturedVersion);
          capturedOwner = capturedVersion;
          documentCaptured = true;
          return handle;
        },
        () => {
          if (!disposed && version === capturedVersion) {
            lifecycleFailed = true;
            state.fail('capture_document_failed');
          }
          return null;
        },
      );
    track(document.then(() => undefined));
  };
  const releaseSockets = (): void => {
    for (const [socket, handlers] of sockets) {
      socket.off('framereceived', handlers.frame);
      socket.off('close', handlers.close);
    }
    sockets.clear();
  };
  const onRequest = (request: Request): void => {
    if (disposed) return;
    if (request.serviceWorker() !== null) return;
    let frame: Frame;
    try {
      frame = request.frame();
    } catch {
      lifecycleFailed = true;
      state.fail('request_frame_unavailable');
      return;
    }
    if (frame !== page.mainFrame()) return;
    sequence += 1;
    if (request.isNavigationRequest()) {
      const from = request.redirectedFrom();
      const redirectedFrom = from === null ? null : navigations.get(from);
      if (from !== null && redirectedFrom === undefined) return;
      if (!state.navigate(sequence, redirectedFrom ?? null)) return;
      navigations.set(request, sequence);
      navigation = request;
      navigationPrepared = false;
      if (from === null) {
        version += 1;
        requests.clear();
        releaseSockets();
        releaseDocument();
      }
      return;
    }
    const path = ownedApiPath(request.url(), page.url(), frame === page.mainFrame());
    if (path !== null && state.requestStarted(sequence, path, request.method() === 'GET'))
      requests.set(request, sequence);
  };
  const currentDocumentCheck = (capturedVersion: number, capturedCheck: number): boolean =>
    !disposed && version === capturedVersion && documentCheck === capturedCheck;
  const onFrame = (frame: Frame): void => {
    if (disposed || frame !== page.mainFrame()) return;
    documentCheck += 1;
    const committedCheck = documentCheck;
    const committedVersion = version;
    if (navigation !== null) {
      const id = navigations.get(navigation);
      if (id === undefined || documentAddress(navigation.url()) !== documentAddress(frame.url())) {
        state.fail();
        return;
      }
      state.commit(id);
      navigation = null;
      captureDocument();
      return;
    }
    state.checkDocument();
    const previous = document;
    track(
      previous.then(async (handle) => {
        if (handle === null || !currentDocumentCheck(committedVersion, committedCheck)) {
          if (currentDocumentCheck(committedVersion, committedCheck)) state.fail();
          return;
        }
        let same = false;
        try {
          same = await page.evaluate((observed) => observed.document === window.document, handle);
        } catch {
          if (currentDocumentCheck(committedVersion, committedCheck)) state.fail();
          return;
        }
        if (!currentDocumentCheck(committedVersion, committedCheck)) return;
        if (same) state.sameDocument();
        else state.fail();
      }),
    );
  };
  const onResponse = (response: Response): void => {
    if (disposed) return;
    const id = requests.get(response.request());
    if (id !== undefined) state.response(id, response.status());
  };
  const onFinished = (request: Request): void => {
    if (disposed) return;
    const id = requests.get(request);
    if (id === undefined) return;
    requests.delete(request);
    state.requestFinished(id);
  };
  const onFailed = (request: Request): void => {
    if (disposed) return;
    const nav = navigations.get(request);
    if (nav !== undefined) state.navigationFailed(nav);
    const id = requests.get(request);
    if (id === undefined) return;
    requests.delete(request);
    state.requestFailed(id, isCancellationText(request.failure()?.errorText ?? ''));
  };
  const onSocket = (socket: PageWebSocket): void => {
    if (disposed || !singleFrame() || socket.url() !== realtimeAddress(new URL(page.url()).origin)) return;
    sequence += 1;
    const id = sequence;
    if (!state.socketOpened(id)) return;
    const frame = ({ payload }: { readonly payload: string | Buffer }): void => {
      if (typeof payload === 'string' && parseServerFrame(payload)?.type === 'ready') state.socketReady(id);
    };
    const close = (): void => {
      socket.off('framereceived', frame);
      socket.off('close', close);
      sockets.delete(socket);
      state.socketClosed(id);
    };
    sockets.set(socket, { frame, close });
    socket.on('framereceived', frame);
    socket.on('close', close);
  };
  const detach = (): void => {
    if (disposed) return;
    disposed = true;
    version += 1;
    page.off('request', onRequest);
    page.off('response', onResponse);
    page.off('requestfinished', onFinished);
    page.off('requestfailed', onFailed);
    page.off('framenavigated', onFrame);
    page.off('frameattached', onFrameAttached);
    page.off('websocket', onSocket);
    page.off('close', detach);
    releaseSockets();
    requests.clear();
    navigation = null;
    releaseDocument();
    state.dispose();
  };
  page.on('request', onRequest);
  page.on('response', onResponse);
  page.on('requestfinished', onFinished);
  page.on('requestfailed', onFailed);
  page.on('framenavigated', onFrame);
  page.on('frameattached', onFrameAttached);
  page.on('websocket', onSocket);
  page.on('close', detach);
  singleFrame();
  captureDocument();
  const readyReceived = (): boolean =>
    singleFrame() &&
    !navigationPrepared &&
    documentCaptured &&
    capturedOwner === version &&
    !lifecycleFailed &&
    state.readyReceived();
  return {
    readyReceived,
    coherent: () => state.coherent(readyReceived()),
    capture: () => ({
      owner: version,
      captured: capturedOwner,
      waits: activeWaits,
      handles: activeHandles,
      publications: [...publications],
    }),
    initialize: async (): Promise<void> => {
      await joinOperations();
      if (disposed || !documentCaptured || capturedOwner !== version || !singleFrame())
        throw new Error('navigation_initial_capture_failed');
    },
    prepareNavigation: async (): Promise<void> => {
      if (disposed) throw new Error('navigation_observer_disposed');
      navigationPrepared = true;
      version += 1;
      releaseSockets();
      releaseDocument();
      await joinOperations();
    },
    unsettled: () => [...(lifecycleFailed ? ['observer_lifecycle_failed'] : []), ...state.unsettled()],
    snapshot: () => state.snapshot(),
    dispose: (): Promise<void> => {
      if (disposal !== null) return disposal;
      emitBoundedDiagnostic({ kind: 'readiness', ...state.evidence() });
      detach();
      disposal = joinOperations();
      return disposal;
    },
  };
}

export async function expectRefetchedAfterTheSocketIsReady(traffic: TrafficAfterReady): Promise<void> {
  try {
    await expect.poll(() => traffic.coherent()).toBe(true);
  } finally {
    await traffic.dispose();
  }
}

export const todayQueryPaths: readonly string[] = [
  '/api/v1/notifications',
  '/api/v1/tutor/code',
  '/api/v1/join-requests',
  '/api/v1/calendar',
  '/api/v1/students',
  '/api/v1/channels',
  '/api/v1/library',
];

export const unverifiedTutorTodayQueryPaths: readonly string[] = [
  '/api/v1/notifications',
  '/api/v1/tutor/code',
  '/api/v1/calendar',
  '/api/v1/channels',
  '/api/v1/library',
];

export async function expectSettledAfterTheSocketIsReady(page: Page, traffic: TrafficAfterReady): Promise<void> {
  await expectRefetchedAfterTheSocketIsReady(traffic);
  await pageSettled(page);
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

export async function expectReadyForNavigation(page: Page, traffic: TrafficAfterReady): Promise<void> {
  await expectSettledAfterTheSocketIsReady(page, traffic);
  await page.evaluate(async () => {
    await document.fonts.ready;
  });
  await expect.poll(() => page.evaluate(() => document.fonts.status)).toBe('loaded');
  expect(
    await page.evaluate(
      () => [...document.fonts].filter((font) => font.status === 'error' || font.status === 'loading').length,
    ),
  ).toBe(0);
}
