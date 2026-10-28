import { type ApiClient, apiErrorFrom } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import type { CalendarRange } from './calendarTime.ts';
import { calendarKey } from './keys.ts';

export type Calendar = components['schemas']['Calendar'];
export type CalendarItem = components['schemas']['CalendarItem'];
export type CalendarAudience = components['schemas']['CalendarAudience'];
export type LessonStatus = CalendarItem['status'];
export type NewLesson = components['schemas']['NewLesson'];

export function calendarRangeKey(range: CalendarRange, tutor: string | null): readonly unknown[] {
  return [...calendarKey, range.from, range.to, tutor];
}

export async function fetchCalendar(client: ApiClient, range: CalendarRange, tutor: string | null): Promise<Calendar> {
  const { data, error, response } = await client.GET('/api/v1/calendar', {
    params: {
      query: {
        from: new Date(range.from).toISOString(),
        to: new Date(range.to).toISOString(),
        ...(tutor === null ? {} : { tutor }),
      },
    },
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

export const lessonDurationLimits = { minimum: 15, maximum: 240, step: 5 } as const;

export const seriesCountLimit = 100;
