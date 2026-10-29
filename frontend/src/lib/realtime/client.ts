import type { RefreshResult, Scheduler, SessionState } from '../api/session.ts';
import { type EntityKind, type ServerFrame, authFrame, closeCodes, parseServerFrame, pingFrame } from './frames.ts';
import type { RealtimeSocket, SocketFactory } from './socket.ts';

export const tokenMarginMilliseconds = 30_000;
export const pingEveryMilliseconds = 30_000;
export const pongWithinMilliseconds = 10_000;
export const firstBackoffMilliseconds = 1_000;
export const backoffCapMilliseconds = 30_000;
export const tooManyBackoffCapMilliseconds = 60_000;
export const listenerReconnectMilliseconds = 1_000;

export type RealtimeEvent =
  | { readonly kind: 'notification'; readonly id: string }
  | { readonly kind: 'entity'; readonly entity: EntityKind; readonly id: string }
  | { readonly kind: 'connected'; readonly reconnect: boolean };

export interface TokenSource {
  readonly freshToken: (minimumValidityMilliseconds: number, epoch: number) => Promise<string | null>;
  readonly subscribeTokens: (listener: (token: string, epoch: number) => void) => () => void;
  readonly refreshAfterSocketRefusal: () => Promise<RefreshResult>;
  readonly currentEpoch: () => number;
}

export interface ClientDependencies {
  readonly sockets: SocketFactory;
  readonly tokens: TokenSource;
  readonly epoch: number;
  readonly scheduler: Scheduler;
  readonly random: () => number;
  readonly onEvent: (event: RealtimeEvent) => void;
  readonly onFailure: (error: Error) => void;
  readonly connectedBefore: boolean;
}

interface Connection {
  readonly socket: RealtimeSocket;
  sentToken: string;
  ready: boolean;
}

export class RealtimeClient {
  private readonly deps: ClientDependencies;
  private stopped = false;
  private started = false;
  private connection: Connection | null = null;
  private generation = 0;
  private attempt = 0;
  private connectedBefore: boolean;
  private latestToken: string | null = null;
  private pings = 0;
  private awaitedPong: string | null = null;
  private cancelReconnect: (() => void) | null = null;
  private cancelPing: (() => void) | null = null;
  private cancelPong: (() => void) | null = null;
  private unsubscribeTokens: (() => void) | null = null;

  constructor(dependencies: ClientDependencies) {
    this.deps = dependencies;
    this.connectedBefore = dependencies.connectedBefore;
  }

  start(): void {
    if (this.started) {
      throw new Error('a realtime client starts once');
    }
    this.started = true;
    this.unsubscribeTokens = this.deps.tokens.subscribeTokens((token, epoch) => {
      if (epoch === this.deps.epoch) {
        this.renewed(token);
      }
    });
    void this.connect();
  }

  stop(): void {
    this.stopped = true;
    this.unsubscribeTokens?.();
    this.unsubscribeTokens = null;
    this.cancelReconnect?.();
    this.cancelReconnect = null;
    this.drop();
  }

  private async connect(): Promise<void> {
    this.cancelReconnect = null;
    const token = await this.deps.tokens.freshToken(tokenMarginMilliseconds, this.deps.epoch).then(
      (value): string | null | undefined => value,
      (): undefined => undefined,
    );
    if (this.stopped || token === null) {
      return;
    }
    if (token === undefined) {
      this.reconnectLater(backoffCapMilliseconds);
      return;
    }
    this.latestToken = token;
    this.generation += 1;
    const generation = this.generation;
    const current = (): boolean => !this.stopped && generation === this.generation;
    const socket = this.deps.sockets({
      onOpen: () => {
        if (current() && this.connection !== null) {
          this.connection.sentToken = this.latestToken ?? token;
          this.connection.socket.send(authFrame(this.connection.sentToken));
        }
      },
      onText: (text) => {
        if (current()) {
          this.receive(text);
        }
      },
      onBinary: () => {
        if (current()) {
          this.fail('the server sent a binary frame');
        }
      },
      onClose: (code) => {
        if (current()) {
          void this.closed(code);
        }
      },
    });
    this.connection = { socket, sentToken: token, ready: false };
  }

  private receive(text: string): void {
    const frame = parseServerFrame(text);
    if (frame === null) {
      this.fail('the server sent a frame the client cannot parse');
      return;
    }
    this.handle(frame);
  }

  private handle(frame: ServerFrame): void {
    switch (frame.type) {
      case 'ready':
        this.authenticated();
        return;
      case 'pong':
        if (frame.ref !== null && frame.ref === this.awaitedPong) {
          this.attempt = 0;
          this.awaitedPong = null;
          this.cancelPong?.();
          this.cancelPong = null;
        }
        return;
      case 'error':
        this.deps.onFailure(new Error(`the realtime server refused a frame with ${frame.code}`));
        return;
      case 'notification.new':
        this.deps.onEvent({ kind: 'notification', id: frame.id });
        return;
      case 'entity.changed':
        this.deps.onEvent({ kind: 'entity', entity: frame.kind, id: frame.id });
        return;
      case 'message.new':
      case 'message.updated':
      case 'message.deleted':
      case 'read.updated':
        return;
    }
  }

  private authenticated(): void {
    const connection = this.connection;
    if (connection === null || connection.ready) {
      return;
    }
    connection.ready = true;
    this.schedulePing();
    if (this.latestToken !== null && this.latestToken !== connection.sentToken) {
      this.renewed(this.latestToken);
    }
    this.deps.onEvent({ kind: 'connected', reconnect: this.connectedBefore || this.attempt > 0 });
    this.connectedBefore = true;
  }

  private renewed(token: string): void {
    this.latestToken = token;
    const connection = this.connection;
    if (connection?.ready === true && connection.sentToken !== token) {
      connection.sentToken = token;
      connection.socket.send(authFrame(token));
    }
  }

  private schedulePing(): void {
    this.cancelPing = this.deps.scheduler.schedule(pingEveryMilliseconds, () => {
      this.cancelPing = null;
      this.ping();
    });
  }

  private ping(): void {
    const connection = this.connection;
    if (connection === null) {
      return;
    }
    this.pings += 1;
    const ref = `ping-${String(this.pings)}`;
    this.awaitedPong = ref;
    connection.socket.send(pingFrame(ref));
    this.cancelPong = this.deps.scheduler.schedule(pongWithinMilliseconds, () => {
      this.cancelPong = null;
      if (this.awaitedPong === ref) {
        this.drop();
        this.reconnectLater(backoffCapMilliseconds);
      }
    });
    this.schedulePing();
  }

  private async closed(code: number): Promise<void> {
    this.forget();
    if (code === closeCodes.unauthenticated) {
      if (this.deps.tokens.currentEpoch() !== this.deps.epoch) {
        return;
      }
      const result = await this.deps.tokens.refreshAfterSocketRefusal().then(
        (value): RefreshResult | null => value,
        (): null => null,
      );
      if (!this.stopped && (result === null || result.kind === 'renewed')) {
        this.reconnectLater(backoffCapMilliseconds);
      }
      return;
    }
    if (code === closeCodes.reconnect) {
      this.reconnectLater(backoffCapMilliseconds, listenerReconnectMilliseconds);
      return;
    }
    this.reconnectLater(code === closeCodes.tooMany ? tooManyBackoffCapMilliseconds : backoffCapMilliseconds);
  }

  private fail(message: string): void {
    this.deps.onFailure(new Error(message));
    this.drop();
    this.reconnectLater(backoffCapMilliseconds);
  }

  private reconnectLater(cap: number, floor = 0): void {
    if (this.stopped) {
      return;
    }
    const ceiling = Math.min(cap, firstBackoffMilliseconds * 2 ** this.attempt);
    this.attempt += 1;
    this.cancelReconnect = this.deps.scheduler.schedule(floor + this.deps.random() * ceiling, () => {
      void this.connect();
    });
  }

  private drop(): void {
    const connection = this.connection;
    this.forget();
    connection?.socket.close(closeCodes.client);
  }

  private forget(): void {
    this.connection = null;
    this.generation += 1;
    this.awaitedPong = null;
    this.cancelPing?.();
    this.cancelPing = null;
    this.cancelPong?.();
    this.cancelPong = null;
  }
}

export interface SessionOwner {
  readonly subscribe: (listener: () => void) => () => void;
  readonly snapshot: () => SessionState;
  readonly currentEpoch: () => number;
}

export function connectWhileSignedIn(session: SessionOwner, create: (epoch: number) => RealtimeClient): () => void {
  let owned: { readonly epoch: number; readonly client: RealtimeClient } | null = null;
  const follow = (): void => {
    const signedIn = session.snapshot().kind === 'signedIn';
    const epoch = session.currentEpoch();
    if (owned !== null && (!signedIn || owned.epoch !== epoch)) {
      owned.client.stop();
      owned = null;
    }
    if (signedIn && owned === null) {
      owned = { epoch, client: create(epoch) };
      owned.client.start();
    }
  };
  const unsubscribe = session.subscribe(follow);
  follow();
  return (): void => {
    unsubscribe();
    owned?.client.stop();
    owned = null;
  };
}
