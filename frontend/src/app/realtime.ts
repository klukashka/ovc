import type { QueryClient } from '@tanstack/react-query';

import { type Scheduler, type Session, browserScheduler } from '../lib/api/session.ts';
import { ChatSignals, chatSignals } from '../lib/realtime/chatSignals.ts';
import { RealtimeClient, connectWhileSignedIn } from '../lib/realtime/client.ts';
import { type SocketFactory, browserSocketFactory } from '../lib/realtime/socket.ts';
import type { ConnectionState } from './connectionState.ts';
import { liveRefetch } from './liveRefetch.ts';
import { notificationArrivals } from './notificationArrivals.ts';
import type { DocumentLifecycle } from './documentLifecycle.ts';

export { resumeAfterBeforeUnloadMilliseconds } from './documentLifecycle.ts';

export interface RealtimeWiring {
  readonly session: Session;
  readonly connection: ConnectionState;
  readonly queryClient: QueryClient;
  readonly sockets: SocketFactory;
  readonly scheduler: Scheduler;
  readonly lifecycle: DocumentLifecycle;
  readonly signals?: ChatSignals;
  readonly onFailure: (error: Error) => void;
}

export function wireRealtime({
  session,
  connection,
  queryClient,
  sockets,
  scheduler,
  lifecycle,
  onFailure,
  signals = chatSignals,
}: RealtimeWiring): () => void {
  const onEvent = liveRefetch(queryClient, notificationArrivals, scheduler);
  let epoch = session.currentEpoch();
  const unsubscribeSession = session.subscribe(() => {
    const nextEpoch = session.currentEpoch();
    if (nextEpoch !== epoch) {
      onEvent.cancel();
      signals.disconnect();
      epoch = nextEpoch;
    }
  });
  const connect = (connectedBefore: boolean): (() => void) =>
    connectWhileSignedIn(session, (epoch) => {
      const client = new RealtimeClient({
        sockets,
        tokens: session,
        epoch,
        scheduler,
        random: Math.random,
        onEvent: (event): void => {
          if (event.kind === 'entity' && event.entity === 'channel') signals.clear(event.id);
          onEvent(event);
        },
        onChatSignal: (signal): void => {
          signals.publish(signal);
        },
        onFailure,
        connectedBefore,
        onReadiness: (ready): void => {
          if (ready) signals.connect((channelId, typing) => client.sendTyping(channelId, typing));
          else signals.disconnect();
          connection.readiness(epoch, ready);
        },
      });
      return client;
    });
  let disconnect = (): void => undefined;
  let started = false;
  let disposed = false;
  let unsubscribeLifecycle = (): void => undefined;
  const stop = (): void => {
    disconnect();
    disconnect = (): void => undefined;
    signals.disconnect();
    onEvent.suspend();
    onEvent.cancel();
  };
  const dispose = (): void => {
    if (disposed) return;
    disposed = true;
    stop();
    connection.suspend();
    unsubscribeSession();
    unsubscribeLifecycle();
  };
  unsubscribeLifecycle = lifecycle.subscribe((phase) => {
    if (phase === 'disposed') {
      dispose();
      return;
    }
    stop();
    if (phase !== 'active') {
      if (phase === 'hidden') connection.suspend();
      return;
    }
    onEvent.resume();
    connection.resume();
    disconnect = connect(started);
    started = true;
  });
  return dispose;
}

export function startRealtime(
  session: Session,
  queryClient: QueryClient,
  origin: string,
  connection: ConnectionState,
  lifecycle: DocumentLifecycle,
): () => void {
  return wireRealtime({
    session,
    connection,
    queryClient,
    sockets: browserSocketFactory(origin),
    scheduler: browserScheduler,
    lifecycle,
    onFailure: reportRealtimeError,
  });
}

export function reportRealtimeError(error: Error): void {
  window.reportError(error);
}
