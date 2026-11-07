import type { ReadinessFailure, ReadinessPhase } from './navigation-readiness.ts';
import { HttpNegativePhase, LanguageFailurePhase, type HttpNegative } from './diagnostic-expectations.ts';
import { test } from '@playwright/test';
import type { Browser, BrowserContext, BrowserContextOptions, Page, Request } from '@playwright/test';

export interface BoundedDiagnosticEvidence {
  readonly kind: 'readiness' | 'warning' | 'http_phase';
  readonly phase?: ReadinessPhase;
  readonly reason?: ReadinessFailure;
  readonly generation?: number;
  readonly pending?: number;
  readonly missing?: number;
  readonly opens?: number;
  readonly ready?: number;
  readonly closes?: number;
  readonly declared?: number;
  readonly matched?: number;
  readonly completed?: number;
  readonly faults?: number;
  readonly classification?: 'forced_layout' | 'other';
  readonly provenance?: 'injected_evaluator' | 'own_asset' | 'other';
}
const diagnosticCounts = new Map<string, number>();
export function emitBoundedDiagnostic(evidence: BoundedDiagnosticEvidence): void {
  if (process.env.E2E_READINESS_DIAGNOSTICS !== '1') return;
  const id = test.info().testId;
  const count = (diagnosticCounts.get(id) ?? 0) + 1;
  diagnosticCounts.set(id, count);
  if (count > 128) throw new Error('diagnostic_evidence_overflow');
  process.stdout.write(`OVC_SAFE_DIAGNOSTIC ${JSON.stringify(evidence)}\n`);
}

export type DiagnosticCategory =
  'console_warning' | 'console_error' | 'page_error' | 'http_error' | 'expectation_failure';
export type DiagnosticRoute = 'messages' | 'settings' | 'authentication' | 'unknown';
export interface DiagnosticRecord {
  readonly category: DiagnosticCategory;
  readonly classification: 'unknown';
  readonly engine: string;
  readonly stage: string;
  readonly route: DiagnosticRoute;
  readonly count: number;
}
export interface ConsoleExpectation {
  readonly expected: string[];
  readonly abortedPaths: RegExp[];
  readonly expectedWarnings: Map<string, number>;
}
export interface ObservedBrowser {
  readonly newContext: (options?: BrowserContextOptions) => Promise<BrowserContext>;
}

export function diagnosticRoute(address: string): DiagnosticRoute {
  if (!URL.canParse(address)) return 'unknown';
  const path = new URL(address).pathname;
  if (path === '/messages' || path.startsWith('/messages/')) return 'messages';
  if (path === '/settings' || path.startsWith('/settings/')) return 'settings';
  if (['/sign-in', '/sign-up', '/verify-email', '/reset-password', '/unsubscribe'].includes(path))
    return 'authentication';
  return 'unknown';
}

export function observeBrowserContext(
  context: BrowserContext,
  engine: string,
  stage = 'journey',
): {
  readonly finish: () => Promise<void>;
  readonly records: () => readonly DiagnosticRecord[];
  readonly expect: (page: Page) => ConsoleExpectation;
  readonly expectHttp: (page: Page, negative: HttpNegative) => void;
  readonly finishHttp: (page: Page) => Promise<void>;
  readonly beginLanguageFailure: (page: Page, origin: string, language: 'en' | 'ru') => void;
  readonly finishLanguageFailure: (page: Page) => Promise<void>;
  readonly verify: () => void;
  readonly warnings: () => number;
  readonly errors: () => number;
} {
  const records = new Map<string, DiagnosticRecord>();
  const expectations = new Map<Page, ConsoleExpectation>();
  const record = (category: DiagnosticCategory, page: Page | null): void => {
    const route = diagnosticRoute(page?.url() ?? '');
    const key = `${category}:${route}`;
    const previous = records.get(key);
    records.set(key, {
      category,
      classification: 'unknown',
      engine,
      stage,
      route,
      count: Math.min(1_000_000, (previous?.count ?? 0) + 1),
    });
  };
  const phases = new Map<Page, HttpNegativePhase>();
  const languages = new Map<Page, LanguageFailurePhase>();
  const started = new Map<Request, { readonly phase: HttpNegativePhase; readonly id: number }>();
  const pending = new Map<Page, Set<Promise<void>>>();
  let sequence = 0;
  const finishPhase = (page: Page): void => {
    const phase = phases.get(page);
    if (phase === undefined) return;
    phases.delete(page);
    for (const [request, entry] of started) if (entry.phase === phase) started.delete(request);
    emitBoundedDiagnostic({ kind: 'http_phase', ...phase.evidence() });
    const result = phase.finish();
    for (let index = 0; index < result.httpFailures; index += 1) record('http_error', page);
    for (let index = 0; index < result.consoleFailures; index += 1) record('console_error', page);
  };
  const finishLanguage = (page: Page, phase: LanguageFailurePhase): void => {
    const valid = phase.finish();
    for (let index = 0; index < phase.unexpectedWarnings(); index += 1) record('console_warning', page);
    if (!valid && (phase.expectedWarningMissing() || phase.unexpectedWarnings() === 0))
      record('expectation_failure', page);
  };
  const pageOf = (request: Request): Page | null => {
    try {
      return request.frame().page();
    } catch {
      record('page_error', null);
      return null;
    }
  };
  context.on('request', (request) => {
    const page = pageOf(request);
    if (page === null) return;
    const phase = phases.get(page);
    languages.get(page)?.request(request.url());
    if (phase !== undefined) {
      if (started.size >= 2000) {
        record('http_error', page);
        return;
      }
      sequence += 1;
      phase.start(sequence, request.method(), request.url());
      started.set(request, { phase, id: sequence });
    }
  });
  context.on('requestfailed', (request) => {
    const page = pageOf(request);
    if (page === null) return;
    languages.get(page)?.failure(request.url(), request.failure() !== null);
  });
  context.on('response', (response) => {
    if (response.status() < 400) return;
    const request = response.request();
    const page = pageOf(request);
    if (page === null) return;
    const entry = started.get(request);
    if (entry === undefined || phases.get(page) !== entry.phase) {
      record('http_error', page);
      return;
    }
    if (!entry.phase.response(entry.id, response.status(), response.statusText())) return;
    const outstanding = pending.get(page) ?? new Set<Promise<void>>();
    pending.set(page, outstanding);
    const completion = response
      .finished()
      .then(
        (error) => {
          entry.phase.complete(entry.id, error === null);
        },
        () => {
          entry.phase.complete(entry.id, false);
        },
      )
      .finally(() => {
        outstanding.delete(completion);
      });
    outstanding.add(completion);
  });
  context.on('console', (message) => {
    const severity = message.type();
    if (severity !== 'warning' && severity !== 'error') return;
    const page = message.page();
    if (severity === 'warning') {
      const address = message.location().url;
      const ownAsset =
        page !== null &&
        URL.canParse(address) &&
        URL.canParse(page.url()) &&
        new URL(address).origin === new URL(page.url()).origin &&
        new URL(address).pathname.startsWith('/assets/');
      emitBoundedDiagnostic({
        kind: 'warning',
        classification: message.text().startsWith('Layout was forced') ? 'forced_layout' : 'other',
        provenance: address === 'debugger eval code' ? 'injected_evaluator' : ownAsset ? 'own_asset' : 'other',
      });
    }
    if (
      severity === 'warning' &&
      page !== null &&
      languages.get(page)?.warning(message.text(), message.location().url) === true
    )
      return;
    if (severity === 'warning' && page !== null) {
      const known = expectations.get(page)?.expectedWarnings;
      const text = message.text();
      const matched = [...(known?.keys() ?? [])].find((fragment) => text.includes(fragment));
      if (known !== undefined && matched !== undefined) {
        known.set(matched, (known.get(matched) ?? 0) + 1);
        return;
      }
    }
    if (severity === 'error') {
      const text = message.text();
      const address = message.location().url;
      if (page !== null && phases.get(page)?.capture(text, address) === true) return;
      const expectation = page === null ? undefined : expectations.get(page);
      const index = expectation?.expected.indexOf(text) ?? -1;
      if (expectation !== undefined && index !== -1) {
        expectation.expected.splice(index, 1);
        return;
      }
      const path = URL.canParse(address) ? new URL(address).pathname : '';
      if (
        text.startsWith('Failed to load resource') &&
        expectation?.abortedPaths.some((pattern) => pattern.test(path)) === true
      )
        return;
    }
    record(severity === 'warning' ? 'console_warning' : 'console_error', page);
  });
  context.on('weberror', (error) => {
    record('page_error', error.page());
  });
  const result = {
    finish: async (): Promise<void> => {
      await Promise.all([...pending.values()].flatMap((entries) => [...entries]));
      result.verify();
    },
    records: (): readonly DiagnosticRecord[] => [...records.values()],
    expect: (page: Page): ConsoleExpectation => {
      const existing = expectations.get(page);
      if (existing !== undefined) return existing;
      const created: ConsoleExpectation = { expected: [], abortedPaths: [], expectedWarnings: new Map() };
      expectations.set(page, created);
      return created;
    },
    expectHttp: (page: Page, negative: HttpNegative): void => {
      const phase = phases.get(page) ?? new HttpNegativePhase();
      phase.add(negative);
      phases.set(page, phase);
    },
    finishHttp: async (page: Page): Promise<void> => {
      await Promise.all([...(pending.get(page) ?? [])]);
      if (!page.isClosed()) await page.evaluate(() => Promise.resolve());
      finishPhase(page);
    },
    beginLanguageFailure: (page: Page, origin: string, language: 'en' | 'ru'): void => {
      if (languages.has(page)) throw new Error('language_failure_phase_already_active');
      languages.set(page, new LanguageFailurePhase(engine, origin, language));
    },
    finishLanguageFailure: async (page: Page): Promise<void> => {
      if (!page.isClosed()) await page.evaluate(() => Promise.resolve());
      const phase = languages.get(page);
      if (phase === undefined) throw new Error('language_failure_phase_missing');
      languages.delete(page);
      finishLanguage(page, phase);
    },
    warnings: (): number =>
      [...records.values()]
        .filter((entry) => entry.category === 'console_warning')
        .reduce((sum, entry) => sum + entry.count, 0),
    errors: (): number =>
      [...records.values()]
        .filter((entry) => entry.category !== 'console_warning')
        .reduce((sum, entry) => sum + entry.count, 0),
    verify: (): void => {
      for (const page of phases.keys()) finishPhase(page);
      for (const [page, phase] of languages) {
        finishLanguage(page, phase);
      }
      languages.clear();
      const missing = [...expectations.values()].reduce(
        (sum, entry) =>
          sum + entry.expected.length + [...entry.expectedWarnings.values()].filter((seen) => seen === 0).length,
        0,
      );
      if (records.size !== 0 || missing !== 0)
        throw new Error(
          JSON.stringify({
            event: 'browser.diagnostics_failed',
            records: result.records(),
            missingExpectedErrors: missing,
          }),
        );
    },
  };
  return result;
}

export async function installPolicyObserver(context: BrowserContext): Promise<void> {
  await context.addInitScript(() => {
    document.addEventListener('securitypolicyviolation', () => {
      console.error('csp_violation');
    });
  });
}

export function observedBrowser(
  browser: Browser,
  observe: (context: BrowserContext) => Promise<void>,
): ObservedBrowser {
  return {
    newContext: async (options): Promise<BrowserContext> => {
      const context = await browser.newContext(options);
      try {
        await observe(context);
        return context;
      } catch (error) {
        try {
          await context.close();
        } catch (cleanup) {
          throw new AggregateError([error, cleanup], 'browser context setup failed');
        }
        throw error;
      }
    },
  };
}
