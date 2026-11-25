import { asApiError } from '../../lib/api/client.ts';
import type { ActionState } from './action.ts';

export function hasUnknownOutcome(state: ActionState): boolean {
  return state.kind === 'refused' && (state.error.status === 0 || state.error.status >= 500);
}

export async function settledRequest<T>(request: () => Promise<T | ActionState>): Promise<T | ActionState> {
  try {
    return await request();
  } catch (thrown: unknown) {
    return { kind: 'refused', error: asApiError(thrown), retryAfter: null };
  }
}
