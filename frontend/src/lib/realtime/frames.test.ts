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
});
