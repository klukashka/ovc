import { useRef, useState } from 'react';

import { type ApiError, apiErrorFrom, asApiError } from '../../lib/api/client.ts';
import { retryAfterSeconds } from './Form.tsx';

export type ActionState =
  | { readonly kind: 'idle' }
  | { readonly kind: 'busy' }
  | { readonly kind: 'done' }
  | { readonly kind: 'refused'; readonly error: ApiError; readonly retryAfter: number | null };

export interface Action {
  readonly state: ActionState;
  readonly busy: boolean;
  readonly run: (work: () => Promise<ActionState>) => void;
  readonly reset: () => void;
}

export function refusedBy(response: Response, error: unknown): ActionState {
  return { kind: 'refused', error: apiErrorFrom(response.status, error), retryAfter: retryAfterSeconds(response) };
}

export const done: ActionState = { kind: 'done' };

export function useAction(onSettled?: (state: ActionState) => void): Action {
  const [state, setState] = useState<ActionState>({ kind: 'idle' });
  const running = useRef(false);
  function settle(settled: ActionState): void {
    running.current = false;
    setState(settled);
    onSettled?.(settled);
  }
  return {
    state,
    busy: state.kind === 'busy',
    run: (work): void => {
      if (running.current) {
        return;
      }
      running.current = true;
      setState({ kind: 'busy' });
      work().then(settle, (thrown: unknown) => {
        settle({ kind: 'refused', error: asApiError(thrown), retryAfter: null });
      });
    },
    reset: (): void => {
      if (running.current) {
        return;
      }
      setState({ kind: 'idle' });
    },
  };
}

const staleCodes: ReadonlySet<string> = new Set([
  'not_found',
  'relationship_not_active',
  'relationship_not_provisional',
  'not_editable',
]);

export function showsStaleData(state: ActionState): boolean {
  return state.kind === 'refused' && state.error.code !== undefined && staleCodes.has(state.error.code);
}
