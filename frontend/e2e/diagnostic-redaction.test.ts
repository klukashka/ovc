import { createHash } from 'node:crypto';

import { describe, expect, test } from 'vitest';

import {
  redactAddress,
  redactFailedRequest,
  redactMethod,
  redactPath,
  redactText,
  redactedTextLimit,
} from './diagnostic-redaction.ts';

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
const shortSecret = tokenShaped('short secret').slice(0, 15);
const cappedAtLimit = (text: string): string =>
  `${text.slice(0, redactedTextLimit - ' [truncated]'.length)} [truncated]`;

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

  test('a bearer token shorter than an opaque token is replaced', () => {
    const message = `Authorization: Bearer ${shortSecret}`;

    const redacted = redactText(message);

    expect(redacted).toBe('Authorization: Bearer :redacted');
  });

  test('a named secret assignment is replaced whatever prefix its name carries', () => {
    const assignments = [
      `token=${shortSecret}`,
      `access_token=${shortSecret}`,
      `refresh_token=${shortSecret}`,
      `ovc_refresh=${shortSecret}`,
      `x-session.id=${shortSecret}`,
    ];

    const redacted = assignments.map((assignment) => redactText(assignment));

    expect(redacted).toEqual([
      'token=:redacted',
      'access_token=:redacted',
      'refresh_token=:redacted',
      'ovc_refresh=:redacted',
      'x-session.id=:redacted',
    ]);
  });

  test('a JSON string value under a secret-named key is replaced', () => {
    const bodies = [
      `{"token":"${shortSecret}"}`,
      `{"accessToken": "${shortSecret}"}`,
      `{"refresh":"${shortSecret}","password":"a \\"quoted\\" ${shortSecret}"}`,
      `{"access":"${shortSecret}","cookie":"${shortSecret}","client_secret":"${shortSecret}"}`,
    ];

    const redacted = bodies.map((body) => redactText(body));

    expect(redacted).toEqual([
      '{"token":":redacted"}',
      '{"accessToken":":redacted"}',
      '{"refresh":":redacted","password":":redacted"}',
      '{"access":":redacted","cookie":":redacted","client_secret":":redacted"}',
    ]);
  });

  test('an API error code under the exact key code stays readable while other code values and prefixed code keys are replaced', () => {
    const body = `{"code":"not_found","Code":"forbidden","code":"K7QM-4XRP","authCode":"${shortSecret}","verification_code":"${shortSecret}"}`;

    const redacted = redactText(body);

    expect(redacted).toBe(
      '{"code":"not_found","Code":"forbidden","code":":redacted","authCode":":redacted","verification_code":":redacted"}',
    );
  });

  test('an assignment under a name that is not a secret stays readable and the secret after it is replaced', () => {
    const message = `page=2 lang=ru-RU step=token=${shortSecret}`;

    const redacted = redactText(message);

    expect(redacted).toBe('page=2 lang=ru-RU step=token=:redacted');
  });

  test('adversarial runs of secret names at the scanning bound are redacted without a closing equals sign or quote', () => {
    const run = `${'tokenx'.repeat(50)}-`.repeat(26);
    const repeatedNames = 'a.token-'.repeat(1000);
    const repeatedKeys = '"tokenx":'.repeat(888);
    const repeatedAssignments = 'x='.repeat(4000);
    const inputs = [run, `"${run}`, `"${run}":"${shortSecret}"`, repeatedNames, repeatedKeys, repeatedAssignments];

    const redacted = inputs.map((input) => redactText(input));

    expect(Math.max(...inputs.map((input) => input.length))).toBeLessThanOrEqual(8000);
    expect(redacted).toEqual([
      ':redacted',
      '":redacted',
      '":redacted":":redacted"',
      cappedAtLimit(repeatedNames),
      cappedAtLimit('"tokenx":":redacted":'.repeat(20)),
      cappedAtLimit(repeatedAssignments),
    ]);
  });

  test('the value of a Cookie or Set-Cookie header is replaced', () => {
    const headers = [
      `Cookie: theme=${shortSecret}; lang=${shortSecret}`,
      `set-cookie: ${shortSecret}; Path=/; HttpOnly`,
    ];

    const redacted = headers.map((header) => redactText(header));

    expect(redacted).toEqual(['Cookie: :redacted', 'set-cookie: :redacted']);
  });

  test('a free-standing opaque token of 32 or more characters is replaced', () => {
    const message = `request failed for ${resetToken} after retry`;

    const redacted = redactText(message);

    expect(redacted).toBe('request failed for :redacted after retry');
  });

  test('a short path segment that is neither plain nor opaque is replaced', () => {
    const path = '/api/v1/students/Maria/notes';

    const redacted = redactPath(path);

    expect(redacted).toBe('/api/v1/students/:redacted/notes');
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

const confirmationToken = tokenShaped('email confirmation');
const unsubscribeToken = tokenShaped('unsubscribe');
const conferencingKey = tokenShaped('conferencing routing key');
const engineError = 'net::ERR_CONNECTION_REFUSED';

function expectNoneOf(record: object, secrets: readonly string[]): void {
  const serialized = JSON.stringify(record);
  for (const secret of secrets) expect(serialized).not.toContain(secret);
}

describe('the redaction of a failed request record', () => {
  test('a token segment of each token route is replaced in the recorded path', () => {
    const addresses = [
      `${origin}/invite/${invitationToken}`,
      `${origin}/api/v1/invitations/${invitationToken}/accept`,
      `${origin}/verify-email/${confirmationToken}`,
      `${origin}/reset-password/${resetToken}`,
      `${origin}/api/v1/unsubscribe/${unsubscribeToken}`,
      `${origin}/api/v1/integrations/bbb/${conferencingKey}/ended`,
    ];

    const records = addresses.map((address) => redactFailedRequest('POST', address, engineError));

    expect(records.map((record) => record.path)).toEqual([
      '/invite/:redacted',
      '/api/v1/invitations/:redacted/accept',
      '/verify-email/:redacted',
      '/reset-password/:redacted',
      '/api/v1/unsubscribe/:redacted',
      '/api/v1/integrations/bbb/:redacted/ended',
    ]);
    expectNoneOf(records, [invitationToken, confirmationToken, resetToken, unsubscribeToken, conferencingKey]);
  });

  test('the query string and fragment are dropped from the recorded path', () => {
    const address = `${origin}/unsubscribe?token=${unsubscribeToken}#confirm=${resetToken}`;

    const record = redactFailedRequest('GET', address, 'net::ERR_ABORTED');

    expect(record).toEqual({ method: 'GET', path: '/unsubscribe', failure: 'cancellation' });
  });

  test('an address that is not absolute drops its query string and fragment and redacts its path', () => {
    const address = `/invite/${invitationToken}?token=${resetToken}#code=${confirmationToken}`;

    const record = redactFailedRequest('GET', address, engineError);

    expect(record).toEqual({ method: 'GET', path: '/invite/:redacted', failure: 'other' });
  });

  test('a JSON Web Token, a bearer token, a cookie and an email address in the path are replaced', () => {
    const address = `${origin}/api/v1/${jsonWebTokenValue}/${encodeURIComponent(`Bearer ${signature}`)}/ovc_refresh=${shortSecret}/${email}`;

    const record = redactFailedRequest('GET', address, engineError);

    expect(record.path).toBe('/api/v1/:redacted/:redacted/:redacted/:redacted');
    expectNoneOf(record, [jsonWebTokenValue, signature, shortSecret, email, 'Bearer', 'ovc_refresh']);
  });

  test('every cancellation text of the request-failure rule is recorded as a cancellation', () => {
    const reasons = [
      'net::ERR_ABORTED',
      'NS_BINDING_ABORTED',
      'cancelled',
      'Load request cancelled',
      'Provisional navigation canceled.',
    ];

    const failures = reasons.map((reason) => redactFailedRequest('GET', `${origin}/today`, reason).failure);

    expect(failures).toEqual(reasons.map(() => 'cancellation'));
  });

  test("any other failure is recorded as other, never with the engine's error text", () => {
    const reasons = [engineError, `NS_ERROR_FAILURE ${email}`, ''];

    const records = reasons.map((reason) => redactFailedRequest('GET', `${origin}/today`, reason));

    expect(records.map((record) => record.failure)).toEqual(['other', 'other', 'other']);
    expectNoneOf(records, [engineError, 'NS_ERROR_FAILURE', email]);
  });

  test('a method outside the plain upper-case form is recorded as OTHER', () => {
    const record = redactFailedRequest('X-Secret-Method-Name', `${origin}/today`, engineError);

    expect(record.method).toBe('OTHER');
  });
});
