export interface SocketEvents {
  readonly onOpen: () => void;
  readonly onText: (text: string) => void;
  readonly onBinary: () => void;
  readonly onClose: (code: number) => void;
}

export interface RealtimeSocket {
  readonly send: (frame: string) => void;
  readonly sendBinary: (bytes: Uint8Array<ArrayBuffer>) => void;
  readonly close: (code: number) => void;
}

export type SocketFactory = (events: SocketEvents) => RealtimeSocket;

export const realtimePath = '/api/v1/ws';

export function realtimeAddress(origin: string): string {
  const url = new URL(realtimePath, origin);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString();
}

export interface WebSocketLike {
  binaryType: BinaryType;
  onopen: ((event: Event) => void) | null;
  onmessage: ((event: MessageEvent) => void) | null;
  onclose: ((event: CloseEvent) => void) | null;
  send(data: string | Uint8Array<ArrayBuffer>): void;
  close(code?: number): void;
}

export function webSocketFactory(open: () => WebSocketLike): SocketFactory {
  return (events) => {
    const socket = open();
    socket.binaryType = 'arraybuffer';
    socket.onopen = (): void => {
      events.onOpen();
    };
    socket.onmessage = (event): void => {
      const data: unknown = event.data;
      if (typeof data === 'string') {
        events.onText(data);
      } else {
        events.onBinary();
      }
    };
    socket.onclose = (event): void => {
      events.onClose(event.code);
    };
    return {
      send: (frame): void => {
        socket.send(frame);
      },
      sendBinary: (bytes): void => {
        socket.send(bytes);
      },
      close: (code): void => {
        socket.onopen = null;
        socket.onmessage = null;
        socket.onclose = null;
        socket.close(code);
      },
    };
  };
}

export function browserSocketFactory(origin: string): SocketFactory {
  const address = realtimeAddress(origin);
  return webSocketFactory(() => new WebSocket(address));
}
