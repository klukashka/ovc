import { useQuery } from '@tanstack/react-query';

import { fetchMyTutors, myTutorsKey } from '../features/tutors/api.ts';
import { useApiClient } from '../lib/api/client.ts';
import type { Me } from '../lib/api/session.ts';

export type ProvisionalLink =
  | { readonly kind: 'notNeeded' }
  | { readonly kind: 'pending' }
  | { readonly kind: 'failed'; readonly retry: () => void }
  | { readonly kind: 'known'; readonly linked: boolean };

export function useProvisionalLink(me: Me | null): ProvisionalLink {
  const client = useApiClient();
  const needed = me !== null && me.role === 'student' && me.state === 'unverified';
  const tutors = useQuery({
    queryKey: myTutorsKey,
    queryFn: () => fetchMyTutors(client),
    enabled: needed,
    retry: false,
  });
  if (!needed) {
    return { kind: 'notNeeded' };
  }
  if (tutors.data !== undefined) {
    return { kind: 'known', linked: tutors.data.some((tutor) => tutor.state === 'provisional') };
  }
  if (tutors.isError) {
    return {
      kind: 'failed',
      retry: (): void => {
        void tutors.refetch();
      },
    };
  }
  return { kind: 'pending' };
}

export function provisionallyLinked(link: ProvisionalLink): boolean {
  return link.kind === 'known' && link.linked;
}
