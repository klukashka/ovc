import { describe, expect, test } from 'vitest';

import { apiErrorFrom } from '../../lib/api/client.ts';
import { type ManualScheduler, manualScheduler } from '../../lib/api/fakeTransport.ts';
import type { DraftBody, ReviewSubmission, SaveOutcome, SaveRequest } from './api.ts';
import type { DraftPosition } from './draft.ts';
import { DraftSave, type DraftRefusal, draftRetryDelaysMilliseconds, draftSaveDelayMilliseconds } from './draftSave.ts';

interface PendingSave {
  readonly request: SaveRequest;
  readonly answer: (outcome: SaveOutcome) => Promise<void>;
}

function body(feedback: string): DraftBody {
  return { feedback, items: [] };
}

function submissionAt(position: DraftPosition): ReviewSubmission {
  return {
    id: 'submission-1',
    issueId: 'issue-1',
    title: 'Past simple',
    status: 'submitted',
    late: false,
    issuedBeforeReclaim: false,
    answerPolicy: 'after_check',
    firstSubmittedAt: '2026-11-01T10:00:00Z',
    maxPoints: 10,
    finalPoints: null,
    percentage: null,
    studentName: 'Anna',
    questions: [],
    answers: new Map(),
    draft: { ...position, feedback: '', items: new Map() },
    published: null,
    version: 1,
  };
}

class ReviewServer {
  readonly received: PendingSave[] = [];

  readonly send = (request: SaveRequest): Promise<SaveOutcome> =>
    new Promise((resolve) => {
      this.received.push({
        request,
        answer: async (outcome) => {
          resolve(outcome);
          await Promise.resolve();
          await Promise.resolve();
        },
      });
    });

  pending(index: number): PendingSave {
    const save = this.received[index];
    if (save === undefined) {
      throw new Error(`no save number ${String(index)} was sent`);
    }
    return save;
  }
}

interface Setup {
  readonly scheduler: ManualScheduler;
  readonly server: ReviewServer;
  readonly save: DraftSave;
  readonly saved: ReviewSubmission[];
  readonly savedAfterLeaving: ReviewSubmission[];
  readonly refused: DraftRefusal[];
}

function setup(position: DraftPosition = { revision: 1, version: 0 }): Setup {
  const scheduler = manualScheduler();
  const server = new ReviewServer();
  const saved: ReviewSubmission[] = [];
  const savedAfterLeaving: ReviewSubmission[] = [];
  const refused: DraftRefusal[] = [];
  let ids = 0;
  const save = new DraftSave({
    scheduler,
    position,
    body: body(''),
    newSaveId: (): string => {
      ids += 1;
      return `save-${String(ids)}`;
    },
    send: server.send,
  });
  save.handle({
    onSaved: (submission) => {
      saved.push(submission);
    },
    onSavedAfterLeaving: (submission) => {
      savedAfterLeaving.push(submission);
    },
    onRefused: (refusal) => {
      refused.push(refusal);
    },
  });
  return { scheduler, server, save, saved, savedAfterLeaving, refused };
}

const networkError = apiErrorFrom(0, undefined);

describe('review draft autosave', () => {
  test('the delay before a draft save is exactly 1000 milliseconds', () => {
    const { scheduler, server, save } = setup();

    save.changed(body('a'));
    scheduler.pass(999);
    const beforeOneSecond = server.received.length;
    scheduler.pass(1);

    expect(beforeOneSecond).toBe(0);
    expect(server.received).toHaveLength(1);
  });

  test('saves the latest draft one second after the last change at the draft position', () => {
    const { scheduler, server, save } = setup();
    save.changed(body('a'));
    scheduler.pass(draftSaveDelayMilliseconds - 1);
    save.changed(body('ab'));

    scheduler.pass(draftSaveDelayMilliseconds);

    expect(server.received.map((entry) => entry.request)).toEqual([
      { revision: 1, version: 0, saveId: 'save-1', body: body('ab') },
    ]);
    expect(save.snapshot()).toBe('saving');
  });

  test('a save that answers after newer typing keeps the typing and sends it next with the new version', async () => {
    const { scheduler, server, save, saved } = setup();
    save.changed(body('first'));
    scheduler.pass(draftSaveDelayMilliseconds);
    save.changed(body('first and more'));

    await server.pending(0).answer({ kind: 'saved', submission: submissionAt({ revision: 1, version: 1 }) });
    scheduler.pass(draftSaveDelayMilliseconds);

    expect(saved).toHaveLength(1);
    expect(server.pending(1).request).toEqual({
      revision: 1,
      version: 1,
      saveId: 'save-2',
      body: body('first and more'),
    });
  });

  test('a save that answers after the page was left reports the saved submission for the cache', async () => {
    const { scheduler, server, save, saved, savedAfterLeaving } = setup();
    save.changed(body('first'));
    scheduler.pass(draftSaveDelayMilliseconds);
    save.leave();
    const stored = submissionAt({ revision: 1, version: 1 });

    await server.pending(0).answer({ kind: 'saved', submission: stored });

    expect(savedAfterLeaving).toEqual([stored]);
    expect(saved).toEqual([]);
  });

  test('a failed save is retried with the same save identifier and the same body', async () => {
    const { scheduler, server, save } = setup();
    save.changed(body('kept'));
    scheduler.pass(draftSaveDelayMilliseconds);
    save.changed(body('kept, typed later'));

    await server.pending(0).answer({ kind: 'retry', retryAfterSeconds: null, error: networkError });
    const statusAfterFailure = save.snapshot();
    scheduler.pass(draftRetryDelaysMilliseconds[0] ?? 0);

    expect(statusAfterFailure).toBe('failed');
    expect(server.pending(1).request).toEqual(server.pending(0).request);
  });

  test('a rate-limited save waits for its Retry-After', async () => {
    const { scheduler, server, save } = setup();
    save.changed(body('busy'));
    scheduler.pass(draftSaveDelayMilliseconds);

    await server.pending(0).answer({ kind: 'retry', retryAfterSeconds: 7, error: networkError });
    scheduler.pass(6_999);
    const sentBeforeRetryAfter = server.received.length;
    scheduler.pass(1);

    expect(sentBeforeRetryAfter).toBe(1);
    expect(server.received).toHaveLength(2);
  });

  test('a conflict stops saving and keeps the draft unsaved', async () => {
    const { scheduler, server, save } = setup({ revision: 2, version: 3 });
    save.changed(body('mine'));
    scheduler.pass(draftSaveDelayMilliseconds);
    const flushed = save.flush();

    await server.pending(0).answer({ kind: 'conflict' });
    save.changed(body('mine, more'));
    scheduler.pass(draftSaveDelayMilliseconds * 5);

    await expect(flushed).resolves.toBe(false);
    expect(save.snapshot()).toBe('conflict');
    expect(save.unsaved).toBe(true);
    expect(server.received).toHaveLength(1);
  });

  test('a refetch showing a newer draft version stops saving with the conflict instead of overwriting it', () => {
    const { scheduler, server, save } = setup({ revision: 2, version: 3 });

    const conflicted = save.observed({ revision: 2, version: 4 });
    save.changed(body('would overwrite'));
    scheduler.pass(draftSaveDelayMilliseconds * 5);

    expect(conflicted).toBe(true);
    expect(save.snapshot()).toBe('conflict');
    expect(server.received).toHaveLength(0);
  });

  test('a refetch showing another revision stops saving with the conflict', () => {
    const { save } = setup({ revision: 3, version: 0 });

    const conflicted = save.observed({ revision: 2, version: 5 });

    expect(conflicted).toBe(true);
    expect(save.snapshot()).toBe('conflict');
  });

  test('a stale refetch with an older version of the same draft is ignored', async () => {
    const { scheduler, server, save } = setup({ revision: 2, version: 3 });
    save.changed(body('newer'));
    scheduler.pass(draftSaveDelayMilliseconds);
    await server.pending(0).answer({ kind: 'saved', submission: submissionAt({ revision: 2, version: 4 }) });

    const conflicted = save.observed({ revision: 2, version: 3 });

    expect(conflicted).toBe(false);
    expect(save.snapshot()).toBe('saved');
    expect(save.currentPosition).toEqual({ revision: 2, version: 4 });
  });

  test('a refetch while a save is in flight is not taken as a conflict', () => {
    const { scheduler, save } = setup({ revision: 2, version: 3 });
    save.changed(body('in flight'));
    scheduler.pass(draftSaveDelayMilliseconds);

    const conflicted = save.observed({ revision: 2, version: 4 });

    expect(conflicted).toBe(false);
    expect(save.snapshot()).toBe('saving');
  });

  test('a forced flush creates the draft when none exists and nothing changed', async () => {
    const { server, save } = setup({ revision: 2, version: 0 });

    const flushed = save.flush(true);
    await server.pending(0).answer({ kind: 'saved', submission: submissionAt({ revision: 2, version: 1 }) });

    await expect(flushed).resolves.toBe(true);
    expect(server.pending(0).request).toEqual({ revision: 2, version: 0, saveId: 'save-1', body: body('') });
    expect(save.currentPosition).toEqual({ revision: 2, version: 1 });
  });

  test('a forced flush sends nothing when the draft exists and nothing changed', async () => {
    const { server, save } = setup({ revision: 2, version: 4 });

    const flushed = save.flush(true);

    await expect(flushed).resolves.toBe(true);
    expect(server.received).toHaveLength(0);
  });

  test('a field refusal is reported and the next change saves again with a new identifier', async () => {
    const { scheduler, server, save, refused } = setup();
    save.changed(body('too long'));
    scheduler.pass(draftSaveDelayMilliseconds);
    const invalid = apiErrorFrom(422, {
      error: { code: 'validation_failed', message: 'invalid', request_id: 'r', fields: [] },
    });

    await server.pending(0).answer({ kind: 'invalid', error: invalid });
    const statusAfterRefusal = save.snapshot();
    save.changed(body('shorter'));
    scheduler.pass(draftSaveDelayMilliseconds);

    expect(statusAfterRefusal).toBe('refused');
    expect(refused.map((entry) => entry.kind)).toEqual(['invalid']);
    expect(server.pending(1).request.saveId).toBe('save-2');
    expect(server.pending(1).request.body).toEqual(body('shorter'));
  });

  test('a corrected file that is no longer available is reported for the page to drop it', async () => {
    const { scheduler, server, save, refused } = setup();
    save.changed(body('with a file'));
    scheduler.pass(draftSaveDelayMilliseconds);

    await server.pending(0).answer({ kind: 'fileGone', error: apiErrorFrom(404, undefined) });

    expect(refused.map((entry) => entry.kind)).toEqual(['fileGone']);
    expect(save.snapshot()).toBe('refused');
  });

  test('work that changed meanwhile stops saving', async () => {
    const { scheduler, server, save, refused } = setup();
    save.changed(body('late'));
    scheduler.pass(draftSaveDelayMilliseconds);

    await server.pending(0).answer({ kind: 'notEditable' });
    save.changed(body('later'));
    scheduler.pass(draftSaveDelayMilliseconds * 5);

    expect(refused.map((entry) => entry.kind)).toEqual(['notEditable']);
    expect(save.snapshot()).toBe('stopped');
    expect(server.received).toHaveLength(1);
  });

  test('a save that answers after the draft was reloaded is ignored', async () => {
    const { scheduler, server, save, saved } = setup({ revision: 2, version: 3 });
    save.changed(body('before publishing'));
    scheduler.pass(draftSaveDelayMilliseconds);
    save.reloaded({ revision: 3, version: 0 }, body('after publishing'));

    await server.pending(0).answer({ kind: 'conflict' });

    expect(saved).toHaveLength(0);
    expect(save.snapshot()).toBe('idle');
    expect(save.currentPosition).toEqual({ revision: 3, version: 0 });
  });

  test('storage that is full is reported for the page to drop the unbound files', async () => {
    const { scheduler, server, save, refused } = setup();
    save.changed(body('with a large file'));
    scheduler.pass(draftSaveDelayMilliseconds);

    await server.pending(0).answer({ kind: 'storageFull', error: apiErrorFrom(507, undefined) });

    expect(refused.map((entry) => entry.kind)).toEqual(['storageFull']);
    expect(save.snapshot()).toBe('refused');
  });

  test('a change while a publication runs sends nothing until the publication settles', () => {
    const { scheduler, server, save } = setup({ revision: 2, version: 3 });
    save.hold();
    save.changed(body('typed while publishing'));
    scheduler.pass(draftSaveDelayMilliseconds * 5);
    const sentWhileHeld = server.received.length;

    save.reloaded({ revision: 3, version: 0 }, body('typed while publishing'));
    save.changed(body('typed while publishing'));
    save.release();
    scheduler.pass(draftSaveDelayMilliseconds);

    expect(sentWhileHeld).toBe(0);
    expect(server.received.map((entry) => entry.request)).toEqual([
      { revision: 3, version: 0, saveId: 'save-1', body: body('typed while publishing') },
    ]);
  });
});
