import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { gzipSync } from 'node:zlib';

import { buildManifestPath, checkBundle, parseManifest } from './bundle-budget.ts';

const frontendRoot = resolve(import.meta.dirname, '..');
const distRoot = resolve(frontendRoot, 'dist');
const pagePattern = /Pages?\.tsx$/;

function pageModules(): string[] {
  const features = resolve(frontendRoot, 'src', 'features');
  return readdirSync(features, { recursive: true, encoding: 'utf8' })
    .filter((name) => pagePattern.test(name))
    .map((name) => relative(frontendRoot, join(features, name)))
    .sort();
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
const pages = pageModules();
if (pages.length === 0) {
  process.stderr.write('No page module was found in src/features; the bundle check would prove nothing\n');
  process.exit(1);
}
const report = checkBundle(parseManifest(manifestJson()), pages, gzipSize);
process.stdout.write(`${report.lines.join('\n')}\n`);
if (report.failures.length > 0) {
  process.stderr.write(`The bundle is outside its budget:\n${report.failures.join('\n')}\n`);
  process.exit(1);
}
