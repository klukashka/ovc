import { useSyncExternalStore } from 'react';

export class NotificationArrivals {
  private count = 0;
  private readonly listeners = new Set<() => void>();

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return (): void => {
      this.listeners.delete(listener);
    };
  };

  readonly snapshot = (): number => this.count;

  arrived(): void {
    this.count += 1;
    for (const listener of this.listeners) {
      listener();
    }
  }
}

export const notificationArrivals = new NotificationArrivals();

export function useNotificationArrivals(): number {
  return useSyncExternalStore(notificationArrivals.subscribe, notificationArrivals.snapshot);
}
