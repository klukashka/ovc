import { createHash } from 'node:crypto';

import { describe, expect, test } from 'vitest';

import { redactAddress, redactMethod, redactPath, redactText, redactedTextLimit } from './diagnostic-redaction.ts';

const origin = 'https://127.0.0.1:4300';
const tokenShaped = (seed: string): string => createHash('sha256').update(seed).digest('base64url');
const invitationToken = tokenShaped('invitation');
const resetToken = tokenShaped('password reset');
const signature = tokenShaped('storage signature');
const jsonWebTokenValue = [
  { alg: 'HS256', typ: 'JWT' },
  { sub: 'account', role: 'tutor' },
]
  .map((part) => Buffer.from(JSON.stringify(part)).toString('base64url'))
  .concat(tokenShaped('session signature'))
  .join('.');
const email = 'maria.tutor+e2e@example.invalid';

describe('the redaction of browser diagnostics details', () => {
  test('an address keeps its origin and route but drops its query string and fragment', () => {
    const address = `${origin}/reset-password?token=${resetToken}#step=${resetToken}`;

    const redacted = redactAddress(address);

    expect(redacted).toBe(`${origin}/reset-password?:redacted`);
  });

  test('a token carried as a path segment of a token route is replaced', () => {
    const addresses = [
      `${origin}/invite/${invitationToken}`,
      `${origin}/api/v1/invitations/abc/accept`,
      `${origin}/api/v1/integrations/bbb/routingkey/ended`,
    ];

    const redacted = addresses.map((address) => redactAddress(address));

    expect(redacted).toEqual([
      `${origin}/invite/:redacted`,
      `${origin}/api/v1/invitations/:redacted/accept`,
      `${origin}/api/v1/integrations/bbb/:redacted/ended`,
    ]);
  });

  test('identifiers, route words and asset names stay readable while opaque segments are replaced', () => {
    const path = `/api/v1/lessons/0b3c1f9e-6a4d-4c3b-9f1e-2d7a8b9c0d1e/join/assets/index-B3xQ_9aZ.js/${signature}`;

    const redacted = redactPath(path);

    expect(redacted).toBe(
      '/api/v1/lessons/0b3c1f9e-6a4d-4c3b-9f1e-2d7a8b9c0d1e/join/assets/index-B3xQ_9aZ.js/:redacted',
    );
  });

  test('a hex or base64url segment of 16 or more characters is replaced unless it is a route word', () => {
    const path =
      '/api/v1/files/abcdef0123456789abcd/attachments/k9x2m7q4w8z5r3t6/blocked-requesters/0b3c1f9e-6a4d-4c3b-9f1e-2d7a8b9c0d1e';

    const redacted = redactPath(path);

    expect(redacted).toBe(
      '/api/v1/files/:redacted/attachments/:redacted/blocked-requesters/0b3c1f9e-6a4d-4c3b-9f1e-2d7a8b9c0d1e',
    );
  });

  test('a console message loses signed storage queries, join links, bearer tokens, JWTs and email addresses', () => {
    const message = [
      `Failed to load https://storage.local/ovc/recording.mp4?X-Amz-Signature=${signature}&X-Amz-Credential=key`,
      `join https://meet.local/bigbluebutton/api/join?checksum=${signature}`,
      `relative /unsubscribe?token=${resetToken}`,
      `Authorization: Bearer ${invitationToken}`,
      `jwt ${jsonWebTokenValue}`,
      `user ${email}`,
      `reset token=${resetToken}`,
    ].join(' | ');

    const redacted = redactText(message);

    for (const secret of [signature, resetToken, invitationToken, jsonWebTokenValue, email, 'eyJ', '@example']) {
      expect(redacted).not.toContain(secret);
    }
    expect(redacted).toBe(
      [
        'Failed to load https://storage.local/ovc/recording.mp4?:redacted',
        'join https://meet.local/bigbluebutton/api/join?:redacted',
        'relative /unsubscribe?:redacted',
        'Authorization: Bearer :redacted',
        'jwt :redacted',
        'user :redacted',
        'reset token=:redacted',
      ].join(' | '),
    );
  });

  test('a long message is capped after redaction and never ends in a fragment of a secret', () => {
    const message = `${'Layout was forced '.repeat(600)}${email}`;

    const redacted = redactText(message);

    expect(redacted.length).toBeLessThanOrEqual(redactedTextLimit);
    expect(redacted.endsWith('[truncated]')).toBe(true);
    expect(redacted).not.toContain('@');
  });

  test('a method outside the plain upper-case form is replaced', () => {
    const methods = ['GET', 'PATCH', 'get', 'X-Secret-Method-Name'];

    const redacted = methods.map((method) => redactMethod(method));

    expect(redacted).toEqual(['GET', 'PATCH', 'OTHER', 'OTHER']);
  });
});
