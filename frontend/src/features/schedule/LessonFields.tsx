import { type ReactElement, useId } from 'react';

import { fieldClass } from '../../components/ui/Form.tsx';

export const titleLimit = 120;
export const descriptionLimit = 2000;

export function InputField({
  label,
  name,
  type,
  defaultValue,
  error,
  hintId,
  min,
  max,
  step,
}: {
  readonly label: string;
  readonly name: string;
  readonly type: 'date' | 'time' | 'number';
  readonly defaultValue: string;
  readonly error: string | undefined;
  readonly hintId?: string;
  readonly min?: number;
  readonly max?: number;
  readonly step?: number;
}): ReactElement {
  const inputId = useId();
  const errorId = useId();
  const described = [hintId ?? null, error === undefined ? null : errorId].filter((id) => id !== null).join(' ');
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={inputId} className="text-label">
        {label}
      </label>
      <input
        id={inputId}
        name={name}
        type={type}
        required
        defaultValue={defaultValue}
        min={min}
        max={max}
        step={step}
        aria-invalid={error === undefined ? undefined : true}
        aria-describedby={described === '' ? undefined : described}
        className={fieldClass}
      />
      {error === undefined ? null : (
        <p id={errorId} className="text-small text-danger">
          {error}
        </p>
      )}
    </div>
  );
}
