import type { ReactElement } from 'react';

import type { ChatPresenceSnapshot } from './presence.ts';

export function TypingIndicator({
  presence,
  someoneTyping,
  namedTyping,
}: {
  readonly presence: ChatPresenceSnapshot;
  readonly someoneTyping: string;
  readonly namedTyping: (name: string) => string;
}): ReactElement {
  return (
    <div role="status" aria-live="polite" className="text-small text-text-muted">
      {presence.typing.map((id) => {
        const peer = presence.peers.find((candidate) => candidate.id === id);
        const label = peer?.label ?? (id === null ? undefined : presence.tutorLabel);
        return (
          <p key={id ?? 'anonymous'} dir="auto">
            {label === undefined ? someoneTyping : namedTyping(label)}
            <span aria-hidden="true" className="motion-safe:animate-pulse">
              …
            </span>
          </p>
        );
      })}
    </div>
  );
}
