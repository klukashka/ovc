export type ChannelFilter = 'all' | 'direct' | 'group' | 'unread';

export function channelFilter(search: string): ChannelFilter {
  const value = new URLSearchParams(search).get('filter');
  return value === 'direct' || value === 'group' || value === 'unread' ? value : 'all';
}

export function channelFilterSearch(filter: ChannelFilter): string {
  return filter === 'all' ? '' : `?filter=${filter}`;
}
