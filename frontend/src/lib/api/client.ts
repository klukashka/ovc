import createClient, { type Client } from 'openapi-fetch';
import { createContext, useContext } from 'react';

import type { components, paths } from './schema.ts';

export type ApiClient = Client<paths>;

export type ReadyStatus = components['schemas']['ReadyStatus'];

export type ApiErrorBody = components['schemas']['ErrorBody'];

export class ApiError extends Error {
  readonly status: number;
  readonly body: ApiErrorBody | undefined;

  constructor(status: number, body: ApiErrorBody | undefined) {
    super(body === undefined ? `API request failed with status ${String(status)}` : `API error ${body.code}`);
    this.name = 'ApiError';
    this.status = status;
    this.body = body;
  }
}

export function createApiClient(origin: string, fetchImplementation: typeof fetch): ApiClient {
  return createClient<paths>({ baseUrl: origin, fetch: fetchImplementation });
}

export const ApiClientContext = createContext<ApiClient | null>(null);

export function useApiClient(): ApiClient {
  const client = useContext(ApiClientContext);
  if (client === null) {
    throw new Error('ApiClientContext is not provided');
  }
  return client;
}

function isErrorEnvelope(value: unknown): value is { error: ApiErrorBody } {
  if (typeof value !== 'object' || value === null || !('error' in value)) {
    return false;
  }
  const body = value.error;
  return typeof body === 'object' && body !== null && 'code' in body && typeof body.code === 'string';
}

export async function fetchReadiness(client: ApiClient, signal: AbortSignal): Promise<ReadyStatus> {
  const { data, error, response } = await client.GET('/api/v1/health/ready', { signal });
  if (data !== undefined) {
    return data;
  }
  throw new ApiError(response.status, isErrorEnvelope(error) ? error.error : undefined);
}
