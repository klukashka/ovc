import { defineConfig, devices } from '@playwright/test';

import { resolveRunOutput, runOutputVariable } from './e2e/run-output.ts';

const browserlessSuites = ['**/session-transport.spec.ts', '**/api-contracts.spec.ts', '**/realtime-contract.spec.ts'];
const runOutput = resolveRunOutput(import.meta.dirname, process.env[runOutputVariable], process.pid);
process.env[runOutputVariable] = runOutput;

export default defineConfig({
  testDir: './e2e',
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
    { name: 'firefox', use: { ...devices['Desktop Firefox'] }, testIgnore: browserlessSuites },
    { name: 'webkit', use: { ...devices['Desktop Safari'] }, testIgnore: browserlessSuites },
  ],
});
