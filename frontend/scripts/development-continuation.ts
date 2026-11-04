export type ContinuationErrorKind =
  'route_already_handled' | 'target_closed' | 'fetch_continue_protocol' | 'timeout' | 'unknown';

export function continuationErrorKind(error: unknown): ContinuationErrorKind {
  if (!(error instanceof Error)) return 'unknown';
  const firstLine = error.message.split('\n')[0] ?? '';
  const message = firstLine.startsWith('route.continue: ') ? firstLine.slice('route.continue: '.length) : firstLine;
  if (message === 'Route is already handled!') return 'route_already_handled';
  if (message === 'Target page, context or browser has been closed') return 'target_closed';
  if (message.startsWith('Protocol error (Fetch.continueRequest): ')) return 'fetch_continue_protocol';
  if (error.name === 'TimeoutError') return 'timeout';
  return 'unknown';
}
