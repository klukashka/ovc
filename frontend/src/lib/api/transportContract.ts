import type { components } from './schema.ts';
import type { AuthTransport } from './transport.ts';

type EmailGroup = components['schemas']['EmailPreference']['group'];

export interface ContractSubject {
  readonly transport: AuthTransport;
  readonly origin: string;
  readonly email: string;
  readonly password: string;
  readonly otherTutorEmail: string;
  readonly otherTutorPassword: string;
  readonly student: () => Promise<{ readonly email: string; readonly password: string }>;
  readonly expireAccessTokens: () => Promise<void>;
  readonly blockAccount: () => Promise<void>;
  readonly unsubscribeToken: (group: EmailGroup) => Promise<string>;
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

function cardRequest(subject: ContractSubject, token: string, path: string, body?: unknown): Request {
  return new Request(`${subject.origin}/api/v1/students${path}`, {
    method: body === undefined ? 'GET' : 'POST',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
}

function joinRequestsRequest(subject: ContractSubject, token: string): Request {
  return new Request(`${subject.origin}/api/v1/join-requests`, { headers: { Authorization: `Bearer ${token}` } });
}

function notificationsRequest(subject: ContractSubject, token: string): Request {
  return new Request(`${subject.origin}/api/v1/notifications`, { headers: { Authorization: `Bearer ${token}` } });
}

function emailPreferencesRequest(subject: ContractSubject, token: string, body?: unknown): Request {
  return new Request(`${subject.origin}/api/v1/users/me/email-preferences`, {
    method: body === undefined ? 'GET' : 'PUT',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
}

function unsubscribeRequest(subject: ContractSubject, method: 'GET' | 'POST', token: string): Request {
  const path = method === 'GET' ? '/api/v1/unsubscribe/preview' : '/api/v1/unsubscribe';
  return new Request(`${subject.origin}${path}?${new URLSearchParams({ token }).toString()}`, { method });
}

async function emailGroups(response: Response): Promise<string> {
  check(response.status === 200, `expected 200 with the email groups, got ${String(response.status)}`);
  return JSON.stringify(await response.json());
}

function cardFields(value: unknown): { id: unknown; name: unknown; state: unknown; url: unknown } {
  const card = typeof value === 'object' && value !== null ? value : {};
  const invitation = 'invitation' in card && typeof card.invitation === 'object' ? card.invitation : null;
  return {
    id: 'id' in card ? card.id : undefined,
    name: 'display_name' in card ? card.display_name : undefined,
    state: 'state' in card ? card.state : undefined,
    url: invitation !== null && 'url' in invitation ? invitation.url : undefined,
  };
}

const headersThatVary: ReadonlySet<string> = new Set(['date', 'content-length', 'x-request-id']);

interface RefusalShape {
  readonly status: number;
  readonly code: string | undefined;
  readonly same: (other: RefusalShape) => boolean;
  readonly describe: () => string;
}

async function refusalShape(response: Response): Promise<RefusalShape> {
  const body: unknown = await response.json();
  const error = typeof body === 'object' && body !== null && 'error' in body ? body.error : null;
  const code = typeof error === 'object' && error !== null && 'code' in error ? error.code : undefined;
  const message = typeof error === 'object' && error !== null && 'message' in error ? error.message : undefined;
  const headers = [...response.headers.entries()]
    .filter(([name]) => !headersThatVary.has(name))
    .map(([name, value]) => `${name}: ${value}`)
    .sort()
    .join('; ');
  const summary = `${String(response.status)} ${String(code)} ${String(message)} [${headers}]`;
  return {
    status: response.status,
    code: typeof code === 'string' ? code : undefined,
    same: (other) => other.describe() === summary,
    describe: () => summary,
  };
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

  'a tutor reads back the card it created and an unknown card is not found': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const created = await subject.transport.send(cardRequest(subject, token, '', { display_name: 'Contract student' }));
    const createdCard = cardFields(await created.json());
    const read = await subject.transport.send(cardRequest(subject, token, `/${String(createdCard.id)}`));
    const missing = await subject.transport.send(cardRequest(subject, token, '/00000000-0000-4000-8000-00000000ffff'));

    check(created.status === 201, `expected 201, got ${String(created.status)}`);
    check(typeof createdCard.id === 'string', 'expected a card identifier');
    check(createdCard.name === 'Contract student' && createdCard.state === 'invited', 'expected an invited card');
    check(typeof createdCard.url === 'string', 'expected an open invitation link');
    check(read.status === 200, `expected 200, got ${String(read.status)}`);
    const readCard = cardFields(await read.json());
    check(readCard.id === createdCard.id && readCard.url === createdCard.url, 'expected the same card and link');
    check(missing.status === 404, `expected 404, got ${String(missing.status)}`);
    check((await errorCode(missing)) === 'not_found', 'expected not_found');
  },

  'another tutor cannot read a tutor’s card and gets not_found': async (subject) => {
    const owner = await accessToken(await subject.transport.login(subject.email, subject.password));
    const created = cardFields(
      await (await subject.transport.send(cardRequest(subject, owner, '', { display_name: 'Private student' }))).json(),
    );
    const other = await accessToken(await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword));

    const read = await subject.transport.send(cardRequest(subject, other, `/${String(created.id)}`));

    check(typeof created.id === 'string', 'expected a card identifier');
    check(read.status === 404, `expected 404, got ${String(read.status)}`);
    check((await errorCode(read)) === 'not_found', 'expected not_found');
  },

  'a student is refused on the card routes as on an unknown route with not_found': async (subject) => {
    const tutor = await accessToken(await subject.transport.login(subject.email, subject.password));
    const card = cardFields(
      await (await subject.transport.send(cardRequest(subject, tutor, '', { display_name: 'A tutor’s card' }))).json(),
    );
    const { email, password } = await subject.student();
    const student = await accessToken(await subject.transport.login(email, password));

    const created = await refusalShape(
      await subject.transport.send(cardRequest(subject, student, '', { display_name: 'Someone' })),
    );
    const read = await refusalShape(await subject.transport.send(cardRequest(subject, student, `/${String(card.id)}`)));
    const unknown = await refusalShape(
      await subject.transport.send(
        new Request(`${subject.origin}/api/v1/no-such-route`, { headers: { Authorization: `Bearer ${student}` } }),
      ),
    );

    check(typeof card.id === 'string', 'expected a card identifier');
    check(unknown.status === 404 && unknown.code === 'not_found', 'expected not_found on an unknown route');
    check(created.same(unknown), `expected creation to be refused as an unknown route: ${created.describe()}`);
    check(read.same(unknown), `expected reading to be refused as an unknown route: ${read.describe()}`);
  },

  'a tutor without join requests reads an empty page (a refused read uses the refusal envelope the 404 scenarios check)':
    async (subject) => {
      const token = await accessToken(
        await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword),
      );

      const read = await subject.transport.send(joinRequestsRequest(subject, token));

      check(read.status === 200, `expected 200, got ${String(read.status)}`);
      const body: unknown = await read.json();
      check(
        typeof body === 'object' &&
          body !== null &&
          Object.keys(body).sort().join(',') === 'items,next_cursor' &&
          'items' in body &&
          Array.isArray(body.items) &&
          body.items.length === 0 &&
          'next_cursor' in body &&
          body.next_cursor === null,
        `expected exactly {items: [], next_cursor: null}, got ${JSON.stringify(body)}`,
      );
    },

  'a tutor without notifications reads an empty page with no unread rows': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword));

    const read = await subject.transport.send(notificationsRequest(subject, token));

    check(read.status === 200, `expected 200, got ${String(read.status)}`);
    const body: unknown = await read.json();
    check(
      typeof body === 'object' &&
        body !== null &&
        Object.keys(body).sort().join(',') === 'items,next_cursor,unread_count' &&
        'items' in body &&
        Array.isArray(body.items) &&
        body.items.length === 0 &&
        'next_cursor' in body &&
        body.next_cursor === null &&
        'unread_count' in body &&
        body.unread_count === 0,
      `expected exactly {items: [], next_cursor: null, unread_count: 0}, got ${JSON.stringify(body)}`,
    );
  },

  'a student reading the join requests is refused as on an unknown route with not_found': async (subject) => {
    const { email, password } = await subject.student();
    const student = await accessToken(await subject.transport.login(email, password));

    const read = await refusalShape(await subject.transport.send(joinRequestsRequest(subject, student)));
    const unknown = await refusalShape(
      await subject.transport.send(
        new Request(`${subject.origin}/api/v1/no-such-route`, { headers: { Authorization: `Bearer ${student}` } }),
      ),
    );

    check(unknown.status === 404 && unknown.code === 'not_found', 'expected not_found on an unknown route');
    check(read.same(unknown), `expected the read to be refused as an unknown route: ${read.describe()}`);
  },

  'the link of a new card shows its tutor without a session': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));
    const created = cardFields(
      await (await subject.transport.send(cardRequest(subject, token, '', { display_name: 'Invited student' }))).json(),
    );
    const invitationToken =
      typeof created.url === 'string' ? (new URL(created.url).pathname.split('/').at(-1) ?? '') : '';
    const me: unknown = await (await subject.transport.me(token, timeout())).json();
    const tutorName = typeof me === 'object' && me !== null && 'name' in me ? me.name : undefined;

    const page = await subject.transport.send(
      new Request(`${subject.origin}/api/v1/invitations/${encodeURIComponent(invitationToken)}`),
    );

    check(page.status === 200, `expected 200, got ${String(page.status)}`);
    const body: unknown = await page.json();
    const tutor = typeof body === 'object' && body !== null && 'tutor' in body ? body.tutor : undefined;
    check(
      typeof tutor === 'object' &&
        tutor !== null &&
        'name' in tutor &&
        tutor.name === tutorName &&
        'subjects' in tutor &&
        Array.isArray(tutor.subjects),
      'expected the tutor of the card',
    );
  },

  'an unknown invitation token answers not_found without a session': async (subject) => {
    const response = await subject.transport.send(
      new Request(`${subject.origin}/api/v1/invitations/an-unknown-invitation-token`),
    );

    check(response.status === 404, `expected 404, got ${String(response.status)}`);
    check((await errorCode(response)) === 'not_found', 'expected not_found');
  },

  'an unsubscribe link previews its group and turns it off only on POST, and a tampered link is token_invalid': async (
    subject,
  ) => {
    const access = await accessToken(await subject.transport.login(subject.email, subject.password));
    const unsubscribeToken = await subject.unsubscribeToken('submissions');
    const before = await emailGroups(await subject.transport.send(emailPreferencesRequest(subject, access)));

    const preview = await subject.transport.send(unsubscribeRequest(subject, 'GET', unsubscribeToken));
    const afterPreview = await emailGroups(await subject.transport.send(emailPreferencesRequest(subject, access)));
    const applied = await subject.transport.send(unsubscribeRequest(subject, 'POST', unsubscribeToken));
    const afterPost = await emailGroups(await subject.transport.send(emailPreferencesRequest(subject, access)));
    const saved = await emailGroups(
      await subject.transport.send(
        emailPreferencesRequest(subject, access, { groups: [{ group: 'submissions', enabled: true }] }),
      ),
    );
    const tampered = await subject.transport.send(unsubscribeRequest(subject, 'GET', `${unsubscribeToken}x`));

    const allOn =
      '{"groups":[{"group":"lesson_reminders","enabled":true},{"group":"submissions","enabled":true},{"group":"message_digest","enabled":true}]}';
    check(before === allOn, `expected the tutor's three groups on, got ${before}`);
    check(preview.status === 200, `expected 200, got ${String(preview.status)}`);
    check(JSON.stringify(await preview.json()) === '{"group":"submissions"}', 'expected exactly the group');
    check(afterPreview === allOn, `expected the preview to change nothing, got ${afterPreview}`);
    check(applied.status === 204, `expected 204, got ${String(applied.status)}`);
    check(afterPost.includes('{"group":"submissions","enabled":false}'), `expected submissions off, got ${afterPost}`);
    check(saved === allOn, `expected the save to answer the full list, got ${saved}`);
    check(tampered.status === 422, `expected 422, got ${String(tampered.status)}`);
    check((await errorCode(tampered)) === 'token_invalid', 'expected token_invalid');
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

  'a second sign-in in the same browser replaces the refresh cookie': async (subject) => {
    const earlier = await accessToken(await subject.transport.login(subject.email, subject.password));
    const later = await accessToken(await subject.transport.login(subject.email, subject.password));

    const refreshed = await accessToken(await subject.transport.refresh(timeout()));
    const signedOut = await subject.transport.send(logoutRequest(subject, refreshed));

    check(signedOut.status === 204, `expected 204, got ${String(signedOut.status)}`);
    check(
      (await subject.transport.me(later, timeout())).status === 401,
      'expected the later session to be the one refreshed',
    );
    check((await subject.transport.me(earlier, timeout())).status === 200, 'expected the earlier session to stay');
  },

  'a blocked account loses its session and is refused as account_blocked after the correct password': async (
    subject,
  ) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    await subject.blockAccount();

    const reused = await subject.transport.me(token, timeout());
    check(reused.status === 401, `expected the session to be revoked, got ${String(reused.status)}`);
    const refreshed = await subject.transport.refresh(timeout());
    check(refreshed.status === 401, `expected the revoked session to be refused, got ${String(refreshed.status)}`);
    const wrong = await subject.transport.login(subject.email, `${subject.password} wrong`);
    check((await errorCode(wrong)) === 'invalid_credentials', 'expected a wrong password to stay invalid_credentials');
    const right = await subject.transport.login(subject.email, subject.password);
    check(right.status === 403, `expected 403, got ${String(right.status)}`);
    check((await errorCode(right)) === 'account_blocked', 'expected account_blocked');
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
