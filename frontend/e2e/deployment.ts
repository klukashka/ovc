import { type ChildProcess, execFileSync, spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { closeSync, createWriteStream, mkdirSync, openSync } from 'node:fs';
import { join, resolve } from 'node:path';

export const apiAddress = '127.0.0.1:18766';
export const previewOrigin = 'http://127.0.0.1:4173';

const frontendRoot = resolve(import.meta.dirname, '..');
const repositoryRoot = resolve(frontendRoot, '..');
const backendRoot = resolve(repositoryRoot, 'backend');
const outputRoot = resolve(frontendRoot, 'test-results', 'deployment');
const readinessDeadlineMilliseconds = 30_000;
const stopDeadlineMilliseconds = 20_000;
const pollIntervalMilliseconds = 100;
const pollTimeoutMilliseconds = 2_000;
const loopbackHosts: ReadonlySet<string> = new Set(['127.0.0.1', '[::1]', 'localhost']);

interface AdminConnection {
  readonly user: string;
  readonly password: string;
}

export interface E2ESettings {
  readonly adminUrl: URL;
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
  return { adminUrl };
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

function composeEnvironment(connection: AdminConnection): NodeJS.ProcessEnv {
  return { ...process.env, PGPASSWORD: connection.password };
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
    { cwd: repositoryRoot, env: composeEnvironment(connection), stdio: ['ignore', 'ignore', 'inherit'] },
  );
}

function buildBinaries(): { api: string; worker: string } {
  const binaries = join(outputRoot, 'bin');
  mkdirSync(binaries, { recursive: true });
  const api = join(binaries, 'api');
  const worker = join(binaries, 'worker');
  execFileSync('go', ['build', '-o', api, './cmd/api'], { cwd: backendRoot, stdio: 'inherit' });
  execFileSync('go', ['build', '-o', worker, './cmd/worker'], { cwd: backendRoot, stdio: 'inherit' });
  return { api, worker };
}

function backendEnvironment(database: string, adminUrl: URL): NodeJS.ProcessEnv {
  return {
    OVC_HTTP_ADDR: apiAddress,
    OVC_DATABASE_URL: databaseUrl(adminUrl, database),
    OVC_DB_POOL_SIZE: '5',
    OVC_LOG_LEVEL: 'info',
    OVC_LOG_FORMAT: 'json',
    OVC_API_DOCS_ENABLED: 'false',
    OVC_TEST_CONTROL: '0',
    OVC_CLOCK: 'system',
    OVC_MEETING_PROVIDER: 'fake',
    OVC_MAIL_BACKEND: 'sink',
  };
}

interface ManagedProcess {
  readonly name: string;
  readonly child: ChildProcess;
  readonly exit: Promise<string>;
}

function exitDescription(code: number | null, signal: NodeJS.Signals | null): string {
  return signal === null ? `exit code ${String(code)}` : `signal ${signal}`;
}

function hasExited(child: ChildProcess): boolean {
  return child.exitCode !== null || child.signalCode !== null;
}

function start(name: string, binary: string, environment: NodeJS.ProcessEnv, logName: string): ManagedProcess {
  const log = createWriteStream(join(outputRoot, logName));
  const child = spawn(binary, [], { env: environment, stdio: ['ignore', 'pipe', 'pipe'] });
  child.stdout.pipe(log);
  child.stderr.pipe(log);
  const exit = new Promise<string>((resolveExit) => {
    child.once('exit', (code, signal) => {
      resolveExit(exitDescription(code, signal));
    });
    child.once('error', (error) => {
      resolveExit(`spawn error ${error.message}`);
    });
  });
  return { name, child, exit };
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

async function waitForReadiness(api: ManagedProcess): Promise<void> {
  const deadline = Date.now() + readinessDeadlineMilliseconds;
  while (Date.now() < deadline) {
    if (hasExited(api.child)) {
      throw new Error(`the API stopped with ${await api.exit} before becoming ready`);
    }
    const ready = await fetch(`http://${apiAddress}/api/v1/health/ready`, {
      signal: AbortSignal.timeout(pollTimeoutMilliseconds),
    }).then(
      (response) => response.ok,
      () => false,
    );
    if (ready) {
      return;
    }
    await delay(pollIntervalMilliseconds);
  }
  throw new Error(`the API did not become ready within ${String(readinessDeadlineMilliseconds)} ms`);
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
  if (outcome !== 'exit code 0') {
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

export async function startDeployment(settings: E2ESettings): Promise<() => Promise<void>> {
  mkdirSync(outputRoot, { recursive: true });
  const connection = adminConnection(settings.adminUrl);
  const database = `ovc_e2e_${randomBytes(6).toString('hex')}_test`;
  const binaries = buildBinaries();
  runInPostgresContainer('createdb', connection, [database]);
  const environment = backendEnvironment(database, settings.adminUrl);
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
    runToCompletion(binaries.api, ['migrate'], environment, 'migrate.log');
    const api = start('api', binaries.api, environment, 'api.log');
    processes.push(api, start('worker', binaries.worker, environment, 'worker.log'));
    await waitForReadiness(api);
  } catch (error) {
    const cleanup = await teardown().then(
      (): unknown[] => [],
      (teardownError: unknown) => [teardownError],
    );
    throw cleanup.length === 0
      ? error
      : new AggregateError([error, ...cleanup], 'the end-to-end deployment failed to start');
  }
  return teardown;
}
