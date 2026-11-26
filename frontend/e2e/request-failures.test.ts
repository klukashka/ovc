import { describe, expect, test } from 'vitest';

import {
  type FailedRequest,
  type PageActivity,
  PageActivityRecorder,
  frameLookupFailureCounts,
  unexpectedFailures,
} from './request-failures.ts';

function cancelled(started: number, failed: number): FailedRequest {
  return {
    method: 'GET',
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
    const activity: PageActivity = { documents: [1, 3], routeChanges: [], requests: [2], closes: [] };

    const unexpected = unexpectedFailures([cancelled(2, 4)], activity);

    expect(unexpected).toEqual([]);
  });

  test('a cancellation reported just before the navigation that caused it is forgiven', () => {
    const activity: PageActivity = { documents: [1, 5], routeChanges: [], requests: [2], closes: [] };
    const failures = [cancelled(2, 3), { ...cancelled(2, 4), description: 'GET https://ovc.example/api/v1/groups' }];

    const unexpected = unexpectedFailures(failures, activity);

    expect(unexpected).toEqual([]);
  });

  test('a cancellation followed by other requests before a later navigation still fails', () => {
    const activity: PageActivity = { documents: [1, 5], routeChanges: [], requests: [2, 4], closes: [] };

    const unexpected = unexpectedFailures([cancelled(2, 3)], activity);

    expect(unexpected).toEqual([cancellationMessage]);
  });

  test('a cancellation before a navigation that started before the request still fails', () => {
    const activity: PageActivity = { documents: [2], routeChanges: [], requests: [3], closes: [] };

    const unexpected = unexpectedFailures([cancelled(3, 4)], activity);

    expect(unexpected).toEqual([cancellationMessage]);
  });

  test('a cancellation without a later navigation, before or after a close, one whose start was not seen, and any other failure, still fail', () => {
    const activity: PageActivity = { documents: [1], routeChanges: [], requests: [2, 3, 4, 7], closes: [6] };
    const failures: FailedRequest[] = [
      cancelled(2, 5),
      cancelled(Number.POSITIVE_INFINITY, 5),
      {
        method: 'GET',
        description: 'GET https://ovc.example/api/v1/students',
        reason: 'net::ERR_ABORTED',
        answered: true,
        started: 3,
        failed: 5,
      },
      {
        method: 'GET',
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

  test.each([
    'net::ERR_ABORTED',
    'NS_BINDING_ABORTED',
    'cancelled',
    'Load request cancelled',
    'Provisional navigation canceled.',
  ])('the exact cancellation text %s of a browser engine is forgiven across a navigation', (reason) => {
    const activity: PageActivity = { documents: [1, 3], routeChanges: [], requests: [2], closes: [] };

    const unexpected = unexpectedFailures([{ ...cancelled(2, 4), reason }], activity);

    expect(unexpected).toEqual([]);
  });

  test.each([
    'net::ERR_CONNECTION_ABORTED',
    'NS_ERROR_NET_RESET',
    'The network connection was lost.',
    'Load request cancelled by the server',
  ])('a transport failure %s that only resembles a cancellation still fails across a navigation', (reason) => {
    const activity: PageActivity = { documents: [1, 3], routeChanges: [], requests: [2], closes: [] };

    const unexpected = unexpectedFailures([{ ...cancelled(2, 4), reason }], activity);

    expect(unexpected).toEqual([`request failed: GET https://ovc.example/api/v1/tutors ${reason}`]);
  });
});

describe('the failed requests of a watched page whose context the test closed', () => {
  test('a cancellation that started before the marked close and failed after it is forgiven', () => {
    const activity: PageActivity = { documents: [1], routeChanges: [], requests: [2], closes: [3] };

    const unexpected = unexpectedFailures([cancelled(2, 4)], activity);

    expect(unexpected).toEqual([]);
  });

  test('a stale cancellation that failed before the marked close still fails', () => {
    const activity: PageActivity = { documents: [1], routeChanges: [], requests: [2], closes: [4] };

    const unexpected = unexpectedFailures([cancelled(2, 3)], activity);

    expect(unexpected).toEqual([cancellationMessage]);
  });

  test('a cancellation of a request started after the marked close still fails', () => {
    const activity: PageActivity = { documents: [1], routeChanges: [], requests: [3], closes: [2] };

    const unexpected = unexpectedFailures([cancelled(3, 4)], activity);

    expect(unexpected).toEqual([cancellationMessage]);
  });

  test('a transport failure across the marked close still fails', () => {
    const activity: PageActivity = { documents: [1], routeChanges: [], requests: [2], closes: [3] };

    const unexpected = unexpectedFailures([{ ...cancelled(2, 4), reason: 'NS_ERROR_NET_RESET' }], activity);

    expect(unexpected).toEqual(['request failed: GET https://ovc.example/api/v1/tutors NS_ERROR_NET_RESET']);
  });
});

function write(method: string, started: number, failed: number): FailedRequest {
  return {
    ...cancelled(started, failed),
    method,
    description: `${method} https://ovc.example/api/v1/submissions/answers`,
  };
}

describe('the failed requests of a watched page across a client-side route change', () => {
  test('a cancelled GET across a route change is forgiven, whether the change coincides with it or follows it', () => {
    const activity: PageActivity = { documents: [1], routeChanges: [3, 7], requests: [2, 5], closes: [] };

    const unexpected = unexpectedFailures([cancelled(2, 4), cancelled(5, 6)], activity);

    expect(unexpected).toEqual([]);
  });

  test.each(['PUT', 'POST', 'DELETE', 'PATCH'])(
    'an aborted %s still fails when a route change follows it or coincides with it',
    (method) => {
      const activity: PageActivity = { documents: [1], routeChanges: [4, 8], requests: [2, 6], closes: [] };

      const unexpected = unexpectedFailures([write(method, 2, 5), write(method, 6, 7)], activity);

      expect(unexpected).toEqual([
        `request failed: ${method} https://ovc.example/api/v1/submissions/answers NS_BINDING_ABORTED`,
        `request failed: ${method} https://ovc.example/api/v1/submissions/answers NS_BINDING_ABORTED`,
      ]);
    },
  );

  test('an aborted write across a full document navigation stays forgiven', () => {
    const activity: PageActivity = { documents: [1, 3], routeChanges: [], requests: [2], closes: [] };

    const unexpected = unexpectedFailures([write('PUT', 2, 4)], activity);

    expect(unexpected).toEqual([]);
  });
});

describe('the navigation bookkeeping of a watched page', () => {
  test('a committed document navigation counts once and a link click to another path is a route change', () => {
    const recorder = new PageActivityRecorder();

    recorder.request(true);
    recorder.mainFrameNavigated('https://ovc.example/today');
    const read = recorder.request(false);
    recorder.mainFrameNavigated('https://ovc.example/homework');
    const failed = recorder.failed();

    expect(recorder.activity()).toEqual({ documents: [1], routeChanges: [3], requests: [read], closes: [] });
    expect(unexpectedFailures([cancelled(read, failed)], recorder.activity())).toEqual([]);
    expect(unexpectedFailures([write('PUT', read, failed)], recorder.activity())).toHaveLength(1);
  });

  test('an address update that changes only the query or the hash is not a route change', () => {
    const recorder = new PageActivityRecorder();
    recorder.request(true);
    recorder.mainFrameNavigated('https://ovc.example/students');
    const read = recorder.request(false);

    recorder.mainFrameNavigated('https://ovc.example/students?q=ann');
    recorder.mainFrameNavigated('https://ovc.example/students?q=ann#list');
    const failed = recorder.failed();

    expect(recorder.activity().routeChanges).toEqual([]);
    expect(unexpectedFailures([cancelled(read, failed)], recorder.activity())).toEqual([cancellationMessage]);
  });

  test('a query-only update followed by a return to the earlier path is one route change back', () => {
    const recorder = new PageActivityRecorder();
    recorder.request(true);
    recorder.mainFrameNavigated('https://ovc.example/students');

    recorder.mainFrameNavigated('https://ovc.example/students/42');
    recorder.mainFrameNavigated('https://ovc.example/students/42?tab=notes');
    recorder.mainFrameNavigated('https://ovc.example/students');

    expect(recorder.activity().routeChanges).toEqual([2, 3]);
  });

  test('a document request that never commits leaves the next same-document change uncounted', () => {
    const recorder = new PageActivityRecorder();
    recorder.request(true);
    recorder.mainFrameNavigated('https://ovc.example/files');
    recorder.request(true);

    recorder.mainFrameNavigated('https://ovc.example/today');

    expect(recorder.activity()).toEqual({ documents: [1, 2], routeChanges: [], requests: [], closes: [] });
  });

  test('a marked close is recorded at its own step', () => {
    const recorder = new PageActivityRecorder();
    recorder.request(true);

    recorder.close();

    expect(recorder.activity().closes).toEqual([2]);
  });
});

describe('a request whose page the diagnostics cannot find', () => {
  test('only a request issued before its new tab has a frame is not counted', () => {
    const failures = {
      beforeTheFrame: new Error(
        'Frame for this navigation request is not available, because the request was issued before the frame is created. You can check whether the request is a navigation request by calling isNavigationRequest() method.',
      ),
      serviceWorker: new Error('Service Worker requests do not have an associated frame.'),
      detached: new Error('Target page, context or browser has been closed'),
      nameless: new Error(''),
      notAnError: 'was issued before the frame is created',
    };

    const counted = Object.fromEntries(
      Object.entries(failures).map(([name, thrown]) => [name, frameLookupFailureCounts(thrown)]),
    );

    expect(counted).toEqual({
      beforeTheFrame: false,
      serviceWorker: true,
      detached: true,
      nameless: true,
      notAnError: true,
    });
  });
});
