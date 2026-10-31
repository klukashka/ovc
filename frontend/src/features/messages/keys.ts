export const channelsKey = ['channels'] as const;
export const unreadChannelsKey = ['channels-unread'] as const;
export const channelKey = (id: string): readonly ['channel', string] => ['channel', id];
export const messagesKey = (id: string): readonly ['messages', string] => ['messages', id];
