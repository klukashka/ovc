import { type ReactElement, type ReactNode, type RefObject, useEffect, useRef, useState } from 'react';

import { focusIsLost } from './LoadState.tsx';

export function LiveNotice({ message }: { readonly message: string | null }): ReactElement {
  const [announced, setAnnounced] = useState<string | null>(null);

  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      setAnnounced(message);
    });
    return (): void => {
      window.cancelAnimationFrame(frame);
    };
  }, [message]);

  return (
    <p role="status" className={message !== null && announced !== null ? 'text-text' : 'sr-only'}>
      {message === null ? null : announced}
    </p>
  );
}

export function useFocusOnMount(element: RefObject<HTMLElement | null>, whenFocusIsLost = false): void {
  useEffect(() => {
    const notice = element.current;
    if (notice === null) {
      return undefined;
    }
    const takeFocus = (): void => {
      if (notice.isConnected && (!whenFocusIsLost || focusIsLost())) {
        notice.focus();
      }
    };
    const closingDialogs = [...document.querySelectorAll('dialog[open]')];
    takeFocus();
    if (closingDialogs.length === 0 || document.activeElement === notice) {
      return undefined;
    }
    const stopped = new AbortController();
    queueMicrotask(() => {
      if (stopped.signal.aborted || closingDialogs.some((dialog) => dialog.hasAttribute('open'))) {
        return;
      }
      takeFocus();
      const left = document.activeElement;
      if (left === notice) {
        return;
      }
      for (const dialog of closingDialogs) {
        dialog.addEventListener(
          'close',
          () => {
            if (document.activeElement === left || focusIsLost()) {
              takeFocus();
            }
          },
          { once: true, signal: stopped.signal },
        );
      }
    });
    return (): void => {
      stopped.abort();
    };
  }, [element, whenFocusIsLost]);
}

export function FocusedNotice({
  message,
  whenFocusIsLost = false,
}: {
  readonly message: ReactNode;
  readonly whenFocusIsLost?: boolean;
}): ReactElement {
  const element = useRef<HTMLParagraphElement>(null);
  useFocusOnMount(element, whenFocusIsLost);

  return (
    <p ref={element} role="status" tabIndex={-1} className="text-text outline-none">
      {message}
    </p>
  );
}
