import { type ChildProcess, spawn, spawnSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, test } from 'vitest';

import {
  type RunLockHolder,
  RunLockRefusedError,
  acquireRunLock,
  holdsRunLock,
  reclaimStale,
  releaseRunLock,
} from './run-lock.ts';

let directory = '';
let lockPath = '';
let liveProcess: ChildProcess | undefined;

beforeEach(() => {
  directory = mkdtempSync(join(tmpdir(), 'ovc-run-lock-'));
  lockPath = join(directory, 'e2e-run.lock');
});

afterEach(async () => {
  rmSync(directory, { recursive: true, force: true });
  const running = liveProcess;
  liveProcess = undefined;
  if (running?.exitCode === null && running.signalCode === null) {
    const exited = new Promise((resolveExit) => running.once('exit', resolveExit));
    running.kill('SIGTERM');
    await exited;
  }
});

function exitedProcessId(): number {
  const result = spawnSync(process.execPath, ['--eval', '']);
  if (result.status !== 0) {
    throw new Error('the short-lived process did not run');
  }
  return result.pid;
}

function liveProcessId(): number {
  const child = spawn(process.execPath, ['--eval', 'process.stdin.resume()'], { stdio: ['pipe', 'ignore', 'ignore'] });
  liveProcess = child;
  if (child.pid === undefined) {
    throw new Error('the long-lived process did not start');
  }
  return child.pid;
}

function collectNotices(): { readonly notices: string[]; readonly notice: (message: string) => void } {
  const notices: string[] = [];
  return {
    notices,
    notice: (message: string): void => {
      notices.push(message);
    },
  };
}

function lockedHolder(): unknown {
  return JSON.parse(readFileSync(lockPath, 'utf8'));
}

function writeLock(holder: RunLockHolder): void {
  writeFileSync(lockPath, JSON.stringify(holder));
}

describe('end-to-end run lock', () => {
  test('refuses a second acquisition while the first run holds the lock, naming the holding pid and the lock file', () => {
    const first = { pid: liveProcessId(), startedAt: '2026-10-25T10:00:00.000Z' };
    const second = { pid: process.pid, startedAt: '2026-10-25T10:01:00.000Z' };
    const { notices, notice } = collectNotices();
    acquireRunLock(lockPath, first, notice);

    const refusal = (): void => {
      acquireRunLock(lockPath, second, notice);
    };

    expect(refusal).toThrow(RunLockRefusedError);
    expect(refusal).toThrow(`pid ${String(first.pid)}, started 2026-10-25T10:00:00.000Z`);
    expect(refusal).toThrow(`if pid ${String(first.pid)} is not an end-to-end run, remove ${lockPath}`);
    expect(lockedHolder()).toEqual(first);
    expect(notices).toEqual([]);
  });

  test('reclaims a stale lock whose process is no longer running and logs a notice', () => {
    const stalePid = exitedProcessId();
    writeLock({ pid: stalePid, startedAt: '2026-10-24T09:00:00.000Z' });
    const holder = { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z' };
    const { notices, notice } = collectNotices();

    acquireRunLock(lockPath, holder, notice);

    expect(lockedHolder()).toEqual(holder);
    expect(notices).toHaveLength(1);
    expect(notices[0]).toContain(`pid ${String(stalePid)}`);
  });

  test('reclaims a lock left by an earlier run of the same process id as stale', () => {
    writeLock({ pid: process.pid, startedAt: '2026-10-24T09:00:00.000Z' });
    const holder = { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z' };
    const { notices, notice } = collectNotices();

    acquireRunLock(lockPath, holder, notice);

    expect(lockedHolder()).toEqual(holder);
    expect(notices).toHaveLength(1);
  });

  test('is held and released only by the exact holder and can be taken again after the release', () => {
    const holder = { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z' };
    const samePidOtherStart = { pid: process.pid, startedAt: '2026-10-25T11:00:00.000Z' };
    const { notice } = collectNotices();
    acquireRunLock(lockPath, holder, notice);

    const heldByOtherStart = holdsRunLock(lockPath, samePidOtherStart);
    const releasedByOtherStart = releaseRunLock(lockPath, samePidOtherStart);
    const releasedByOtherPid = releaseRunLock(lockPath, { pid: process.pid + 1, startedAt: holder.startedAt });
    const releasedByHolder = releaseRunLock(lockPath, holder);
    acquireRunLock(lockPath, holder, notice);

    expect(heldByOtherStart).toBe(false);
    expect(releasedByOtherStart).toBe(false);
    expect(releasedByOtherPid).toBe(false);
    expect(releasedByHolder).toBe(true);
    expect(holdsRunLock(lockPath, holder)).toBe(true);
  });

  test('restores a lock that a live run took after it was judged stale, and refuses', () => {
    const judgedStale = { pid: exitedProcessId(), startedAt: '2026-10-24T09:00:00.000Z' };
    const liveHolder = { pid: liveProcessId(), startedAt: '2026-10-25T10:00:00.000Z' };
    writeLock(liveHolder);
    const { notices, notice } = collectNotices();

    const reclaim = (): void => {
      reclaimStale(lockPath, judgedStale, notice);
    };

    expect(reclaim).toThrow(RunLockRefusedError);
    expect(lockedHolder()).toEqual(liveHolder);
    expect(notices).toEqual([]);
  });

  test('refuses a lock file it cannot read as a run lock', () => {
    writeFileSync(lockPath, 'not a lock');
    const { notice } = collectNotices();

    const acquisition = (): void => {
      acquireRunLock(lockPath, { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z' }, notice);
    };

    expect(acquisition).toThrow('is not an end-to-end run lock');
    expect(readFileSync(lockPath, 'utf8')).toBe('not a lock');
  });
});
