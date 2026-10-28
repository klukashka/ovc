import { type ApiClient, type ApiError, apiErrorFrom } from '../../lib/api/client.ts';
import { type components, groupColourValues } from '../../lib/api/schema.ts';

export type Group = components['schemas']['Group'];
export type GroupDetail = components['schemas']['GroupDetail'];
export type GroupSummary = components['schemas']['GroupSummary'];
export type GroupMember = components['schemas']['GroupMember'];
export type GroupColour = Group['colour'];
export type StudentCardPage = components['schemas']['StudentCardPage'];

export const groupColours: readonly GroupColour[] = groupColourValues;

export const groupMemberLimit = 30;

export const groupLimit = 200;

export const groupsKey = ['groups'] as const;

export const createdNotice = 'created';

export function groupKey(id: string): readonly unknown[] {
  return [...groupsKey, 'group', id];
}

export const memberChoiceKey = ['group-member-choice'] as const;

export function activeStudentsKey(search: string): readonly unknown[] {
  return [...memberChoiceKey, search];
}

export function isGroupColour(value: string): value is GroupColour {
  return groupColours.some((colour) => colour === value);
}

export function firstUnusedColour(groups: readonly GroupSummary[]): GroupColour {
  const used = new Set(groups.filter((group) => group.archived_at === null).map((group) => group.colour));
  return groupColours.find((colour) => !used.has(colour)) ?? 'clay';
}

export function groupCapReached(error: ApiError): boolean {
  return error.fieldCode('tutor.groups') === 'too_long';
}

export function memberCapReached(error: ApiError): boolean {
  return error.fieldCode('add') === 'too_long' || error.fieldCode('member_ids') === 'invalid';
}

export async function fetchGroups(client: ApiClient): Promise<readonly GroupSummary[]> {
  const { data, error, response } = await client.GET('/api/v1/groups');
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data.groups;
}

export async function fetchGroup(client: ApiClient, id: string): Promise<GroupDetail> {
  const { data, error, response } = await client.GET('/api/v1/groups/{id}', { params: { path: { id } } });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

export async function fetchActiveStudentsPage(
  client: ApiClient,
  search: string,
  cursor: string | null,
): Promise<StudentCardPage> {
  const { data, error, response } = await client.GET('/api/v1/students', {
    params: {
      query: {
        state: 'active',
        ...(search === '' ? {} : { q: search }),
        ...(cursor === null ? {} : { cursor }),
      },
    },
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}
