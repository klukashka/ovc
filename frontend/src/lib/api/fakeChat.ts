import type { components } from './schema.ts';
import type { Me } from './session.ts';

type Channel = components['schemas']['ChannelDetail'];
type Message = components['schemas']['Message'];

export interface FakeChatMember {
  readonly accountId: string;
  readonly label?: string;
  readonly channel?: Channel;
  lastReadSeq?: number;
  suspended?: boolean;
}

interface StoredMessage {
  message: Message;
  readonly senderId: string;
  readonly key?: string;
}

interface StoredChannel {
  channel: Channel;
  readonly members: Map<string, FakeChatMember>;
  readonly messages: StoredMessage[];
  activity: number;
}

function answer(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' },
  });
}

function refused(status: number, code: string): Response {
  return answer(status, { error: { code, message: code, request_id: 'fake-request' } });
}

function validBody(body: unknown): body is { readonly body: string } {
  return typeof body === 'object' && body !== null && 'body' in body && typeof body.body === 'string';
}

function acceptableText(body: string): boolean {
  return (
    body.trim().length > 0 &&
    Array.from(body).length <= 4_000 &&
    Array.from(body).every((character) => {
      const point = character.codePointAt(0);
      return point !== undefined && (point === 9 || point === 10 || (point >= 32 && point !== 127));
    })
  );
}

export class FakeChatServer {
  private readonly channels = new Map<string, StoredChannel>();
  private sequence = 0;
  private issued = 0;
  private loseSendResponse = false;
  private failDelete = false;
  private channelPause: { readonly answered: () => void; readonly resumed: Promise<void> } | null = null;

  failNextDelete(): void {
    this.failDelete = true;
  }

  pauseNextChannelResponse(): { readonly answered: Promise<void>; readonly resume: () => void } {
    let answer = (): void => {
      throw new Error('the channel pause is not initialized');
    };
    let resume = (): void => {
      throw new Error('the channel pause is not initialized');
    };
    const answered = new Promise<void>((resolve) => {
      answer = resolve;
    });
    const resumed = new Promise<void>((resolve) => {
      resume = resolve;
    });
    this.channelPause = { answered: answer, resumed };
    return { answered, resume };
  }
  private listPause: {
    readonly answered: () => void;
    readonly resumed: Promise<void>;
    readonly cursorOnly: boolean;
  } | null = null;
  private sendPause: { readonly answered: () => void; readonly resumed: Promise<void> } | null = null;

  pauseNextChannelListResponse(cursorOnly = true): { readonly answered: Promise<void>; readonly resume: () => void } {
    let answer = (): void => {
      throw new Error('The list pause is not initialized');
    };
    let resume = (): void => {
      throw new Error('The list pause is not initialized');
    };
    const answered = new Promise<void>((resolve) => {
      answer = resolve;
    });
    const resumed = new Promise<void>((resolve) => {
      resume = resolve;
    });
    this.listPause = { answered: answer, resumed, cursorOnly };
    return { answered, resume };
  }

  pauseNextSendResponse(): { readonly answered: Promise<void>; readonly resume: () => void } {
    let answer = (): void => {
      throw new Error('The send pause is not initialized');
    };
    let resume = (): void => {
      throw new Error('The send pause is not initialized');
    };
    const answered = new Promise<void>((resolve) => {
      answer = resolve;
    });
    const resumed = new Promise<void>((resolve) => {
      resume = resolve;
    });
    this.sendPause = { answered: answer, resumed };
    return { answered, resume };
  }
  private readPause: { readonly answered: () => void; readonly resumed: Promise<void> } | null = null;

  pauseNextReadResponse(): { readonly answered: Promise<void>; readonly resume: () => void } {
    let answer = (): void => {
      throw new Error('the read pause is not initialized');
    };
    let resume = (): void => {
      throw new Error('the read pause is not initialized');
    };
    const answered = new Promise<void>((resolve) => {
      answer = resolve;
    });
    const resumed = new Promise<void>((resolve) => {
      resume = resolve;
    });
    this.readPause = { answered: answer, resumed };
    return { answered, resume };
  }

  loseNextSendResponse(): void {
    this.loseSendResponse = true;
  }

  addChannel(seed: { readonly channel: Channel; readonly members: readonly FakeChatMember[] }): void {
    this.sequence += 1;
    this.channels.set(seed.channel.id, {
      channel: { ...seed.channel },
      members: new Map(seed.members.map((member) => [member.accountId, { ...member }])),
      messages: [],
      activity: this.sequence,
    });
  }

  addMessage(channelId: string, seed: Message & { readonly senderId: string }): void {
    const channel = this.requiredChannel(channelId);
    const { senderId, ...message } = seed;
    channel.messages.push({ message, senderId });
    this.sequence += 1;
    channel.activity = this.sequence;
  }

  setWritable(channelId: string, writable: boolean): void {
    this.requiredChannel(channelId).channel.writable = writable;
  }

  removeMember(channelId: string, accountId: string): void {
    this.requiredChannel(channelId).members.delete(accountId);
  }

  suspendMember(channelId: string, accountId: string, suspended: boolean): void {
    const member = this.requiredChannel(channelId).members.get(accountId);
    if (member === undefined) {
      throw new Error('no fake channel member');
    }
    member.suspended = suspended;
  }

  readSeq(channelId: string, accountId: string): number {
    return this.requiredChannel(channelId).members.get(accountId)?.lastReadSeq ?? 0;
  }

  history(channelId: string): readonly Message[] {
    return this.requiredChannel(channelId).messages.map((stored) => ({ ...stored.message }));
  }

  private requiredChannel(channelId: string): StoredChannel {
    const channel = this.channels.get(channelId);
    if (channel === undefined) {
      throw new Error('no fake channel');
    }
    return channel;
  }

  private visible(channel: StoredChannel, caller: Me): boolean {
    const member = channel.members.get(caller.id);
    return (
      member !== undefined &&
      member.suspended !== true &&
      (caller.state !== 'unverified' || caller.role !== 'student' || channel.channel.kind === 'direct')
    );
  }

  private messageFor(stored: StoredMessage, caller: Me, channel: StoredChannel): Message {
    const label = channel.members.get(stored.senderId)?.label;
    return {
      ...stored.message,
      own: stored.senderId === caller.id,
      sender_label: label ?? stored.message.sender_label,
    };
  }

  private detail(channel: StoredChannel, caller: Me): Channel {
    const member = channel.members.get(caller.id);
    const base = member?.channel ?? channel.channel;
    const last = [...channel.messages].sort((left, right) => right.message.seq - left.message.seq)[0];
    const result: Channel = {
      ...base,
      writable: channel.channel.writable,
      unread_count: Math.min(99, Math.max(0, (last?.message.seq ?? 0) - (member?.lastReadSeq ?? 0))),
      last_message:
        last === undefined
          ? null
          : {
              seq: last.message.seq,
              own: last.senderId === caller.id,
              created_at: last.message.created_at,
              sender_label: this.messageFor(last, caller, channel).sender_label,
              sender_label_kind: last.message.sender_label_kind,
              preview:
                last.message.deleted || last.message.erased
                  ? null
                  : Array.from(last.message.body ?? '')
                      .slice(0, 120)
                      .join('') || null,
              attachment_count: last.message.attachments.length,
            },
    };
    if (caller.role === 'student') {
      delete result.members;
      delete result.group_id;
      delete result.relationship_id;
      result.unconfirmed = false;
    }
    return result;
  }

  async handle(request: Request, caller: Me, now: number): Promise<Response> {
    const address = new URL(request.url);
    const parts = address.pathname.split('/').filter(Boolean);
    const id = parts[3] ?? '';
    if (parts[2] === 'messages') {
      return this.changeMessage(request, caller, id, now);
    }
    if (request.method === 'GET' && (id === '' || id === 'unread')) {
      const visible = [...this.channels.values()].filter((channel) => this.visible(channel, caller));
      const unread = Math.min(100, visible.filter((channel) => this.detail(channel, caller).unread_count > 0).length);
      if (id === 'unread') {
        return answer(200, { unread_channels: unread });
      }
      const q = (address.searchParams.get('q') ?? '').toLocaleLowerCase();
      const sorted = visible
        .filter((channel) => (this.detail(channel, caller).label ?? '').toLocaleLowerCase().includes(q))
        .sort((left, right) => right.activity - left.activity || left.channel.id.localeCompare(right.channel.id));
      const cursor = address.searchParams.get('cursor');
      const offset = cursor === null ? 0 : sorted.findIndex((channel) => channel.channel.id === cursor) + 1;
      const limit = Math.min(100, Math.max(1, Number(address.searchParams.get('limit') ?? 30)));
      const page = sorted.slice(offset, offset + limit);
      const response = answer(200, {
        items: page.map((channel) => this.detail(channel, caller)),
        next_cursor: offset + limit < sorted.length ? page.at(-1)?.channel.id : null,
        unread_channels: unread,
      });
      const pause = this.listPause;
      if (pause !== null && (!pause.cursorOnly || cursor !== null)) {
        this.listPause = null;
        pause.answered();
        await pause.resumed;
      }
      return response;
    }
    const channel = this.channels.get(id);
    if (parts.length === 4 && request.method === 'GET') {
      const response =
        channel === undefined || !this.visible(channel, caller)
          ? refused(404, 'not_found')
          : answer(200, this.detail(channel, caller));
      const pause = this.channelPause;
      this.channelPause = null;
      if (pause !== null) {
        pause.answered();
        await pause.resumed;
      }
      return response;
    }
    if (channel === undefined || !this.visible(channel, caller)) {
      return refused(404, 'not_found');
    }
    if (parts[4] === 'messages' && request.method === 'GET') {
      return this.listMessages(channel, caller, address.searchParams);
    }
    if (parts[4] === 'messages' && request.method === 'POST') {
      const response = await this.sendMessage(channel, caller, request, now);
      if (response.status === 201 && this.loseSendResponse) {
        this.loseSendResponse = false;
        throw new TypeError('The fake connection closed before the response');
      }
      const pause = this.sendPause;
      this.sendPause = null;
      if (pause !== null) {
        pause.answered();
        await pause.resumed;
      }
      return response;
    }
    if (parts[4] === 'read' && request.method === 'POST') {
      const body: unknown = await request.json();
      if (
        typeof body !== 'object' ||
        body === null ||
        !('seq' in body) ||
        typeof body.seq !== 'number' ||
        !Number.isSafeInteger(body.seq) ||
        body.seq < 0
      ) {
        return refused(422, 'validation_failed');
      }
      const member = channel.members.get(caller.id);
      if (member === undefined) {
        throw new Error('visible channel has no member');
      }
      const maximum = Math.max(0, ...channel.messages.map((stored) => stored.message.seq));
      member.lastReadSeq = Math.max(member.lastReadSeq ?? 0, Math.min(body.seq, maximum));
      const pause = this.readPause;
      this.readPause = null;
      if (pause !== null) {
        pause.answered();
        await pause.resumed;
      }
      return new Response(null, { status: 204 });
    }
    return refused(404, 'not_found');
  }

  private listMessages(channel: StoredChannel, caller: Me, query: URLSearchParams): Response {
    if (query.has('cursor') && query.has('after_seq')) {
      return refused(422, 'validation_failed');
    }
    const after = query.has('after_seq') ? Number(query.get('after_seq')) : null;
    const before = Number(query.get('cursor') ?? Number.MAX_SAFE_INTEGER);
    const limit = Math.min(100, Math.max(1, Number(query.get('limit') ?? 50)));
    const sorted = channel.messages
      .filter((stored) => (after === null ? stored.message.seq < before : stored.message.seq > after))
      .sort((left, right) =>
        after === null ? right.message.seq - left.message.seq : left.message.seq - right.message.seq,
      );
    const page = sorted.slice(0, limit);
    return answer(200, {
      items: page.map((stored) => this.messageFor(stored, caller, channel)),
      next_cursor: after === null && sorted.length > limit ? String(page.at(-1)?.message.seq) : null,
    });
  }

  private async sendMessage(channel: StoredChannel, caller: Me, request: Request, now: number): Promise<Response> {
    const key = request.headers.get('Idempotency-Key') ?? '';
    if (!/^[A-Za-z0-9._-]{1,64}$/.test(key)) {
      return refused(422, 'validation_failed');
    }
    const previous = channel.messages.find((stored) => stored.senderId === caller.id && stored.key === key);
    if (previous !== undefined) {
      return answer(201, this.messageFor(previous, caller, channel));
    }
    if (!channel.channel.writable) {
      return refused(409, 'channel_read_only');
    }
    const body: unknown = await request.json();
    if (
      !validBody(body) ||
      !acceptableText(body.body) ||
      !('attachment_ids' in body) ||
      !Array.isArray(body.attachment_ids) ||
      body.attachment_ids.length !== 0
    ) {
      return refused(422, 'validation_failed');
    }
    this.issued += 1;
    const stored: StoredMessage = {
      key,
      senderId: caller.id,
      message: {
        id: `00000000-0000-4000-9000-${String(this.issued).padStart(12, '0')}`,
        body: body.body,
        attachments: [],
        created_at: new Date(now).toISOString(),
        edited_at: null,
        deleted: false,
        erased: false,
        own: true,
        sender_label: caller.name,
        sender_label_kind: null,
        sent_before_reclaim: false,
        seq: Math.max(0, ...channel.messages.map((entry) => entry.message.seq)) + 1,
        version: 1,
      },
    };
    channel.messages.push(stored);
    this.sequence += 1;
    channel.activity = this.sequence;
    const member = channel.members.get(caller.id);
    if (member === undefined) {
      throw new Error('visible channel has no member');
    }
    member.lastReadSeq = stored.message.seq;
    return answer(201, this.messageFor(stored, caller, channel));
  }

  private async changeMessage(request: Request, caller: Me, id: string, now: number): Promise<Response> {
    const channel = [...this.channels.values()].find((entry) =>
      entry.messages.some((stored) => stored.message.id === id),
    );
    const stored = channel?.messages.find((entry) => entry.message.id === id);
    if (channel === undefined || stored === undefined || !this.visible(channel, caller)) {
      return refused(404, 'not_found');
    }
    if (request.method === 'DELETE' && (stored.senderId === caller.id || caller.role === 'tutor')) {
      if (this.failDelete) {
        this.failDelete = false;
        return refused(500, 'internal_error');
      }
      stored.message = { ...stored.message, body: null, attachments: [], deleted: true };
      return new Response(null, { status: 204 });
    }
    if (
      request.method !== 'PATCH' ||
      stored.senderId !== caller.id ||
      stored.message.deleted ||
      stored.message.erased
    ) {
      return refused(404, 'not_found');
    }
    if (!channel.channel.writable) {
      return refused(409, 'channel_read_only');
    }
    if (now - Date.parse(stored.message.created_at) >= 24 * 60 * 60 * 1_000) {
      return refused(409, 'edit_window_passed');
    }
    const body: unknown = await request.json();
    if (!validBody(body) || !acceptableText(body.body) || !('version' in body) || typeof body.version !== 'number') {
      return refused(422, 'validation_failed');
    }
    if (body.version !== stored.message.version) {
      return refused(409, 'conflict');
    }
    stored.message = {
      ...stored.message,
      body: body.body,
      version: stored.message.version + 1,
      edited_at: new Date(now).toISOString(),
    };
    return answer(200, this.messageFor(stored, caller, channel));
  }
}
