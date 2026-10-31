import type { ReactElement, ReactNode } from 'react';

function safeAddress(candidate: string): string | null {
  if (/\p{Cf}/u.test(candidate)) return null;
  try {
    const parsed = new URL(candidate);
    return (parsed.protocol === 'https:' || parsed.protocol === 'http:') &&
      parsed.username === '' &&
      parsed.password === ''
      ? parsed.href
      : null;
  } catch {
    return null;
  }
}

export function MessageBody({ body }: { readonly body: string }): ReactElement {
  const parts: ReactNode[] = [];
  let end = 0;
  for (const match of body.matchAll(/\S+/gu)) {
    const start = match.index;
    const token = match[0];
    parts.push(body.slice(end, start));
    const href = /^https?:\/\//iu.test(token) ? safeAddress(token) : null;
    parts.push(
      href === null ? (
        token
      ) : (
        <a
          key={start}
          href={href}
          target="_blank"
          rel="noopener noreferrer nofollow"
          className="text-accent-text underline"
        >
          {token}
        </a>
      ),
    );
    end = start + token.length;
  }
  parts.push(body.slice(end));
  return (
    <p dir="auto" className="whitespace-pre-wrap break-words">
      {parts}
    </p>
  );
}
