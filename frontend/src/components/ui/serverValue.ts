import { type RefObject, useEffect, useRef } from 'react';

type ValueField = HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement;

export interface FieldAccess<Field extends HTMLElement> {
  readonly read: (field: Field) => string;
  readonly write: (field: Field, value: string) => void;
}

export const valueAccess: FieldAccess<ValueField> = {
  read: (field) => field.value,
  write: (field, value) => {
    field.value = value;
  },
};

export function useAddressValue<Field extends ValueField>(value: string): RefObject<Field | null> {
  const field = useRef<Field>(null);
  const shown = useRef(value);

  useEffect(() => {
    if (shown.current === value) {
      return;
    }
    shown.current = value;
    if (field.current !== null) {
      field.current.value = value;
    }
  }, [value]);

  return field;
}

export function useStoredValue<Field extends HTMLElement>(
  value: string | undefined,
  reloads: number,
  access: FieldAccess<Field>,
): RefObject<Field | null> {
  const field = useRef<Field>(null);
  const stored = useRef(value);
  const written = useRef(value);
  const reloadsSeen = useRef(reloads);

  useEffect(() => {
    const element = field.current;
    if (element === null || value === undefined) {
      return;
    }
    if (value !== stored.current) {
      stored.current = value;
      const shown = access.read(element);
      if (shown === value) {
        written.current = value;
      } else if (shown === written.current) {
        access.write(element, value);
        written.current = value;
      }
    }
    if (reloads !== reloadsSeen.current) {
      reloadsSeen.current = reloads;
      if (stored.current !== written.current) {
        access.write(element, value);
        written.current = value;
      }
    }
  }, [value, reloads, access]);

  return field;
}
