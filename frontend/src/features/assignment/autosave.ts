import { asApiError } from '../../lib/api/client.ts';
import type { Scheduler } from '../../lib/api/session.ts';
import type { SaveOutcome, SaveRequest, SentAnswer, Submission } from './api.ts';

export const autosaveDelayMilliseconds = 1000;

export const retryDelaysMilliseconds: readonly number[] = [1000, 2000, 4000, 8000, 16000];

export const steadyRetryMilliseconds = 30000;

export type AutosaveStatus =
  | { readonly kind: 'idle' }
  | { readonly kind: 'pending' }
  | { readonly kind: 'saving' }
  | { readonly kind: 'saved' }
  | { readonly kind: 'failed' }
  | { readonly kind: 'conflict' }
  | { readonly kind: 'stopped' };

export type Refusal = Extract<SaveOutcome, { readonly kind: 'changed' | 'gone' | 'invalid' | 'storageFull' }>;

export interface AnswerAutosaveOptions {
  readonly scheduler: Scheduler;
  readonly version: number;
  readonly newSaveId: () => string;
  readonly collect: (questions: readonly string[]) => readonly SentAnswer[];
  readonly send: (request: SaveRequest) => Promise<SaveOutcome>;
  readonly onSaved: (submission: Submission, sent: readonly SentAnswer[]) => void;
  readonly onSavedAfterLeaving: (submission: Submission) => void;
  readonly onRefused: (refusal: Refusal, sent: readonly SentAnswer[]) => void;
}

interface Attempt {
  readonly answers: readonly SentAnswer[];
  readonly saveId: string;
}

export class AnswerAutosave {
  private readonly options: AnswerAutosaveOptions;
  private version: number;
  private readonly dirty = new Set<string>();
  private retained: Attempt | null = null;
  private sending = false;
  private stopped = false;
  private leaving = false;
  private failures = 0;
  private status: AutosaveStatus = { kind: 'idle' };
  private cancelTimer: () => void = () => undefined;
  private readonly listeners = new Set<() => void>();
  private waiters: ((clean: boolean) => void)[] = [];

  constructor(options: AnswerAutosaveOptions) {
    this.options = options;
    this.version = options.version;
  }

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return (): void => {
      this.listeners.delete(listener);
    };
  };

  readonly snapshot = (): AutosaveStatus => this.status;

  get unsaved(): boolean {
    return this.dirty.size > 0 || this.sending || this.retained !== null;
  }

  get currentVersion(): number {
    return this.version;
  }

  changed(question: string): void {
    this.dirty.add(question);
    if (this.stopped) {
      return;
    }
    if (this.status.kind === 'failed') {
      return;
    }
    if (!this.sending) {
      this.setStatus({ kind: 'pending' });
    }
    this.arm(autosaveDelayMilliseconds);
  }

  forget(questions: readonly string[]): void {
    for (const question of questions) {
      this.dirty.delete(question);
    }
    if (this.stopped || this.sending || this.leaving || this.status.kind === 'failed') {
      return;
    }
    if (this.dirty.size > 0) {
      this.setStatus({ kind: 'pending' });
      this.arm(autosaveDelayMilliseconds);
      return;
    }
    if (this.retained === null) {
      this.stopTimer();
      this.setStatus({ kind: 'saved' });
    }
  }

  adopt(version: number): boolean {
    if (this.unsaved || this.stopped || version <= this.version) {
      return false;
    }
    this.version = version;
    return true;
  }

  retryNow(): void {
    if (this.stopped) {
      return;
    }
    this.flushNow();
  }

  flush(): Promise<boolean> {
    if (this.stopped) {
      return Promise.resolve(false);
    }
    if (!this.unsaved) {
      return Promise.resolve(true);
    }
    const settled = new Promise<boolean>((resolve) => {
      this.waiters.push(resolve);
    });
    this.flushNow();
    return settled;
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
      this.arm(autosaveDelayMilliseconds);
    }
  }

  drop(): void {
    this.stop();
    this.dirty.clear();
    this.retained = null;
  }

  reloaded(version: number): void {
    this.stopTimer();
    this.version = version;
    this.dirty.clear();
    this.retained = null;
    this.failures = 0;
    this.stopped = false;
    this.setStatus({ kind: 'idle' });
  }

  conflicted(): void {
    this.stopTimer();
    this.stopped = true;
    this.resolveWaiters(false);
    this.setStatus({ kind: 'conflict' });
  }

  stop(): void {
    this.stopTimer();
    this.stopped = true;
    this.resolveWaiters(false);
    this.setStatus({ kind: 'stopped' });
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
    if (this.dirty.size === 0) {
      return null;
    }
    const questions = [...this.dirty];
    this.dirty.clear();
    return { answers: this.options.collect(questions), saveId: this.options.newSaveId() };
  }

  private send(attempt: Attempt): void {
    this.sending = true;
    this.retained = attempt;
    this.setStatus({ kind: 'saving' });
    void this.options.send({ version: this.version, saveId: attempt.saveId, answers: attempt.answers }).then(
      (outcome) => {
        this.settle(outcome, attempt);
      },
      (error: unknown) => {
        this.settle({ kind: 'retry', retryAfterSeconds: null, error: asApiError(error) }, attempt);
      },
    );
  }

  private settle(outcome: SaveOutcome, attempt: Attempt): void {
    this.sending = false;
    if (this.leaving && !this.stopped) {
      this.retained = null;
      const next = outcome.kind === 'saved' ? this.nextAttempt() : null;
      if (outcome.kind === 'saved') {
        this.version = outcome.submission.version;
        this.options.onSavedAfterLeaving(outcome.submission);
      }
      if (next !== null) {
        this.send(next);
      }
      return;
    }
    if (this.stopped) {
      return;
    }
    if (outcome.kind === 'retry') {
      this.failures += 1;
      this.setStatus({ kind: 'failed' });
      this.resolveWaiters(false);
      const backoff = retryDelaysMilliseconds[this.failures - 1] ?? steadyRetryMilliseconds;
      this.arm(outcome.retryAfterSeconds === null ? backoff : outcome.retryAfterSeconds * 1000);
      return;
    }
    this.retained = null;
    this.failures = 0;
    switch (outcome.kind) {
      case 'saved':
        this.version = outcome.submission.version;
        this.options.onSaved(outcome.submission, attempt.answers);
        if (this.dirty.size === 0) {
          this.setStatus({ kind: 'saved' });
          this.resolveWaiters(true);
          return;
        }
        if (this.waiters.length > 0) {
          this.flushNow();
          return;
        }
        this.setStatus({ kind: 'pending' });
        this.arm(autosaveDelayMilliseconds);
        return;
      case 'conflict':
        for (const sent of attempt.answers) {
          this.dirty.add(sent.block.id);
        }
        this.stopTimer();
        this.stopped = true;
        this.setStatus({ kind: 'conflict' });
        this.resolveWaiters(false);
        return;
      case 'invalid':
      case 'storageFull':
      case 'gone': {
        const typedMeanwhile = this.dirty.size > 0;
        for (const sent of attempt.answers) {
          this.dirty.add(sent.block.id);
        }
        this.stopTimer();
        this.setStatus({ kind: 'idle' });
        this.resolveWaiters(false);
        this.options.onRefused(outcome, attempt.answers);
        if (typedMeanwhile) {
          this.setStatus({ kind: 'pending' });
          this.arm(autosaveDelayMilliseconds);
        }
        return;
      }
      case 'changed':
        this.stop();
        this.options.onRefused(outcome, attempt.answers);
        return;
    }
  }

  private resolveWaiters(clean: boolean): void {
    const waiting = this.waiters;
    this.waiters = [];
    for (const resolve of waiting) {
      resolve(clean);
    }
  }

  private setStatus(status: AutosaveStatus): void {
    if (this.status.kind === status.kind) {
      return;
    }
    this.status = status;
    for (const listener of this.listeners) {
      listener();
    }
  }
}
