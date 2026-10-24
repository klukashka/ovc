import { type ReactElement, useEffect, useRef, useState } from 'react';

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

export function FocusedNotice({ message }: { readonly message: string }): ReactElement {
  const element = useRef<HTMLParagraphElement>(null);

  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      element.current?.focus();
    });
    return (): void => {
      window.cancelAnimationFrame(frame);
    };
  }, []);

  return (
    <p ref={element} role="status" tabIndex={-1} className="text-text outline-none">
      {message}
    </p>
  );
}
