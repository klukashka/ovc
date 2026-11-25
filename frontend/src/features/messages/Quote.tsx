import type { ReactElement } from 'react';

import type { MessageQuote, PinnedMessage } from './api.ts';
import type { ChatTexts } from './translations.ts';

type Quoted = Pick<MessageQuote, 'sender_label' | 'sender_label_kind' | 'own' | 'sent_before_reclaim'> & {
  readonly preview?: string | null;
  readonly state?: MessageQuote['state'];
};

export function quotedSender(quote: Quoted, texts: ChatTexts): string {
  if (quote.own) return texts.you;
  if (quote.sender_label_kind !== null || quote.sender_label === null) return texts.deletedUser;
  return quote.sender_label;
}

export function quotedText(quote: Quoted, texts: ChatTexts): string {
  if (quote.state === 'deleted') return texts.deleted;
  if (quote.state === 'removed') return texts.erased;
  return quote.preview ?? texts.attachmentOnly;
}

export function QuoteLines({ quote, texts }: { readonly quote: Quoted; readonly texts: ChatTexts }): ReactElement {
  return (
    <>
      <span dir="auto" className="block truncate text-label text-accent-text">
        {quotedSender(quote, texts)}
      </span>
      <span dir="auto" className="block truncate text-small text-text-muted">
        {quotedText(quote, texts)}
      </span>
      {quote.sent_before_reclaim ? <span className="block text-caption text-text-muted">{texts.reclaimed}</span> : null}
    </>
  );
}

export function Quote({
  quote,
  texts,
  onJump,
}: {
  readonly quote: MessageQuote;
  readonly texts: ChatTexts;
  readonly onJump: (quote: MessageQuote) => void;
}): ReactElement {
  return (
    <button
      type="button"
      className="block w-full min-w-0 border-l-2 border-accent py-0.5 pl-2 text-left hover:bg-hover"
      onClick={() => {
        onJump(quote);
      }}
    >
      <QuoteLines quote={quote} texts={texts} />
    </button>
  );
}

export function PinnedBar({
  pinned,
  texts,
  tutor,
  busy,
  onJump,
  onUnpin,
}: {
  readonly pinned: PinnedMessage;
  readonly texts: ChatTexts;
  readonly tutor: boolean;
  readonly busy: boolean;
  readonly onJump: (pinned: PinnedMessage) => void;
  readonly onUnpin: () => void;
}): ReactElement {
  return (
    <section
      aria-label={texts.pinned}
      className="flex shrink-0 items-center gap-2 border-y border-border bg-surface px-3 py-1.5"
    >
      <button
        type="button"
        className="min-w-0 flex-1 border-l-2 border-accent pl-2 text-left"
        onClick={() => {
          onJump(pinned);
        }}
      >
        <span className="block text-caption text-accent-text">{texts.pinned}</span>
        <QuoteLines quote={pinned} texts={texts} />
      </button>
      {tutor ? (
        <button
          type="button"
          className="inline-flex min-h-11 items-center px-3 text-small text-accent-text underline aria-disabled:opacity-60"
          aria-disabled={busy}
          onClick={() => {
            if (!busy) onUnpin();
          }}
        >
          {texts.unpin}
        </button>
      ) : null}
    </section>
  );
}
