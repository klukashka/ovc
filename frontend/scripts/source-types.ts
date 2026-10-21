import { readdirSync } from 'node:fs';
import { extname, join, relative, resolve } from 'node:path';

const lintedExtensions = new Set(['.ts', '.tsx', '.css']);

export function unlintedSourceFiles(paths: readonly string[]): string[] {
  return paths.filter((path) => !lintedExtensions.has(extname(path)));
}

function sourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    return entry.isDirectory() ? sourceFiles(path) : [path];
  });
}

export function checkSourceTypes(frontendRoot: string): string[] {
  const root = resolve(frontendRoot, 'src');
  return unlintedSourceFiles(sourceFiles(root)).map((path) => relative(frontendRoot, path));
}
