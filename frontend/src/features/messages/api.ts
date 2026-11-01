import { type ApiClient, apiErrorFrom } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';

export type Channel = components['schemas']['ChannelDetail'];
export type ChannelItem = components['schemas']['ChannelItem'];
export type ChannelPage = components['schemas']['ChannelPage'];
export type Message = components['schemas']['Message'];
export type MessagePage = components['schemas']['MessagePage'];

export async function fetchChannels(
  client: ApiClient,
  q: string,
  cursor: string | null,
  signal: AbortSignal,
): Promise<ChannelPage> {
  const { data, error, response } = await client.GET('/api/v1/channels', {
    signal,
    params: { query: { q, ...(cursor === null ? {} : { cursor }) } },
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

export async function sendMessage(
  client: ApiClient,
  id: string,
  body: string,
  key: string,
  attachments: readonly string[] = [],
): Promise<Message> {
  const { data, error, response } = await client.POST('/api/v1/channels/{id}/messages', {
    params: { path: { id }, header: { 'Idempotency-Key': key } },
    body: { body, attachment_ids: [...attachments] },
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
