export type ReadinessRequestId = number;
export type ReadinessSocketId = number;
export type ReadinessWait = 'document_not_ready' | 'pending_api_request' | 'required_read_missing';

export function ownedApiPath(address: string, documentAddress: string, mainDocument: boolean): string | null {
  if (!mainDocument || !URL.canParse(address) || !URL.canParse(documentAddress)) return null;
  const url = new URL(address);
  const document = new URL(documentAddress);
  return (url.protocol === 'https:' || url.protocol === 'http:') &&
    url.origin === document.origin &&
    url.pathname.startsWith('/api/') &&
    url.pathname !== '/api/v1/ws'
    ? url.pathname
    : null;
}

export type ReadinessFailure =
  | 'ambiguous_frame'
  | 'capture_history_overflow'
  | 'none'
  | 'observer_failure'
  | 'navigation_failed'
  | 'request_failed'
  | 'duplicate_request'
  | 'response_refused_or_duplicate'
  | 'response_missing'
  | 'observer_operation_failed'
  | 'capture_document_failed'
  | 'request_frame_unavailable';

export type ReadinessPhase = 'committed' | 'navigating' | 'checking' | 'failed' | 'disposed';

interface ObservedQuery {
  readonly path: string;
  readonly readyEpoch: number | null;
  readonly requiredRead: boolean;
  status: number | null;
}

export class NavigationReadiness {
  private phase: ReadinessPhase = 'committed';
  private failureReason: ReadinessFailure = 'none';
  private opens = 0;
  private readies = 0;
  private closes = 0;
  private navigation: ReadinessRequestId | null = null;
  private socket: ReadinessSocketId | null = null;
  private ready = false;
  private readyEpoch = 0;
  private generation = 0;
  private readonly pending = new Map<ReadinessRequestId, ObservedQuery>();
  private readonly completed = new Set<string>();
  private readonly required: readonly string[];

  constructor(required: readonly string[]) {
    this.required = [...new Set(required)];
  }

  navigate(id: ReadinessRequestId, redirectedFrom: ReadinessRequestId | null): boolean {
    if (this.phase === 'disposed') return false;
    if (redirectedFrom !== null) {
      if (this.phase !== 'navigating' || this.navigation !== redirectedFrom) return false;
      this.navigation = id;
      return true;
    }
    this.generation += 1;
    this.phase = 'navigating';
    this.navigation = id;
    this.socket = null;
    this.ready = false;
    this.pending.clear();
    this.completed.clear();
    return true;
  }

  commit(id: ReadinessRequestId): void {
    if (this.phase !== 'navigating' || this.navigation !== id) {
      this.fail();
      return;
    }
    this.navigation = null;
    this.phase = 'committed';
  }

  navigationFailed(id: ReadinessRequestId): void {
    if (this.navigation === id) this.fail('navigation_failed');
  }

  checkDocument(): void {
    if (this.phase === 'committed') this.phase = 'checking';
  }

  sameDocument(): void {
    if (this.phase === 'checking') this.phase = 'committed';
  }

  socketOpened(id: ReadinessSocketId): boolean {
    if (this.phase !== 'committed' && this.phase !== 'checking') return false;
    this.opens += 1;
    this.socket = id;
    this.ready = false;
    this.readyEpoch += 1;
    this.completed.clear();
    return true;
  }

  socketReady(id: ReadinessSocketId): void {
    if (this.socket === id && (this.phase === 'committed' || this.phase === 'checking')) {
      this.readies += 1;
      this.ready = true;
    }
  }

  socketClosed(id: ReadinessSocketId): void {
    if (this.socket !== id) return;
    this.closes += 1;
    this.socket = null;
    this.ready = false;
    this.completed.clear();
  }

  requestStarted(id: ReadinessRequestId, path: string, requiredRead = true): boolean {
    if (this.phase !== 'committed' && this.phase !== 'checking') return false;
    if (this.pending.has(id)) {
      this.fail('duplicate_request');
      return false;
    }
    this.pending.set(id, { path, readyEpoch: this.ready ? this.readyEpoch : null, requiredRead, status: null });
    return true;
  }

  response(id: ReadinessRequestId, status: number): void {
    const query = this.pending.get(id);
    if (query === undefined) return;
    if (
      query.status !== null ||
      (query.requiredRead && this.required.includes(query.path) && (status < 200 || status >= 300))
    ) {
      this.fail('response_refused_or_duplicate');
      return;
    }
    query.status = status;
  }

  requestFinished(id: ReadinessRequestId): void {
    const query = this.pending.get(id);
    if (query === undefined) return;
    if (query.status === null) {
      this.fail('response_missing');
      return;
    }
    this.pending.delete(id);
    if (
      query.requiredRead &&
      this.required.includes(query.path) &&
      query.readyEpoch !== null &&
      query.readyEpoch === this.readyEpoch &&
      query.status >= 200 &&
      query.status < 300
    )
      this.completed.add(query.path);
  }

  requestFailed(id: ReadinessRequestId): void {
    if (this.pending.has(id)) this.fail('request_failed');
  }

  fail(reason: ReadinessFailure = 'observer_failure'): void {
    if (this.failureReason === 'none') this.failureReason = reason;
    if (this.phase !== 'disposed') this.phase = 'failed';
  }

  readyReceived(): boolean {
    return this.phase === 'committed' && this.ready;
  }

  coherent(captured: boolean): boolean {
    return captured && this.readyReceived() && this.unsettled().length === 0;
  }

  unsettled(): ReadinessWait[] {
    return [
      ...(this.phase === 'committed' ? [] : ['document_not_ready' as const]),
      ...[...this.pending.values()].map((): ReadinessWait => 'pending_api_request'),
      ...this.required.filter((path) => !this.completed.has(path)).map((): ReadinessWait => 'required_read_missing'),
    ];
  }

  snapshot(): { readonly generation: number; readonly pending: number; readonly phase: ReadinessPhase } {
    return { generation: this.generation, pending: this.pending.size, phase: this.phase };
  }

  evidence(): {
    readonly phase: ReadinessPhase;
    readonly generation: number;
    readonly pending: number;
    readonly missing: number;
    readonly reason: ReadinessFailure;
    readonly opens: number;
    readonly ready: number;
    readonly closes: number;
  } {
    return {
      ...this.snapshot(),
      missing: this.required.filter((path) => !this.completed.has(path)).length,
      reason: this.failureReason,
      opens: this.opens,
      ready: this.readies,
      closes: this.closes,
    };
  }

  dispose(): void {
    this.phase = 'disposed';
    this.navigation = null;
    this.socket = null;
    this.ready = false;
    this.pending.clear();
    this.completed.clear();
  }
}

export class AccountSwitchBoundary {
  private phase: 'logout' | 'logout_response' | 'confirmation' | 'observing' | 'failed' = 'logout';
  private logout: number | null = null;

  request(id: number, kind: 'logout' | 'confirmation', owned: boolean): boolean {
    if (!owned) {
      this.phase = 'failed';
      return false;
    }
    if (kind === 'logout' && this.phase === 'logout') {
      this.logout = id;
      this.phase = 'logout_response';
      return false;
    }
    if (kind === 'confirmation' && this.phase === 'confirmation') {
      this.phase = 'observing';
      return true;
    }
    this.phase = 'failed';
    return false;
  }

  response(id: number, status: number): void {
    this.phase = this.phase === 'logout_response' && id === this.logout && status === 204 ? 'confirmation' : 'failed';
  }

  valid(): boolean {
    return this.phase === 'observing';
  }
}
