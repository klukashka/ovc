import type { ReactElement, ReactNode } from 'react';

import { dateTimeFormat } from '../../lib/format/dates.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import type { Message, MessageQuote } from './api.ts';
import { Attachments } from './Attachments.tsx';
import { ReadTicksIcon } from './icons.tsx';
import { Reactions } from './Reactions.tsx';
import type { ReactionValue } from './api.ts';
import { MessageBody } from './MessageBody.tsx';
import type { MessagePlace } from './messageGroups.ts';
import { Quote } from './Quote.tsx';
import type { ChatTexts } from './translations.ts';

function bubbleCorners(own: boolean, place: MessagePlace): string {
  const near = own ? ['rounded-tr-sm', 'rounded-br-sm'] : ['rounded-tl-sm', 'rounded-bl-sm'];
  return [place.firstOfGroup ? '' : near[0], place.lastOfGroup ? '' : near[1]].join(' ');
}

export function MessageRow({
  message,
  place,
  texts,
  language,
  timeZone,
  groupChat,
  menu,
  refresh,
  onJump,
  peerReadSeq,
  reactable = false,
  onToggleReaction = (): void => undefined,
  tracked = true,
}: {
  readonly message: Message;
  readonly place: MessagePlace;
  readonly texts: ChatTexts;
  readonly language: Language;
  readonly timeZone: string;
  readonly groupChat: boolean;
  readonly menu: ReactNode;
  readonly refresh: () => Promise<boolean>;
  readonly onJump: (quote: MessageQuote) => void;
  readonly peerReadSeq: number | null;
  readonly reactable?: boolean;
  readonly onToggleReaction?: (reaction: ReactionValue, mine: boolean) => void;
  readonly tracked?: boolean;
}): ReactElement {
  const own = message.own;
  const sender = own ? texts.you : (message.sender_label ?? texts.deletedUser);
  const senderShown = groupChat && !own && place.firstOfGroup;
  const ticked = own && !message.deleted && !message.erased && peerReadSeq !== null;
  const read = peerReadSeq !== null && peerReadSeq >= message.seq;
  const time = dateTimeFormat(language, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23', timeZone }).format(
    new Date(message.created_at),
  );
  return (
    <li
      data-row={message.id}
      data-seq={tracked ? message.seq : undefined}
      data-window-seq={tracked ? undefined : message.seq}
      data-own={own ? '' : undefined}
      tabIndex={-1}
      className={`group flex flex-col outline-none focus-visible:ring-2 ${place.firstOfGroup ? 'pt-2' : 'pt-0.5'}`}
    >
      {place.firstOfDay ? (
        <p className="my-2 self-center rounded-full bg-hover px-3 py-0.5 text-caption text-text-muted">{place.day}</p>
      ) : null}
      <div
        className={`flex max-w-[85%] items-end gap-1 desktop:max-w-[72%] ${own ? 'flex-row-reverse self-end' : 'self-start'}`}
      >
        <div
          data-bubble=""
          className={`flex min-w-0 flex-col gap-1 rounded-lg px-3 py-1.5 ${own ? 'bg-mine' : 'border border-border bg-surface'} ${bubbleCorners(own, place)}`}
        >
          <span dir="auto" className={senderShown ? 'text-label text-accent-text [overflow-wrap:anywhere]' : 'sr-only'}>
            {sender}
          </span>
          {message.reply_to === null || message.deleted || message.erased ? null : (
            <Quote quote={message.reply_to} texts={texts} onJump={onJump} />
          )}
          {message.deleted ? (
            <p className="text-text-muted italic">{texts.deleted}</p>
          ) : message.erased ? (
            <p className="text-text-muted italic">{texts.erased}</p>
          ) : message.body === null || message.body === '' ? null : (
            <MessageBody body={message.body} />
          )}
          <Attachments message={message} texts={texts} language={language} refresh={refresh} />
          {message.sent_before_reclaim ? <p className="text-small text-text-muted">{texts.reclaimed}</p> : null}
          {message.deleted || message.erased ? null : (
            <Reactions reactions={message.reactions} texts={texts} writable={reactable} onToggle={onToggleReaction} />
          )}
          <span className="flex justify-end gap-1 text-caption text-text-muted">
            {message.edited_at === null || message.deleted || message.erased ? null : <span>{texts.edited}</span>}
            <time dateTime={message.created_at}>{time}</time>
            {ticked ? (
              <span data-ticks={read ? 'read' : 'sent'} className="flex items-center">
                <ReadTicksIcon read={read} />
                <span className="sr-only">{read ? texts.readTick : texts.sentTick}</span>
              </span>
            ) : null}
          </span>
        </div>
        {menu}
      </div>
    </li>
  );
}
