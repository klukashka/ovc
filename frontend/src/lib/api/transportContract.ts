import type { AuthTransport } from './transport.ts';

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
  readonly unsubscribeToken: () => Promise<string>;
  readonly seedTwoLessons: () => Promise<SeededLessons>;
  readonly seedSubmission: () => Promise<SeededSubmission>;
  readonly fillTutorFileQuota: () => Promise<void>;
}

export interface SeededSubmission {
  readonly email: string;
  readonly password: string;
  readonly submissionId: string;
  readonly textQuestionId: string;
  readonly fileQuestionId: string;
}

export interface SeededLessons {
  readonly from: string;
  readonly to: string;
  readonly laterStart: string;
  readonly earlierStart: string;
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

const emptyWeek = { from: '2026-11-01T21:00:00.000Z', to: '2026-11-08T21:00:00.000Z' };

function calendarRequest(subject: ContractSubject, token: string, query: Record<string, string> = emptyWeek): Request {
  return new Request(`${subject.origin}/api/v1/calendar?${new URLSearchParams(query).toString()}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

async function fieldProblems(response: Response): Promise<string> {
  const body: unknown = await response.json();
  if (typeof body !== 'object' || body === null || !('error' in body)) {
    return '';
  }
  const error = body.error;
  if (typeof error !== 'object' || error === null || !('code' in error) || !('fields' in error)) {
    return '';
  }
  const fields: unknown = error.fields;
  const described = Array.isArray(fields)
    ? fields.map((field: unknown) =>
        typeof field === 'object' && field !== null && 'path' in field && 'code' in field
          ? `${String(field.path)}:${String(field.code)}`
          : '?',
      )
    : [];
  return `${String(error.code)} ${described.join(',')}`;
}

function startsOf(body: unknown): number[] {
  if (typeof body !== 'object' || body === null || !('items' in body) || !Array.isArray(body.items)) {
    return [];
  }
  const items: unknown[] = body.items;
  return items.map((item) =>
    typeof item === 'object' && item !== null && 'starts_at' in item && typeof item.starts_at === 'string'
      ? Date.parse(item.starts_at)
      : Number.NaN,
  );
}

const dayMilliseconds = 24 * 60 * 60 * 1_000;

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

function submissionRequest(subject: ContractSubject, token: string, id: string): Request {
  return new Request(`${subject.origin}/api/v1/submissions/${encodeURIComponent(id)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
}

interface SentAnswerValue {
  readonly question_id: string;
  readonly value: Readonly<Record<string, unknown>>;
}

function answersRequest(
  subject: ContractSubject,
  token: string,
  id: string,
  version: number,
  answers: readonly SentAnswerValue[],
): Request {
  return new Request(`${subject.origin}/api/v1/submissions/${encodeURIComponent(id)}/answers`, {
    method: 'PUT',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ version, save_id: crypto.randomUUID(), answers }),
  });
}

function answerUploadRequest(subject: ContractSubject, token: string, seeded: SeededSubmission): Request {
  const scope = `submission:${seeded.submissionId}:${seeded.fileQuestionId}`;
  const body = new FormData();
  body.append(
    'file',
    new File(['%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n'], 'Essay.pdf', {
      type: 'application/pdf',
    }),
  );
  return new Request(
    `${subject.origin}/api/v1/uploads?${new URLSearchParams({ context: 'answer', scope }).toString()}`,
    {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}` },
      body,
    },
  );
}

const randomSubmissionId = '6f1e2d3c-4b5a-4978-8a6b-5c4d3e2f1a0b';

const studentSubmissionFields: readonly string[] = [
  'id',
  'title',
  'tutor',
  'status',
  'late',
  'issued_before_reclaim',
  'deadline_at',
  'version',
  'blocks',
  'answers',
  'editable_question_ids',
  'review',
];

const answerKeyFields: readonly string[] = [
  'points',
  'max_points',
  'auto_points',
  'override_points',
  'final_points',
  'percentage',
  'correct',
  'auto_correct',
  'correct_option_ids',
  'accepted_answers',
  'correct_value',
  'tolerance',
  'explanation',
  'revealed',
];

function keysOf(value: unknown): string[] {
  if (Array.isArray(value)) {
    const entries: unknown[] = value;
    return entries.flatMap(keysOf);
  }
  if (typeof value !== 'object' || value === null) {
    return [];
  }
  return Object.entries(value).flatMap(([key, entry]) => [key, ...keysOf(entry)]);
}

function versionOf(body: unknown): number {
  return typeof body === 'object' && body !== null && 'version' in body && typeof body.version === 'number'
    ? body.version
    : Number.NaN;
}

function fileAnswerCount(body: unknown, questionId: string): number {
  if (typeof body !== 'object' || body === null || !('answers' in body) || !Array.isArray(body.answers)) {
    return Number.NaN;
  }
  const answers: unknown[] = body.answers;
  const found = answers.find(
    (entry) =>
      typeof entry === 'object' && entry !== null && 'question_id' in entry && entry.question_id === questionId,
  );
  return typeof found === 'object' && found !== null && 'files' in found && Array.isArray(found.files)
    ? found.files.length
    : 0;
}

async function studentSubmission(subject: ContractSubject): Promise<{ seeded: SeededSubmission; token: string }> {
  const seeded = await subject.seedSubmission();
  const token = await accessToken(await subject.transport.login(seeded.email, seeded.password));
  return { seeded, token };
}

async function currentVersion(subject: ContractSubject, token: string, seeded: SeededSubmission): Promise<number> {
  const read = await subject.transport.send(submissionRequest(subject, token, seeded.submissionId));
  check(read.status === 200, `expected the student's submission, got ${String(read.status)}`);
  return versionOf(await read.json());
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

  'a tutor without lessons reads an empty calendar week that is not truncated': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword));

    const read = await subject.transport.send(calendarRequest(subject, token));

    check(read.status === 200, `expected 200, got ${String(read.status)}`);
    const body: unknown = await read.json();
    check(
      typeof body === 'object' &&
        body !== null &&
        Object.keys(body).sort().join(',') === 'items,truncated' &&
        'items' in body &&
        Array.isArray(body.items) &&
        body.items.length === 0 &&
        'truncated' in body &&
        body.truncated === false,
      `expected exactly {items: [], truncated: false}, got ${JSON.stringify(body)}`,
    );
  },

  'a tutor reads the calendar lessons of a range ordered by start, whatever order they were scheduled in': async (
    subject,
  ) => {
    const seeded = await subject.seedTwoLessons();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const read = await subject.transport.send(calendarRequest(subject, token, { from: seeded.from, to: seeded.to }));

    check(read.status === 200, `expected 200, got ${String(read.status)}`);
    const starts = startsOf(await read.json());
    check(
      starts.length === 2 &&
        starts[0] === Date.parse(seeded.earlierStart) &&
        starts[1] === Date.parse(seeded.laterStart),
      `expected the two seeded lessons earliest first, got starts ${JSON.stringify(starts)}`,
    );
  },

  'a calendar range that ends at or before its start, or spans more than 64 days, is refused at query.to': async (
    subject,
  ) => {
    const token = await accessToken(await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword));
    const from = Date.parse(emptyWeek.from);

    const reversed = await subject.transport.send(
      calendarRequest(subject, token, { from: emptyWeek.to, to: emptyWeek.from }),
    );
    const empty = await subject.transport.send(
      calendarRequest(subject, token, { from: emptyWeek.from, to: emptyWeek.from }),
    );
    const longest = await subject.transport.send(
      calendarRequest(subject, token, {
        from: emptyWeek.from,
        to: new Date(from + 64 * dayMilliseconds).toISOString(),
      }),
    );
    const overlong = await subject.transport.send(
      calendarRequest(subject, token, {
        from: emptyWeek.from,
        to: new Date(from + 64 * dayMilliseconds + 1_000).toISOString(),
      }),
    );

    check(reversed.status === 422, `expected 422 for a reversed range, got ${String(reversed.status)}`);
    check((await fieldProblems(reversed)) === 'validation_failed query.to:invalid', 'expected query.to invalid');
    check(empty.status === 422, `expected 422 for an empty range, got ${String(empty.status)}`);
    check((await fieldProblems(empty)) === 'validation_failed query.to:invalid', 'expected query.to invalid');
    check(longest.status === 200, `expected 200 for a 64-day range, got ${String(longest.status)}`);
    check(overlong.status === 422, `expected 422 for an overlong range, got ${String(overlong.status)}`);
    check((await fieldProblems(overlong)) === 'validation_failed query.to:too_long', 'expected query.to too_long');
  },

  'a tutor filter on a tutor’s calendar is refused as not allowed': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.otherTutorEmail, subject.otherTutorPassword));

    const read = await subject.transport.send(
      calendarRequest(subject, token, { ...emptyWeek, tutor: '00000000-0000-4000-8000-000000000099' }),
    );

    check(read.status === 422, `expected 422, got ${String(read.status)}`);
    const problems = await fieldProblems(read);
    check(
      problems === 'validation_failed query.tutor:not_allowed',
      `expected query.tutor not_allowed, got ${problems}`,
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
    const unsubscribeToken = await subject.unsubscribeToken();
    const access = await accessToken(await subject.transport.login(subject.email, subject.password));
    const before = await emailGroups(await subject.transport.send(emailPreferencesRequest(subject, access)));

    const preview = await subject.transport.send(unsubscribeRequest(subject, 'GET', unsubscribeToken));
    const afterPreview = await emailGroups(await subject.transport.send(emailPreferencesRequest(subject, access)));
    const applied = await subject.transport.send(unsubscribeRequest(subject, 'POST', unsubscribeToken));
    const afterPost = await emailGroups(await subject.transport.send(emailPreferencesRequest(subject, access)));
    const saved = await emailGroups(
      await subject.transport.send(
        emailPreferencesRequest(subject, access, { groups: [{ group: 'lesson_reminders', enabled: true }] }),
      ),
    );
    const tampered = await subject.transport.send(unsubscribeRequest(subject, 'GET', `${unsubscribeToken}x`));

    const allOn =
      '{"groups":[{"group":"lesson_reminders","enabled":true},{"group":"submissions","enabled":true},{"group":"message_digest","enabled":true}]}';
    check(before === allOn, `expected the tutor's three groups on, got ${before}`);
    check(preview.status === 200, `expected 200, got ${String(preview.status)}`);
    check(JSON.stringify(await preview.json()) === '{"group":"lesson_reminders"}', 'expected exactly the group');
    check(afterPreview === allOn, `expected the preview to change nothing, got ${afterPreview}`);
    check(applied.status === 204, `expected 204, got ${String(applied.status)}`);
    check(
      afterPost.includes('{"group":"lesson_reminders","enabled":false}'),
      `expected lesson reminders off, got ${afterPost}`,
    );
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
  'abort after refresh headers rejects the unread response body and preserves the rotated cookie': async (subject) => {
    await accessToken(await subject.transport.login(subject.email, subject.password));
    const controller = new AbortController();

    const response = await subject.transport.refresh(controller.signal);
    check(response.status === 200, 'expected refresh headers before abort');
    controller.abort();
    let bodyFailed = false;
    try {
      await response.arrayBuffer();
    } catch {
      bodyFailed = true;
    }
    const next = await subject.transport.refresh(timeout());

    check(bodyFailed, 'expected the aborted unread response body to fail');
    check(next.status === 200, 'expected the already rotated cookie to remain usable');
    await accessToken(next);
  },
  'a student reads their own submission without answer keys, points or scores': async (subject) => {
    const { seeded, token } = await studentSubmission(subject);

    const response = await subject.transport.send(submissionRequest(subject, token, seeded.submissionId));

    check(response.status === 200, `expected the student's submission, got ${String(response.status)}`);
    const body: unknown = await response.json();
    const keys = keysOf(body);
    check(keys.includes('editable_question_ids'), 'expected the student schema with editable_question_ids');
    const topLevel = typeof body === 'object' && body !== null ? Object.keys(body).sort() : [];
    check(
      JSON.stringify(topLevel) === JSON.stringify([...studentSubmissionFields].sort()),
      `expected exactly the unrevealed student schema fields, got ${topLevel.join(', ')}`,
    );
    const leaked = answerKeyFields.filter((field) => keys.includes(field));
    check(leaked.length === 0, `expected no answer key, points or scores, found ${leaked.join(', ')}`);
  },
  'the tutor who issued a submission reads it in the tutor schema': async (subject) => {
    const seeded = await subject.seedSubmission();
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    const response = await subject.transport.send(submissionRequest(subject, token, seeded.submissionId));

    check(response.status === 200, `expected the tutor's submission, got ${String(response.status)}`);
    const body: unknown = await response.json();
    const topLevel = typeof body === 'object' && body !== null ? Object.keys(body) : [];
    check(
      ['id', 'answer_policy', 'revisions', 'max_points', 'student'].every((field) => topLevel.includes(field)),
      `expected the tutor schema, got ${topLevel.join(', ')}`,
    );
    check(!topLevel.includes('editable_question_ids'), 'expected no student-only field in the tutor schema');
  },
  "a submission outside the student's work is refused as not_found on read and save": async (subject) => {
    const { token } = await studentSubmission(subject);

    const read = await subject.transport.send(submissionRequest(subject, token, randomSubmissionId));
    const save = await subject.transport.send(answersRequest(subject, token, randomSubmissionId, 1, []));

    check(read.status === 404 && (await errorCode(read)) === 'not_found', 'expected a read out of scope refused');
    check(save.status === 404 && (await errorCode(save)) === 'not_found', 'expected a save out of scope refused');
  },
  'a saved answer advances the version and a save at the old version is refused as conflict': async (subject) => {
    const { seeded, token } = await studentSubmission(subject);
    const version = await currentVersion(subject, token, seeded);
    const answer = [{ question_id: seeded.textQuestionId, value: { text: 'went' } }];

    const saved = await subject.transport.send(answersRequest(subject, token, seeded.submissionId, version, answer));
    const stale = await subject.transport.send(answersRequest(subject, token, seeded.submissionId, version, answer));

    check(saved.status === 200, `expected the answer saved, got ${String(saved.status)}`);
    check(versionOf(await saved.json()) > version, 'expected the saved submission at a later version');
    check(stale.status === 409 && (await errorCode(stale)) === 'conflict', 'expected the stale save refused');
  },
  'answers outside the question limits are refused as validation_failed naming the field': async (subject) => {
    const { seeded, token } = await studentSubmission(subject);
    const version = await currentVersion(subject, token, seeded);
    const tooLong = [{ question_id: seeded.textQuestionId, value: { text: 'a'.repeat(501) } }];
    const unknown = [{ question_id: randomSubmissionId, value: { text: 'went' } }];

    const long = await subject.transport.send(answersRequest(subject, token, seeded.submissionId, version, tooLong));
    const stranger = await subject.transport.send(
      answersRequest(subject, token, seeded.submissionId, version, unknown),
    );

    const longProblems = await fieldProblems(long);
    const strangerProblems = await fieldProblems(stranger);
    check(
      long.status === 422 && longProblems === 'validation_failed body.answers[0].value.text:too_long',
      `expected a too long text refused, got ${String(long.status)} ${longProblems}`,
    );
    check(
      stranger.status === 422 && strangerProblems === 'validation_failed body.answers[0].question_id:invalid',
      `expected an unknown question refused, got ${String(stranger.status)} ${strangerProblems}`,
    );
  },
  "binding an answer file over the tutor's full quota is refused as quota_exceeded and binds nothing": async (
    subject,
  ) => {
    const { seeded, token } = await studentSubmission(subject);
    const uploaded = await subject.transport.send(answerUploadRequest(subject, token, seeded));
    check(uploaded.status === 201, `expected the answer file uploaded, got ${String(uploaded.status)}`);
    const uploadBody: unknown = await uploaded.json();
    const attachmentId =
      typeof uploadBody === 'object' && uploadBody !== null && 'id' in uploadBody && typeof uploadBody.id === 'string'
        ? uploadBody.id
        : '';
    const version = await currentVersion(subject, token, seeded);
    await subject.fillTutorFileQuota();

    const bound = await subject.transport.send(
      answersRequest(subject, token, seeded.submissionId, version, [
        { question_id: seeded.fileQuestionId, value: { attachment_ids: [attachmentId] } },
      ]),
    );

    check(bound.status === 507, `expected the binding refused over the quota, got ${String(bound.status)}`);
    check((await errorCode(bound)) === 'quota_exceeded', 'expected quota_exceeded');
    const read = await subject.transport.send(submissionRequest(subject, token, seeded.submissionId));
    const after: unknown = await read.json();
    check(versionOf(after) === version, 'expected the refused binding to leave the version');
    check(fileAnswerCount(after, seeded.fileQuestionId) === 0, 'expected the refused binding to bind no file');
  },
  'account me and send retain abort ownership after headers and release cancelled readers': async (subject) => {
    const token = await accessToken(await subject.transport.login(subject.email, subject.password));

    for (const method of ['me', 'send'] as const) {
      const controller = new AbortController();
      const response =
        method === 'me'
          ? await subject.transport.me(token, controller.signal)
          : await subject.transport.send(new Request(meRequest(subject, token), { signal: controller.signal }));
      controller.abort();
      const reader = response.body?.getReader();
      check(reader !== undefined, 'expected an account response body');
      let failed = false;
      let completed = false;
      let bytes = 0;
      try {
        while (!failed && !completed) {
          const outcome = await reader?.read().then(
            (part) => ({ kind: 'read' as const, part }),
            () => ({ kind: 'failed' as const }),
          );
          check(outcome !== undefined, 'expected an account body read outcome');
          if (outcome?.kind === 'failed') failed = true;
          else if (outcome?.kind === 'read') {
            completed = outcome.part.done;
            if (!outcome.part.done) bytes += outcome.part.value.byteLength;
            check(bytes <= 64 * 1024, 'account body exceeded the observation limit');
          }
        }
      } finally {
        reader?.releaseLock();
      }
      check(failed, 'expected account body abort to reject reading');
      check(response.body?.locked === false, 'expected aborted account reader release');
      const next =
        method === 'me'
          ? await subject.transport.me(token, timeout())
          : await subject.transport.send(meRequest(subject, token));
      const cancellationReader = next.body?.getReader();
      check(cancellationReader !== undefined, 'expected a cancellable account response body');
      try {
        await cancellationReader?.cancel();
      } finally {
        cancellationReader?.releaseLock();
      }
      check(next.body?.locked === false, 'expected cancelled account reader release');
      const wholeController = new AbortController();
      const whole =
        method === 'me'
          ? await subject.transport.me(token, wholeController.signal)
          : await subject.transport.send(new Request(meRequest(subject, token), { signal: wholeController.signal }));
      wholeController.abort();
      const wholeRead = await whole.arrayBuffer().then(
        () => 'read',
        () => 'failed',
      );
      check(wholeRead === 'failed', 'expected account body abort to reject a whole read');
    }
  },
};
