import { readdirSync } from 'node:fs';
import { basename, join } from 'node:path';

import { type Project, defineConfig, devices } from '@playwright/test';

import { resolveRunOutput, runOutputVariable } from './e2e/run-output.ts';

const testDir = join(import.meta.dirname, 'e2e');
const browserlessSuites = ['session-transport.spec.ts', 'api-contracts.spec.ts', 'realtime-contract.spec.ts'];
const specGroupCount = 4;
const runOutput = resolveRunOutput(import.meta.dirname, process.env[runOutputVariable], process.pid);
process.env[runOutputVariable] = runOutput;

const browserSpecs = readdirSync(testDir, { recursive: true, encoding: 'utf8' })
  .filter((file) => file.endsWith('.spec.ts') && !browserlessSuites.includes(basename(file)))
  .sort();

const specGroups = Array.from({ length: specGroupCount }, (_, group) =>
  browserSpecs.filter((_, index) => index % specGroupCount === group),
);

function specPattern(spec: string): RegExp {
  return new RegExp(`^${join(testDir, spec).replaceAll(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`);
}

function specGroupProjects(engine: string, settings: Omit<Project, 'name' | 'testMatch'>): Project[] {
  return specGroups.map((specs, group) => ({
    ...settings,
    name: `${engine}-${String(group + 1)}`,
    testMatch: specs.map(specPattern),
  }));
}

export default defineConfig({
  testDir,
  testMatch: '**/*.spec.ts',
  outputDir: runOutput,
  fullyParallel: true,
  forbidOnly: true,
  retries: 0,
  reporter: [['list']],
  globalSetup: './e2e/global-setup.ts',
  globalTeardown: './e2e/global-teardown.ts',
  use: {
    trace: 'retain-on-failure',
    ignoreHTTPSErrors: true,
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    ...specGroupProjects('firefox', { use: { ...devices['Desktop Firefox'] }, grepInvert: /@coarse-pointer/ }),
    ...specGroupProjects('webkit', { use: { ...devices['Desktop Safari'] } }),
  ],
});
