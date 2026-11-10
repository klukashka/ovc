import { CancelledError, QueryClient } from '@tanstack/react-query';
import { expect, test } from 'vitest';

import type { LessonDetail } from './api.ts';
import { lessonKey } from './keys.ts';
import { replaceLessonDetail } from './lessonCache.ts';

const liveLesson: LessonDetail = {
  id: '00000000-0000-4000-8000-000000000004',
  title: 'Разговорная практика',
  description: null,
  starts_at: '2026-11-03T15:00:00Z',
  ends_at: '2026-11-03T16:00:00Z',
  duration_minutes: 60,
  status: 'live',
  took_place: true,
  timezone: 'Europe/Moscow',
  series_id: null,
  series: null,
  version: 2,
  cancelled_at: null,
  other_timezone: null,
  audience: { kind: 'one_to_one', label: 'Анна', label_kind: null, colour: 'moss', members: null },
  tutor: null,
  channel_id: null,
  room_open: true,
  embed_allowed: false,
  materials: [],
};

const endedLesson: LessonDetail = { ...liveLesson, status: 'ended', room_open: false, version: 3 };

function pausedRead(): { readonly answer: Promise<LessonDetail>; readonly release: () => void } {
  let release = (): void => undefined;
  const answer = new Promise<LessonDetail>((resolve) => {
    release = (): void => {
      resolve(liveLesson);
    };
  });
  return { answer, release };
}

test('a lesson read that started before the replacement cannot bring the older detail back', async () => {
  const queryClient = new QueryClient();
  const read = pausedRead();
  const reading = queryClient.query({ queryKey: lessonKey(liveLesson.id), queryFn: () => read.answer }).then(
    (detail): unknown => detail,
    (failure: unknown): unknown => failure,
  );

  await replaceLessonDetail(queryClient, endedLesson);
  read.release();
  await read.answer;
  const staleOutcome = await reading;

  expect(staleOutcome).toBeInstanceOf(CancelledError);
  expect(queryClient.getQueryData(lessonKey(liveLesson.id))).toEqual(endedLesson);
  expect(queryClient.getQueryState(lessonKey(liveLesson.id))?.fetchStatus).toBe('idle');
});
