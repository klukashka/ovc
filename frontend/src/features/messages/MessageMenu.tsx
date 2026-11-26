import { type ReactElement, useEffect, useId, useLayoutEffect, useRef, useState } from 'react';

import { useScheduler } from '../../app/clock.ts';
import { ActionButton, quietButtonClass } from '../../components/ui/Form.tsx';
import { useChatWrite } from '../../components/ui/chatWrite.ts';
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
  const [open, setOpen] = useState(false);
  const [picking, setPicking] = useState(false);
  const picker = useRef<HTMLDivElement>(null);
  const reactButton = useRef<HTMLButtonElement>(null);
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
    };
  }, []);
  useEffect(
    () => (): void => {
      cancelPress.current();
    },
    [],
  );
  useEffect(() => {
    if (!picking) return;
    const pressed = picker.current?.querySelector<HTMLElement>('button[aria-pressed="true"]');
    (pressed ?? picker.current?.querySelector<HTMLElement>('button'))?.focus();
  }, [picking]);
  useEffect(() => {
    if (!open) return undefined;
    menu.current?.querySelector<HTMLElement>('button')?.focus();
    function outside(event: PointerEvent): void {
      if (
        event.target instanceof Node &&
        !menu.current?.contains(event.target) &&
        !button.current?.contains(event.target)
      )
        setOpen(false);
    }
    function escape(event: KeyboardEvent): void {
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        if (picker.current?.contains(document.activeElement) === true) {
          setPicking(false);
          reactButton.current?.focus();
          return;
        }
        setOpen(false);
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
              setPicking(false);
              setOpen(true);
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
          setPicking(false);
          setOpen((value) => !value);
        }}
      >
        <MoreIcon size={18} />
      </button>
      {open ? (
        <div
          ref={menu}
          id={id}
          className="absolute right-0 z-20 flex min-w-40 flex-col rounded-md border border-border bg-surface p-2 shadow-raised"
        >
          {onReply === null ? null : (
            <ActionButton
              busy={busy}
              unavailable={guard.unavailable}
              unavailableReason={guard.reason}
              className={quietButtonClass}
              onClick={() => {
                guard.run(() => {
                  setOpen(false);
                  onReply();
                });
              }}
            >
              {texts.reply}
            </ActionButton>
          )}
          {reaction === null ? null : (
            <button
              ref={reactButton}
              type="button"
              aria-expanded={picking}
              className={quietButtonClass}
              onClick={() => {
                setPicking((value) => !value);
              }}
            >
              {texts.react}
            </button>
          )}
          {reaction !== null && picking ? (
            <div ref={picker} role="group" aria-label={texts.reactions} className="flex flex-wrap gap-1 py-1">
              {reactionValues.map((value) => (
                <button
                  key={value}
                  type="button"
                  aria-label={reactionName(texts, value)}
                  aria-pressed={reaction.current === value}
                  className="inline-flex size-11 items-center justify-center rounded-md text-heading hover:bg-hover aria-pressed:bg-mine"
                  onClick={() => {
                    guard.run(() => {
                      setOpen(false);
                      button.current?.focus();
                      onReact(value);
                    });
                  }}
                >
                  <span aria-hidden="true">{reactionEmoji(value)}</span>
                </button>
              ))}
              {reaction.current === null ? null : (
                <button
                  type="button"
                  className={quietButtonClass}
                  onClick={() => {
                    guard.run(() => {
                      setOpen(false);
                      button.current?.focus();
                      onReact(null);
                    });
                  }}
                >
                  {texts.reactionRemove}
                </button>
              )}
            </div>
          ) : null}
          {editable ? (
            <ActionButton
              busy={busy}
              unavailable={guard.unavailable}
              unavailableReason={guard.reason}
              className={quietButtonClass}
              onClick={() => {
                guard.run(() => {
                  setOpen(false);
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
                  setOpen(false);
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
              {editable || pin !== null || reaction !== null || onReply !== null ? (
                <hr className="my-1 border-border" />
              ) : null}
              <ActionButton
                busy={busy}
                unavailable={guard.unavailable}
                unavailableReason={guard.reason}
                className={deleteItemClass}
                onClick={() => {
                  guard.run(() => {
                    setOpen(false);
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
      ) : null}
    </div>
  );
}
