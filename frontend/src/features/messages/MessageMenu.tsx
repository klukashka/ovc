import { type ReactElement, useEffect, useId, useLayoutEffect, useRef, useState } from 'react';

import { useScheduler } from '../../app/clock.ts';
import { ActionButton, quietButtonClass } from '../../components/ui/Form.tsx';
import { useChatWrite } from '../../components/ui/chatWrite.ts';
import { MoreIcon } from '../../components/ui/icons.tsx';
import type { ChatTexts } from './translations.ts';

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
}: {
  readonly texts: ChatTexts;
  readonly editable: boolean;
  readonly busy: boolean;
  readonly onEdit: () => void;
  readonly onDelete: (() => void) | null;
  readonly onReply?: (() => void) | null;
  readonly pin?: PinAction | null;
  readonly onPin?: (action: PinAction) => void;
}): ReactElement {
  const [open, setOpen] = useState(false);
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
            <ActionButton
              busy={busy}
              unavailable={guard.unavailable}
              unavailableReason={guard.reason}
              className={quietButtonClass}
              onClick={() => {
                guard.run(() => {
                  setOpen(false);
                  button.current?.focus();
                  onDelete();
                });
              }}
            >
              {texts.delete}
            </ActionButton>
          )}
        </div>
      ) : null}
    </div>
  );
}
