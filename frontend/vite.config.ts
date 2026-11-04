import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { mkdirSync, readFileSync, renameSync, rmSync } from 'node:fs';
import { Agent } from 'node:http';
import { dirname, join, resolve } from 'node:path';
import { type Plugin, type ProxyOptions, loadEnv } from 'vite';
import { type ViteUserConfig, defineConfig } from 'vitest/config';

import {
  buildManifestPath,
  firstLoadModules,
  languageChunkContentProblems,
  languageChunkFileName,
  languageChunks,
} from './scripts/bundle-budget.ts';
import { developmentLogger, developmentTls } from './scripts/development-tls.ts';
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

function languageChunksHoldOnlyTexts(): Plugin {
  return {
    name: 'ovc-language-chunks-hold-only-texts',
    apply: 'build',
    generateBundle(_options, bundle): void {
      const chunks = Object.values(bundle).flatMap((output) => (output.type === 'chunk' ? [output] : []));
      const problems = languageChunkContentProblems(chunks);
      if (problems.length > 0) {
        this.error(`language chunks must hold only their own texts:\n${problems.join('\n')}`);
      }
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

function chunkFileName(chunk: { facadeModuleId: string | null }): string {
  const language = languageChunks.find(({ module }) => chunk.facadeModuleId?.endsWith(`/${module}`) === true);
  return language === undefined ? 'assets/[name]-[hash].js' : languageChunkFileName(language.language);
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

export function frontendConfiguration(mode: string, environment: Readonly<Record<string, string>>): ViteUserConfig {
  const apiTarget = environment.OVC_FRONTEND_API_TARGET ?? defaultApiTarget;
  return {
    ...(mode === 'measurement-dev-tls' ? { customLogger: developmentLogger() } : {}),
    plugins: [react(), tailwindcss(), languageChunksHoldOnlyTexts(), moveBuildManifest()],
    build: {
      assetsInlineLimit: 0,
      sourcemap: false,
      manifest: true,
      rolldownOptions: {
        output: {
          chunkFileNames: chunkFileName,
          codeSplitting: { groups: [...firstLoadGroups(), { name: 'vendor', test: /[\\/]node_modules[\\/]/ }] },
        },
      },
    },
    server: {
      ...developmentTls(
        mode,
        environment.OVC_FRONTEND_PREVIEW_CERTIFICATE,
        environment.OVC_FRONTEND_PREVIEW_PRIVATE_KEY,
        import.meta.dirname,
      ),
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
}

export default defineConfig(({ mode }) =>
  frontendConfiguration(mode, loadEnv(mode, import.meta.dirname, 'OVC_FRONTEND_')),
);
