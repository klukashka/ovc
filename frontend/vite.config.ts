import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { readFileSync } from 'node:fs';
import { loadEnv } from 'vite';
import { defineConfig } from 'vitest/config';

import { pageHeaders, responseHeaders } from './security-headers.ts';

const defaultApiTarget = 'http://127.0.0.1:8000';

function apiProxy(target: string): Record<string, { target: string; changeOrigin: boolean }> {
  return { '/api': { target, changeOrigin: false } };
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
    plugins: [react(), tailwindcss()],
    build: { assetsInlineLimit: 0, sourcemap: false },
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
          test: { name: 'node', environment: 'node', include: ['src/**/*.test.ts', 'scripts/**/*.test.ts'] },
        },
      ],
    },
  };
});
