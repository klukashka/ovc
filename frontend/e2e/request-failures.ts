export interface FailedRequest {
  readonly description: string;
  readonly reason: string;
  readonly answered: boolean;
  readonly started: number;
  readonly failed: number;
}

export interface PageActivity {
  readonly navigations: readonly number[];
  readonly requests: readonly number[];
}

const cancelledRequest = /abort|cancel/i;

function navigationDuring(failed: FailedRequest, activity: PageActivity): boolean {
  return activity.navigations.some((step) => step > failed.started && step < failed.failed);
}

function navigationRightAfter(failed: FailedRequest, activity: PageActivity): boolean {
  const next = activity.navigations.find((step) => step > failed.failed);
  return (
    next !== undefined &&
    failed.started < failed.failed &&
    !activity.requests.some((step) => step > failed.failed && step < next)
  );
}

function leftByNavigation(failed: FailedRequest, activity: PageActivity): boolean {
  return (
    cancelledRequest.test(failed.reason) &&
    (navigationDuring(failed, activity) || navigationRightAfter(failed, activity))
  );
}

export function unexpectedFailures(failures: readonly FailedRequest[], activity: PageActivity): string[] {
  return failures
    .filter((failed) => !failed.answered && !leftByNavigation(failed, activity))
    .map((failed) => `request failed: ${failed.description} ${failed.reason}`);
}
