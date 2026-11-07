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
  readonly closes: readonly number[];
}

const cancellationTexts: readonly string[] = [
  'net::ERR_ABORTED',
  'NS_BINDING_ABORTED',
  'cancelled',
  'Load request cancelled',
  'Provisiolal navigation canceled.',
];

export function isCancellationText(reason: string): boolean {
  return cancellationTexts.includes(reason);
}

function isCancellation(failed: FailedRequest): boolean {
  return isCancellationText(failed.reason);
}

export function cancellationAfterNavigationAction(failed: FailedRequest, action: number): boolean {
  return (
    !failed.answered &&
    isCancellation(failed) &&
    Number.isFinite(failed.started) &&
    failed.started < action &&
    action < failed.failed
  );
}

function stepDuring(steps: readonly number[], failed: FailedRequest): boolean {
  return steps.some((step) => step > failed.started && step < failed.failed);
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
  return stepDuring(activity.navigations, failed) || navigationRightAfter(failed, activity);
}

function leftByClose(failed: FailedRequest, activity: PageActivity): boolean {
  return stepDuring(activity.closes, failed);
}

export function unexpectedFailures(failures: readonly FailedRequest[], activity: PageActivity): string[] {
  return failures
    .filter(
      (failed) =>
        !failed.answered &&
        !(isCancellation(failed) && (leftByNavigation(failed, activity) || leftByClose(failed, activity))),
    )
    .map((failed) => `request failed: ${failed.description} ${failed.reason}`);
}
