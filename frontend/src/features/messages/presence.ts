import { createContext, useContext, useMemo, useSyncExternalStore } from 'react';

import { useScheduler } from '../../app/clock.ts';
import type { Me, Scheduler } from '../../lib/api/session.ts';
import { ChatSignals, type ChatSignalEvent, chatSignals } from '../../lib/realtime/chatSignals.ts';
import type { Channel, ChannelItem } from './api.ts';

export interface PresencePeer {
  readonly id: string;
  readonly label: string;
  readonly online: boolean | null;
}

export interface ChatPresenceSnapshot {
  readonly peers: readonly PresencePeer[];
  readonly typing: readonly (string | null)[];
  readonly tutorLabel?: string;
}

function peersOf(channel: Channel | ChannelItem, ownId: string, role: Me['role']): readonly PresencePeer[] {
  if (!channel.writable || (channel.kind === 'group' && role === 'student')) return [];
  if (role === 'tutor' && channel.kind === 'group') {
    return 'members' in channel
      ? (channel.members ?? [])
          .filter((member) => member.user_id !== ownId)
          .map((member) => ({ id: member.user_id, label: member.label, online: member.online }))
      : [];
  }
  if (channel.user_id === null || channel.user_id === ownId) return [];
  const label = channel.kind === 'group' ? channel.tutor?.name : channel.label;
  return label === null || label === undefined ? [] : [{ id: channel.user_id, label, online: channel.online }];
}

export class ChatPresence {
  private current: ChatPresenceSnapshot;
  private active = true;
  private typingSent = false;
  private readonly listeners = new Set<() => void>();
  private readonly expiry = new Map<string | null, () => void>();
  private unsubscribe: (() => void) | null = null;

  private readonly channel: Channel | ChannelItem;
  private readonly ownId: string;
  private readonly role: Me['role'];
  private readonly signals: ChatSignals;
  private readonly scheduler: Scheduler;

  constructor(
    channel: Channel | ChannelItem,
    ownId: string,
    role: Me['role'],
    signals: ChatSignals,
    scheduler: Scheduler,
  ) {
    this.channel = channel;
    this.ownId = ownId;
    this.role = role;
    this.signals = signals;
    this.scheduler = scheduler;
    this.current = {
      peers: peersOf(channel, ownId, role),
      typing: [],
      ...(channel.writable && channel.kind === 'group' && role === 'student' && channel.tutor !== null
        ? { tutorLabel: channel.tutor.name }
        : {}),
    };
  }

  readonly snapshot = (): ChatPresenceSnapshot => this.current;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    this.unsubscribe ??= this.signals.subscribe((event) => {
      this.receive(event);
    });
    return () => {
      this.listeners.delete(listener);
      if (this.listeners.size === 0) {
        this.stopTyping();
        this.unsubscribe?.();
        this.unsubscribe = null;
        this.cancelTimers();
        this.current = { ...this.current, typing: [] };
      }
    };
  };

  readonly startTyping = (): void => {
    if (this.active && this.channel.writable && this.signals.send(this.channel.id, true)) this.typingSent = true;
  };

  readonly stopTyping = (): void => {
    if (this.active && this.channel.writable && this.typingSent) this.signals.send(this.channel.id, false);
    this.typingSent = false;
  };

  private receive(event: ChatSignalEvent): void {
    if (event.type === 'clear') {
      if (event.channelId !== undefined && event.channelId !== this.channel.id) return;
      this.active = false;
      this.typingSent = false;
      this.cancelTimers();
      this.publish({ peers: [], typing: [] });
      return;
    }
    if (!this.active || !this.channel.writable) return;
    if (event.type === 'presence') {
      if (!this.current.peers.some((peer) => peer.id === event.userId && peer.online !== null)) return;
      this.publish({
        ...this.current,
        peers: this.current.peers.map((peer) => (peer.id === event.userId ? { ...peer, online: event.online } : peer)),
      });
      return;
    }
    if (event.channelId !== this.channel.id || event.userId === this.ownId) return;
    const identity = this.current.tutorLabel !== undefined && event.userId !== null ? 'group-tutor' : event.userId;
    if (event.userId === null) {
      if (this.role !== 'student' || this.channel.kind !== 'group') return;
    } else if (this.current.tutorLabel === undefined && !this.current.peers.some((peer) => peer.id === event.userId))
      return;
    this.expiry.get(identity)?.();
    this.expiry.set(
      identity,
      this.scheduler.schedule(5_000, () => {
        this.expiry.delete(identity);
        this.publish({ ...this.current, typing: this.current.typing.filter((id) => id !== identity) });
      }),
    );
    if (!this.current.typing.includes(identity)) {
      this.publish({ ...this.current, typing: [...this.current.typing, identity] });
    }
  }

  private cancelTimers(): void {
    for (const cancel of this.expiry.values()) cancel();
    this.expiry.clear();
  }

  private publish(snapshot: ChatPresenceSnapshot): void {
    this.current = snapshot;
    for (const listener of this.listeners) listener();
  }
}

export const ChatSignalsContext = createContext(chatSignals);

export function useChatSignals(
  channel: Channel | ChannelItem,
  ownId: string,
  role: Me['role'],
  revision: string | number,
): ChatPresenceSnapshot & Pick<ChatPresence, 'startTyping' | 'stopTyping'> {
  const signals = useContext(ChatSignalsContext);
  const scheduler = useScheduler();
  const presence = useMemo(
    () => ({ revision, state: new ChatPresence(channel, ownId, role, signals, scheduler) }),
    [channel, ownId, role, signals, scheduler, revision],
  ).state;
  const snapshot = useSyncExternalStore(presence.subscribe, presence.snapshot);
  return { ...snapshot, startTyping: presence.startTyping, stopTyping: presence.stopTyping };
}
