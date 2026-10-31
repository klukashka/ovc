import { type ServerFrame, authFrame, closeCodes, parseServerFrame, pingFrame } from './frames.ts';
import type { RealtimeSocket, SocketFactory } from './socket.ts';

export interface RealtimeContractSubject {
  readonly sockets: SocketFactory;
  readonly token: () => Promise<string>;
  readonly otherAccountToken: () => Promise<string>;
  readonly revoke: (token: string) => Promise<void>;
  readonly closeSockets: (code: typeof closeCodes.tooMany | typeof closeCodes.reconnect) => Promise<void>;
  readonly changeRelationship: () => Promise<void>;
  readonly changeChannel: () => Promise<void>;
  readonly exchangeMessage: () => Promise<void>;
  readonly readChannel: () => Promise<void>;
  readonly memberConnectsAndTypes: () => Promise<() => void>;
  readonly notify: () => Promise<void>;
  readonly readNotifications: () => Promise<void>;
}

export type RealtimeContractScenario = (subject: RealtimeContractSubject) => Promise<void>;

type Observed =
  | { readonly kind: 'frame'; readonly frame: ServerFrame }
  | { readonly kind: 'closed'; readonly code: number }
  | { readonly kind: 'refused'; readonly text: string };

class ContractViolation extends Error {
  override readonly name = 'ContractViolation';
}

function check(condition: boolean, message: string): void {
  if (!condition) {
    throw new ContractViolation(message);
  }
}

const deadlineMilliseconds = 10_000;

class ContractConnection {
  private readonly observed: Observed[] = [];
  private readonly waiting: (() => void)[] = [];
  private readonly opened: Promise<void>;
  readonly socket: RealtimeSocket;

  constructor(sockets: SocketFactory) {
    let markOpen = (): void => undefined;
    this.opened = new Promise((resolve) => {
      markOpen = resolve;
    });
    this.socket = sockets({
      onOpen: () => {
        markOpen();
      },
      onText: (text) => {
        const frame = parseServerFrame(text);
        this.push(frame === null ? { kind: 'refused', text } : { kind: 'frame', frame });
      },
      onBinary: () => {
        this.push({ kind: 'refused', text: 'a binary frame' });
      },
      onClose: (code) => {
        this.push({ kind: 'closed', code });
      },
    });
  }

  async open(): Promise<void> {
    await this.within(this.opened, 'the socket did not open');
  }

  async next(): Promise<Observed> {
    if (this.observed.length === 0) {
      await this.within(
        new Promise<void>((resolve) => {
          this.waiting.push(resolve);
        }),
        'nothing arrived before the deadline',
      );
    }
    const first = this.observed.shift();
    if (first === undefined) {
      throw new ContractViolation('nothing arrived');
    }
    if (first.kind === 'refused') {
      throw new ContractViolation(`the client's parser refused a server frame: ${first.text}`);
    }
    return first;
  }

  async frame(): Promise<ServerFrame> {
    const observed = await this.next();
    if (observed.kind !== 'frame') {
      throw new ContractViolation(`expected a frame, the socket closed with ${closedCode(observed)}`);
    }
    return observed.frame;
  }

  async frameOfType(type: ServerFrame['type']): Promise<ServerFrame> {
    for (;;) {
      const frame = await this.frame();
      if (frame.type === type) {
        return frame;
      }
    }
  }

  async framesOfTypes(types: readonly ServerFrame['type'][]): Promise<ReadonlyMap<ServerFrame['type'], ServerFrame>> {
    const seen = new Map<ServerFrame['type'], ServerFrame>();
    while (types.some((type) => !seen.has(type))) {
      const frame = await this.frame();
      if (types.includes(frame.type) && !seen.has(frame.type)) {
        seen.set(frame.type, frame);
      }
    }
    return seen;
  }

  async closedWith(code: number): Promise<void> {
    for (;;) {
      const observed = await this.next();
      if (observed.kind === 'closed') {
        check(observed.code === code, `expected the close code ${String(code)}, got ${String(observed.code)}`);
        return;
      }
    }
  }

  async authenticated(token: string): Promise<void> {
    await this.open();
    this.socket.send(authFrame(token));
    check((await this.frame()).type === 'ready', 'expected ready after auth');
  }

  close(): void {
    this.socket.close(closeCodes.client);
  }

  private push(observed: Observed): void {
    this.observed.push(observed);
    for (const resume of this.waiting.splice(0)) {
      resume();
    }
  }

  private async within(promise: Promise<void>, message: string): Promise<void> {
    let cancel = (): void => undefined;
    const timeout = new Promise<void>((_resolve, reject) => {
      const handle = setTimeout(() => {
        reject(new ContractViolation(message));
      }, deadlineMilliseconds);
      cancel = (): void => {
        clearTimeout(handle);
      };
    });
    try {
      await Promise.race([promise, timeout]);
    } finally {
      cancel();
    }
  }
}

function closedCode(observed: Observed): string {
  return observed.kind === 'closed' ? String(observed.code) : 'no close';
}

async function withConnection(
  subject: RealtimeContractSubject,
  run: (connection: ContractConnection) => Promise<void>,
): Promise<void> {
  const connection = new ContractConnection(subject.sockets);
  try {
    await run(connection);
  } finally {
    connection.close();
  }
}

export const realtimeContract: Readonly<Record<string, RealtimeContractScenario>> = {
  'auth is answered with ready, and an in-band auth of the same session with ready again': (subject) =>
    withConnection(subject, async (connection) => {
      const token = await subject.token();
      await connection.authenticated(token);

      connection.socket.send(authFrame(token));

      check((await connection.frame()).type === 'ready', 'expected ready after an in-band auth');
    }),
  'ping is answered with pong echoing its ref, or with no ref': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      connection.socket.send(pingFrame('ref-1'));
      const echoed = await connection.frame();
      connection.socket.send(JSON.stringify({ type: 'ping' }));
      const bare = await connection.frame();

      check(echoed.type === 'pong' && echoed.ref === 'ref-1', 'expected pong with ref-1');
      check(bare.type === 'pong' && bare.ref === null, 'expected pong without a ref');
    }),
  'an unknown frame is answered with an error frame and the socket stays open': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      connection.socket.send(JSON.stringify({ type: 'message.send', ref: 'ref-2' }));
      const refused = await connection.frame();
      connection.socket.send(pingFrame('ref-3'));
      const after = await connection.frame();

      check(
        refused.type === 'error' && refused.code === 'bad_request' && refused.ref === 'ref-2',
        'expected a bad_request error frame echoing ref-2',
      );
      check(after.type === 'pong' && after.ref === 'ref-3', 'expected the socket to stay open');
    }),
  'an unknown frame without a ref is answered with an error frame whose ref is null': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      connection.socket.send(JSON.stringify({ type: 'message.read' }));

      const refused = await connection.frame();
      check(refused.type === 'error' && refused.ref === null, 'expected an error frame with a null ref');
    }),
  'typing into a channel outside the account is answered with not_found and the socket stays open': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      connection.socket.send(
        JSON.stringify({ type: 'typing.start', ref: 'ref-5', payload: { channel_id: crypto.randomUUID() } }),
      );
      const refused = await connection.frameOfType('error');
      connection.socket.send(pingFrame('ref-6'));
      const after = await connection.frameOfType('pong');

      check(
        refused.type === 'error' && refused.code === 'not_found' && refused.ref === 'ref-5',
        'expected a not_found error frame echoing ref-5',
      );
      check(after.type === 'pong' && after.ref === 'ref-6', 'expected the socket to stay open');
    }),
  'another member connecting arrives as presence, and their typing as typing': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      const leave = await subject.memberConnectsAndTypes();
      try {
        const seen = await connection.framesOfTypes(['presence', 'typing']);
        const presence = seen.get('presence');
        const typing = seen.get('typing');
        check(
          presence?.type === 'presence' && presence.online && presence.userId !== '',
          'expected the member to come online',
        );
        check(
          typing?.type === 'typing' &&
            presence?.type === 'presence' &&
            typing.userId === presence.userId &&
            typing.channelId !== '',
          'expected the member typing in a shared channel',
        );
      } finally {
        leave();
      }
    }),
  'a frame before auth closes with 4401': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.open();

      connection.socket.send(pingFrame('ref-4'));

      await connection.closedWith(closeCodes.unauthenticated);
    }),
  'an invalid token closes with 4401': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.open();

      connection.socket.send(authFrame('not-a-token'));

      await connection.closedWith(closeCodes.unauthenticated);
    }),
  'an in-band auth of another account closes with 4401': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      connection.socket.send(authFrame(await subject.otherAccountToken()));

      await connection.closedWith(closeCodes.unauthenticated);
    }),
  'revoking the session closes with 4401': (subject) =>
    withConnection(subject, async (connection) => {
      const token = await subject.token();
      await connection.authenticated(token);

      await subject.revoke(token);

      await connection.closedWith(closeCodes.unauthenticated);
    }),
  'a binary frame closes with 1003': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      connection.socket.sendBinary(new Uint8Array([1, 2, 3]));

      await connection.closedWith(closeCodes.binaryFrame);
    }),
  'a frame over 16 KiB closes with 1009': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      connection.socket.send(JSON.stringify({ type: 'ping', ref: 'r', padding: 'x'.repeat(17 * 1024) }));

      await connection.closedWith(closeCodes.frameTooLarge);
    }),
  'the server closes with 4429 on request': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      await subject.closeSockets(closeCodes.tooMany);

      await connection.closedWith(closeCodes.tooMany);
    }),
  'the server closes with 1012 on request': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      await subject.closeSockets(closeCodes.reconnect);

      await connection.closedWith(closeCodes.reconnect);
    }),
  'a changed relationship arrives as entity.changed': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      await subject.changeRelationship();

      const frame = await connection.frameOfType('entity.changed');
      check(frame.type === 'entity.changed' && frame.kind === 'relationship', 'expected a relationship change');
    }),
  'a changed channel arrives as entity.changed of kind channel': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      await subject.changeChannel();

      const frame = await connection.frameOfType('entity.changed');
      check(frame.type === 'entity.changed' && frame.kind === 'channel', 'expected a channel change');
    }),
  'a message, its edit and its deletion arrive as message.new, message.updated and message.deleted': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      await subject.exchangeMessage();

      const created = await connection.frameOfType('message.new');
      const updated = await connection.frameOfType('message.updated');
      const deleted = await connection.frameOfType('message.deleted');
      check(created.type === 'message.new' && created.seq > 0, 'expected a new message with its sequence');
      check(
        created.type === 'message.new' &&
          updated.type === 'message.updated' &&
          deleted.type === 'message.deleted' &&
          created.messageId === updated.messageId &&
          updated.messageId === deleted.messageId &&
          created.channelId === deleted.channelId,
        'expected the same message to be updated and then deleted',
      );
    }),
  'reading a channel arrives as read.updated': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      await subject.readChannel();

      const frame = await connection.frameOfType('read.updated');
      check(frame.type === 'read.updated' && frame.channelId !== '', 'expected the read channel');
    }),
  'an inserted notification arrives as notification.new': (subject) =>
    withConnection(subject, async (connection) => {
      await connection.authenticated(await subject.token());

      await subject.notify();

      const frame = await connection.frameOfType('notification.new');
      check(frame.type === 'notification.new' && frame.id !== '', 'expected a notification identifier');
    }),
  'marking notifications read arrives as entity.changed of kind notification': (subject) =>
    withConnection(subject, async (connection) => {
      await subject.notify();
      await connection.authenticated(await subject.token());

      await subject.readNotifications();

      const frame = await connection.frameOfType('entity.changed');
      check(frame.type === 'entity.changed' && frame.kind === 'notification', 'expected a notification change');
    }),
};
