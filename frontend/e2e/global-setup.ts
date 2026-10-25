import type { FullConfig } from '@playwright/test';

import { prepareRun, readE2ESettings, removeEarlierRunOutputs, takeRunLock } from './deployment.ts';

export default function globalSetup(config: FullConfig): void {
  readE2ESettings(process.env);
  takeRunLock();
  removeEarlierRunOutputs(config.projects.map((project) => project.outputDir));
  prepareRun();
}
