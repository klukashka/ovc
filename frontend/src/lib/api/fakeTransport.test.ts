import { describe, expect, test } from 'vitest';

import { pastSimpleQuestions, pastSimpleReview, pastSimpleSubmission } from './fakeSubmissions.ts';
import {
  AccountResponseBodies,
  FakeAuthServer,
  type SeededFakeGroup,
  SignalAwareRefreshBodies,
} from './fakeTransport.ts';
import type { components } from './schema.ts';
import {
  type ContractSubject,
  type SeededDeadline,
  type SeededGroup,
  type SeededLessons,
  type SeededLibrary,
  type SeededSharedStudent,
  type SeededStudentHome,
  type SeededSubmission,
  authTransportContract,
} from './transportContract.ts';

type CalendarItem = components['schemas']['CalendarItem'];
type IssueItem = components['schemas']['IssueItem'];
type MyTutor = components['schemas']['MyTutor'];
type StudentAssignmentItem = components['schemas']['StudentAssignmentItem'];
type StudentSubmission = components['schemas']['StudentSubmission'];

const origin = 'https://ovc.example';
const pastSimpleRelationship = '5b4a3c2d-1e0f-4a9b-8c7d-6e5f4a3b2c1d';
const homeRelationship = '1c2d3e4f-5a6b-4c7d-8e9f-0a1b2c3d4e5f';
const endedRelationship = '2d3e4f5a-6b7c-4d8e-9f0a-1b2c3d4e5f6a';
const otherStudentRelationship = '3e4f5a6b-7c8d-4e9f-8a1b-2c3d4e5f6a7b';
const dayMilliseconds = 24 * 60 * 60 * 1_000;

function relatedSubmission(id: string, relationshipId: string): StudentSubmission {
  const submission = pastSimpleSubmission(id);
  return { ...submission, tutor: { ...submission.tutor, relationship_id: relationshipId } };
}

function seededSubmission(submissionId: string): SeededSubmission {
  return {
    email: 's@example.org',
    password: 'a student password',
    submissionId,
    textQuestionId: pastSimpleQuestions.text,
    fileQuestionId: pastSimpleQuestions.file,
    relationshipId: pastSimpleRelationship,
  };
}

function currentTutor(relationshipId: string, state: MyTutor['state']): MyTutor {
  return {
    relationship_id: relationshipId,
    state,
    colour: 'moss',
    channel_id: null,
    account_closed_until: null,
    ended_at: state === 'ended' ? '2026-10-30T10:00:00Z' : null,
    ended_by: state === 'ended' ? 'tutor' : null,
    tutor: { name: 'A tutor', avatar_id: null, bio: '', subjects: ['English'] },
  };
}

function assignmentItem(
  submissionId: string,
  status: StudentAssignmentItem['status'],
  relationshipId: string,
): StudentAssignmentItem {
  const checked = status === 'checked';
  return {
    submission_id: submissionId,
    title: 'Past simple',
    tutor: { relationship_id: relationshipId, name: 'A tutor', colour: 'moss' },
    status,
    late: false,
    issued_before_reclaim: false,
    deadline_at: null,
    issued_at: '2026-10-30T10:00:00Z',
    submitted_at: checked ? '2026-10-31T10:00:00Z' : null,
    checked_at: checked ? '2026-10-31T12:00:00Z' : null,
    feedback_excerpt: checked ? 'Well done.' : null,
    has_corrected_files: false,
    feedback_new: checked,
  };
}

function issueDue(id: string, deadline: number): IssueItem {
  return {
    id,
    title: 'Past simple',
    audience: { relationship_id: homeRelationship, display_name: 'A student' },
    lesson_id: null,
    deadline_at: new Date(deadline).toISOString(),
    answer_policy: 'after_check',
    status: 'active',
    issued_at: '2026-10-30T10:00:00Z',
    withdrawn_at: null,
    has_manual_questions: true,
    max_points: 10,
    version: 1,
    progress: { students: 1, submitted: 0, checked: 0, returned: 0, late: 0, needs_checking: 0 },
  };
}

function studentLesson(startsAt: number): CalendarItem {
  return {
    id: '4f5a6b7c-8d9e-4f0a-9b1c-2d3e4f5a6b7c',
    title: 'Lesson',
    starts_at: new Date(startsAt).toISOString(),
    ends_at: new Date(startsAt + 60 * 60 * 1_000).toISOString(),
    status: 'scheduled',
    took_place: false,
    moved: false,
    timezone: 'Europe/Moscow',
    audience: { kind: 'one_to_one', label: 'A tutor', label_kind: null, colour: 'moss' },
    tutor: { name: 'A tutor', colour: 'moss' },
  };
}

function seededGroup(seeded: SeededFakeGroup): SeededGroup {
  const [relationshipId] = seeded.relationshipIds;
  if (relationshipId === undefined) {
    throw new Error('the fake server seeded a group without a member');
  }
  return { groupId: seeded.groupId, relationshipId };
}

async function createdCardId(server: FakeAuthServer, email: string, password: string): Promise<string> {
  const login: unknown = await (await server.transport().login(email, password)).json();
  const token =
    typeof login === 'object' && login !== null && 'access_token' in login && typeof login.access_token === 'string'
      ? login.access_token
      : '';
  const response = await server.transport().send(
    new Request(`${origin}/api/v1/students`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ display_name: 'Shared student' }),
    }),
  );
  const created: unknown = await response.json();
  if (typeof created !== 'object' || created === null || !('id' in created) || typeof created.id !== 'string') {
    throw new Error('the fake server created no card');
  }
  return created.id;
}

function fakeSubject(): ContractSubject {
  const server = new FakeAuthServer({
    refreshBodies: new SignalAwareRefreshBodies(),
    accountBodies: new AccountResponseBodies(),
  });
  server.addAccount({ id: 'account-a', name: 'A tutor', email: 'a@example.org', password: 'a long enough password' });
  server.addAccount({ id: 'account-b', name: 'B tutor', email: 'b@example.org', password: 'another long password' });
  server.addAccount({
    id: 'student-a',
    name: 'A student',
    email: 's@example.org',
    password: 'a student password',
    role: 'student',
  });
  server.addAccount({
    id: 'student-b',
    name: 'B student',
    email: 's2@example.org',
    password: 'another student password',
    role: 'student',
  });
  server.addAccount({
    id: 'tutor-u',
    name: 'Unconfirmed tutor',
    email: 'u@example.org',
    password: 'an unconfirmed password',
    state: 'unverified',
  });
  return {
    transport: server.transport(),
    origin,
    email: 'a@example.org',
    password: 'a long enough password',
    otherTutorEmail: 'b@example.org',
    otherTutorPassword: 'another long password',
    student: async (): Promise<{ readonly email: string; readonly password: string }> => {
      await Promise.resolve();
      return { email: 's@example.org', password: 'a student password' };
    },
    expireAccessTokens: async (): Promise<void> => {
      server.advance(16 * 60);
      await Promise.resolve();
    },
    passSeconds: async (seconds: number): Promise<void> => {
      server.advance(seconds);
      await Promise.resolve();
    },
    blockAccount: async (): Promise<void> => {
      server.block('a@example.org');
      await Promise.resolve();
    },
    unsubscribeToken: async (): Promise<string> => {
      await Promise.resolve();
      return server.unsubscribeToken('a@example.org', 'lesson_reminders');
    },
    seedTwoLessons: async (): Promise<SeededLessons> => {
      await Promise.resolve();
      const lesson = (startsAt: string, endsAt: string): CalendarItem => ({
        id: `00000000-0000-4000-8000-${String(Date.parse(startsAt)).slice(-12)}`,
        title: 'Lesson',
        starts_at: startsAt,
        ends_at: endsAt,
        status: 'scheduled',
        took_place: false,
        moved: false,
        timezone: 'Europe/Moscow',
        audience: { kind: 'one_to_one', label: 'A student', label_kind: null, colour: 'moss' },
        tutor: null,
      });
      server.addCalendarItem('a@example.org', lesson('2026-11-03T09:00:00Z', '2026-11-03T10:00:00Z'));
      server.addCalendarItem('a@example.org', lesson('2026-11-03T06:00:00Z', '2026-11-03T07:00:00Z'));
      return {
        from: '2026-11-03T05:00:00Z',
        to: '2026-11-03T12:00:00Z',
        laterStart: '2026-11-03T09:00:00Z',
        earlierStart: '2026-11-03T06:00:00Z',
      };
    },
    seedSubmission: async (): Promise<SeededSubmission> => {
      await Promise.resolve();
      const submissionId = '4c3b2a19-0f8e-4d7c-9b6a-5f4e3d2c1b0a';
      server.addSubmission('s@example.org', 'a@example.org', pastSimpleSubmission(submissionId));
      server.addReviewedSubmission('a@example.org', pastSimpleReview(submissionId));
      return seededSubmission(submissionId);
    },
    fillTutorFileQuota: async (): Promise<void> => {
      await Promise.resolve();
      server.fillFileQuota('a@example.org');
    },
    seedUpcomingDeadline: async (): Promise<SeededDeadline> => {
      await Promise.resolve();
      const issueId = '6a7b8c9d-0e1f-4a2b-8c3d-4e5f6a7b8c9d';
      server.addIssue('a@example.org', issueDue(issueId, server.serverNow() + 2 * dayMilliseconds));
      return { issueId };
    },
    seedStudentHome: async (): Promise<SeededStudentHome> => {
      await Promise.resolve();
      server.seedTutors('s@example.org', [
        currentTutor(homeRelationship, 'active'),
        currentTutor(endedRelationship, 'ended'),
      ]);
      server.seedTutors('s2@example.org', [currentTutor(otherStudentRelationship, 'active')]);
      server.addAssignment(
        's@example.org',
        assignmentItem('7b8c9d0e-1f2a-4b3c-9d4e-5f6a7b8c9d0e', 'not_started', homeRelationship),
      );
      server.addAssignment(
        's@example.org',
        assignmentItem('8c9d0e1f-2a3b-4c4d-8e5f-6a7b8c9d0e1f', 'checked', homeRelationship),
      );
      server.addCalendarItem('s@example.org', studentLesson(server.serverNow() + dayMilliseconds), homeRelationship);
      return {
        email: 's@example.org',
        password: 'a student password',
        relationshipId: homeRelationship,
        endedRelationshipId: endedRelationship,
        otherStudentRelationshipId: otherStudentRelationship,
      };
    },
    unconfirmedTutor: async (): Promise<{ readonly email: string; readonly password: string }> => {
      await Promise.resolve();
      return { email: 'u@example.org', password: 'an unconfirmed password' };
    },
    seedSharedStudentIssues: async (): Promise<SeededSharedStudent> => {
      const cardId = await createdCardId(server, 'a@example.org', 'a long enough password');
      const otherTutorCardId = await createdCardId(server, 'b@example.org', 'another long password');
      server.addSubmission(
        's@example.org',
        'a@example.org',
        relatedSubmission('9d0e1f2a-3b4c-4d5e-9f6a-7b8c9d0e1f2a', cardId),
      );
      server.addSubmission(
        's@example.org',
        'b@example.org',
        relatedSubmission('0e1f2a3b-4c5d-4e6f-8a7b-8c9d0e1f2a3b', otherTutorCardId),
      );
      server.addSubmission(
        's@example.org',
        'b@example.org',
        relatedSubmission('1f2a3b4c-5d6e-4f7a-9b8c-9d0e1f2a3b4c', otherTutorCardId),
      );
      return { cardId, otherTutorCardId };
    },
    seedCheckedSubmission: async (): Promise<SeededSubmission> => {
      await Promise.resolve();
      const submissionId = '5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a';
      server.addSubmission('s@example.org', 'a@example.org', {
        ...pastSimpleSubmission(submissionId),
        status: 'checked',
        version: 4,
        editable_question_ids: [],
        review: { feedback: 'Well done.', items: [] },
      });
      server.addAssignment('s@example.org', assignmentItem(submissionId, 'checked', pastSimpleRelationship));
      return seededSubmission(submissionId);
    },
    seedOtherStudentSubmission: async (): Promise<SeededSubmission> => {
      await Promise.resolve();
      const submissionId = '7a6b5c4d-3e2f-4a1b-9c8d-7e6f5a4b3c2d';
      server.addSubmission('s2@example.org', 'a@example.org', pastSimpleSubmission(submissionId));
      return { ...seededSubmission(submissionId), email: 's2@example.org', password: 'another student password' };
    },
    returnSeededSubmission: async (seeded: SeededSubmission): Promise<void> => {
      await Promise.resolve();
      server.returnSubmission(seeded.submissionId);
    },
    endSeededRelationship: async (seeded: SeededSubmission): Promise<void> => {
      await Promise.resolve();
      server.endRelationshipOf(seeded.submissionId);
    },
    readyExport: async (token: string): Promise<void> => {
      await Promise.resolve();
      server.finishExportOf(token);
    },
    seedGroupWithMember: async (): Promise<SeededGroup> => {
      await Promise.resolve();
      return seededGroup(server.seedGroup('a@example.org', { name: 'Contract group', members: ['Group student'] }));
    },
    endGroupMemberRelationship: async (seeded: SeededGroup): Promise<void> => {
      await Promise.resolve();
      server.endStudentRelationship(seeded.relationshipId);
    },
    seedGroupWithFutureLesson: async (): Promise<SeededGroup> => {
      await Promise.resolve();
      return seededGroup(
        server.seedGroup('a@example.org', { name: 'Contract group', members: ['Group student'], futureLessonCount: 1 }),
      );
    },
    seedLibraryFolderAndTemplate: async (): Promise<SeededLibrary> => {
      await Promise.resolve();
      const { folderId, childFolderId, templateId, templateVersion } = server.seedLibrary('a@example.org');
      return { folderId, childFolderId, templateId, templateVersion };
    },
  };
}

describe('the fake authentication transport', () => {
  test.each(Object.entries(authTransportContract))('keeps the contract: %s', async (_name, scenario) => {
    const subject = fakeSubject();

    const run = scenario(subject);

    await expect(run).resolves.toBeUndefined();
  });
});

const chatId = '00000000-0000-4000-8000-000000000101';
const messageId = '00000000-0000-4000-8000-000000000102';

function chatFixture(): FakeAuthServer {
  const server = new FakeAuthServer();
  server.addAccount({
    id: 'tutor',
    name: 'Tutor',
    email: 'tutor@example.org',
    password: 'password',
    state: 'unverified',
  });
  server.addAccount({
    id: 'student',
    name: 'Student',
    email: 'student@example.org',
    password: 'password',
    role: 'student',
    state: 'unverified',
  });
  server.addAccount({ id: 'outsider', name: 'Other tutor', email: 'other@example.org', password: 'password' });
  server.chat.addChannel({
    channel: {
      id: chatId,
      kind: 'direct',
      label: 'Student card',
      label_kind: null,
      colour: 'moss',
      tutor: null,
      last_message: null,
      unread_count: 0,
      pinned: null,
      peer_read_seq: null,
      writable: true,
      online: false,
      unconfirmed: true,
      user_id: 'student',
      relationship_id: 'relationship',
    },
    members: [
      { accountId: 'tutor', label: 'Tutor' },
      { accountId: 'student', label: 'Student card' },
    ],
  });
  server.chat.addMessage(chatId, {
    id: messageId,
    body: 'Earlier message',
    attachments: [],
    created_at: '2026-11-01T00:00:00Z',
    edited_at: null,
    deleted: false,
    erased: false,
    own: false,
    sender_label: 'Student card',
    sender_label_kind: null,
    sent_before_reclaim: true,
    reply_to: null,
    reactions: [],
    seq: 1,
    version: 1,
    senderId: 'student',
  });
  return server;
}

async function chatToken(server: FakeAuthServer, email: string): Promise<string> {
  const response = await server.transport().login(email, 'password');
  const body: unknown = await response.json();
  if (typeof body !== 'object' || body === null || !('access_token' in body) || typeof body.access_token !== 'string') {
    throw new Error('fake login did not issue a token');
  }
  return body.access_token;
}

function chatRequest(path: string, token: string, method = 'GET', body?: unknown, key?: string): Request {
  return new Request(`${origin}/api/v1/${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
      ...(key === undefined ? {} : { 'Idempotency-Key': key }),
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
}

describe('fake chat audience and state', () => {
  test('a transient channel-list refusal has the API error contract and the next read succeeds', async () => {
    const server = chatFixture();
    const token = await chatToken(server, 'tutor@example.org');
    server.chat.failNextChannelListRead();

    const refused = await server.transport().send(chatRequest('channels', token));
    const recovered = await server.transport().send(chatRequest('channels', token));

    expect(refused.status).toBe(503);
    expect(await refused.json()).toEqual({
      error: { code: 'service_unavailable', message: 'service_unavailable', request_id: 'fake-request' },
    });
    expect(refused.headers.get('Cache-Control')).toBe('no-store');
    expect(recovered.status).toBe(200);
    expect(await recovered.json()).toMatchObject({ items: [{ id: chatId }], next_cursor: null });
  });
  test('an unverified tutor is refused on verified routes but can read their chat', async () => {
    const server = chatFixture();
    const token = await chatToken(server, 'tutor@example.org');

    const cards = await server.transport().send(chatRequest('students', token));
    const chat = await server.transport().send(chatRequest(`channels/${chatId}`, token));

    expect(cards.status).toBe(403);
    expect(await cards.json()).toMatchObject({ error: { code: 'email_not_verified' } });
    expect(chat.status).toBe(200);
    expect(await chat.json()).toMatchObject({ unconfirmed: true, unread_count: 1 });
  });

  test('role refusals precede confirmation and outsiders cannot read the channel', async () => {
    const server = chatFixture();
    const student = await chatToken(server, 'student@example.org');
    const outsider = await chatToken(server, 'other@example.org');

    const cards = await server.transport().send(chatRequest('students', student));
    const outside = await server.transport().send(chatRequest(`channels/${chatId}/messages`, outsider));
    const own = await server.transport().send(chatRequest(`channels/${chatId}`, student));

    expect(cards.status).toBe(404);
    expect(outside.status).toBe(404);
    expect(await own.json()).not.toHaveProperty('relationship_id');
  });

  test('an idempotent send creates one message and marks only its sender read', async () => {
    const server = chatFixture();
    const token = await chatToken(server, 'tutor@example.org');
    const request = (): Request =>
      chatRequest(`channels/${chatId}/messages`, token, 'POST', { body: 'Hello', attachment_ids: [] }, 'send-one');

    const first = await server.transport().send(request());
    const retry = await server.transport().send(request());

    expect(first.status).toBe(201);
    expect(await retry.json()).toEqual(await first.json());
    expect(server.chat.history(chatId)).toHaveLength(2);
    expect(server.chat.readSeq(chatId, 'tutor')).toBe(2);
    expect(server.chat.readSeq(chatId, 'student')).toBe(0);
  });

  test('readonly preserves own and tutor deletes but refuses edits', async () => {
    const server = chatFixture();
    server.chat.setWritable(chatId, false);
    const student = await chatToken(server, 'student@example.org');
    const tutor = await chatToken(server, 'tutor@example.org');

    const edit = await server
      .transport()
      .send(chatRequest(`messages/${messageId}`, student, 'PATCH', { body: 'Edited', version: 1 }));
    const ownDelete = await server.transport().send(chatRequest(`messages/${messageId}`, student, 'DELETE'));
    const tutorDelete = await server.transport().send(chatRequest(`messages/${messageId}`, tutor, 'DELETE'));

    expect(edit.status).toBe(409);
    expect(await edit.json()).toMatchObject({ error: { code: 'channel_read_only' } });
    expect(ownDelete.status).toBe(204);
    expect(tutorDelete.status).toBe(204);
    expect(server.chat.history(chatId)[0]).toMatchObject({ deleted: true, body: null });
  });

  test('stale versions conflict and a suspended member loses history and writes', async () => {
    const server = chatFixture();
    const token = await chatToken(server, 'student@example.org');

    const stale = await server
      .transport()
      .send(chatRequest(`messages/${messageId}`, token, 'PATCH', { body: 'Changed', version: 2 }));
    server.chat.suspendMember(chatId, 'student', true);
    const history = await server.transport().send(chatRequest(`channels/${chatId}/messages`, token));
    const removal = await server.transport().send(chatRequest(`messages/${messageId}`, token, 'DELETE'));

    expect(stale.status).toBe(409);
    expect(await stale.json()).toMatchObject({ error: { code: 'conflict' } });
    expect(history.status).toBe(404);
    expect(removal.status).toBe(404);
  });

  test('search matches live bodies of a member chat literally, refuses short terms and limits each caller', async () => {
    const server = chatFixture();
    const token = await chatToken(server, 'tutor@example.org');
    const search = (q: string): Promise<Response> =>
      server.transport().send(chatRequest(`channels/${chatId}/messages/search`, token, 'POST', { q }));
    const outsider = await chatToken(server, 'other@example.org');

    const found: unknown = await (await search('EARLIER')).json();
    const literal: unknown = await (await search('ear%')).json();
    const short = await search(' e ');
    const outside = await server
      .transport()
      .send(chatRequest(`channels/${chatId}/messages/search`, outsider, 'POST', { q: 'earlier' }));
    const statuses: number[] = [];
    for (let attempt = 0; attempt < 30; attempt += 1) statuses.push((await search('earlier')).status);

    expect(found).toMatchObject({ items: [{ body: 'Earlier message' }], next_cursor: null });
    expect(literal).toMatchObject({ items: [], next_cursor: null });
    expect(short.status).toBe(422);
    expect(outside.status).toBe(404);
    expect(statuses.filter((status) => status === 429)).toHaveLength(2);
  });

  test('reactions name the reactors only to the tutor and follow the read-only rule', async () => {
    const server = chatFixture();
    const student = await chatToken(server, 'student@example.org');
    const tutor = await chatToken(server, 'tutor@example.org');
    const react = (token: string, reaction: string): Promise<Response> =>
      server.transport().send(chatRequest(`messages/${messageId}/reaction`, token, 'PUT', { reaction }));

    const set = await react(student, 'heart');
    const unknown = await react(student, 'rocket');
    const forTutor: unknown = await (
      await server.transport().send(chatRequest(`channels/${chatId}/messages`, tutor))
    ).json();
    const forStudent: unknown = await (
      await server.transport().send(chatRequest(`channels/${chatId}/messages`, student))
    ).json();
    server.chat.setWritable(chatId, false);
    const readOnly = await react(student, 'joy');
    const removed = await server.transport().send(chatRequest(`messages/${messageId}/reaction`, student, 'DELETE'));

    expect(set.status).toBe(204);
    expect(unknown.status).toBe(422);
    expect(forTutor).toMatchObject({
      items: [{ reactions: [{ reaction: 'heart', count: 1, mine: false, members: [{ label: 'Student card' }] }] }],
    });
    expect(forStudent).toMatchObject({ items: [{ reactions: [{ reaction: 'heart', count: 1, mine: true }] }] });
    expect(JSON.stringify(forStudent)).not.toContain('members');
    expect(readOnly.status).toBe(409);
    expect(removed.status).toBe(204);
  });

  test('read markers clamp and never move backwards', async () => {
    const server = chatFixture();
    const token = await chatToken(server, 'tutor@example.org');

    const high = await server.transport().send(chatRequest(`channels/${chatId}/read`, token, 'POST', { seq: 99 }));
    const old = await server.transport().send(chatRequest(`channels/${chatId}/read`, token, 'POST', { seq: 0 }));
    const unread = await server.transport().send(chatRequest('channels/unread', token));

    expect(high.status).toBe(204);
    expect(old.status).toBe(204);
    expect(server.chat.readSeq(chatId, 'tutor')).toBe(1);
    expect(await unread.json()).toEqual({ unread_channels: 0 });
  });

  test('a direct read reaches the other member only while the chat is writable, and stays frozen until reported again', async () => {
    const server = chatFixture();
    const student = await chatToken(server, 'student@example.org');
    const tutor = await chatToken(server, 'tutor@example.org');
    const read = (): Promise<Response> =>
      server.transport().send(chatRequest(`channels/${chatId}/read`, student, 'POST', { seq: 1 }));
    server.chat.setWritable(chatId, false);

    await read();
    const frozen: unknown = await (await server.transport().send(chatRequest(`channels/${chatId}`, tutor))).json();
    server.chat.setWritable(chatId, true);
    const restored: unknown = await (await server.transport().send(chatRequest(`channels/${chatId}`, tutor))).json();
    await read();
    const reported: unknown = await (await server.transport().send(chatRequest(`channels/${chatId}`, tutor))).json();
    const page: unknown = await (
      await server.transport().send(chatRequest(`channels/${chatId}/messages`, tutor))
    ).json();
    const forStudent: unknown = await (
      await server.transport().send(chatRequest(`channels/${chatId}`, student))
    ).json();

    expect(frozen).toMatchObject({ peer_read_seq: 0 });
    expect(restored).toMatchObject({ peer_read_seq: 0 });
    expect(reported).toMatchObject({ peer_read_seq: 1 });
    expect(page).toMatchObject({ peer_read_seq: 1 });
    expect(forStudent).toMatchObject({ peer_read_seq: 0 });
  });
});

test('a held fake history response retains the authorized REST snapshot and no-store contract', async () => {
  const server = chatFixture();
  const token = await chatToken(server, 'tutor@example.org');
  const pause = server.chat.pauseNextHistoryResponse();
  const reading = server.transport().send(chatRequest(`channels/${chatId}/messages?after_seq=0&limit=1`, token));
  await pause.answered;

  const current = await server.transport().send(chatRequest(`channels/${chatId}/messages?after_seq=0&limit=1`, token));
  pause.resume();
  const held = await reading;
  const heldBody: unknown = await held.json();
  const currentBody: unknown = await current.json();

  expect(held.status).toBe(200);
  expect(held.headers.get('Cache-Control')).toBe('no-store');
  expect(heldBody).toEqual(currentBody);
});

test('a held fake history refusal keeps its real no-store response after membership is restored', async () => {
  const server = chatFixture();
  const token = await chatToken(server, 'tutor@example.org');
  server.chat.suspendMember(chatId, 'tutor', true);
  const pause = server.chat.pauseNextHistoryResponse();
  const reading = server.transport().send(chatRequest(`channels/${chatId}/messages?after_seq=0&limit=1`, token));
  await pause.answered;

  server.chat.suspendMember(chatId, 'tutor', false);
  const current = await server.transport().send(chatRequest(`channels/${chatId}/messages?after_seq=0&limit=1`, token));
  pause.resume();
  const held = await reading;
  const body: unknown = await held.json();

  expect(current.status).toBe(200);
  expect(held.status).toBe(404);
  expect(held.headers.get('Cache-Control')).toBe('no-store');
  expect(body).toMatchObject({ error: { code: 'not_found' } });
});

describe('the passive initial-read transport observation', () => {
  test.each([false, true])('preserves routing and authentication responses when armed=%s', async (armed) => {
    const server = chatFixture();
    const token = await chatToken(server, 'tutor@example.org');
    const observed = armed ? server.chat.observeInitialReads(origin, chatId) : null;

    const own = await server.transport().send(chatRequest(`channels/${chatId}`, token));
    const list = await server.transport().send(chatRequest('channels?q=', token));
    const unread = await server.transport().send(chatRequest('channels/unread', token));
    const denied = await server.transport().send(chatRequest(`channels/${chatId}`, 'invalid-token'));
    const ownBody: unknown = await own.json();
    const listBody: unknown = await list.json();
    const unreadBody: unknown = await unread.json();
    const deniedBody: unknown = await denied.json();
    const snapshot = observed?.snapshot();
    observed?.stop();

    expect(own.status).toBe(200);
    expect(ownBody).toMatchObject({ id: chatId, unconfirmed: true });
    expect(list.status).toBe(200);
    expect(listBody).toMatchObject({ items: [{ id: chatId }], next_cursor: null });
    expect(unread.status).toBe(200);
    expect(unreadBody).toEqual({ unread_channels: 1 });
    expect(denied.status).toBe(401);
    expect(deniedBody).toMatchObject({ error: { code: 'unauthenticated' } });
    expect(own.headers.get('Cache-Control')).toBe('no-store');
    if (armed) expect(snapshot?.map((entry) => entry.totalAttempts)).toEqual([1, 1, 2]);
    else expect(snapshot).toBeUndefined();
  });

  test('observes producer abort before the second send synchronously without changing nextTurn or responses', async () => {
    const server = chatFixture();
    const token = await chatToken(server, 'tutor@example.org');
    const observed = server.chat.observeInitialReads(origin, chatId);
    const first = new AbortController();
    const transport = server.transport();

    const firstResponse = transport.send(
      new Request(chatRequest(`channels/${chatId}`, token), { signal: first.signal }),
    );
    const atFirstEntry = observed.snapshot().find((entry) => entry.target === 'current_channel');
    first.abort();
    const secondResponse = transport.send(chatRequest(`channels/${chatId}`, token));
    const atSecondEntry = observed.snapshot().find((entry) => entry.target === 'current_channel');
    const responses = await Promise.all([firstResponse, secondResponse]);
    observed.stop();

    expect(atFirstEntry).toMatchObject({ totalAttempts: 1, firstAborted: false, retainedLifetimes: 1 });
    expect(atSecondEntry).toMatchObject({
      totalAttempts: 2,
      firstAborted: true,
      firstAbortedAtSecondInvocation: true,
      retainedLifetimes: 2,
    });
    expect(responses.map((response) => response.status)).toEqual([200, 200]);
  });

  test('retains only two lifetimes and ignores wrong query origin and method without altering the transport', async () => {
    const server = chatFixture();
    const token = await chatToken(server, 'tutor@example.org');
    const observed = server.chat.observeInitialReads(origin, chatId);

    for (let index = 0; index < 5; index += 1) await server.transport().send(chatRequest(`channels/${chatId}`, token));
    await server.transport().send(chatRequest('channels?q=other', token));
    await server.transport().send(
      new Request(`https://other.example/api/v1/channels/${chatId}`, {
        headers: { Authorization: `Bearer ${token}` },
      }),
    );
    const wrongMethod = await server.transport().send(chatRequest('channels/unread', token, 'POST', {}));
    const snapshot = observed.snapshot();
    observed.stop();
    await server.transport().send(chatRequest(`channels/${chatId}`, token));
    const afterStop = observed.snapshot();

    expect(snapshot.find((entry) => entry.target === 'current_channel')).toMatchObject({
      totalAttempts: 5,
      retainedLifetimes: 2,
    });
    expect(snapshot.find((entry) => entry.target === 'list')?.totalAttempts).toBe(0);
    expect(afterStop).toEqual(snapshot);
    expect(wrongMethod.status).toBe(403);
    expect(await wrongMethod.json()).toMatchObject({ error: { code: 'email_not_verified' } });
    expect(JSON.stringify(snapshot)).not.toContain('Authorization');
    expect(JSON.stringify(snapshot)).not.toContain(origin);
    expect(JSON.stringify(snapshot)).not.toContain(token);
  });
});

test('armed and unarmed initial observation preserve identical complete transport responses', async () => {
  const baseline = chatFixture();
  const armed = chatFixture();
  const baselineToken = await chatToken(baseline, 'tutor@example.org');
  const armedToken = await chatToken(armed, 'tutor@example.org');
  const observation = armed.chat.observeInitialReads(origin, chatId);
  const paths = ['channels/unread', 'channels?q=', `channels/${chatId}`];

  const baselineResults: {
    readonly status: number;
    readonly headers: readonly (readonly [string, string])[];
    readonly body: unknown;
  }[] = [];
  const armedResults: typeof baselineResults = [];
  for (const path of paths) {
    const first = await baseline.transport().send(chatRequest(path, baselineToken));
    const second = await armed.transport().send(chatRequest(path, armedToken));
    baselineResults.push({ status: first.status, headers: [...first.headers], body: (await first.json()) as unknown });
    armedResults.push({ status: second.status, headers: [...second.headers], body: (await second.json()) as unknown });
  }
  observation.stop();

  expect(armedResults).toEqual(baselineResults);
});

test('destination observation preserves real responses and synchronously retains only two lifetimes', async () => {
  const baseline = chatFixture();
  const armed = chatFixture();
  const firstToken = await chatToken(baseline, 'tutor@example.org');
  const token = await chatToken(armed, 'tutor@example.org');
  const observed = armed.chat.observeDestinationDetail(origin, chatId);
  const controller = new AbortController();

  const first = armed.transport().send(
    new Request(`${origin}/api/v1/channels/${chatId}`, {
      headers: { Authorization: `Bearer ${token}` },
      signal: controller.signal,
    }),
  );
  controller.abort();
  const second = armed.transport().send(chatRequest(`channels/${chatId}`, token));
  const entry = observed.snapshot()[0];
  const responses = await Promise.all([first, second]);
  const baselineResponse = await baseline.transport().send(chatRequest(`channels/${chatId}`, firstToken));
  const actual = responses[1];
  const expectedBody: unknown = await baselineResponse.json();
  const actualBody: unknown = await actual.json();
  await armed.transport().send(chatRequest(`channels/${chatId}`, token));
  await armed.transport().send(chatRequest(`channels/${chatId}?q=wrong`, token));
  const bounded = observed.snapshot()[0];
  observed.stop();
  await armed.transport().send(chatRequest(`channels/${chatId}`, token));

  expect(entry).toMatchObject({ totalAttempts: 2, firstAbortedAtSecondInvocation: true, retainedLifetimes: 2 });
  expect(actual.status).toBe(baselineResponse.status);
  expect([...actual.headers]).toEqual([...baselineResponse.headers]);
  expect(actualBody).toEqual(expectedBody);
  expect(bounded).toMatchObject({ totalAttempts: 3, retainedLifetimes: 2 });
  expect(observed.snapshot()[0]).toEqual(bounded);
  expect(() => armed.chat.observeDestinationDetail(origin, chatId)).toThrow('destination_observation_already_used');
});
