import type { AuthTransport } from './transport.ts';

export interface ContractSubject {
  readonly transport: AuthTransport;
  readonly origin: string;
  readonly email: string;
  readonly password: string;
  readonly expireAccessTokens: () => Promise<void>;
}

export type ContractScenario = (subject: ContractSubject) => Promise<void>;

class ContractViolation extends Error {
  override readonly name = 'ContractViolation';
}

function check(condition: boolean, message: string): void {
  if (!condition) {
    throw new ContractViolation(message);
  }
}

async function errorCode(response: Response): Promise<string | undefined> {
  const body: unknown = await response.json();
  if (typeof body !== 'object' || body === null || !('error' in body)) {
    return undefined;
  }
  const error = body.error;
  return typeof error === 'object' && error !== null && 'code' in error && typeof error.code === 'string'
    ? error.code
    : undefined;
}

async function accessToken(response: Response): Promise<string> {
  check(response.status === 200, `expected 200 with a token, got ${String(response.status)}`);
  const body: unknown = await response.json();
  check(
    typeof body === 'object' &&
      body !== null &&
      'access_token' in body &&
      typeof body.access_token === 'string' &&
      'expires_in' in body &&
      body.expires_in === 900,
    'expected an access token valid for 900 seconds',
  );
  return typeof body === 'object' && body !== null && 'access_token' in body && typeof body.access_token === 'string'
    ? body.access_token
    : '';
}

function timeout(): AbortSignal {
  return AbortSignal.timeout(10_000);
}

function logoutRequest(subject: ContractSubject, token: string): Request {
  return new Request(`${subject.origin}/api/v1/auth/logout`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  });
}

function meRequest(subject: ContractSubject, token: string): Request {
  return new Request(`${subject.origin}/api/v1/users/me`, { headers: { Authorization: `Bearer ${token}` } });
}

export const authTransportContract: Readonly<Record<string, ContractScenario>> = {
  'a request sent with an expired token is refused as token_expired': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    await subject.expireAccessTokens();

    const response = await subject.transport.send(meRequest(subject, token));

    check(response.status === 401, `expected 401, got ${String(response.status)}`);
    check((await errorCode(response)) === 'token_expired', 'expected token_expired');
  },

  'two refreshes with the same cookie rotate once and revoke the session as reuse': async (subject) => {
    await accessToken(await subject.transport.login(subject.email, subject.password));

    const answers = await Promise.all([subject.transport.refresh(timeout()), subject.transport.refresh(timeout())]);

    const statuses = answers.map((answer) => answer.status).sort();
    check(statuses.join(',') === '200,401', `expected one 200 and one 401, got ${statuses.join(',')}`);
    const winner = answers.find((answer) => answer.status === 200);
    const revoked = await subject.transport.me(winner === undefined ? '' : await accessToken(winner), timeout());
    check(revoked.status === 401, `expected the rotated token to be revoked, got ${String(revoked.status)}`);
    const afterwards = await subject.transport.refresh(timeout());
    check(
      afterwards.status === 204 || afterwards.status === 401,
      `expected no token from the revoked session, got ${String(afterwards.status)}`,
    );
  },

  'a refresh without a session answers 204 with no token': async (subject) => {
    const response = await subject.transport.refresh(timeout());

    check(response.status === 204, `expected 204, got ${String(response.status)}`);
  },

  'a wrong password is refused as invalid credentials': async (subject) => {
    const response = await subject.transport.login(subject.email, `${subject.password} wrong`);

    check(response.status === 401, `expected 401, got ${String(response.status)}`);
    check((await errorCode(response)) === 'invalid_credentials', 'expected invalid_credentials');
  },

  'sign-in answers a token that reads the account': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const me = await subject.transport.me(token, timeout());

    check(me.status === 200, `expected 200, got ${String(me.status)}`);
    const body: unknown = await me.json();
    check(
      typeof body === 'object' && body !== null && 'email' in body && body.email === subject.email,
      'expected the account',
    );
  },

  'a refresh after sign-in answers a working token': async (subject) => {
    await accessToken(await subject.transport.login(subject.email, subject.password));

    const refreshed = await accessToken(await subject.transport.refresh(timeout()));

    check((await subject.transport.me(refreshed, timeout())).status === 200, 'expected the refreshed token to work');
  },

  'an expired token is refused as token_expired and a refresh renews it': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    await subject.expireAccessTokens();

    const expired = await subject.transport.me(token, timeout());
    const renewed = await accessToken(await subject.transport.refresh(timeout()));

    check(expired.status === 401, `expected 401, got ${String(expired.status)}`);
    check((await errorCode(expired)) === 'token_expired', 'expected token_expired');
    check((await subject.transport.me(renewed, timeout())).status === 200, 'expected the renewed token to work');
  },

  'sign-out ends the session for its token and its refresh cookie': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const signedOut = await subject.transport.send(logoutRequest(subject, token));

    check(signedOut.status === 204, `expected 204, got ${String(signedOut.status)}`);
    const reused = await subject.transport.me(token, timeout());
    check(reused.status === 401, `expected 401, got ${String(reused.status)}`);
    check((await errorCode(reused)) === 'unauthenticated', 'expected unauthenticated');
    check((await subject.transport.refresh(timeout())).status === 204, 'expected no session to refresh');
  },
};
