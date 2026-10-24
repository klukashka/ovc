import { type ApiClient, apiErrorFrom } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';

export type MyTutor = components['schemas']['MyTutor'];
export type SentJoinRequest = components['schemas']['SentJoinRequest'];
export type SentJoinRequestPage = components['schemas']['SentJoinRequestPage'];

export const myTutorsKey = ['tutors'] as const;

export const sentRequestsKey = ['join-requests', 'sent'] as const;

export async function fetchMyTutors(client: ApiClient, signal: AbortSignal): Promise<readonly MyTutor[]> {
  const { data, error, response } = await client.GET('/api/v1/tutors', { signal });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data.tutors;
}

export async function fetchSentRequestsPage(
  client: ApiClient,
  cursor: string | null,
  signal: AbortSignal,
): Promise<SentJoinRequestPage> {
  const { data, error, response } = await client.GET('/api/v1/join-requests/sent', {
    params: { query: cursor === null ? {} : { cursor } },
    signal,
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

export function currentTutors(tutors: readonly MyTutor[]): readonly MyTutor[] {
  return tutors.filter((tutor) => tutor.state !== 'ended');
}

export function pastTutors(tutors: readonly MyTutor[]): readonly MyTutor[] {
  return tutors.filter((tutor) => tutor.state === 'ended');
}

export function requestsToShow(requests: readonly SentJoinRequest[]): readonly SentJoinRequest[] {
  return requests.filter((request) => request.state !== 'accepted');
}
