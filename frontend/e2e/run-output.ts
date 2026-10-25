import { lstatSync } from 'node:fs';
import { basename, dirname, resolve } from 'node:path';

export const runOutputVariable = 'E2E_HARNESS_RUN_OUTPUT';

const runFolderPattern = /^run-[1-9][0-9]*$/;

export function playwrightOutputRoot(frontendRoot: string): string {
  return resolve(frontendRoot, 'test-results', 'playwright');
}

export function resolveRunOutput(frontendRoot: string, inherited: string | undefined, pid: number): string {
  const root = playwrightOutputRoot(frontendRoot);
  if (inherited === undefined) {
    return resolve(root, `run-${String(pid)}`);
  }
  const output = resolve(frontendRoot, inherited);
  if (dirname(output) !== root || !runFolderPattern.test(basename(output))) {
    throw new Error(
      `${runOutputVariable} must name a run-<pid> folder directly inside ${root}, because Playwright clears that folder; it is set by the harness and is not a setting`,
    );
  }
  return output;
}

export function assertOutputRootIsReal(frontendRoot: string): void {
  const resultsFolder = resolve(frontendRoot, 'test-results');
  for (const folder of [resultsFolder, playwrightOutputRoot(frontendRoot)]) {
    const entry = lstatSync(folder, { throwIfNoEntry: false });
    if (entry !== undefined && !entry.isDirectory()) {
      throw new Error(
        `${folder} must be a real folder, not a link or a file, before the harness removes earlier run outputs inside it`,
      );
    }
  }
}
