import { type ReactElement, type ReactNode, type RefObject, useEffect, useLayoutEffect, useRef } from 'react';

import { focusHeadingIfLost } from './LoadState.tsx';

const phoneDialogClass =
  'max-tablet:h-dvh max-tablet:max-h-none max-tablet:w-full max-tablet:max-w-none max-tablet:rounded-none max-tablet:border-0 max-tablet:p-4';

export const dialogClass = `m-auto w-full max-w-md rounded-lg border border-border bg-surface p-6 text-text shadow-raised backdrop:bg-scrim dark:border-border-strong dark:shadow-none ${phoneDialogClass}`;

const wideDialogClass = `m-auto w-full max-w-5xl rounded-lg border border-border bg-surface p-6 text-text shadow-raised backdrop:bg-scrim dark:border-border-strong dark:shadow-none ${phoneDialogClass}`;

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

  useLayoutEffect(() => {
    const element = dialog.current;
    return (): void => {
      if (element?.open === true) {
        element.close();
      }
    };
  }, []);

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
