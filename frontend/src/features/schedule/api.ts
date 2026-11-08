import { type ApiClient, apiErrorFrom } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import type { CalendarRange } from './calendarTime.ts';
import { calendarKey } from './keys.ts';

export type Calendar = components['schemas']['Calendar'];
export type CalendarItem = components['schemas']['CalendarItem'];
export type CalendarAudience = components['schemas']['CalendarAudience'];
export type LessonStatus = CalendarItem['status'];
export type NewLesson = components['schemas']['NewLesson'];

export type CalendarSpan = Pick<CalendarRange, 'from' | 'to'>;

export function calendarRangeKey(range: CalendarSpan, tutor: string | null): readonly unknown[] {
  return [...calendarKey, range.from, range.to, tutor];
}

export async function fetchCalendar(client: ApiClient, range: CalendarSpan, tutor: string | null): Promise<Calendar> {
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

export type LessonDetail = components['schemas']['LessonDetail'];
export type LessonSeries = components['schemas']['LessonSeries'];
export type LessonPatch = components['schemas']['LessonPatch'];
export type LessonScope = LessonPatch['scope'];
export type ChangedLessons = components['schemas']['CreatedLessons'];

export async function fetchLesson(client: ApiClient, id: string): Promise<LessonDetail> {
  const { data, error, response } = await client.GET('/api/v1/lessons/{id}', { params: { path: { id } } });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

export const lessonDurationLimits = { minimum: 15, maximum: 240, step: 5 } as const;

export const seriesCountLimit = 100;
