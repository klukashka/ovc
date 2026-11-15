import { type ApiClient, apiErrorFrom } from '../../lib/api/client.ts';
import { type components, pathsApiV1IssuesGetParametersQueryStatusValues } from '../../lib/api/schema.ts';

export type Issue = components['schemas']['Issue'];
export type IssueItem = components['schemas']['IssueItem'];
export type IssuePage = components['schemas']['IssuePage'];
export type IssueSubmission = components['schemas']['IssueSubmission'];
export type IssueStudentSubmission = components['schemas']['IssueStudentSubmission'];
export type IssueAudience = components['schemas']['IssueAudience'];
export type NewIssue = components['schemas']['NewIssue'];
export type IssuePatch = components['schemas']['IssuePatch'];
export type AnswerPolicy = NewIssue['answer_policy'];
export type IssueStatus = (typeof pathsApiV1IssuesGetParametersQueryStatusValues)[number];
export type SubmissionStatus = IssueSubmission['status'];

export const answerPolicies: readonly AnswerPolicy[] = ['never', 'after_submission', 'after_check'];

export const issuePageSize = 20;

export const issuesKey = ['issues'] as const;

export interface IssueFilter {
  readonly status: IssueStatus;
  readonly student?: string;
  readonly group?: string;
}

export function issueListKey(filter: IssueFilter): readonly unknown[] {
  return [...issuesKey, 'list', filter.status, filter.student ?? null, filter.group ?? null];
}

export function issueKey(id: string): readonly unknown[] {
  return [...issuesKey, 'detail', id];
}

export function isIssueStatus(value: string | null): value is IssueStatus {
  return pathsApiV1IssuesGetParametersQueryStatusValues.some((status) => status === value);
}

export function issueAddress(id: string): string {
  return `/homework/issued/${encodeURIComponent(id)}`;
}

export function builderAddress(templateId: string): string {
  return `/homework/templates/${encodeURIComponent(templateId)}`;
}

export async function fetchIssuePage(
  client: ApiClient,
  filter: IssueFilter,
  cursor: string | null,
  signal: AbortSignal | null,
): Promise<IssuePage> {
  const { data, error, response } = await client.GET('/api/v1/issues', {
    params: {
      query: {
        status: filter.status,
        limit: issuePageSize,
        ...(filter.student === undefined ? {} : { student: filter.student }),
        ...(filter.group === undefined ? {} : { group: filter.group }),
        ...(cursor === null ? {} : { cursor }),
      },
    },
    signal,
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

export async function fetchIssue(client: ApiClient, id: string, signal: AbortSignal): Promise<Issue> {
  const { data, error, response } = await client.GET('/api/v1/issues/{id}', { params: { path: { id } }, signal });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

export function audienceName(audience: IssueAudience): string {
  return audience.display_name ?? audience.name ?? '';
}
