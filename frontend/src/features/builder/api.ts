import { type ApiClient, type FieldProblem, apiErrorFrom, asApiError } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import { retryAfterSeconds } from '../../components/ui/Form.tsx';
import type { SaveOutcome, SaveRequest, Template } from './autosave.ts';

export type StudentTemplateResponse = components['schemas']['StudentTemplate'];

export const templateKey = (id: string): readonly unknown[] => ['builder', 'template', id];

export const studentViewKey = (id: string): readonly unknown[] => ['builder', 'student-view', id];

export async function fetchTemplate(client: ApiClient, id: string, signal: AbortSignal): Promise<Template> {
  const { data, error, response } = await client.GET('/api/v1/library/templates/{id}', {
    params: { path: { id } },
    signal,
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

export async function fetchStudentView(client: ApiClient, id: string): Promise<StudentTemplateResponse> {
  const { data, error, response } = await client.GET('/api/v1/library/templates/{id}/student-view', {
    params: { path: { id } },
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

function storageLimitReached(status: number, fields: readonly FieldProblem[]): boolean {
  return status === 422 && fields.some((field) => field.field === 'body' && field.code === 'too_many');
}

export async function saveTemplate(client: ApiClient, id: string, request: SaveRequest): Promise<SaveOutcome> {
  try {
    const { data, error, response } = await client.PUT('/api/v1/library/templates/{id}', {
      params: { path: { id } },
      body: { title: request.title, blocks: [...request.blocks], version: request.version, save_id: request.saveId },
    });
    if (data !== undefined) {
      return { kind: 'saved', template: data };
    }
    const refusal = apiErrorFrom(response.status, error);
    if (response.status === 409) {
      return { kind: 'conflict' };
    }
    if (response.status === 429) {
      return { kind: 'rateLimited', retryAfterSeconds: retryAfterSeconds(response), error: refusal };
    }
    if (response.status === 413) {
      return { kind: 'refused', reason: 'tooLarge', error: refusal };
    }
    if (storageLimitReached(response.status, refusal.fields)) {
      return { kind: 'refused', reason: 'storageLimit', error: refusal };
    }
    if (response.status === 404) {
      return { kind: 'refused', reason: 'notFound', error: refusal };
    }
    if (response.status >= 500 || response.status === 0) {
      return { kind: 'uncertain', error: refusal };
    }
    return { kind: 'refused', reason: 'other', error: refusal };
  } catch (thrown: unknown) {
    return { kind: 'uncertain', error: asApiError(thrown) };
  }
}
