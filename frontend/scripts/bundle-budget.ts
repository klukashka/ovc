import { resolve } from 'node:path';

import { type Language, languages } from '../src/lib/i18n/languages.ts';

export interface ManifestChunk {
  readonly file: string;
  readonly name: string | null;
  readonly isEntry: boolean;
  readonly isDynamicEntry: boolean;
  readonly imports: readonly string[];
  readonly dynamicImports: readonly string[];
  readonly css: readonly string[];
  readonly assets: readonly string[];
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

export interface LanguageChunk {
  readonly language: Language;
  readonly module: string;
}

export const languageChunks: readonly LanguageChunk[] = languages.map((language) => ({
  language,
  module: `src/lib/i18n/${language}.ts`,
}));

export const languageChunkPattern = new RegExp(`^assets/language-(${languages.join('|')})-[^/]+\\.js$`);

export function languageChunkFileName(language: Language): string {
  return `assets/language-${language}-[hash].js`;
}

export function languageOfChunkFile(file: string): Language | null {
  const named = languageChunkPattern.exec(file)?.[1];
  return languages.find((language) => language === named) ?? null;
}

export interface BuiltChunk {
  readonly fileName: string;
  readonly facadeModuleId: string | null;
  readonly moduleIds: readonly string[];
}

function isModule(id: string | null, module: string): boolean {
  return id?.endsWith(`/${module}`) === true;
}

export function languageChunkContentProblems(chunks: readonly BuiltChunk[]): string[] {
  const problems: string[] = [];
  for (const { module } of languageChunks) {
    const holders = chunks.filter((chunk) => chunk.moduleIds.some((id) => isModule(id, module)));
    for (const holder of holders) {
      if (holder.moduleIds.length !== 1 || !isModule(holder.facadeModuleId, module)) {
        problems.push(`${holder.fileName} holds ${module} together with other modules`);
      }
    }
    if (holders.length !== 1) {
      problems.push(`${module} is built into ${String(holders.length)} chunks instead of one of its own`);
    }
  }
  return problems;
}

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
      dynamicImports: strings('dynamicImports' in value ? value.dynamicImports : undefined, 'dynamicImports', key),
      css: strings('css' in value ? value.css : undefined, 'css', key),
      assets: strings('assets' in value ? value.assets : undefined, 'assets', key),
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

interface LanguageLoad {
  readonly size: number;
  readonly failures: readonly string[];
}

function largestLanguageLoad(
  manifest: Manifest,
  initial: ReadonlySet<string>,
  sizeOf: (file: string) => number,
): LanguageLoad {
  const failures: string[] = [];
  let largest = 0;
  for (const { language, module } of languageChunks) {
    const chunk = manifest.get(module);
    if (
      chunk === undefined ||
      !chunk.isDynamicEntry ||
      initial.has(chunk.file) ||
      languageOfChunkFile(chunk.file) !== language
    ) {
      failures.push(`${module} is not a language chunk loaded on demand`);
      continue;
    }
    if (chunk.imports.length > 0 || chunk.dynamicImports.length > 0) {
      failures.push(`${module} imports other chunks, so its chunk could carry code`);
    }
    const added = new Set([...staticFiles(manifest, [module])].filter((file) => !initial.has(file)));
    largest = Math.max(largest, total(added, sizeOf));
  }
  const languageFiles = new Set(languageChunks.map(({ module }) => manifest.get(module)?.file));
  for (const [key, chunk] of manifest) {
    if (languageChunkPattern.test(chunk.file) && !languageFiles.has(chunk.file)) {
      failures.push(`${key} is built as ${chunk.file}, a language chunk name, but is not a language's texts`);
    }
  }
  return { size: largest, failures };
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
  const staticSize = total(new Set([...initial].filter((file) => file.endsWith('.js'))), sizeOf);
  const language = largestLanguageLoad(manifest, initial, sizeOf);
  failures.push(...language.failures);
  const entrySize = staticSize + language.size;
  lines.push(
    `entry and its static imports: ${kilobytes(staticSize)} gzip of JavaScript, with the largest language chunk ${kilobytes(language.size)}: ${kilobytes(entrySize)} (limit ${kilobytes(entryLimit)})`,
  );
  if (entrySize > entryLimit) {
    failures.push(
      `the entry and its static imports with the largest language chunk are ${kilobytes(entrySize)} gzip of JavaScript, above ${kilobytes(entryLimit)}`,
    );
  }
  for (const budget of firstLoadBudgets) {
    const size = total(staticFiles(manifest, [entryKey, ...budget.pages]), sizeOf) + language.size;
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

export type ServedEntryKind = 'file' | 'directory' | 'link' | 'other';

export interface ServedEntry {
  path: string;
  kind: ServedEntryKind;
}

const servedAssetPattern = /^assets\/[^/]+\.(?:js|css|woff2?)$/;

export interface ServedFiles {
  readonly trackedPublicFiles: ReadonlySet<string>;
  readonly namedAssets: ReadonlySet<string>;
}

export function manifestNamedFiles(manifest: Manifest): Set<string> {
  return new Set([...manifest.values()].flatMap((chunk) => [chunk.file, ...chunk.css, ...chunk.assets]));
}

export interface PublicListing {
  readonly indexEntries: string;
  readonly uncommitted: string;
}

export interface CommittedPublicFiles {
  readonly files: ReadonlySet<string>;
  readonly problems: readonly string[];
}

const regularFileModes: readonly string[] = ['100644', '100755'];
const indexEntryPattern = /^(\S) (\d{6}) [0-9a-f]+ \d\t(.+)$/s;
const plainIndexTag = 'H';

function records(listing: string): string[] {
  return listing.split('\0').filter((record) => record !== '');
}

export function committedPublicFiles(
  publicFolder: string,
  listing: PublicListing,
  kindOf: (path: string) => ServedEntryKind,
): CommittedPublicFiles {
  const uncommitted = records(listing.uncommitted);
  const problems =
    uncommitted.length === 0
      ? []
      : [
          `${publicFolder}/ has changes that are not committed: ${uncommitted.map((record) => record.trim()).join(', ')}`,
        ];
  const files = new Set<string>();
  for (const record of records(listing.indexEntries)) {
    const match = indexEntryPattern.exec(record);
    if (match === null) {
      throw new Error(`unreadable index entry under ${publicFolder}/: ${record}`);
    }
    const [, tag = '', mode = '', path = ''] = match;
    if (tag === plainIndexTag && regularFileModes.includes(mode) && kindOf(path) === 'file') {
      files.add(path.slice(publicFolder.length + 1));
    }
  }
  return { files, problems };
}

function isDotted(path: string): boolean {
  return path.split('/').some((segment) => segment.startsWith('.'));
}

function isServedPath(entry: ServedEntry, served: ServedFiles): boolean {
  if (isDotted(entry.path)) {
    return false;
  }
  if (entry.kind === 'directory') {
    return entry.path === 'assets';
  }
  if (entry.kind !== 'file') {
    return false;
  }
  return (
    entry.path === entryKey ||
    served.trackedPublicFiles.has(entry.path) ||
    (servedAssetPattern.test(entry.path) && served.namedAssets.has(entry.path))
  );
}

export function unexpectedServedEntries(entries: readonly ServedEntry[], served: ServedFiles): string[] {
  return entries
    .filter((entry) => !isServedPath(entry, served))
    .map((entry) => `${entry.path} (${entry.kind})`)
    .sort();
}
