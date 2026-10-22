import { readdirSync } from 'node:fs';
import { extname, join, relative, resolve } from 'node:path';

export const sourceExtensions: ReadonlySet<string> = new Set(['.ts', '.tsx', '.css']);
export const publicExtensions: ReadonlySet<string> = new Set(['.js', '.svg', '.png', '.ico', '.webp', '.woff2']);

export function unlintedFiles(paths: readonly string[], allowed: ReadonlySet<string>): string[] {
  return paths.filter((path) => !allowed.has(extname(path)));
}

function filesUnder(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    return entry.isDirectory() ? filesUnder(path) : [path];
  });
}

export function checkSourceTypes(frontendRoot: string): string[] {
  const checked: readonly [string, ReadonlySet<string>][] = [
    ['src', sourceExtensions],
    ['public', publicExtensions],
  ];
  return checked.flatMap(([directory, allowed]) =>
    unlintedFiles(filesUnder(resolve(frontendRoot, directory)), allowed).map((path) => relative(frontendRoot, path)),
  );
}
