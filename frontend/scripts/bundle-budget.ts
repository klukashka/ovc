import { resolve } from 'node:path';

export interface ManifestChunk {
  readonly file: string;
  readonly name: string | null;
  readonly isEntry: boolean;
  readonly isDynamicEntry: boolean;
  readonly imports: readonly string[];
  readonly css: readonly string[];
}

export type Manifest = ReadonlyMap<string, ManifestChunk>;

export interface Budget {
  readonly name: string;
  readonly pages: readonly string[];
  readonly limit: number;
}

export interface BundleReport {
  readonly lines: readonly string[];
  readonly failures: readonly string[];
}

export const buildManifestPath = resolve(import.meta.dirname, '..', 'build-report', 'manifest.json');

export const kilobyte = 1000;
export const entryLimit = 180 * kilobyte;
export const entryKey = 'index.html';

export interface FirstLoadModule {
  readonly chunk: string;
  readonly module: string;
}

export const firstLoadModules: readonly FirstLoadModule[] = [
  { chunk: 'access-gate', module: 'src/routes/Gate.tsx' },
  { chunk: 'access-rule', module: 'src/routes/access.ts' },
  { chunk: 'address-token', module: 'src/features/auth/AddressToken.tsx' },
  { chunk: 'invitation-token', module: 'src/features/auth/InvitationToken.tsx' },
];

export const firstLoadBudgets: readonly Budget[] = [
  { name: "a student's Home", pages: ['src/features/home/HomePage.tsx'], limit: 250 * kilobyte },
  { name: "a tutor's Today", pages: ['src/features/home/TodayPage.tsx'], limit: 400 * kilobyte },
];

function strings(value: unknown, field: string, key: string): string[] {
  if (value === undefined) {
    return [];
  }
  if (!Array.isArray(value) || !value.every((item): item is string => typeof item === 'string')) {
    throw new Error(`manifest entry ${key} has a malformed ${field}`);
  }
  return value;
}

function flag(value: unknown, field: string, key: string): boolean {
  if (value === undefined) {
    return false;
  }
  if (typeof value !== 'boolean') {
    throw new Error(`manifest entry ${key} has a malformed ${field}`);
  }
  return value;
}

export function parseManifest(json: unknown): Manifest {
  if (typeof json !== 'object' || json === null || Array.isArray(json)) {
    throw new Error('the build manifest is not an object');
  }
  const chunks = new Map<string, ManifestChunk>();
  const entries: [string, unknown][] = Object.entries(json);
  for (const [key, value] of entries) {
    if (typeof value !== 'object' || value === null || !('file' in value) || typeof value.file !== 'string') {
      throw new Error(`manifest entry ${key} has no file`);
    }
    const name = 'name' in value ? value.name : undefined;
    if (name !== undefined && typeof name !== 'string') {
      throw new Error(`manifest entry ${key} has a malformed name`);
    }
    chunks.set(key, {
      file: value.file,
      name: name ?? null,
      isEntry: flag('isEntry' in value ? value.isEntry : undefined, 'isEntry', key),
      isDynamicEntry: flag('isDynamicEntry' in value ? value.isDynamicEntry : undefined, 'isDynamicEntry', key),
      imports: strings('imports' in value ? value.imports : undefined, 'imports', key),
      css: strings('css' in value ? value.css : undefined, 'css', key),
    });
  }
  return chunks;
}

function chunkAt(manifest: Manifest, key: string): ManifestChunk {
  const chunk = manifest.get(key);
  if (chunk === undefined) {
    throw new Error(`the build manifest has no entry ${key}`);
  }
  return chunk;
}

export function staticFiles(manifest: Manifest, keys: readonly string[]): Set<string> {
  const files = new Set<string>();
  const visited = new Set<string>();
  const pending = [...keys];
  for (let key = pending.pop(); key !== undefined; key = pending.pop()) {
    if (visited.has(key)) {
      continue;
    }
    visited.add(key);
    const chunk = chunkAt(manifest, key);
    files.add(chunk.file);
    chunk.css.forEach((file) => files.add(file));
    pending.push(...chunk.imports);
  }
  return files;
}

function total(files: ReadonlySet<string>, sizeOf: (file: string) => number): number {
  return [...files].reduce((sum, file) => sum + sizeOf(file), 0);
}

function kilobytes(bytes: number): string {
  return `${(bytes / kilobyte).toFixed(1)} KB`;
}

function inFirstLoad(manifest: Manifest, initial: ReadonlySet<string>, chunk: string): boolean {
  const named = [...manifest.values()].filter((candidate) => candidate.name === chunk);
  return named.length === 1 && named.every((candidate) => initial.has(candidate.file) && !candidate.isDynamicEntry);
}

export function checkBundle(
  manifest: Manifest,
  pageModules: readonly string[],
  sizeOf: (file: string) => number,
): BundleReport {
  const lines: string[] = [];
  const failures: string[] = [];
  const entry = chunkAt(manifest, entryKey);
  if (!entry.isEntry) {
    failures.push(`${entryKey} is not an entry of the build`);
  }
  const initial = staticFiles(manifest, [entryKey]);
  const entrySize = total(new Set([...initial].filter((file) => file.endsWith('.js'))), sizeOf);
  lines.push(
    `entry and its static imports: ${kilobytes(entrySize)} gzip of JavaScript (limit ${kilobytes(entryLimit)})`,
  );
  if (entrySize > entryLimit) {
    failures.push(
      `the entry and its static imports are ${kilobytes(entrySize)} gzip of JavaScript, above ${kilobytes(entryLimit)}`,
    );
  }
  for (const budget of firstLoadBudgets) {
    const size = total(staticFiles(manifest, [entryKey, ...budget.pages]), sizeOf);
    lines.push(`first load of ${budget.name}: ${kilobytes(size)} gzip (limit ${kilobytes(budget.limit)})`);
    if (size > budget.limit) {
      failures.push(`the first load of ${budget.name} is ${kilobytes(size)} gzip, above ${kilobytes(budget.limit)}`);
    }
  }
  for (const page of pageModules) {
    const chunk = manifest.get(page);
    if (chunk === undefined || !chunk.isDynamicEntry || initial.has(chunk.file)) {
      failures.push(`${page} is not loaded on demand`);
    }
  }
  lines.push(`${String(pageModules.length)} page modules load on demand`);
  for (const { chunk, module } of firstLoadModules) {
    if (!inFirstLoad(manifest, initial, chunk)) {
      failures.push(`${module} is not part of the first load in its own chunk ${chunk}`);
    }
  }
  return { lines, failures };
}
