import type { QueryClient } from '@tanstack/react-query';

import { type Scheduler, type Session, browserScheduler } from '../lib/api/session.ts';
import { RealtimeClient, connectWhileSignedIn } from '../lib/realtime/client.ts';
import { type SocketFactory, browserSocketFactory } from '../lib/realtime/socket.ts';
import { liveRefetch } from './liveRefetch.ts';
import { notificationArrivals } from './notificationArrivals.ts';

export const resumeAfterBeforeUnloadMilliseconds = 2_000;

export interface RealtimePage {
  readonly events: EventTarget;
  readonly loaded: () => boolean;
}

export interface RealtimeWiring {
  readonly session: Session;
  readonly queryClient: QueryClient;
  readonly sockets: SocketFactory;
  readonly scheduler: Scheduler;
  readonly page: RealtimePage;
  readonly onFailure: (error: Error) => void;
}

export function wireRealtime({ session, queryClient, sockets, scheduler, page, onFailure }: RealtimeWiring): void {
  const onEvent = liveRefetch(queryClient, notificationArrivals);
  const connect = (connectedBefore: boolean): (() => void) =>
    connectWhileSignedIn(
      session,
      (epoch) =>
        new RealtimeClient({
          sockets,
          tokens: session,
          epoch,
          scheduler,
          random: Math.random,
          onEvent,
          onFailure,
          connectedBefore,
        }),
    );
  let disconnect = (): void => undefined;
  let cancelResume = (): void => undefined;
  let started = false;
  const start = (): void => {
    cancelResume();
    disconnect();
    disconnect = connect(started);
    started = true;
  };
  if (page.loaded()) {
    start();
  } else {
    page.events.addEventListener('load', start, { once: true });
  }
  page.events.addEventListener('beforeunload', () => {
    cancelResume();
    page.events.removeEventListener('load', start);
    disconnect();
    cancelResume = scheduler.schedule(resumeAfterBeforeUnloadMilliseconds, start);
  });
  page.events.addEventListener('pagehide', () => {
    cancelResume();
    page.events.removeEventListener('load', start);
    disconnect();
  });
  page.events.addEventListener('pageshow', (event) => {
    if ('persisted' in event && event.persisted === true) {
      start();
    }
  });
}

export function startRealtime(session: Session, queryClient: QueryClient, origin: string): void {
  wireRealtime({
    session,
    queryClient,
    sockets: browserSocketFactory(origin),
    scheduler: browserScheduler,
    page: { events: window, loaded: () => document.readyState === 'complete' },
    onFailure: reportError,
  });
}
