export const initialReadTargets = ['unread', 'list', 'current_channel'] as const;
export type InitialReadTarget = (typeof initialReadTargets)[number];
export type DevelopmentReadTarget = InitialReadTarget | 'history' | 'other';
export type OwnershipFailure =
  | 'document_generation_changed'
  | 'account_boundary_changed'
  | 'account_identity_mismatch'
  | 'scope_boundary_changed'
  | 'spa_scope_changed';
export type InitialWindowClosure = 'open' | 'ready' | 'complete' | 'deadline' | 'ownership' | 'sealed';
export interface InitialReadIdentity {
  readonly id: number;
  readonly target: DevelopmentReadTarget;
  readonly account: string;
  readonly generation: number;
}
interface Attempt extends InitialReadIdentity {
  readonly order: number;
  readonly ordinal: number;
  readonly eligible: boolean;
  status: number | null;
  ended: boolean;
}
interface TargetState {
  attempts: number;
  cancelled: number;
  replacements: number;
  completed: number;
  afterReady: number;
  firstId: number | null;
  secondCompletedAt: number | null;
  secondSuccessful: boolean;
  replacementId: number | null;
}

export class DevelopmentInitialReads {
  private readonly requests = new Map<number, Attempt>();
  private readonly targets = new Map<DevelopmentReadTarget, TargetState>();
  private readonly failures = new Map<string, number>();
  private order = 0;
  private readyOrder: number | null = null;
  private closure: InitialWindowClosure = 'open';
  private rawFailures = 0;
  private initialSettlementComplete = false;

  private readonly account: string;
  private readonly generation: number;
  private readonly deadline: number;

  constructor(account: string, generation: number, deadline: number) {
    this.account = account;
    this.generation = generation;
    this.deadline = deadline;
    for (const target of [...initialReadTargets, 'history', 'other'] as const) {
      this.targets.set(target, {
        attempts: 0,
        cancelled: 0,
        replacements: 0,
        completed: 0,
        afterReady: 0,
        firstId: null,
        secondCompletedAt: null,
        secondSuccessful: false,
        replacementId: null,
      });
    }
  }

  private problem(kind: string): void {
    this.failures.set(kind, Math.min(1000, (this.failures.get(kind) ?? 0) + 1));
  }

  private target(target: DevelopmentReadTarget): TargetState {
    const state = this.targets.get(target);
    if (state === undefined) throw new Error('development_target_missing');
    return state;
  }

  private owns(identity: InitialReadIdentity): boolean {
    return identity.account === this.account && identity.generation === this.generation;
  }

  private tick(now: number): void {
    this.order += 1;
    if (now >= this.deadline && this.closure === 'open') this.closure = 'deadline';
  }

  begin(identity: InitialReadIdentity, now: number): void {
    this.tick(now);
    if (this.requests.size >= 2000 || this.requests.has(identity.id)) {
      this.problem('request_identity_overflow_or_duplicate');
      return;
    }
    if (!this.owns(identity)) this.problem('request_ownership_changed');
    const state = this.target(identity.target);
    state.attempts += 1;
    if (state.attempts === 1) state.firstId = identity.id;
    if (state.attempts === 2) state.replacementId = identity.id;
    const eligible =
      state.attempts === 1 &&
      this.owns(identity) &&
      !this.initialSettlementComplete &&
      this.closure !== 'ownership' &&
      now < this.deadline &&
      initialReadTargets.some((target) => target === identity.target);
    this.requests.set(identity.id, {
      ...identity,
      order: this.order,
      ordinal: state.attempts,
      eligible,
      status: null,
      ended: false,
    });
  }

  failed(id: number, cancellation: boolean, now: number): void {
    this.tick(now);
    this.rawFailures += 1;
    const request = this.requests.get(id);
    if (request === undefined || request.ended) {
      this.problem('unknown_or_repeated_request_failure');
      return;
    }
    request.ended = true;
    const state = this.target(request.target);
    if (!this.owns(request)) {
      this.problem('failure_ownership_changed');
      return;
    }
    if (!initialReadTargets.some((target) => target === request.target)) {
      this.problem('unexpected_target_cancellation');
      return;
    }
    if (!cancellation) {
      this.problem('non_cancellation_failure');
      return;
    }
    if (this.initialSettlementComplete) {
      this.problem('cancellation_after_initial_seal');
      return;
    }
    if (request.ordinal !== 1 || state.cancelled !== 0) {
      this.problem('repeated_initial_cancellation');
      return;
    }
    if (!request.eligible) {
      this.problem('cancellation_after_initial_window');
      return;
    }
    if (now >= this.deadline) {
      this.problem('cancellation_after_deadline');
      return;
    }
    if (this.closure === 'ownership') {
      this.problem('failure_after_ownership_change');
      return;
    }
    state.cancelled = 1;
    state.replacements =
      state.secondSuccessful && state.secondCompletedAt !== null && state.secondCompletedAt < this.deadline ? 1 : 0;
  }

  completed(id: number, status: number, now: number): void {
    this.tick(now);
    const request = this.requests.get(id);
    if (request === undefined || request.ended) {
      this.problem('unknown_or_repeated_completion');
      return;
    }
    request.ended = true;
    request.status = status;
    if (!this.owns(request)) {
      this.problem('completion_ownership_changed');
      return;
    }
    if (status >= 400 || status === 0) {
      this.problem('http_failure');
      return;
    }
    const state = this.target(request.target);
    if (status !== 200) {
      if (request.target !== 'other') this.problem('target_response_not_successful');
      return;
    }
    const first = state.firstId === null ? undefined : this.requests.get(state.firstId);
    if (
      state.replacementId === request.id &&
      first?.eligible === true &&
      first.status !== 200 &&
      now >= this.deadline
    ) {
      this.problem('replacement_completed_after_deadline');
      return;
    }
    state.completed += 1;
    if (this.readyOrder !== null && request.order > this.readyOrder) state.afterReady += 1;
    if (initialReadTargets.some((target) => target === request.target)) {
      if (this.closure === 'open') this.closure = 'complete';
      if (state.replacementId === request.id) {
        state.secondCompletedAt = now;
        state.secondSuccessful = true;
        if (state.cancelled === 1 && now < this.deadline) state.replacements = 1;
      }
    }
  }

  ready(now: number): void {
    this.tick(now);
    this.readyOrder ??= this.order;
    if (this.closure === 'open') this.closure = 'ready';
  }

  ownershipChanged(kind: OwnershipFailure = 'scope_boundary_changed'): void {
    this.order += 1;
    if (this.closure === 'open' || this.unresolved() !== 0 || this.pendingOwned() !== 0) this.problem(kind);
    this.closure = 'ownership';
  }

  private unresolved(): number {
    return initialReadTargets.filter((target) => {
      const state = this.target(target);
      return state.cancelled !== 0 && state.replacements !== 1;
    }).length;
  }

  initialSettled(now: number): boolean {
    this.tick(now);
    if (now >= this.deadline) {
      this.problem('initial_settlement_after_deadline');
      return false;
    }
    const settled =
      this.readyOrder !== null &&
      this.unresolved() === 0 &&
      initialReadTargets.every((target) => this.target(target).afterReady > 0) &&
      this.target('history').afterReady > 0 &&
      this.pending() === 0;
    return settled;
  }

  sealInitial(): void {
    this.initialSettlementComplete = true;
    this.closure = 'sealed';
  }

  private pendingOwned(): number {
    return [...this.requests.values()].filter((request) => !request.ended && request.target !== 'other').length;
  }

  pending(): number {
    return [...this.requests.values()].filter((request) => !request.ended).length;
  }

  expectedCancellations(): number {
    return initialReadTargets.reduce((sum, target) => sum + this.target(target).cancelled, 0);
  }

  failedRequests(): number {
    return this.rawFailures;
  }

  problemCount(): number {
    return [...this.failures.values()].reduce((sum, count) => sum + count, 0);
  }

  failureRecords(): readonly { readonly kind: string; readonly count: number }[] {
    const records = [...this.failures].map(([kind, count]) => ({ kind, count }));
    const unresolved = this.unresolved();
    if (unresolved !== 0) records.push({ kind: 'unresolved_initial_replacement', count: unresolved });
    return records;
  }

  summary(now: number): {
    readonly pending: number;
    readonly pendingOwned: number;
    readonly initialSettlementComplete: boolean;
    readonly unresolved: number;
    readonly ready: boolean;
    readonly closure: InitialWindowClosure;
    readonly deadlineExpired: boolean;
  } {
    return {
      pending: this.pending(),
      pendingOwned: this.pendingOwned(),
      initialSettlementComplete: this.initialSettlementComplete,
      unresolved: this.unresolved(),
      ready: this.readyOrder !== null,
      closure: this.closure,
      deadlineExpired: now >= this.deadline,
    };
  }

  snapshot(): readonly {
    readonly target: DevelopmentReadTarget;
    readonly attempts: number;
    readonly cancelled: number;
    readonly replacements: number;
    readonly completed: number;
    readonly afterReady: number;
    readonly closure: InitialWindowClosure;
    readonly firstEligible: boolean;
    readonly secondCompleteBeforeDeadline: boolean;
  }[] {
    return [...this.targets].map(([target, state]) => ({
      target,
      attempts: state.attempts,
      cancelled: state.cancelled,
      replacements: state.replacements,
      completed: state.completed,
      afterReady: state.afterReady,
      closure: this.closure,
      firstEligible: state.firstId !== null && this.requests.get(state.firstId)?.eligible === true,
      secondCompleteBeforeDeadline:
        state.secondSuccessful && state.secondCompletedAt !== null && state.secondCompletedAt < this.deadline,
    }));
  }

  verify(now: number): void {
    this.tick(now);
    if (this.problemCount() !== 0 || this.unresolved() !== 0 || this.pending() !== 0) {
      throw new Error(
        JSON.stringify({
          event: 'development.initial_reads_failed',
          failures: [...this.failures].map(([kind, count]) => ({ kind, count })),
          unresolved: this.unresolved(),
          pending: this.pending(),
          closure: this.closure,
        }),
      );
    }
  }
}

export class DevelopmentNavigationReads {
  private readonly reads: DevelopmentInitialReads;
  private observed = false;
  private invalid = false;
  private sealed = false;
  private readonly deadline: number;
  constructor(account: string, generation: number, deadline: number) {
    this.deadline = deadline;
    this.reads = new DevelopmentInitialReads(account, generation, deadline);
  }
  navigation(matches: boolean): void {
    if (this.sealed) return;
    if (!matches || this.observed) this.ownershipChanged('spa_scope_changed');
    else this.observed = true;
  }
  ownershipChanged(kind: OwnershipFailure): void {
    this.invalid = true;
    this.reads.ownershipChanged(kind);
  }
  begin(identity: InitialReadIdentity, now: number): void {
    this.reads.begin(identity.target === 'current_channel' ? identity : { ...identity, target: 'other' }, now);
  }
  completed(id: number, status: number, now: number): void {
    this.reads.completed(id, status, now);
  }
  failed(id: number, cancellation: boolean, now: number): void {
    this.reads.failed(id, cancellation, now);
  }
  verify(now: number): void {
    this.reads.verify(now);
    if (this.invalid || !this.observed || (!this.sealed && now >= this.deadline))
      throw new Error('development_navigation_boundary_failed');
  }
  seal(now: number): void {
    this.verify(now);
    if ((this.reads.snapshot().find((entry) => entry.target === 'current_channel')?.completed ?? 0) === 0)
      throw new Error('development_navigation_detail_incomplete');
    this.reads.sealInitial();
    this.sealed = true;
  }
  failures(): readonly { readonly kind: string; readonly count: number }[] {
    return [
      ...this.reads.failureRecords(),
      ...(!this.observed ? [{ kind: 'navigation_not_observed', count: 1 }] : []),
      ...(this.invalid ? [{ kind: 'navigation_boundary_invalidated', count: 1 }] : []),
    ];
  }
  expired(now: number): boolean {
    return now >= this.deadline;
  }
  expected(): number {
    return this.reads.expectedCancellations();
  }
  raw(): number {
    return this.reads.failedRequests();
  }
  unexpected(): number {
    return this.reads.problemCount();
  }
}
