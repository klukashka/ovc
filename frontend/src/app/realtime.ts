import type { QueryClient } from '@tanstack/react-query';

import { type Session, browserScheduler } from '../lib/api/session.ts';
import { RealtimeClient, connectWhileSignedIn } from '../lib/realtime/client.ts';
import { browserSocketFactory } from '../lib/realtime/socket.ts';
import { liveRefetch } from './liveRefetch.ts';

export function startRealtime(session: Session, queryClient: QueryClient, origin: string): void {
  const sockets = browserSocketFactory(origin);
  const onEvent = liveRefetch(queryClient);
  const connect = (): (() => void) =>
    connectWhileSignedIn(
      session,
      (epoch) =>
        new RealtimeClient({
          sockets,
          tokens: session,
          epoch,
          scheduler: browserScheduler,
          random: Math.random,
          onEvent,
          onFailure: reportError,
        }),
    );
  let disconnect = (): void => undefined;
  const start = (): void => {
    disconnect = connect();
  };
  if (document.readyState === 'complete') {
    start();
  } else {
    window.addEventListener('load', start, { once: true });
  }
  const leave = (): void => {
    window.removeEventListener('load', start);
    disconnect();
  };
  window.addEventListener('beforeunload', leave);
  window.addEventListener('pagehide', leave);
  window.addEventListener('pageshow', (event) => {
    if (event.persisted) {
      start();
    }
  });
}
