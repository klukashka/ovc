import { describe, expect, test } from 'vitest';

import { apiErrorFrom } from './client.ts';

function envelope(code: string): unknown {
  return { error: { code, message: 'Server message', request_id: 'request-1' } };
}

describe('API errors', () => {
  test('a known code keeps its code and request identifier', () => {
    const received = envelope('rate_limited');

    const error = apiErrorFrom(429, received);

    expect([error.status, error.code, error.requestId]).toEqual([429, 'rate_limited', 'request-1']);
  });

  test('an unknown code keeps the request identifier without claiming a code', () => {
    const received = envelope('code_from_a_newer_server');

    const error = apiErrorFrom(409, received);

    expect([error.status, error.code, error.requestId]).toEqual([409, undefined, 'request-1']);
  });

  test('a body that is not an error envelope keeps only the status', () => {
    const received = { error: { code: 'rate_limited' } };

    const error = apiErrorFrom(502, received);

    expect([error.status, error.code, error.requestId]).toEqual([502, undefined, undefined]);
  });
});
