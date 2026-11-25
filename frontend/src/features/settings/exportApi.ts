import { type ApiClient, apiErrorFrom } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';

export type Export = components['schemas']['Export'];
export type CurrentExport = components['schemas']['CurrentExport'];
export type ExportFailure = NonNullable<Export['failure']>;

export const currentExportKey = ['export', 'current'] as const;

export const exportRefetchMilliseconds = 10_000;

export async function fetchCurrentExport(client: ApiClient): Promise<CurrentExport> {
  const { data, error, response } = await client.GET('/api/v1/exports/current');
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

export function exportInProgress(current: Export | null): boolean {
  return current?.state === 'queued' || current?.state === 'building';
}
