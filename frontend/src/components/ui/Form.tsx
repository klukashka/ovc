import { type ReactElement, type ReactNode, useEffect, useId, useRef } from 'react';
import type { TFunction } from 'i18next';

import type { ApiError } from '../../lib/api/client.ts';
import { ChevronDownIcon } from './icons.tsx';

export const fieldClass =
  'min-h-11 w-full rounded-md border border-border-strong bg-field px-3 text-body-phone text-text aria-invalid:border-danger tablet:text-body';
export const linkClass = 'inline-flex min-h-11 items-center text-accent-text underline';
export const primaryButtonClass =
  'inline-flex min-h-11 items-center justify-center rounded-md bg-accent px-4 text-strong text-on-accent hover:bg-accent-hover disabled:opacity-60';
export const quietButtonClass =
  'inline-flex min-h-11 items-center justify-center rounded-md border border-border-strong px-4 text-strong text-text hover:bg-surface disabled:opacity-60';
export const selectClass =
  'h-11 w-full appearance-none rounded-md border border-border-strong bg-field pr-9 pl-3 text-text aria-invalid:border-danger';

export function SelectChevron(): ReactElement {
  return (
    <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-icon">
      <ChevronDownIcon />
    </span>
  );
}

const passwordFields: ReadonlySet<string> = new Set(['password', 'new_password', 'current_password']);

export function fieldMessage(t: TFunction, error: ApiError | null, field: string): string | undefined {
  const code = error?.fieldCode(field);
  if (code === undefined) {
    return undefined;
  }
  if (passwordFields.has(field) && (code === 'too_short' || code === 'too_long')) {
    return t('fieldErrors.passwordLength');
  }
  return t(`fieldErrors.${code}`);
}

export function TextField({
  label,
  name,
  type = 'text',
  autoComplete,
  hint,
  error,
  required = true,
  minLength,
  maxLength,
  defaultValue,
  readOnly,
}: {
  readonly label: string;
  readonly name: string;
  readonly type?: 'text' | 'email' | 'password';
  readonly autoComplete: string;
  readonly hint?: string;
  readonly error?: string | undefined;
  readonly required?: boolean;
  readonly minLength?: number;
  readonly maxLength?: number;
  readonly defaultValue?: string;
  readonly readOnly?: boolean;
}): ReactElement {
  const inputId = useId();
  const hintId = useId();
  const errorId = useId();
  const described = [hint === undefined ? null : hintId, error === undefined ? null : errorId]
    .filter((id) => id !== null)
    .join(' ');
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={inputId} className="text-label">
        {label}
      </label>
      <input
        id={inputId}
        name={name}
        type={type}
        autoComplete={autoComplete}
        required={required}
        minLength={minLength}
        maxLength={maxLength}
        defaultValue={defaultValue}
        readOnly={readOnly}
        aria-invalid={error === undefined ? undefined : true}
        aria-describedby={described === '' ? undefined : described}
        className={fieldClass}
      />
      {hint === undefined ? null : (
        <p id={hintId} className="text-small text-text-muted">
          {hint}
        </p>
      )}
      {error === undefined ? null : (
        <p id={errorId} className="text-small text-danger">
          {error}
        </p>
      )}
    </div>
  );
}

export function FormAlert({ children }: { readonly children: ReactNode }): ReactElement {
  const element = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    element.current?.focus();
  }, [children]);

  return (
    <p ref={element} role="alert" tabIndex={-1} className="text-danger outline-none">
      {children}
    </p>
  );
}

export function FormNotice({
  takeFocus = false,
  children,
}: {
  readonly takeFocus?: boolean;
  readonly children: ReactNode;
}): ReactElement {
  const element = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    const notice = element.current;
    if (!takeFocus || notice === null) {
      return;
    }
    const focused = document.activeElement;
    const focusLost = focused === null || !focused.isConnected || focused.contains(notice);
    const focusNearby = focused !== null && notice.closest('section, article')?.contains(focused) === true;
    if (focusLost || focusNearby) {
      notice.focus();
    }
  }, [takeFocus]);

  return (
    <p ref={element} role="status" tabIndex={takeFocus ? -1 : undefined} className="text-text outline-none">
      {children}
    </p>
  );
}

export function SubmitButton({
  busy,
  quiet = false,
  children,
}: {
  readonly busy: boolean;
  readonly quiet?: boolean;
  readonly children: ReactNode;
}): ReactElement {
  return (
    <button type="submit" disabled={busy} aria-busy={busy} className={quiet ? quietButtonClass : primaryButtonClass}>
      {children}
    </button>
  );
}

export function retryMinutes(seconds: number): number {
  return Math.max(1, Math.ceil(seconds / 60));
}

export function retryAfterSeconds(response: Response): number | null {
  const header = response.headers.get('Retry-After');
  return header === null || !/^[0-9]+$/.test(header) ? null : Number(header);
}

export function stringField(data: FormData, name: string): string {
  const value = data.get(name);
  if (typeof value !== 'string') {
    throw new Error(`the form has no ${name} field`);
  }
  return value;
}

export function errorMessage(t: TFunction, error: ApiError | null, retryAfter: number | null): string {
  if (error?.code === 'rate_limited' && retryAfter !== null) {
    return t('form.throttled', { minutes: retryMinutes(retryAfter) });
  }
  if (error?.code === undefined || error.code === 'rate_limited') {
    return t('form.failed', { requestId: error?.requestId ?? '—' });
  }
  return t(`errors.${error.code}`);
}
