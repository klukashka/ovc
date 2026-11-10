import { type TestAccount, testNetAddress } from './accounts.ts';
import type { Deployment } from './deployment.ts';
import { readLesson } from './schedule.ts';
import { accessToken, callApi } from './tutoring.ts';

export interface MaterialFile {
  readonly name: string;
  readonly mimeType: string;
  readonly buffer: Buffer;
}

export const worksheetFile: MaterialFile = {
  name: 'Рабочий лист.pdf',
  mimeType: 'application/pdf',
  buffer: Buffer.from('%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n'),
};

function identifierOf(body: unknown): string {
  if (typeof body !== 'object' || body === null || !('id' in body) || typeof body.id !== 'string') {
    throw new Error('the answer has no identifier');
  }
  return body.id;
}

function listOf(body: unknown, name: string): readonly unknown[] {
  if (typeof body !== 'object' || body === null || !(name in body)) {
    throw new Error(`the answer has no ${name}`);
  }
  const value: unknown = Object.entries(body).find(([key]) => key === name)?.[1];
  if (!Array.isArray(value)) {
    throw new Error(`${name} is not a list`);
  }
  return value;
}

function titleOf(item: unknown): string {
  if (typeof item !== 'object' || item === null || !('title' in item) || typeof item.title !== 'string') {
    throw new Error('a material has no title');
  }
  return item.title;
}

export async function createLibraryFolder(deployment: Deployment, tutor: TestAccount, name: string): Promise<string> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'POST', '/library/folders', {
    name,
    parent_id: null,
  });
  if (answer.status !== 201) {
    throw new Error(`creating a folder answered ${String(answer.status)}`);
  }
  return identifierOf(answer.body);
}

export async function createLibraryMaterial(
  deployment: Deployment,
  tutor: TestAccount,
  title: string,
  folderId: string | null = null,
  file: MaterialFile = worksheetFile,
): Promise<string> {
  const token = await accessToken(deployment, tutor);
  const body = new FormData();
  body.append('file', new Blob([new Uint8Array(file.buffer)], { type: file.mimeType }), file.name);
  const uploaded = await fetch(`${deployment.apiUrl}/api/v1/uploads?context=library`, {
    signal: AbortSignal.timeout(10_000),
    method: 'POST',
    headers: { Origin: deployment.origin, 'X-Forwarded-For': testNetAddress(), Authorization: `Bearer ${token}` },
    body,
  });
  if (uploaded.status !== 201) {
    throw new Error(`uploading a material answered ${String(uploaded.status)}`);
  }
  const answer = await callApi(deployment, token, 'POST', '/library/materials', {
    attachment_id: identifierOf(await uploaded.json()),
    title,
    folder_id: folderId,
  });
  if (answer.status !== 201) {
    throw new Error(`creating a material answered ${String(answer.status)}`);
  }
  return identifierOf(answer.body);
}

export async function deleteLibraryMaterial(deployment: Deployment, tutor: TestAccount, id: string): Promise<void> {
  const answer = await callApi(deployment, await accessToken(deployment, tutor), 'DELETE', `/library/materials/${id}`);
  if (answer.status !== 204) {
    throw new Error(`deleting a material answered ${String(answer.status)}`);
  }
}

export async function attachLessonMaterials(
  deployment: Deployment,
  tutor: TestAccount,
  lessonId: string,
  materialIds: readonly string[],
): Promise<void> {
  const { version } = await readLesson(deployment, tutor, lessonId);
  const answer = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'POST',
    `/lessons/${lessonId}/materials`,
    { material_ids: materialIds, version },
  );
  if (answer.status !== 200) {
    throw new Error(`attaching materials answered ${String(answer.status)}`);
  }
}

export async function lessonMaterialTitles(
  deployment: Deployment,
  account: TestAccount,
  lessonId: string,
): Promise<readonly string[]> {
  const answer = await callApi(deployment, await accessToken(deployment, account), 'GET', `/lessons/${lessonId}`);
  if (answer.status !== 200) {
    throw new Error(`reading the lesson answered ${String(answer.status)}`);
  }
  return listOf(answer.body, 'materials').map(titleOf);
}

export async function scheduledCardLessonIds(
  deployment: Deployment,
  tutor: TestAccount,
  cardId: string,
): Promise<readonly string[]> {
  const answer = await callApi(
    deployment,
    await accessToken(deployment, tutor),
    'GET',
    `/students/${cardId}/lessons?limit=100`,
  );
  if (answer.status !== 200) {
    throw new Error(`reading the card's lessons answered ${String(answer.status)}`);
  }
  return listOf(answer.body, 'items').flatMap((item) =>
    typeof item === 'object' && item !== null && 'status' in item && item.status === 'scheduled'
      ? [identifierOf(item)]
      : [],
  );
}
