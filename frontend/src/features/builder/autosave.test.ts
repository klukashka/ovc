import { describe, expect, test } from 'vitest';

import { apiErrorFrom } from '../../lib/api/client.ts';
import { type ManualScheduler, manualScheduler } from '../../lib/api/fakeTransport.ts';
import {
  type SaveBody,
  type SaveOutcome,
  type SaveRequest,
  type Template,
  TemplateAutosave,
  autosaveDelayMilliseconds,
} from './autosave.ts';

interface PendingSave {
  readonly request: SaveRequest;
  readonly answer: (outcome: SaveOutcome) => Promise<void>;
}

class TemplateServer {
  readonly received: PendingSave[] = [];
  private version: number;
  private lastSaveId: string | null = null;
  private storedTitle = '';

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
      return { kind: 'saved', template: this.template() };
    }
    if (request.version !== this.version) {
      return { kind: 'conflict' };
    }
    this.version += 1;
    this.lastSaveId = request.saveId;
    this.storedTitle = request.title;
    return { kind: 'saved', template: this.template() };
  }

  get title(): string {
    return this.storedTitle;
  }

  private template(): Template {
    return {
      id: 'template-1',
      title: this.storedTitle,
      folder_id: null,
      blocks: [],
      version: this.version,
      ready: false,
      problems: [{ block_id: null, code: 'no_blocks' }],
      question_count: 0,
      max_points: 0,
      created_at: '2026-11-10T10:00:00Z',
      updated_at: '2026-11-10T10:00:00Z',
    };
  }
}

const networkError = apiErrorFrom(0, undefined);

function body(title: string): SaveBody {
  return { title, blocks: [] };
}

function setUp(version = 1): {
  readonly scheduler: ManualScheduler;
  readonly server: TemplateServer;
  readonly autosave: TemplateAutosave;
  readonly saved: Template[];
} {
  const scheduler = manualScheduler();
  const server = new TemplateServer(version);
  const saved: Template[] = [];
  let counter = 0;
  const autosave = new TemplateAutosave({
    scheduler,
    version,
    newSaveId: (): string => {
      counter += 1;
      return `save-${String(counter)}`;
    },
    send: server.send,
    onSaved: (template): void => {
      saved.push(template);
    },
  });
  return { scheduler, server, autosave, saved };
}

describe('template autosave', () => {
  test('a save is sent one second after the last change, with the latest text only', () => {
    const { scheduler, server, autosave } = setUp();

    autosave.changed(body('P'));
    scheduler.pass(autosaveDelayMilliseconds - 1);
    autosave.changed(body('Past simple'));
    scheduler.pass(autosaveDelayMilliseconds - 1);
    const beforeDelay = server.received.length;
    scheduler.pass(1);

    expect(beforeDelay).toBe(0);
    expect(server.received.map((save) => save.request.title)).toEqual(['Past simple']);
    expect(autosave.snapshot()).toEqual({ kind: 'saving' });
  });

  test('a save answering after newer typing keeps that typing and sends it next with the new version', async () => {
    const { scheduler, server, autosave } = setUp(4);
    autosave.changed(body('Week 1'));
    scheduler.pass(autosaveDelayMilliseconds);
    autosave.changed(body('Week 12'));
    scheduler.pass(autosaveDelayMilliseconds);
    const inFlightWhileTyping = server.received.length;

    const first = server.pending(0);
    await first.answer(server.apply(first.request));

    expect(inFlightWhileTyping).toBe(1);
    expect(server.received.map((save) => [save.request.title, save.request.version])).toEqual([
      ['Week 1', 4],
      ['Week 12', 5],
    ]);
    expect(autosave.snapshot()).toEqual({ kind: 'saving' });
  });

  test('a retry after an uncertain failure resends the identical body under the same save identifier', async () => {
    const { scheduler, server, autosave, saved } = setUp(2);
    autosave.changed(body('Fractions'));
    scheduler.pass(autosaveDelayMilliseconds);
    const lost = server.pending(0);
    server.apply(lost.request);
    await lost.answer({ kind: 'uncertain', error: networkError });
    const failed = autosave.snapshot();
    autosave.changed(body('Fractions and decimals'));
    scheduler.pass(autosaveDelayMilliseconds);

    const replay = server.pending(1);
    await replay.answer(server.apply(replay.request));
    const next = server.pending(2);
    await next.answer(server.apply(next.request));

    expect(failed).toEqual({ kind: 'failed', reason: 'uncertain', error: networkError });
    expect([replay.request.title, replay.request.saveId, replay.request.version]).toEqual(['Fractions', 'save-1', 2]);
    expect([next.request.title, next.request.saveId, next.request.version]).toEqual([
      'Fractions and decimals',
      'save-2',
      3,
    ]);
    expect(server.title).toBe('Fractions and decimals');
    expect(saved.map((template) => template.version)).toEqual([3, 4]);
    expect(autosave.snapshot()).toEqual({ kind: 'saved' });
  });

  test('a stale version stops autosave and keeps the unsaved changes until a reload', async () => {
    const { scheduler, server, autosave } = setUp(1);
    server.apply({ title: 'Changed in another tab', blocks: [], version: 1, saveId: 'other-tab' });
    autosave.changed(body('Mine'));
    scheduler.pass(autosaveDelayMilliseconds);
    const stale = server.pending(0);
    await stale.answer(server.apply(stale.request));
    autosave.changed(body('Mine, typed after'));
    scheduler.pass(autosaveDelayMilliseconds);
    const sentDuringConflict = server.received.length;
    const unsavedDuringConflict = autosave.unsaved;

    autosave.reloaded(2);

    expect(sentDuringConflict).toBe(1);
    expect(unsavedDuringConflict).toBe(true);
    expect(autosave.unsaved).toBe(false);
    expect(autosave.snapshot()).toEqual({ kind: 'saved' });
  });

  test('a blank title is never sent, and the title typed after it is', () => {
    const { scheduler, server, autosave } = setUp();

    autosave.changed(body('  '));
    scheduler.pass(autosaveDelayMilliseconds * 3);
    const blankStatus = autosave.snapshot();
    autosave.changed(body('Verbs'));
    scheduler.pass(autosaveDelayMilliseconds);

    expect(blankStatus).toEqual({ kind: 'blankTitle' });
    expect(server.received.map((save) => save.request.title)).toEqual(['Verbs']);
  });

  test('a rate-limited save is sent again after Retry-After under a new save identifier', async () => {
    const { scheduler, server, autosave } = setUp();
    autosave.changed(body('Verbs'));
    scheduler.pass(autosaveDelayMilliseconds);
    const limited = server.pending(0);
    await limited.answer({ kind: 'rateLimited', retryAfterSeconds: 3, error: networkError });
    scheduler.pass(2999);
    const sentBeforeRetryAfter = server.received.length;

    scheduler.pass(1);

    expect(sentBeforeRetryAfter).toBe(1);
    expect(server.received.map((save) => save.request.saveId)).toEqual(['save-1', 'save-2']);
  });

  test('typing and Retry during a Retry-After wait never send before it ends', async () => {
    const { scheduler, server, autosave } = setUp();
    autosave.changed(body('Verbs'));
    scheduler.pass(autosaveDelayMilliseconds);
    const limited = server.pending(0);
    await limited.answer({ kind: 'rateLimited', retryAfterSeconds: 30, error: networkError });

    autosave.changed(body('Verbs and nouns'));
    scheduler.pass(autosaveDelayMilliseconds);
    autosave.retry();
    scheduler.pass(28_998);
    const sentDuringWait = server.received.length;
    scheduler.pass(2);

    expect(sentDuringWait).toBe(1);
    expect(server.received.map((save) => save.request.title)).toEqual(['Verbs', 'Verbs and nouns']);
  });

  test('a flush during a Retry-After wait sends once the wait ends', async () => {
    const { scheduler, server, autosave } = setUp();
    autosave.changed(body('Verbs'));
    scheduler.pass(autosaveDelayMilliseconds);
    const limited = server.pending(0);
    await limited.answer({ kind: 'rateLimited', retryAfterSeconds: 5, error: networkError });

    const flushed = autosave.flush();
    scheduler.pass(4999);
    const sentDuringWait = server.received.length;
    scheduler.pass(1);
    const resent = server.pending(1);
    await resent.answer(server.apply(resent.request));

    expect(sentDuringWait).toBe(1);
    expect(await flushed).toBe(true);
  });

  test('a flush waits for the save in flight and for the typing after it', async () => {
    const { scheduler, server, autosave } = setUp();
    autosave.changed(body('A'));
    scheduler.pass(autosaveDelayMilliseconds);
    autosave.changed(body('AB'));
    const flushed = autosave.flush();

    const first = server.pending(0);
    await first.answer(server.apply(first.request));
    const second = server.pending(1);
    await second.answer(server.apply(second.request));

    expect(await flushed).toBe(true);
    expect(server.title).toBe('AB');
  });
});
