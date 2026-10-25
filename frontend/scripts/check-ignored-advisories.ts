import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { ignoredAdvisoryProblems } from './ignored-advisories.ts';

const frontendRoot = resolve(import.meta.dirname, '..');

const problems = ignoredAdvisoryProblems(
  readFileSync(resolve(frontendRoot, 'pnpm-workspace.yaml'), 'utf8'),
  readFileSync(resolve(frontendRoot, 'osv-scanner.toml'), 'utf8'),
  readFileSync(resolve(frontendRoot, 'package.json'), 'utf8'),
  new Date(),
);

if (problems.length > 0) {
  process.stderr.write(`Every auditConfig.ignoreGhsas entry needs a current OSV entry:\n${problems.join('\n')}\n`);
  process.exitCode = 1;
}
