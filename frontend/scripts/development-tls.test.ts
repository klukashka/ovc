import { execFileSync, spawnSync } from 'node:child_process';
import { chmodSync, mkdtempSync, mkdirSync, rmSync, symlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { afterAll, beforeAll, expect, test } from 'vitest';

import { assertFrontendPreparation } from '../e2e/deployment.ts';
import { frontendConfiguration } from '../vite.config.ts';
import { pageHeaders, responseHeaders } from '../security-headers.ts';
import { developmentTls } from './development-tls.ts';

const temporary = mkdtempSync(join(tmpdir(), 'ovc-tls-contract-'));
const root = join(temporary, 'frontend');
const certificate = join(temporary, 'certificate.pem');
const privateKey = join(temporary, 'private-key.pem');

beforeAll(() => {
  chmodSync(temporary, 0o700);
  mkdirSync(root);
  execFileSync(
    'openssl',
    [
      'req',
      '-x509',
      '-newkey',
      'rsa:2048',
      '-nodes',
      '-days',
      '1',
      '-subj',
      '/CN=127.0.0.1',
      '-addext',
      'subjectAltName=IP:127.0.0.1',
      '-keyout',
      privateKey,
      '-out',
      certificate,
    ],
    { stdio: 'ignore' },
  );
  chmodSync(privateKey, 0o600);
});

afterAll(() => {
  rmSync(temporary, { recursive: true });
});

test('default development remains HTTP while preview retains its TLS and distinct headers', () => {
  const settings = { OVC_FRONTEND_PREVIEW_CERTIFICATE: certificate, OVC_FRONTEND_PREVIEW_PRIVATE_KEY: privateKey };

  const configuration = frontendConfiguration('development', settings);

  expect(configuration.server?.https).toBeUndefined();
  expect(configuration.preview?.https).toBeDefined();
  expect(configuration.server).toMatchObject({
    host: '127.0.0.1',
    strictPort: true,
    fs: { strict: true },
    cors: false,
    headers: responseHeaders,
  });
  expect(configuration.preview).toMatchObject({
    host: '127.0.0.1',
    strictPort: true,
    cors: false,
    headers: pageHeaders,
  });
  expect(configuration.server?.proxy).toHaveProperty('/api');
  expect(
    Object.values(configuration.server?.proxy ?? {}).some(
      (value) => typeof value !== 'string' && value.ws === true && value.changeOrigin === false,
    ),
  ).toBe(true);
});

test('the named development mode requires both valid confined files and preserves serving policy', () => {
  const settings = { OVC_FRONTEND_PREVIEW_CERTIFICATE: certificate, OVC_FRONTEND_PREVIEW_PRIVATE_KEY: privateKey };

  const configuration = frontendConfiguration('measurement-dev-tls', settings);

  expect(configuration.server?.https).toBeDefined();
  expect(configuration.server).toMatchObject({
    host: '127.0.0.1',
    strictPort: true,
    fs: { strict: true },
    cors: false,
    headers: responseHeaders,
  });
  expect((): ReturnType<typeof developmentTls> =>
    developmentTls('measurement-dev-tls', undefined, undefined, root),
  ).toThrow('development_tls_settings_missing');
  expect((): ReturnType<typeof developmentTls> =>
    developmentTls('measurement-dev-tls', certificate, undefined, root),
  ).toThrow('development_tls_settings_missing');
  expect((): ReturnType<typeof developmentTls> =>
    developmentTls('measurement-dev-tls', undefined, privateKey, root),
  ).toThrow('development_tls_settings_missing');
  expect((): ReturnType<typeof developmentTls> =>
    developmentTls('measurement-dev-tls', certificate, certificate, root),
  ).toThrow('development_tls_material_invalid_or_unconfined');
  expect((): ReturnType<typeof developmentTls> =>
    developmentTls('measurement-dev-tls', join(temporary, 'absent.pem'), privateKey, root),
  ).toThrow('development_tls_material_invalid_or_unconfined');
});

test('source-tree aliases and served-output certificate paths fail closed without disclosing paths', () => {
  const served = join(root, 'dist');
  mkdirSync(served);
  const alias = join(served, 'key.pem');
  symlinkSync(privateKey, alias);
  const outsideAlias = join(temporary, 'frontend-alias');
  symlinkSync(root, outsideAlias);

  const attempts = [
    (): ReturnType<typeof developmentTls> => developmentTls('measurement-dev-tls', certificate, alias, root),
    (): ReturnType<typeof developmentTls> =>
      developmentTls('measurement-dev-tls', certificate, join(outsideAlias, 'dist', 'key.pem'), root),
  ];

  for (const attempt of attempts) {
    expect(attempt).toThrow('development_tls_material_invalid_or_unconfined');
    try {
      attempt();
    } catch (error) {
      expect(String(error)).not.toContain(temporary);
    }
  }
});

test('named-mode framework logger exports no console text exception content or private paths', () => {
  const module = new URL('./development-tls.ts', import.meta.url).href;
  const secret = 'synthetic-private-material';
  const program = `import { developmentLogger } from ${JSON.stringify(module)}; const logger = developmentLogger(); const failure = new Error(${JSON.stringify(secret)}); logger.info(${JSON.stringify(secret)}); logger.warn(${JSON.stringify(secret)}); logger.warnOnce(${JSON.stringify(secret)}); logger.error(${JSON.stringify(secret)}, { error: failure }); if (!logger.hasErrorLogged(failure)) throw new Error('logger_failed');`;

  const result = spawnSync(process.execPath, ['--input-type=module', '-e', program], { encoding: 'utf8' });

  expect(result.status).toBe(0);
  expect(`${result.stdout}${result.stderr}`).not.toContain(secret);
  expect(`${result.stdout}${result.stderr}`).not.toContain(temporary);
  expect(result.stdout).toContain('"kind":"info"');
  expect(result.stderr).toContain('"kind":"warning"');
  expect(result.stderr).toContain('"kind":"error"');
});

test('development preparation refuses an existing fixture TLS directory before creating or starting services', () => {
  const fixtureTlsRoot = join(root, 'test-results', 'deployment', 'tls');
  mkdirSync(fixtureTlsRoot, { recursive: true });

  const attempt = (): void => {
    assertFrontendPreparation('measurement-dev-tls', fixtureTlsRoot);
  };

  expect(attempt).toThrow('development_fixture_refuses_existing_source_tls_material');
  expect((): void => {
    assertFrontendPreparation('preview', fixtureTlsRoot);
  }).not.toThrow();
});
