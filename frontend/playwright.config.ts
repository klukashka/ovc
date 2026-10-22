import { defineConfig, devices } from '@playwright/test';

const browserlessSuites = ['**/session-transport.spec.ts'];

export default defineConfig({
  testDir: './e2e',
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
