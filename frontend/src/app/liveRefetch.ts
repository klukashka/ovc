import type { QueryClient, QueryKey } from '@tanstack/react-query';

import { groupsKey, memberChoiceKey } from '../features/groups/api.ts';
import { notificationsKey } from '../features/notifications/api.ts';
import { calendarKey, lessonKey } from '../features/schedule/keys.ts';
import { joinRequestsKey, studentCardKey, studentCardsKey, studentListsKey } from '../features/students/api.ts';
import { myTutorsKey, sentRequestsKey } from '../features/tutors/api.ts';
import type { RealtimeEvent } from '../lib/realtime/client.ts';
import type { EntityKind } from '../lib/realtime/frames.ts';
import type { NotificationArrivals } from './notificationArrivals.ts';

function queriesOf(entity: EntityKind, id: string): readonly QueryKey[] {
  switch (entity) {
    case 'relationship':
      return [studentListsKey, studentCardKey(id), groupsKey, memberChoiceKey, myTutorsKey];
    case 'join_request':
      return [joinRequestsKey, sentRequestsKey];
    case 'notification':
      return [notificationsKey];
    case 'lesson':
      return [calendarKey, lessonKey(id), studentListsKey, studentCardsKey, groupsKey];
  }
}

export function liveRefetch(
  queryClient: QueryClient,
  arrivals: Pick<NotificationArrivals, 'arrived'>,
): (event: RealtimeEvent) => void {
  return (event) => {
    switch (event.kind) {
      case 'connected':
        void queryClient.invalidateQueries({
          predicate: (query) =>
            query.state.status === 'success' ||
            query.state.fetchStatus === 'fetching' ||
            (event.reconnect && query.state.status === 'error'),
        });
        return;
      case 'notification':
        arrivals.arrived();
        void queryClient.invalidateQueries({ queryKey: notificationsKey });
        return;
      case 'entity':
        for (const queryKey of queriesOf(event.entity, event.id)) {
          void queryClient.invalidateQueries({ queryKey });
        }
    }
  };
}
