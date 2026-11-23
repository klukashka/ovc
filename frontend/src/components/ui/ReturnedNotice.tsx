import { type ReactElement, type RefObject, useEffect, useRef } from 'react';

import { focusIsOnHeadingOrLost } from './formRoute.ts';

export function useFocusAfterReturn(element: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    let active = true;
    queueMicrotask(() => {
      if (active && element.current?.isConnected === true && focusIsOnHeadingOrLost()) {
        element.current.focus();
      }
    });
    return (): void => {
      active = false;
    };
  }, [element]);
}

export function ReturnedNotice({ message }: { readonly message: string }): ReactElement {
  const element = useRef<HTMLParagraphElement>(null);
  useFocusAfterReturn(element);

  return (
    <p ref={element} role="status" tabIndex={-1} className="text-text outline-none">
      {message}
    </p>
  );
}
