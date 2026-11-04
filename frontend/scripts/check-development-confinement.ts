import assert from 'node:assert/strict';
import { chmodSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { request } from 'node:https';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { createServer } from 'vite';

import { createFixtureCertificate } from '../e2e/deployment.ts';
import { frontendConfiguration } from '../vite.config.ts';
import { measurementScope, recordMeasurement } from './slice6-measurement-support.ts';

async function read(origin: string, path: string): Promise<{ readonly status: number; readonly body: Buffer }> {
  return new Promise((resolve, reject) => {
    const outgoing = request(new URL(path, origin), { rejectUnauthorized: false }, (response) => {
      const chunks: Buffer[] = [];
      response.on('data', (chunk: Buffer) => {
        chunks.push(chunk);
      });
      response.on('end', () => {
        resolve({ status: response.statusCode ?? 0, body: Buffer.concat(chunks) });
      });
      response.on('error', reject);
    });
    outgoing.on('error', reject);
    outgoing.end();
  });
}

async function run(): Promise<void> {
  await measurementScope(async (cleanup) => {
    const temporary = mkdtempSync(join(tmpdir(), 'ovc-dev-confinement-'));
    cleanup(async (): Promise<void> => {
      rmSync(temporary, { recursive: true });
      await Promise.resolve();
    });
    chmodSync(temporary, 0o700);
    const tls = createFixtureCertificate(temporary);
    const configuration = frontendConfiguration('measurement-dev-tls', {
      OVC_FRONTEND_PREVIEW_CERTIFICATE: tls.certificate,
      OVC_FRONTEND_PREVIEW_PRIVATE_KEY: tls.privateKey,
    });
    const server = await createServer({
      ...configuration,
      configFile: false,
      root: new URL('..', import.meta.url).pathname,
      server: { ...configuration.server, port: 0, open: false },
    });
    cleanup(() => server.close());
    await server.listen();
    const address = server.httpServer?.address();
    if (address === undefined || address === null || typeof address === 'string')
      throw new Error('development server address unavailable');
    assert.equal(address.address, '127.0.0.1');
    const origin = `https://127.0.0.1:${String(address.port)}`;
    assert.equal((await read(origin, '/')).status, 200);
    for (const file of [tls.certificate, tls.privateKey]) {
      const material = readFileSync(file);
      const encoded = file.replace(/[a-z]/i, (character) => `%${character.charCodeAt(0).toString(16)}`);
      for (const path of [`/@fs/${file}`, `/@fs/${encoded}`]) {
        const result = await read(origin, path);
        assert.ok(result.status === 403 || result.status === 404);
        assert.equal(result.body.includes(material), false);
      }
    }
    recordMeasurement({ event: 'browser.development_confinement', https: true, loopback: true, materialServed: false });
  });
}

try {
  await run();
} catch {
  recordMeasurement({ event: 'browser.development_confinement_failed', kind: 'verification_failed' });
  process.exitCode = 1;
}
