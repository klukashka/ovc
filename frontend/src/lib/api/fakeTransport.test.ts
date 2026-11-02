import { describe, expect, test } from 'vitest';

import { FakeAuthServer } from './fakeTransport.ts';
import type { components } from './schema.ts';
import { type ContractSubject, type SeededLessons, authTransportContract } from './transportContract.ts';

type CalendarItem = components['schemas']['CalendarItem'];

const origin = 'https://ovc.example';

function fakeSubject(): ContractSubject {
  const server = new FakeAuthServer();
  server.addAccount({ id: 'account-a', name: 'A tutor', email: 'a@example.org', password: 'a long enough password' });
  server.addAccount({ id: 'account-b', name: 'B tutor', email: 'b@example.org', password: 'another long password' });
  server.addAccount({
    id: 'student-a',
    name: 'A student',
    email: 's@example.org',
    password: 'a student password',
    role: 'student',
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
});
