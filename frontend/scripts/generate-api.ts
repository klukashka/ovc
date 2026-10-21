import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import openapiTS, { astToString } from 'openapi-typescript';

const frontendRoot = resolve(import.meta.dirname, '..');
const backendRoot = resolve(frontendRoot, '../backend');
const schemaPath = resolve(frontendRoot, 'src/lib/api/schema.ts');

function openApiDocument(): string {
  return execFileSync('go', ['run', './cmd/api', 'openapi'], {
    cwd: backendRoot,
    encoding: 'utf8',
    env: { PATH: process.env.PATH ?? '', HOME: process.env.HOME ?? '', GOFLAGS: '-mod=readonly' },
    stdio: ['ignore', 'pipe', 'inherit'],
  });
}

async function generatedSchema(): Promise<string> {
  const ast = await openapiTS(openApiDocument(), { enumValues: true });
  return astToString(ast);
}

async function main(checkOnly: boolean): Promise<void> {
  const generated = await generatedSchema();
  if (!checkOnly) {
    writeFileSync(schemaPath, generated);
    return;
  }
  const committed = readFileSync(schemaPath, 'utf8');
  if (committed !== generated) {
    process.stderr.write('src/lib/api/schema.ts is out of date; run pnpm api:generate\n');
    process.exitCode = 1;
  }
}

await main(process.argv.includes('--check'));
