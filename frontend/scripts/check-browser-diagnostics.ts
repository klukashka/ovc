import { readinessDeadlineMilliseconds } from '../e2e/deployment.ts';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { once } from 'node:events';

import { chromium, firefox, webkit } from '@playwright/test';

import { installPolicyObserver, observeBrowserContext } from '../e2e/browser-diagnostics.ts';
import { measurementScope, recordMeasurement, untilMeasurementCondition } from './slice6-measurement-support.ts';

async function run(): Promise<void> {
  await measurementScope(async (cleanup) => {
    const secret = 'synthetic-private-marker-do-not-export';
    const server = createServer((request, response) => {
      if (request.url === '/assets/language-en-canary.js') {
        response.destroy();
        return;
      }
      if (request.url === '/assets/canary-entry.js') {
        response.setHeader('Content-Type', 'application/javascript');
        response.end(
          "export function diagnosticWarning(text) { console.warn(text); } export const expectedFailure = import('/assets/language-en-canary.js').then(() => false, () => true);",
        );
        return;
      }
      response.setHeader('Content-Type', 'text/html');
      if (request.url === '/language-failure') {
        response.end(
          '<!DOCTYPE html><html><head><link rel="icon" href="data:,"></head><body><script type="module" src="/assets/canary-entry.js"></script></body></html>',
        );
        return;
      }
      const action = request.url === '/error' ? 'error' : 'warn';
      response.end(
        `<!DOCTYPE html><html><head><link rel="icon" href="data:,"></head><body><script>console.${action}(${JSON.stringify(secret)})</script></body></html>`,
      );
    });
    server.listen(0, '127.0.0.1');
    cleanup(
      () =>
        new Promise<void>((resolve, reject) => {
          server.close((error) => {
            if (error === undefined) resolve();
            else reject(error);
          });
        }),
    );
    await once(server, 'listening');
    const address = server.address();
    if (address === null || typeof address === 'string') throw new Error('diagnostic canary address unavailable');
    const origin = `http://127.0.0.1:${String(address.port)}`;
    for (const engine of [chromium, firefox, webkit]) {
      await measurementScope(async (release) => {
        const browser = await engine.launch();
        release(() => browser.close());
        const context = await browser.newContext();
        release(() => context.close());
        const observer = observeBrowserContext(context, engine.name(), 'canary');
        await installPolicyObserver(context);
        const page = await context.newPage();
        observer.expect(page).expected.push(secret);

        await page.goto(`${origin}/error`);
        await page.goto(`${origin}/warning`);
        assert.equal(observer.warnings(), 1);
        assert.equal(observer.errors(), 0);
        assert.throws(() => {
          observer.verify();
        });
        const additional = await context.newPage();
        await additional.goto(`${origin}/warning`);
        await page.goto(`${origin}/error`);

        assert.equal(observer.warnings(), 2);
        assert.equal(observer.errors(), 1);
        assert.throws(() => {
          observer.verify();
        });
        await context.close();
        await assert.rejects(async (): Promise<void> => {
          await observer.finish();
        });
        assert.equal(observer.warnings(), 2);
        assert.equal(observer.errors(), 1);
        const exported = JSON.stringify(observer.records());
        assert.equal(exported.includes(secret), false);
        assert.equal(exported.includes(origin), false);
        assert.deepEqual([...new Set(observer.records().map((record) => record.classification))], ['unknown']);
        recordMeasurement({
          event: 'browser.diagnostic_canary',
          engine: engine.name(),
          warnings: observer.warnings(),
          errors: observer.errors(),
          secretExported: false,
        });
      });
    }
    for (const control of ['baseline', 'duplicate', 'extra'] as const) {
      await measurementScope(async (release) => {
        const browser = await firefox.launch();
        release(() => browser.close());
        const context = await browser.newContext();
        release(() => context.close());
        const observer = observeBrowserContext(context, 'firefox', 'language_warning_control');
        await installPolicyObserver(context);
        const page = await context.newPage();
        let failed = false;
        const observed: { nativeWarning: string | null } = { nativeWarning: null };
        page.on('requestfailed', (request) => {
          if (request.url() === `${origin}/assets/language-en-canary.js`) failed = true;
        });
        page.on('console', (message) => {
          if (message.type() === 'warning' && observed.nativeWarning === null) observed.nativeWarning = message.text();
        });
        observer.beginLanguageFailure(page, origin, 'en');
        await page.goto(`${origin}/language-failure`);
        await untilMeasurementCondition(() => failed && observed.nativeWarning !== null, readinessDeadlineMilliseconds);
        if (observed.nativeWarning === null) throw new Error('native_language_warning_unverified');
        const moduleFailed = await page.evaluate(async (address) => {
          const imported: unknown = await import(address);
          if (typeof imported !== 'object' || imported === null || !('expectedFailure' in imported))
            throw new Error('canary_failure_module_invalid');
          return (await imported.expectedFailure) === true;
        }, `${origin}/assets/canary-entry.js`);
        assert.equal(moduleFailed, true);
        if (control !== 'baseline')
          await page.evaluate(
            async ({ address, text }) => {
              const imported: unknown = await import(address);
              if (
                typeof imported !== 'object' ||
                imported === null ||
                !('diagnosticWarning' in imported) ||
                typeof imported.diagnosticWarning !== 'function'
              )
                throw new Error('canary_warning_module_invalid');
              (imported as { readonly diagnosticWarning: (value: string) => void }).diagnosticWarning(text);
            },
            {
              address: `${origin}/assets/canary-entry.js`,
              text: control === 'duplicate' ? observed.nativeWarning : secret,
            },
          );
        await observer.finishLanguageFailure(page);
        await context.close();
        if (control === 'baseline') await observer.finish();
        else
          await assert.rejects(async (): Promise<void> => {
            await observer.finish();
          });
        assert.equal(observer.warnings(), control === 'baseline' ? 0 : 1);
        assert.equal(observer.errors(), 0);
        assert.equal(JSON.stringify(observer.records()).includes(secret), false);
        assert.equal(JSON.stringify(observer.records()).includes(origin), false);
        recordMeasurement({
          event: 'browser.language_warning_control',
          engine: 'firefox',
          control,
          actualRequestFailed: failed,
          nativeWarningObserved: true,
          rejected: control !== 'baseline',
        });
      });
    }
  });
}

try {
  await run();
} catch {
  recordMeasurement({ event: 'browser.diagnostic_canary_failed', kind: 'verification_failed' });
  process.exitCode = 1;
}
