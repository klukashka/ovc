import {
  type TestAccount,
  confirmationToken,
  newTestAccount,
  seedAccount,
  seededSession,
  testNetAddress,
} from './accounts.ts';
import type { Deployment } from './deployment.ts';
import type { TestControl } from './fixtures.ts';

export interface ApiAnswer {
  readonly status: number;
  readonly body: unknown;
}

function headers(deployment: Deployment, token: string | null): Record<string, string> {
  return {
    'Content-Type': 'application/json',
    Origin: deployment.origin,
    'X-Forwarded-For': testNetAddress(),
    ...(token === null ? {} : { Authorization: `Bearer ${token}` }),
  };
}

export async function callApi(
  deployment: Deployment,
  token: string | null,
  method: 'GET' | 'POST' | 'PATCH' | 'DELETE',
  path: string,
  body?: unknown,
): Promise<ApiAnswer> {
  const response = await fetch(`${deployment.apiUrl}/api/v1${path}`, {
    signal: AbortSignal.timeout(10_000),
    method,
    headers: headers(deployment, token),
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  const text = await response.text();
  return { status: response.status, body: text === '' ? null : (JSON.parse(text) as unknown) };
}

function field(value: unknown, name: string): unknown {
  if (typeof value !== 'object' || value === null || !(name in value)) {
    throw new Error(`the answer has no ${name}`);
  }
  return (value as Record<string, unknown>)[name];
}

function text(value: unknown, name: string): string {
  const found = field(value, name);
  if (typeof found !== 'string') {
    throw new Error(`${name} is not a string`);
  }
  return found;
}

function expectStatus(answer: ApiAnswer, status: number, what: string): void {
  if (answer.status !== status) {
    throw new Error(`${what} answered ${String(answer.status)}`);
  }
}

export async function accessToken(deployment: Deployment, account: TestAccount): Promise<string> {
  if (account.seededOn !== null) {
    return (await seededSession(account.seededOn, account)).accessToken;
  }
  const answer = await callApi(deployment, null, 'POST', '/auth/login', {
    email: account.email,
    password: account.password,
  });
  expectStatus(answer, 200, 'sign-in');
  return text(answer.body, 'access_token');
}

export async function tutorCode(deployment: Deployment, tutor: TestAccount): Promise<string> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'GET', '/tutor/code');
  expectStatus(answer, 200, 'reading the tutor code');
  return text(answer.body, 'code');
}

export async function invitationIsUsable(deployment: Deployment, link: string): Promise<boolean> {
  const token = invitationToken(link);
  const answer = await callApi(deployment, null, 'GET', `/invitations/${encodeURIComponent(token)}`);
  return answer.status === 200;
}

export function invitationToken(link: string): string {
  return new URL(link).pathname.split('/').at(-1) ?? '';
}

export interface CreatedCard {
  readonly id: string;
  readonly link: string;
}

export async function createCard(
  deployment: Deployment,
  tutor: TestAccount,
  displayName: string,
  email: string | null,
): Promise<CreatedCard> {
  const created = await callApi(deployment, await accessToken(deployment, tutor), 'POST', '/students', {
    display_name: displayName,
    ...(email === null ? {} : { email }),
  });
  expectStatus(created, 201, 'creating a card');
  return { id: text(created.body, 'id'), link: text(field(created.body, 'invitation'), 'url') };
}

export async function sendInvitation(deployment: Deployment, tutor: TestAccount, cardId: string): Promise<void> {
  const sent = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'POST',
    `/students/${encodeURIComponent(cardId)}/invitation/send`,
  );
  expectStatus(sent, 202, 'sending the invitation');
}

export async function signUpOnInvitation(
  deployment: Deployment,
  link: string,
  student: TestAccount,
  language: 'ru' | 'en',
): Promise<void> {
  const accepted = await callApi(
    deployment,
    null,
    'POST',
    `/invitations/${encodeURIComponent(invitationToken(link))}/accept`,
    {
      name: student.name,
      email: student.email,
      password: student.password,
      language,
      timezone: 'Europe/Moscow',
      consent: true,
    },
  );
  expectStatus(accepted, 202, 'sign-up on the invitation');
}

export async function myTutorNames(deployment: Deployment, student: TestAccount): Promise<string[]> {
  const answer = await callApi(deployment, await accessToken(deployment, student), 'GET', '/tutors');
  expectStatus(answer, 200, 'listing the tutors');
  const tutors: unknown = field(answer.body, 'tutors');
  if (!Array.isArray(tutors)) {
    throw new Error('the tutors are not a list');
  }
  return tutors.map((tutor: unknown) => text(field(tutor, 'tutor'), 'name'));
}

export async function setCardNotes(
  deployment: Deployment,
  tutor: TestAccount,
  cardId: string,
  notes: string,
): Promise<void> {
  const patched = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'PATCH',
    `/students/${encodeURIComponent(cardId)}`,
    { notes },
  );
  expectStatus(patched, 200, 'saving the notes');
}

export async function removeStudent(deployment: Deployment, tutor: TestAccount, cardId: string): Promise<void> {
  const removed = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'POST',
    `/students/${encodeURIComponent(cardId)}/remove`,
  );
  expectStatus(removed, 200, 'removing the student');
}

export async function enterTutorCode(deployment: Deployment, student: TestAccount, code: string): Promise<void> {
  const entered = await callApi(deployment, await accessToken(deployment, student), 'POST', '/join-requests', { code });
  expectStatus(entered, 200, 'entering the tutor code');
}

export async function visibleJoinRequests(deployment: Deployment, tutor: TestAccount): Promise<number> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'GET', '/join-requests');
  expectStatus(answer, 200, 'listing join requests');
  const items: unknown = field(answer.body, 'items');
  if (!Array.isArray(items)) {
    throw new Error('the join requests are not a list');
  }
  return items.length;
}

export async function registerStudentWithCode(
  deployment: Deployment,
  student: TestAccount,
  code: string,
  language: 'ru' | 'en' = 'ru',
): Promise<void> {
  const registered = await callApi(deployment, null, 'POST', '/auth/register', {
    role: 'student',
    name: student.name,
    email: student.email,
    password: student.password,
    language,
    timezone: 'Europe/Moscow',
    consent: true,
    tutor_code: code,
  });
  expectStatus(registered, 202, 'student sign-up');
}

async function seedStudentWithRequest(deployment: Deployment, name: string, code: string): Promise<TestAccount> {
  const student = await seedAccount(deployment, name, 'student', 'ru');
  await enterTutorCode(deployment, student, code);
  return student;
}

async function confirmByMail(deployment: Deployment, control: TestControl, student: TestAccount): Promise<void> {
  const token = confirmationToken(await control.mail(student.email));
  const confirmed = await callApi(deployment, await accessToken(deployment, student), 'POST', '/auth/verify-email', {
    token,
  });
  expectStatus(confirmed, 200, 'confirmation in the student’s own browser');
}

export type RequestDecision = 'accept' | 'decline' | 'block';

async function decideRequestOf(
  deployment: Deployment,
  tutorToken: string,
  studentEmail: string,
  decision: RequestDecision,
): Promise<string> {
  const requests = await callApi(deployment, tutorToken, 'GET', '/join-requests?limit=100');
  expectStatus(requests, 200, 'listing join requests');
  const items: unknown = field(requests.body, 'items');
  const listed: readonly unknown[] = Array.isArray(items) ? items : [];
  const request = listed.find((item) => text(field(item, 'student'), 'email') === studentEmail);
  if (request === undefined) {
    throw new Error('the student’s join request is not visible to the tutor');
  }
  const id = text(request, 'id');
  const decided =
    decision === 'accept'
      ? await callApi(deployment, tutorToken, 'POST', `/join-requests/${id}/accept`)
      : await callApi(deployment, tutorToken, 'POST', `/join-requests/${id}/decline`, { block: decision === 'block' });
  expectStatus(decided, 200, `deciding a join request (${decision})`);
  return id;
}

export async function decideJoinRequest(
  deployment: Deployment,
  tutor: TestAccount,
  studentEmail: string,
  decision: RequestDecision,
): Promise<string> {
  return decideRequestOf(deployment, await accessToken(deployment, tutor), studentEmail, decision);
}

async function declineWithBlock(deployment: Deployment, tutorToken: string, student: TestAccount): Promise<void> {
  await decideRequestOf(deployment, tutorToken, student.email, 'block');
}

async function tutorCodeWith(deployment: Deployment, tutorToken: string): Promise<string> {
  const answer = await callApi(deployment, tutorToken, 'GET', '/tutor/code');
  expectStatus(answer, 200, 'reading the tutor code');
  return text(answer.body, 'code');
}

export async function seedBlockedStudent(
  deployment: Deployment,
  tutor: TestAccount,
  name: string,
): Promise<TestAccount> {
  const tutorToken = await accessToken(deployment, tutor);
  const student = await seedStudentWithRequest(deployment, name, await tutorCodeWith(deployment, tutorToken));
  await declineWithBlock(deployment, tutorToken, student);
  return student;
}

async function seedRequesters(
  deployment: Deployment,
  code: string,
  count: number,
  prefix: string,
): Promise<readonly TestAccount[]> {
  return Promise.all(
    Array.from({ length: count }, (_, index) =>
      seedStudentWithRequest(deployment, `${prefix} ${String(index + 1).padStart(2, '0')}`, code),
    ),
  );
}

export async function seedBlockedStudents(deployment: Deployment, tutor: TestAccount, count: number): Promise<void> {
  const tutorToken = await accessToken(deployment, tutor);
  const students = await seedRequesters(deployment, await tutorCodeWith(deployment, tutorToken), count, 'Blocked');
  for (const student of students) {
    await declineWithBlock(deployment, tutorToken, student);
  }
}

export async function seedJoinRequests(
  deployment: Deployment,
  tutor: TestAccount,
  count: number,
): Promise<readonly TestAccount[]> {
  return seedRequesters(deployment, await tutorCode(deployment, tutor), count, 'Requester');
}

const codeEntriesPerHour = 10;

export async function seedDeclinedRequests(
  deployment: Deployment,
  control: TestControl,
  student: TestAccount,
  tutor: TestAccount,
  count: number,
): Promise<void> {
  const code = await tutorCode(deployment, tutor);
  for (let entered = 0; entered < count; entered += codeEntriesPerHour) {
    if (entered > 0) {
      await control.advance(3601);
    }
    const studentToken = await accessToken(deployment, student);
    const tutorToken = await accessToken(deployment, tutor);
    for (let index = entered; index < Math.min(count, entered + codeEntriesPerHour); index += 1) {
      const sent = await callApi(deployment, studentToken, 'POST', '/join-requests', { code });
      expectStatus(sent, 200, 'entering the tutor code');
      await decideRequestOf(deployment, tutorToken, student.email, 'decline');
    }
  }
}

export async function seedActiveStudent(
  deployment: Deployment,
  tutor: TestAccount,
  displayName: string,
  language: 'ru' | 'en' = 'en',
): Promise<{ readonly student: TestAccount; readonly cardId: string }> {
  const student = await seedAccount(deployment, displayName, 'student', language);
  const card = await createCard(deployment, tutor, displayName, student.email);
  await acceptSignedIn(deployment, student, card.link);
  return { student, cardId: card.id };
}

export async function signUpActiveStudentOnInvitation(
  deployment: Deployment,
  control: TestControl,
  tutor: TestAccount,
  displayName: string,
  language: 'ru' | 'en' = 'en',
): Promise<{ readonly student: TestAccount; readonly cardId: string }> {
  const student = newTestAccount(displayName, 'student');
  const card = await createCard(deployment, tutor, displayName, student.email);
  await signUpOnInvitation(deployment, card.link, student, language);
  await confirmInTheOwnBrowser(deployment, control, student);
  return { student, cardId: card.id };
}

export async function confirmInTheOwnBrowser(
  deployment: Deployment,
  control: TestControl,
  student: TestAccount,
): Promise<void> {
  await control.tick();
  await confirmByMail(deployment, control, student);
}

export async function seedInvitedCards(deployment: Deployment, tutor: TestAccount, count: number): Promise<void> {
  const tutorToken = await accessToken(deployment, tutor);
  for (let index = 1; index <= count; index += 1) {
    const created = await callApi(deployment, tutorToken, 'POST', '/students', {
      display_name: `Invited ${String(index).padStart(2, '0')}`,
    });
    expectStatus(created, 201, 'creating a card');
  }
}

export async function endRelationship(deployment: Deployment, tutor: TestAccount, cardId: string): Promise<void> {
  const ended = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'POST',
    `/students/${encodeURIComponent(cardId)}/end`,
  );
  expectStatus(ended, 200, 'ending the relationship');
}

export async function acceptSignedIn(deployment: Deployment, student: TestAccount, link: string): Promise<void> {
  const accepted = await callApi(
    deployment,
    await accessToken(deployment, student),
    'POST',
    `/invitations/${encodeURIComponent(invitationToken(link))}/accept`,
    {},
  );
  expectStatus(accepted, 200, 'signed-in acceptance');
}

export async function deleteAccount(deployment: Deployment, account: TestAccount): Promise<void> {
  const deleted = await callApi(deployment, await accessToken(deployment, account), 'DELETE', '/users/me', {
    password: account.password,
  });
  expectStatus(deleted, 204, 'deleting the account');
}

async function acceptRequestOf(deployment: Deployment, tutorToken: string, studentEmail: string): Promise<string> {
  const requests = await callApi(deployment, tutorToken, 'GET', '/join-requests?limit=100');
  expectStatus(requests, 200, 'listing join requests');
  const items: unknown = field(requests.body, 'items');
  const listed: readonly unknown[] = Array.isArray(items) ? items : [];
  const request = listed.find((item) => text(field(item, 'student'), 'email') === studentEmail);
  if (request === undefined) {
    throw new Error('the student’s join request is not visible to the tutor');
  }
  const accepted = await callApi(deployment, tutorToken, 'POST', `/join-requests/${text(request, 'id')}/accept`);
  expectStatus(accepted, 200, 'accepting a join request');
  return text(accepted.body, 'id');
}

export async function seedStudentThroughCode(
  deployment: Deployment,
  tutor: TestAccount,
  name: string,
  language: 'ru' | 'en' = 'en',
): Promise<{ readonly student: TestAccount; readonly cardId: string }> {
  const tutorToken = await accessToken(deployment, tutor);
  const student = await seedAccount(deployment, name, 'student', language);
  await enterTutorCode(deployment, student, await tutorCodeWith(deployment, tutorToken));
  return { student, cardId: await acceptRequestOf(deployment, tutorToken, student.email) };
}

export async function signUpStudentThroughCode(
  deployment: Deployment,
  control: TestControl,
  tutor: TestAccount,
  name: string,
  language: 'ru' | 'en' = 'en',
): Promise<{ readonly student: TestAccount; readonly cardId: string }> {
  const tutorToken = await accessToken(deployment, tutor);
  const student = newTestAccount(name, 'student');
  await registerStudentWithCode(deployment, student, await tutorCodeWith(deployment, tutorToken), language);
  await control.tick();
  await confirmByMail(deployment, control, student);
  return { student, cardId: await acceptRequestOf(deployment, tutorToken, student.email) };
}

export async function createGroup(
  deployment: Deployment,
  tutor: TestAccount,
  name: string,
  memberIds: readonly string[],
): Promise<string> {
  const created = await callApi(deployment, await accessToken(deployment, tutor), 'POST', '/groups', {
    name,
    colour: 'moss',
    member_ids: memberIds,
  });
  expectStatus(created, 201, 'creating a group');
  return text(created.body, 'id');
}

export async function groupMemberNames(deployment: Deployment, tutor: TestAccount, groupId: string): Promise<string[]> {
  const answer = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'GET',
    `/groups/${encodeURIComponent(groupId)}`,
  );
  expectStatus(answer, 200, 'reading the group');
  const members: unknown = field(answer.body, 'members');
  if (!Array.isArray(members)) {
    throw new Error('the members are not a list');
  }
  return members.map((member: unknown) => text(member, 'display_name'));
}

export interface CardState {
  readonly state: string;
  readonly groups: readonly string[];
}

export async function cardState(deployment: Deployment, tutor: TestAccount, cardId: string): Promise<CardState> {
  const answer = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'GET',
    `/students/${encodeURIComponent(cardId)}`,
  );
  expectStatus(answer, 200, 'reading the card');
  const groups: unknown = field(answer.body, 'groups');
  if (!Array.isArray(groups)) {
    throw new Error('the card groups are not a list');
  }
  return { state: text(answer.body, 'state'), groups: groups.map((group: unknown) => text(group, 'name')) };
}

export async function cardStatesNamed(
  deployment: Deployment,
  tutor: TestAccount,
  displayName: string,
): Promise<string[]> {
  const answer = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'GET',
    `/students?q=${encodeURIComponent(displayName)}&limit=100`,
  );
  expectStatus(answer, 200, 'searching the cards');
  const items: unknown = field(answer.body, 'items');
  if (!Array.isArray(items)) {
    throw new Error('the cards are not a list');
  }
  return items.map((item: unknown) => text(item, 'state'));
}

export async function seedActiveStudentsThroughCode(
  deployment: Deployment,
  tutor: TestAccount,
  count: number,
): Promise<readonly string[]> {
  const tutorToken = await accessToken(deployment, tutor);
  const students = await seedRequesters(deployment, await tutorCodeWith(deployment, tutorToken), count, 'Student');
  const cardIds: string[] = [];
  for (const student of students) {
    cardIds.push(await acceptRequestOf(deployment, tutorToken, student.email));
  }
  return cardIds;
}

export async function seedEmptyGroups(deployment: Deployment, tutor: TestAccount, count: number): Promise<void> {
  const tutorToken = await accessToken(deployment, tutor);
  const batch = 20;
  for (let first = 0; first < count; first += batch) {
    const created = await Promise.all(
      Array.from({ length: Math.min(batch, count - first) }, (_, index) =>
        callApi(deployment, tutorToken, 'POST', '/groups', {
          name: `Group ${String(first + index + 1).padStart(3, '0')}`,
          colour: 'slate',
          member_ids: [],
        }),
      ),
    );
    for (const answer of created) {
      expectStatus(answer, 201, 'creating a group');
    }
  }
}

export async function addToGroup(
  deployment: Deployment,
  tutor: TestAccount,
  groupId: string,
  cardIds: readonly string[],
): Promise<void> {
  const changed = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'PATCH',
    `/groups/${encodeURIComponent(groupId)}`,
    { add: cardIds },
  );
  expectStatus(changed, 200, 'adding to the group');
}

export async function archiveGroup(deployment: Deployment, tutor: TestAccount, groupId: string): Promise<void> {
  const archived = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'POST',
    `/groups/${encodeURIComponent(groupId)}/archive`,
  );
  expectStatus(archived, 200, 'archiving the group');
}

export async function unarchiveGroup(deployment: Deployment, tutor: TestAccount, groupId: string): Promise<void> {
  const unarchived = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'POST',
    `/groups/${encodeURIComponent(groupId)}/unarchive`,
  );
  expectStatus(unarchived, 200, 'unarchiving the group');
}
