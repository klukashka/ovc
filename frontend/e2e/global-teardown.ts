import { finishRun } from './deployment.ts';

export default function globalTeardown(): void {
  finishRun();
}
