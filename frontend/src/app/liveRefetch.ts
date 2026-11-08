import type { QueryClient, QueryKey } from '@tanstack/react-query';

import { type Scheduler, browserScheduler } from '../lib/api/session.ts';
import { evictChannel } from '../features/messages/cache.ts';
import { chatRefetch } from '../features/messages/refetch.ts';

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
      return [studentListsKey, studentCardKey(id), groupsKey, memberChoiceKey, myTutorsKey, calendarKey];
    case 'join_request':
      return [joinRequestsKey, sentRequestsKey];
    case 'notification':
      return [notificationsKey];
    case 'lesson':
      return [calendarKey, lessonKey(id), studentListsKey, studentCardsKey, groupsKey];
    case 'channel':
      return [calendarKey];
  }
}

export function liveRefetch(
  queryClient: QueryClient,
  arrivals: Pick<NotificationArrivals, 'arrived'>,
  scheduler: Scheduler = browserScheduler,
): ((event: RealtimeEvent) => void) & {
  readonly cancel: () => void;
  readonly suspend: () => void;
  readonly resume: () => void;
} {
  const coalescer = chatRefetch(queryClient, scheduler);
  const cancel = coalescer.cancel;
  const onEvent = (event: RealtimeEvent): void => {
    switch (event.kind) {
      case 'connected':
        cancel();
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
      case 'message':
        coalescer.changed(event.channelId, event);
        return;
      case 'read':
        coalescer.read();
        return;
      case 'entity':
        if (event.entity === 'channel') {
          evictChannel(queryClient, event.id, true);
          coalescer.changed(event.id);
        }
        for (const queryKey of queriesOf(event.entity, event.id)) {
          void queryClient.invalidateQueries({ queryKey });
        }
    }
  };
  return Object.assign(onEvent, { cancel, suspend: coalescer.suspend, resume: coalescer.resume });
}
