import { createContext, useContext } from 'react';

export interface VisibilitySource {
  readonly visible: () => boolean;
  readonly subscribe: (onChange: () => void) => () => void;
}

export const documentVisibility: VisibilitySource = {
  visible: () => document.visibilityState === 'visible',
  subscribe: (onChange) => {
    document.addEventListener('visibilitychange', onChange);
    return (): void => {
      document.removeEventListener('visibilitychange', onChange);
    };
  },
};

export const VisibilityContext = createContext<VisibilitySource>(documentVisibility);

export function useVisibility(): VisibilitySource {
  return useContext(VisibilityContext);
}

export interface ControlledVisibility extends VisibilitySource {
  readonly show: () => void;
  readonly hide: () => void;
}

export function controlledVisibility(): ControlledVisibility {
  let shown = true;
  const listeners = new Set<() => void>();
  const change = (next: boolean): void => {
    shown = next;
    for (const listener of [...listeners]) listener();
  };
  return {
    visible: () => shown,
    subscribe: (onChange) => {
      listeners.add(onChange);
      return (): void => {
        listeners.delete(onChange);
      };
    },
    show: (): void => {
      change(true);
    },
    hide: (): void => {
      change(false);
    },
  };
}
