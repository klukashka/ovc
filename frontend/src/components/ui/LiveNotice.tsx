import { type ReactElement, useEffect, useRef, useState } from 'react';

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

export function FocusedNotice({
  message,
  whenFocusIsLost = false,
}: {
  readonly message: string;
  readonly whenFocusIsLost?: boolean;
}): ReactElement {
  const element = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      if (!whenFocusIsLost || focusIsLost()) {
        element.current?.focus();
      }
    });
    return (): void => {
      window.cancelAnimationFrame(frame);
    };
  }, [whenFocusIsLost]);

  return (
    <p ref={element} role="status" tabIndex={-1} className="text-text outline-none">
      {message}
    </p>
  );
}
