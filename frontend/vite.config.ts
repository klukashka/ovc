import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { mkdirSync, readFileSync, renameSync, rmSync } from 'node:fs';
import { Agent } from 'node:http';
import { dirname, join, resolve } from 'node:path';
import { type Plugin, type ProxyOptions, loadEnv } from 'vite';
import { defineConfig } from 'vitest/config';

import { buildManifestPath, firstLoadModules } from './scripts/bundle-budget.ts';
import { realtimePath } from './src/lib/realtime/socket.ts';
import { pageHeaders, responseHeaders } from './security-headers.ts';

const defaultApiTarget = 'http://127.0.0.1:8000';
const idleApiConnectionMilliseconds = 60_000;

function apiProxy(target: string): Record<string, ProxyOptions> {
  const reusedConnections = new Agent({ keepAlive: true, timeout: idleApiConnectionMilliseconds });
  return {
    [`^${realtimePath}$`]: { target, changeOrigin: false, ws: true },
    '/api': { target, changeOrigin: false, ws: false, agent: reusedConnections },
  };
}

function moveBuildManifest(): Plugin {
  return {
    name: 'ovc-move-build-manifest',
    apply: 'build',
    writeBundle(options): void {
      const outDir = options.dir ?? resolve(import.meta.dirname, 'dist');
      mkdirSync(dirname(buildManifestPath), { recursive: true });
      renameSync(join(outDir, '.vite', 'manifest.json'), buildManifestPath);
      rmSync(join(outDir, '.vite'), { recursive: true });
    },
  };
}

function firstLoadGroups(): {
  name: string;
  test: (id: string) => boolean;
  priority: number;
  includeDependenciesRecursively: boolean;
}[] {
  return firstLoadModules.map(({ chunk, module }) => ({
    name: chunk,
    test: (id: string): boolean => id.endsWith(`/${module}`),
    priority: 1,
    includeDependenciesRecursively: false,
  }));
}

function setting(value: string | undefined): string | undefined {
  return value === '' ? undefined : value;
}

function previewTls(
  certificateSetting: string | undefined,
  privateKeySetting: string | undefined,
): { https?: { cert: Buffer; key: Buffer } } {
  const certificate = setting(certificateSetting);
  const privateKey = setting(privateKeySetting);
  if (certificate === undefined && privateKey === undefined) {
    return {};
  }
  if (certificate === undefined || privateKey === undefined) {
    throw new Error('Set both OVC_FRONTEND_PREVIEW_CERTIFICATE and OVC_FRONTEND_PREVIEW_PRIVATE_KEY, or neither');
  }
  return { https: { cert: readFileSync(certificate), key: readFileSync(privateKey) } };
}

export default defineConfig(({ mode }) => {
  const environment = loadEnv(mode, import.meta.dirname, 'OVC_FRONTEND_');
  const apiTarget = environment.OVC_FRONTEND_API_TARGET ?? defaultApiTarget;
  return {
    plugins: [react(), tailwindcss(), moveBuildManifest()],
    build: {
      assetsInlineLimit: 0,
      sourcemap: false,
      manifest: true,
      rolldownOptions: {
        output: {
          codeSplitting: { groups: [...firstLoadGroups(), { name: 'vendor', test: /[\\/]node_modules[\\/]/ }] },
        },
      },
    },
    server: {
      host: '127.0.0.1',
      port: 5173,
      strictPort: true,
      fs: { strict: true },
      cors: false,
      headers: responseHeaders,
      proxy: apiProxy(apiTarget),
    },
    preview: {
      host: '127.0.0.1',
      port: 4173,
      strictPort: true,
      cors: false,
      headers: pageHeaders,
      proxy: apiProxy(apiTarget),
      ...previewTls(environment.OVC_FRONTEND_PREVIEW_CERTIFICATE, environment.OVC_FRONTEND_PREVIEW_PRIVATE_KEY),
    },
    test: {
      projects: [
        { extends: true, test: { name: 'browser', environment: 'jsdom', include: ['src/**/*.test.tsx'] } },
        {
          extends: true,
          test: {
            name: 'node',
            environment: 'node',
            include: ['src/**/*.test.ts', 'scripts/**/*.test.ts', 'e2e/**/*.test.ts'],
          },
        },
      ],
    },
  };
});
