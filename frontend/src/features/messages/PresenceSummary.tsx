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
}: {
  readonly presence: ChatPresenceSnapshot;
  readonly group: boolean;
  readonly viewerRole: Me['role'];
  readonly onlineLabel: string;
  readonly membersLabel: string;
}): ReactElement | null {
  if (group && viewerRole === 'tutor') {
    return presence.peers.length === 0 ? null : (
      <ul aria-label={membersLabel} className="flex flex-wrap gap-3 text-small">
        {presence.peers.map((peer) => (
          <li key={peer.id}>
            <span dir="auto">{peer.label}</span>
            {peer.online === true ? <span> · {onlineLabel}</span> : null}
          </li>
        ))}
      </ul>
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
