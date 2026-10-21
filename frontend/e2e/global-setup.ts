import { prepareRun, readE2ESettings } from './deployment.ts';

export default function globalSetup(): void {
  readE2ESettings(process.env);
  prepareRun();
}
