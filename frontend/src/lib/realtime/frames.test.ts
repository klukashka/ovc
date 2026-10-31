import { describe, expect, test } from 'vitest';

import { parseServerFrame } from './frames.ts';

describe('frames', () => {
  test('parses the message and read frames strictly', () => {
    const accepted = [
      '{"type":"message.new","payload":{"channel_id":"channel-1","message_id":"message-1","seq":7}}',
      '{"type":"message.updated","payload":{"channel_id":"channel-1","message_id":"message-1"}}',
      '{"type":"message.deleted","payload":{"channel_id":"channel-1","message_id":"message-1"}}',
      '{"type":"read.updated","payload":{"channel_id":"channel-1"}}',
    ];
    const refused = [
      '{"type":"message.new","payload":{"channel_id":"channel-1","message_id":"message-1"}}',
      '{"type":"message.new","payload":{"channel_id":"channel-1","message_id":"message-1","seq":0}}',
      '{"type":"message.new","payload":{"channel_id":"channel-1","message_id":"message-1","seq":1.5}}',
      '{"type":"message.new","payload":{"channel_id":"channel-1","message_id":"message-1","seq":"7"}}',
      '{"type":"message.new","payload":{"channel_id":"channel-1","message_id":"message-1","seq":7,"body":"text"}}',
      '{"type":"message.updated","payload":{"channel_id":"channel-1","message_id":"message-1","seq":7}}',
      '{"type":"message.deleted","payload":{"channel_id":"","message_id":"message-1"}}',
      '{"type":"message.deleted","ref":"r","payload":{"channel_id":"channel-1","message_id":"message-1"}}',
      '{"type":"read.updated","payload":{"channel_id":"channel-1","message_id":"message-1"}}',
      '{"type":"read.updated","payload":{}}',
      '{"type":"read.updated"}',
      '{"type":"message.send","payload":{"channel_id":"channel-1"}}',
    ];

    const parsed = accepted.map((text) => parseServerFrame(text));
    const refusals = refused.map((text) => parseServerFrame(text));

    expect(parsed).toEqual([
      { type: 'message.new', channelId: 'channel-1', messageId: 'message-1', seq: 7 },
      { type: 'message.updated', channelId: 'channel-1', messageId: 'message-1' },
      { type: 'message.deleted', channelId: 'channel-1', messageId: 'message-1' },
      { type: 'read.updated', channelId: 'channel-1' },
    ]);
    expect(refusals).toEqual(refused.map(() => null));
  });

  test('parses typing and presence strictly', () => {
    const accepted = [
      '{"type":"typing","payload":{"channel_id":"channel-1","user_id":"user-1"}}',
      '{"type":"typing","payload":{"channel_id":"channel-1","user_id":null}}',
      '{"type":"presence","payload":{"user_id":"user-1","online":true}}',
      '{"type":"presence","payload":{"user_id":"user-1","online":false}}',
    ];
    const refused = [
      '{"type":"typing","payload":{"channel_id":"channel-1"}}',
      '{"type":"typing","payload":{"channel_id":"","user_id":"user-1"}}',
      '{"type":"typing","payload":{"channel_id":"channel-1","user_id":""}}',
      '{"type":"typing","payload":{"channel_id":"channel-1","user_id":7}}',
      '{"type":"typing","payload":{"channel_id":"channel-1","user_id":"user-1","name":"A name"}}',
      '{"type":"typing","ref":"r","payload":{"channel_id":"channel-1","user_id":null}}',
      '{"type":"presence","payload":{"user_id":"user-1"}}',
      '{"type":"presence","payload":{"user_id":"user-1","online":"true"}}',
      '{"type":"presence","payload":{"user_id":null,"online":true}}',
      '{"type":"presence","payload":{"user_id":"user-1","online":true,"last_seen_at":"2026-10-30T10:00:00Z"}}',
      '{"type":"typing.start","payload":{"channel_id":"channel-1"}}',
    ];

    const parsed = accepted.map((text) => parseServerFrame(text));
    const refusals = refused.map((text) => parseServerFrame(text));

    expect(parsed).toEqual([
      { type: 'typing', channelId: 'channel-1', userId: 'user-1' },
      { type: 'typing', channelId: 'channel-1', userId: null },
      { type: 'presence', userId: 'user-1', online: true },
      { type: 'presence', userId: 'user-1', online: false },
    ]);
    expect(refusals).toEqual(refused.map(() => null));
  });
});
