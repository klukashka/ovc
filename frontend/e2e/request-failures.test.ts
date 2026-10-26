import { describe, expect, test } from 'vitest';

import { type FailedRequest, unexpectedFailures } from './request-failures.ts';

function cancelled(document: number): FailedRequest {
  return {
    description: 'GET https://ovc.example/api/v1/tutors',
    reason: 'NS_BINDING_ABORTED',
    answered: false,
    document,
  };
}

describe('the failed requests of a watched page', () => {
  test('a cancellation whose document changed afterwards is forgiven, even when the change was seen after it', () => {
    const failures = [cancelled(1)];

    const unexpected = unexpectedFailures(failures, 2);

    expect(unexpected).toEqual([]);
  });

  test('a cancellation without a document change, one whose start was not seen, and any other failure, still fail', () => {
    const failures: FailedRequest[] = [
      cancelled(2),
      cancelled(Number.POSITIVE_INFINITY),
      {
        description: 'GET https://ovc.example/api/v1/students',
        reason: 'net::ERR_ABORTED',
        answered: true,
        document: 2,
      },
      {
        description: 'GET https://ovc.example/api/v1/groups',
        reason: 'net::ERR_CONNECTION_REFUSED',
        answered: false,
        document: 1,
      },
    ];

    const unexpected = unexpectedFailures(failures, 2);

    expect(unexpected).toEqual([
      'request failed: GET https://ovc.example/api/v1/tutors NS_BINDING_ABORTED',
      'request failed: GET https://ovc.example/api/v1/tutors NS_BINDING_ABORTED',
      'request failed: GET https://ovc.example/api/v1/groups net::ERR_CONNECTION_REFUSED',
    ]);
  });
});
