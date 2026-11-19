import { type UseQueryResult, useQuery } from '@tanstack/react-query';

import { type ApiClient, apiErrorFrom, useApiClient } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';

export type Home = components['schemas']['Home'];
export type TutorHome = components['schemas']['TutorHome'];
export type StudentHome = components['schemas']['StudentHome'];
export type StudentHomeTutor = components['schemas']['StudentHomeTutor'];
export type TutorHomeChecklist = components['schemas']['TutorHomeChecklist'];

export const homeKey = ['home'] as const;

export const tutorHomeKey = [...homeKey, 'tutor'] as const;

export const studentHomesKey = [...homeKey, 'student'] as const;

export function studentHomeKey(tutor: string | null): readonly unknown[] {
  return [...studentHomesKey, tutor];
}

class HomeRoleMismatch extends Error {
  override readonly name = 'HomeRoleMismatch';
}

function isTutorHome(home: Home): home is TutorHome {
  return 'checklist' in home && !('todo' in home);
}

function isStudentHome(home: Home): home is StudentHome {
  return 'todo' in home && !('checklist' in home);
}

async function fetchHome(client: ApiClient, tutor: string | null): Promise<Home> {
  const { data, error, response } = await client.GET('/api/v1/home', {
    params: { query: tutor === null ? {} : { tutor } },
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

async function fetchTutorHome(client: ApiClient): Promise<TutorHome> {
  const home = await fetchHome(client, null);
  if (!isTutorHome(home)) {
    throw new HomeRoleMismatch('the home answer is not the tutor part');
  }
  return home;
}

async function fetchStudentHome(client: ApiClient, tutor: string | null): Promise<StudentHome> {
  const home = await fetchHome(client, tutor);
  if (!isStudentHome(home)) {
    throw new HomeRoleMismatch('the home answer is not the student part');
  }
  return home;
}

export function useTutorHome(): UseQueryResult<TutorHome> {
  const client = useApiClient();
  return useQuery({
    queryKey: tutorHomeKey,
    queryFn: () => fetchTutorHome(client),
    structuralSharing: false,
    retry: false,
  });
}

export function useStudentHomeRead(tutor: string | null): UseQueryResult<StudentHome> {
  const client = useApiClient();
  return useQuery({
    queryKey: studentHomeKey(tutor),
    queryFn: () => fetchStudentHome(client, tutor),
    retry: false,
  });
}

export function shownCount(count: number): string {
  return count > 99 ? '99+' : String(count);
}
