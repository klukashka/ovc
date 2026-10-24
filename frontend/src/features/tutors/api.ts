import { type ApiClient, apiErrorFrom } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';

export type MyTutor = components['schemas']['MyTutor'];

export const myTutorsKey = ['tutors'] as const;

export async function fetchMyTutors(client: ApiClient, signal: AbortSignal): Promise<readonly MyTutor[]> {
  const { data, error, response } = await client.GET('/api/v1/tutors', { signal });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data.tutors;
}

export function currentTutors(tutors: readonly MyTutor[]): readonly MyTutor[] {
  return tutors.filter((tutor) => tutor.state !== 'ended');
}
