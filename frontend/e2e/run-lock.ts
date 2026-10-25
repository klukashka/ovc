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
}

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
      `another end-to-end run from ${holder.checkout} holds ${path} (pid ${String(holder.pid)}, started ${holder.startedAt}); wait for it to finish or stop it before starting another run, and if pid ${String(holder.pid)} is not an end-to-end run, remove ${path}`,
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
  if (
    typeof pid !== 'number' ||
    !Number.isSafeInteger(pid) ||
    pid <= 0 ||
    typeof startedAt !== 'string' ||
    typeof checkout !== 'string' ||
    !isSafeCheckoutPath(checkout)
  ) {
    throw malformed;
  }
  return { pid, startedAt, checkout };
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

function readHolder(path: string): RunLockHolder | undefined {
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
    return parseHolder(path, readFileSync(descriptor, 'utf8'));
  } finally {
    closeSync(descriptor);
  }
}

function sameHolder(left: RunLockHolder, right: RunLockHolder): boolean {
  return left.pid === right.pid && left.startedAt === right.startedAt && left.checkout === right.checkout;
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
