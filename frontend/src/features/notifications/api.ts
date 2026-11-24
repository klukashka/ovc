import { type ApiClient, apiErrorFrom } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';

export type Notification = components['schemas']['Notification'];
export type NotificationEvent = Notification['event'];
export type NotificationLink = components['schemas']['NotificationLink'];
export type NotificationPage = components['schemas']['NotificationPage'];

export const notificationsKey = ['notifications'] as const;
export const latestNotificationsKey = ['notifications', 'latest'] as const;
export const notificationPagesKey = ['notifications', 'pages'] as const;

export const panelRows = 10;
export const unreadCountCap = 99;

export async function fetchNotifications(
  client: ApiClient,
  limit: number | null,
  cursor: string | null,
): Promise<NotificationPage> {
  const { data, error, response } = await client.GET('/api/v1/notifications', {
    params: { query: { ...(limit === null ? {} : { limit }), ...(cursor === null ? {} : { cursor }) } },
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

export type ReadSelection = { readonly ids: readonly string[] } | { readonly through: string };

export async function markNotificationsRead(client: ApiClient, selection: ReadSelection): Promise<number> {
  const body = 'ids' in selection ? { ids: [...selection.ids] } : { through: selection.through };
  const { data, error, response } = await client.POST('/api/v1/notifications/read', { body });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data.unread_count;
}

export function linkAddress(link: NotificationLink): string {
  switch (link.kind) {
    case 'student_card':
      return `/students/${encodeURIComponent(link.id)}`;
    case 'join_requests':
      return '/students';
    case 'my_tutors':
      return '/tutors';
    case 'lesson':
      return `/lessons/${encodeURIComponent(link.id)}`;
    case 'assignment':
      return `/homework/${encodeURIComponent(link.id)}`;
    case 'review':
      return `/homework/review/${encodeURIComponent(link.id)}`;
    case 'my_data':
      return '/settings/data';
  }
}
