import { readFileSync, readdirSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';

import { type CommentFinding, cssComments, htmlComments, scriptComments } from './comments.ts';

const frontendRoot = resolve(import.meta.dirname, '..');
const skippedDirectories = new Set(['node_modules', 'dist', 'playwright-report', 'test-results']);
const generatedFiles = new Set(['src/lib/api/schema.ts']);
const scriptExtensions = ['.ts', '.tsx', '.js', '.mjs', '.cjs', '.mts', '.cts', '.jsx', '.json'];

function handWrittenFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      return skippedDirectories.has(entry.name) ? [] : handWrittenFiles(path);
    }
    return generatedFiles.has(relative(frontendRoot, path)) ? [] : [path];
  });
}

function findings(path: string): CommentFinding[] {
  const source = readFileSync(path, 'utf8');
  if (scriptExtensions.some((extension) => path.endsWith(extension))) {
    return scriptComments(path, source);
  }
  if (path.endsWith('.css')) {
    return cssComments(source);
  }
  if (path.endsWith('.html')) {
    return htmlComments(source);
  }
  return [];
}

const problems = handWrittenFiles(frontendRoot).flatMap((path) =>
  findings(path).map((finding) => `${relative(frontendRoot, path)}:${String(finding.line)}: comment ${finding.text}`),
);

if (problems.length > 0) {
  process.stderr.write(`Comments are not allowed in hand-written frontend files:\n${problems.join('\n')}\n`);
  process.exitCode = 1;
}
