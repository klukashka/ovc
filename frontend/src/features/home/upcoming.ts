import { type UseQueryResult, useQuery } from '@tanstack/react-query';

import { useLocalToday } from '../../app/clock.ts';
import { useApiClient } from '../../lib/api/client.ts';
import type { Me } from '../../lib/api/session.ts';
import { type LocalDate, addDays, localMidnight } from '../../lib/time/localDates.ts';
import { type Calendar, calendarRangeKey, fetchCalendar } from '../schedule/api.ts';

const upcomingDays = 62;

export interface UpcomingLessons {
  readonly today: LocalDate;
  readonly query: UseQueryResult<Calendar>;
}

export function useUpcomingLessons(me: Me, tutor: string | null, enabled = true): UpcomingLessons {
  const client = useApiClient();
  const today = useLocalToday(me.timezone);
  const range = {
    from: localMidnight(today, me.timezone),
    to: localMidnight(addDays(today, upcomingDays), me.timezone),
  };
  const query = useQuery({
    queryKey: calendarRangeKey(range, tutor),
    queryFn: () => fetchCalendar(client, range, tutor),
    enabled,
    retry: false,
  });
  return { today, query };
}
