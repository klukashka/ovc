export interface FailedRequest {
  readonly method: string;
  readonly description: string;
  readonly reason: string;
  readonly answered: boolean;
  readonly started: number;
  readonly failed: number;
}

export interface PageActivity {
  readonly documents: readonly number[];
  readonly routeChanges: readonly number[];
  readonly requests: readonly number[];
  readonly closes: readonly number[];
}

const cancellationTexts: readonly string[] = [
  'net::ERR_ABORTED',
  'NS_BINDING_ABORTED',
  'cancelled',
  'Load request cancelled',
  'Provisional navigation canceled.',
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

function navigationRightAfter(
  failed: FailedRequest,
  navigations: readonly number[],
  requests: readonly number[],
): boolean {
  const next = navigations.find((step) => step > failed.failed);
  return (
    next !== undefined &&
    failed.started < failed.failed &&
    !requests.some((step) => step > failed.failed && step < next)
  );
}

function leftBy(navigations: readonly number[], failed: FailedRequest, activity: PageActivity): boolean {
  return stepDuring(navigations, failed) || navigationRightAfter(failed, navigations, activity.requests);
}

function leftByNavigation(failed: FailedRequest, activity: PageActivity): boolean {
  return (
    leftBy(activity.documents, failed, activity) ||
    (failed.method === 'GET' && leftBy(activity.routeChanges, failed, activity))
  );
}

function leftByClose(failed: FailedRequest, activity: PageActivity): boolean {
  return stepDuring(activity.closes, failed);
}

export function isUnexpectedFailure(failed: FailedRequest, activity: PageActivity): boolean {
  return (
    !failed.answered &&
    !(isCancellation(failed) && (leftByNavigation(failed, activity) || leftByClose(failed, activity)))
  );
}

export function unexpectedFailures(failures: readonly FailedRequest[], activity: PageActivity): string[] {
  return failures
    .filter((failed) => isUnexpectedFailure(failed, activity))
    .map((failed) => `request failed: ${failed.description} ${failed.reason}`);
}

function pathnameOf(address: string): string {
  return URL.canParse(address) ? new URL(address).pathname : address;
}

export class PageActivityRecorder {
  private step = 0;
  private documentRequested = false;
  private pathname: string | undefined;
  private readonly documents: number[] = [];
  private readonly routeChanges: number[] = [];
  private readonly requests: number[] = [];
  private readonly closes: number[] = [];

  request(mainFrameDocument: boolean): number {
    this.step += 1;
    if (mainFrameDocument) {
      this.documentRequested = true;
      this.documents.push(this.step);
    } else {
      this.requests.push(this.step);
    }
    return this.step;
  }

  mainFrameNavigated(address: string): void {
    const pathname = pathnameOf(address);
    const previous = this.pathname;
    this.pathname = pathname;
    if (this.documentRequested) {
      this.documentRequested = false;
      return;
    }
    if (previous === undefined || previous === pathname) {
      return;
    }
    this.step += 1;
    this.routeChanges.push(this.step);
  }

  failed(): number {
    this.step += 1;
    return this.step;
  }

  close(): void {
    this.step += 1;
    this.closes.push(this.step);
  }

  activity(): PageActivity {
    return {
      documents: [...this.documents],
      routeChanges: [...this.routeChanges],
      requests: [...this.requests],
      closes: [...this.closes],
    };
  }
}
