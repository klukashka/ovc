import { randomUUID } from 'node:crypto';

import { type TestAccount, testNetAddress } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { accessToken, callApi } from './tutoring.ts';

export const sentinel = {
  points: 37,
  accepted: 'quintessentially',
  explanation: 'Explanation sentinel 5f2c',
  correctOption: 'Option sentinel 9a1d',
} as const;

export const pdfFile = {
  name: 'Essay.pdf',
  mimeType: 'application/pdf',
  buffer: Buffer.from('%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n'),
};

export const pictureFile = {
  name: 'Photo.png',
  mimeType: 'image/png',
  buffer: Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=',
    'base64',
  ),
};

export interface PastSimpleTexts {
  readonly title: string;
  readonly instruction: string;
  readonly choicePrompt: string;
  readonly textPrompt: string;
  readonly filePrompt: string;
}

export const pastSimple: PastSimpleTexts = {
  title: 'Past simple, week 1',
  instruction: 'Use the past simple in every answer.',
  choicePrompt: 'Yesterday I ___ to school.',
  textPrompt: 'Write the past form of “go”.',
  filePrompt: 'Upload a photo of your written sentences.',
};

function field(value: unknown, name: string): unknown {
  if (typeof value !== 'object' || value === null || !(name in value)) {
    throw new Error(`the answer has no ${name}`);
  }
  return Object.entries(value).find(([key]) => key === name)?.[1];
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

function listField(value: unknown, name: string): readonly unknown[] {
  const found = field(value, name);
  if (!Array.isArray(found)) {
    throw new Error(`${name} is not a list`);
  }
  return found;
}

async function uploadMaterial(deployment: Deployment, token: string, title: string, folderId: string): Promise<string> {
  const body = new FormData();
  body.append('file', new Blob([new Uint8Array(pdfFile.buffer)], { type: pdfFile.mimeType }), 'Irregular verbs.pdf');
  const uploaded = await fetch(`${deployment.apiUrl}/api/v1/uploads?context=library`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: { Origin: deployment.origin, 'X-Forwarded-For': testNetAddress(), Authorization: `Bearer ${token}` },
    body,
  });
  if (uploaded.status !== 201) {
    throw new Error(`uploading a material answered ${String(uploaded.status)}`);
  }
  const material = await callApi(deployment, token, 'POST', '/library/materials', {
    attachment_id: textField(await uploaded.json(), 'id'),
    title,
    folder_id: folderId,
  });
  if (material.status !== 201) {
    throw new Error(`creating a material answered ${String(material.status)}`);
  }
  return textField(material.body, 'id');
}

export interface TemplateOptions {
  readonly texts?: PastSimpleTexts;
  readonly withSentinels?: boolean;
  readonly withMaterial?: boolean;
  readonly fileKinds?: readonly string[];
  readonly maxFiles?: number;
}

export async function createPastSimpleTemplate(
  deployment: Deployment,
  tutor: TestAccount,
  options: TemplateOptions = {},
): Promise<string> {
  const texts = options.texts ?? pastSimple;
  const token = await accessToken(deployment, tutor);
  const marked = options.withSentinels === true;
  const blocks: Record<string, unknown>[] = [{ id: randomUUID(), type: 'instruction', text: texts.instruction }];
  if (options.withMaterial === true) {
    const folder = await callApi(deployment, token, 'POST', '/library/folders', { name: 'Grammar', parent_id: null });
    if (folder.status !== 201) {
      throw new Error(`creating a folder answered ${String(folder.status)}`);
    }
    const material = await uploadMaterial(deployment, token, 'Irregular verbs', textField(folder.body, 'id'));
    blocks.push({ id: randomUUID(), type: 'material', material_id: material });
  }
  blocks.push(
    {
      id: randomUUID(),
      type: 'single_choice',
      prompt: texts.choicePrompt,
      points: marked ? sentinel.points : 2,
      explanation: marked ? sentinel.explanation : 'Go is irregular: go, went, gone.',
      options: [
        { id: randomUUID(), text: 'goed', correct: false },
        { id: randomUUID(), text: marked ? sentinel.correctOption : 'went', correct: true },
      ],
    },
    {
      id: randomUUID(),
      type: 'short_text',
      prompt: texts.textPrompt,
      points: 2,
      accepted_answers: [marked ? sentinel.accepted : 'went'],
      explanation: marked ? sentinel.explanation : 'The past form of go is went.',
    },
    {
      id: randomUUID(),
      type: 'file',
      prompt: texts.filePrompt,
      points: 6,
      allowed_kinds: options.fileKinds ?? ['images', 'pdf'],
      max_files: options.maxFiles ?? 2,
      explanation: marked ? sentinel.explanation : 'Check the word order.',
    },
  );
  const created = await callApi(deployment, token, 'POST', '/library/templates', {
    title: texts.title,
    folder_id: null,
    blocks,
  });
  if (created.status !== 201 || field(created.body, 'ready') !== true) {
    throw new Error(`creating the template answered ${String(created.status)} without a ready template`);
  }
  return textField(created.body, 'id');
}

export interface StudentWork {
  readonly submissionId: string;
  readonly title: string;
  readonly status: string;
}

export async function studentWork(
  deployment: Deployment,
  student: TestAccount,
  status: 'todo' | 'submitted' | 'checked' | 'closed',
): Promise<StudentWork[]> {
  const answer = await callApi(
    deployment,
    await accessToken(deployment, student),
    'GET',
    `/assignments?status=${status}&limit=100`,
  );
  if (answer.status !== 200) {
    throw new Error(`listing assignments answered ${String(answer.status)}`);
  }
  return listField(answer.body, 'items').map((item) => ({
    submissionId: textField(item, 'submission_id'),
    title: textField(item, 'title'),
    status: textField(item, 'status'),
  }));
}

export async function onlySubmission(deployment: Deployment, student: TestAccount): Promise<string> {
  const [work] = await studentWork(deployment, student, 'todo');
  if (work === undefined) {
    throw new Error('the student has no work to do');
  }
  return work.submissionId;
}

export interface StoredAnswers {
  readonly version: number;
  readonly status: string;
  readonly texts: readonly string[];
  readonly files: readonly string[];
  readonly late: boolean;
}

export async function storedAnswers(
  deployment: Deployment,
  student: TestAccount,
  submissionId: string,
): Promise<StoredAnswers> {
  const answer = await callApi(
    deployment,
    await accessToken(deployment, student),
    'GET',
    `/submissions/${submissionId}`,
  );
  if (answer.status !== 200) {
    throw new Error(`reading the submission answered ${String(answer.status)}`);
  }
  const answers = listField(answer.body, 'answers');
  return {
    version: numberField(answer.body, 'version'),
    status: textField(answer.body, 'status'),
    late: field(answer.body, 'late') === true,
    texts: answers.flatMap((entry) => {
      const value = field(entry, 'value');
      const text = typeof value === 'object' && value !== null && 'text' in value ? value.text : undefined;
      return typeof text === 'string' && text !== '' ? [text] : [];
    }),
    files: answers.flatMap((entry) => listField(entry, 'files').map((file) => textField(file, 'filename'))),
  };
}

export async function questionIds(
  deployment: Deployment,
  student: TestAccount,
  submissionId: string,
): Promise<readonly string[]> {
  const answer = await callApi(
    deployment,
    await accessToken(deployment, student),
    'GET',
    `/submissions/${submissionId}`,
  );
  return listField(answer.body, 'blocks')
    .filter((block) => textField(block, 'type') !== 'instruction' && textField(block, 'type') !== 'material')
    .map((block) => textField(block, 'id'));
}

export async function saveTextFromAnotherDevice(
  deployment: Deployment,
  student: TestAccount,
  submissionId: string,
  questionId: string,
  text: string,
): Promise<void> {
  const stored = await storedAnswers(deployment, student, submissionId);
  const answer = await callApi(
    deployment,
    await accessToken(deployment, student),
    'PUT',
    `/submissions/${submissionId}/answers`,
    { version: stored.version, save_id: randomUUID(), answers: [{ question_id: questionId, value: { text } }] },
  );
  if (answer.status !== 200) {
    throw new Error(`saving from another device answered ${String(answer.status)}`);
  }
}

export async function answerSentinelsAndSubmit(
  deployment: Deployment,
  student: TestAccount,
  submissionId: string,
): Promise<void> {
  const token = await accessToken(deployment, student);
  const read = await callApi(deployment, token, 'GET', `/submissions/${submissionId}`);
  const blocks = listField(read.body, 'blocks');
  const choice = blocks.find((block) => textField(block, 'type') === 'single_choice');
  const text = blocks.find((block) => textField(block, 'type') === 'short_text');
  const option = listField(choice, 'options').find((entry) => textField(entry, 'text') === sentinel.correctOption);
  const saved = await callApi(deployment, token, 'PUT', `/submissions/${submissionId}/answers`, {
    version: numberField(read.body, 'version'),
    save_id: randomUUID(),
    answers: [
      { question_id: textField(choice, 'id'), value: { option_ids: [textField(option, 'id')] } },
      { question_id: textField(text, 'id'), value: { text: sentinel.accepted } },
    ],
  });
  if (saved.status !== 200) {
    throw new Error(`saving the sentinel answers answered ${String(saved.status)}`);
  }
  await submitThroughTheApi(deployment, student, submissionId);
}

export async function submitThroughTheApi(
  deployment: Deployment,
  student: TestAccount,
  submissionId: string,
): Promise<void> {
  const stored = await storedAnswers(deployment, student, submissionId);
  const answer = await callApi(
    deployment,
    await accessToken(deployment, student),
    'POST',
    `/submissions/${submissionId}/submit`,
    { version: stored.version },
  );
  if (answer.status !== 200) {
    throw new Error(`submitting answered ${String(answer.status)}`);
  }
}

export async function removeGroupMember(
  deployment: Deployment,
  tutor: TestAccount,
  groupId: string,
  cardId: string,
): Promise<void> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'PATCH', `/groups/${groupId}`, {
    remove: [cardId],
  });
  if (answer.status !== 200) {
    throw new Error(`removing the member answered ${String(answer.status)}`);
  }
}

export async function uploadedId(response: { json: () => Promise<unknown> }): Promise<string> {
  return textField(await response.json(), 'id');
}

async function saveFilesFromAnotherDevice(
  deployment: Deployment,
  student: TestAccount,
  submissionId: string,
  questionId: string,
  attachmentIds: readonly string[],
): Promise<void> {
  const stored = await storedAnswers(deployment, student, submissionId);
  const answer = await callApi(
    deployment,
    await accessToken(deployment, student),
    'PUT',
    `/submissions/${submissionId}/answers`,
    {
      version: stored.version,
      save_id: randomUUID(),
      answers: [{ question_id: questionId, value: { attachment_ids: attachmentIds } }],
    },
  );
  if (answer.status !== 200) {
    throw new Error(`saving files from another device answered ${String(answer.status)}`);
  }
}

export async function bindAndDropFromAnotherDevice(
  deployment: Deployment,
  student: TestAccount,
  submissionId: string,
  questionId: string,
  attachmentId: string,
): Promise<void> {
  await saveFilesFromAnotherDevice(deployment, student, submissionId, questionId, [attachmentId]);
  await saveFilesFromAnotherDevice(deployment, student, submissionId, questionId, []);
}
