import type { ReactElement } from 'react';

import type { Me } from '../../lib/api/session.ts';
import type { ChannelItem } from './api.ts';
import { type ChatPresenceSnapshot, useChatSignals } from './presence.ts';

export function PresenceSummary({
  presence,
  group,
  viewerRole,
  onlineLabel,
  membersLabel,
  collapsed = false,
}: {
  readonly presence: ChatPresenceSnapshot;
  readonly group: boolean;
  readonly viewerRole: Me['role'];
  readonly onlineLabel: string;
  readonly membersLabel: string;
  readonly collapsed?: boolean;
}): ReactElement | null {
  if (group && viewerRole === 'tutor') {
    if (presence.peers.length === 0) return null;
    const members = (
      <ul aria-label={membersLabel} className="flex flex-wrap gap-3 text-small">
        {presence.peers.map((peer) => (
          <li key={peer.id} className="min-w-0 [overflow-wrap:anywhere]">
            <span dir="auto">{peer.label}</span>
            {peer.online === true ? <span> · {onlineLabel}</span> : null}
          </li>
        ))}
      </ul>
    );
    return collapsed ? (
      <details className="w-full">
        <summary className="min-h-11 cursor-pointer py-2.5 text-accent-text">{membersLabel}</summary>
        {members}
      </details>
    ) : (
      members
    );
  }
  return !group && presence.peers.some((peer) => peer.online === true) ? (
    <span className="block text-small">{onlineLabel}</span>
  ) : null;
}

export function ChannelPresence({
  channel,
  me,
  revision,
  onlineLabel,
}: {
  readonly channel: ChannelItem;
  readonly me: Pick<Me, 'id' | 'role'>;
  readonly revision: string | number;
  readonly onlineLabel: string;
}): ReactElement | null {
  const presence = useChatSignals(channel, me.id, me.role, revision);
  return channel.kind === 'direct' && presence.peers.some((peer) => peer.online === true) ? (
    <span className="block text-small">{onlineLabel}</span>
  ) : null;
}
