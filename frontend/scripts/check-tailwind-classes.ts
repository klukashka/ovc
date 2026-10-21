import { readFileSync, readdirSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import ts from 'typescript';

import { classUses, missingClasses } from './tailwind-classes.ts';

const frontendRoot = resolve(import.meta.dirname, '..');
const assets = resolve(frontendRoot, 'dist', 'assets');

function builtCss(): string {
  const files = readdirSync(assets).filter((name) => name.endsWith('.css'));
  if (files.length === 0) {
    throw new Error('dist/assets holds no stylesheet; run pnpm build first');
  }
  return files.map((name) => readFileSync(join(assets, name), 'utf8')).join('\n');
}

function program(): { program: ts.Program; files: string[] } {
  const configPath = resolve(frontendRoot, 'tsconfig.app.json');
  const config = ts.readConfigFile(configPath, (path) => ts.sys.readFile(path));
  if (config.error !== undefined) {
    throw new Error(ts.flattenDiagnosticMessageText(config.error.messageText, '\n'));
  }
  const parsed = ts.parseJsonConfigFileContent(config.config, ts.sys, frontendRoot);
  const files = parsed.fileNames.filter((name) => name.endsWith('.tsx'));
  return { program: ts.createProgram(parsed.fileNames, parsed.options), files };
}

const { program: compiled, files } = program();
const uses = classUses(compiled, files);
if (uses.length === 0) {
  process.stderr.write('No className was found in src/; the class check would prove nothing\n');
  process.exit(1);
}
const missing = missingClasses(uses, builtCss());
if (missing.length > 0) {
  const lines = missing.map((use) => `${relative(frontendRoot, use.file)}: ${use.token}`);
  process.stderr.write(`These classes produce no CSS in the build:\n${lines.join('\n')}\n`);
  process.exit(1);
}
