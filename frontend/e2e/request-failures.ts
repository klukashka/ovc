export interface FailedRequest {
  readonly description: string;
  readonly reason: string;
  readonly answered: boolean;
  readonly document: number;
}

const cancelledRequest = /abort|cancel/i;

function leftByNavigation(failed: FailedRequest, documents: number): boolean {
  return cancelledRequest.test(failed.reason) && documents > failed.document;
}

export function unexpectedFailures(failures: readonly FailedRequest[], documents: number): string[] {
  return failures
    .filter((failed) => !failed.answered && !leftByNavigation(failed, documents))
    .map((failed) => `request failed: ${failed.description} ${failed.reason}`);
}
