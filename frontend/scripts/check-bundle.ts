import { execFileSync } from 'node:child_process';
import { existsSync, lstatSync, readFileSync, readdirSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { gzipSync } from 'node:zlib';

import {
  buildManifestPath,
  checkBundle,
  type CommittedPublicFiles,
  committedPublicFiles,
  manifestNamedFiles,
  parseManifest,
  type ServedEntry,
  type ServedEntryKind,
  unexpectedServedEntries,
} from './bundle-budget.ts';

const frontendRoot = resolve(import.meta.dirname, '..');
const distRoot = resolve(frontendRoot, 'dist');
const pagePattern = /Pages?\.tsx$/;
const publicFolder = 'public';

function pageModules(): string[] {
  const features = resolve(frontendRoot, 'src', 'features');
  return readdirSync(features, { recursive: true, encoding: 'utf8' })
    .filter((name) => pagePattern.test(name))
    .map((name) => relative(frontendRoot, join(features, name)))
    .sort();
}

function entryKind(path: string): ServedEntryKind {
  const entry = lstatSync(path);
  if (entry.isSymbolicLink()) {
    return 'link';
  }
  if (entry.isDirectory()) {
    return 'directory';
  }
  return entry.isFile() ? 'file' : 'other';
}

function servedEntries(root: string): ServedEntry[] {
  return readdirSync(root, { recursive: true, encoding: 'utf8' }).map((name) => ({
    path: name,
    kind: entryKind(join(root, name)),
  }));
}

function git(...args: string[]): string {
  return execFileSync('git', args, { cwd: frontendRoot, encoding: 'utf8' });
}

function committedPublic(): CommittedPublicFiles {
  return committedPublicFiles(
    publicFolder,
    {
      indexEntries: git('ls-files', '-s', '-v', '-z', '--', publicFolder),
      uncommitted: git('status', '--porcelain', '-z', '--untracked-files=all', '--', publicFolder),
    },
    (path) => entryKind(join(frontendRoot, path)),
  );
}

function gzipSize(file: string): number {
  return gzipSync(readFileSync(join(distRoot, file))).byteLength;
}

function manifestJson(): unknown {
  try {
    return JSON.parse(readFileSync(buildManifestPath, 'utf8'));
  } catch (error) {
    throw new Error('build-report/manifest.json is missing or unreadable; run pnpm build first', { cause: error });
  }
}

if (existsSync(join(distRoot, '.vite'))) {
  process.stderr.write('dist/.vite exists: the build left its manifest in the served build\n');
  process.exit(1);
}
const manifest = parseManifest(manifestJson());
const publicFiles = committedPublic();
if (publicFiles.problems.length > 0) {
  process.stderr.write(`${publicFiles.problems.join('\n')}\n`);
  process.exit(1);
}
const unexpected = unexpectedServedEntries(servedEntries(distRoot), {
  trackedPublicFiles: publicFiles.files,
  namedAssets: manifestNamedFiles(manifest),
});
if (unexpected.length > 0) {
  process.stderr.write(`dist/ holds entries the served build must not contain:\n${unexpected.join('\n')}\n`);
  process.exit(1);
}
const pages = pageModules();
if (pages.length === 0) {
  process.stderr.write('No page module was found in src/features; the bundle check would prove nothing\n');
  process.exit(1);
}
const report = checkBundle(manifest, pages, gzipSize);
process.stdout.write(`${report.lines.join('\n')}\n`);
if (report.failures.length > 0) {
  process.stderr.write(`The bundle is outside its budget:\n${report.failures.join('\n')}\n`);
  process.exit(1);
}
