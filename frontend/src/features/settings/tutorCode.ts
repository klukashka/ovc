import { type ApiClient, apiErrorFrom } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';

export type TutorCode = components['schemas']['TutorCode'];

export const tutorCodeKey = ['tutor', 'code'] as const;

export async function fetchTutorCode(client: ApiClient, signal: AbortSignal): Promise<TutorCode> {
  const { data, error, response } = await client.GET('/api/v1/tutor/code', { signal });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}
