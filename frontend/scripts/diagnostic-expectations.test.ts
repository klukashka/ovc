import { ownFailurePosition } from '../e2e/diagnostic-expectations.ts';
import { expect, test } from 'vitest';

import { HttpNegativePhase, LanguageFailurePhase } from '../e2e/diagnostic-expectations.ts';

const origin = 'https://fixture.example';
const address = `${origin}/api/v1/auth/login`;
const native401 = 'Failed to load resource: the server responded with a status of 401 (Unauthorized)';
const asset = `${origin}/assets/language-en-example.js`;
const caller = `${origin}/assets/index-example.js`;
const warning = `[JavaScript Warning: "Loading failed for the module with source “${asset}”." {file: "${caller}" line: 0}]`;

function negative(): HttpNegativePhase {
  const phase = new HttpNegativePhase();
  phase.add({ method: 'POST', address, status: 401 });
  return phase;
}

test.each(['console_first', 'response_first', 'no_console'] as const)(
  'finite HTTP ownership resolves %s with complete real-response facts',
  (ordering) => {
    const phase = negative();
    phase.start(1, 'POST', address);

    if (ordering === 'console_first') phase.capture(native401, address);
    phase.response(1, 401, 'Unauthorized');
    phase.complete(1, true);
    if (ordering === 'response_first') phase.capture(native401, address);
    const result = phase.finish();

    expect(result).toEqual({ httpFailures: 0, consoleFailures: 0 });
  },
);

test.each([
  'method',
  'query',
  'origin',
  'status',
  'missing',
  'incomplete',
  'duplicate',
  'location',
  'ambiguous',
  'extra_response',
] as const)('finite HTTP ownership rejects %s without broad status matching', (failure) => {
  const phase = negative();
  if (failure === 'ambiguous') phase.add({ method: 'POST', address: `${origin}/api/v1/auth/refresh`, status: 401 });

  if (failure !== 'missing') {
    phase.start(
      1,
      failure === 'method' ? 'GET' : 'POST',
      failure === 'query'
        ? `${address}?unexpected=1`
        : failure === 'origin'
          ? address.replace(origin, 'https://other.example')
          : address,
    );
    phase.response(1, failure === 'status' ? 403 : 401, 'Unauthorized');
    if (failure !== 'incomplete') phase.complete(1, true);
    phase.capture(
      native401,
      failure === 'ambiguous' ? '' : failure === 'location' ? `${origin}/api/v1/other` : address,
    );
    if (failure === 'duplicate') phase.capture(native401, address);
    if (failure === 'ambiguous') {
      phase.start(2, 'POST', `${origin}/api/v1/auth/refresh`);
      phase.response(2, 401, 'Unauthorized');
      phase.complete(2, true);
    }
    if (failure === 'extra_response') {
      phase.start(2, 'POST', address);
      phase.response(2, 401, 'Unauthorized');
      phase.complete(2, true);
    }
  }
  const result = phase.finish();

  expect(result.httpFailures + result.consoleFailures).toBeGreaterThan(0);
});

test('ending an HTTP phase revokes all remaining grants and candidates', () => {
  const phase = negative();

  const result = phase.finish();
  const late = phase.capture(native401, address);

  expect(result.httpFailures).toBe(1);
  expect(late).toBe(false);
  expect((): void => {
    phase.add({ method: 'POST', address, status: 401 });
  }).toThrow('http_negative_declaration_invalid');
});

test('missing console location is accepted only for a unique completed phase response', () => {
  const phase = negative();
  phase.start(1, 'POST', address);

  phase.capture(native401, '');
  phase.response(1, 401, 'Unauthorized');
  phase.complete(1, true);

  expect(phase.finish()).toEqual({ httpFailures: 0, consoleFailures: 0 });
});

test.each(['warning_first', 'failure_first'] as const)('the named language negative phase resolves %s', (ordering) => {
  const phase = new LanguageFailurePhase('firefox', origin, 'en');
  phase.request(asset);

  if (ordering === 'warning_first') phase.warning(warning, caller);
  phase.failure(asset, true);
  if (ordering === 'failure_first') phase.warning(warning, caller);

  expect(phase.finish()).toBe(true);
});

test.each(['engine', 'language', 'request', 'failure', 'duplicate', 'other_warning', 'caller', 'phase'] as const)(
  'the named language warning rejects %s',
  (failure) => {
    const phase = new LanguageFailurePhase(
      failure === 'engine' ? 'chromium' : 'firefox',
      origin,
      failure === 'language' ? 'ru' : 'en',
    );
    phase.request(failure === 'request' ? `${asset}?wrong=1` : asset);

    phase.failure(asset, failure !== 'failure');
    phase.warning(warning, failure === 'caller' ? `${origin}/wrong.js` : caller);
    if (failure === 'duplicate') phase.warning(warning, caller);
    if (failure === 'other_warning') phase.warning('synthetic-private-unrelated-warning', caller);
    const result = phase.finish();

    expect(result).toBe(failure === 'phase' || failure === 'engine');
    if (failure === 'duplicate' || failure === 'other_warning') expect(phase.unexpectedWarnings()).toBe(1);
    if (failure === 'phase' || failure === 'engine') expect(phase.warning(warning, caller)).toBe(false);
  },
);

test('HTTP diagnostic counts preserve unmatched declarations and do not infer unobserved traffic', () => {
  const phase = new HttpNegativePhase();
  phase.add({ method: 'GET', address: 'https://example.test/api/private-identity', status: 401 });
  phase.add({ method: 'POST', address: 'https://example.test/api/refresh', status: 401 });

  phase.start(1, 'GET', 'https://example.test/api/private-identity');
  phase.response(1, 401, 'Unauthorized');
  phase.complete(1, true);
  const evidence = phase.evidence();
  const verdict = phase.finish();

  expect(evidence).toEqual({ declared: 2, matched: 1, completed: 1, faults: 0 });
  expect(JSON.stringify(evidence)).not.toContain('private-identity');
  expect(verdict.httpFailures).toBe(1);
});

test('failure position exports only local source coordinates from real stack frames', () => {
  const error = new Error('private diagnostic content');
  const source = new URL(import.meta.url).pathname;

  const position = ownFailurePosition(error, source);
  const unrelated = ownFailurePosition(error, '/different/source.ts');

  expect(position.line).toBeGreaterThan(0);
  expect(position.column).toBeGreaterThan(0);
  expect(JSON.stringify(position)).not.toContain('private diagnostic content');
  expect(unrelated).toEqual({ line: -1, column: -1 });
});
