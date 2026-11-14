import { asApiError } from '../../lib/api/client.ts';
import type { Scheduler } from '../../lib/api/session.ts';
import type { DraftBody, ReviewSubmission, SaveOutcome, SaveRequest } from './api.ts';
import type { DraftPosition } from './draft.ts';

export const draftSaveDelayMilliseconds = 1000;

export const draftRetryDelaysMilliseconds: readonly number[] = [1000, 2000, 4000, 8000, 16000];

export const draftSteadyRetryMilliseconds = 30000;

export type DraftSaveStatus = 'idle' | 'pending' | 'saving' | 'saved' | 'failed' | 'refused' | 'conflict' | 'stopped';

export type DraftRefusal = Exclude<SaveOutcome, { readonly kind: 'saved' | 'conflict' | 'retry' }>;

export interface DraftSaveOptions {
  readonly scheduler: Scheduler;
  readonly position: DraftPosition;
  readonly body: DraftBody;
  readonly newSaveId: () => string;
  readonly send: (request: SaveRequest) => Promise<SaveOutcome>;
}

export interface DraftSaveHandlers {
  readonly onSaved: (submission: ReviewSubmission) => void;
  readonly onRefused: (refusal: DraftRefusal) => void;
}

interface Attempt {
  readonly saveId: string;
  readonly body: DraftBody;
}

export class DraftSave {
  private readonly options: DraftSaveOptions;
  private position: DraftPosition;
  private latest: DraftBody;
  private handlers: DraftSaveHandlers = { onSaved: () => undefined, onRefused: () => undefined };
  private dirty = false;
  private forced = false;
  private retained: Attempt | null = null;
  private sending = false;
  private stopped = false;
  private leaving = false;
  private failures = 0;
  private generation = 0;
  private held = false;
  private status: DraftSaveStatus = 'idle';
  private cancelTimer: () => void = () => undefined;
  private readonly listeners = new Set<() => void>();
  private waiters: ((clean: boolean) => void)[] = [];

  constructor(options: DraftSaveOptions) {
    this.options = options;
    this.position = options.position;
    this.latest = options.body;
  }

  handle(handlers: DraftSaveHandlers): void {
    this.handlers = handlers;
  }

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return (): void => {
      this.listeners.delete(listener);
    };
  };

  readonly snapshot = (): DraftSaveStatus => this.status;

  get currentPosition(): DraftPosition {
    return this.position;
  }

  get unsaved(): boolean {
    return this.dirty || this.sending || this.retained !== null;
  }

  changed(body: DraftBody): void {
    this.latest = body;
    this.dirty = true;
    if (this.stopped || this.held || this.status === 'failed') {
      return;
    }
    if (!this.sending) {
      this.setStatus('pending');
    }
    this.arm(draftSaveDelayMilliseconds);
  }

  observed(position: DraftPosition): boolean {
    if (this.stopped || this.sending) {
      return false;
    }
    if (position.revision === this.position.revision && position.version <= this.position.version) {
      return false;
    }
    this.conflicted();
    return true;
  }

  flush(force = false): Promise<boolean> {
    if (this.stopped) {
      return Promise.resolve(false);
    }
    if (force && this.position.version === 0) {
      this.forced = true;
    }
    if (!this.unsaved && !this.forced) {
      return Promise.resolve(true);
    }
    const settled = new Promise<boolean>((resolve) => {
      this.waiters.push(resolve);
    });
    this.flushNow();
    return settled;
  }

  hold(): void {
    this.held = true;
    this.stopTimer();
  }

  release(): void {
    this.held = false;
    if (this.dirty && !this.stopped && !this.sending) {
      this.setStatus('pending');
      this.arm(draftSaveDelayMilliseconds);
    }
  }

  retryNow(): void {
    if (!this.stopped) {
      this.flushNow();
    }
  }

  leave(): void {
    if (this.stopped || this.leaving) {
      return;
    }
    if (!this.sending) {
      this.flushNow();
    }
    this.stopTimer();
    this.leaving = true;
  }

  resume(): void {
    if (!this.leaving) {
      return;
    }
    this.leaving = false;
    if (!this.stopped && !this.sending && this.unsaved) {
      this.arm(draftSaveDelayMilliseconds);
    }
  }

  reloaded(position: DraftPosition, body: DraftBody): void {
    this.stopTimer();
    this.generation += 1;
    this.sending = false;
    this.position = position;
    this.latest = body;
    this.dirty = false;
    this.forced = false;
    this.retained = null;
    this.failures = 0;
    this.stopped = false;
    this.setStatus('idle');
  }

  conflicted(): void {
    this.stopTimer();
    this.stopped = true;
    this.resolveWaiters(false);
    this.setStatus('conflict');
  }

  stop(): void {
    this.stopTimer();
    this.stopped = true;
    this.resolveWaiters(false);
    this.setStatus('stopped');
  }

  private arm(milliseconds: number): void {
    this.stopTimer();
    this.cancelTimer = this.options.scheduler.schedule(milliseconds, () => {
      this.cancelTimer = (): void => undefined;
      this.flushNow();
    });
  }

  private stopTimer(): void {
    this.cancelTimer();
    this.cancelTimer = (): void => undefined;
  }

  private flushNow(): void {
    this.stopTimer();
    if (this.sending || this.stopped) {
      return;
    }
    const attempt = this.nextAttempt();
    if (attempt === null) {
      this.resolveWaiters(true);
      return;
    }
    this.send(attempt);
  }

  private nextAttempt(): Attempt | null {
    if (this.retained !== null) {
      return this.retained;
    }
    if (!this.dirty && !this.forced) {
      return null;
    }
    this.dirty = false;
    this.forced = false;
    return { saveId: this.options.newSaveId(), body: this.latest };
  }

  private send(attempt: Attempt): void {
    this.sending = true;
    this.retained = attempt;
    this.setStatus('saving');
    const request: SaveRequest = { ...this.position, saveId: attempt.saveId, body: attempt.body };
    const generation = this.generation;
    void this.options.send(request).then(
      (outcome) => {
        if (generation === this.generation) {
          this.settle(outcome);
        }
      },
      (error: unknown) => {
        if (generation === this.generation) {
          this.settle({ kind: 'retry', retryAfterSeconds: null, error: asApiError(error) });
        }
      },
    );
  }

  private adoptSaved(submission: ReviewSubmission): void {
    const draft = submission.draft;
    if (draft !== null) {
      this.position = { revision: draft.revision, version: draft.version };
    }
  }

  private settle(outcome: SaveOutcome): void {
    this.sending = false;
    if (this.leaving && !this.stopped) {
      this.retained = outcome.kind === 'retry' ? this.retained : null;
      if (outcome.kind === 'saved') {
        this.adoptSaved(outcome.submission);
        const next = this.nextAttempt();
        if (next !== null) {
          this.send(next);
        }
      }
      return;
    }
    if (this.stopped) {
      return;
    }
    if (outcome.kind === 'retry') {
      this.failures += 1;
      this.setStatus('failed');
      this.resolveWaiters(false);
      const backoff = draftRetryDelaysMilliseconds[this.failures - 1] ?? draftSteadyRetryMilliseconds;
      this.arm(outcome.retryAfterSeconds === null ? backoff : outcome.retryAfterSeconds * 1000);
      return;
    }
    this.retained = null;
    this.failures = 0;
    switch (outcome.kind) {
      case 'saved':
        this.adoptSaved(outcome.submission);
        this.handlers.onSaved(outcome.submission);
        if (!this.dirty) {
          this.setStatus('saved');
          this.resolveWaiters(true);
          return;
        }
        if (this.waiters.length > 0) {
          this.flushNow();
          return;
        }
        this.setStatus('pending');
        this.arm(draftSaveDelayMilliseconds);
        return;
      case 'conflict':
        this.dirty = true;
        this.conflicted();
        return;
      case 'notEditable':
        this.stop();
        this.handlers.onRefused(outcome);
        return;
      case 'fileGone':
      case 'storageFull':
      case 'unconfirmed':
      case 'invalid':
      case 'refused':
        this.dirty = true;
        this.stopTimer();
        this.setStatus('refused');
        this.resolveWaiters(false);
        this.handlers.onRefused(outcome);
    }
  }

  private resolveWaiters(clean: boolean): void {
    const waiting = this.waiters;
    this.waiters = [];
    for (const resolve of waiting) {
      resolve(clean);
    }
  }

  private setStatus(status: DraftSaveStatus): void {
    if (this.status === status) {
      return;
    }
    this.status = status;
    for (const listener of this.listeners) {
      listener();
    }
  }
}
