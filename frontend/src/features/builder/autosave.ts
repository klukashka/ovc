import { type ApiError, asApiError } from '../../lib/api/client.ts';
import type { Scheduler } from '../../lib/api/session.ts';
import type { components } from '../../lib/api/schema.ts';
import type { TemplateBlockInput } from './blocks.ts';

export type Template = components['schemas']['Template'];

export const autosaveDelayMilliseconds = 1000;

const defaultRetryAfterSeconds = 1;

export interface SaveBody {
  readonly title: string;
  readonly blocks: readonly TemplateBlockInput[];
}

export interface SaveRequest extends SaveBody {
  readonly version: number;
  readonly saveId: string;
}

export type RefusalReason = 'tooLarge' | 'storageLimit' | 'notFound' | 'other';

export type SaveOutcome =
  | { readonly kind: 'saved'; readonly template: Template }
  | { readonly kind: 'conflict' }
  | { readonly kind: 'refused'; readonly reason: RefusalReason; readonly error: ApiError }
  | { readonly kind: 'rateLimited'; readonly retryAfterSeconds: number | null; readonly error: ApiError }
  | { readonly kind: 'uncertain'; readonly error: ApiError };

export type AutosaveStatus =
  | { readonly kind: 'saved' }
  | { readonly kind: 'editing' }
  | { readonly kind: 'saving' }
  | { readonly kind: 'blankTitle' }
  | { readonly kind: 'conflict' }
  | { readonly kind: 'failed'; readonly reason: RefusalReason | 'rateLimited' | 'uncertain'; readonly error: ApiError };

export interface AutosaveOptions {
  readonly scheduler: Scheduler;
  readonly version: number;
  readonly newSaveId: () => string;
  readonly send: (request: SaveRequest) => Promise<SaveOutcome>;
  readonly onSaved: (template: Template) => void;
}

interface Attempt {
  readonly body: SaveBody;
  readonly saveId: string;
}

function blank(title: string): boolean {
  return title.trim() === '';
}

export class TemplateAutosave {
  private readonly options: AutosaveOptions;
  private version: number;
  private latest: SaveBody | null = null;
  private dirty = false;
  private sending = false;
  private retained: Attempt | null = null;
  private stopped = false;
  private waitUntil = 0;
  private status: AutosaveStatus = { kind: 'saved' };
  private cancelTimer: () => void = () => undefined;
  private timerArmed = false;
  private readonly listeners = new Set<() => void>();
  private waiters: ((clean: boolean) => void)[] = [];

  constructor(options: AutosaveOptions) {
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
    return this.dirty || this.sending || this.retained !== null;
  }

  changed(body: SaveBody): void {
    if (this.stopped) {
      this.latest = body;
      return;
    }
    this.latest = body;
    this.dirty = true;
    if (blank(body.title)) {
      this.stopTimer();
      if (!this.sending) {
        this.setStatus({ kind: 'blankTitle' });
      }
      this.resolveWaiters(false);
      return;
    }
    if (this.status.kind !== 'saving' && this.status.kind !== 'failed') {
      this.setStatus({ kind: 'editing' });
    }
    this.arm(this.delayFrom(autosaveDelayMilliseconds));
  }

  retry(): void {
    if (this.stopped) {
      return;
    }
    if (this.latest !== null) {
      this.dirty = true;
    }
    const wait = this.delayFrom(0);
    if (wait > 0) {
      this.arm(wait);
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
    if (this.latest !== null && blank(this.latest.title) && this.retained === null) {
      return Promise.resolve(false);
    }
    const settled = new Promise<boolean>((resolve) => {
      this.waiters.push(resolve);
    });
    const wait = this.delayFrom(0);
    if (wait > 0) {
      this.arm(wait);
    } else {
      this.flushNow();
    }
    return settled;
  }

  reloaded(version: number): void {
    this.stopTimer();
    this.version = version;
    this.latest = null;
    this.dirty = false;
    this.retained = null;
    this.stopped = false;
    this.setStatus({ kind: 'saved' });
  }

  leave(): void {
    if (!this.stopped && this.unsaved) {
      this.flushNow();
    }
  }

  private delayFrom(milliseconds: number): number {
    return Math.max(milliseconds, this.waitUntil - this.options.scheduler.wallClock());
  }

  private arm(milliseconds: number): void {
    this.stopTimer();
    this.timerArmed = true;
    this.cancelTimer = this.options.scheduler.schedule(milliseconds, () => {
      this.timerArmed = false;
      this.cancelTimer = (): void => undefined;
      this.flushNow();
    });
  }

  private stopTimer(): void {
    this.cancelTimer();
    this.timerArmed = false;
    this.cancelTimer = (): void => undefined;
  }

  private flushNow(): void {
    this.stopTimer();
    if (this.sending) {
      return;
    }
    const attempt = this.nextAttempt();
    if (attempt === null) {
      if (this.latest !== null && this.dirty && blank(this.latest.title)) {
        this.setStatus({ kind: 'blankTitle' });
        this.resolveWaiters(false);
      }
      return;
    }
    this.send(attempt);
  }

  private nextAttempt(): Attempt | null {
    if (this.retained !== null) {
      return this.retained;
    }
    if (!this.dirty || this.latest === null || blank(this.latest.title)) {
      return null;
    }
    this.dirty = false;
    return { body: this.latest, saveId: this.options.newSaveId() };
  }

  private send(attempt: Attempt): void {
    this.sending = true;
    this.retained = attempt;
    this.setStatus({ kind: 'saving' });
    void this.options.send({ ...attempt.body, version: this.version, saveId: attempt.saveId }).then(
      (outcome) => {
        this.settle(outcome);
      },
      (error: unknown) => {
        this.settle({ kind: 'uncertain', error: asApiError(error) });
      },
    );
  }

  private settle(outcome: SaveOutcome): void {
    this.sending = false;
    if (this.stopped) {
      return;
    }
    if (outcome.kind === 'uncertain') {
      this.setStatus({ kind: 'failed', reason: 'uncertain', error: outcome.error });
      this.resolveWaiters(false);
      return;
    }
    this.retained = null;
    switch (outcome.kind) {
      case 'saved':
        this.version = outcome.template.version;
        this.options.onSaved(outcome.template);
        this.afterSaved();
        return;
      case 'conflict':
        this.stopTimer();
        this.stopped = true;
        this.dirty = true;
        this.setStatus({ kind: 'conflict' });
        this.resolveWaiters(false);
        return;
      case 'rateLimited':
        this.dirty = true;
        this.setStatus({ kind: 'failed', reason: 'rateLimited', error: outcome.error });
        this.resolveWaiters(false);
        this.waitUntil =
          this.options.scheduler.wallClock() + (outcome.retryAfterSeconds ?? defaultRetryAfterSeconds) * 1000;
        this.arm(this.delayFrom(0));
        return;
      case 'refused':
        this.dirty = true;
        this.setStatus({ kind: 'failed', reason: outcome.reason, error: outcome.error });
        this.resolveWaiters(false);
        return;
    }
  }

  private afterSaved(): void {
    if (!this.dirty) {
      this.setStatus({ kind: 'saved' });
      this.resolveWaiters(true);
      return;
    }
    if (this.latest !== null && blank(this.latest.title)) {
      this.setStatus({ kind: 'blankTitle' });
      this.resolveWaiters(false);
      return;
    }
    if (this.timerArmed && this.waiters.length === 0) {
      this.setStatus({ kind: 'editing' });
      return;
    }
    this.flushNow();
  }

  private resolveWaiters(clean: boolean): void {
    const waiting = this.waiters;
    this.waiters = [];
    for (const resolve of waiting) {
      resolve(clean);
    }
  }

  private setStatus(status: AutosaveStatus): void {
    this.status = status;
    for (const listener of this.listeners) {
      listener();
    }
  }
}
