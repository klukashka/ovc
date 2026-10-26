import { describe, expect, test } from 'vitest';

import { type FailedRequest, type PageActivity, unexpectedFailures } from './request-failures.ts';

function cancelled(started: number, failed: number): FailedRequest {
  return {
    description: 'GET https://ovc.example/api/v1/tutors',
    reason: 'NS_BINDING_ABORTED',
    answered: false,
    started,
    failed,
  };
}

const cancellationMessage = 'request failed: GET https://ovc.example/api/v1/tutors NS_BINDING_ABORTED';

describe('the failed requests of a watched page', () => {
  test('a cancellation by a navigation that started after the request and before its failure is forgiven', () => {
    const activity: PageActivity = { navigations: [1, 3], requests: [2] };

    const unexpected = unexpectedFailures([cancelled(2, 4)], activity);

    expect(unexpected).toEqual([]);
  });

  test('a cancellation reported just before the navigation that caused it is forgiven', () => {
    const activity: PageActivity = { navigations: [1, 5], requests: [2] };
    const failures = [cancelled(2, 3), { ...cancelled(2, 4), description: 'GET https://ovc.example/api/v1/groups' }];

    const unexpected = unexpectedFailures(failures, activity);

    expect(unexpected).toEqual([]);
  });

  test('a cancellation followed by other requests before a later navigation still fails', () => {
    const activity: PageActivity = { navigations: [1, 5], requests: [2, 4] };

    const unexpected = unexpectedFailures([cancelled(2, 3)], activity);

    expect(unexpected).toEqual([cancellationMessage]);
  });

  test('a cancellation before a navigation that started before the request still fails', () => {
    const activity: PageActivity = { navigations: [2], requests: [3] };

    const unexpected = unexpectedFailures([cancelled(3, 4)], activity);

    expect(unexpected).toEqual([cancellationMessage]);
  });

  test('a cancellation without a later navigation, one whose start was not seen, and any other failure, still fail', () => {
    const activity: PageActivity = { navigations: [1], requests: [2, 3, 4, 7] };
    const failures: FailedRequest[] = [
      cancelled(2, 5),
      cancelled(Number.POSITIVE_INFINITY, 5),
      {
        description: 'GET https://ovc.example/api/v1/students',
        reason: 'net::ERR_ABORTED',
        answered: true,
        started: 3,
        failed: 5,
      },
      {
        description: 'GET https://ovc.example/api/v1/groups',
        reason: 'net::ERR_CONNECTION_REFUSED',
        answered: false,
        started: 4,
        failed: 5,
      },
      cancelled(7, 8),
    ];

    const unexpected = unexpectedFailures(failures, activity);

    expect(unexpected).toEqual([
      cancellationMessage,
      cancellationMessage,
      'request failed: GET https://ovc.example/api/v1/groups net::ERR_CONNECTION_REFUSED',
      cancellationMessage,
    ]);
  });
});
