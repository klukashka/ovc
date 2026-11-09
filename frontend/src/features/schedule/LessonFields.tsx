import { type ReactElement, type RefObject, useId } from 'react';

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
  alignRows = false,
  inputRef,
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
  readonly alignRows?: boolean;
  readonly inputRef?: RefObject<HTMLInputElement | null>;
}): ReactElement {
  const inputId = useId();
  const errorId = useId();
  const described = [hintId ?? null, error === undefined ? null : errorId].filter((id) => id !== null).join(' ');
  return (
    <div className={alignRows ? 'row-span-3 grid grid-rows-subgrid gap-1' : 'flex flex-col gap-1'}>
      <label htmlFor={inputId} className="text-label">
        {label}
      </label>
      <input
        ref={inputRef}
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
