import { type ReactElement, type ReactNode, type RefObject, useEffect, useRef } from 'react';

import { focusHeadingIfLost } from './LoadState.tsx';

export const dialogClass =
  'm-auto w-full max-w-md rounded-lg border border-border bg-surface p-6 text-text shadow-raised backdrop:bg-scrim dark:border-border-strong dark:shadow-none';

const wideDialogClass =
  'm-auto w-full max-w-5xl rounded-lg border border-border bg-surface p-6 text-text shadow-raised backdrop:bg-scrim dark:border-border-strong dark:shadow-none';

export function returnFocusTo(target: HTMLElement | null): void {
  if (target?.isConnected === true) {
    target.focus();
    return;
  }
  focusHeadingIfLost();
}

export function ModalDialog({
  open,
  busy,
  labelledBy,
  describedBy,
  returnFocus,
  initialFocus,
  wide = false,
  onClose,
  children,
}: {
  readonly open: boolean;
  readonly busy: boolean;
  readonly labelledBy: string;
  readonly describedBy?: string;
  readonly returnFocus: RefObject<HTMLElement | null>;
  readonly initialFocus?: RefObject<HTMLElement | null>;
  readonly wide?: boolean;
  readonly onClose: () => void;
  readonly children: ReactNode;
}): ReactElement {
  const dialog = useRef<HTMLDialogElement>(null);
  const focusReturnedOnClose = useRef(false);

  useEffect(() => {
    const element = dialog.current;
    if (element === null) {
      return;
    }
    if (open && !element.open) {
      focusReturnedOnClose.current = false;
      element.showModal();
      initialFocus?.current?.focus();
    }
    if (!open && element.open) {
      focusReturnedOnClose.current = true;
      element.close();
      returnFocusTo(returnFocus.current);
    }
  }, [open, returnFocus, initialFocus]);

  return (
    <dialog
      ref={dialog}
      aria-labelledby={labelledBy}
      aria-describedby={describedBy}
      onCancel={(event) => {
        if (busy) {
          event.preventDefault();
        }
      }}
      onClose={() => {
        if (dialog.current?.open === true) {
          return;
        }
        if (busy) {
          dialog.current?.showModal();
          return;
        }
        onClose();
        if (focusReturnedOnClose.current) {
          focusReturnedOnClose.current = false;
          return;
        }
        returnFocusTo(returnFocus.current);
      }}
      className={wide ? wideDialogClass : dialogClass}
    >
      {open ? children : null}
    </dialog>
  );
}
