import { type ReactElement, type ReactNode, type Ref, useEffect, useId, useRef, useState } from 'react';
import type { TFunction } from 'i18next';

import type { ApiError } from '../../lib/api/client.ts';
import { ChevronDownIcon } from './icons.tsx';
import { useStoredValue, valueAccess } from './serverValue.ts';

export const fieldClass =
  'min-h-11 w-full rounded-md border border-border-strong bg-field px-3 text-body-phone text-text aria-invalid:border-danger tablet:text-body';
export const linkClass = 'inline-flex min-h-11 items-center text-accent-text underline';
export const primaryButtonClass =
  'inline-flex min-h-11 items-center justify-center rounded-md bg-accent px-4 text-strong text-on-accent hover:bg-accent-hover aria-disabled:opacity-60';
export const quietButtonClass =
  'inline-flex min-h-11 items-center justify-center rounded-md border border-border-strong px-4 text-strong text-text hover:bg-surface aria-disabled:opacity-60';
export const dangerButtonClass =
  'inline-flex min-h-11 items-center justify-center rounded-md border border-danger px-4 text-strong text-danger hover:bg-surface aria-disabled:opacity-60';
export const textButtonClass =
  'inline-flex min-h-11 items-center px-3 text-small text-accent-text underline aria-disabled:opacity-60';
export const selectClass =
  'h-11 w-full appearance-none rounded-md border border-border-strong bg-field pr-9 pl-3 text-text aria-invalid:border-danger';
export const phoneFullWidthClass = 'flex flex-col tablet:block';

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
  reloads = 0,
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
  readonly reloads?: number;
}): ReactElement {
  const inputId = useId();
  const hintId = useId();
  const errorId = useId();
  const input = useStoredValue<HTMLInputElement>(defaultValue, reloads, valueAccess);
  const [initialValue] = useState(defaultValue);
  const described = [hint === undefined ? null : hintId, error === undefined ? null : errorId]
    .filter((id) => id !== null)
    .join(' ');
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={inputId} className="text-label">
        {label}
      </label>
      <input
        ref={input}
        id={inputId}
        name={name}
        type={type}
        autoComplete={autoComplete}
        required={required}
        minLength={minLength}
        maxLength={maxLength}
        defaultValue={initialValue}
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

const triggerControls = 'button, input[type="file"]';

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
    const focusOnTrigger =
      focused?.matches(triggerControls) === true && notice.closest('form, section')?.contains(focused) === true;
    if (focusLost || focusOnTrigger) {
      notice.focus();
    }
  }, [takeFocus]);

  return (
    <p ref={element} role="status" tabIndex={takeFocus ? -1 : undefined} className="text-text outline-none">
      {children}
    </p>
  );
}

export function ActionButton({
  id,
  busy,
  unavailable = false,
  unavailableReason,
  type = 'button',
  form,
  label,
  firstInRow = false,
  onClick,
  className,
  ref,
  children,
}: {
  readonly id?: string;
  readonly busy: boolean;
  readonly unavailable?: boolean;
  readonly unavailableReason?: string | undefined;
  readonly type?: 'button' | 'submit';
  readonly form?: string;
  readonly label?: string;
  readonly firstInRow?: boolean;
  readonly onClick?: () => void;
  readonly className: string;
  readonly ref?: Ref<HTMLButtonElement>;
  readonly children: ReactNode;
}): ReactElement {
  const refused = busy || unavailable;
  const reasonId = useId();
  return (
    <>
      <button
        id={id}
        ref={ref}
        type={type}
        form={form}
        aria-disabled={refused}
        aria-busy={busy}
        title={unavailableReason}
        aria-describedby={unavailableReason === undefined ? undefined : reasonId}
        aria-label={label}
        data-first-in-row={firstInRow ? '' : undefined}
        onClick={(event) => {
          if (refused) {
            event.preventDefault();
            return;
          }
          onClick?.();
        }}
        className={className}
      >
        {children}
      </button>
      {unavailableReason === undefined ? null : (
        <span id={reasonId} className="sr-only">
          {unavailableReason}
        </span>
      )}
    </>
  );
}

export function FormActions({ children }: { readonly children: ReactNode }): ReactElement {
  return <div className="flex flex-wrap items-center gap-3">{children}</div>;
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
    <ActionButton type="submit" busy={busy} className={quiet ? quietButtonClass : primaryButtonClass}>
      {children}
    </ActionButton>
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
