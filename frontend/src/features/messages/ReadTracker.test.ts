import { describe, expect, test } from 'vitest';

import { manualScheduler } from '../../lib/api/fakeTransport.ts';
import { ReadTracker, type ReadView, type ReadWriter } from './ReadTracker.ts';

class View implements ReadView {
  private changed: (() => void) | null = null;
  private seq = 0;
  private visible = true;
  readonly shownSequence = (): number => (this.visible ? this.seq : 0);
  readonly subscribe = (changed: () => void): (() => void) => {
    this.changed = changed;
    return (): void => {
      this.changed = null;
    };
  };
  show(seq: number): void {
    this.seq = seq;
    this.changed?.();
  }
  visibility(visible: boolean): void {
    this.visible = visible;
    this.changed?.();
  }
}

class Writer implements ReadWriter {
  readonly sent: number[] = [];
  readonly signals: AbortSignal[] = [];
  private complete: (() => void) | null = null;
  private refuse: (() => void) | null = null;
  readonly mark = (seq: number, signal: AbortSignal): Promise<void> => {
    this.sent.push(seq);
    this.signals.push(signal);
    return new Promise((resolve, reject) => {
      this.complete = resolve;
      this.refuse = (): void => {
        reject(new Error('Read unavailable'));
      };
    });
  };
  async finish(fail = false): Promise<void> {
    if (fail) this.refuse?.();
    else this.complete?.();
    await Promise.resolve();
    await Promise.resolve();
  }
}

function fixture(): {
  readonly clock: ReturnType<typeof manualScheduler>;
  readonly view: View;
  readonly writer: Writer;
  readonly tracker: ReadTracker;
  readonly outcomes: string[];
} {
  const clock = manualScheduler();
  const view = new View();
  const writer = new Writer();
  const outcomes: string[] = [];
  const tracker = new ReadTracker(clock, view, writer, {
    read: (): void => {
      outcomes.push('read');
    },
    failed: (): void => {
      outcomes.push('failed');
    },
  });
  return { clock, view, writer, tracker, outcomes };
}

describe('read marker lifecycle', () => {
  test('coalesces visible sequences, moves forward and waits one second between writes', async () => {
    const { clock, view, writer, tracker } = fixture();

    view.show(2);
    view.show(4);
    clock.pass(0);
    await writer.finish();
    view.show(7);
    clock.pass(999);
    const beforeSecond = [...writer.sent];
    clock.pass(1);
    await writer.finish();
    view.show(3);
    clock.pass(5000);
    tracker.stop();

    expect(beforeSecond).toEqual([4]);
    expect(writer.sent).toEqual([4, 7]);
    expect(clock.pendingDelays()).toEqual([]);
  });

  test('hidden pages cancel queued markers and returning schedules only currently shown content', async () => {
    const { clock, view, writer, tracker } = fixture();

    view.show(1);
    view.visibility(false);
    clock.pass(5000);
    const whileHidden = [...writer.sent];
    view.show(20);
    view.visibility(true);
    clock.pass(0);
    await writer.finish();
    tracker.stop();

    expect(whileHidden).toEqual([]);
    expect(writer.sent).toEqual([20]);
  });

  test('stop aborts the old writer and ignores its late response after a new tracker starts', async () => {
    const old = fixture();
    old.view.show(5);
    old.clock.pass(0);

    old.tracker.stop();
    const next = fixture();
    next.view.show(2);
    next.clock.pass(0);
    await old.writer.finish();
    await next.writer.finish();
    next.tracker.stop();

    expect(old.writer.signals[0]?.aborted).toBe(true);
    expect(old.outcomes).toEqual([]);
    expect(next.outcomes).toEqual(['read']);
    expect(next.writer.sent).toEqual([2]);
  });

  test('a failed write is visible and retries explicitly without a background loop', async () => {
    const { clock, view, writer, tracker, outcomes } = fixture();
    view.show(3);
    clock.pass(0);
    await writer.finish(true);

    view.show(5);
    clock.pass(5000);
    const beforeRetry = [...writer.sent];
    tracker.retry();
    clock.pass(0);
    await writer.finish();
    tracker.stop();

    expect(beforeRetry).toEqual([3]);
    expect(writer.sent).toEqual([3, 5]);
    expect(outcomes).toEqual(['failed', 'read']);
  });

  test('stop removes a pending timer before an old account can write', () => {
    const { clock, view, writer, tracker } = fixture();
    view.show(3);

    tracker.stop();
    view.show(9);
    clock.pass(5000);

    expect(writer.sent).toEqual([]);
    expect(clock.pendingDelays()).toEqual([]);
  });
});
