import createClient, { type Client } from 'openapi-fetch';
import { createContext, useContext } from 'react';

import { type components, errorBodyCodeValues, type paths } from './schema.ts';

export type ApiClient = Client<paths>;

export type ApiErrorBody = components['schemas']['ErrorBody'];

export type OverlappingLesson = components['schemas']['OverlappingLesson'];

export type ErrorCode = ApiErrorBody['code'];

const knownErrorCodes: ReadonlySet<string> = new Set(errorBodyCodeValues);

export function isErrorCode(value: string): value is ErrorCode {
  return knownErrorCodes.has(value);
}

export const fieldCodes = [
  'invalid',
  'required',
  'not_allowed',
  'too_short',
  'too_long',
  'incorrect',
  'too_many',
  'webp_too_large',
] as const;

export type FieldCode = (typeof fieldCodes)[number];

export interface FieldProblem {
  readonly field: string;
  readonly code: FieldCode;
}

interface ReceivedFieldError {
  readonly path: string;
  readonly code: string;
}

interface ReceivedErrorBody {
  readonly code: string;
  readonly message: string;
  readonly request_id: string;
  readonly fields?: readonly ReceivedFieldError[] | null;
  readonly details?: unknown;
}

const bodyFieldPrefix = 'body.';

function isFieldCode(value: string): value is FieldCode {
  return fieldCodes.some((code) => code === value);
}

function fieldProblems(received: readonly ReceivedFieldError[] | null | undefined): readonly FieldProblem[] {
  return (received ?? []).map((problem) => ({
    field: problem.path.startsWith(bodyFieldPrefix) ? problem.path.slice(bodyFieldPrefix.length) : problem.path,
    code: isFieldCode(problem.code) ? problem.code : 'invalid',
  }));
}

function existingStudentOf(details: unknown): string | undefined {
  return typeof details === 'object' &&
    details !== null &&
    'student_id' in details &&
    typeof details.student_id === 'string'
    ? details.student_id
    : undefined;
}

function relationshipsOf(details: unknown): readonly string[] {
  if (
    typeof details !== 'object' ||
    details === null ||
    !('relationship_ids' in details) ||
    !Array.isArray(details.relationship_ids)
  ) {
    return [];
  }
  return details.relationship_ids.filter((id: unknown): id is string => typeof id === 'string');
}

function isOverlappingLesson(value: unknown): value is OverlappingLesson {
  return (
    typeof value === 'object' &&
    value !== null &&
    'title' in value &&
    typeof value.title === 'string' &&
    'starts_at' in value &&
    typeof value.starts_at === 'string' &&
    Number.isFinite(Date.parse(value.starts_at)) &&
    'ends_at' in value &&
    typeof value.ends_at === 'string' &&
    Number.isFinite(Date.parse(value.ends_at))
  );
}

function overlappingLessonsOf(details: unknown): readonly OverlappingLesson[] {
  if (typeof details !== 'object' || details === null || !('lessons' in details) || !Array.isArray(details.lessons)) {
    return [];
  }
  return details.lessons.filter(isOverlappingLesson);
}

function moreOverlappingOf(details: unknown): number {
  return typeof details === 'object' &&
    details !== null &&
    'more' in details &&
    typeof details.more === 'number' &&
    Number.isInteger(details.more) &&
    details.more > 0
    ? details.more
    : 0;
}

function isReceivedFieldError(value: unknown): value is ReceivedFieldError {
  return (
    typeof value === 'object' &&
    value !== null &&
    'path' in value &&
    typeof value.path === 'string' &&
    'code' in value &&
    typeof value.code === 'string'
  );
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: ErrorCode | undefined;
  readonly requestId: string | undefined;
  readonly fields: readonly FieldProblem[];
  readonly existingStudentId: string | undefined;
  readonly relationshipIds: readonly string[];
  readonly overlappingLessons: readonly OverlappingLesson[];
  readonly moreOverlapping: number;

  constructor(status: number, body: ReceivedErrorBody | undefined) {
    super(body === undefined ? `API request failed with status ${String(status)}` : `API error ${body.code}`);
    this.name = 'ApiError';
    this.status = status;
    this.code = body !== undefined && isErrorCode(body.code) ? body.code : undefined;
    this.requestId = body?.request_id;
    this.fields = fieldProblems(body?.fields);
    this.existingStudentId = existingStudentOf(body?.details);
    this.relationshipIds = relationshipsOf(body?.details);
    this.overlappingLessons = overlappingLessonsOf(body?.details);
    this.moreOverlapping = moreOverlappingOf(body?.details);
  }

  fieldCode(field: string): FieldCode | undefined {
    return this.fields.find((problem) => problem.field === field)?.code;
  }
}

export function apiErrorFrom(status: number, received: unknown): ApiError {
  return new ApiError(status, isErrorEnvelope(received) ? received.error : undefined);
}

export function asApiError(thrown: unknown): ApiError {
  return thrown instanceof ApiError ? thrown : apiErrorFrom(0, undefined);
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
    typeof body.request_id === 'string' &&
    (!('fields' in body) ||
      body.fields === null ||
      (Array.isArray(body.fields) && body.fields.every(isReceivedFieldError)))
  );
}
