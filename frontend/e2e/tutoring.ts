import { type TestAccount, confirmationToken, newTestAccount, testNetAddress } from './accounts.ts';
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
  method: 'GET' | 'POST' | 'DELETE',
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
  const token = new URL(link).pathname.split('/').at(-1) ?? '';
  const answer = await callApi(deployment, null, 'GET', `/invitations/${encodeURIComponent(token)}`);
  return answer.status === 200;
}

async function signUpStudentWithCode(
  deployment: Deployment,
  control: TestControl,
  student: TestAccount,
  code: string,
): Promise<void> {
  const registered = await callApi(deployment, null, 'POST', '/auth/register', {
    role: 'student',
    name: student.name,
    email: student.email,
    password: student.password,
    language: 'ru',
    timezone: 'Europe/Moscow',
    consent: true,
    tutor_code: code,
  });
  expectStatus(registered, 202, 'student sign-up');
  await control.tick();
  const token = confirmationToken(await control.mail(student.email));
  const confirmed = await callApi(deployment, await accessToken(deployment, student), 'POST', '/auth/verify-email', {
    token,
  });
  expectStatus(confirmed, 200, 'confirmation in the student’s own browser');
}

export async function seedBlockedStudent(
  deployment: Deployment,
  control: TestControl,
  tutor: TestAccount,
  name: string,
): Promise<TestAccount> {
  const student = newTestAccount(name, 'student');
  await signUpStudentWithCode(deployment, control, student, await tutorCode(deployment, tutor));
  const tutorToken = await accessToken(deployment, tutor);
  const requests = await callApi(deployment, tutorToken, 'GET', '/join-requests');
  expectStatus(requests, 200, 'listing join requests');
  const items: unknown = field(requests.body, 'items');
  const listed: readonly unknown[] = Array.isArray(items) ? items : [];
  const request = listed.find((item) => text(field(item, 'student'), 'email') === student.email);
  if (request === undefined) {
    throw new Error('the student’s join request is not visible to the tutor');
  }
  const declined = await callApi(deployment, tutorToken, 'POST', `/join-requests/${text(request, 'id')}/decline`, {
    block: true,
  });
  expectStatus(declined, 200, 'declining with a block');
  return student;
}

export async function seedActiveStudent(
  deployment: Deployment,
  control: TestControl,
  tutor: TestAccount,
  displayName: string,
): Promise<{ readonly student: TestAccount; readonly cardId: string }> {
  const student = newTestAccount(displayName, 'student');
  const tutorToken = await accessToken(deployment, tutor);
  const created = await callApi(deployment, tutorToken, 'POST', '/students', {
    display_name: displayName,
    email: student.email,
  });
  expectStatus(created, 201, 'creating a card');
  const cardId = text(created.body, 'id');
  const link = text(field(created.body, 'invitation'), 'url');
  const token = new URL(link).pathname.split('/').at(-1) ?? '';
  const accepted = await callApi(deployment, null, 'POST', `/invitations/${encodeURIComponent(token)}/accept`, {
    name: student.name,
    email: student.email,
    password: student.password,
    language: 'en',
    timezone: 'Europe/Moscow',
    consent: true,
  });
  expectStatus(accepted, 202, 'sign-up on the invitation');
  await control.tick();
  const confirmation = confirmationToken(await control.mail(student.email));
  const confirmed = await callApi(deployment, await accessToken(deployment, student), 'POST', '/auth/verify-email', {
    token: confirmation,
  });
  expectStatus(confirmed, 200, 'confirmation in the student’s own browser');
  return { student, cardId };
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
