import createClient, { type Client } from 'openapi-fetch';
import { createContext, useContext } from 'react';

import { type components, errorBodyCodeValues, type paths } from './schema.ts';

export type ApiClient = Client<paths>;

export type ReadyStatus = components['schemas']['ReadyStatus'];

export type ApiErrorBody = components['schemas']['ErrorBody'];

export type ErrorCode = ApiErrorBody['code'];

const knownErrorCodes: ReadonlySet<string> = new Set(errorBodyCodeValues);

export function isErrorCode(value: string): value is ErrorCode {
  return knownErrorCodes.has(value);
}

interface ReceivedErrorBody {
  readonly code: string;
  readonly message: string;
  readonly request_id: string;
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: ErrorCode | undefined;
  readonly requestId: string | undefined;

  constructor(status: number, body: ReceivedErrorBody | undefined) {
    super(body === undefined ? `API request failed with status ${String(status)}` : `API error ${body.code}`);
    this.name = 'ApiError';
    this.status = status;
    this.code = body !== undefined && isErrorCode(body.code) ? body.code : undefined;
    this.requestId = body?.request_id;
  }
}

export function apiErrorFrom(status: number, received: unknown): ApiError {
  return new ApiError(status, isErrorEnvelope(received) ? received.error : undefined);
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

function isErrorEnvelope(value: unknown): value is { error: ReceivedErrorBody } {
  if (typeof value !== 'object' || value === null || !('error' in value)) {
    return false;
  }
  const body = value.error;
  return (
    typeof body === 'object' &&
    body !== null &&
    'code' in body &&
    typeof body.code === 'string' &&
    'message' in body &&
    typeof body.message === 'string' &&
    'request_id' in body &&
    typeof body.request_id === 'string'
  );
}

export async function fetchReadiness(client: ApiClient, signal: AbortSignal): Promise<ReadyStatus> {
  const { data, error, response } = await client.GET('/api/v1/health/ready', { signal });
  if (data?.status === 'ready') {
    return data;
  }
  if (data !== undefined) {
    throw new ApiError(response.status, undefined);
  }
  throw apiErrorFrom(response.status, error);
}
