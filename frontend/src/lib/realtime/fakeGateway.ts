import type { FakeAuthServer } from '../api/fakeTransport.ts';
import { type EntityKind, closeCodes, refPattern } from './frames.ts';
import type { RealtimeSocket, SocketEvents, SocketFactory } from './socket.ts';

const frameLimitBytes = 16 * 1024;

interface FakeConnection {
  readonly events: SocketEvents;
  account: string | null;
  session: number | null;
  answersPings: boolean;
  closed: boolean;
  readonly tokens: string[];
  readonly offered: string[];
}

type ClientFrame =
  | { readonly kind: 'auth'; readonly token: string }
  | { readonly kind: 'ping'; readonly ref: string | null }
  | { readonly kind: 'typing'; readonly ref: string | null }
  | { readonly kind: 'refused'; readonly ref: string | null };

function isFields(value: unknown): value is Readonly<Record<string, unknown>> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function parsedJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

function clientFrame(text: string): ClientFrame {
  const parsed = parsedJson(text);
  if (!isFields(parsed) || !Object.keys(parsed).every((key) => ['type', 'ref', 'payload'].includes(key))) {
    return { kind: 'refused', ref: null };
  }
  const ref = parsed.ref;
  if (ref !== undefined && (typeof ref !== 'string' || !refPattern.test(ref))) {
    return { kind: 'refused', ref: null };
  }
  const echoed = typeof ref === 'string' ? ref : null;
  const payload = parsed.payload;
  if (parsed.type === 'ping' && payload === undefined) {
    return { kind: 'ping', ref: echoed };
  }
  if (
    (parsed.type === 'typing.start' || parsed.type === 'typing.stop') &&
    isFields(payload) &&
    Object.keys(payload).every((key) => key === 'channel_id') &&
    typeof payload.channel_id === 'string'
  ) {
    return { kind: 'typing', ref: echoed };
  }
  if (
    parsed.type === 'auth' &&
    isFields(payload) &&
    Object.keys(payload).every((key) => key === 'access_token') &&
    typeof payload.access_token === 'string'
  ) {
    return { kind: 'auth', token: payload.access_token };
  }
  return { kind: 'refused', ref: echoed };
}

function later(task: () => void): void {
  void Promise.resolve().then(task);
}

export class FakeGateway {
  private readonly server: FakeAuthServer;
  private readonly connections: FakeConnection[] = [];
  private frameIds = 0;
  private readonly refusals: number[] = [];

  constructor(server: FakeAuthServer) {
    this.server = server;
  }

  get openSockets(): number {
    return this.connections.filter((connection) => !connection.closed).length;
  }

  get socketsOpened(): number {
    return this.connections.length;
  }

  readySockets(accountId: string): number {
    return this.connections.filter((connection) => !connection.closed && connection.account === accountId).length;
  }

  tokensFrom(accountId: string): readonly string[] {
    return this.connections
      .filter((connection) => connection.account === accountId)
      .flatMap((connection) => connection.tokens);
  }

  refuseConnections(count: number, code: number = closeCodes.tryLater): void {
    for (let refused = 0; refused < count; refused += 1) {
      this.refusals.push(code);
    }
  }

  tokensOfferedOnSocketsOf(accountId: string): readonly string[] {
    return this.connections
      .filter((connection) => connection.account === accountId)
      .flatMap((connection) => connection.offered);
  }

  readonly sockets: SocketFactory = (events) => {
    const connection: FakeConnection = {
      events,
      account: null,
      session: null,
      answersPings: true,
      closed: false,
      tokens: [],
      offered: [],
    };
    this.connections.push(connection);
    const refusal = this.refusals.shift() ?? null;
    later(() => {
      if (connection.closed) {
        return;
      }
      if (refusal !== null) {
        this.close(connection, refusal);
        return;
      }
      events.onOpen();
    });
    const socket: RealtimeSocket = {
      send: (frame): void => {
        later(() => {
          this.receive(connection, frame);
        });
      },
      sendBinary: (): void => {
        later(() => {
          this.close(connection, closeCodes.binaryFrame);
        });
      },
      close: (): void => {
        connection.closed = true;
      },
    };
    return socket;
  };

  closeAccount(accountId: string, code: number): number {
    const affected = this.connections.filter((connection) => !connection.closed && connection.account === accountId);
    for (const connection of affected) {
      this.close(connection, code);
    }
    return affected.length;
  }

  recheck(): void {
    for (const connection of this.connections) {
      if (!connection.closed && connection.session !== null && !this.server.sessionIsLive(connection.session)) {
        this.close(connection, closeCodes.unauthenticated);
      }
    }
  }

  notify(accountId: string, id: string): void {
    this.deliver(accountId, { type: 'notification.new', payload: { id } });
  }

  changed(accountId: string, kind: EntityKind, id: string): void {
    this.deliver(accountId, { type: 'entity.changed', payload: { kind, id } });
  }

  messageExchanged(accountId: string, channelId: string, messageId: string, seq: number): void {
    this.deliver(accountId, { type: 'message.new', payload: { channel_id: channelId, message_id: messageId, seq } });
    this.deliver(accountId, { type: 'message.updated', payload: { channel_id: channelId, message_id: messageId } });
    this.deliver(accountId, { type: 'message.deleted', payload: { channel_id: channelId, message_id: messageId } });
  }

  channelRead(accountId: string, channelId: string): void {
    this.deliver(accountId, { type: 'read.updated', payload: { channel_id: channelId } });
  }

  memberTyped(accountId: string, memberId: string, channelId: string): void {
    this.deliver(accountId, { type: 'presence', payload: { user_id: memberId, online: true } });
    this.deliver(accountId, { type: 'typing', payload: { channel_id: channelId, user_id: memberId } });
  }

  stopAnsweringPings(accountId: string): void {
    for (const connection of this.connections) {
      if (connection.account === accountId) {
        connection.answersPings = false;
      }
    }
  }

  private deliver(accountId: string, frame: unknown): void {
    for (const connection of this.connections) {
      if (!connection.closed && connection.account === accountId) {
        this.send(connection, frame);
      }
    }
  }

  private receive(connection: FakeConnection, text: string): void {
    if (connection.closed) {
      return;
    }
    if (new TextEncoder().encode(text).length > frameLimitBytes) {
      this.close(connection, closeCodes.frameTooLarge);
      return;
    }
    const frame = clientFrame(text);
    if (frame.kind === 'auth') {
      connection.offered.push(frame.token);
    }
    if (connection.session === null) {
      this.authenticate(connection, frame);
      return;
    }
    if (!this.server.sessionIsLive(connection.session)) {
      this.close(connection, closeCodes.unauthenticated);
      return;
    }
    switch (frame.kind) {
      case 'auth':
        this.authenticate(connection, frame);
        return;
      case 'ping':
        if (connection.answersPings) {
          this.send(connection, frame.ref === null ? { type: 'pong' } : { type: 'pong', ref: frame.ref });
        }
        return;
      case 'typing':
        this.refuse(connection, 'not_found', 'The requested resource does not exist.', frame.ref);
        return;
      case 'refused':
        this.refuse(connection, 'bad_request', 'The request is malformed.', frame.ref);
    }
  }

  private refuse(connection: FakeConnection, code: string, message: string, ref: string | null): void {
    this.frameIds += 1;
    this.send(connection, {
      type: 'error',
      payload: { code, message, request_id: `fake-frame-${String(this.frameIds)}`, ref },
    });
  }

  private authenticate(connection: FakeConnection, frame: ClientFrame): void {
    const live = frame.kind === 'auth' ? this.server.liveSessionOf(frame.token) : null;
    if (
      live === null ||
      (connection.session !== null && (live.sessionId !== connection.session || live.accountId !== connection.account))
    ) {
      this.close(connection, closeCodes.unauthenticated);
      return;
    }
    connection.account = live.accountId;
    connection.session = live.sessionId;
    if (frame.kind === 'auth') {
      connection.tokens.push(frame.token);
    }
    this.send(connection, { type: 'ready', payload: {} });
  }

  private send(connection: FakeConnection, frame: unknown): void {
    const text = JSON.stringify(frame);
    later(() => {
      if (!connection.closed) {
        connection.events.onText(text);
      }
    });
  }

  private close(connection: FakeConnection, code: number): void {
    if (connection.closed) {
      return;
    }
    connection.closed = true;
    later(() => {
      connection.events.onClose(code);
    });
  }
}
