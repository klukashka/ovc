import type { ReactElement } from 'react';

import { messageReactionReactionValues } from '../../lib/api/schema.ts';

import type { MessageReaction, ReactionValue } from './api.ts';
import type { ChatTexts } from './translations.ts';

export const reactionValues: readonly ReactionValue[] = messageReactionReactionValues;

const emoji: Readonly<Record<ReactionValue, string>> = {
  thumbs_up: '👍',
  heart: '❤️',
  joy: '😂',
  wow: '😮',
  sad: '😢',
  check: '✅',
};

export function reactionEmoji(reaction: ReactionValue): string {
  return emoji[reaction];
}

export function reactionName(texts: ChatTexts, reaction: ReactionValue): string {
  switch (reaction) {
    case 'thumbs_up':
      return texts.reactionThumbsUp;
    case 'heart':
      return texts.reactionHeart;
    case 'joy':
      return texts.reactionJoy;
    case 'wow':
      return texts.reactionWow;
    case 'sad':
      return texts.reactionSad;
    case 'check':
      return texts.reactionCheck;
  }
}

function reactorNames(texts: ChatTexts, reaction: MessageReaction): string | null {
  if (reaction.members === undefined || reaction.members.length === 0) return null;
  return reaction.members.map((member) => member.label ?? texts.deletedUser).join(', ');
}

export function Reactions({
  reactions,
  texts,
  writable,
  onToggle,
}: {
  readonly reactions: readonly MessageReaction[];
  readonly texts: ChatTexts;
  readonly writable: boolean;
  readonly onToggle: (reaction: ReactionValue, mine: boolean) => void;
}): ReactElement | null {
  if (reactions.length === 0) return null;
  return (
    <ul aria-label={texts.reactions} className="flex flex-wrap gap-1">
      {reactions.map((reaction) => {
        const names = reactorNames(texts, reaction);
        const content = (
          <>
            <span aria-hidden="true">{reactionEmoji(reaction.reaction)}</span>
            <span className="sr-only">{reactionName(texts, reaction.reaction)}</span>
            <span>{reaction.count}</span>
            {reaction.mine ? <span className="sr-only">{texts.yourReaction}</span> : null}
            {names === null ? null : <span className="sr-only">{names}</span>}
          </>
        );
        const chipClass = `inline-flex min-h-8 items-center gap-1 rounded-full border px-2 text-small ${
          reaction.mine ? 'border-accent bg-mine' : 'border-border bg-surface'
        }`;
        return (
          <li key={reaction.reaction} data-reaction={reaction.reaction}>
            {writable ? (
              <button
                type="button"
                className={chipClass}
                title={names ?? undefined}
                aria-pressed={reaction.mine}
                onClick={() => {
                  onToggle(reaction.reaction, reaction.mine);
                }}
              >
                {content}
              </button>
            ) : (
              <span className={chipClass} title={names ?? undefined}>
                {content}
              </span>
            )}
          </li>
        );
      })}
    </ul>
  );
}
