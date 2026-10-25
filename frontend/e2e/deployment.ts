import { type ChildProcess, execFileSync, spawn, spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { chmodSync, closeSync, createWriteStream, mkdirSync, openSync, readdirSync, rmSync } from 'node:fs';
import { createConnection } from 'node:net';
import { join, resolve } from 'node:path';

import { type RunLockHolder, acquireRunLock, holdsRunLock, releaseRunLock } from './run-lock.ts';
import { assertOutputRootIsReal, playwrightOutputRoot } from './run-output.ts';

const frontendRoot = resolve(import.meta.dirname, '..');
const repositoryRoot = resolve(frontendRoot, '..');
const backendRoot = resolve(repositoryRoot, 'backend');
const outputRoot = resolve(frontendRoot, 'test-results', 'deployment');
const binariesRoot = join(outputRoot, 'bin');
const tlsRoot = join(outputRoot, 'tls');
const runLockPath = resolve(frontendRoot, 'test-results', 'e2e-run.lock');
const runOutputsRoot = playwrightOutputRoot(frontendRoot);
const previewCertificate = join(tlsRoot, 'preview-certificate.pem');
const previewPrivateKey = join(tlsRoot, 'preview-private-key.pem');
const readinessDeadlineMilliseconds = 30_000;
const stopDeadlineMilliseconds = 20_000;
const pollIntervalMilliseconds = 100;
const pollTimeoutMilliseconds = 2_000;
const firstApiPort = 18_800;
const firstPreviewPort = 4_300;
const loopbackHosts: ReadonlySet<string> = new Set(['127.0.0.1', '[::1]', 'localhost']);
const storageBucket = 'ovc-e2e';
const storagePortPattern = /^[1-9][0-9]{0,4}$/;

interface AdminConnection {
  readonly user: string;
  readonly password: string;
}

interface StorageSettings {
  readonly endpoint: string;
  readonly accessKey: string;
  readonly secretKey: string;
}

export interface E2ESettings {
  readonly adminUrl: URL;
  readonly storage: StorageSettings;
}

export interface Deployment {
  readonly origin: string;
  readonly apiUrl: string;
  readonly administer: (args: readonly string[]) => void;
  readonly stop: () => Promise<void>;
}

function readStorageSettings(environment: NodeJS.ProcessEnv): StorageSettings {
  const accessKey = environment.OVC_DEV_STORAGE_ACCESS_KEY ?? '';
  const secretKey = environment.OVC_DEV_STORAGE_SECRET_KEY ?? '';
  if (accessKey === '' || secretKey === '') {
    throw new Error(
      'OVC_DEV_STORAGE_ACCESS_KEY and OVC_DEV_STORAGE_SECRET_KEY must be set to run the end-to-end suites; run them through tools/dev-env.sh',
    );
  }
  const port = environment.OVC_DEV_STORAGE_PORT ?? '58333';
  if (!storagePortPattern.test(port)) {
    throw new Error('OVC_DEV_STORAGE_PORT must be a port number');
  }
  return { endpoint: `http://127.0.0.1:${port}`, accessKey, secretKey };
}

export function readE2ESettings(environment: NodeJS.ProcessEnv): E2ESettings {
  const raw = environment.OVC_TEST_DATABASE_ADMIN_URL;
  if (raw === undefined || raw === '') {
    throw new Error('OVC_TEST_DATABASE_ADMIN_URL must be set to run the end-to-end suites');
  }
  const adminUrl = new URL(raw);
  if (adminUrl.protocol !== 'postgres:' && adminUrl.protocol !== 'postgresql:') {
    throw new Error('OVC_TEST_DATABASE_ADMIN_URL must be a postgres:// URL');
  }
  if (adminUrl.searchParams.has('host') || adminUrl.searchParams.has('hostaddr')) {
    throw new Error('OVC_TEST_DATABASE_ADMIN_URL must not override its host in query parameters');
  }
  if (!loopbackHosts.has(adminUrl.hostname)) {
    throw new Error('OVC_TEST_DATABASE_ADMIN_URL must name a loopback host: 127.0.0.1, [::1] or localhost');
  }
  return { adminUrl, storage: readStorageSettings(environment) };
}

function adminConnection(adminUrl: URL): AdminConnection {
  return {
    user: decodeURIComponent(adminUrl.username),
    password: decodeURIComponent(adminUrl.password),
  };
}

function databaseUrl(adminUrl: URL, database: string): string {
  const url = new URL(adminUrl);
  url.pathname = `/${database}`;
  return url.toString();
}

function runInPostgresContainer(program: 'createdb' | 'dropdb', connection: AdminConnection, args: string[]): void {
  execFileSync(
    'docker',
    [
      'compose',
      'exec',
      '-T',
      '-e',
      'PGPASSWORD',
      'postgres',
      program,
      '-h',
      '127.0.0.1',
      '-U',
      connection.user,
      ...args,
    ],
    {
      cwd: repositoryRoot,
      env: { ...process.env, PGPASSWORD: connection.password },
      stdio: ['ignore', 'ignore', 'inherit'],
    },
  );
}

function ensureStorageBucket(): void {
  const result = spawnSync('docker', ['compose', 'exec', '-T', 'storage', 'weed', 'shell'], {
    cwd: repositoryRoot,
    input: `s3.bucket.create -name ${storageBucket}\n`,
    encoding: 'utf8',
  });
  const output = `${result.stdout}${result.stderr}`;
  if (
    !output.includes(`created bucket ${storageBucket}`) &&
    !output.includes(`bucket ${storageBucket} already exists`)
  ) {
    throw new Error(`the storage bucket ${storageBucket} could not be created`);
  }
}

function deleteStorageBucket(): void {
  const result = spawnSync('docker', ['compose', 'exec', '-T', 'storage', 'weed', 'shell'], {
    cwd: repositoryRoot,
    input: `s3.bucket.delete -name ${storageBucket}\n`,
    encoding: 'utf8',
  });
  const output = `${result.stdout}${result.stderr}`;
  if (output.includes('bucket not found')) {
    return;
  }
  if (result.error !== undefined || result.status !== 0 || output.includes('error')) {
    throw new Error(`the storage bucket ${storageBucket} could not be deleted`);
  }
}

function createPreviewCertificate(): void {
  mkdirSync(tlsRoot, { recursive: true, mode: 0o700 });
  chmodSync(tlsRoot, 0o700);
  execFileSync(
    'openssl',
    [
      'req',
      '-x509',
      '-newkey',
      'rsa:2048',
      '-nodes',
      '-days',
      '1',
      '-subj',
      '/CN=127.0.0.1',
      '-addext',
      'subjectAltName=IP:127.0.0.1',
      '-keyout',
      previewPrivateKey,
      '-out',
      previewCertificate,
    ],
    { stdio: ['ignore', 'ignore', 'inherit'] },
  );
  chmodSync(tlsRoot, 0o700);
  chmodSync(previewPrivateKey, 0o600);
}

const runLock: { holder: RunLockHolder | undefined } = { holder: undefined };

function assertStillHeld(holder: RunLockHolder): void {
  if (!holdsRunLock(runLockPath, holder)) {
    throw new Error(
      `${runLockPath} no longer holds this run (pid ${String(holder.pid)}, started ${holder.startedAt}); another run took it, so this run stops instead of touching the shared deployment files and bucket`,
    );
  }
}

export function takeRunLock(): void {
  const holder = { pid: process.pid, startedAt: new Date().toISOString() };
  acquireRunLock(runLockPath, holder, (message) => {
    process.stderr.write(`${message}\n`);
  });
  runLock.holder = holder;
  process.once('exit', () => {
    releaseRunLock(runLockPath, holder);
  });
  assertStillHeld(holder);
}

export function removeEarlierRunOutputs(currentOutputs: readonly string[]): void {
  if (runLock.holder === undefined) {
    throw new Error('earlier run outputs are removed only after this run takes the run lock');
  }
  assertStillHeld(runLock.holder);
  const kept = new Set(currentOutputs.map((output) => resolve(output)));
  assertOutputRootIsReal(frontendRoot);
  mkdirSync(runOutputsRoot, { recursive: true });
  assertOutputRootIsReal(frontendRoot);
  for (const entry of readdirSync(runOutputsRoot)) {
    const path = join(runOutputsRoot, entry);
    if (!kept.has(path)) {
      rmSync(path, { recursive: true, force: true });
    }
  }
}

export function finishRun(): void {
  const { holder } = runLock;
  if (holder === undefined) {
    return;
  }
  assertStillHeld(holder);
  try {
    deleteStorageBucket();
  } finally {
    releaseRunLock(runLockPath, holder);
    runLock.holder = undefined;
  }
}

export function prepareRun(): void {
  mkdirSync(binariesRoot, { recursive: true });
  createPreviewCertificate();
  ensureStorageBucket();
  execFileSync('go', ['build', '-o', join(binariesRoot, 'api'), './cmd/api'], { cwd: backendRoot, stdio: 'inherit' });
  execFileSync('go', ['build', '-o', join(binariesRoot, 'worker'), './cmd/worker'], {
    cwd: backendRoot,
    stdio: 'inherit',
  });
  execFileSync('go', ['build', '-o', join(binariesRoot, 'ovc-admin'), './cmd/ovc-admin'], {
    cwd: backendRoot,
    stdio: 'inherit',
  });
  execFileSync('pnpm', ['build'], { cwd: frontendRoot, stdio: 'inherit' });
}

function backendEnvironment(
  database: string,
  settings: E2ESettings,
  apiAddress: string,
  origin: string,
): NodeJS.ProcessEnv {
  return {
    OVC_HTTP_ADDR: apiAddress,
    OVC_DATABASE_URL: databaseUrl(settings.adminUrl, database),
    OVC_DB_POOL_SIZE: '5',
    OVC_LOG_LEVEL: 'info',
    OVC_LOG_FORMAT: 'json',
    OVC_API_DOCS_ENABLED: 'false',
    OVC_TEST_CONTROL: '1',
    OVC_CLOCK: 'test',
    OVC_MEETING_PROVIDER: 'fake',
    OVC_MAIL_BACKEND: 'sink',
    OVC_PUBLIC_URL: origin,
    OVC_JWT_SECRET: randomBytes(32).toString('hex'),
    OVC_FIELD_ENCRYPTION_KEY: randomBytes(32).toString('base64'),
    OVC_TRUSTED_PROXY_CIDRS: '127.0.0.1/32',
    OVC_STORAGE_BACKEND: 's3',
    OVC_STORAGE_ENDPOINT: settings.storage.endpoint,
    OVC_STORAGE_REGION: 'us-east-1',
    OVC_STORAGE_BUCKET: storageBucket,
    OVC_STORAGE_ACCESS_KEY: settings.storage.accessKey,
    OVC_STORAGE_SECRET_KEY: settings.storage.secretKey,
  };
}

const adminVariables = [
  'OVC_DATABASE_URL',
  'OVC_LOG_LEVEL',
  'OVC_LOG_FORMAT',
  'OVC_HTTP_ADDR',
  'OVC_TEST_CONTROL',
  'OVC_CLOCK',
  'OVC_MEETING_PROVIDER',
  'OVC_MAIL_BACKEND',
] as const;

function adminEnvironment(environment: NodeJS.ProcessEnv): NodeJS.ProcessEnv {
  return Object.fromEntries(adminVariables.map((name) => [name, environment[name]]));
}

interface ManagedProcess {
  readonly name: string;
  readonly child: ChildProcess;
  readonly exit: Promise<string>;
  readonly cleanExits: ReadonlySet<string>;
}

const goExits: ReadonlySet<string> = new Set(['exit code 0']);
const previewExits: ReadonlySet<string> = new Set(['exit code 0', 'signal SIGTERM', 'exit code 143']);

function exitDescription(code: number | null, signal: NodeJS.Signals | null): string {
  return signal === null ? `exit code ${String(code)}` : `signal ${signal}`;
}

function hasExited(child: ChildProcess): boolean {
  return child.exitCode !== null || child.signalCode !== null;
}

function start(
  name: string,
  command: string,
  args: string[],
  environment: NodeJS.ProcessEnv,
  logName: string,
  cleanExits: ReadonlySet<string> = goExits,
): ManagedProcess {
  const log = createWriteStream(join(outputRoot, logName));
  const child = spawn(command, args, { cwd: frontendRoot, env: environment, stdio: ['ignore', 'pipe', 'pipe'] });
  child.stdout.pipe(log, { end: false });
  child.stderr.pipe(log, { end: false });
  child.once('close', () => {
    log.end();
  });
  const exit = new Promise<string>((resolveExit) => {
    child.once('exit', (code, signal) => {
      resolveExit(exitDescription(code, signal));
    });
    child.once('error', (error) => {
      resolveExit(`spawn error ${error.message}`);
    });
  });
  return { name, child, exit, cleanExits };
}

function runToCompletion(binary: string, args: string[], environment: NodeJS.ProcessEnv, logName: string): void {
  const log = join(outputRoot, logName);
  const descriptor = openSync(log, 'w');
  try {
    execFileSync(binary, args, { env: environment, stdio: ['ignore', descriptor, descriptor] });
  } catch (error) {
    throw new Error(`${binary} ${args.join(' ')} failed; see ${log}`, { cause: error });
  } finally {
    closeSync(descriptor);
  }
}

function delay(milliseconds: number): Promise<void> {
  return new Promise((resolveDelay) => setTimeout(resolveDelay, milliseconds));
}

function portAnswers(address: string): Promise<boolean> {
  const [host = '', port = ''] = address.split(':');
  return new Promise((resolveProbe) => {
    const socket = createConnection({ host, port: Number(port), timeout: pollTimeoutMilliseconds });
    socket.once('connect', () => {
      socket.destroy();
      resolveProbe(true);
    });
    socket.once('error', () => {
      resolveProbe(false);
    });
    socket.once('timeout', () => {
      socket.destroy();
      resolveProbe(false);
    });
  });
}

async function assertAddressFree(address: string): Promise<void> {
  if (await portAnswers(address)) {
    throw new Error(`${address} is already in use; stop the process holding it before running the end-to-end suites`);
  }
}

async function waitUntilOk(managed: ManagedProcess, url: string): Promise<void> {
  const deadline = Date.now() + readinessDeadlineMilliseconds;
  while (Date.now() < deadline) {
    if (hasExited(managed.child)) {
      throw new Error(`${managed.name} stopped with ${await managed.exit} before becoming ready`);
    }
    const ready = await fetch(url, { signal: AbortSignal.timeout(pollTimeoutMilliseconds) }).then(
      (response) => response.ok,
      () => false,
    );
    if (ready) {
      return;
    }
    await delay(pollIntervalMilliseconds);
  }
  throw new Error(`${managed.name} did not become ready within ${String(readinessDeadlineMilliseconds)} ms`);
}

async function waitUntilListening(managed: ManagedProcess, address: string): Promise<void> {
  const deadline = Date.now() + readinessDeadlineMilliseconds;
  while (Date.now() < deadline) {
    if (hasExited(managed.child)) {
      throw new Error(`${managed.name} stopped with ${await managed.exit} before listening`);
    }
    if (await portAnswers(address)) {
      return;
    }
    await delay(pollIntervalMilliseconds);
  }
  throw new Error(`${managed.name} did not listen within ${String(readinessDeadlineMilliseconds)} ms`);
}

async function stop(managed: ManagedProcess): Promise<void> {
  if (hasExited(managed.child)) {
    throw new Error(`${managed.name} stopped unexpectedly with ${await managed.exit}`);
  }
  managed.child.kill('SIGTERM');
  const outcome = await Promise.race([managed.exit, delay(stopDeadlineMilliseconds).then(() => 'late')]);
  if (outcome === 'late') {
    managed.child.kill('SIGKILL');
    throw new Error(`${managed.name} did not stop within ${String(stopDeadlineMilliseconds)} ms`);
  }
  if (!managed.cleanExits.has(outcome)) {
    throw new Error(`${managed.name} stopped with ${outcome} after SIGTERM`);
  }
}

function failures(results: readonly PromiseSettledResult<void>[]): unknown[] {
  const reasons: unknown[] = [];
  for (const result of results) {
    if (result.status === 'rejected') {
      const reason: unknown = result.reason;
      reasons.push(reason);
    }
  }
  return reasons;
}

async function setSharedClock(apiUrl: string): Promise<void> {
  const response = await fetch(`${apiUrl}/__test__/clock`, {
    signal: AbortSignal.timeout(pollTimeoutMilliseconds),
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ set: new Date().toISOString() }),
  });
  if (!response.ok) {
    throw new Error(`setting the shared clock answered ${String(response.status)}`);
  }
}

export async function startDeployment(settings: E2ESettings, slot: number, label: string): Promise<Deployment> {
  mkdirSync(outputRoot, { recursive: true });
  const apiAddress = `127.0.0.1:${String(firstApiPort + slot)}`;
  const previewPort = firstPreviewPort + slot;
  const origin = `https://127.0.0.1:${String(previewPort)}`;
  const apiUrl = `http://${apiAddress}`;
  await assertAddressFree(apiAddress);
  await assertAddressFree(`127.0.0.1:${String(previewPort)}`);
  const connection = adminConnection(settings.adminUrl);
  const database = `ovc_e2e_${randomBytes(6).toString('hex')}_test`;
  runInPostgresContainer('createdb', connection, [database]);
  const environment = backendEnvironment(database, settings, apiAddress, origin);
  const processes: ManagedProcess[] = [];
  const teardown = async (): Promise<void> => {
    const stopped = await Promise.allSettled(processes.map((managed) => stop(managed)));
    const problems = failures(stopped);
    try {
      runInPostgresContainer('dropdb', connection, ['--force', database]);
    } catch (error) {
      problems.push(error);
    }
    if (problems.length > 0) {
      throw new AggregateError(problems, 'the end-to-end deployment did not stop cleanly');
    }
  };
  try {
    runToCompletion(join(binariesRoot, 'api'), ['migrate'], environment, `migrate-${label}-${String(slot)}.log`);
    const api = start('api', join(binariesRoot, 'api'), [], environment, `api-${label}-${String(slot)}.log`);
    processes.push(
      api,
      start('worker', join(binariesRoot, 'worker'), [], environment, `worker-${label}-${String(slot)}.log`),
    );
    await waitUntilOk(api, `${apiUrl}/api/v1/health/live`);
    await setSharedClock(apiUrl);
    await waitUntilOk(api, `${apiUrl}/api/v1/health/ready`);
    const preview = start(
      'preview',
      join(frontendRoot, 'node_modules', '.bin', 'vite'),
      ['preview', '--port', String(previewPort), '--strictPort'],
      {
        PATH: process.env.PATH,
        HOME: process.env.HOME,
        OVC_FRONTEND_API_TARGET: apiUrl,
        OVC_FRONTEND_PREVIEW_CERTIFICATE: previewCertificate,
        OVC_FRONTEND_PREVIEW_PRIVATE_KEY: previewPrivateKey,
      },
      `preview-${label}-${String(slot)}.log`,
      previewExits,
    );
    processes.push(preview);
    await waitUntilListening(preview, `127.0.0.1:${String(previewPort)}`);
  } catch (error) {
    const cleanup = await teardown().then(
      (): unknown[] => [],
      (teardownError: unknown) => [teardownError],
    );
    throw cleanup.length === 0
      ? error
      : new AggregateError([error, ...cleanup], 'the end-to-end deployment failed to start');
  }
  const administer = (args: readonly string[]): void => {
    runToCompletion(
      join(binariesRoot, 'ovc-admin'),
      [...args],
      adminEnvironment(environment),
      `admin-${label}-${String(slot)}-${randomBytes(4).toString('hex')}.log`,
    );
  };
  return { origin, apiUrl, administer, stop: teardown };
}
