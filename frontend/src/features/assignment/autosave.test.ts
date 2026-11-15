import { describe, expect, test } from 'vitest';

import { apiErrorFrom } from '../../lib/api/client.ts';
import { type ManualScheduler, manualScheduler } from '../../lib/api/fakeTransport.ts';
import type { QuestionBlock, SaveOutcome, SaveRequest, SentAnswer, Submission } from './api.ts';
import {
  AnswerAutosave,
  type Refusal,
  autosaveDelayMilliseconds,
  retryDelaysMilliseconds,
  steadyRetryMilliseconds,
} from './autosave.ts';

interface PendingSave {
  readonly request: SaveRequest;
  readonly answer: (outcome: SaveOutcome) => Promise<void>;
}

const firstQuestion: QuestionBlock = { type: 'short_text', id: 'question-1', prompt: 'Go', images: [] };
const secondQuestion: QuestionBlock = { type: 'short_text', id: 'question-2', prompt: 'See', images: [] };
const blocks: readonly QuestionBlock[] = [firstQuestion, secondQuestion];

class AnswerServer {
  readonly received: PendingSave[] = [];
  private version: number;
  private lastSaveId: string | null = null;

  constructor(version: number) {
    this.version = version;
  }

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

  apply(request: SaveRequest): SaveOutcome {
    if (request.saveId === this.lastSaveId) {
      return { kind: 'saved', submission: this.submission() };
    }
    if (request.version !== this.version) {
      return { kind: 'conflict' };
    }
    this.version += 1;
    this.lastSaveId = request.saveId;
    return { kind: 'saved', submission: this.submission() };
  }

  private submission(): Submission {
    return {
      id: 'submission-1',
      title: 'Past simple',
      tutor: { name: 'Maria', colour: null, relationshipId: 'relationship-1' },
      status: 'draft',
      deadlineAt: null,
      late: false,
      issuedBeforeReclaim: false,
      version: this.version,
      editable: new Set(blocks.map((block) => block.id)),
      blocks,
      answers: new Map(),
      review: null,
      revealed: null,
    };
  }
}

const networkError = apiErrorFrom(0, undefined);

interface Setup {
  readonly scheduler: ManualScheduler;
  readonly server: AnswerServer;
  readonly autosave: AnswerAutosave;
  readonly values: Map<string, string>;
  readonly saved: (readonly SentAnswer[])[];
  readonly savedAfterLeaving: number[];
  readonly refused: Refusal[];
}

function setUp(version = 1): Setup {
  const scheduler = manualScheduler();
  const server = new AnswerServer(version);
  const values = new Map<string, string>();
  const saved: (readonly SentAnswer[])[] = [];
  const savedAfterLeaving: number[] = [];
  const refused: Refusal[] = [];
  let counter = 0;
  const autosave = new AnswerAutosave({
    scheduler,
    version,
    newSaveId: (): string => {
      counter += 1;
      return `save-${String(counter)}`;
    },
    collect: (questions): readonly SentAnswer[] =>
      blocks
        .filter((block) => questions.includes(block.id))
        .map((block) => ({ block, answer: { optionIds: [], text: values.get(block.id) ?? '', files: [] } })),
    send: server.send,
    onSaved: (_submission, sent): void => {
      saved.push(sent);
    },
    onSavedAfterLeaving: (submission): void => {
      savedAfterLeaving.push(submission.version);
    },
    onRefused: (refusal): void => {
      refused.push(refusal);
    },
  });
  return { scheduler, server, autosave, values, saved, savedAfterLeaving, refused };
}

function type(setup: Setup, question: QuestionBlock, text: string): void {
  setup.values.set(question.id, text);
  setup.autosave.changed(question.id);
}

function sentTexts(save: PendingSave): readonly string[] {
  return save.request.answers.map((sent) => sent.answer.text);
}

describe('answer autosave', () => {
  test('one second after the last change it sends only the changed answers', () => {
    const setup = setUp();

    type(setup, firstQuestion, 'w');
    setup.scheduler.pass(autosaveDelayMilliseconds - 1);
    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds - 1);
    const beforeDelay = setup.server.received.length;
    setup.scheduler.pass(1);

    expect(beforeDelay).toBe(0);
    expect(setup.server.received.map(sentTexts)).toEqual([['went']]);
    expect(setup.autosave.snapshot()).toEqual({ kind: 'saving' });
  });

  test('a save answering after newer typing leaves that typing unsaved and sends it next with the new version', async () => {
    const setup = setUp(4);
    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds);
    type(setup, firstQuestion, 'went away');
    setup.scheduler.pass(autosaveDelayMilliseconds);
    const inFlightWhileTyping = setup.server.received.length;

    const first = setup.server.pending(0);
    await first.answer(setup.server.apply(first.request));
    const unsavedAfterFirst = setup.autosave.unsaved;
    setup.scheduler.pass(autosaveDelayMilliseconds);

    expect(inFlightWhileTyping).toBe(1);
    expect(unsavedAfterFirst).toBe(true);
    expect(setup.server.received.map(sentTexts)).toEqual([['went'], ['went away']]);
    expect(setup.server.pending(1).request.version).toBe(5);
  });

  test('a failed save keeps the same save identifier and retries after 1, 2, 4, 8, 16 and then 30 seconds', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds);
    const delays: number[] = [];

    for (let attempt = 0; attempt < 7; attempt += 1) {
      await setup.server.pending(attempt).answer({ kind: 'retry', retryAfterSeconds: null, error: networkError });
      delays.push(setup.scheduler.pendingDelays()[0] ?? -1);
      setup.scheduler.pass(delays.at(-1) ?? 0);
    }

    expect(delays).toEqual([...retryDelaysMilliseconds, steadyRetryMilliseconds, steadyRetryMilliseconds]);
    expect(new Set(setup.server.received.map((save) => save.request.saveId))).toEqual(new Set(['save-1']));
    expect(setup.autosave.snapshot()).toEqual({ kind: 'saving' });
  });

  test('a rate-limited save waits for its Retry-After', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds);

    await setup.server.pending(0).answer({ kind: 'retry', retryAfterSeconds: 7, error: networkError });

    expect(setup.autosave.snapshot()).toEqual({ kind: 'failed' });
    expect(setup.scheduler.pendingDelays()).toEqual([7000]);
  });

  test('trying now sends the retained request at once, and a replay of it answers the current state', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds);
    const first = setup.server.pending(0);
    setup.server.apply(first.request);
    await first.answer({ kind: 'retry', retryAfterSeconds: null, error: networkError });

    setup.autosave.retryNow();
    const replay = setup.server.pending(1);
    await replay.answer(setup.server.apply(replay.request));

    expect(replay.request.saveId).toBe(first.request.saveId);
    expect(setup.autosave.snapshot()).toEqual({ kind: 'saved' });
    expect(setup.autosave.currentVersion).toBe(2);
  });

  test('a conflict stops saving until the answers are reloaded, and keeps the sent answers unsaved', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds);
    await setup.server.pending(0).answer({ kind: 'conflict' });

    type(setup, secondQuestion, 'saw');
    setup.scheduler.pass(autosaveDelayMilliseconds * 5);
    const sentWhileStopped = setup.server.received.length;
    const unsavedWhileStopped = setup.autosave.unsaved;
    setup.autosave.reloaded(9);
    type(setup, secondQuestion, 'saw');
    setup.scheduler.pass(autosaveDelayMilliseconds);

    expect(sentWhileStopped).toBe(1);
    expect(unsavedWhileStopped).toBe(true);
    expect(setup.server.pending(1).request.version).toBe(9);
    expect(sentTexts(setup.server.pending(1))).toEqual(['saw']);
  });

  test('a refetched version is adopted only while no answer is unsaved', async () => {
    const setup = setUp(3);
    type(setup, firstQuestion, 'went');

    const adoptedWhileUnsaved = setup.autosave.adopt(6);
    setup.scheduler.pass(autosaveDelayMilliseconds);
    const save = setup.server.pending(0);
    await save.answer(setup.server.apply(save.request));
    const adoptedWhenClean = setup.autosave.adopt(6);

    expect(adoptedWhileUnsaved).toBe(false);
    expect(save.request.version).toBe(3);
    expect(adoptedWhenClean).toBe(true);
    expect(setup.autosave.currentVersion).toBe(6);
  });

  test('a flush sends the pending answers at once and resolves once they are saved', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');

    const flushed = setup.autosave.flush();
    const save = setup.server.pending(0);
    await save.answer(setup.server.apply(save.request));

    expect(await flushed).toBe(true);
  });

  test('a flush resolves false when the save fails', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');

    const flushed = setup.autosave.flush();
    await setup.server.pending(0).answer({ kind: 'retry', retryAfterSeconds: null, error: networkError });

    expect(await flushed).toBe(false);
  });

  test('a refusal that changed the work stops saving and reports it', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds);

    await setup.server.pending(0).answer({ kind: 'changed' });
    type(setup, secondQuestion, 'saw');
    setup.scheduler.pass(autosaveDelayMilliseconds);

    expect(setup.refused.map((refusal) => refusal.kind)).toEqual(['changed']);
    expect(setup.server.received).toHaveLength(1);
    expect(setup.autosave.snapshot()).toEqual({ kind: 'stopped' });
  });

  test('an invalid answer is reported and sent again only after the next change', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds);

    await setup.server
      .pending(0)
      .answer({ kind: 'invalid', questions: [firstQuestion.id], error: apiErrorFrom(422, undefined) });
    setup.scheduler.pass(autosaveDelayMilliseconds * 3);
    const sentBeforeChange = setup.server.received.length;
    type(setup, firstQuestion, 'gone');
    setup.scheduler.pass(autosaveDelayMilliseconds);

    expect(setup.refused.map((refusal) => refusal.kind)).toEqual(['invalid']);
    expect(sentBeforeChange).toBe(1);
    expect(sentTexts(setup.server.pending(1))).toEqual(['gone']);
  });

  test('leaving sends the pending answers, and answers typed during a save follow it', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds);
    type(setup, secondQuestion, 'saw');

    setup.autosave.leave();
    const first = setup.server.pending(0);
    await first.answer(setup.server.apply(first.request));

    expect(setup.server.received.map(sentTexts)).toEqual([['went'], ['saw']]);
    expect(setup.server.pending(1).request.version).toBe(2);
  });

  test('every save answering after the page was left reports the saved submission for the cache', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds);
    type(setup, secondQuestion, 'saw');
    setup.autosave.leave();
    const first = setup.server.pending(0);
    await first.answer(setup.server.apply(first.request));

    const second = setup.server.pending(1);
    await second.answer(setup.server.apply(second.request));

    expect(setup.savedAfterLeaving).toEqual([2, 3]);
    expect(setup.saved).toEqual([]);
  });

  test('leaving and resuming at once, as React does when it remounts in development, keeps saving', () => {
    const setup = setUp();
    setup.autosave.leave();
    setup.autosave.resume();

    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds);

    expect(setup.server.received.map(sentTexts)).toEqual([['went']]);
  });

  test('a refetched version lower than the saved one is not adopted', async () => {
    const setup = setUp(3);
    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds);
    const save = setup.server.pending(0);
    await save.answer(setup.server.apply(save.request));

    const adopted = setup.autosave.adopt(3);

    expect(adopted).toBe(false);
    expect(setup.autosave.currentVersion).toBe(4);
  });

  test('a save refused as not found keeps the answers unsaved and saving resumes on the next change', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds);

    await setup.server.pending(0).answer({ kind: 'gone' });
    type(setup, secondQuestion, 'saw');
    setup.scheduler.pass(autosaveDelayMilliseconds);

    expect(setup.refused.map((refusal) => refusal.kind)).toEqual(['gone']);
    expect(setup.server.received.map(sentTexts)).toEqual([['went'], ['went', 'saw']]);
  });

  test('answers typed while a refused save was in flight are sent without waiting for another change', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds);
    type(setup, secondQuestion, 'saw');

    await setup.server
      .pending(0)
      .answer({ kind: 'invalid', questions: [firstQuestion.id], error: apiErrorFrom(422, undefined) });
    setup.scheduler.pass(autosaveDelayMilliseconds);

    expect(setup.server.received).toHaveLength(2);
    expect(sentTexts(setup.server.pending(1))).toContain('saw');
  });

  test('dropped answers are not sent when the page is left', () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');

    setup.autosave.drop();
    setup.autosave.leave();
    setup.scheduler.pass(autosaveDelayMilliseconds);

    expect(setup.server.received).toHaveLength(0);
  });

  test('forgetting refused answers that returned to their saved value reports the work saved without a request', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');
    setup.scheduler.pass(autosaveDelayMilliseconds);
    await setup.server.pending(0).answer({ kind: 'gone' });

    setup.autosave.forget([firstQuestion.id]);
    setup.scheduler.pass(autosaveDelayMilliseconds * 3);

    expect(setup.server.received).toHaveLength(1);
    expect(setup.autosave.snapshot()).toEqual({ kind: 'saved' });
    expect(setup.autosave.unsaved).toBe(false);
  });

  test('forgetting one refused answer still sends the other answers of that request', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');
    type(setup, secondQuestion, 'saw');
    setup.scheduler.pass(autosaveDelayMilliseconds);
    await setup.server.pending(0).answer({ kind: 'gone' });

    setup.autosave.forget([firstQuestion.id]);
    setup.scheduler.pass(autosaveDelayMilliseconds);

    expect(setup.server.received.map(sentTexts)).toEqual([['went', 'saw'], ['saw']]);
  });

  test('forgetting answers while a failed save waits for its retry keeps the failure and its backoff', async () => {
    const setup = setUp();
    type(setup, firstQuestion, 'went');
    type(setup, secondQuestion, 'saw');
    setup.scheduler.pass(autosaveDelayMilliseconds);
    await setup.server.pending(0).answer({ kind: 'retry', retryAfterSeconds: null, error: networkError });
    const delaysBefore = setup.scheduler.pendingDelays();

    setup.autosave.forget([firstQuestion.id]);

    expect(setup.autosave.snapshot()).toEqual({ kind: 'failed' });
    expect(setup.scheduler.pendingDelays()).toEqual(delaysBefore);
  });
});
