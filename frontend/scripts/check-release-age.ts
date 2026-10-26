import { readFileSync, statSync } from 'node:fs';
import { resolve } from 'node:path';

import { releaseAgeProblems } from './release-age.ts';

const repositoryRoot = resolve(import.meta.dirname, '..', '..');

function workspace(path: string): { workspace: string; yaml: string } {
  return { workspace: path, yaml: readFileSync(resolve(repositoryRoot, path), 'utf8') };
}

const problems = releaseAgeProblems(
  [workspace('frontend/pnpm-workspace.yaml'), workspace('tools/docs/pnpm-workspace.yaml')],
  statSync(resolve(repositoryRoot, 'deploy'), { throwIfNoEntry: false })?.isDirectory() === true,
);

if (problems.length > 0) {
  process.stderr.write(`The dependency release age breaks ADR-0017:\n${problems.join('\n')}\n`);
  process.exitCode = 1;
}
