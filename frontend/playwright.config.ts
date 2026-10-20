import { defineConfig, devices } from '@playwright/test';

import { apiAddress, previewOrigin } from './e2e/deployment.ts';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: true,
  retries: 0,
  reporter: [['list']],
  globalSetup: './e2e/global-setup.ts',
  use: {
    baseURL: previewOrigin,
    trace: 'retain-on-failure',
  },
  webServer: {
    command: 'pnpm build && pnpm preview',
    url: previewOrigin,
    reuseExistingServer: false,
    timeout: 120_000,
    env: { OVC_FRONTEND_API_TARGET: `http://${apiAddress}` },
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    { name: 'firefox', use: { ...devices['Desktop Firefox'] } },
    { name: 'webkit', use: { ...devices['Desktop Safari'] } },
  ],
});
