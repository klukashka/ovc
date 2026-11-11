import { type ReactElement, type ReactNode, useId, useState } from 'react';

import { fieldClass } from '../../components/ui/Form.tsx';
import { cutToCodePoints, parseInteger } from './blocks.ts';

export function focusId(prefix: string, blockId: string, part: string): string {
  return `${prefix}-${blockId}-${part}`;
}

export function Warnings({
  id,
  messages,
}: {
  readonly id: string;
  readonly messages: readonly string[];
}): ReactElement | null {
  if (messages.length === 0) {
    return null;
  }
  return (
    <div id={id} className="flex flex-col gap-1">
      {messages.map((message) => (
        <p key={message} className="text-small text-warning">
          {message}
        </p>
      ))}
    </div>
  );
}

export function TextArea({
  id,
  label,
  hint,
  value,
  limit,
  warnings,
  rows = 4,
  onChange,
}: {
  readonly id: string;
  readonly label: string;
  readonly hint?: string;
  readonly value: string;
  readonly limit: number;
  readonly warnings: readonly string[];
  readonly rows?: number;
  readonly onChange: (value: string) => void;
}): ReactElement {
  const hintId = useId();
  const warningId = useId();
  const described = [hint === undefined ? null : hintId, warnings.length === 0 ? null : warningId].filter(
    (entry) => entry !== null,
  );
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-strong">
        {label}
      </label>
      <textarea
        id={id}
        value={value}
        rows={rows}
        dir="auto"
        aria-describedby={described.length === 0 ? undefined : described.join(' ')}
        onChange={(event) => {
          onChange(cutToCodePoints(event.target.value, limit));
        }}
        className={`${fieldClass} py-2`}
      />
      {hint === undefined ? null : (
        <p id={hintId} className="text-small text-text-muted">
          {hint}
        </p>
      )}
      <Warnings id={warningId} messages={warnings} />
    </div>
  );
}

export function TextInput({
  id,
  label,
  value,
  limit,
  warnings,
  inputMode,
  onChange,
}: {
  readonly id: string;
  readonly label: string;
  readonly value: string;
  readonly limit: number;
  readonly warnings: readonly string[];
  readonly inputMode?: 'text' | 'decimal';
  readonly onChange: (value: string) => void;
}): ReactElement {
  const warningId = useId();
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-strong">
        {label}
      </label>
      <input
        id={id}
        type="text"
        value={value}
        dir="auto"
        inputMode={inputMode}
        aria-describedby={warnings.length === 0 ? undefined : warningId}
        onChange={(event) => {
          onChange(cutToCodePoints(event.target.value, limit));
        }}
        className={fieldClass}
      />
      <Warnings id={warningId} messages={warnings} />
    </div>
  );
}

export function IntegerField({
  id,
  label,
  value,
  range,
  rangeMessage,
  onChange,
}: {
  readonly id: string;
  readonly label: string;
  readonly value: number;
  readonly range: { readonly min: number; readonly max: number };
  readonly rangeMessage: string;
  readonly onChange: (value: number) => void;
}): ReactElement {
  const [draft, setDraft] = useState<string | null>(null);
  const messageId = useId();
  const invalid = draft !== null && parseInteger(draft, range) === null;
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-strong">
        {label}
      </label>
      <input
        id={id}
        type="text"
        inputMode="numeric"
        value={draft ?? String(value)}
        aria-invalid={invalid}
        aria-describedby={invalid ? messageId : undefined}
        onChange={(event) => {
          const typed = event.target.value;
          const parsed = parseInteger(typed, range);
          setDraft(parsed === null ? typed : null);
          if (parsed !== null) {
            onChange(parsed);
          }
        }}
        className={`${fieldClass} max-w-32`}
      />
      {invalid ? (
        <p id={messageId} className="text-small text-danger">
          {rangeMessage}
        </p>
      ) : null}
    </div>
  );
}

export function Group({
  label,
  describedBy,
  children,
}: {
  readonly label: string;
  readonly describedBy?: string | undefined;
  readonly children: ReactNode;
}): ReactElement {
  return (
    <fieldset aria-describedby={describedBy} className="flex min-w-0 flex-col gap-2">
      <legend className="mb-1 text-strong">{label}</legend>
      {children}
    </fieldset>
  );
}
