import { createContext, useContext, useSyncExternalStore } from 'react';

import type { Scheduler } from '../lib/api/session.ts';
import type { SessionOwner } from '../lib/realtime/client.ts';

export const offlineGraceMilliseconds = 5_000;
export type ConnectionStatus = 'waiting' | 'offline' | 'recovered';

export class ConnectionState {
  private status: ConnectionStatus = 'waiting';
  private epoch: number | null = null;
  private paused = false;
  private ready = false;
  private cancelGrace: (() => void) | null = null;
  private readonly listeners = new Set<() => void>();

  private readonly scheduler: Scheduler;

  constructor(scheduler: Scheduler) {
    this.scheduler = scheduler;
  }

  readonly snapshot = (): ConnectionStatus => this.status;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  follow(session: SessionOwner): () => void {
    const sync = (): void => {
      this.account(session.snapshot().kind === 'signedIn' ? session.currentEpoch() : null);
    };
    const unsubscribe = session.subscribe(sync);
    sync();
    return () => {
      unsubscribe();
      this.account(null);
    };
  }

  account(epoch: number | null): void {
    if (this.epoch === epoch) {
      return;
    }
    this.cancel();
    this.epoch = epoch;
    this.ready = false;
    this.publish('waiting');
    this.wait();
  }

  readiness(epoch: number, ready: boolean): void {
    if (epoch !== this.epoch || this.paused) {
      return;
    }
    this.ready = ready;
    if (ready) {
      this.cancel();
      if (this.status === 'offline') {
        this.publish('recovered');
      }
    } else {
      this.wait();
    }
  }

  suspend(): void {
    this.paused = true;
    this.ready = false;
    this.cancel();
  }

  resume(): void {
    this.paused = false;
    this.wait();
  }

  private wait(): void {
    if (this.epoch === null || this.paused || this.ready || this.cancelGrace !== null || this.status === 'offline') {
      return;
    }
    this.cancelGrace = this.scheduler.schedule(offlineGraceMilliseconds, () => {
      this.cancelGrace = null;
      this.publish('offline');
    });
  }

  private cancel(): void {
    this.cancelGrace?.();
    this.cancelGrace = null;
  }

  private publish(status: ConnectionStatus): void {
    if (this.status === status) {
      return;
    }
    this.status = status;
    for (const listener of this.listeners) {
      listener();
    }
  }
}

const inactiveConnection = {
  subscribe: (): (() => void) => () => undefined,
  snapshot: (): ConnectionStatus => 'waiting',
};

export const ConnectionContext = createContext<Pick<ConnectionState, 'subscribe' | 'snapshot'>>(inactiveConnection);

export function useConnectionStatus(): ConnectionStatus {
  const state = useContext(ConnectionContext);
  return useSyncExternalStore(state.subscribe, state.snapshot);
}
