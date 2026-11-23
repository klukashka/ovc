import { type RefObject, useEffect, useState } from 'react';
import { type Location, useLocation, useNavigate } from 'react-router';

export type FormResult =
  { readonly kind: 'lessonScheduled' } | { readonly kind: 'assignmentIssued'; readonly issueId: string };

export type FormResultKind = FormResult['kind'];

export interface FormOrigin {
  readonly address: string;
  readonly opener: string;
}

interface PendingReturn {
  readonly originKey: string;
  readonly opener: string | null;
  readonly result: FormResult | null;
  readonly context: string | null;
}

interface StoredPreset {
  readonly account: string;
  readonly preset: unknown;
}

class FormReturns {
  private pending: PendingReturn | null = null;
  private presets = new Map<string, StoredPreset>();

  keptPreset(visit: string, account: string, preset: unknown): void {
    this.presets = new Map([[visit, { account, preset }]]);
  }

  presetFor(visit: string, account: string): unknown {
    const stored = this.presets.get(visit);
    return stored?.account === account ? stored.preset : undefined;
  }

  opened(originKey: string, opener: string, context: string | null): void {
    this.pending = { originKey, opener, result: null, context };
  }

  settled(result: FormResult): void {
    if (this.pending !== null) {
      const focusesNotice = result.kind === 'assignmentIssued';
      this.pending = { ...this.pending, result, opener: focusesNotice ? null : this.pending.opener };
    }
  }

  forgotten(): void {
    this.pending = null;
  }

  peekOpener(locationKey: string): string | null {
    const pending = this.arrivedAt(locationKey);
    return pending?.opener ?? null;
  }

  openerSettled(locationKey: string): void {
    const pending = this.arrivedAt(locationKey);
    if (pending !== null) {
      this.keep({ ...pending, opener: null });
    }
  }

  peekResult<K extends FormResultKind>(locationKey: string, kind: K): Extract<FormResult, { kind: K }> | null {
    const result = this.arrivedAt(locationKey)?.result ?? null;
    return result !== null && isKind(result, kind) ? result : null;
  }

  takeResult(locationKey: string, kind: FormResultKind): void {
    const pending = this.arrivedAt(locationKey);
    if (pending?.result?.kind === kind) {
      this.keep({ ...pending, result: null });
    }
  }

  peekReturned(locationKey: string, context: string): boolean {
    return this.arrivedAt(locationKey)?.context === context;
  }

  takeReturned(locationKey: string, context: string): void {
    const pending = this.arrivedAt(locationKey);
    if (pending?.context === context) {
      this.keep({ ...pending, context: null });
    }
  }

  private arrivedAt(locationKey: string): PendingReturn | null {
    return this.pending?.originKey === locationKey ? this.pending : null;
  }

  private keep(pending: PendingReturn): void {
    this.pending = pending.opener === null && pending.result === null && pending.context === null ? null : pending;
  }
}

function isKind<K extends FormResultKind>(result: FormResult, kind: K): result is Extract<FormResult, { kind: K }> {
  return result.kind === kind;
}

const formReturns = new FormReturns();

function addressOf(location: Location): string {
  return `${location.pathname}${location.search}`;
}

export function formOriginOf(state: unknown): FormOrigin | null {
  if (typeof state !== 'object' || state === null || !('origin' in state)) {
    return null;
  }
  const origin = state.origin;
  if (typeof origin !== 'object' || origin === null || !('address' in origin) || !('opener' in origin)) {
    return null;
  }
  const { address, opener } = origin;
  return typeof address === 'string' && isInAppAddress(address) && typeof opener === 'string'
    ? { address, opener }
    : null;
}

function isInAppAddress(address: string): boolean {
  return address.startsWith('/') && !address.startsWith('//') && !address.includes('\\');
}

function visitOf(state: unknown): string | null {
  return typeof state === 'object' && state !== null && 'visit' in state && typeof state.visit === 'string'
    ? state.visit
    : null;
}

export function useFormPreset(account: string): unknown {
  const location = useLocation();
  const visit = visitOf(location.state);
  const [preset] = useState(() => (visit === null ? undefined : formReturns.presetFor(visit, account)));
  return preset;
}

interface FormRouteState {
  readonly origin: FormOrigin;
  readonly visit: string;
}

export function useOpenFormRoute(
  account: string,
): (address: string, opener: string, preset?: unknown, context?: string) => void {
  const location = useLocation();
  const navigate = useNavigate();
  return (address, opener, preset, context) => {
    formReturns.opened(location.key, opener, context ?? null);
    const visit = crypto.randomUUID();
    formReturns.keptPreset(visit, account, preset);
    const state: FormRouteState = { origin: { address: addressOf(location), opener }, visit };
    void navigate(address, { state });
  };
}

export function useLeaveFormRoute(fallback: string): {
  readonly origin: FormOrigin | null;
  readonly backAddress: string;
  readonly leave: (result?: FormResult) => void;
} {
  const location = useLocation();
  const navigate = useNavigate();
  const origin = formOriginOf(location.state);
  return {
    origin,
    backAddress: origin?.address ?? fallback,
    leave: (result): void => {
      if (origin === null) {
        formReturns.forgotten();
        void navigate(fallback, { replace: true });
        return;
      }
      if (result !== undefined) {
        formReturns.settled(result);
      }
      void navigate(-1);
    },
  };
}

export function useFormResult<K extends FormResultKind>(kind: K): Extract<FormResult, { kind: K }> | null {
  const { key } = useLocation();
  const [result] = useState(() => formReturns.peekResult(key, kind));

  useEffect(() => {
    formReturns.takeResult(key, kind);
  }, [key, kind]);

  return result;
}

export function useFormReturned(context: string): boolean {
  const { key } = useLocation();
  const [returned] = useState(() => formReturns.peekReturned(key, context));

  useEffect(() => {
    formReturns.takeReturned(key, context);
  }, [key, context]);

  return returned;
}

export function focusIsOnHeadingOrLost(): boolean {
  const focused = document.activeElement;
  return (
    focused === null ||
    focused === document.body ||
    !focused.isConnected ||
    (focused instanceof HTMLHeadingElement && focused.matches('main h1'))
  );
}

export function useReturnFocus(opener: string, element: RefObject<HTMLElement | null>, shown = true): void {
  const { key } = useLocation();

  useEffect(() => {
    if (!shown || formReturns.peekOpener(key) !== opener) {
      return undefined;
    }
    let active = true;
    queueMicrotask(() => {
      if (!active || formReturns.peekOpener(key) !== opener) {
        return;
      }
      if (focusIsOnHeadingOrLost()) {
        element.current?.focus();
      }
      formReturns.openerSettled(key);
    });
    return (): void => {
      active = false;
    };
  }, [key, opener, element, shown]);
}
