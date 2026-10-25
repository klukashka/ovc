import { mkdirSync, mkdtempSync, rmSync, symlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { afterEach, describe, expect, test } from 'vitest';

import { assertOutputRootIsReal, resolveRunOutput } from './run-output.ts';

const frontendRoot = '/work/frontend';

describe('end-to-end run output folder', () => {
  test('names a folder for this run when none is inherited', () => {
    const output = resolveRunOutput(frontendRoot, undefined, 4242);

    expect(output).toBe('/work/frontend/test-results/playwright/run-4242');
  });

  test('accepts the folder the runner passed to its workers', () => {
    const output = resolveRunOutput(frontendRoot, '/work/frontend/test-results/playwright/run-17', 99);

    expect(output).toBe('/work/frontend/test-results/playwright/run-17');
  });

  test.each([
    ['the frontend folder', '.'],
    ['the parent folder', '..'],
    ['a home folder', '/Users/someone'],
    ['the deployment folder', 'test-results/deployment'],
    ['the output root itself', 'test-results/playwright'],
    ['a nested run folder', 'test-results/playwright/run-1/run-2'],
    ['a folder that escapes through dots', 'test-results/playwright/../run-1'],
    ['a run folder without a pid', 'test-results/playwright/run-'],
    ['a run folder with pid zero', 'test-results/playwright/run-0'],
    ['a folder with another name', 'test-results/playwright/traces'],
    ['an empty value', ''],
  ])('refuses %s', (_name, inherited) => {
    const resolution = (): string => resolveRunOutput(frontendRoot, inherited, 1);

    expect(resolution).toThrow('E2E_HARNESS_RUN_OUTPUT must name a run-<pid> folder');
  });
});

describe('end-to-end output root before earlier outputs are removed', () => {
  const folders: string[] = [];

  afterEach(() => {
    for (const folder of folders.splice(0)) {
      rmSync(folder, { recursive: true, force: true });
    }
  });

  function scratchFolder(): string {
    const folder = mkdtempSync(join(tmpdir(), 'ovc-run-output-'));
    folders.push(folder);
    return folder;
  }

  test('accepts real folders and folders that do not exist yet', () => {
    const withFolders = scratchFolder();
    mkdirSync(join(withFolders, 'test-results', 'playwright'), { recursive: true });
    const empty = scratchFolder();

    const check = (): void => {
      assertOutputRootIsReal(withFolders);
      assertOutputRootIsReal(empty);
    };

    expect(check).not.toThrow();
  });

  test('refuses an output root that is a link to another folder', () => {
    const frontend = scratchFolder();
    const elsewhere = scratchFolder();
    mkdirSync(join(frontend, 'test-results'), { recursive: true });
    symlinkSync(elsewhere, join(frontend, 'test-results', 'playwright'));

    const check = (): void => {
      assertOutputRootIsReal(frontend);
    };

    expect(check).toThrow(/must be a real folder/);
  });

  test('refuses a results folder that is a link to another folder', () => {
    const frontend = scratchFolder();
    const elsewhere = scratchFolder();
    symlinkSync(elsewhere, join(frontend, 'test-results'));

    const check = (): void => {
      assertOutputRootIsReal(frontend);
    };

    expect(check).toThrow(/must be a real folder/);
  });
});
