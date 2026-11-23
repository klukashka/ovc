import type { ChannelItem } from './api.ts';
import type { ChatTexts } from './translations.ts';

export function channelLabel(channel: Pick<ChannelItem, 'label' | 'label_kind'>, texts: ChatTexts): string {
  if (channel.label !== null) {
    return channel.label;
  }
  switch (channel.label_kind) {
    case 'former_student':
      return texts.formerStudent;
    case 'removed_student':
      return texts.removedStudent;
    case 'deleted_user':
    case null:
      return texts.deletedUser;
  }
}
