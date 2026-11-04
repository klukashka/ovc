export type DiagnosticHttpMethod = 'GET' | 'POST' | 'PATCH' | 'DELETE';
export interface HttpNegative {
  readonly method: DiagnosticHttpMethod;
  readonly address: string;
  readonly status: number;
}
interface HttpObserved {
  readonly id: number;
  readonly method: string;
  readonly address: string;
  status: number | null;
  text: string;
  complete: boolean;
  declared: boolean;
  consumed: boolean;
}
export class HttpNegativePhase {
  private readonly grants: { readonly expected: HttpNegative; used: boolean }[] = [];
  private readonly requests = new Map<number, HttpObserved>();
  private readonly console = [] as { readonly text: string; readonly address: string }[];
  private closed = false;
  private faults = 0;

  add(expected: HttpNegative): void {
    if (
      this.closed ||
      this.grants.length >= 32 ||
      !Number.isInteger(expected.status) ||
      expected.status < 400 ||
      expected.status > 599
    )
      throw new Error('http_negative_declaration_invalid');
    this.grants.push({ expected, used: false });
  }
  start(id: number, method: string, address: string): void {
    if (!this.grants.some((grant) => grant.expected.method === method && grant.expected.address === address)) return;
    if (this.closed || this.requests.size >= 128 || this.requests.has(id)) {
      this.faults += 1;
      return;
    }
    this.requests.set(id, {
      id,
      method,
      address,
      status: null,
      text: '',
      complete: false,
      declared: false,
      consumed: false,
    });
  }
  response(id: number, status: number, statusText: string): boolean {
    const request = this.requests.get(id);
    if (this.closed || request?.status !== null) {
      this.faults += 1;
      return false;
    }
    request.status = status;
    const grant = this.grants.find(
      (entry) =>
        !entry.used &&
        entry.expected.method === request.method &&
        entry.expected.address === request.address &&
        entry.expected.status === status,
    );
    if (grant === undefined) {
      this.faults += 1;
      return false;
    }
    grant.used = true;
    request.declared = true;
    request.text = `Failed to load resource: the server responded with a status of ${String(status)} (${statusText})`;
    return true;
  }
  complete(id: number, succeeded: boolean): void {
    const request = this.requests.get(id);
    if (this.closed || request === undefined || request.complete || !succeeded) {
      this.faults += 1;
      return;
    }
    request.complete = true;
  }
  capture(text: string, address: string): boolean {
    if (
      this.closed ||
      !/^Failed to load resource: the server responded with a status of \d{3} \([^\r\n]*\)$/.test(text)
    )
      return false;
    if (this.console.length >= 32) this.faults += 1;
    else this.console.push({ text, address });
    return true;
  }
  evidence(): {
    readonly declared: number;
    readonly matched: number;
    readonly completed: number;
    readonly faults: number;
  } {
    return {
      declared: this.grants.length,
      matched: this.grants.filter((grant) => grant.used).length,
      completed: [...this.requests.values()].filter((request) => request.declared && request.complete).length,
      faults: this.faults,
    };
  }
  finish(): { readonly httpFailures: number; readonly consoleFailures: number } {
    if (this.closed) throw new Error('http_negative_phase_already_closed');
    this.closed = true;
    let consoleFailures = 0;
    for (const event of this.console) {
      const possible = [...this.requests.values()].filter(
        (request) =>
          request.declared &&
          request.complete &&
          !request.consumed &&
          request.text === event.text &&
          (event.address === '' || event.address === request.address),
      );
      if (possible.length !== 1) consoleFailures += 1;
      else {
        const matched = possible[0];
        if (matched === undefined) throw new Error('http_negative_unique_match_missing');
        matched.consumed = true;
      }
    }
    const httpFailures =
      this.faults +
      this.grants.filter((grant) => !grant.used).length +
      [...this.requests.values()].filter((request) => request.declared && !request.complete).length;
    this.console.length = 0;
    this.requests.clear();
    this.grants.length = 0;
    return { httpFailures, consoleFailures };
  }
}

export class LanguageFailurePhase {
  private address = '';
  private failed = false;
  private readonly warnings: { readonly text: string; readonly location: string }[] = [];
  private faults = 0;
  private closed = false;
  private unconsumedWarnings = 0;
  private missingWarning = false;
  private readonly engine: string;
  private readonly origin: string;
  private readonly language: 'en' | 'ru';
  constructor(engine: string, origin: string, language: 'en' | 'ru') {
    this.engine = engine;
    this.origin = origin;
    this.language = language;
  }
  request(address: string): void {
    const url = new URL(address);
    if (
      url.origin !== this.origin ||
      url.search !== '' ||
      !new RegExp(`^/assets/language-${this.language}-[^/]+\\.js$`).test(url.pathname)
    )
      return;
    if (this.closed || this.address !== '') this.faults += 1;
    else this.address = address;
  }
  failure(address: string, cancelled: boolean): void {
    if (address !== this.address) return;
    if (this.closed || this.failed || !cancelled) this.faults += 1;
    else this.failed = true;
  }
  warning(text: string, location: string): boolean {
    if (this.closed || this.engine !== 'firefox') return false;
    if (this.warnings.length >= 2) this.faults += 1;
    else this.warnings.push({ text, location });
    return true;
  }
  finish(): boolean {
    if (this.closed) throw new Error('language_failure_phase_already_closed');
    this.closed = true;
    const plain = `Loading failed for the module with source “${this.address}”.`;
    const validWarning = ({ text, location }: { readonly text: string; readonly location: string }): boolean => {
      if (text === plain) return location === this.address;
      const wrapper = /^\[JavaScript Warning: "([^"\r\n]+)" \{file: "([^"\r\n]*)" line: 0\}\]$/.exec(text);
      if (wrapper?.[1] !== plain || wrapper[2] !== location || !URL.canParse(location)) return false;
      const caller = new URL(location);
      return (
        caller.origin === this.origin &&
        caller.search === '' &&
        caller.pathname.startsWith('/assets/') &&
        caller.pathname.endsWith('.js')
      );
    };
    const facts = this.faults === 0 && this.failed && this.address !== '';
    const expected = this.engine === 'firefox' && facts ? this.warnings.findIndex(validWarning) : -1;
    this.unconsumedWarnings = this.warnings.length - (expected < 0 ? 0 : 1);
    this.missingWarning = this.engine === 'firefox' && expected < 0;
    const valid = facts && this.unconsumedWarnings === 0 && !this.missingWarning;
    this.warnings.length = 0;
    this.address = '';
    return valid;
  }
  unexpectedWarnings(): number {
    return this.unconsumedWarnings;
  }
  expectedWarningMissing(): boolean {
    return this.missingWarning;
  }
}

export function ownFailurePosition(
  error: unknown,
  sourcePath: string,
): { readonly line: number; readonly column: number } {
  if (!(error instanceof Error)) return { line: -1, column: -1 };
  for (const frame of (error.stack ?? '').split('\n').slice(1)) {
    if (!/^\s+at /.test(frame)) continue;
    const start = frame.indexOf(`${sourcePath}:`);
    if (start < 0) continue;
    const coordinates = /^(\d+):(\d+)\)?$/.exec(frame.slice(start + sourcePath.length + 1));
    if (coordinates === null) continue;
    const line = Number(coordinates[1]);
    const column = Number(coordinates[2]);
    if (
      Number.isSafeInteger(line) &&
      line > 0 &&
      line <= 100000 &&
      Number.isSafeInteger(column) &&
      column > 0 &&
      column <= 100000
    )
      return { line, column };
  }
  return { line: -1, column: -1 };
}
