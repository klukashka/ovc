import type { Scheduler } from '../../lib/api/session.ts';

export interface ReadView {
  readonly shownSequence: () => number;
  readonly subscribe: (changed: () => void) => () => void;
}

export interface ReadWriter {
  readonly mark: (seq: number, signal: AbortSignal) => Promise<void>;
}

interface ReadCallbacks {
  readonly read: () => void;
  readonly failed: () => void;
}

export class ReadTracker {
  private readonly scheduler: Scheduler;
  private readonly view: ReadView;
  private readonly writer: ReadWriter;
  private readonly callbacks: ReadCallbacks;
  private readonly request = new AbortController();
  private readonly unsubscribe: () => void;
  private cancelTimer: (() => void) | null = null;
  private completed = 0;
  private lastStarted = -Infinity;
  private busy = false;
  private stopped = false;
  private failed = false;

  constructor(scheduler: Scheduler, view: ReadView, writer: ReadWriter, callbacks: ReadCallbacks) {
    this.scheduler = scheduler;
    this.view = view;
    this.writer = writer;
    this.callbacks = callbacks;
    this.unsubscribe = view.subscribe(() => {
      this.refresh();
    });
    this.refresh();
  }

  refresh(): void {
    if (this.view.shownSequence() <= this.completed) {
      this.cancelTimer?.();
      this.cancelTimer = null;
      return;
    }
    if (this.stopped || this.busy || this.failed || this.cancelTimer !== null) return;
    this.cancelTimer = this.scheduler.schedule(
      Math.max(0, 1000 - (this.scheduler.wallClock() - this.lastStarted)),
      () => {
        this.cancelTimer = null;
        const seq = this.view.shownSequence();
        if (this.stopped || seq <= this.completed) return;
        this.busy = true;
        this.lastStarted = this.scheduler.wallClock();
        void this.writer
          .mark(seq, this.request.signal)
          .then(() => {
            if (this.stopped) return;
            this.completed = Math.max(this.completed, seq);
            this.busy = false;
            this.callbacks.read();
            this.refresh();
          })
          .catch(() => {
            if (this.stopped) return;
            this.busy = false;
            this.failed = true;
            this.callbacks.failed();
          });
      },
    );
  }

  retry(): void {
    this.failed = false;
    this.refresh();
  }

  stop(): void {
    this.stopped = true;
    this.cancelTimer?.();
    this.cancelTimer = null;
    this.unsubscribe();
    this.request.abort();
  }
}
