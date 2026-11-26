import { type ReactElement, type ReactNode, type RefObject, useEffect, useId, useRef, useState } from 'react';

import { WarningIcon } from './icons.tsx';

const menuButtonClass =
  'inline-flex min-h-11 min-w-11 items-center justify-center rounded-md border border-border-strong px-3 text-strong text-text hover:bg-surface aria-disabled:opacity-60';

const menuItemClass =
  'flex min-h-11 w-full items-center gap-2 rounded-sm px-2.5 text-left text-small hover:bg-hover aria-disabled:opacity-60 desktop:pointer-fine:min-h-8';

const dangerMenuItemClass = `${menuItemClass} text-danger`;

const menuPosition: Readonly<Record<'start' | 'end', string>> = { start: 'left-0', end: 'right-0' };

export interface MenuItem {
  readonly label: string;
  readonly onChoose: () => void;
  readonly danger?: boolean;
}

export function ActionMenu({
  label,
  busy = false,
  unavailable,
  items,
  buttonRef,
  buttonId,
  align = 'end',
  children,
}: {
  readonly label: string;
  readonly busy?: boolean;
  readonly unavailable: boolean;
  readonly items: readonly MenuItem[];
  readonly buttonRef?: RefObject<HTMLButtonElement | null>;
  readonly buttonId?: string;
  readonly align?: 'start' | 'end';
  readonly children: ReactNode;
}): ReactElement {
  const [open, setOpen] = useState(false);
  const menuId = useId();
  const container = useRef<HTMLDivElement>(null);
  const ownButton = useRef<HTMLButtonElement>(null);
  const button = buttonRef ?? ownButton;
  const routineItems = items.filter((item) => item.danger !== true);
  const dangerItems = items.filter((item) => item.danger === true);

  function itemButton(item: MenuItem): ReactElement {
    return (
      <button
        key={item.label}
        type="button"
        aria-disabled={unavailable}
        onClick={() => {
          if (unavailable) {
            return;
          }
          setOpen(false);
          button.current?.focus();
          item.onChoose();
        }}
        className={item.danger === true ? dangerMenuItemClass : menuItemClass}
      >
        {item.danger === true ? <WarningIcon /> : null}
        {item.label}
      </button>
    );
  }

  useEffect(() => {
    if (!open) {
      return undefined;
    }
    const element = container.current;
    function closeOnOutsidePointer(event: PointerEvent): void {
      if (event.target instanceof Node && element?.contains(event.target) !== true) {
        setOpen(false);
      }
    }
    function closeOnEscape(event: KeyboardEvent): void {
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        setOpen(false);
        button.current?.focus();
      }
    }
    function closeOnFocusLeaving(event: FocusEvent): void {
      const target = event.relatedTarget;
      if (target instanceof Node && element?.contains(target) !== true) {
        setOpen(false);
      }
    }
    document.addEventListener('pointerdown', closeOnOutsidePointer);
    element?.addEventListener('keydown', closeOnEscape);
    element?.addEventListener('focusout', closeOnFocusLeaving);
    return (): void => {
      document.removeEventListener('pointerdown', closeOnOutsidePointer);
      element?.removeEventListener('keydown', closeOnEscape);
      element?.removeEventListener('focusout', closeOnFocusLeaving);
    };
  }, [open, button]);

  return (
    <div ref={container} className="relative">
      <button
        id={buttonId}
        ref={button}
        type="button"
        aria-expanded={open}
        aria-controls={menuId}
        aria-label={label}
        aria-disabled={unavailable}
        aria-busy={busy}
        onClick={() => {
          if (!unavailable) {
            setOpen((value) => !value);
          }
        }}
        className={menuButtonClass}
      >
        {children}
      </button>
      {open ? (
        <div
          id={menuId}
          className={`absolute top-full ${menuPosition[align]} z-20 mt-1 w-56 rounded-lg border border-border bg-surface p-1 shadow-raised dark:border-border-strong dark:shadow-none`}
        >
          {routineItems.map(itemButton)}
          {dangerItems.length === 0 || routineItems.length === 0 ? null : <hr className="my-1 border-border" />}
          {dangerItems.map(itemButton)}
        </div>
      ) : null}
    </div>
  );
}
