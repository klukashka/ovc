import { type ReactElement, type RefObject, useEffect, useRef } from 'react';

function focusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(
    container.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])'),
  );
}

export function FrameDialog({
  open,
  onClose,
  returnFocus,
  label,
  className,
  children,
}: {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly returnFocus: RefObject<HTMLButtonElement | null>;
  readonly label: string;
  readonly className: string;
  readonly children: ReactElement;
}): ReactElement {
  const dialog = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const element = dialog.current;
    if (element === null) {
      return undefined;
    }
    const opened: HTMLDialogElement = element;
    function cancel(event: Event): void {
      event.preventDefault();
      if (opened.querySelector('[aria-expanded="true"]') !== null) {
        return;
      }
      onClose();
    }
    function closeOnScrim(event: MouseEvent): void {
      if (event.target === element) {
        onClose();
      }
    }
    function keepFocusInside(event: KeyboardEvent): void {
      if (event.key !== 'Tab' || element === null) {
        return;
      }
      const focusable = focusableElements(element);
      const first = focusable[0];
      const last = focusable.at(-1);
      if (first === undefined || last === undefined) {
        return;
      }
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }
    function syncClosed(): void {
      onClose();
      const active = document.activeElement;
      if (active === null || active === document.body || opened.contains(active)) {
        returnFocus.current?.focus();
      }
    }
    element.addEventListener('close', syncClosed);
    element.addEventListener('cancel', cancel);
    element.addEventListener('click', closeOnScrim);
    element.addEventListener('keydown', keepFocusInside);
    return (): void => {
      element.removeEventListener('close', syncClosed);
      element.removeEventListener('cancel', cancel);
      element.removeEventListener('click', closeOnScrim);
      element.removeEventListener('keydown', keepFocusInside);
    };
  }, [onClose, returnFocus]);

  useEffect(() => {
    const element = dialog.current;
    if (element === null) {
      return;
    }
    if (open && !element.open) {
      element.showModal();
      const current = element.querySelector<HTMLElement>('[aria-current="page"]') ?? focusableElements(element)[0];
      current?.focus();
    }
    if (!open && element.open) {
      element.close();
      returnFocus.current?.focus();
    }
  }, [open, returnFocus]);

  return (
    <dialog ref={dialog} aria-label={label} data-frame-only="" className={className}>
      {children}
    </dialog>
  );
}
