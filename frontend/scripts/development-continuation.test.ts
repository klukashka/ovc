import { expect, test } from 'vitest';

import { continuationErrorKind } from './development-continuation.ts';

test.each([
  ['route.continue: Route is already handled!', 'route_already_handled'],
  ['route.continue: Target page, context or browser has been closed', 'target_closed'],
  ['route.continue: Protocol error (Fetch.continueRequest): synthetic-private-marker', 'fetch_continue_protocol'],
  ['synthetic-private-marker https://private.example/path?credential=synthetic', 'unknown'],
] as const)('continuation diagnostics retain only a fixed error kind', (message, kind) => {
  const error = new Error(message);

  const classified = continuationErrorKind(error);

  expect(classified).toBe(kind);
  expect(classified).not.toContain('synthetic-private-marker');
});
