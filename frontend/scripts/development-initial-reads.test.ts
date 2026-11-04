import { expect, test } from 'vitest';

import {
  DevelopmentInitialReads,
  DevelopmentNavigationReads,
  type DevelopmentReadTarget,
} from './development-initial-reads.ts';

function request(
  id: number,
  target: DevelopmentReadTarget = 'list',
  generation = 1,
  account = 'owned',
): import('./development-initial-reads.ts').InitialReadIdentity {
  return { id, target, generation, account };
}

test('one initial cancellation requires its exact later complete replacement and does not reopen', () => {
  const reads = new DevelopmentInitialReads('owned', 1, 100);
  reads.begin(request(1), 1);

  reads.failed(1, true, 2);
  reads.begin(request(2), 3);
  reads.ready(4);
  reads.completed(2, 200, 5);
  reads.begin(request(3), 6);
  reads.completed(3, 200, 7);

  expect(() => {
    reads.verify(8);
  }).not.toThrow();
  expect(reads.expectedCancellations()).toBe(1);
  expect(reads.snapshot().find((entry) => entry.target === 'list')).toMatchObject({
    cancelled: 1,
    replacements: 1,
    closure: 'ready',
  });
});

test.each(['unread', 'list', 'current_channel'] as const)(
  'first %s enrolls after unrelated ready and completion',
  (target) => {
    const reads = new DevelopmentInitialReads('owned', 1, 100);
    const other = target === 'unread' ? 'list' : 'unread';
    reads.ready(1);
    reads.begin(request(9, other), 2);
    reads.completed(9, 200, 3);

    reads.begin(request(1, target), 4);
    reads.begin(request(2, target), 5);
    reads.completed(2, 200, 6);
    reads.failed(1, true, 7);

    expect(reads.expectedCancellations()).toBe(1);
    expect(reads.failureRecords()).toEqual([]);
  },
);

test.each(['deadline', 'seal'] as const)('first lifetime cannot enroll after %s', (boundary) => {
  const reads = new DevelopmentInitialReads('owned', 1, 10);
  if (boundary === 'seal') reads.sealInitial();

  reads.begin(request(1), boundary === 'deadline' ? 10 : 2);
  reads.failed(1, true, boundary === 'deadline' ? 11 : 3);

  expect(reads.expectedCancellations()).toBe(0);
  expect(reads.failureRecords()).toContainEqual({
    kind: boundary === 'seal' ? 'cancellation_after_initial_seal' : 'cancellation_after_initial_window',
    count: 1,
  });
});

test.each([
  ['repeated', 'repeated_initial_cancellation'],
  ['failed_replacement', 'http_failure'],
  ['missing_replacement', 'unresolved_initial_replacement'],
  ['other_target', 'unexpected_target_cancellation'],
  ['other_generation', 'failure_ownership_changed'],
  ['other_account', 'failure_ownership_changed'],
  ['scope_loss', 'scope_boundary_changed'],
  ['non_cancellation', 'non_cancellation_failure'],
] as const)('the initial expectation rejects %s for its exact cause', (failure, kind) => {
  const reads = new DevelopmentInitialReads('owned', 1, 100);
  reads.begin(
    request(
      1,
      failure === 'other_target' ? 'other' : 'list',
      failure === 'other_generation' ? 2 : 1,
      failure === 'other_account' ? 'other' : 'owned',
    ),
    1,
  );

  reads.failed(1, failure !== 'non_cancellation', 2);
  if (failure === 'scope_loss') {
    reads.ownershipChanged();
    reads.begin(request(2), 3);
    reads.completed(2, 200, 4);
  }
  if (failure === 'repeated' || failure === 'failed_replacement') {
    reads.begin(request(2), 3);
    if (failure === 'repeated') {
      reads.failed(2, true, 4);
      reads.begin(request(3), 5);
      reads.completed(3, 200, 6);
    } else reads.completed(2, 404, 4);
  }

  expect(reads.failureRecords()).toContainEqual({ kind, count: 1 });
  expect(() => {
    reads.verify(7);
  }).toThrow();
});

test('the repeated cancellation guard is independently necessary despite unresolved replacement work', () => {
  const reads = new DevelopmentInitialReads('owned', 1, 100);
  reads.begin(request(1), 1);
  reads.failed(1, true, 2);

  reads.begin(request(2), 3);
  reads.failed(2, true, 4);
  reads.begin(request(3), 5);
  reads.completed(3, 200, 6);

  expect(reads.failureRecords()).toContainEqual({ kind: 'repeated_initial_cancellation', count: 1 });
  expect(reads.summary(7).unresolved).toBe(1);
});

test.each(['document_generation_changed', 'account_boundary_changed'] as const)(
  'the %s guard is independently necessary after the old replacement completes',
  (kind) => {
    const reads = new DevelopmentInitialReads('owned', 1, 100);
    reads.begin(request(1), 1);
    reads.failed(1, true, 2);

    reads.ownershipChanged(kind);
    reads.begin(request(2), 3);
    reads.completed(2, 200, 4);

    expect(reads.summary(5).unresolved).toBe(0);
    expect(reads.failureRecords()).toEqual([{ kind, count: 1 }]);
    expect(() => {
      reads.verify(5);
    }).toThrow();
  },
);

test.each([100, 101])(
  'a designated replacement completing at %s cannot resolve after the original deadline',
  (completion) => {
    const reads = new DevelopmentInitialReads('owned', 1, 100);
    reads.begin(request(1), 1);
    reads.failed(1, true, 2);
    reads.begin(request(2), 3);

    reads.completed(2, 200, completion);

    expect(reads.failureRecords()).toContainEqual({ kind: 'replacement_completed_after_deadline', count: 1 });
    expect(reads.summary(completion).unresolved).toBe(1);
    expect(() => {
      reads.verify(completion);
    }).toThrow();
  },
);

test('response headers or an unfinished replacement cannot resolve an initial cancellation', () => {
  const reads = new DevelopmentInitialReads('owned', 1, 100);
  reads.begin(request(1), 1);
  reads.failed(1, true, 2);

  reads.begin(request(2), 3);

  expect(() => {
    reads.verify(4);
  }).toThrow();
  expect(reads.pending()).toBe(1);
});

test('zero cancellations are valid and a later ordinary SPA request failure is still rejected', () => {
  const reads = new DevelopmentInitialReads('owned', 1, 100);
  reads.begin(request(1), 1);
  reads.completed(1, 200, 2);

  reads.begin(request(2, 'other'), 3);
  reads.failed(2, true, 4);

  expect(() => {
    reads.verify(5);
  }).toThrow();
  expect(reads.expectedCancellations()).toBe(0);
});

test.each(['ready', 'other_complete', 'second_complete'] as const)(
  'immutable first eligibility survives %s before cancellation callback delivery',
  (closure) => {
    const reads = new DevelopmentInitialReads('owned', 1, 100);
    reads.begin(request(1), 1);
    if (closure === 'ready') reads.ready(2);
    if (closure === 'other_complete') {
      reads.begin(request(9, 'unread'), 2);
      reads.completed(9, 200, 3);
    }

    reads.begin(request(2), 4);
    if (closure === 'second_complete') reads.completed(2, 200, 5);
    reads.failed(1, true, 6);
    if (closure !== 'second_complete') reads.completed(2, 200, 7);

    expect(reads.expectedCancellations()).toBe(1);
    expect(reads.snapshot().find((entry) => entry.target === 'list')).toMatchObject({
      firstEligible: true,
      cancelled: 1,
      replacements: 1,
      secondCompleteBeforeDeadline: true,
    });
    expect(() => {
      reads.verify(8);
    }).not.toThrow();
  },
);

test.each([100, 101])('an eligible first cancellation delivered at %s misses the original deadline', (now) => {
  const reads = new DevelopmentInitialReads('owned', 1, 100);
  reads.begin(request(1), 1);
  reads.begin(request(2), 2);
  reads.completed(2, 200, 3);

  reads.failed(1, true, now);

  expect(reads.expectedCancellations()).toBe(0);
  expect(reads.failureRecords()).toContainEqual({ kind: 'cancellation_after_deadline', count: 1 });
});

test('a third successful request cannot substitute for the failed exact second lifetime', () => {
  const reads = new DevelopmentInitialReads('owned', 1, 100);
  reads.begin(request(1), 1);
  reads.begin(request(2), 2);

  reads.completed(2, 503, 3);
  reads.begin(request(3), 4);
  reads.completed(3, 200, 5);
  reads.failed(1, true, 6);

  expect(reads.expectedCancellations()).toBe(1);
  expect(reads.summary(7).unresolved).toBe(1);
  expect(reads.snapshot().find((entry) => entry.target === 'list')?.replacements).toBe(0);
  expect(() => {
    reads.verify(7);
  }).toThrow();
});

test('a successful first lifetime cannot later become an eligible cancellation', () => {
  const reads = new DevelopmentInitialReads('owned', 1, 100);
  reads.begin(request(1), 1);
  reads.completed(1, 200, 2);

  reads.failed(1, true, 3);

  expect(reads.expectedCancellations()).toBe(0);
  expect(reads.failureRecords()).toContainEqual({ kind: 'unknown_or_repeated_request_failure', count: 1 });
});

test.each(['account', 'document'] as const)(
  'a second lifetime with a different %s never resolves an eligible first cancellation',
  (owner) => {
    const reads = new DevelopmentInitialReads('owned', 1, 100);
    reads.begin(request(1), 1);
    reads.failed(1, true, 2);

    reads.begin(request(2, 'list', owner === 'document' ? 2 : 1, owner === 'account' ? 'other' : 'owned'), 3);
    reads.completed(2, 200, 4);

    expect(reads.summary(5).unresolved).toBe(1);
    expect(reads.failureRecords()).toContainEqual({ kind: 'completion_ownership_changed', count: 1 });
  },
);

test('a closed initial window still records an SPA ownership change while its owned request is pending', () => {
  const reads = new DevelopmentInitialReads('owned', 1, 100);
  reads.begin(request(1), 1);
  reads.completed(1, 200, 2);
  reads.begin(request(2, 'current_channel'), 3);

  reads.ownershipChanged('spa_scope_changed');
  reads.completed(2, 200, 4);

  expect(reads.summary(5).unresolved).toBe(0);
  expect(reads.failureRecords()).toEqual([{ kind: 'spa_scope_changed', count: 1 }]);
});

test('a new SPA target does not become pending work of the frozen initial scope', () => {
  const reads = new DevelopmentInitialReads('owned', 1, 100);
  reads.begin(request(1), 1);
  reads.completed(1, 200, 2);
  reads.begin(request(2, 'other'), 3);

  reads.ownershipChanged('spa_scope_changed');
  reads.completed(2, 200, 4);

  expect(reads.failureRecords()).toEqual([]);
  expect(() => {
    reads.verify(5);
  }).not.toThrow();
});

function completedInitialReads(): DevelopmentInitialReads {
  const reads = new DevelopmentInitialReads('owned', 1, 10);
  reads.ready(1);
  for (const [index, target] of (['unread', 'list', 'current_channel', 'history'] as const).entries()) {
    reads.begin(request(index + 1, target), 2);
    reads.completed(index + 1, 200, 3);
  }
  return reads;
}

test.each([10, 11])('zero-cancellation initial settlement at %s cannot pass the original deadline', (now) => {
  const reads = completedInitialReads();

  const settled = reads.initialSettled(now);

  expect(settled).toBe(false);
  expect(reads.failureRecords()).toContainEqual({ kind: 'initial_settlement_after_deadline', count: 1 });
});

test('ordinary later verification remains valid after an initial settlement completed within its deadline', () => {
  const reads = completedInitialReads();
  const settled = reads.initialSettled(9);
  reads.sealInitial();

  reads.begin(request(9, 'other'), 11);
  reads.completed(9, 200, 12);

  expect(settled).toBe(true);
  expect(reads.summary(13).initialSettlementComplete).toBe(true);
  expect(() => {
    reads.verify(13);
  }).not.toThrow();
});

test('successful first response prevents a later lifetime cancellation before assembly seal', () => {
  const reads = new DevelopmentInitialReads('owned', 1, 100);
  reads.begin(request(1), 1);
  reads.completed(1, 200, 2);

  reads.begin(request(2), 3);
  reads.failed(2, true, 4);

  expect(reads.expectedCancellations()).toBe(0);
  expect(reads.failureRecords()).toEqual([{ kind: 'repeated_initial_cancellation', count: 1 }]);
});

test('synthetic explicit seal classification precedes first-identity rejection before deadline', () => {
  const reads = new DevelopmentInitialReads('owned', 1, 100);
  reads.begin(request(1), 1);

  reads.sealInitial();
  reads.failed(1, true, 2);

  expect(reads.failureRecords()).toEqual([{ kind: 'cancellation_after_initial_seal', count: 1 }]);
  expect(reads.expectedCancellations()).toBe(0);
});

test.each(['document_generation_changed', 'account_boundary_changed', 'spa_scope_changed'] as const)(
  'ownership boundary %s cannot enroll a new target',
  (kind) => {
    const reads = new DevelopmentInitialReads('owned', 1, 100);
    reads.ready(1);

    reads.ownershipChanged(kind);
    reads.begin(request(1), 2);
    reads.failed(1, true, 3);

    expect(reads.expectedCancellations()).toBe(0);
    expect(reads.failureRecords()).toContainEqual({ kind: 'cancellation_after_initial_window', count: 1 });
  },
);

test.each(['cancel_first', 'second_first'] as const)(
  'destination exact replacement accepts %s callback ordering',
  (ordering) => {
    const reads = new DevelopmentNavigationReads('owned', 1, 100);
    reads.navigation(true);
    reads.begin(request(1, 'current_channel'), 1);
    reads.begin(request(2, 'current_channel'), 2);

    if (ordering === 'cancel_first') reads.failed(1, true, 3);
    reads.completed(2, 200, 4);
    if (ordering === 'second_first') reads.failed(1, true, 5);
    reads.seal(6);

    expect(reads.expected()).toBe(1);
    expect(reads.raw()).toBe(1);
    expect((): void => {
      reads.verify(101);
    }).not.toThrow();
  },
);

test.each([
  'unobserved',
  'wrong_navigation',
  'repeated_navigation',
  'owner',
  'document',
  'source',
  'wrong_target',
  'repeated',
  'failed_second',
  'missing_second',
  'late_second',
  'first_success',
  'after_seal',
  'noncancel',
] as const)('destination boundary rejects %s without unrelated unresolved cancellation', (failure) => {
  const reads = new DevelopmentNavigationReads('owned', 1, 100);
  if (failure !== 'unobserved') reads.navigation(failure !== 'wrong_navigation');
  if (failure === 'repeated_navigation') reads.navigation(true);
  reads.begin(
    request(
      1,
      failure === 'source' ? 'other' : failure === 'wrong_target' ? 'list' : 'current_channel',
      failure === 'document' ? 2 : 1,
      failure === 'owner' ? 'other' : 'owned',
    ),
    1,
  );

  if (failure === 'first_success' || failure === 'after_seal') {
    reads.completed(1, 200, 2);
    if (failure === 'after_seal') reads.seal(3);
    reads.begin(request(2, 'current_channel'), 4);
    reads.failed(2, true, 5);
  } else {
    reads.failed(1, failure !== 'noncancel', 2);
    if (failure === 'repeated') {
      reads.begin(request(2, 'current_channel'), 3);
      reads.failed(2, true, 4);
    } else if (failure !== 'missing_second') {
      reads.begin(request(2, 'current_channel'), 3);
      reads.completed(2, failure === 'failed_second' ? 404 : 200, failure === 'late_second' ? 100 : 4);
      if (failure === 'failed_second') {
        reads.begin(request(3, 'current_channel'), 5);
        reads.completed(3, 200, 6);
      }
    }
  }

  const reasons: Readonly<Record<typeof failure, string>> = {
    unobserved: 'navigation_not_observed',
    wrong_navigation: 'spa_scope_changed',
    repeated_navigation: 'spa_scope_changed',
    owner: 'failure_ownership_changed',
    document: 'failure_ownership_changed',
    source: 'unexpected_target_cancellation',
    wrong_target: 'unexpected_target_cancellation',
    repeated: 'repeated_initial_cancellation',
    failed_second: 'http_failure',
    missing_second: 'unresolved_initial_replacement',
    late_second: 'replacement_completed_after_deadline',
    first_success: 'repeated_initial_cancellation',
    after_seal: 'cancellation_after_initial_seal',
    noncancel: 'non_cancellation_failure',
  };
  expect(reads.failures()).toContainEqual({ kind: reasons[failure], count: 1 });
  expect((): void => {
    reads.verify(7);
  }).toThrow();
});
