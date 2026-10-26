import type { Locator, Page } from '@playwright/test';

import type { TestAccount } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { accessToken, callApi } from './tutoring.ts';

export interface NotificationRow {
  readonly event: string;
  readonly label: string | null;
  readonly unread: boolean;
}

function isNotificationItem(
  value: unknown,
): value is { readonly event: string; readonly label: string | null; readonly read_at: string | null } {
  return (
    typeof value === 'object' &&
    value !== null &&
    'event' in value &&
    typeof value.event === 'string' &&
    'label' in value &&
    (value.label === null || typeof value.label === 'string') &&
    'read_at' in value &&
    (value.read_at === null || typeof value.read_at === 'string')
  );
}

export async function notificationRows(deployment: Deployment, account: TestAccount): Promise<NotificationRow[]> {
  const answer = await callApi(deployment, await accessToken(deployment, account), 'GET', '/notifications');
  const body = answer.body;
  if (answer.status !== 200 || typeof body !== 'object' || body === null || !('items' in body)) {
    throw new Error(`listing notifications answered ${String(answer.status)}`);
  }
  const items: unknown = body.items;
  if (!Array.isArray(items) || !items.every(isNotificationItem)) {
    throw new Error('the notifications are not a list of rows');
  }
  return items.map((item) => ({ event: item.event, label: item.label, unread: item.read_at === null }));
}

export function bellIn(page: Page, name: RegExp): Locator {
  return page.getByRole('navigation').getByRole('button', { name });
}

export function panelRowsIn(page: Page, heading: string): Locator {
  return page.getByRole('region', { name: heading }).getByRole('list', { name: heading }).getByRole('listitem');
}
