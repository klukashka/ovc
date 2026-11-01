import { execFileSync } from 'node:child_process';

import { performance } from 'node:perf_hooks';

import { testNetAddress } from '../e2e/accounts.ts';
import type { Deployment } from '../e2e/deployment.ts';
import { type SocketFactory, realtimeAddress, webSocketFactory } from '../src/lib/realtime/socket.ts';

type MeasurementWebSocketConstructor = new (
  address: string,
  init: { readonly headers: Readonly<Record<string, string>> },
) => WebSocket;

const MeasurementWebSocket = WebSocket as unknown as MeasurementWebSocketConstructor;

export function measurementSocketFactory(deployment: Deployment): SocketFactory {
  return (events) =>
    webSocketFactory(() => {
      const native = new MeasurementWebSocket(realtimeAddress(deployment.apiUrl), {
        headers: { Origin: deployment.origin, 'X-Forwarded-For': testNetAddress() },
      });
      native.addEventListener(
        'close',
        (event) => {
          events.onClose(event.code);
        },
        { once: true },
      );
      return native;
    })({ ...events, onClose: () => undefined });
}

export function recordMeasurement(record: Readonly<Record<string, string | number | boolean>>): void {
  process.stdout.write(`${JSON.stringify(record)}\n`);
}

export async function untilMeasurementCondition(
  condition: () => boolean | Promise<boolean>,
  deadlineMilliseconds: number,
): Promise<void> {
  const deadline = performance.now() + deadlineMilliseconds;
  while (!(await condition())) {
    if (performance.now() >= deadline) {
      throw new Error('measurement cleanup deadline exceeded');
    }
    await new Promise<void>((resolve) => setTimeout(resolve, 10));
  }
}

export function requiredMeasurementString(value: unknown, field: string): string {
  if (typeof value !== 'object' || value === null || !(field in value)) {
    throw new Error('measurement response is missing a required field');
  }
  const found: unknown = (value as Record<string, unknown>)[field];
  if (typeof found !== 'string') {
    throw new Error('measurement response field has an unexpected type');
  }
  return found;
}

export function measurementIdentifier(value: string): string {
  if (!/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(value)) {
    throw new Error('measurement identifier is not a UUID');
  }
  return value;
}

export function browserProcessMemory(rootPid: number): { readonly processes: number; readonly rssKiB: number } {
  const output = execFileSync('ps', ['-axo', 'pid=,ppid=,rss=,comm='], { encoding: 'utf8' });
  const rows = output
    .trim()
    .split('\n')
    .map((line) => {
      const match = /^\s*(\d+)\s+(\d+)\s+(\d+)\s+(.+)$/.exec(line);
      if (match === null) throw new Error('measurement process table malformed');
      return {
        pid: Number(match[1]),
        parent: Number(match[2]),
        rssKiB: Number(match[3]),
        browser: /chromium|chrome|firefox|webkit|webcontent|minibrowser/i.test(match[4] ?? ''),
      };
    });
  const owned = new Set([rootPid]);
  for (let before = -1; before !== owned.size;) {
    before = owned.size;
    for (const row of rows) if (owned.has(row.parent)) owned.add(row.pid);
  }
  const browsers = rows.filter((row) => owned.has(row.pid));
  return { processes: browsers.length, rssKiB: browsers.reduce((sum, row) => sum + row.rssKiB, 0) };
}

export const measurementSmoke = process.argv.includes('--smoke');

export function measurementCount(full: number, smoke: number): number {
  return measurementSmoke ? smoke : full;
}

export async function measurementScope(
  run: (cleanup: (release: () => Promise<void>) => void) => Promise<void>,
): Promise<void> {
  const releases: (() => Promise<void>)[] = [];
  const problems: unknown[] = [];
  try {
    await run((release) => {
      releases.push(release);
    });
  } catch (error) {
    problems.push(error);
  } finally {
    for (const release of releases.reverse()) {
      try {
        await release();
      } catch (error) {
        problems.push(error);
      }
    }
  }
  if (problems.length > 0) throw new AggregateError(problems, 'measurement scope failed');
}
