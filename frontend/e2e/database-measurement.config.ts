import { defineConfig } from '@playwright/test';

import configuration from '../playwright.config.ts';
import type { MeasurementWorkerOptions } from './fixtures.ts';

export default defineConfig<MeasurementWorkerOptions>({
  ...configuration,
  testDir: '.',
  globalSetup: './global-setup.ts',
  globalTeardown: './global-teardown.ts',
  projects: (configuration.projects ?? []).flatMap((project) =>
    (['on', 'off'] as const).map((mode) => ({
      ...project,
      name: `${project.name ?? 'browser'}-${mode}`,
      use: { ...project.use, measurementDatabaseMode: mode },
    })),
  ),
});
