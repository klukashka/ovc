import { type FieldProblem, fakeRefusal } from './fakeSubmissions.ts';
import type { components } from './schema.ts';

type Group = components['schemas']['Group'];
type GroupDetail = components['schemas']['GroupDetail'];
type GroupMember = components['schemas']['GroupMember'];
type CalendarItem = components['schemas']['CalendarItem'];
type Colour = Group['colour'];

export interface GroupCandidate {
  readonly active: boolean;
  readonly displayName: string;
  readonly name: string;
  readonly avatarId: string | null;
}

export type GroupCandidateLookup = (tutorId: string, relationshipId: string) => GroupCandidate | null;

export interface FakeGroupSeed {
  readonly name: string;
  readonly colour?: Colour;
  readonly relationshipIds: readonly string[];
  readonly futureLessonCount?: number;
  readonly hasOpenSeries?: boolean;
  readonly archived?: boolean;
  readonly channelId?: string | null;
}

interface StoredMember {
  readonly relationshipId: string;
  readonly addedAt: string;
}

interface StoredGroup {
  readonly id: string;
  readonly tutorId: string;
  readonly name: string;
  readonly colour: Colour;
  readonly archivedAt: string | null;
  readonly members: readonly StoredMember[];
  readonly channelId: string | null;
  readonly nextLessons: readonly CalendarItem[];
  readonly futureLessonCount: number;
  readonly hasOpenSeries: boolean;
}

interface RequestedChange {
  readonly name: string | null;
  readonly colour: string | null;
  readonly add: readonly string[];
  readonly remove: readonly string[];
}

type GroupWrite = 'change' | 'archive' | 'unarchive';

const groupPathPrefix = '/api/v1/groups/';
const groupRoute = /^\/api\/v1\/groups\/[^/]+(?:\/(?:archive|unarchive))?$/;
const palette: readonly Colour[] = ['clay', 'slate', 'moss', 'plum', 'ochre', 'teal', 'rust', 'ink'];
const groupMemberLimit = 30;
const groupNameLimit = 60;
const nextLessonLimit = 5;
const controlCharacter = /\p{Cc}/u;
const dayMilliseconds = 24 * 60 * 60 * 1_000;
const lessonMilliseconds = 60 * 60 * 1_000;

function answer(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' },
  });
}

function notActiveRefusal(relationshipIds: readonly string[]): Response {
  return answer(409, {
    error: {
      code: 'relationship_not_active',
      message: 'relationship_not_active',
      request_id: 'fake-request',
      details: { relationship_ids: relationshipIds },
    },
  });
}

function colourOf(value: string | null): Colour | null {
  return palette.find((colour) => colour === value) ?? null;
}

function distinct(ids: readonly string[]): string[] {
  return [...new Set(ids)].sort();
}

function stringList(value: unknown): string[] | null {
  if (value === undefined) {
    return [];
  }
  if (!Array.isArray(value)) {
    return null;
  }
  const entries: unknown[] = value;
  const texts = entries.filter((entry) => typeof entry === 'string');
  return texts.length === entries.length ? texts : null;
}

function optionalText(value: unknown): string | null | undefined {
  if (value === undefined) {
    return null;
  }
  return typeof value === 'string' ? value : undefined;
}

function requestedChange(body: unknown): RequestedChange | null {
  if (typeof body !== 'object' || body === null || Array.isArray(body)) {
    return null;
  }
  const name = optionalText('name' in body ? body.name : undefined);
  const colour = optionalText('colour' in body ? body.colour : undefined);
  const add = stringList('add' in body ? body.add : undefined);
  const remove = stringList('remove' in body ? body.remove : undefined);
  if (name === undefined || colour === undefined || add === null || remove === null) {
    return null;
  }
  return { name, colour, add, remove };
}

function validGroupName(name: string): boolean {
  const trimmed = name.trim();
  return trimmed !== '' && Array.from(trimmed).length <= groupNameLimit && !controlCharacter.test(trimmed);
}

function changeProblems(change: RequestedChange, add: readonly string[], remove: readonly string[]): FieldProblem[] {
  const problems: FieldProblem[] = [];
  if (change.name !== null && !validGroupName(change.name)) {
    problems.push({ path: 'body.name', code: 'invalid' });
  }
  if (change.colour !== null && colourOf(change.colour) === null) {
    problems.push({ path: 'body.colour', code: 'invalid' });
  }
  if (remove.some((id) => add.includes(id))) {
    problems.push({ path: 'body.remove', code: 'invalid' });
  }
  return problems;
}

function writeOf(method: string, action: string | undefined): GroupWrite | null {
  if (method === 'PATCH' && action === undefined) {
    return 'change';
  }
  if (method === 'POST' && (action === 'archive' || action === 'unarchive')) {
    return action;
  }
  return null;
}

async function jsonBody(request: Request): Promise<{ readonly parsed: boolean; readonly body: unknown }> {
  const text = await request.text();
  try {
    const body: unknown = JSON.parse(text);
    return { parsed: true, body };
  } catch {
    return { parsed: false, body: null };
  }
}

export class FakeGroupServer {
  private readonly candidateOf: GroupCandidateLookup;
  private readonly groups = new Map<string, StoredGroup>();
  private created = 0;
  private lessonIds = 0;
  private lostWrites = 0;
  private failingWrites = 0;

  constructor(candidateOf: GroupCandidateLookup) {
    this.candidateOf = candidateOf;
  }

  loseNextWrite(): void {
    this.lostWrites += 1;
  }

  failNextWrite(): void {
    this.failingWrites += 1;
  }

  seed(tutorId: string, seed: FakeGroupSeed, now: number): string {
    this.created += 1;
    const id = `00000000-0000-4000-b000-${String(this.created).padStart(12, '0')}`;
    const colour = seed.colour ?? 'moss';
    const futureLessonCount = seed.futureLessonCount ?? 0;
    const addedAt = new Date(now).toISOString();
    this.groups.set(id, {
      id,
      tutorId,
      name: seed.name,
      colour,
      archivedAt: seed.archived === true ? addedAt : null,
      members: [...new Set(seed.relationshipIds)].map((relationshipId) => ({ relationshipId, addedAt })),
      channelId: seed.channelId ?? null,
      nextLessons: Array.from({ length: Math.min(futureLessonCount, nextLessonLimit) }, (_, index) =>
        this.lessonOf(seed.name, colour, now + (index + 1) * dayMilliseconds),
      ),
      futureLessonCount,
      hasOpenSeries: seed.hasOpenSeries ?? false,
    });
    return id;
  }

  memberIdsOf(groupId: string): readonly string[] {
    return (this.groups.get(groupId)?.members ?? []).map((member) => member.relationshipId);
  }

  removeRelationship(relationshipId: string): void {
    for (const group of this.groups.values()) {
      this.groups.set(group.id, {
        ...group,
        members: group.members.filter((member) => member.relationshipId !== relationshipId),
      });
    }
  }

  handles(path: string): boolean {
    return groupRoute.test(path);
  }

  async handle(request: Request, tutorId: string, now: number): Promise<Response> {
    const route = new URL(request.url).pathname.slice(groupPathPrefix.length);
    const [encodedId = '', action] = route.split('/');
    const id = decodeURIComponent(encodedId);
    if (request.method === 'GET' && action === undefined) {
      const group = this.ownGroup(tutorId, id);
      return group === null ? fakeRefusal(404, 'not_found') : answer(200, this.detailOf(group));
    }
    const write = writeOf(request.method, action);
    if (write === null) {
      return fakeRefusal(405, 'method_not_allowed');
    }
    if (this.failingWrites > 0) {
      this.failingWrites -= 1;
      return fakeRefusal(503, 'service_unavailable');
    }
    const lost = this.lostWrites > 0;
    if (lost) {
      this.lostWrites -= 1;
    }
    const written = await this.write(write, request, tutorId, id, now);
    if (lost) {
      throw new TypeError('the connection was lost before the group write answered');
    }
    return written;
  }

  private async write(
    write: GroupWrite,
    request: Request,
    tutorId: string,
    id: string,
    now: number,
  ): Promise<Response> {
    const group = this.ownGroup(tutorId, id);
    if (group === null) {
      return fakeRefusal(404, 'not_found');
    }
    if (write === 'archive') {
      return this.store(group.archivedAt === null ? this.archived(group, now) : group);
    }
    if (write === 'unarchive') {
      return this.store({ ...group, archivedAt: null });
    }
    const { parsed, body } = await jsonBody(request);
    if (!parsed) {
      return fakeRefusal(400, 'bad_request');
    }
    const change = requestedChange(body);
    return change === null
      ? fakeRefusal(422, 'validation_failed', [{ path: 'body', code: 'invalid' }])
      : this.change(group, change, now);
  }

  private change(group: StoredGroup, change: RequestedChange, now: number): Response {
    const add = distinct(change.add);
    const remove = distinct(change.remove);
    const outside = distinct([...add, ...remove]).filter(
      (relationshipId) => this.candidateOf(group.tutorId, relationshipId) === null,
    );
    if (outside.length > 0) {
      return fakeRefusal(404, 'not_found');
    }
    if (add.length > 0 && group.archivedAt !== null) {
      return fakeRefusal(409, 'audience_not_active');
    }
    const inactive = add.filter((relationshipId) => this.candidateOf(group.tutorId, relationshipId)?.active !== true);
    if (inactive.length > 0) {
      return notActiveRefusal(inactive);
    }
    const problems = changeProblems(change, add, remove);
    if (problems.length > 0) {
      return fakeRefusal(422, 'validation_failed', problems);
    }
    const kept = group.members.filter((member) => !remove.includes(member.relationshipId));
    const joining = add.filter((relationshipId) => !kept.some((member) => member.relationshipId === relationshipId));
    if (kept.length + joining.length > groupMemberLimit) {
      return fakeRefusal(422, 'validation_failed', [{ path: 'body.add', code: 'too_long' }]);
    }
    const addedAt = new Date(now).toISOString();
    return this.store({
      ...group,
      name: change.name === null ? group.name : change.name.trim(),
      colour: colourOf(change.colour) ?? group.colour,
      members: [...kept, ...joining.map((relationshipId) => ({ relationshipId, addedAt }))],
    });
  }

  private archived(group: StoredGroup, now: number): StoredGroup {
    return {
      ...group,
      archivedAt: new Date(now).toISOString(),
      nextLessons: [],
      futureLessonCount: 0,
      hasOpenSeries: false,
    };
  }

  private store(group: StoredGroup): Response {
    this.groups.set(group.id, group);
    return answer(200, this.viewOf(group));
  }

  private ownGroup(tutorId: string, id: string): StoredGroup | null {
    const group = this.groups.get(id);
    return group?.tutorId === tutorId ? group : null;
  }

  private viewOf(group: StoredGroup): Group {
    return {
      id: group.id,
      name: group.name,
      colour: group.colour,
      archived_at: group.archivedAt,
      members: this.membersOf(group),
    };
  }

  private detailOf(group: StoredGroup): GroupDetail {
    return {
      ...this.viewOf(group),
      channel_id: group.channelId,
      next_lessons: [...group.nextLessons],
      future_lesson_count: group.futureLessonCount,
      has_open_series: group.hasOpenSeries,
    };
  }

  private membersOf(group: StoredGroup): GroupMember[] {
    return group.members.flatMap((member): GroupMember[] => {
      const candidate = this.candidateOf(group.tutorId, member.relationshipId);
      return candidate === null
        ? []
        : [
            {
              relationship_id: member.relationshipId,
              display_name: candidate.displayName,
              name: candidate.name,
              avatar_id: candidate.avatarId,
              added_at: member.addedAt,
            },
          ];
    });
  }

  private lessonOf(groupName: string, colour: Colour, startsAt: number): CalendarItem {
    this.lessonIds += 1;
    return {
      id: `00000000-0000-4000-b100-${String(this.lessonIds).padStart(12, '0')}`,
      title: 'Group lesson',
      starts_at: new Date(startsAt).toISOString(),
      ends_at: new Date(startsAt + lessonMilliseconds).toISOString(),
      status: 'scheduled',
      took_place: false,
      timezone: 'Europe/Moscow',
      audience: { kind: 'group', label: groupName, label_kind: null, colour },
      tutor: null,
    };
  }
}
