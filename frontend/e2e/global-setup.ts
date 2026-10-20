import { readE2ESettings, startDeployment } from './deployment.ts';

export default async function globalSetup(): Promise<() => Promise<void>> {
  return startDeployment(readE2ESettings(process.env));
}
