import { describe, expect, test } from 'vitest';

import { manualScheduler } from '../../lib/api/fakeTransport.ts';
import { ChatSignals } from '../../lib/realtime/chatSignals.ts';
import type { Channel } from './api.ts';
import { ChatPresence } from './presence.ts';

const group: Channel = {
  id: 'group',
  kind: 'group',
  colour: null,
  label: 'Group',
  label_kind: null,
  last_message: null,
  online: true,
  tutor: { name: 'Tutor', colour: null },
  unconfirmed: false,
  unread_count: 0,
  pinned: null,
  user_id: 'tutor',
  writable: true,
  members: [
    { user_id: 'student', label: 'Card name', online: false },
    { user_id: 'other', label: 'Other card', online: true },
  ],
};

function rig(
  role: 'tutor' | 'student',
  channel = group,
): {
  readonly signals: ChatSignals;
  readonly clock: ReturnType<typeof manualScheduler>;
  readonly state: ChatPresence;
  readonly release: () => void;
  readonly sent: { channel: string; typing: boolean }[];
} {
  const signals = new ChatSignals();
  const clock = manualScheduler();
  const sent: { channel: string; typing: boolean }[] = [];
  signals.connect((channel, typing) => {
    sent.push({ channel, typing });
    return true;
  });
  const state = new ChatPresence(channel, role, role, signals, clock);
  const release = state.subscribe(() => undefined);
  return { signals, clock, state, release, sent };
}

describe('authorized presence and ephemeral typing', () => {
  test('student ignores a roster while anonymous group typing expires after its last frame', () => {
    const { state, signals, clock } = rig('student');

    signals.publish({ type: 'presence', userId: 'other', online: false });
    signals.publish({ type: 'typing', channelId: 'group', userId: null });
    clock.pass(4_000);
    signals.publish({ type: 'typing', channelId: 'group', userId: null });
    clock.pass(4_999);
    const beforeExpiry = state.snapshot();
    clock.pass(1);

    expect(beforeExpiry.peers).toEqual([]);
    expect(beforeExpiry.typing).toEqual([null]);
    expect(state.snapshot().typing).toEqual([]);
    expect(clock.pendingDelays()).toEqual([]);
  });

  test('tutor uses only current card labels and bounded known peers', () => {
    const { state, signals } = rig('tutor');

    signals.publish({ type: 'presence', userId: 'student', online: true });
    signals.publish({ type: 'typing', channelId: 'group', userId: 'student' });
    for (let index = 0; index < 100; index += 1) {
      signals.publish({ type: 'typing', channelId: 'group', userId: `unknown-${String(index)}` });
    }
    signals.publish({ type: 'typing', channelId: 'elsewhere', userId: 'other' });
    signals.publish({ type: 'typing', channelId: 'group', userId: null });

    expect(state.snapshot().peers[0]).toEqual({ id: 'student', label: 'Card name', online: true });
    expect(state.snapshot().typing).toEqual(['student']);
  });

  test('scope invalidation clears state and ignores late frames until a fresh REST scope exists', () => {
    const { state, signals, clock, sent } = rig('tutor');

    signals.publish({ type: 'typing', channelId: 'group', userId: 'student' });
    signals.clear();
    signals.publish({ type: 'typing', channelId: 'group', userId: 'student' });
    signals.publish({ type: 'presence', userId: 'student', online: true });
    state.startTyping();
    const refreshed = new ChatPresence(group, 'tutor', 'tutor', signals, clock);
    const release = refreshed.subscribe(() => undefined);
    signals.publish({ type: 'typing', channelId: 'group', userId: 'student' });
    const refreshedSnapshot = refreshed.snapshot();
    release();

    expect(state.snapshot()).toEqual({ peers: [], typing: [] });
    expect(refreshedSnapshot.typing).toEqual(['student']);
    expect(sent).toEqual([]);
    expect(clock.pendingDelays()).toEqual([]);
  });

  test('read-only and disconnected scopes neither show transient identities nor send typing', () => {
    const readonlyRig = rig('tutor', { ...group, writable: false });
    const connected = rig('tutor');

    readonlyRig.signals.publish({ type: 'typing', channelId: 'group', userId: 'student' });
    readonlyRig.state.startTyping();
    connected.state.startTyping();
    connected.signals.disconnect();
    connected.state.startTyping();
    connected.state.stopTyping();

    expect(readonlyRig.state.snapshot()).toEqual({ peers: [], typing: [] });
    expect(readonlyRig.sent).toEqual([]);
    expect(connected.state.snapshot()).toEqual({ peers: [], typing: [] });
    expect(connected.sent).toEqual([{ channel: 'group', typing: true }]);
  });

  test('unmount stops typing and releases indicator timers', () => {
    const { state, signals, clock, release, sent } = rig('tutor');

    state.startTyping();
    signals.publish({ type: 'typing', channelId: 'group', userId: 'student' });
    release();

    expect(sent).toEqual([
      { channel: 'group', typing: true },
      { channel: 'group', typing: false },
    ]);
    expect(clock.pendingDelays()).toEqual([]);
  });
});

test('student group uses the server tutor projection without retaining identifiers or discovering a roster', () => {
  const { state, signals, clock } = rig('student', { ...group, user_id: null, online: null });

  for (let index = 0; index < 100; index += 1) {
    signals.publish({ type: 'typing', channelId: 'group', userId: `server-tutor-${String(index)}` });
  }
  signals.publish({ type: 'typing', channelId: 'group', userId: null });

  expect(state.snapshot()).toEqual({ peers: [], typing: ['group-tutor', null], tutorLabel: 'Tutor' });
  expect(clock.pendingDelays()).toEqual([5_000, 5_000]);
});

test('a channel scope change clears only that channel while unrelated presence remains live', () => {
  const { state, signals } = rig('tutor');

  signals.publish({ type: 'typing', channelId: 'group', userId: 'student' });
  signals.clear('another-channel');
  signals.publish({ type: 'presence', userId: 'student', online: true });
  const unaffected = state.snapshot();
  signals.clear('group');
  signals.publish({ type: 'typing', channelId: 'group', userId: 'student' });

  expect(unaffected.typing).toEqual(['student']);
  expect(unaffected.peers[0]?.online).toBe(true);
  expect(state.snapshot()).toEqual({ peers: [], typing: [] });
});
