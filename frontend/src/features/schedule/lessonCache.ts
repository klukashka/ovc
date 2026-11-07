import type { QueryClient } from '@tanstack/react-query';

import type { LessonDetail } from './api.ts';
import { lessonKey } from './keys.ts';

export async function replaceLessonDetail(queryClient: QueryClient, detail: LessonDetail): Promise<void> {
  const key = lessonKey(detail.id);
  await queryClient.cancelQueries({ queryKey: key });
  queryClient.setQueryData(key, detail);
}
