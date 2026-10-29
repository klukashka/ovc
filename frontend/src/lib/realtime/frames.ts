export const entityKinds = ['relationship', 'join_request', 'notification', 'lesson', 'channel'] as const;

export type EntityKind = (typeof entityKinds)[number];

export type ServerFrame =
  | { readonly type: 'ready' }
  | { readonly type: 'notification.new'; readonly id: string }
  | { readonly type: 'entity.changed'; readonly kind: EntityKind; readonly id: string }
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
    case 'entity.changed': {
      const payload = payloadOf(frame, ['kind', 'id']);
      const id = payload === null ? null : textOf(payload, 'id');
      if (payload === null || id === null || !isEntityKind(payload.kind) || !hasOnly(frame, ['type', 'payload'])) {
        return null;
      }
      return { type: 'entity.changed', kind: payload.kind, id };
    }
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
