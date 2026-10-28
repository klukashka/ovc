import { type ChildProcess, spawn, spawnSync } from 'node:child_process';
import {
  chmodSync,
  closeSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  openSync,
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
  adoptGateRunLock,
  holdsRunLock,
  machineRunLockPath,
  parseGateRunLockDescriptor,
  processAncestors,
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

describe('end-to-end run lock held by the merge gate', () => {
  let openDescriptors: number[] = [];

  afterEach(() => {
    for (const descriptor of openDescriptors) {
      closeSync(descriptor);
    }
    openDescriptors = [];
  });

  function openForReading(path: string): number {
    const descriptor = openSync(path, 'r');
    openDescriptors.push(descriptor);
    return descriptor;
  }

  function gateHolder(pid: number): RunLockHolder {
    return { pid, startedAt: '2026-10-28T10:00:00Z', checkout: mainCheckout, kind: 'merge-gate' };
  }

  function nearestAncestor(): number {
    const [parent] = processAncestors(process.pid);
    if (parent === undefined) {
      throw new Error('this process has no parent process');
    }
    return parent;
  }

  test('lets a run adopt the lock of a merge gate that is its ancestor and whose lock file it inherited open', () => {
    const holder = gateHolder(nearestAncestor());
    writeLock(holder);
    const descriptor = openForReading(lockPath);

    const adopted = adoptGateRunLock(lockPath, descriptor, processAncestors(process.pid), mainCheckout);

    expect(adopted).toEqual(holder);
    expect(holdsRunLock(lockPath, adopted)).toBe(true);
  });

  test('refuses to adopt the lock of a merge gate that is not an ancestor of the run', () => {
    const holder = gateHolder(liveProcessId());
    writeLock(holder);
    const descriptor = openForReading(lockPath);

    const adoption = (): void => {
      adoptGateRunLock(lockPath, descriptor, processAncestors(process.pid), mainCheckout);
    };

    expect(adoption).toThrow(
      `the merge gate pid ${String(holder.pid)} that holds ${lockPath} is not an ancestor of this run`,
    );
    expect(lockedHolder()).toEqual(holder);
  });

  test('refuses to adopt the lock through a descriptor of another file or one that is not open', () => {
    writeLock(gateHolder(nearestAncestor()));
    const otherFile = join(directory, 'other');
    writeFileSync(otherFile, '{}');
    const otherDescriptor = openForReading(otherFile);
    const neverOpenDescriptor = 2_147_483_647;
    const ancestors = processAncestors(process.pid);

    const throughOtherFile = (): void => {
      adoptGateRunLock(lockPath, otherDescriptor, ancestors, mainCheckout);
    };
    const throughClosedDescriptor = (): void => {
      adoptGateRunLock(lockPath, neverOpenDescriptor, ancestors, mainCheckout);
    };

    expect(throughOtherFile).toThrow(`descriptor ${String(otherDescriptor)} is not the lock file ${lockPath}`);
    expect(throughClosedDescriptor).toThrow('which this run did not inherit open from the merge gate');
  });

  test('refuses to adopt the lock of a merge gate that tests another checkout', () => {
    writeLock(gateHolder(nearestAncestor()));
    const descriptor = openForReading(lockPath);

    const adoption = (): void => {
      adoptGateRunLock(lockPath, descriptor, processAncestors(process.pid), otherWorktree);
    };

    expect(adoption).toThrow(
      `the merge gate that holds ${lockPath} tests ${mainCheckout}, not this run's checkout ${otherWorktree}`,
    );
  });

  test('refuses to adopt a lock that an end-to-end run holds', () => {
    writeLock({ pid: nearestAncestor(), startedAt: '2026-10-28T10:00:00Z', checkout: mainCheckout });
    const descriptor = openForReading(lockPath);

    const adoption = (): void => {
      adoptGateRunLock(lockPath, descriptor, processAncestors(process.pid), mainCheckout);
    };

    expect(adoption).toThrow(`E2E_HARNESS_GATE_RUN_LOCK_FD is set, but no merge gate holds ${lockPath}`);
  });

  test('refuses an ordinary run while a merge gate holds the lock, naming the gate', () => {
    const holder = gateHolder(liveProcessId());
    writeLock(holder);
    const { notices, notice } = collectNotices();

    const acquisition = (): void => {
      acquireRunLock(
        lockPath,
        { pid: process.pid, startedAt: '2026-10-28T10:01:00.000Z', checkout: otherWorktree },
        notice,
      );
    };

    expect(acquisition).toThrow(RunLockRefusedError);
    expect(acquisition).toThrow(`the merge gate from ${mainCheckout} holds ${lockPath} for its whole run`);
    expect(lockedHolder()).toEqual(holder);
    expect(notices).toEqual([]);
  });

  test('reclaims the lock of a merge gate that is no longer running', () => {
    writeLock(gateHolder(exitedProcessId()));
    const holder = { pid: process.pid, startedAt: '2026-10-28T10:01:00.000Z', checkout: otherWorktree };
    const { notices, notice } = collectNotices();

    acquireRunLock(lockPath, holder, notice);

    expect(lockedHolder()).toEqual(holder);
    expect(notices).toHaveLength(1);
  });

  test('refuses a lock record of an unknown kind', () => {
    writeLockText(
      JSON.stringify({ pid: process.pid, startedAt: '2026-10-28T10:00:00Z', checkout: mainCheckout, kind: 'other' }),
    );
    const { notice } = collectNotices();

    const acquisition = (): void => {
      acquireRunLock(
        lockPath,
        { pid: process.pid, startedAt: '2026-10-28T10:01:00.000Z', checkout: mainCheckout },
        notice,
      );
    };

    expect(acquisition).toThrow(`${lockPath} is not an end-to-end run lock`);
  });

  test('reads the descriptor the merge gate names and refuses any other value', () => {
    const accepted = ['3', '7', '1024'];
    const refused = ['', '0', '2', '07', '7a', '-7', '10000'];

    const parsed = accepted.map((value) => parseGateRunLockDescriptor(value));
    const unset = parseGateRunLockDescriptor(undefined);
    const refusals = refused.map((value) => {
      try {
        parseGateRunLockDescriptor(value);
        return 'accepted';
      } catch {
        return 'refused';
      }
    });

    expect(unset).toBeUndefined();
    expect(parsed).toEqual([3, 7, 1024]);
    expect(refusals).toEqual(refused.map(() => 'refused'));
  });

  test('lists the parent process first among the ancestors of the current process', () => {
    const ancestors = processAncestors(process.pid);

    expect(ancestors[0]).toBe(process.ppid);
    expect(ancestors).not.toContain(process.pid);
  });
});
