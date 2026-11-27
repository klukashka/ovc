import type { Locator, Page } from '@playwright/test';

import type { TestAccount } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { expectRefetchedAfterTheSocketIsReady, trafficAfterReady } from './realtime.ts';
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

export async function postMessage(
  deployment: Deployment,
  account: TestAccount,
  channel: string,
  text: string,
  replyTo: string | null,
): Promise<string> {
  const response = await callApi(
    deployment,
    await accessToken(deployment, account),
    'POST',
    `/channels/${channel}/messages`,
    {
      body: text,
      attachment_ids: [],
      ...(replyTo === null ? {} : { reply_to_message_id: replyTo }),
    },
  );
  const body = response.body;
  if (
    response.status !== 201 ||
    typeof body !== 'object' ||
    body === null ||
    !('id' in body) ||
    typeof body.id !== 'string'
  ) {
    throw new Error(`posting a message answered ${String(response.status)}`);
  }
  return body.id;
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

export function messages(page: Page): Locator {
  return page.getByRole('list', { name: 'Messages', exact: true }).getByRole('listitem');
}

function conversationQueryPaths(channel: string): readonly string[] {
  return [`/api/v1/channels/${channel}`, `/api/v1/channels/${channel}/messages`];
}

export async function openSettledConversation(
  page: Page,
  channel: string,
  layout: 'with the chat list' | 'conversation only',
): Promise<void> {
  const paths = conversationQueryPaths(channel);
  const traffic = trafficAfterReady(page, layout === 'with the chat list' ? [...paths, '/api/v1/channels'] : paths);
  await traffic.initialize();
  await page.goto(`/messages/${channel}`);
  await expectRefetchedAfterTheSocketIsReady(traffic);
}

export async function settledComposer(page: Page): Promise<void> {
  await page.waitForFunction(() => document.querySelector('textarea')?.readOnly === false);
}

export async function send(page: Page, body: string): Promise<void> {
  await page.getByRole('textbox', { name: 'Message', exact: true }).fill(body);
  const saved = page.waitForResponse(
    (response) =>
      response.request().method() === 'POST' && /\/channels\/[^/]+\/messages$/.test(new URL(response.url()).pathname),
  );
  await page.getByRole('button', { name: 'Send', exact: true }).click();
  const response = await saved;
  if (response.status() !== 201) {
    throw new Error(`sending a message answered ${String(response.status())}`);
  }
  await settledComposer(page);
}
