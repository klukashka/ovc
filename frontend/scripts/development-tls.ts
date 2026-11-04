import { readFileSync, realpathSync } from 'node:fs';
import { dirname, resolve, sep } from 'node:path';
import type { Logger } from 'vite';
import { createSecureContext } from 'node:tls';

export function developmentTls(
  mode: string,
  certificate: string | undefined,
  privateKey: string | undefined,
  frontendRoot: string,
): { https?: { cert: Buffer; key: Buffer } } {
  if (mode !== 'measurement-dev-tls') return {};
  if (certificate === undefined || certificate === '' || privateKey === undefined || privateKey === '')
    throw new Error('development_tls_settings_missing');
  try {
    const root = realpathSync(frontendRoot).toLowerCase();
    const lexicalRoot = resolve(frontendRoot).toLowerCase();
    const paths = [certificate, privateKey].map((path) => resolve(path));
    const candidates = paths.flatMap((path) => [path, realpathSync(path), realpathSync(dirname(path))]);
    if (
      candidates.some(
        (path) =>
          path.toLowerCase() === root ||
          path.toLowerCase().startsWith(`${root}${sep}`) ||
          path.toLowerCase() === lexicalRoot ||
          path.toLowerCase().startsWith(`${lexicalRoot}${sep}`),
      )
    )
      throw new Error('development_tls_path_refused');
    const cert = readFileSync(paths[0] ?? '');
    const key = readFileSync(paths[1] ?? '');
    createSecureContext({ cert, key });
    return { https: { cert, key } };
  } catch {
    throw new Error('development_tls_material_invalid_or_unconfined');
  }
}

export function developmentLogger(): Logger {
  const errors = new WeakSet<Exclude<Parameters<Logger['hasErrorLogged']>[0], undefined>>();
  const logger: Logger = {
    hasWarned: false,
    info: (): void => {
      process.stdout.write('{"event":"development.server","kind":"info"}\n');
    },
    warn: (): void => {
      logger.hasWarned = true;
      process.stderr.write('{"event":"development.server","kind":"warning"}\n');
    },
    warnOnce: (): void => {
      logger.warn('');
    },
    error: (_message, options): void => {
      if (options?.error !== undefined && options.error !== null) errors.add(options.error);
      process.stderr.write('{"event":"development.server","kind":"error"}\n');
    },
    clearScreen: (): void => undefined,
    hasErrorLogged: (error): boolean => errors.has(error),
  };
  return logger;
}
