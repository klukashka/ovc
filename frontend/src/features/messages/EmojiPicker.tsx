import { type ReactElement, type RefObject, useEffect, useId, useLayoutEffect, useRef, useState } from 'react';

import { emojiChoices } from './emoji.ts';
import { EmojiIcon } from './icons.tsx';
import type { ChatTexts } from './translations.ts';

export function EmojiPicker({
  texts,
  writable,
  busy,
  composer,
  onChoose,
}: {
  readonly texts: ChatTexts;
  readonly writable: boolean;
  readonly busy: boolean;
  readonly composer: RefObject<HTMLTextAreaElement | null>;
  readonly onChoose: (emoji: string) => void;
}): ReactElement | null {
  const [open, setOpen] = useState(false);
  const [previousWritable, setPreviousWritable] = useState(writable);
  const shown = open && writable;
  const wasShown = useRef(false);
  const [active, setActive] = useState(0);
  const button = useRef<HTMLButtonElement>(null);
  const picker = useRef<HTMLDivElement>(null);
  const id = useId();
  if (previousWritable !== writable) {
    setPreviousWritable(writable);
    setOpen(false);
  }
  useLayoutEffect(() => {
    const element = picker.current;
    if (!shown || element === null) return;
    element.style.transform = '';
    const bounds = element.getBoundingClientRect();
    const offset = Math.max(1 - bounds.left, Math.min(0, window.innerWidth - 1 - bounds.right));
    element.style.transform = `translateX(${String(offset)}px)`;
    element.querySelector<HTMLElement>('[tabindex="0"]')?.focus({ preventScroll: true });
  }, [shown, active]);
  useLayoutEffect(() => {
    if (wasShown.current && !writable) composer.current?.focus();
    wasShown.current = shown;
  }, [shown, writable, composer]);
  useEffect(() => {
    if (!shown) return undefined;
    function outside(event: PointerEvent): void {
      if (
        event.target instanceof Node &&
        !picker.current?.contains(event.target) &&
        !button.current?.contains(event.target)
      )
        setOpen(false);
    }
    document.addEventListener('pointerdown', outside);
    return (): void => {
      document.removeEventListener('pointerdown', outside);
    };
  }, [shown]);
  if (!writable) return null;
  return (
    <div className="relative shrink-0">
      <button
        ref={button}
        type="button"
        aria-label={texts.emoji}
        aria-expanded={shown}
        aria-controls={id}
        aria-disabled={busy}
        className="inline-flex size-11 items-center justify-center rounded-md text-icon hover:bg-surface aria-disabled:opacity-60 desktop:pointer-fine:size-10"
        onMouseDown={(event) => {
          if (open) event.preventDefault();
        }}
        onClick={() => {
          if (busy) return;
          if (open) button.current?.focus();
          setActive(0);
          setOpen(!open);
        }}
      >
        <EmojiIcon size={20} />
      </button>
      {shown ? (
        <div
          ref={picker}
          id={id}
          role="group"
          aria-label={texts.emoji}
          className="absolute bottom-full left-0 z-20 grid w-[22.375rem] max-w-[calc(100vw-2px)] grid-cols-8 rounded-md border border-border bg-surface p-0.5 shadow-raised"
        >
          {emojiChoices.map((emoji, index) => (
            <button
              key={emoji.name}
              type="button"
              aria-label={texts[emoji.name]}
              aria-disabled={busy}
              tabIndex={index === active ? 0 : -1}
              className="inline-flex min-h-11 items-center justify-center rounded-md text-heading hover:bg-hover aria-disabled:opacity-60"
              onBlur={(event) => {
                if (event.relatedTarget === null || event.relatedTarget === button.current) return;
                if (picker.current?.contains(event.relatedTarget) !== true) setOpen(false);
              }}
              onKeyDown={(event) => {
                if (event.key === 'Escape' || (event.key === 'Tab' && event.shiftKey)) {
                  event.preventDefault();
                  event.stopPropagation();
                  setOpen(false);
                  button.current?.focus();
                  return;
                }
                const moves: Readonly<Record<string, number>> = {
                  ArrowRight: (active + 1) % emojiChoices.length,
                  ArrowLeft: (active + emojiChoices.length - 1) % emojiChoices.length,
                  ArrowDown: (active + 8) % emojiChoices.length,
                  ArrowUp: (active + emojiChoices.length - 8) % emojiChoices.length,
                  Home: 0,
                  End: emojiChoices.length - 1,
                };
                const next = moves[event.key];
                if (next !== undefined) {
                  event.preventDefault();
                  setActive(next);
                }
              }}
              onFocus={() => {
                setActive(index);
              }}
              onClick={() => {
                if (busy) return;
                onChoose(emoji.value);
                setOpen(false);
              }}
            >
              <span aria-hidden="true">{emoji.value}</span>
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
