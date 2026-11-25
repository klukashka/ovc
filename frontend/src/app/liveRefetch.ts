import type { Query, QueryClient, QueryKey } from '@tanstack/react-query';

import { type Scheduler, browserScheduler } from '../lib/api/session.ts';
import { evictChannel } from '../features/messages/cache.ts';
import { chatRefetch } from '../features/messages/refetch.ts';

import { assignmentsKey } from '../features/assignment/keys.ts';
import { groupsKey, memberChoiceKey } from '../features/groups/api.ts';
import { homeKey, tutorHomeKey } from '../features/home/api.ts';
import { issuesKey } from '../features/homework/api.ts';
import { notificationsKey } from '../features/notifications/api.ts';
import { currentExportKey } from '../features/settings/exportApi.ts';
import { reviewKey } from '../features/review/keys.ts';
import { calendarKey, lessonKey, lessonsKey } from '../features/schedule/keys.ts';
import { joinRequestsKey, studentCardKey, studentCardsKey, studentListsKey } from '../features/students/api.ts';
import { myTutorsKey, sentRequestsKey } from '../features/tutors/api.ts';
import type { RealtimeEvent } from '../lib/realtime/client.ts';
import type { EntityKind } from '../lib/realtime/frames.ts';
import type { NotificationArrivals } from './notificationArrivals.ts';

function queriesOf(entity: EntityKind, id: string): readonly QueryKey[] {
  switch (entity) {
    case 'relationship':
      return [studentListsKey, studentCardKey(id), groupsKey, memberChoiceKey, myTutorsKey, calendarKey, homeKey];
    case 'join_request':
      return [joinRequestsKey, sentRequestsKey, tutorHomeKey];
    case 'notification':
      return [notificationsKey];
    case 'lesson':
      return [calendarKey, lessonKey(id), studentListsKey, studentCardsKey, groupsKey, homeKey];
    case 'channel':
      return [calendarKey, homeKey];
    case 'issue':
      return [tutorHomeKey];
    case 'submission':
      return [homeKey];
  }
}

type HomeworkEvent = Extract<RealtimeEvent, { readonly kind: 'entity' }>;

function inFamily(key: QueryKey, family: readonly unknown[], detail: string, id: string): boolean {
  return key[0] === family[0] && (key[1] !== detail || key[2] === id);
}

function homeworkQuery(event: HomeworkEvent, key: QueryKey): boolean {
  if (key[0] === lessonsKey[0]) {
    return typeof key[1] === 'string' && (event.lessonIds ?? []).includes(key[1]);
  }
  if (event.entity === 'issue') {
    return inFamily(key, issuesKey, 'detail', event.id);
  }
  if (event.entity !== 'submission') {
    return false;
  }
  return (
    inFamily(key, assignmentsKey, 'submission', event.id) ||
    inFamily(key, reviewKey, 'submission', event.id) ||
    (event.issueId !== undefined && inFamily(key, issuesKey, 'detail', event.issueId))
  );
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
        void queryClient.invalidateQueries({ queryKey: currentExportKey });
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
        if (event.entity === 'issue' || event.entity === 'submission') {
          void queryClient.invalidateQueries({ predicate: (query: Query) => homeworkQuery(event, query.queryKey) });
        }
    }
  };
  return Object.assign(onEvent, { cancel, suspend: coalescer.suspend, resume: coalescer.resume });
}
