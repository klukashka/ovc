import type { TestAccount } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { accessToken, callApi } from './tutoring.ts';

export async function channelOf(deployment: Deployment, account: TestAccount, resource: string): Promise<string> {
  const response = await callApi(deployment, await accessToken(deployment, account), 'GET', resource);
  const body = response.body;
  if (
    response.status !== 200 ||
    typeof body !== 'object' ||
    body === null ||
    !('channel_id' in body) ||
    typeof body.channel_id !== 'string'
  ) {
    throw new Error('the resource has no accessible channel');
  }
  return body.channel_id;
}

export async function channelUnread(deployment: Deployment, account: TestAccount, channel: string): Promise<number> {
  const response = await callApi(deployment, await accessToken(deployment, account), 'GET', `/channels/${channel}`);
  const body = response.body;
  if (
    response.status !== 200 ||
    typeof body !== 'object' ||
    body === null ||
    !('unread_count' in body) ||
    typeof body.unread_count !== 'number'
  ) {
    throw new Error('the channel has no unread count');
  }
  return body.unread_count;
}
