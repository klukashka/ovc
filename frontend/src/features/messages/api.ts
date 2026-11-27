import { retryAfterSeconds } from '../../components/ui/Form.tsx';
import { type ApiClient, type ApiError, apiErrorFrom } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';

import type { ChannelFilter } from './filters.ts';

export type Channel = components['schemas']['ChannelDetail'];
export type ChannelItem = components['schemas']['ChannelItem'];
export type ChannelPage = components['schemas']['ChannelPage'];
export type Message = components['schemas']['Message'];
export type MessagePage = components['schemas']['MessagePage'];
export type MessageQuote = components['schemas']['MessageQuote'];
export type PinnedMessage = components['schemas']['PinnedMessage'];
export type SearchPage = components['schemas']['SearchPage'];
export type MessageReaction = components['schemas']['MessageReaction'];
export type ReactionValue = MessageReaction['reaction'];

export async function fetchChannels(
  client: ApiClient,
  q: string,
  cursor: string | null,
  signal: AbortSignal,
  filter: ChannelFilter = 'all',
): Promise<ChannelPage> {
  const { data, error, response } = await client.GET('/api/v1/channels', {
    signal,
    params: { query: { q, ...(filter === 'all' ? {} : { filter }), ...(cursor === null ? {} : { cursor }) } },
  });
  if (data === undefined) throw apiErrorFrom(response.status, error);
  return data;
}

export async function fetchChannel(client: ApiClient, id: string, signal: AbortSignal): Promise<Channel> {
  const { data, error, response } = await client.GET('/api/v1/channels/{id}', { signal, params: { path: { id } } });
  if (data === undefined) throw apiErrorFrom(response.status, error);
  return data;
}

export async function fetchMessages(
  client: ApiClient,
  id: string,
  cursor: string | null,
  signal: AbortSignal,
): Promise<MessagePage> {
  const { data, error, response } = await client.GET('/api/v1/channels/{id}/messages', {
    signal,
    params: { path: { id }, query: cursor === null ? {} : { cursor } },
  });
  if (data === undefined) throw apiErrorFrom(response.status, error);
  return data;
}

export async function fetchMessagesAfter(
  client: ApiClient,
  id: string,
  after: number,
  limit: number,
  signal: AbortSignal,
): Promise<MessagePage> {
  const { data, error, response } = await client.GET('/api/v1/channels/{id}/messages', {
    signal,
    params: { path: { id }, query: { after_seq: String(after), limit } },
  });
  if (data === undefined) throw apiErrorFrom(response.status, error);
  return data;
}

export async function sendMessage(
  client: ApiClient,
  id: string,
  body: string,
  key: string,
  attachments: readonly string[] = [],
  replyTo: string | null = null,
): Promise<Message> {
  const { data, error, response } = await client.POST('/api/v1/channels/{id}/messages', {
    params: { path: { id }, header: { 'Idempotency-Key': key } },
    body: { body, attachment_ids: [...attachments], ...(replyTo === null ? {} : { reply_to_message_id: replyTo }) },
  });
  if (data === undefined) throw apiErrorFrom(response.status, error);
  return data;
}

export async function editMessage(client: ApiClient, message: Message, body: string): Promise<Message> {
  const { data, error, response } = await client.PATCH('/api/v1/messages/{id}', {
    params: { path: { id: message.id } },
    body: { body, version: message.version },
  });
  if (data === undefined) throw apiErrorFrom(response.status, error);
  return data;
}

export async function deleteMessage(client: ApiClient, id: string): Promise<void> {
  const { error, response } = await client.DELETE('/api/v1/messages/{id}', { params: { path: { id } } });
  if (!response.ok) throw apiErrorFrom(response.status, error);
}

export async function markRead(client: ApiClient, id: string, seq: number, signal: AbortSignal): Promise<void> {
  const { error, response } = await client.POST('/api/v1/channels/{id}/read', {
    signal,
    params: { path: { id } },
    body: { seq },
  });
  if (!response.ok) throw apiErrorFrom(response.status, error);
}

export async function fetchMessagesAround(
  client: ApiClient,
  id: string,
  seq: number,
  limit: number,
  signal: AbortSignal,
): Promise<MessagePage> {
  const { data, error, response } = await client.GET('/api/v1/channels/{id}/messages', {
    signal,
    params: { path: { id }, query: { around_seq: String(seq), limit } },
  });
  if (data === undefined) throw apiErrorFrom(response.status, error);
  return data;
}

export class SearchRefusal extends Error {
  readonly refusal: ApiError;
  readonly retryAfter: number | null;

  constructor(refusal: ApiError, retryAfter: number | null) {
    super(refusal.message);
    this.refusal = refusal;
    this.retryAfter = retryAfter;
  }
}

export async function searchMessages(
  client: ApiClient,
  id: string,
  q: string,
  cursor: string | null,
  signal: AbortSignal,
): Promise<SearchPage> {
  const { data, error, response } = await client.POST('/api/v1/channels/{id}/messages/search', {
    signal,
    params: { path: { id } },
    body: cursor === null ? { q } : { q, cursor },
  });
  if (data === undefined) throw new SearchRefusal(apiErrorFrom(response.status, error), retryAfterSeconds(response));
  return data;
}

export async function setReaction(client: ApiClient, id: string, reaction: ReactionValue): Promise<void> {
  const { error, response } = await client.PUT('/api/v1/messages/{id}/reaction', {
    params: { path: { id } },
    body: { reaction },
  });
  if (!response.ok) throw apiErrorFrom(response.status, error);
}

export async function removeReaction(client: ApiClient, id: string): Promise<void> {
  const { error, response } = await client.DELETE('/api/v1/messages/{id}/reaction', { params: { path: { id } } });
  if (!response.ok) throw apiErrorFrom(response.status, error);
}

export async function pinMessage(client: ApiClient, id: string, message: string): Promise<void> {
  const { error, response } = await client.PUT('/api/v1/channels/{id}/pin', {
    params: { path: { id } },
    body: { message_id: message },
  });
  if (!response.ok) throw apiErrorFrom(response.status, error);
}

export async function unpinMessage(client: ApiClient, id: string): Promise<void> {
  const { error, response } = await client.DELETE('/api/v1/channels/{id}/pin', { params: { path: { id } } });
  if (!response.ok) throw apiErrorFrom(response.status, error);
}
