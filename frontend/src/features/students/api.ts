import { type ApiClient, apiErrorFrom } from '../../lib/api/client.ts';
import { type components, pathsApiV1StudentsGetParametersQueryStateValues } from '../../lib/api/schema.ts';

export type StudentCard = components['schemas']['StudentCard'];
export type StudentCardDetail = components['schemas']['StudentCardDetail'];
export type CardLessonPage = components['schemas']['CardLessonPage'];
export type StudentCardSummary = components['schemas']['StudentCardSummary'];
export type StudentCardPage = components['schemas']['StudentCardPage'];
export type CardInvitation = components['schemas']['CardInvitation'];
export type RelationshipState = StudentCard['state'];
export type StudentListState = (typeof pathsApiV1StudentsGetParametersQueryStateValues)[number];

export const studentListStates: readonly StudentListState[] = pathsApiV1StudentsGetParametersQueryStateValues;

export const studentsKey = ['students'] as const;

export const joinRequestsKey = ['join-requests', 'received'] as const;

export const withdrawnNotice = 'withdrawn';

export const removedNotice = 'removed';

export function hasNotice(state: unknown, notice: string): boolean {
  return typeof state === 'object' && state !== null && 'notice' in state && state.notice === notice;
}

export const studentListsKey = [...studentsKey, 'list'] as const;

export const firstStudentCardKey = [...studentListsKey, 'first-card'] as const;

export function studentListKey(state: StudentListState, search: string, group: string): readonly unknown[] {
  return [...studentListsKey, state, search, group];
}

export const studentCardsKey = [...studentsKey, 'card'] as const;

export function studentCardKey(id: string): readonly unknown[] {
  return [...studentCardsKey, id];
}

export function cardLessonsKey(id: string): readonly unknown[] {
  return [...studentCardKey(id), 'lessons'];
}

export function isStudentListState(value: string | null): value is StudentListState {
  return studentListStates.some((state) => state === value);
}

export async function fetchStudentsPage(
  client: ApiClient,
  filter: {
    readonly state: StudentListState;
    readonly search: string;
    readonly group: string;
    readonly cursor: string | null;
  },
): Promise<StudentCardPage> {
  const { data, error, response } = await client.GET('/api/v1/students', {
    params: {
      query: {
        state: filter.state,
        ...(filter.search === '' ? {} : { q: filter.search }),
        ...(filter.group === '' ? {} : { group: filter.group }),
        ...(filter.cursor === null ? {} : { cursor: filter.cursor }),
      },
    },
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

export async function fetchStudentCard(client: ApiClient, id: string): Promise<StudentCardDetail> {
  const { data, error, response } = await client.GET('/api/v1/students/{id}', {
    params: { path: { id } },
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

export async function fetchCardLessons(client: ApiClient, id: string, cursor: string | null): Promise<CardLessonPage> {
  const { data, error, response } = await client.GET('/api/v1/students/{id}/lessons', {
    params: { path: { id }, query: cursor === null ? {} : { cursor } },
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}
