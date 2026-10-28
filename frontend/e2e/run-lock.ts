import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import {
  chmodSync,
  closeSync,
  constants,
  fstatSync,
  linkSync,
  lstatSync,
  mkdirSync,
  openSync,
  readFileSync,
  renameSync,
  unlinkSync,
  writeFileSync,
} from 'node:fs';
import { userInfo } from 'node:os';
import { dirname, isAbsolute, join } from 'node:path';

export interface RunLockHolder {
  readonly pid: number;
  readonly startedAt: string;
  readonly checkout: string;
  readonly kind?: 'merge-gate';
}

interface RunLockEntry {
  readonly holder: RunLockHolder;
  readonly device: number;
  readonly inode: number;
}

export const gateRunLockDescriptorVariable = 'E2E_HARNESS_GATE_RUN_LOCK_FD';

const descriptorPattern = /^[1-9][0-9]{0,3}$/;
const firstInheritedDescriptor = 3;
const processIdPattern = /^[0-9]+$/;
const ancestorLimit = 256;

const lockDirectoryMode = 0o700;
const lockFileMode = 0o600;
const permissionBits = 0o777;

export function machineRunLockPath(platform: NodeJS.Platform, homeDirectory: string): string {
  if (platform === 'darwin') {
    return join(homeDirectory, 'Library', 'Caches', 'ovc-e2e', 'run.lock');
  }
  if (platform === 'linux') {
    return join(homeDirectory, '.cache', 'ovc-e2e', 'run.lock');
  }
  throw new Error(`the end-to-end harness runs only on Linux and macOS, not on ${platform}`);
}

export class RunLockRefusedError extends Error {
  readonly holder: RunLockHolder;

  constructor(path: string, holder: RunLockHolder) {
    super(
      holder.kind === 'merge-gate'
        ? `the merge gate from ${holder.checkout} holds ${path} for its whole run (pid ${String(holder.pid)}, started ${holder.startedAt}); wait for the gate to finish before starting an end-to-end run`
        : `another end-to-end run from ${holder.checkout} holds ${path} (pid ${String(holder.pid)}, started ${holder.startedAt}); wait for it to finish or stop it before starting another run, and if pid ${String(holder.pid)} is not an end-to-end run, remove ${path}`,
    );
    this.name = 'RunLockRefusedError';
    this.holder = holder;
  }
}

function errorCode(error: unknown): string | undefined {
  if (error instanceof Error && 'code' in error && typeof error.code === 'string') {
    return error.code;
  }
  return undefined;
}

export function processIsAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    const code = errorCode(error);
    if (code === 'ESRCH') {
      return false;
    }
    if (code === 'EPERM') {
      return true;
    }
    throw error;
  }
}

function parseHolder(path: string, text: string): RunLockHolder {
  const malformed = new Error(`${path} is not an end-to-end run lock; remove it by hand once no run is active`);
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch (error) {
    throw new Error(malformed.message, { cause: error });
  }
  if (
    typeof parsed !== 'object' ||
    parsed === null ||
    !('pid' in parsed) ||
    !('startedAt' in parsed) ||
    !('checkout' in parsed)
  ) {
    throw malformed;
  }
  const { pid, startedAt, checkout } = parsed;
  const kind = 'kind' in parsed ? parsed.kind : undefined;
  if (
    typeof pid !== 'number' ||
    !Number.isSafeInteger(pid) ||
    pid <= 0 ||
    typeof startedAt !== 'string' ||
    typeof checkout !== 'string' ||
    !isSafeCheckoutPath(checkout) ||
    (kind !== undefined && kind !== 'merge-gate')
  ) {
    throw malformed;
  }
  return kind === undefined ? { pid, startedAt, checkout } : { pid, startedAt, checkout, kind };
}

function isControlCharacter(character: string): boolean {
  const code = character.charCodeAt(0);
  return code < 0x20 || code === 0x7f;
}

function isSafeCheckoutPath(checkout: string): boolean {
  return isAbsolute(checkout) && !Array.from(checkout).some(isControlCharacter);
}

function openLockFile(path: string): number | undefined {
  try {
    return openSync(path, constants.O_RDONLY | constants.O_NOFOLLOW);
  } catch (error) {
    const code = errorCode(error);
    if (code === 'ENOENT') {
      return undefined;
    }
    if (code === 'ELOOP') {
      throw new Error(`${path} is a link, not an end-to-end run lock; remove it by hand once no run is active`, {
        cause: error,
      });
    }
    throw error;
  }
}

function readEntry(path: string): RunLockEntry | undefined {
  const descriptor = openLockFile(path);
  if (descriptor === undefined) {
    return undefined;
  }
  try {
    const entry = fstatSync(descriptor);
    if (!entry.isFile() || entry.uid !== userInfo().uid) {
      throw new Error(
        `${path} must be a regular file owned by this user to be an end-to-end run lock; remove it by hand once no run is active`,
      );
    }
    return { holder: parseHolder(path, readFileSync(descriptor, 'utf8')), device: entry.dev, inode: entry.ino };
  } finally {
    closeSync(descriptor);
  }
}

function readHolder(path: string): RunLockHolder | undefined {
  return readEntry(path)?.holder;
}

function sameHolder(left: RunLockHolder, right: RunLockHolder): boolean {
  return (
    left.pid === right.pid &&
    left.startedAt === right.startedAt &&
    left.checkout === right.checkout &&
    left.kind === right.kind
  );
}

function linkIfAbsent(source: string, target: string): boolean {
  try {
    linkSync(source, target);
    return true;
  } catch (error) {
    if (errorCode(error) === 'EEXIST') {
      return false;
    }
    throw error;
  }
}

function uniqueSibling(path: string, purpose: string): string {
  return `${path}.${purpose}.${String(process.pid)}.${randomBytes(6).toString('hex')}`;
}

export function reclaimStale(path: string, stale: RunLockHolder, notice: (message: string) => void): void {
  const moved = uniqueSibling(path, 'stale');
  try {
    renameSync(path, moved);
  } catch (error) {
    if (errorCode(error) === 'ENOENT') {
      return;
    }
    throw error;
  }
  const movedHolder = readHolder(moved);
  if (movedHolder !== undefined && !sameHolder(movedHolder, stale)) {
    const restored = linkIfAbsent(moved, path);
    unlinkSync(moved);
    if (restored) {
      throw new RunLockRefusedError(path, movedHolder);
    }
    throw new Error(`${path} changed hands while a stale lock was reclaimed; start the run again`);
  }
  unlinkSync(moved);
  notice(
    `reclaimed the stale end-to-end run lock ${path} of pid ${String(stale.pid)}, started ${stale.startedAt}, which no running end-to-end run holds`,
  );
}

function createIfAbsent(directory: string): void {
  try {
    mkdirSync(directory, { mode: lockDirectoryMode });
  } catch (error) {
    if (errorCode(error) !== 'EEXIST') {
      throw error;
    }
  }
}

function prepareLockDirectory(directory: string): void {
  mkdirSync(dirname(directory), { recursive: true });
  createIfAbsent(directory);
  const entry = lstatSync(directory);
  if (!entry.isDirectory() || entry.uid !== userInfo().uid) {
    throw new Error(`${directory} must be a real folder owned by this user before it holds the end-to-end run lock`);
  }
  if ((entry.mode & permissionBits) !== lockDirectoryMode) {
    chmodSync(directory, lockDirectoryMode);
  }
}

export function acquireRunLock(path: string, holder: RunLockHolder, notice: (message: string) => void): void {
  prepareLockDirectory(dirname(path));
  const candidate = uniqueSibling(path, 'candidate');
  writeFileSync(candidate, JSON.stringify(holder), { flag: 'wx', mode: lockFileMode });
  try {
    for (let attempt = 0; attempt < 3; attempt += 1) {
      if (linkIfAbsent(candidate, path)) {
        return;
      }
      const current = readHolder(path);
      if (current === undefined) {
        continue;
      }
      if (current.pid !== process.pid && processIsAlive(current.pid)) {
        throw new RunLockRefusedError(path, current);
      }
      reclaimStale(path, current, notice);
    }
    throw new Error(`${path} could not be taken; start the run again`);
  } finally {
    unlinkSync(candidate);
  }
}

export function holdsRunLock(path: string, holder: RunLockHolder): boolean {
  const current = readHolder(path);
  return current !== undefined && sameHolder(current, holder);
}

export function releaseRunLock(path: string, holder: RunLockHolder): boolean {
  if (!holdsRunLock(path, holder)) {
    return false;
  }
  unlinkSync(path);
  return true;
}

export function parseGateRunLockDescriptor(value: string | undefined): number | undefined {
  if (value === undefined) {
    return undefined;
  }
  const descriptor = Number(value);
  if (!descriptorPattern.test(value) || descriptor < firstInheritedDescriptor) {
    throw new Error(
      `${gateRunLockDescriptorVariable} must name an inherited descriptor number of ${String(firstInheritedDescriptor)} or more; the merge gate sets it for its own end-to-end entry, and it is not a setting`,
    );
  }
  return descriptor;
}

function parentProcessId(pid: number): number {
  const output = execFileSync('ps', ['-o', 'ppid=', '-p', String(pid)], { encoding: 'utf8' }).trim();
  if (!processIdPattern.test(output)) {
    throw new Error(`ps reported no parent process id for pid ${String(pid)}`);
  }
  return Number(output);
}

export function processAncestors(pid: number): readonly number[] {
  const ancestors: number[] = [];
  let current = parentProcessId(pid);
  while (current > 0) {
    if (ancestors.length === ancestorLimit) {
      throw new Error(`pid ${String(pid)} has more than ${String(ancestorLimit)} ancestor processes`);
    }
    ancestors.push(current);
    if (current === 1) {
      break;
    }
    current = parentProcessId(current);
  }
  return ancestors;
}

function inheritedIdentity(descriptor: number): { readonly device: number; readonly inode: number } {
  try {
    const entry = fstatSync(descriptor);
    return { device: entry.dev, inode: entry.ino };
  } catch (error) {
    throw new Error(
      `${gateRunLockDescriptorVariable} names descriptor ${String(descriptor)}, which this run did not inherit open from the merge gate`,
      { cause: error },
    );
  }
}

export function adoptGateRunLock(
  path: string,
  descriptor: number,
  ancestors: readonly number[],
  checkout: string,
): RunLockHolder {
  const entry = readEntry(path);
  if (entry?.holder.kind !== 'merge-gate') {
    throw new Error(
      `${gateRunLockDescriptorVariable} is set, but no merge gate holds ${path}; only the merge gate's own end-to-end entry runs under the gate's lock`,
    );
  }
  const { holder } = entry;
  if (holder.checkout !== checkout) {
    throw new Error(
      `the merge gate that holds ${path} tests ${holder.checkout}, not this run's checkout ${checkout}; only the merge gate's own end-to-end entry runs under the gate's lock`,
    );
  }
  if (!ancestors.includes(holder.pid)) {
    throw new Error(
      `the merge gate pid ${String(holder.pid)} that holds ${path} is not an ancestor of this run; only the merge gate's own end-to-end entry runs under the gate's lock`,
    );
  }
  const inherited = inheritedIdentity(descriptor);
  if (inherited.device !== entry.device || inherited.inode !== entry.inode) {
    throw new Error(
      `descriptor ${String(descriptor)} is not the lock file ${path} that the merge gate holds; only the merge gate's own end-to-end entry runs under the gate's lock`,
    );
  }
  return holder;
}
