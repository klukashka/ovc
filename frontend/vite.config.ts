import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { loadEnv } from 'vite';
import { defineConfig } from 'vitest/config';

import { pageHeaders, responseHeaders } from './security-headers.ts';

const defaultApiTarget = 'http://127.0.0.1:8000';

function apiProxy(target: string): Record<string, { target: string; changeOrigin: boolean }> {
  return { '/api': { target, changeOrigin: false } };
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
