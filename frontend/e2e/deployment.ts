import { type ChildProcess, execFileSync, spawn, spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import {
  chmodSync,
  closeSync,
  createWriteStream,
  mkdirSync,
  lstatSync,
  mkdtempSync,
  openSync,
  readdirSync,
  rmSync,
} from 'node:fs';
import { createConnection } from 'node:net';
import { request as httpsRequest } from 'node:https';
import { tmpdir, userInfo } from 'node:os';
import { join, resolve } from 'node:path';

import {
  type RunLockHolder,
  acquireRunLock,
  adoptGateRunLock,
  gateRunLockDescriptorVariable,
  holdsRunLock,
  machineRunLockPath,
  parseGateRunLockDescriptor,
  processAncestors,
  releaseRunLock,
} from './run-lock.ts';
import { type RoomOriginServer, startRoomOrigin } from './room-origin.ts';
import { assertOutputRootIsReal, playwrightOutputRoot } from './run-output.ts';

const frontendRoot = resolve(import.meta.dirname, '..');
const repositoryRoot = resolve(frontendRoot, '..');
const backendRoot = resolve(repositoryRoot, 'backend');
const outputRoot = resolve(frontendRoot, 'test-results', 'deployment');
const binariesRoot = join(outputRoot, 'bin');
const tlsRoot = join(outputRoot, 'tls');
const runLockPath = machineRunLockPath(process.platform, userInfo().homedir);
const runOutputsRoot = playwrightOutputRoot(frontendRoot);
const previewCertificate = join(tlsRoot, 'preview-certificate.pem');
const previewPrivateKey = join(tlsRoot, 'preview-private-key.pem');
export const readinessDeadlineMilliseconds = 30_000;
const stopDeadlineMilliseconds = 20_000;
const killDeadlineMilliseconds = 5_000;
const pollIntervalMilliseconds = 100;
const pollTimeoutMilliseconds = 2_000;
const firstApiPort = 18_800;
const firstPreviewPort = 4_300;
const firstRoomPort = 18_900;
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

export interface DevelopmentLifecycleObserver {
  readonly processStarted: (pid: number) => void;
  readonly temporaryTlsCreated: (directory: string) => void;
  readonly beforeFrontendStart?: (port: number) => Promise<void>;
}

export interface MeasurementDeploymentOptions {
  readonly synchronousCommit: 'on' | 'off';
}

export interface MeasurementPeerStopOptions {
  readonly shutdownWindow: 'upload_drain';
}

export interface MeasurementPeer {
  readonly apiUrl: string;
  readonly pid: number;
  readonly stop: (options?: MeasurementPeerStopOptions) => Promise<void>;
}

export interface Deployment {
  readonly origin: string;
  readonly apiUrl: string;
  readonly roomOrigin: string;
  readonly administer: (args: readonly string[]) => void;
  readonly startMeasurementPeer?: () => Promise<MeasurementPeer>;
  readonly measurementProcessIds?: () => readonly number[];
  readonly measurementQuery?: (sql: string) => string;
  readonly holdMeasurementPresence?: (user: string) => Promise<() => Promise<void>>;
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

function runInPostgresContainer(
  program: 'createdb' | 'dropdb' | 'psql',
  connection: AdminConnection,
  args: string[],
): string {
  return execFileSync(
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
      stdio: ['ignore', 'pipe', 'inherit'],
      encoding: 'utf8',
    },
  );
}

function ensureStorageBucket(storage: StorageSettings): void {
  const result = spawnSync(join(binariesRoot, 'api'), ['ensure-bucket'], {
    cwd: backendRoot,
    env: {
      PATH: process.env.PATH ?? '',
      OVC_STORAGE_BACKEND: 's3',
      OVC_STORAGE_ENDPOINT: storage.endpoint,
      OVC_STORAGE_REGION: 'us-east-1',
      OVC_STORAGE_BUCKET: storageBucket,
      OVC_STORAGE_ACCESS_KEY: storage.accessKey,
      OVC_STORAGE_SECRET_KEY: storage.secretKey,
    },
    encoding: 'utf8',
  });
  if (result.status !== 0 || result.stdout !== 'ok\n') {
    throw new Error(`the storage bucket ${storageBucket} could not be created with its lifecycle rule`);
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

export function createFixtureCertificate(root: string): { readonly certificate: string; readonly privateKey: string } {
  const certificate = join(root, 'preview-certificate.pem');
  const privateKey = join(root, 'preview-private-key.pem');
  mkdirSync(root, { recursive: true, mode: 0o700 });
  chmodSync(root, 0o700);
  try {
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
        privateKey,
        '-out',
        certificate,
      ],
      { stdio: 'ignore' },
    );
  } catch {
    throw new Error('fixture_tls_generation_failed');
  }
  chmodSync(root, 0o700);
  chmodSync(privateKey, 0o600);
  return { certificate, privateKey };
}

const runLock: { holder: RunLockHolder | undefined; owned: boolean } = { holder: undefined, owned: false };

function assertStillHeld(holder: RunLockHolder): void {
  if (!holdsRunLock(runLockPath, holder)) {
    throw new Error(
      `${runLockPath} no longer holds this run (pid ${String(holder.pid)}, started ${holder.startedAt}); another run took it, so this run stops instead of touching the shared deployment files and bucket`,
    );
  }
}

export function takeRunLock(environment: NodeJS.ProcessEnv): void {
  const descriptor = parseGateRunLockDescriptor(environment[gateRunLockDescriptorVariable]);
  if (descriptor !== undefined) {
    const gateHolder = adoptGateRunLock(runLockPath, descriptor, processAncestors(process.pid), repositoryRoot);
    process.stderr.write(
      `running under the end-to-end run lock ${runLockPath} of the merge gate pid ${String(gateHolder.pid)}, which releases it\n`,
    );
    runLock.holder = gateHolder;
    runLock.owned = false;
    assertStillHeld(gateHolder);
    return;
  }
  const holder = { pid: process.pid, startedAt: new Date().toISOString(), checkout: repositoryRoot };
  acquireRunLock(runLockPath, holder, (message) => {
    process.stderr.write(`${message}\n`);
  });
  runLock.holder = holder;
  runLock.owned = true;
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
    if (runLock.owned) {
      releaseRunLock(runLockPath, holder);
    }
    runLock.holder = undefined;
  }
}

export function assertFrontendPreparation(
  frontendMode: 'preview' | 'measurement-dev-tls',
  sourceTlsRoot: string,
): void {
  if (frontendMode === 'measurement-dev-tls' && lstatSync(sourceTlsRoot, { throwIfNoEntry: false }) !== undefined) {
    throw new Error('development_fixture_refuses_existing_source_tls_material');
  }
}

export function prepareRun(frontendMode: 'preview' | 'measurement-dev-tls' = 'preview'): void {
  assertFrontendPreparation(frontendMode, tlsRoot);
  mkdirSync(binariesRoot, { recursive: true });
  if (frontendMode === 'preview') createFixtureCertificate(tlsRoot);
  execFileSync('go', ['build', '-o', `${binariesRoot}/`, './cmd/api', './cmd/worker', './cmd/ovc-admin'], {
    cwd: backendRoot,
    stdio: 'inherit',
  });
  ensureStorageBucket(readStorageSettings(process.env));
  execFileSync('pnpm', ['build'], { cwd: frontendRoot, stdio: 'inherit' });
}

function backendEnvironment(
  database: string,
  settings: E2ESettings,
  apiAddress: string,
  origin: string,
  roomOrigin: string,
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
    OVC_FAKE_ROOM_ORIGIN: roomOrigin,
    OVC_JWT_SECRET: randomBytes(32).toString('hex'),
    OVC_FIELD_ENCRYPTION_KEY: randomBytes(32).toString('base64'),
    OVC_UNSUBSCRIBE_SIGNING_SECRET: randomBytes(32).toString('base64'),
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

async function waitUntilDevelopmentReady(managed: ManagedProcess, origin: string): Promise<void> {
  const deadline = Date.now() + readinessDeadlineMilliseconds;
  while (Date.now() < deadline) {
    if (hasExited(managed.child)) throw new Error('development_server_exited_before_https_readiness');
    const ready = await new Promise<boolean>((resolveReady) => {
      const request = httpsRequest(
        `${origin}/api/v1/health/live`,
        { rejectUnauthorized: false, timeout: pollTimeoutMilliseconds },
        (response) => {
          response.resume();
          resolveReady(response.statusCode === 200);
        },
      );
      request.once('timeout', () => {
        request.destroy();
      });
      request.once('error', () => {
        resolveReady(false);
      });
      request.end();
    });
    if (ready) return;
    await delay(pollIntervalMilliseconds);
  }
  throw new Error('development_server_https_readiness_deadline');
}

type StopWindow = 'default' | 'measurement_upload_drain';
type StopOutcome = 'already_exited' | 'clean_exit' | 'unexpected_exit' | 'forced_exit' | 'kill_unconfirmed';

async function stop(
  managed: ManagedProcess,
  window: StopWindow = 'default',
  observe?: (outcome: StopOutcome) => void,
): Promise<void> {
  const deadline = window === 'measurement_upload_drain' ? 45_000 : stopDeadlineMilliseconds;
  if (hasExited(managed.child)) {
    observe?.('already_exited');
    throw new Error(`${managed.name} stopped unexpectedly with ${await managed.exit}`);
  }
  managed.child.kill('SIGTERM');
  const outcome = await Promise.race([managed.exit, delay(deadline).then(() => 'late')]);
  if (outcome === 'late') {
    managed.child.kill('SIGKILL');
    const killed = await Promise.race([managed.exit, delay(killDeadlineMilliseconds).then(() => 'late')]);
    if (killed === 'late') {
      observe?.('kill_unconfirmed');
      throw new Error(
        `the ${managed.name} process (pid ${String(managed.child.pid)}) did not stop within ${String(deadline)} ms of SIGTERM and did not exit within ${String(killDeadlineMilliseconds)} ms of SIGKILL, so its address may still be held`,
      );
    }
    observe?.('forced_exit');
    throw new Error(
      `the ${managed.name} process did not stop within ${String(deadline)} ms of SIGTERM and exited with ${killed} after SIGKILL`,
    );
  }
  if (!managed.cleanExits.has(outcome)) {
    observe?.('unexpected_exit');
    throw new Error(`${managed.name} stopped with ${outcome} after SIGTERM`);
  }
  observe?.('clean_exit');
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

async function setSharedClock(apiUrl: string, instant: string): Promise<void> {
  const response = await fetch(`${apiUrl}/__test__/clock`, {
    signal: AbortSignal.timeout(pollTimeoutMilliseconds),
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ set: instant }),
  });
  if (!response.ok) {
    throw new Error(`setting the shared clock answered ${String(response.status)}`);
  }
}

export async function startDeployment(
  settings: E2ESettings,
  slot: number,
  label: string,
  measurement?: MeasurementDeploymentOptions,
  frontendMode: 'preview' | 'measurement-dev-tls' = 'preview',
  lifecycle?: DevelopmentLifecycleObserver,
): Promise<Deployment> {
  assertFrontendPreparation(frontendMode, tlsRoot);
  mkdirSync(outputRoot, { recursive: true });
  const apiAddress = `127.0.0.1:${String(firstApiPort + slot)}`;
  const previewPort = firstPreviewPort + slot;
  const origin = `https://127.0.0.1:${String(previewPort)}`;
  const apiUrl = `http://${apiAddress}`;
  const roomPort = firstRoomPort + slot;
  const roomOrigin = `https://127.0.0.1:${String(roomPort)}`;
  await assertAddressFree(apiAddress);
  await assertAddressFree(`127.0.0.1:${String(previewPort)}`);
  await assertAddressFree(`127.0.0.1:${String(roomPort)}`);
  const connection = adminConnection(settings.adminUrl);
  const database = `ovc_e2e_${randomBytes(6).toString('hex')}_test`;
  runInPostgresContainer('createdb', connection, [database]);
  const environment = backendEnvironment(database, settings, apiAddress, origin, roomOrigin);
  const processes: ManagedProcess[] = [];
  let roomServer: RoomOriginServer | undefined;
  let temporaryTls: string | undefined;
  const heldMeasurements = new Set<() => Promise<void>>();
  const teardown = async (): Promise<void> => {
    const released = await Promise.allSettled([...heldMeasurements].map((release) => release()));
    const room = await Promise.allSettled(roomServer === undefined ? [] : [roomServer.stop()]);
    const stopped = await Promise.allSettled(processes.map((managed) => stop(managed)));
    const problems = [...failures(released), ...failures(room), ...failures(stopped)];
    if (temporaryTls !== undefined && processes.every((managed) => hasExited(managed.child))) {
      try {
        rmSync(temporaryTls, { recursive: true });
      } catch {
        problems.push(new Error('development_fixture_tls_cleanup_failed'));
      }
    }
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
    if (measurement !== undefined) {
      configureMeasurementDatabase(settings, database, measurement.synchronousCommit);
    }
    runToCompletion(join(binariesRoot, 'api'), ['migrate'], environment, `migrate-${label}-${String(slot)}.log`);
    const activation = Number(
      runInPostgresContainer('psql', connection, [
        '--dbname',
        database,
        '--no-psqlrc',
        '--tuples-only',
        '--no-align',
        '--set',
        'ON_ERROR_STOP=1',
        '--command',
        'SELECT (extract(epoch FROM active_at) * 1000)::bigint FROM chat_upload_accounting_activation',
      ]).trim(),
    );
    if (!Number.isFinite(activation) || activation <= 0) {
      throw new Error('the migrated database has no valid daily upload activation');
    }
    const initialClock = new Date(Math.max(Date.now(), activation)).toISOString();
    const api = start('api', join(binariesRoot, 'api'), [], environment, `api-${label}-${String(slot)}.log`);
    processes.push(
      api,
      start('worker', join(binariesRoot, 'worker'), [], environment, `worker-${label}-${String(slot)}.log`),
    );
    for (const managed of processes) {
      const pid = managed.child.pid;
      if (pid !== undefined) lifecycle?.processStarted(pid);
    }
    await waitUntilOk(api, `${apiUrl}/api/v1/health/live`);
    await setSharedClock(apiUrl, initialClock);
    await waitUntilOk(api, `${apiUrl}/api/v1/health/ready`);
    let tls = { certificate: previewCertificate, privateKey: previewPrivateKey };
    if (frontendMode === 'measurement-dev-tls') {
      temporaryTls = mkdtempSync(join(tmpdir(), 'ovc-dev-tls-'));
      lifecycle?.temporaryTlsCreated(temporaryTls);
      chmodSync(temporaryTls, 0o700);
      tls = createFixtureCertificate(temporaryTls);
    }
    roomServer = await startRoomOrigin(tls, roomPort, firstApiPort + slot);
    await lifecycle?.beforeFrontendStart?.(previewPort);
    const preview = start(
      frontendMode === 'preview' ? 'preview' : 'development',
      join(frontendRoot, 'node_modules', '.bin', 'vite'),
      [
        ...(frontendMode === 'preview' ? ['preview'] : ['--mode', 'measurement-dev-tls']),
        '--port',
        String(previewPort),
        '--strictPort',
      ],
      {
        PATH: process.env.PATH,
        HOME: process.env.HOME,
        OVC_FRONTEND_API_TARGET: apiUrl,
        OVC_FRONTEND_TEST_ROOM_ORIGIN: roomOrigin,
        OVC_FRONTEND_PREVIEW_CERTIFICATE: tls.certificate,
        OVC_FRONTEND_PREVIEW_PRIVATE_KEY: tls.privateKey,
      },
      `preview-${label}-${String(slot)}.log`,
      previewExits,
    );
    processes.push(preview);
    if (preview.child.pid !== undefined) lifecycle?.processStarted(preview.child.pid);
    if (frontendMode === 'preview') await waitUntilListening(preview, `127.0.0.1:${String(previewPort)}`);
    else await waitUntilDevelopmentReady(preview, origin);
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
  let measurementPeerOrdinal = 0;
  const startMeasurementPeer = async (): Promise<MeasurementPeer> => {
    if (measurement === undefined) {
      throw new Error('peer processes require an opt-in measurement deployment');
    }
    const peerAddress = `127.0.0.1:${String(firstApiPort + 1000 + slot)}`;
    await assertAddressFree(peerAddress);
    measurementPeerOrdinal += 1;
    const peerOrdinal = measurementPeerOrdinal;
    const peer = start(
      'measurement-api-peer',
      join(binariesRoot, 'api'),
      [],
      { ...environment, OVC_HTTP_ADDR: peerAddress },
      `peer-${label}-${String(slot)}-${String(measurementPeerOrdinal)}.log`,
    );
    processes.push(peer);
    const peerUrl = `http://${peerAddress}`;
    await waitUntilOk(peer, `${peerUrl}/api/v1/health/ready`);
    const pid = peer.child.pid;
    if (pid === undefined) {
      throw new Error('measurement peer has no process identifier');
    }
    return {
      apiUrl: peerUrl,
      pid,
      stop: async (options?: MeasurementPeerStopOptions): Promise<void> => {
        const window: StopWindow = options?.shutdownWindow === 'upload_drain' ? 'measurement_upload_drain' : 'default';
        try {
          await stop(peer, window, (outcome) => {
            process.stdout.write(
              `${JSON.stringify({ event: 'measurement.peer_stop', ordinal: peerOrdinal, outcome, window, exitCode: peer.child.exitCode ?? 'unavailable', signal: peer.child.signalCode ?? 'none' })}\n`,
            );
          });
        } finally {
          if (hasExited(peer.child)) processes.splice(processes.indexOf(peer), 1);
        }
      },
    };
  };
  return {
    origin,
    apiUrl,
    roomOrigin,
    administer,
    stop: teardown,
    ...(measurement === undefined
      ? {}
      : {
          startMeasurementPeer,
          holdMeasurementPresence: async (user: string): Promise<() => Promise<void>> => {
            const release = await holdMeasurementPresence(settings, database, user);
            const tracked = async (): Promise<void> => {
              try {
                await release();
              } finally {
                heldMeasurements.delete(tracked);
              }
            };
            heldMeasurements.add(tracked);
            return tracked;
          },
          measurementQuery: (sql: string): string => {
            assertMeasurementDatabase(database);
            return measurementQuery(settings, database, sql);
          },
          measurementProcessIds: (): readonly number[] =>
            processes.flatMap((managed) => (managed.child.pid === undefined ? [] : [managed.child.pid])),
        }),
  };
}

export function assertMeasurementDatabase(database: string): void {
  if (!/^ovc_e2e_[a-z0-9]+_test$/.test(database)) {
    throw new Error('measurement settings require a newly owned end-to-end database');
  }
}

function configureMeasurementDatabase(settings: E2ESettings, database: string, mode: 'on' | 'off'): void {
  assertMeasurementDatabase(database);
  if (!loopbackHosts.has(settings.adminUrl.hostname)) {
    throw new Error('measurement databases must be on loopback');
  }
  const query = (target: string, sql: string): string => measurementQuery(settings, target, sql);
  const adminDatabase = decodeURIComponent(settings.adminUrl.pathname.slice(1));
  const before = query(adminDatabase, 'SHOW synchronous_commit');
  const owned = query(
    adminDatabase,
    `SELECT pg_get_userbyid(datdba) = current_user FROM pg_database WHERE datname = '${database}'`,
  );
  if (owned !== 't') {
    throw new Error('measurement database is not owned by the fixture administrator');
  }
  query(adminDatabase, `ALTER DATABASE "${database}" SET synchronous_commit = '${mode}'`);
  const observed = query(database, 'SHOW synchronous_commit');
  const after = query(adminDatabase, 'SHOW synchronous_commit');
  if (observed !== mode || before !== after) {
    throw new Error('measurement database setting escaped confinement or was not applied');
  }
  process.stdout.write(
    `${JSON.stringify({ event: 'measurement.database', mode, observed, adminBefore: before, adminAfter: after })}\n`,
  );
}

function measurementQuery(settings: E2ESettings, target: string, sql: string): string {
  if (!loopbackHosts.has(settings.adminUrl.hostname)) {
    throw new Error('measurement queries require loopback');
  }
  const connection = adminConnection(settings.adminUrl);
  const result = spawnSync(
    'docker',
    [
      'compose',
      'exec',
      '-T',
      '-e',
      'PGPASSWORD',
      'postgres',
      'psql',
      '-X',
      '-At',
      '-v',
      'ON_ERROR_STOP=1',
      '-h',
      '127.0.0.1',
      '-U',
      connection.user,
      '-d',
      target,
    ],
    { cwd: repositoryRoot, env: { ...process.env, PGPASSWORD: connection.password }, input: sql, encoding: 'utf8' },
  );
  if (result.error !== undefined || result.status !== 0) {
    throw new Error('measurement database operation failed');
  }
  return result.stdout.trim();
}

async function holdMeasurementPresence(
  settings: E2ESettings,
  database: string,
  user: string,
): Promise<() => Promise<void>> {
  assertMeasurementDatabase(database);
  if (
    !loopbackHosts.has(settings.adminUrl.hostname) ||
    !/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(user)
  )
    throw new Error('measurement lock target invalid');
  const connection = adminConnection(settings.adminUrl);
  const child = spawn(
    'docker',
    [
      'compose',
      'exec',
      '-T',
      '-e',
      'PGPASSWORD',
      'postgres',
      'psql',
      '-X',
      '-At',
      '-v',
      'ON_ERROR_STOP=1',
      '-h',
      '127.0.0.1',
      '-U',
      connection.user,
      '-d',
      database,
    ],
    {
      cwd: repositoryRoot,
      env: { ...process.env, PGPASSWORD: connection.password },
      stdio: ['pipe', 'pipe', 'ignore'],
    },
  );
  let inputFailed = false;
  child.stdin.on('error', () => {
    inputFailed = true;
  });
  const exited = new Promise<number | null>((resolveExit) => {
    child.once('exit', resolveExit);
    child.once('error', () => {
      resolveExit(null);
    });
  });
  const awaitExit = async (milliseconds: number): Promise<number | null | 'late'> => {
    let timeout: ReturnType<typeof setTimeout> | undefined;
    try {
      return await Promise.race([
        exited,
        new Promise<'late'>((resolveLate) => {
          timeout = setTimeout(() => {
            resolveLate('late');
          }, milliseconds);
        }),
      ]);
    } finally {
      clearTimeout(timeout);
    }
  };
  let releasePromise: Promise<void> | undefined;
  const release = (): Promise<void> => {
    releasePromise ??= (async (): Promise<void> => {
      if (!child.stdin.destroyed) child.stdin.end('COMMIT;\n\\q\n');
      const code = await awaitExit(15_000);
      if (code === 'late') {
        child.kill('SIGTERM');
        if ((await awaitExit(5_000)) === 'late') {
          child.kill('SIGKILL');
          if ((await awaitExit(5_000)) === 'late')
            throw new Error('measurement lock child retained after forced cleanup');
        }
        throw new Error('measurement row lock cleanup required process termination');
      }
      if (code !== 0 || inputFailed) throw new Error('measurement row lock did not release cleanly');
    })();
    return releasePromise;
  };
  let timeout: ReturnType<typeof setTimeout> | undefined;
  const ready = new Promise<void>((resolveReady, rejectReady) => {
    timeout = setTimeout(() => {
      rejectReady(new Error('measurement row lock deadline'));
    }, 10_000);
    let output = '';
    child.stdout.on('data', (chunk: Buffer) => {
      output += chunk.toString('utf8');
      if (output.includes('measurement-locked')) resolveReady();
    });
    child.once('error', () => {
      rejectReady(new Error('measurement row lock process failed'));
    });
    child.stdin.once('error', () => {
      rejectReady(new Error('measurement row lock input failed'));
    });
    child.once('exit', () => {
      rejectReady(new Error('measurement row lock exited before readiness'));
    });
  });
  child.stdin.write(
    `SET statement_timeout='10s';\nSET idle_in_transaction_session_timeout='30s';\nBEGIN;\nSELECT 1 FROM user_presence WHERE user_id='${user}'::uuid FOR UPDATE;\n\\echo measurement-locked\n`,
  );
  try {
    await ready;
  } catch (error) {
    try {
      await release();
    } catch {
      throw new Error('measurement lock setup and cleanup failed');
    }
    throw error;
  } finally {
    clearTimeout(timeout);
  }
  return release;
}
