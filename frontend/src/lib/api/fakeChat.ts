import { type components, messageReactionReactionValues } from './schema.ts';
import type { Me } from './session.ts';

type Channel = components['schemas']['ChannelDetail'];
type Message = components['schemas']['Message'];
type TutorHomeUnreadChannels = components['schemas']['TutorHomeUnreadChannels'];

const firstChannelPage = 30;
const homeChannelLimit = 5;

export type FakeInitialReadTarget = 'unread' | 'list' | 'current_channel';
export interface FakeInitialReadSnapshot {
  readonly target: FakeInitialReadTarget;
  readonly totalAttempts: number;
  readonly retainedLifetimes: number;
  readonly firstAborted: boolean;
  readonly secondAborted: boolean;
  readonly firstAbortedAtSecondInvocation: boolean | null;
}
export interface FakeInitialReadObservation {
  readonly snapshot: () => readonly FakeInitialReadSnapshot[];
  readonly stop: () => void;
}
interface ObservedInitialRead {
  totalAttempts: number;
  first: AbortSignal | null;
  second: AbortSignal | null;
  firstAbortedAtSecondInvocation: boolean | null;
}

export interface FakeChatMember {
  readonly accountId: string;
  readonly label?: string;
  readonly channel?: Channel;
  lastReadSeq?: number;
  sharedReadSeq?: number;
  suspended?: boolean;
}

const reactionOrder = messageReactionReactionValues;
type FakeReaction = (typeof reactionOrder)[number];

function isReaction(value: unknown): value is FakeReaction {
  return typeof value === 'string' && reactionOrder.some((reaction) => reaction === value);
}

interface StoredMessage {
  message: Message;
  reactions?: Map<string, FakeReaction>;
  readonly senderId: string;
  readonly key?: string;
  readonly replyTo?: string;
}

interface StoredChannel {
  channel: Channel;
  readonly members: Map<string, FakeChatMember>;
  readonly messages: StoredMessage[];
  activity: number;
  pinned: string | null;
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

function refusedAt(status: number, code: string, path: string): Response {
  return answer(status, {
    error: { code, message: code, request_id: 'fake-request', fields: [{ path, code: 'not_found' }] },
  });
}

function previewOf(message: Message): string | null {
  if (message.deleted || message.erased) return null;
  return (
    Array.from(message.body ?? '')
      .slice(0, 120)
      .join('') || null
  );
}

function replyTarget(body: object): string | null | undefined {
  if (!('reply_to_message_id' in body)) return null;
  return typeof body.reply_to_message_id === 'string' ? body.reply_to_message_id : undefined;
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
  private destinationObservation: {
    readonly origin: string;
    readonly channel: string;
    readonly state: ObservedInitialRead;
  } | null = null;
  private destinationObservationUsed = false;

  observeDestinationDetail(origin: string, channel: string): FakeInitialReadObservation {
    if (this.destinationObservationUsed) throw new Error('destination_observation_already_used');
    this.destinationObservationUsed = true;
    const state: ObservedInitialRead = {
      totalAttempts: 0,
      first: null,
      second: null,
      firstAbortedAtSecondInvocation: null,
    };
    const observation = { origin, channel, state };
    this.destinationObservation = observation;
    return {
      snapshot: (): readonly FakeInitialReadSnapshot[] => [
        {
          target: 'current_channel',
          totalAttempts: state.totalAttempts,
          retainedLifetimes: state.second === null ? (state.first === null ? 0 : 1) : 2,
          firstAborted: state.first?.aborted ?? false,
          secondAborted: state.second?.aborted ?? false,
          firstAbortedAtSecondInvocation: state.firstAbortedAtSecondInvocation,
        },
      ],
      stop: (): void => {
        if (this.destinationObservation === observation) this.destinationObservation = null;
      },
    };
  }

  private initialReadObservation: {
    readonly channelId: string;
    readonly origin: string;
    readonly targets: Map<FakeInitialReadTarget, ObservedInitialRead>;
  } | null = null;

  observeInitialReads(origin: string, channelId: string): FakeInitialReadObservation {
    if (this.initialReadObservation !== null) throw new Error('initial_read_observation_already_armed');
    const targets = new Map<FakeInitialReadTarget, ObservedInitialRead>();
    for (const target of ['unread', 'list', 'current_channel'] as const)
      targets.set(target, { totalAttempts: 0, first: null, second: null, firstAbortedAtSecondInvocation: null });
    const observation = { origin, channelId, targets };
    this.initialReadObservation = observation;
    return {
      snapshot: (): readonly FakeInitialReadSnapshot[] =>
        [...targets].map(([target, value]) => ({
          target,
          totalAttempts: value.totalAttempts,
          retainedLifetimes: value.second === null ? (value.first === null ? 0 : 1) : 2,
          firstAborted: value.first?.aborted ?? false,
          secondAborted: value.second?.aborted ?? false,
          firstAbortedAtSecondInvocation: value.firstAbortedAtSecondInvocation,
        })),
      stop: (): void => {
        if (this.initialReadObservation === observation) this.initialReadObservation = null;
      },
    };
  }

  observeInitialReadInvocation(method: string, address: string, signal: AbortSignal): void {
    const destination = this.destinationObservation;
    if (destination !== null && method === 'GET') {
      const url = new URL(address);
      if (
        url.origin === destination.origin &&
        url.pathname === `/api/v1/channels/${destination.channel}` &&
        [...url.searchParams].length === 0
      ) {
        const state = destination.state;
        state.totalAttempts += 1;
        if (state.totalAttempts === 1) state.first = signal;
        if (state.totalAttempts === 2) {
          state.second = signal;
          state.firstAbortedAtSecondInvocation = state.first?.aborted ?? false;
        }
      }
    }
    const observation = this.initialReadObservation;
    if (observation === null || method !== 'GET') return;
    const url = new URL(address);
    if (url.origin !== observation.origin) return;
    const query = [...url.searchParams];
    const target =
      url.pathname === '/api/v1/channels/unread' && query.length === 0
        ? 'unread'
        : url.pathname === '/api/v1/channels' && query.length === 1 && query[0]?.[0] === 'q' && query[0][1] === ''
          ? 'list'
          : url.pathname === `/api/v1/channels/${observation.channelId}` && query.length === 0
            ? 'current_channel'
            : null;
    if (target === null) return;
    const state = observation.targets.get(target);
    if (state === undefined) throw new Error('initial_read_target_missing');
    state.totalAttempts += 1;
    if (state.totalAttempts === 1) state.first = signal;
    else if (state.totalAttempts === 2) {
      state.second = signal;
      state.firstAbortedAtSecondInvocation = state.first?.aborted ?? false;
    }
  }

  private readonly channels = new Map<string, StoredChannel>();
  private sequence = 0;
  private issued = 0;
  private uploaded = 0;
  private readonly uploads = new Map<
    string,
    {
      readonly caller: string;
      readonly channel: string;
      readonly file: components['schemas']['MessageAttachment'];
      bound: boolean;
    }
  >();
  readonly sends: {
    readonly body: string;
    readonly ids: readonly string[];
    readonly key: string;
    readonly replyTo: string | null;
  }[] = [];

  async upload(request: Request, caller: Me, body: FormData, now: number): Promise<Response> {
    request.signal.throwIfAborted();
    const address = new URL(request.url);
    if (address.searchParams.get('context') !== 'chat') return refused(422, 'validation_failed');
    if (!caller.email_verified) return refused(403, 'email_not_verified');
    const scope = address.searchParams.get('scope') ?? '';
    const channel = scope.startsWith('channel:') ? this.channels.get(scope.slice(8)) : undefined;
    if (channel === undefined || !this.visible(channel, caller)) return refused(404, 'not_found');
    if (!channel.channel.writable) return refused(409, 'channel_read_only');
    const parts = [...body.entries()];
    const file = body.get('file');
    if (parts.length !== 1 || file === null || typeof file === 'string') return refused(422, 'validation_failed');
    if (file.size > 25_000_000) return refused(413, 'payload_too_large');
    if (file.type !== 'text/plain' || !file.name.endsWith('.txt')) return refused(415, 'unsupported_media_type');
    this.uploaded += 1;
    const id = `00000000-0000-4000-8000-${String(this.uploaded).padStart(12, '0')}`;
    const uploaded = { id, filename: file.name, size_bytes: file.size, media_type: file.type };
    this.uploads.set(id, {
      caller: caller.id,
      channel: channel.channel.id,
      bound: false,
      file: {
        ...uploaded,
        width: null,
        height: null,
        url: `/api/v1/files/${id}?t=fake-download`,
        expires_at: new Date(now + 600_000).toISOString(),
      },
    });
    await Promise.resolve();
    request.signal.throwIfAborted();
    return answer(201, uploaded);
  }
  private loseSendResponse = false;
  private failDelete = false;
  private failHistoryRead = false;
  private readonly searchRefusals: { readonly status: number; readonly code: string }[] = [];
  private readonly searches = new Map<string, { window: number; count: number }>();
  private readonly reactionWrites = new Map<string, { window: number; count: number }>();
  private failChannelListRead = false;

  failNextChannelListRead(): void {
    this.failChannelListRead = true;
  }
  private failChannelRead = false;
  private messagePause: { readonly answered: () => void; readonly resumed: Promise<void> } | null = null;

  failNextHistoryRead(): void {
    this.failHistoryRead = true;
  }
  private channelPause: { readonly answered: () => void; readonly resumed: Promise<void> } | null = null;

  failNextDelete(): void {
    this.failDelete = true;
  }

  failNextChannelRead(): void {
    this.failChannelRead = true;
  }

  pauseNextMessageResponse(): { readonly answered: Promise<void>; readonly resume: () => void } {
    let answer = (): void => {
      throw new Error('The message pause is not initialized');
    };
    let resume = (): void => {
      throw new Error('The message pause is not initialized');
    };
    const answered = new Promise<void>((resolve) => {
      answer = resolve;
    });
    const resumed = new Promise<void>((resolve) => {
      resume = resolve;
    });
    this.messagePause = { answered: answer, resumed };
    return { answered, resume };
  }

  private historyPause: { readonly answered: () => void; readonly resumed: Promise<void> } | null = null;

  pauseNextHistoryResponse(): { readonly answered: Promise<void>; readonly resume: () => void } {
    let answer = (): void => {
      throw new Error('the history pause is not initialized');
    };
    let resume = (): void => {
      throw new Error('the history pause is not initialized');
    };
    const answered = new Promise<void>((resolve) => {
      answer = resolve;
    });
    const resumed = new Promise<void>((resolve) => {
      resume = resolve;
    });
    this.historyPause = { answered: answer, resumed };
    return { answered, resume };
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
    readonly answered: (signal: AbortSignal) => void;
    readonly resumed: Promise<void>;
    readonly cursorOnly: boolean;
  } | null = null;
  private sendPause: { readonly answered: () => void; readonly resumed: Promise<void> } | null = null;

  pauseNextChannelListResponse(cursorOnly = true): {
    readonly answered: Promise<AbortSignal>;
    readonly resume: () => void;
  } {
    let answer: (signal: AbortSignal) => void = (): void => {
      throw new Error('The list pause is not initialized');
    };
    let resume = (): void => {
      throw new Error('The list pause is not initialized');
    };
    const answered = new Promise<AbortSignal>((resolve) => {
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
      pinned: null,
    });
  }

  addMessage(channelId: string, seed: Message & { readonly senderId: string; readonly replyTo?: string }): void {
    const channel = this.requiredChannel(channelId);
    const { senderId, replyTo, ...message } = seed;
    channel.messages.push(replyTo === undefined ? { message, senderId } : { message, senderId, replyTo });
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

  pinOf(channelId: string): string | null {
    return this.requiredChannel(channelId).pinned;
  }

  pin(channelId: string, messageId: string): void {
    this.requiredChannel(channelId).pinned = messageId;
  }

  readSeq(channelId: string, accountId: string): number {
    return this.requiredChannel(channelId).members.get(accountId)?.lastReadSeq ?? 0;
  }

  memberRead(channelId: string, accountId: string, seq: number): void {
    const channel = this.requiredChannel(channelId);
    const member = channel.members.get(accountId);
    if (member === undefined) {
      throw new Error('no fake channel member');
    }
    const reported = Math.min(seq, Math.max(0, ...channel.messages.map((stored) => stored.message.seq)));
    member.lastReadSeq = Math.max(member.lastReadSeq ?? 0, reported);
    this.expose(channel, member, reported);
  }

  private expose(channel: StoredChannel, member: FakeChatMember, reported: number): void {
    const members = [...channel.members.values()];
    const shareable =
      channel.channel.kind === 'direct' &&
      channel.channel.writable &&
      members.length === 2 &&
      members.every((entry) => entry.suspended !== true);
    if (shareable) member.sharedReadSeq = Math.max(member.sharedReadSeq ?? 0, reported);
  }

  private peerRead(channel: StoredChannel, caller: Me): number | null {
    if (channel.channel.kind !== 'direct') return null;
    const peer = [...channel.members.values()].find((entry) => entry.accountId !== caller.id);
    return peer === undefined ? null : (peer.sharedReadSeq ?? 0);
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

  homeUnreadChannels(caller: Me): TutorHomeUnreadChannels {
    const visible = this.visibleChannels(caller);
    return {
      items: this.byActivity(visible)
        .slice(0, firstChannelPage)
        .map((channel) => this.detail(channel, caller))
        .filter((detail) => detail.unread_count > 0)
        .slice(0, homeChannelLimit)
        .map((detail) => ({
          channel_id: detail.id,
          label: detail.label,
          label_kind: detail.label_kind,
          colour: detail.colour,
          unread_count: detail.unread_count,
        })),
      unread_total: this.unreadTotal(visible, caller),
    };
  }

  private visibleChannels(caller: Me): StoredChannel[] {
    return [...this.channels.values()].filter((channel) => this.visible(channel, caller));
  }

  private unreadTotal(visible: readonly StoredChannel[], caller: Me): number {
    return Math.min(100, visible.filter((channel) => this.detail(channel, caller).unread_count > 0).length);
  }

  private byActivity(channels: readonly StoredChannel[]): StoredChannel[] {
    return [...channels].sort(
      (left, right) => right.activity - left.activity || left.channel.id.localeCompare(right.channel.id),
    );
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
    const quoted = channel.messages.find((entry) => entry.message.id === stored.replyTo);
    return {
      ...stored.message,
      own: stored.senderId === caller.id,
      reactions: this.reactionsFor(stored, caller, channel),
      sender_label: label ?? stored.message.sender_label,
      reply_to:
        quoted === undefined
          ? null
          : {
              message_id: quoted.message.id,
              seq: quoted.message.seq,
              sender_label:
                quoted.senderId === caller.id ? null : this.messageFor(quoted, caller, channel).sender_label,
              sender_label_kind: quoted.message.sender_label_kind,
              own: quoted.senderId === caller.id,
              preview: previewOf(quoted.message),
              state: quoted.message.erased ? 'removed' : quoted.message.deleted ? 'deleted' : 'available',
              sent_before_reclaim: quoted.message.sent_before_reclaim,
            },
    };
  }

  private reactionsFor(stored: StoredMessage, caller: Me, channel: StoredChannel): Message['reactions'] {
    const reactions = stored.reactions;
    if (reactions === undefined || reactions.size === 0) return [];
    return reactionOrder.flatMap((reaction) => {
      const reactors = [...reactions].filter(([, value]) => value === reaction).map(([account]) => account);
      if (reactors.length === 0) return [];
      const view: Message['reactions'][number] = {
        reaction,
        count: reactors.length,
        mine: reactors.includes(caller.id),
      };
      if (caller.role === 'tutor') {
        view.members = reactors
          .filter((account) => account !== caller.id)
          .map((account) => {
            const label = channel.members.get(account)?.label ?? null;
            return { label, label_kind: label === null ? 'deleted_user' : null };
          });
      }
      return [view];
    });
  }

  memberReact(messageId: string, accountId: string, reaction: FakeReaction | null): void {
    const stored = [...this.channels.values()]
      .flatMap((channel) => channel.messages)
      .find((entry) => entry.message.id === messageId);
    if (stored === undefined) throw new Error('no fake message');
    stored.reactions ??= new Map();
    if (reaction === null) stored.reactions.delete(accountId);
    else stored.reactions.set(accountId, reaction);
  }

  private async react(
    channel: StoredChannel,
    stored: StoredMessage,
    caller: Me,
    request: Request,
    now: number,
  ): Promise<Response> {
    if (stored.message.deleted || stored.message.erased) return refused(404, 'not_found');
    const window = Math.floor(now / 60_000);
    const counted = this.reactionWrites.get(caller.id);
    const count = counted?.window === window ? counted.count : 0;
    if (count >= 30) {
      return new Response(
        JSON.stringify({ error: { code: 'rate_limited', message: 'rate_limited', request_id: 'fake-request' } }),
        { status: 429, headers: { 'Content-Type': 'application/json', 'Retry-After': '60' } },
      );
    }
    if (request.method === 'DELETE') {
      this.reactionWrites.set(caller.id, { window, count: count + 1 });
      stored.reactions?.delete(caller.id);
      return new Response(null, { status: 204 });
    }
    const body: unknown = await request.json();
    if (typeof body !== 'object' || body === null || !('reaction' in body) || !isReaction(body.reaction)) {
      return refusedAt(422, 'validation_failed', 'body.reaction');
    }
    if (!channel.channel.writable) return refused(409, 'channel_read_only');
    this.reactionWrites.set(caller.id, { window, count: count + 1 });
    stored.reactions ??= new Map();
    stored.reactions.set(caller.id, body.reaction);
    return new Response(null, { status: 204 });
  }

  private pinnedFor(channel: StoredChannel, caller: Me): Channel['pinned'] {
    const stored = channel.messages.find((entry) => entry.message.id === channel.pinned);
    if (stored === undefined || stored.message.deleted || stored.message.erased) return null;
    const view = this.messageFor(stored, caller, channel);
    return {
      message_id: view.id,
      seq: view.seq,
      sender_label: view.own ? null : view.sender_label,
      sender_label_kind: view.sender_label_kind,
      own: view.own,
      preview: previewOf(view),
      sent_before_reclaim: view.sent_before_reclaim,
    };
  }

  private detail(channel: StoredChannel, caller: Me): Channel {
    const member = channel.members.get(caller.id);
    const base = member?.channel ?? channel.channel;
    const last = [...channel.messages].sort((left, right) => right.message.seq - left.message.seq)[0];
    const result: Channel = {
      ...base,
      pinned: this.pinnedFor(channel, caller),
      peer_read_seq: this.peerRead(channel, caller),
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
      const response = await this.changeMessage(request, caller, id, now);
      const pause = this.messagePause;
      this.messagePause = null;
      if (pause !== null) {
        pause.answered();
        await pause.resumed;
      }
      return response;
    }
    if (request.method === 'GET' && (id === '' || id === 'unread')) {
      const visible = this.visibleChannels(caller);
      const unread = this.unreadTotal(visible, caller);
      if (id === 'unread') {
        return answer(200, { unread_channels: unread });
      }
      if (this.failChannelListRead) {
        this.failChannelListRead = false;
        return refused(503, 'service_unavailable');
      }
      const q = (address.searchParams.get('q') ?? '').toLocaleLowerCase();
      const sorted = this.byActivity(
        visible.filter((channel) => (this.detail(channel, caller).label ?? '').toLocaleLowerCase().includes(q)),
      );
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
        pause.answered(request.signal);
        await pause.resumed;
      }
      return response;
    }
    const channel = this.channels.get(id);
    if (parts.length === 4 && request.method === 'GET') {
      if (this.failChannelRead) {
        this.failChannelRead = false;
        return refused(503, 'service_unavailable');
      }
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
    if (parts[4] === 'messages' && request.method === 'GET') {
      let response: Response;
      if (channel === undefined || !this.visible(channel, caller)) response = refused(404, 'not_found');
      else if (this.failHistoryRead) {
        this.failHistoryRead = false;
        response = refused(503, 'service_unavailable');
      } else response = this.listMessages(channel, caller, address.searchParams);
      const pause = this.historyPause;
      this.historyPause = null;
      if (pause !== null) {
        pause.answered();
        await pause.resumed;
      }
      return response;
    }
    if (channel === undefined || !this.visible(channel, caller)) {
      return refused(404, 'not_found');
    }
    if (parts[4] === 'messages' && parts[5] === 'search' && request.method === 'POST') {
      return this.searchMessages(channel, caller, request, now);
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
    if (parts[4] === 'pin' && (request.method === 'PUT' || request.method === 'DELETE')) {
      if (caller.role !== 'tutor') return refused(404, 'not_found');
      if (request.method === 'DELETE') {
        channel.pinned = null;
        return new Response(null, { status: 204 });
      }
      const body: unknown = await request.json();
      const message =
        typeof body === 'object' && body !== null && 'message_id' in body && typeof body.message_id === 'string'
          ? channel.messages.find((entry) => entry.message.id === body.message_id)
          : undefined;
      if (message === undefined || message.message.deleted || message.message.erased)
        return refusedAt(404, 'not_found', 'body.message_id');
      if (!channel.channel.writable) return refused(409, 'channel_read_only');
      channel.pinned = message.message.id;
      return new Response(null, { status: 204 });
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
      this.expose(channel, member, Math.min(body.seq, maximum));
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
    if (query.has('around_seq')) {
      const around = Number(query.get('around_seq'));
      const limit = Math.min(100, Math.max(1, Number(query.get('limit') ?? 50)));
      const newerCount = Math.floor(limit / 2);
      const olderCount = limit - newerCount;
      const ascending = [...channel.messages].sort((left, right) => left.message.seq - right.message.seq);
      const older = ascending.filter((stored) => stored.message.seq <= around);
      const newer = ascending.filter((stored) => stored.message.seq > around);
      const kept = [...older.slice(-olderCount), ...newer.slice(0, newerCount)];
      return answer(200, {
        items: kept.map((stored) => this.messageFor(stored, caller, channel)),
        next_cursor: older.length > olderCount ? String(older.at(-olderCount)?.message.seq) : null,
        has_newer: newer.length > newerCount,
        peer_read_seq: this.peerRead(channel, caller),
      });
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
      has_newer: false,
      peer_read_seq: this.peerRead(channel, caller),
    });
  }

  private searchPause: { readonly answered: () => void; readonly resumed: Promise<void> } | null = null;

  pauseNextSearchResponse(): { readonly answered: Promise<void>; readonly resume: () => void } {
    let answer = (): void => {
      throw new Error('the search pause is not initialized');
    };
    let resume = (): void => {
      throw new Error('the search pause is not initialized');
    };
    const answered = new Promise<void>((resolve) => {
      answer = resolve;
    });
    const resumed = new Promise<void>((resolve) => {
      resume = resolve;
    });
    this.searchPause = { answered: answer, resumed };
    return { answered, resume };
  }

  refuseNextSearch(status: number, code: string): void {
    this.searchRefusals.push({ status, code });
  }

  private async searchMessages(channel: StoredChannel, caller: Me, request: Request, now: number): Promise<Response> {
    const body: unknown = await request.json();
    if (typeof body !== 'object' || body === null || !('q' in body) || typeof body.q !== 'string') {
      return refused(422, 'validation_failed');
    }
    const term = body.q.trim();
    const length = Array.from(term).length;
    if (length < 2 || length > 100 || Array.from(term).some((character) => /\p{Cc}/u.test(character))) {
      return refusedAt(422, 'validation_failed', 'body.q');
    }
    const cursor = 'cursor' in body && typeof body.cursor === 'string' ? Number(body.cursor) : null;
    if (cursor !== null && (!Number.isSafeInteger(cursor) || cursor <= 0)) {
      return refusedAt(422, 'validation_failed', 'body.cursor');
    }
    const planned = this.searchRefusals.shift();
    if (planned !== undefined) {
      return new Response(
        JSON.stringify({ error: { code: planned.code, message: planned.code, request_id: 'fake-request' } }),
        {
          status: planned.status,
          headers: { 'Content-Type': 'application/json', 'Cache-Control': 'no-store', 'Retry-After': '30' },
        },
      );
    }
    const window = Math.floor(now / 60_000);
    const counted = this.searches.get(caller.id);
    const count = counted?.window === window ? counted.count : 0;
    if (count >= 30) {
      return new Response(
        JSON.stringify({ error: { code: 'rate_limited', message: 'rate_limited', request_id: 'fake-request' } }),
        {
          status: 429,
          headers: { 'Content-Type': 'application/json', 'Cache-Control': 'no-store', 'Retry-After': '60' },
        },
      );
    }
    this.searches.set(caller.id, { window, count: count + 1 });
    const folded = term.toLocaleLowerCase();
    const matching = channel.messages
      .filter(
        (stored) =>
          !stored.message.deleted &&
          !stored.message.erased &&
          stored.message.body !== null &&
          stored.message.body.toLocaleLowerCase().includes(folded) &&
          (cursor === null || stored.message.seq < cursor),
      )
      .sort((left, right) => right.message.seq - left.message.seq);
    const page = matching.slice(0, 30);
    const response = answer(200, {
      items: page.map((stored) => this.messageFor(stored, caller, channel)),
      next_cursor: matching.length > 30 ? String(page.at(-1)?.message.seq) : null,
    });
    const pause = this.searchPause;
    this.searchPause = null;
    if (pause !== null) {
      pause.answered();
      await pause.resumed;
    }
    return response;
  }

  private async sendMessage(channel: StoredChannel, caller: Me, request: Request, now: number): Promise<Response> {
    const key = request.headers.get('Idempotency-Key') ?? '';
    const body: unknown = await request.json();
    if (
      !validBody(body) ||
      !('attachment_ids' in body) ||
      !Array.isArray(body.attachment_ids) ||
      !body.attachment_ids.every((id: unknown): id is string => typeof id === 'string')
    )
      return refused(422, 'validation_failed');
    const ids = body.attachment_ids;
    const replyTo = replyTarget(body);
    if (replyTo === undefined) return refused(422, 'validation_failed');
    this.sends.push({ body: body.body, ids: [...ids], key, replyTo });
    if (!/^[A-Za-z0-9._-]{1,64}$/.test(key)) {
      return refused(422, 'validation_failed');
    }
    const previous = channel.messages.find((stored) => stored.senderId === caller.id && stored.key === key);
    if (previous !== undefined) {
      return answer(201, this.messageFor(previous, caller, channel));
    }
    if (replyTo !== null) {
      const quoted = channel.messages.find((entry) => entry.message.id === replyTo);
      if (quoted === undefined || quoted.message.deleted || quoted.message.erased)
        return refusedAt(404, 'not_found', 'body.reply_to_message_id');
    }
    if (!channel.channel.writable) {
      return refused(409, 'channel_read_only');
    }
    if (
      (!acceptableText(body.body) && !(body.body === '' && ids.length > 0)) ||
      ids.length > 10 ||
      new Set(ids).size !== ids.length
    )
      return refused(422, 'validation_failed');
    const attachments: components['schemas']['MessageAttachment'][] = [];
    for (const id of ids) {
      const upload = this.uploads.get(id);
      if (upload?.caller !== caller.id || upload.channel !== channel.channel.id || upload.bound)
        return refused(404, 'not_found');
      attachments.push(upload.file);
    }
    for (const id of ids) {
      const upload = this.uploads.get(id);
      if (upload === undefined) throw new Error('resolved upload disappeared');
      upload.bound = true;
    }
    this.issued += 1;
    const stored: StoredMessage = {
      key,
      senderId: caller.id,
      ...(replyTo === null ? {} : { replyTo }),
      message: {
        id: `00000000-0000-4000-9000-${String(this.issued).padStart(12, '0')}`,
        body: body.body,
        attachments,
        created_at: new Date(now).toISOString(),
        edited_at: null,
        deleted: false,
        erased: false,
        own: true,
        sender_label: caller.name,
        sender_label_kind: null,
        sent_before_reclaim: false,
        reply_to: null,
        reactions: [],
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
    this.expose(channel, member, stored.message.seq);
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
    if (new URL(request.url).pathname.endsWith('/reaction')) {
      return this.react(channel, stored, caller, request, now);
    }
    if (request.method === 'DELETE' && (stored.senderId === caller.id || caller.role === 'tutor')) {
      if (this.failDelete) {
        this.failDelete = false;
        return refused(500, 'internal_error');
      }
      stored.message = { ...stored.message, body: null, attachments: [], deleted: true };
      stored.reactions?.clear();
      if (channel.pinned === stored.message.id) channel.pinned = null;
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
