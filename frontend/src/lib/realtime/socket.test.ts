import { expect, test } from 'vitest';

import { closeCodes } from './frames.ts';
import { type WebSocketLike, webSocketFactory } from './socket.ts';

const connecting = 0;
const open = 1;
const clientClose = closeCodes.client;

class RecordingSocket implements WebSocketLike {
  binaryType: BinaryType = 'blob';
  readyState: number;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  readonly closes: (number | undefined)[] = [];

  constructor(readyState: number) {
    this.readyState = readyState;
  }

  send(): void {
    throw new Error('this socket sends nothing');
  }

  close(code?: number): void {
    this.closes.push(code);
  }

  opened(): void {
    this.readyState = open;
    this.onopen?.(new Event('open'));
  }
}

function factoryOver(socket: RecordingSocket): {
  readonly close: (code: number) => void;
  readonly opens: () => number;
} {
  let opens = 0;
  const created = webSocketFactory(() => socket)({
    onOpen: () => {
      opens += 1;
    },
    onText: () => undefined,
    onBinary: () => undefined,
    onClose: () => undefined,
  });
  return { close: created.close, opens: () => opens };
}

test('closing an open socket closes it at once', () => {
  const socket = new RecordingSocket(open);
  const created = factoryOver(socket);

  created.close(clientClose);

  expect(socket.closes).toEqual([clientClose]);
});

test('closing a connecting socket waits for it to open and never reports the opening', () => {
  const socket = new RecordingSocket(connecting);
  const created = factoryOver(socket);

  created.close(clientClose);
  const closesWhileConnecting = [...socket.closes];
  socket.opened();

  expect(closesWhileConnecting).toEqual([]);
  expect(socket.closes).toEqual([clientClose]);
  expect(created.opens()).toBe(0);
});

test('closing a connecting socket drops its message and close handlers', () => {
  const socket = new RecordingSocket(connecting);
  const created = factoryOver(socket);

  created.close(clientClose);

  expect(socket.closes).toEqual([]);
  expect(socket.onclose).toBeNull();
  expect(socket.onmessage).toBeNull();
});
