import { type ReactElement, type ReactNode, type RefObject, useEffect, useId, useRef, useState } from 'react';

const menuButtonClass =
  'inline-flex min-h-11 min-w-11 items-center justify-center rounded-md border border-border-strong px-3 text-strong text-text hover:bg-surface aria-disabled:opacity-60';

export interface MenuItem {
  readonly label: string;
  readonly onChoose: () => void;
}

export function ActionMenu({
  label,
  busy = false,
  unavailable,
  items,
  buttonRef,
  children,
}: {
  readonly label: string;
  readonly busy?: boolean;
  readonly unavailable: boolean;
  readonly items: readonly MenuItem[];
  readonly buttonRef?: RefObject<HTMLButtonElement | null>;
  readonly children: ReactNode;
}): ReactElement {
  const [open, setOpen] = useState(false);
  const menuId = useId();
  const container = useRef<HTMLDivElement>(null);
  const ownButton = useRef<HTMLButtonElement>(null);
  const button = buttonRef ?? ownButton;

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
          className="absolute top-full right-0 z-20 mt-1 w-56 rounded-lg border border-border bg-surface p-1 shadow-raised dark:border-border-strong dark:shadow-none"
        >
          {items.map((item) => (
            <button
              key={item.label}
              type="button"
              onClick={() => {
                setOpen(false);
                button.current?.focus();
                item.onChoose();
              }}
              className="flex min-h-11 w-full items-center rounded-sm px-2.5 text-left text-small hover:bg-hover desktop:pointer-fine:min-h-8"
            >
              {item.label}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
