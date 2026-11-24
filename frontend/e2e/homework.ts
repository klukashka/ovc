import { randomUUID } from 'node:crypto';

import { type TestAccount, testNetAddress } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { accessToken } from './tutoring.ts';

export interface SavedTemplate {
  readonly id: string;
  readonly version: number;
}

export type IssueAudienceSeed = { readonly relationship_id: string } | { readonly group_id: string };

export type AnswerPolicy = 'never' | 'after_submission' | 'after_check';

export interface IssueSeed {
  readonly lessonId?: string;
  readonly deadlineAt?: string;
  readonly policy?: AnswerPolicy;
}

function field(value: unknown, name: string): unknown {
  if (typeof value !== 'object' || value === null || !(name in value)) {
    throw new Error(`the answer has no ${name}`);
  }
  return (value as Record<string, unknown>)[name];
}

function textField(value: unknown, name: string): string {
  const found = field(value, name);
  if (typeof found !== 'string') {
    throw new Error(`${name} is not a string`);
  }
  return found;
}

function numberField(value: unknown, name: string): number {
  const found = field(value, name);
  if (typeof found !== 'number') {
    throw new Error(`${name} is not a number`);
  }
  return found;
}

async function send(
  deployment: Deployment,
  tutor: TestAccount,
  method: 'GET' | 'POST' | 'PUT' | 'PATCH',
  path: string,
  expected: number,
  body?: unknown,
): Promise<unknown> {
  const response = await fetch(`${deployment.apiUrl}/api/v1${path}`, {
    signal: AbortSignal.timeout(10_000),
    method,
    headers: {
      'Content-Type': 'application/json',
      Origin: deployment.origin,
      'X-Forwarded-For': testNetAddress(),
      Authorization: `Bearer ${await accessToken(deployment, tutor)}`,
      ...(method === 'POST' ? { 'Idempotency-Key': randomUUID() } : {}),
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  if (response.status !== expected) {
    throw new Error(`${method} ${path} answered ${String(response.status)}`);
  }
  const text = await response.text();
  return text === '' ? null : (JSON.parse(text) as unknown);
}

function question(prompt: string, acceptedAnswers: readonly string[]): Record<string, unknown> {
  return { id: randomUUID(), type: 'short_text', prompt, points: 2, accepted_answers: acceptedAnswers };
}

export async function createFolder(deployment: Deployment, tutor: TestAccount, name: string): Promise<string> {
  const created = await send(deployment, tutor, 'POST', '/library/folders', 201, { name, parent_id: null });
  return textField(created, 'id');
}

export async function createReadyTemplate(
  deployment: Deployment,
  tutor: TestAccount,
  title: string,
  folderId: string | null = null,
): Promise<SavedTemplate> {
  const created = await send(deployment, tutor, 'POST', '/library/templates', 201, {
    title,
    folder_id: folderId,
    blocks: [question('What is one half plus one half?', ['1'])],
  });
  if (field(created, 'ready') !== true) {
    throw new Error('the seeded template is not ready');
  }
  return { id: textField(created, 'id'), version: numberField(created, 'version') };
}

export async function createReadyHandCheckedTemplate(
  deployment: Deployment,
  tutor: TestAccount,
  title: string,
): Promise<SavedTemplate> {
  const created = await send(deployment, tutor, 'POST', '/library/templates', 201, {
    title,
    folder_id: null,
    blocks: [{ id: randomUUID(), type: 'long_text', prompt: 'Explain one half.', points: 2, max_length: 400 }],
  });
  if (field(created, 'ready') !== true) {
    throw new Error('the seeded template is not ready');
  }
  return { id: textField(created, 'id'), version: numberField(created, 'version') };
}

export async function createIncompleteTemplate(
  deployment: Deployment,
  tutor: TestAccount,
  title: string,
): Promise<SavedTemplate> {
  const created = await send(deployment, tutor, 'POST', '/library/templates', 201, {
    title,
    folder_id: null,
    blocks: [question('An unfinished question', [])],
  });
  return { id: textField(created, 'id'), version: numberField(created, 'version') };
}

export async function makeTemplateIncomplete(
  deployment: Deployment,
  tutor: TestAccount,
  template: SavedTemplate,
  title: string,
): Promise<void> {
  const saved = await send(deployment, tutor, 'PUT', `/library/templates/${template.id}`, 200, {
    title,
    version: template.version,
    save_id: randomUUID(),
    blocks: [question('An unfinished question', [])],
  });
  if (field(saved, 'ready') !== false) {
    throw new Error('the template is still ready');
  }
}

export async function issueThroughTheApi(
  deployment: Deployment,
  tutor: TestAccount,
  templateId: string,
  audience: IssueAudienceSeed,
  seed: IssueSeed = {},
): Promise<string> {
  const created = await send(deployment, tutor, 'POST', '/issues', 201, {
    template_id: templateId,
    audience,
    lesson_id: seed.lessonId ?? null,
    deadline_at: seed.deadlineAt ?? null,
    answer_policy: seed.policy ?? 'after_check',
  });
  return textField(created, 'id');
}

export async function issueManyThroughTheApi(
  deployment: Deployment,
  tutor: TestAccount,
  templateId: string,
  audience: IssueAudienceSeed,
  count: number,
  seed: IssueSeed = {},
): Promise<void> {
  for (let index = 0; index < count; index += 1) {
    await issueThroughTheApi(deployment, tutor, templateId, audience, seed);
  }
}

export interface IssueState {
  readonly status: string;
  readonly deadlineAt: string | null;
  readonly lessonId: string | null;
  readonly version: number;
  readonly students: number;
}

export async function readIssue(deployment: Deployment, tutor: TestAccount, issueId: string): Promise<IssueState> {
  const issue = await send(deployment, tutor, 'GET', `/issues/${issueId}`, 200);
  const deadline = field(issue, 'deadline_at');
  const lesson = field(issue, 'lesson_id');
  return {
    status: textField(issue, 'status'),
    deadlineAt: typeof deadline === 'string' ? deadline : null,
    lessonId: typeof lesson === 'string' ? lesson : null,
    version: numberField(issue, 'version'),
    students: numberField(field(issue, 'progress'), 'students'),
  };
}

export async function issueTitles(deployment: Deployment, tutor: TestAccount, query: string): Promise<string[]> {
  const page = await send(deployment, tutor, 'GET', `/issues?${query}`, 200);
  const items = field(page, 'items');
  if (!Array.isArray(items)) {
    throw new Error('the issues are not a list');
  }
  return items.map((item: unknown) => textField(item, 'title'));
}

export async function issueIdsOf(deployment: Deployment, tutor: TestAccount, query: string): Promise<string[]> {
  const page = await send(deployment, tutor, 'GET', `/issues?${query}`, 200);
  const items = field(page, 'items');
  if (!Array.isArray(items)) {
    throw new Error('the issues are not a list');
  }
  return items.map((item: unknown) => textField(item, 'id'));
}

export async function changeDeadlineThroughTheApi(
  deployment: Deployment,
  tutor: TestAccount,
  issueId: string,
  deadlineAt: string,
): Promise<void> {
  const issue = await readIssue(deployment, tutor, issueId);
  await send(deployment, tutor, 'PATCH', `/issues/${issueId}`, 200, {
    deadline_at: deadlineAt,
    version: issue.version,
  });
}

export async function withdrawThroughTheApi(
  deployment: Deployment,
  tutor: TestAccount,
  issueId: string,
): Promise<void> {
  await send(deployment, tutor, 'POST', `/issues/${issueId}/withdraw`, 200);
}
