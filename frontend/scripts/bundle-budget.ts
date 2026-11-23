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
export const entryLimit = 184 * kilobyte;
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

export const moduleGraphPath = resolve(import.meta.dirname, '..', 'build-report', 'module-graph.json');

export const builderPage = 'src/features/builder/BuilderPage.tsx';

export interface GraphModule {
  readonly id: string;
  readonly imports: readonly string[];
  readonly dynamicImports: readonly string[];
}

export interface GraphChunk {
  readonly file: string;
  readonly modules: readonly string[];
}

export interface ModuleGraph {
  readonly chunks: readonly GraphChunk[];
  readonly modules: readonly GraphModule[];
}

function stringList(value: unknown, field: string): string[] {
  if (!Array.isArray(value) || !value.every((item): item is string => typeof item === 'string')) {
    throw new Error(`the module graph has a malformed ${field}`);
  }
  return value;
}

function record(value: unknown, field: string): object {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new Error(`the module graph has a malformed ${field}`);
  }
  return value;
}

export function parseModuleGraph(json: unknown): ModuleGraph {
  const graph = record(json, 'graph');
  const chunks = 'chunks' in graph && Array.isArray(graph.chunks) ? graph.chunks : null;
  const modules = 'modules' in graph && Array.isArray(graph.modules) ? graph.modules : null;
  if (chunks === null || modules === null) {
    throw new Error('the module graph has no chunks or modules');
  }
  return {
    chunks: chunks.map((value: unknown) => {
      const chunk = record(value, 'chunk');
      if (!('file' in chunk) || typeof chunk.file !== 'string' || !('modules' in chunk)) {
        throw new Error('the module graph has a chunk without a file or modules');
      }
      return { file: chunk.file, modules: stringList(chunk.modules, 'chunk modules') };
    }),
    modules: modules.map((value: unknown) => {
      const module = record(value, 'module');
      if (
        !('id' in module) ||
        typeof module.id !== 'string' ||
        !('imports' in module) ||
        !('dynamicImports' in module)
      ) {
        throw new Error('the module graph has a module without an identifier or imports');
      }
      return {
        id: module.id,
        imports: stringList(module.imports, 'imports'),
        dynamicImports: stringList(module.dynamicImports, 'dynamic imports'),
      };
    }),
  };
}

function reachable(graph: ModuleGraph, roots: readonly string[], barrier: string | null): Set<string> {
  const edges = new Map(graph.modules.map((module) => [module.id, [...module.imports, ...module.dynamicImports]]));
  const seen = new Set<string>();
  const pending = roots.filter((root) => root !== barrier);
  for (let id = pending.pop(); id !== undefined; id = pending.pop()) {
    if (seen.has(id)) {
      continue;
    }
    seen.add(id);
    pending.push(...(edges.get(id) ?? []).filter((next) => next !== barrier));
  }
  return seen;
}

export interface OwnedPage {
  readonly page: string;
  readonly name: string;
}

export const ownedPages: readonly OwnedPage[] = [
  { page: builderPage, name: 'builder' },
  { page: 'src/features/review/ReviewPage.tsx', name: 'review screen' },
  { page: 'src/features/review/ReviewQueuePage.tsx', name: 'review queue' },
  { page: 'src/features/homework/IssuePage.tsx', name: 'issue page' },
  { page: 'src/features/homework/IssuedPage.tsx', name: 'issued list' },
];

export const studentPages: readonly string[] = [
  'src/features/assignment/AssignmentPage.tsx',
  'src/features/assignment/HomeworkPage.tsx',
  'src/features/home/HomePage.tsx',
  'src/features/tutors/MyTutorsPage.tsx',
];

export const tutorOnlyFolders: readonly string[] = ['src/features/review/', 'src/features/homework/'];

export function pageOnlyModules(
  graph: ModuleGraph,
  manifest: Manifest,
  pageModules: readonly string[],
  ownedPage: string,
): Set<string> {
  const entryFile = chunkAt(manifest, entryKey).file;
  const entryModules = graph.chunks.find((chunk) => chunk.file === entryFile)?.modules ?? [];
  const otherRoots = [...entryModules, ...pageModules.filter((page) => page !== ownedPage)];
  const others = reachable(graph, otherRoots, ownedPage);
  return new Set([...reachable(graph, [ownedPage], null)].filter((id) => !others.has(id)));
}

export function builderOnlyModules(
  graph: ModuleGraph,
  manifest: Manifest,
  pageModules: readonly string[],
): Set<string> {
  return pageOnlyModules(graph, manifest, pageModules, builderPage);
}

export function checkPageChunk(
  manifest: Manifest,
  graph: ModuleGraph,
  pageModules: readonly string[],
  sizeOf: (file: string) => number,
  owned: OwnedPage,
): BundleReport {
  const failures: string[] = [];
  if (manifest.get(owned.page)?.isDynamicEntry !== true) {
    return { lines: [], failures: [`${owned.page} is not loaded on demand`] };
  }
  const pageOnly = pageOnlyModules(graph, manifest, pageModules, owned.page);
  const elsewhere = new Map<string, string>();
  for (const [name, keys] of [
    ['the entry', [entryKey]],
    ...pageModules.filter((page) => page !== owned.page).map((page) => [page, [page]] as const),
  ] as const) {
    for (const file of staticFiles(manifest, keys)) {
      if (!elsewhere.has(file)) {
        elsewhere.set(file, name);
      }
    }
  }
  for (const chunk of graph.chunks) {
    const loader = elsewhere.get(chunk.file);
    if (loader === undefined) {
      continue;
    }
    for (const module of chunk.modules.filter((id) => pageOnly.has(id))) {
      failures.push(
        `${module} is reachable only from the ${owned.name} but is built into ${chunk.file}, loaded by ${loader}`,
      );
    }
  }
  if (pageOnly.size === 0) {
    failures.push(`no module is reachable only from the ${owned.name}, so the ${owned.name} check would prove nothing`);
  }
  const initial = staticFiles(manifest, [entryKey]);
  const pageFiles = new Set([...staticFiles(manifest, [owned.page])].filter((file) => !initial.has(file)));
  const size = total(pageFiles, sizeOf);
  return {
    lines: [
      `${owned.name} chunk and its imports beyond the entry: ${kilobytes(size)} gzip, ${String(pageOnly.size)} modules reachable only from the ${owned.name}`,
    ],
    failures,
  };
}

export function checkBuilderChunk(
  manifest: Manifest,
  graph: ModuleGraph,
  pageModules: readonly string[],
  sizeOf: (file: string) => number,
): BundleReport {
  return checkPageChunk(manifest, graph, pageModules, sizeOf, { page: builderPage, name: 'builder' });
}

export function checkStudentReach(
  graph: ModuleGraph,
  students: readonly string[],
  forbidden: readonly string[],
): BundleReport {
  const failures: string[] = absentFolders(graph, forbidden, 'student page');
  for (const page of students) {
    if (!graph.modules.some((module) => module.id === page)) {
      failures.push(`${page} is not in the module graph, so the student page check would prove nothing`);
      continue;
    }
    for (const module of reachable(graph, [page], null)) {
      if (forbidden.some((folder) => module.startsWith(folder))) {
        failures.push(`${module} is tutor code but is reachable from the student page ${page}`);
      }
    }
  }
  return {
    lines: [`${String(students.length)} student pages reach no module of ${forbidden.join(', ')}`],
    failures,
  };
}

function absentFolders(graph: ModuleGraph, folders: readonly string[], check: string): string[] {
  return folders
    .filter((folder) => !graph.modules.some((module) => module.id.startsWith(folder)))
    .map((folder) => `no module of ${folder} is in the module graph, so the ${check} check would prove nothing`);
}

export interface SharedPage {
  readonly page: string;
  readonly forbidden: readonly string[];
  readonly pinnedFolder: string;
  readonly pinned: readonly string[];
}

export const sharedLessonPage: SharedPage = {
  page: 'src/features/schedule/LessonPage.tsx',
  forbidden: ['src/features/review/'],
  pinnedFolder: 'src/features/homework/',
  pinned: [
    'src/features/homework/IssueAssignment.tsx',
    'src/features/homework/LessonIssues.tsx',
    'src/features/homework/api.ts',
    'src/features/homework/texts.en.ts',
    'src/features/homework/texts.ru.ts',
  ],
};

export function checkSharedPage(graph: ModuleGraph, shared: SharedPage): BundleReport {
  const failures = absentFolders(graph, [...shared.forbidden, shared.pinnedFolder], 'shared page');
  if (!graph.modules.some((module) => module.id === shared.page)) {
    return {
      lines: [],
      failures: [
        ...failures,
        `${shared.page} is not in the module graph, so the shared page check would prove nothing`,
      ],
    };
  }
  const reached = reachable(graph, [shared.page], null);
  for (const module of reached) {
    if (shared.forbidden.some((folder) => module.startsWith(folder))) {
      failures.push(`${module} is tutor code but is reachable from the shared page ${shared.page}`);
    } else if (module.startsWith(shared.pinnedFolder) && !shared.pinned.includes(module)) {
      failures.push(`${module} is reachable from the shared page ${shared.page} but is not in its pinned list`);
    }
  }
  for (const module of shared.pinned.filter((pinned) => !reached.has(pinned))) {
    failures.push(`${module} is pinned for the shared page ${shared.page} but is no longer reachable from it`);
  }
  return {
    lines: [
      `the shared page ${shared.page} reaches no module of ${shared.forbidden.join(', ')} and only its ${String(shared.pinned.length)} pinned modules of ${shared.pinnedFolder}`,
    ],
    failures,
  };
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
