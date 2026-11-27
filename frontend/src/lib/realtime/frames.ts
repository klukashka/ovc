export const entityKinds = [
  'relationship',
  'join_request',
  'notification',
  'lesson',
  'channel',
  'channel_pin',
  'issue',
  'submission',
] as const;

export type EntityKind = (typeof entityKinds)[number];

export type ServerFrame =
  | { readonly type: 'ready' }
  | { readonly type: 'notification.new'; readonly id: string }
  | {
      readonly type: 'entity.changed';
      readonly kind: EntityKind;
      readonly id: string;
      readonly issueId?: string;
      readonly lessonIds?: readonly string[];
    }
  | { readonly type: 'message.new'; readonly channelId: string; readonly messageId: string; readonly seq: number }
  | { readonly type: 'message.updated' | 'message.deleted'; readonly channelId: string; readonly messageId: string }
  | { readonly type: 'read.updated'; readonly channelId: string }
  | { readonly type: 'typing'; readonly channelId: string; readonly userId: string | null }
  | { readonly type: 'presence'; readonly userId: string; readonly online: boolean }
  | { readonly type: 'pong'; readonly ref: string | null }
  | {
      readonly type: 'error';
      readonly code: string;
      readonly message: string;
      readonly requestId: string;
      readonly ref: string | null;
    };

export const closeCodes = {
  client: 1000,
  shutdown: 1001,
  binaryFrame: 1003,
  frameTooLarge: 1009,
  reconnect: 1012,
  tryLater: 1013,
  unauthenticated: 4401,
  tooMany: 4429,
} as const;

export type CloseCode = (typeof closeCodes)[keyof typeof closeCodes];

export const refPattern = /^[A-Za-z0-9._-]{1,64}$/;

type Fields = Readonly<Record<string, unknown>>;

function isObject(value: unknown): value is Fields {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function hasOnly(value: Fields, allowed: readonly string[]): boolean {
  return Object.keys(value).every((key) => allowed.includes(key));
}

function isEntityKind(value: unknown): value is EntityKind {
  return entityKinds.some((kind) => kind === value);
}

function textOf(value: Fields, key: string): string | null {
  const field = value[key];
  return typeof field === 'string' && field !== '' ? field : null;
}

function refOf(value: Fields, key: string, required: boolean): string | null | undefined {
  if (!(key in value)) {
    return required ? undefined : null;
  }
  const field = value[key];
  if (field === null && required) {
    return null;
  }
  return typeof field === 'string' && refPattern.test(field) ? field : undefined;
}

function payloadOf(frame: Fields, allowed: readonly string[]): Fields | null {
  const payload = frame.payload;
  return isObject(payload) && hasOnly(payload, allowed) ? payload : null;
}

function idFrame(frame: Fields): string | null {
  const payload = payloadOf(frame, ['id']);
  return payload === null || !hasOnly(frame, ['type', 'payload']) ? null : textOf(payload, 'id');
}

const maxLessonIds = 2;

function lessonIdsOf(value: unknown): readonly string[] | null {
  if (!Array.isArray(value) || value.length === 0 || value.length > maxLessonIds) {
    return null;
  }
  const lessons: string[] = [];
  for (const item of value) {
    if (typeof item !== 'string' || item === '' || lessons.includes(item)) {
      return null;
    }
    lessons.push(item);
  }
  return lessons;
}

function entityFrame(frame: Fields): ServerFrame | null {
  const payload = payloadOf(frame, ['kind', 'id', 'issue_id', 'lesson_ids']);
  if (payload === null || !hasOnly(frame, ['type', 'payload'])) {
    return null;
  }
  const id = textOf(payload, 'id');
  const kind = payload.kind;
  if (id === null || !isEntityKind(kind)) {
    return null;
  }
  const hasIssue = 'issue_id' in payload;
  const hasLessons = 'lesson_ids' in payload;
  if ((hasIssue && kind !== 'submission') || (hasLessons && kind !== 'issue' && kind !== 'submission')) {
    return null;
  }
  const issueId = hasIssue ? textOf(payload, 'issue_id') : null;
  const lessonIds = hasLessons ? lessonIdsOf(payload.lesson_ids) : null;
  if ((hasIssue && issueId === null) || (hasLessons && lessonIds === null)) {
    return null;
  }
  return {
    type: 'entity.changed',
    kind,
    id,
    ...(issueId === null ? {} : { issueId }),
    ...(lessonIds === null ? {} : { lessonIds }),
  };
}

function seqOf(value: Fields, key: string): number | null {
  const field = value[key];
  return typeof field === 'number' && Number.isSafeInteger(field) && field > 0 ? field : null;
}

function messageFrame(frame: Fields, type: 'message.updated' | 'message.deleted'): ServerFrame | null {
  const payload = payloadOf(frame, ['channel_id', 'message_id']);
  if (payload === null || !hasOnly(frame, ['type', 'payload'])) {
    return null;
  }
  const channelId = textOf(payload, 'channel_id');
  const messageId = textOf(payload, 'message_id');
  return channelId === null || messageId === null ? null : { type, channelId, messageId };
}

function typingFrame(frame: Fields): ServerFrame | null {
  const payload = payloadOf(frame, ['channel_id', 'user_id']);
  if (payload === null || !hasOnly(frame, ['type', 'payload']) || !('user_id' in payload)) {
    return null;
  }
  const channelId = textOf(payload, 'channel_id');
  const userId = payload.user_id === null ? null : textOf(payload, 'user_id');
  if (channelId === null || (payload.user_id !== null && userId === null)) {
    return null;
  }
  return { type: 'typing', channelId, userId };
}

function presenceFrame(frame: Fields): ServerFrame | null {
  const payload = payloadOf(frame, ['user_id', 'online']);
  if (payload === null || !hasOnly(frame, ['type', 'payload'])) {
    return null;
  }
  const userId = textOf(payload, 'user_id');
  const online = payload.online;
  return userId === null || typeof online !== 'boolean' ? null : { type: 'presence', userId, online };
}

function parseFields(frame: Fields): ServerFrame | null {
  switch (frame.type) {
    case 'ready': {
      const payload = payloadOf(frame, []);
      return payload !== null && hasOnly(frame, ['type', 'payload']) ? { type: 'ready' } : null;
    }
    case 'notification.new': {
      const id = idFrame(frame);
      return id === null ? null : { type: 'notification.new', id };
    }
    case 'entity.changed':
      return entityFrame(frame);
    case 'message.new': {
      const payload = payloadOf(frame, ['channel_id', 'message_id', 'seq']);
      if (payload === null || !hasOnly(frame, ['type', 'payload'])) {
        return null;
      }
      const channelId = textOf(payload, 'channel_id');
      const messageId = textOf(payload, 'message_id');
      const seq = seqOf(payload, 'seq');
      return channelId === null || messageId === null || seq === null
        ? null
        : { type: 'message.new', channelId, messageId, seq };
    }
    case 'message.updated':
      return messageFrame(frame, 'message.updated');
    case 'message.deleted':
      return messageFrame(frame, 'message.deleted');
    case 'read.updated': {
      const payload = payloadOf(frame, ['channel_id']);
      const channelId = payload === null ? null : textOf(payload, 'channel_id');
      return channelId === null || !hasOnly(frame, ['type', 'payload']) ? null : { type: 'read.updated', channelId };
    }
    case 'typing':
      return typingFrame(frame);
    case 'presence':
      return presenceFrame(frame);
    case 'pong': {
      const ref = refOf(frame, 'ref', false);
      return ref === undefined || !hasOnly(frame, ['type', 'ref']) ? null : { type: 'pong', ref };
    }
    case 'error': {
      const payload = payloadOf(frame, ['code', 'message', 'request_id', 'ref']);
      if (payload === null || !hasOnly(frame, ['type', 'payload'])) {
        return null;
      }
      const code = textOf(payload, 'code');
      const message = textOf(payload, 'message');
      const requestId = textOf(payload, 'request_id');
      const ref = refOf(payload, 'ref', true);
      if (code === null || message === null || requestId === null || ref === undefined) {
        return null;
      }
      return { type: 'error', code, message, requestId, ref };
    }
    default:
      return null;
  }
}

export function parseServerFrame(text: string): ServerFrame | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    return null;
  }
  return isObject(parsed) ? parseFields(parsed) : null;
}

export function authFrame(accessToken: string): string {
  return JSON.stringify({ type: 'auth', payload: { access_token: accessToken } });
}

export function pingFrame(ref: string): string {
  return JSON.stringify({ type: 'ping', ref });
}

export function outgoingTypingFrame(channelId: string, typing: boolean): string {
  return JSON.stringify({ type: typing ? 'typing.start' : 'typing.stop', payload: { channel_id: channelId } });
}
