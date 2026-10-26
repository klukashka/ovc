import { readFileSync, readdirSync, statSync } from 'node:fs';
import { resolve } from 'node:path';

import { type WorkspaceFolder, releaseAgeProblems } from './release-age.ts';

const repositoryRoot = resolve(import.meta.dirname, '..', '..');
const pnpmfilePattern = /^\.?pnpmfile\./i;

function text(path: string): string {
  return readFileSync(resolve(repositoryRoot, path), 'utf8');
}

function optionalText(path: string): string | undefined {
  return statSync(resolve(repositoryRoot, path), { throwIfNoEntry: false }) === undefined ? undefined : text(path);
}

function workspaceFolder(folder: string): WorkspaceFolder {
  return {
    folder,
    workspaceYaml: text(`${folder}/pnpm-workspace.yaml`),
    packageJson: text(`${folder}/package.json`),
    npmrc: optionalText(`${folder}/.npmrc`),
    pnpmfiles: readdirSync(resolve(repositoryRoot, folder)).filter((name) => pnpmfilePattern.test(name)),
  };
}

const problems = releaseAgeProblems(
  [workspaceFolder('frontend'), workspaceFolder('tools/docs')],
  statSync(resolve(repositoryRoot, 'deploy'), { throwIfNoEntry: false })?.isDirectory() === true,
);

if (problems.length > 0) {
  process.stderr.write(`The dependency release age breaks ADR-0017:\n${problems.join('\n')}\n`);
  process.exitCode = 1;
}
