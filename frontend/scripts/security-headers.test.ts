import { expect, test } from 'vitest';

import { contentSecurityPolicy, contentSecurityPolicyWithTestRoom, previewHeaders } from '../security-headers.ts';
import { frontendConfiguration } from '../vite.config.ts';

test('without a test room the preview server keeps every frame out', () => {
  const unset = previewHeaders(undefined)['Content-Security-Policy'];
  const empty = previewHeaders('')['Content-Security-Policy'];

  expect(unset).toBe(contentSecurityPolicy);
  expect(empty).toBe(contentSecurityPolicy);
  expect(contentSecurityPolicy).toContain("frame-src 'none'");
});

test('the test room origin is admitted only as the frame source', () => {
  const policy = contentSecurityPolicyWithTestRoom('https://127.0.0.1:18900');

  expect(policy).toContain('frame-src https://127.0.0.1:18900;');
  expect(policy).toContain("frame-ancestors 'none'");
  expect(policy).toContain("connect-src 'self'");
});

test.each([
  'https://room.example',
  'https://127.0.0.1.example:18900',
  'https://localhost:18900',
  'http://127.0.0.1:18900',
  'https://127.0.0.1:18900/path',
  "https://127.0.0.1:18900; script-src 'unsafe-inline'",
  '*',
])('the origin %s is refused', (origin) => {
  const refused = (): string => contentSecurityPolicyWithTestRoom(origin);

  expect(refused).toThrow('The test room frame origin');
});

test('the preview server built without the setting sends the production frame policy', () => {
  const configuration = frontendConfiguration('production', {});

  const headers = configuration.preview?.headers;

  expect(headers).toEqual(previewHeaders(undefined));
  expect(JSON.stringify(headers)).toContain("frame-src 'none'");
});
