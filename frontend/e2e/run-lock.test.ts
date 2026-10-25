import { type ChildProcess, spawn, spawnSync } from 'node:child_process';
import {
  chmodSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { afterEach, beforeEach, describe, expect, test } from 'vitest';

import {
  type RunLockHolder,
  RunLockRefusedError,
  acquireRunLock,
  holdsRunLock,
  machineRunLockPath,
  reclaimStale,
  releaseRunLock,
} from './run-lock.ts';

const mainCheckout = '/work/ovc';
const otherWorktree = '/work/ovc-worktrees/feature';

let directory = '';
let lockPath = '';
let liveProcess: ChildProcess | undefined;

beforeEach(() => {
  directory = mkdtempSync(join(tmpdir(), 'ovc-run-lock-'));
  lockPath = machineRunLockPath(process.platform, directory);
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
  writeLockText(JSON.stringify(holder));
}

function writeLockText(text: string): void {
  mkdirSync(dirname(lockPath), { recursive: true, mode: 0o700 });
  writeFileSync(lockPath, text);
}

function permissions(path: string): number {
  return lstatSync(path).mode & 0o777;
}

describe('end-to-end run lock', () => {
  test('refuses a second acquisition while the first run holds the lock, naming the holding pid and the lock file', () => {
    const first = { pid: liveProcessId(), startedAt: '2026-10-25T10:00:00.000Z', checkout: mainCheckout };
    const second = { pid: process.pid, startedAt: '2026-10-25T10:01:00.000Z', checkout: mainCheckout };
    const { notices, notice } = collectNotices();
    acquireRunLock(lockPath, first, notice);

    const refusal = (): void => {
      acquireRunLock(lockPath, second, notice);
    };

    expect(refusal).toThrow(RunLockRefusedError);
    expect(refusal).toThrow(`another end-to-end run from ${mainCheckout} holds ${lockPath}`);
    expect(refusal).toThrow(`pid ${String(first.pid)}, started 2026-10-25T10:00:00.000Z`);
    expect(refusal).toThrow(`if pid ${String(first.pid)} is not an end-to-end run, remove ${lockPath}`);
    expect(lockedHolder()).toEqual(first);
    expect(notices).toEqual([]);
  });

  test('reclaims a stale lock whose process is no longer running and logs a notice', () => {
    const stalePid = exitedProcessId();
    writeLock({ pid: stalePid, startedAt: '2026-10-24T09:00:00.000Z', checkout: mainCheckout });
    const holder = { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z', checkout: mainCheckout };
    const { notices, notice } = collectNotices();

    acquireRunLock(lockPath, holder, notice);

    expect(lockedHolder()).toEqual(holder);
    expect(notices).toHaveLength(1);
    expect(notices[0]).toContain(`pid ${String(stalePid)}`);
  });

  test('reclaims a lock left by an earlier run of the same process id as stale', () => {
    writeLock({ pid: process.pid, startedAt: '2026-10-24T09:00:00.000Z', checkout: mainCheckout });
    const holder = { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z', checkout: mainCheckout };
    const { notices, notice } = collectNotices();

    acquireRunLock(lockPath, holder, notice);

    expect(lockedHolder()).toEqual(holder);
    expect(notices).toHaveLength(1);
  });

  test('is held and released only by the exact holder and can be taken again after the release', () => {
    const holder = { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z', checkout: mainCheckout };
    const samePidOtherStart = { pid: process.pid, startedAt: '2026-10-25T11:00:00.000Z', checkout: mainCheckout };
    const { notice } = collectNotices();
    acquireRunLock(lockPath, holder, notice);

    const heldByOtherStart = holdsRunLock(lockPath, samePidOtherStart);
    const releasedByOtherStart = releaseRunLock(lockPath, samePidOtherStart);
    const releasedByOtherPid = releaseRunLock(lockPath, {
      pid: process.pid + 1,
      startedAt: holder.startedAt,
      checkout: mainCheckout,
    });
    const releasedByHolder = releaseRunLock(lockPath, holder);
    acquireRunLock(lockPath, holder, notice);

    expect(heldByOtherStart).toBe(false);
    expect(releasedByOtherStart).toBe(false);
    expect(releasedByOtherPid).toBe(false);
    expect(releasedByHolder).toBe(true);
    expect(holdsRunLock(lockPath, holder)).toBe(true);
  });

  test('restores a lock that a live run took after it was judged stale, and refuses', () => {
    const judgedStale = { pid: exitedProcessId(), startedAt: '2026-10-24T09:00:00.000Z', checkout: mainCheckout };
    const liveHolder = { pid: liveProcessId(), startedAt: '2026-10-25T10:00:00.000Z', checkout: mainCheckout };
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
    writeLockText('not a lock');
    const { notice } = collectNotices();

    const acquisition = (): void => {
      acquireRunLock(
        lockPath,
        { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z', checkout: mainCheckout },
        notice,
      );
    };

    expect(acquisition).toThrow('is not an end-to-end run lock');
    expect(readFileSync(lockPath, 'utf8')).toBe('not a lock');
  });

  test('places the lock in one per-user cache folder that names no checkout, on macOS and on Linux only', () => {
    const home = '/home/tutor';

    const macPath = machineRunLockPath('darwin', home);
    const linuxPath = machineRunLockPath('linux', home);
    const unsupported = (): string => machineRunLockPath('win32', home);

    expect(macPath).toBe('/home/tutor/Library/Caches/ovc-e2e/run.lock');
    expect(linuxPath).toBe('/home/tutor/.cache/ovc-e2e/run.lock');
    expect(unsupported).toThrow('runs only on Linux and macOS');
  });

  test('makes a run from another checkout contend for the same lock and names the checkout that holds it', () => {
    const fromMain = { pid: liveProcessId(), startedAt: '2026-10-25T10:00:00.000Z', checkout: mainCheckout };
    const fromWorktree = { pid: process.pid, startedAt: '2026-10-25T10:01:00.000Z', checkout: otherWorktree };
    const { notices, notice } = collectNotices();
    acquireRunLock(machineRunLockPath(process.platform, directory), fromMain, notice);

    const refusal = (): void => {
      acquireRunLock(machineRunLockPath(process.platform, directory), fromWorktree, notice);
    };

    expect(refusal).toThrow(RunLockRefusedError);
    expect(refusal).toThrow(`another end-to-end run from ${mainCheckout} holds ${lockPath}`);
    expect(lockedHolder()).toEqual(fromMain);
    expect(holdsRunLock(lockPath, { ...fromMain, checkout: otherWorktree })).toBe(false);
    expect(notices).toEqual([]);
  });

  test('creates the lock folder readable only by its owner and the lock file readable only by its owner', () => {
    const holder = { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z', checkout: mainCheckout };
    const { notice } = collectNotices();

    acquireRunLock(lockPath, holder, notice);

    expect(permissions(dirname(lockPath))).toBe(0o700);
    expect(permissions(lockPath)).toBe(0o600);
  });

  test('narrows an existing lock folder that others could read to its owner only', () => {
    mkdirSync(dirname(lockPath), { recursive: true });
    chmodSync(dirname(lockPath), 0o755);
    const holder = { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z', checkout: mainCheckout };
    const { notice } = collectNotices();

    acquireRunLock(lockPath, holder, notice);

    expect(permissions(dirname(lockPath))).toBe(0o700);
    expect(lockedHolder()).toEqual(holder);
  });

  test('refuses a lock record that names no checkout', () => {
    writeLockText(JSON.stringify({ pid: process.pid, startedAt: '2026-10-24T09:00:00.000Z' }));
    const { notice } = collectNotices();

    const acquisition = (): void => {
      acquireRunLock(
        lockPath,
        { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z', checkout: mainCheckout },
        notice,
      );
    };

    expect(acquisition).toThrow('is not an end-to-end run lock');
  });

  test('reclaims a stale lock that a run from another checkout left and logs a notice', () => {
    const stalePid = exitedProcessId();
    writeLock({ pid: stalePid, startedAt: '2026-10-24T09:00:00.000Z', checkout: otherWorktree });
    const holder = { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z', checkout: mainCheckout };
    const { notices, notice } = collectNotices();

    acquireRunLock(lockPath, holder, notice);

    expect(lockedHolder()).toEqual(holder);
    expect(notices).toHaveLength(1);
    expect(notices[0]).toContain(`pid ${String(stalePid)}`);
  });

  test('refuses a lock folder that is a link to another folder', () => {
    const target = join(directory, 'elsewhere');
    mkdirSync(target, { mode: 0o700 });
    mkdirSync(dirname(dirname(lockPath)), { recursive: true });
    symlinkSync(target, dirname(lockPath));
    const { notice } = collectNotices();

    const acquisition = (): void => {
      acquireRunLock(
        lockPath,
        { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z', checkout: mainCheckout },
        notice,
      );
    };

    expect(acquisition).toThrow('must be a real folder owned by this user');
  });

  test('refuses a lock file that is a link', () => {
    const target = join(directory, 'planted-lock');
    writeFileSync(
      target,
      JSON.stringify({ pid: exitedProcessId(), startedAt: '2026-10-24T09:00:00.000Z', checkout: mainCheckout }),
    );
    mkdirSync(dirname(lockPath), { recursive: true, mode: 0o700 });
    symlinkSync(target, lockPath);
    const { notice } = collectNotices();

    const acquisition = (): void => {
      acquireRunLock(
        lockPath,
        { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z', checkout: mainCheckout },
        notice,
      );
    };

    expect(acquisition).toThrow('is a link, not an end-to-end run lock');
  });

  test('refuses a lock record whose checkout is not an absolute path free of control characters', () => {
    const records = ['relative/ovc', '/work/ovc\nforged line'].map((checkout) =>
      JSON.stringify({ pid: process.pid, startedAt: '2026-10-24T09:00:00.000Z', checkout }),
    );
    const { notice } = collectNotices();

    const refusals = records.map((record) => {
      writeLockText(record);
      try {
        acquireRunLock(
          lockPath,
          { pid: process.pid, startedAt: '2026-10-25T10:00:00.000Z', checkout: mainCheckout },
          notice,
        );
        return 'acquired';
      } catch (error) {
        return error instanceof Error ? error.message : 'unknown';
      }
    });

    expect(refusals).toEqual([
      `${lockPath} is not an end-to-end run lock; remove it by hand once no run is active`,
      `${lockPath} is not an end-to-end run lock; remove it by hand once no run is active`,
    ]);
  });
});
