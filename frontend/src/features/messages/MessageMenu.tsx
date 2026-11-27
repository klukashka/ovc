import { type ReactElement, useEffect, useId, useLayoutEffect, useRef } from 'react';

import { useScheduler } from '../../app/clock.ts';
import { ActionButton, quietButtonClass } from '../../components/ui/Form.tsx';
import { useChatWrite } from '../../components/ui/chatWrite.ts';
import { focusIsLost } from '../../components/ui/LoadState.tsx';
import { MoreIcon, WarningIcon } from '../../components/ui/icons.tsx';
import type { ReactionValue } from './api.ts';
import { reactionEmoji, reactionName, reactionValues } from './Reactions.tsx';
import type { ChatTexts } from './translations.ts';

const deleteItemClass =
  'inline-flex min-h-11 items-center justify-center gap-2 rounded-md border border-border-strong px-4 text-strong text-danger hover:bg-surface aria-disabled:opacity-60';

const moreButtonClass =
  'inline-flex size-11 items-center justify-center rounded-full text-icon hover:bg-surface hover:text-text desktop:pointer-fine:size-8 pointer-fine:opacity-0 pointer-fine:group-hover:opacity-100 pointer-fine:group-focus-within:opacity-100 pointer-fine:aria-expanded:opacity-100';

export type PinAction = 'pin' | 'unpin';

export function MessageMenu({
  texts,
  open,
  openRequest,
  onOpenChange,
  onCopy,
  editable,
  busy,
  onEdit,
  onDelete,
  onReply = null,
  pin = null,
  onPin = (): void => undefined,
  reaction = null,
  onReact = (): void => undefined,
}: {
  readonly texts: ChatTexts;
  readonly open: boolean;
  readonly openRequest: number;
  readonly onOpenChange: (open: boolean) => void;
  readonly onCopy: (() => void) | null;
  readonly editable: boolean;
  readonly busy: boolean;
  readonly onEdit: () => void;
  readonly onDelete: (() => void) | null;
  readonly onReply?: (() => void) | null;
  readonly pin?: PinAction | null;
  readonly onPin?: (action: PinAction) => void;
  readonly reaction?: { readonly current: ReactionValue | null } | null;
  readonly onReact?: (reaction: ReactionValue | null) => void;
}): ReactElement {
  const changeOpen = useRef(onOpenChange);
  useLayoutEffect(() => {
    changeOpen.current = onOpenChange;
  }, [onOpenChange]);
  const focusedItem = useRef<HTMLElement | null>(null);
  const menu = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const cancelPress = useRef<() => void>(() => undefined);
  const pressedLong = useRef(false);
  const scheduler = useScheduler();
  const id = useId();
  const guard = useChatWrite(busy);
  useLayoutEffect(() => {
    const opener = button.current;
    const row = opener?.closest<HTMLElement>('[data-row]');
    const container = opener?.parentElement;
    return (): void => {
      if (container?.contains(document.activeElement) === true) row?.focus();
      changeOpen.current(false);
    };
  }, []);
  useEffect(
    () => (): void => {
      cancelPress.current();
    },
    [],
  );
  useLayoutEffect(() => {
    const element = menu.current;
    if (!open || element === null) return;
    element.style.transform = '';
    const bounds = element.getBoundingClientRect();
    const pane = element.closest('[data-history-pane]')?.getBoundingClientRect();
    const left = Math.max(8, pane?.left ?? 8);
    const right = Math.min(window.innerWidth - 8, pane?.right ?? window.innerWidth - 8);
    const top = Math.max(8, pane?.top ?? 8);
    const bottom = Math.min(window.innerHeight - 8, pane?.bottom ?? window.innerHeight - 8);
    const x = Math.max(left - bounds.left, Math.min(0, right - bounds.right));
    const y = Math.max(top - bounds.top, Math.min(0, bottom - bounds.bottom));
    element.style.transform = `translate(${String(x)}px, ${String(y)}px)`;
    (
      element.querySelector<HTMLElement>('[data-menu-actions] button') ?? element.querySelector<HTMLElement>('button')
    )?.focus({ preventScroll: true });
  }, [open, openRequest]);
  useLayoutEffect(() => {
    if (open && focusedItem.current !== null && !focusedItem.current.isConnected && focusIsLost())
      (
        menu.current?.querySelector<HTMLElement>('[data-menu-actions] button') ??
        menu.current?.querySelector<HTMLElement>('button')
      )?.focus({ preventScroll: true });
  });
  useEffect(() => {
    if (!open) return undefined;
    function outside(event: PointerEvent): void {
      const bubble = button.current?.closest('[data-row]')?.querySelector('[data-bubble]');
      if (
        (event.button === 2 || (event.button === 0 && event.ctrlKey)) &&
        event.target instanceof Node &&
        bubble?.contains(event.target) === true
      )
        return;
      if (
        event.target instanceof Node &&
        !menu.current?.contains(event.target) &&
        !button.current?.contains(event.target)
      )
        changeOpen.current(false);
    }
    function escape(event: KeyboardEvent): void {
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        changeOpen.current(false);
        button.current?.focus();
      }
    }
    document.addEventListener('keydown', escape);
    document.addEventListener('pointerdown', outside);
    return (): void => {
      document.removeEventListener('pointerdown', outside);
      document.removeEventListener('keydown', escape);
    };
  }, [open]);
  return (
    <div className="relative">
      <button
        ref={button}
        type="button"
        aria-label={texts.menu}
        aria-expanded={open}
        aria-controls={id}
        className={moreButtonClass}
        onPointerDown={(event) => {
          pressedLong.current = false;
          cancelPress.current();
          if (event.pointerType === 'touch')
            cancelPress.current = scheduler.schedule(500, () => {
              pressedLong.current = true;
              onOpenChange(true);
            });
        }}
        onPointerUp={() => {
          cancelPress.current();
        }}
        onPointerCancel={() => {
          cancelPress.current();
          pressedLong.current = false;
        }}
        onPointerMove={() => {
          cancelPress.current();
        }}
        onClick={() => {
          if (pressedLong.current) {
            pressedLong.current = false;
            return;
          }
          onOpenChange(!open);
        }}
      >
        <MoreIcon size={18} />
      </button>
      {open ? (
        <div
          ref={menu}
          id={id}
          onFocusCapture={(event) => {
            focusedItem.current = event.target;
          }}
          onBlurCapture={(event) => {
            if (event.relatedTarget !== null && !event.currentTarget.contains(event.relatedTarget))
              focusedItem.current = null;
          }}
          className="absolute right-0 z-20 flex w-max max-w-[calc(100vw-2rem)] flex-col rounded-md border border-border bg-surface p-2 shadow-raised"
        >
          {reaction === null ? null : (
            <>
              <div role="group" aria-label={texts.reactions} className="flex flex-wrap gap-1 py-1">
                {reactionValues.map((value) => (
                  <button
                    key={value}
                    type="button"
                    aria-label={reactionName(texts, value)}
                    aria-pressed={reaction.current === value}
                    aria-disabled={guard.unavailable}
                    className="inline-flex size-11 items-center justify-center rounded-md text-heading hover:bg-hover aria-pressed:bg-mine"
                    onClick={() => {
                      guard.run(() => {
                        changeOpen.current(false);
                        button.current?.focus({ preventScroll: true });
                        onReact(value);
                      });
                    }}
                  >
                    <span aria-hidden="true">{reactionEmoji(value)}</span>
                  </button>
                ))}
              </div>
              {reaction.current === null ? null : (
                <button
                  type="button"
                  aria-disabled={guard.unavailable}
                  className={quietButtonClass}
                  onClick={() => {
                    guard.run(() => {
                      changeOpen.current(false);
                      button.current?.focus({ preventScroll: true });
                      onReact(null);
                    });
                  }}
                >
                  {texts.reactionRemove}
                </button>
              )}
            </>
          )}
          <div data-menu-actions="" className="flex flex-col">
            {onReply === null ? null : (
              <ActionButton
                busy={busy}
                unavailable={guard.unavailable}
                unavailableReason={guard.reason}
                className={quietButtonClass}
                onClick={() => {
                  guard.run(() => {
                    changeOpen.current(false);
                    onReply();
                  });
                }}
              >
                {texts.reply}
              </ActionButton>
            )}
            {onCopy === null ? null : (
              <button
                type="button"
                className={quietButtonClass}
                onClick={() => {
                  onCopy();
                  changeOpen.current(false);
                  button.current?.focus({ preventScroll: true });
                }}
              >
                {texts.copyText}
              </button>
            )}
            {editable ? (
              <ActionButton
                busy={busy}
                unavailable={guard.unavailable}
                unavailableReason={guard.reason}
                className={quietButtonClass}
                onClick={() => {
                  guard.run(() => {
                    changeOpen.current(false);
                    onEdit();
                  });
                }}
              >
                {texts.edit}
              </ActionButton>
            ) : null}
            {pin === null ? null : (
              <ActionButton
                busy={busy}
                unavailable={guard.unavailable}
                unavailableReason={guard.reason}
                className={quietButtonClass}
                onClick={() => {
                  guard.run(() => {
                    changeOpen.current(false);
                    button.current?.focus();
                    onPin(pin);
                  });
                }}
              >
                {pin === 'pin' ? texts.pin : texts.unpin}
              </ActionButton>
            )}
            {onDelete === null ? null : (
              <>
                {editable || pin !== null || reaction !== null || onReply !== null || onCopy !== null ? (
                  <hr className="my-1 border-border" />
                ) : null}
                <ActionButton
                  busy={busy}
                  unavailable={guard.unavailable}
                  unavailableReason={guard.reason}
                  className={deleteItemClass}
                  onClick={() => {
                    guard.run(() => {
                      changeOpen.current(false);
                      button.current?.focus();
                      onDelete();
                    });
                  }}
                >
                  <WarningIcon />
                  {texts.delete}
                </ActionButton>
              </>
            )}
          </div>
        </div>
      ) : null}
    </div>
  );
}
