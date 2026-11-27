import { type ReactElement, type ReactNode, useCallback, useEffect, useLayoutEffect, useRef } from 'react';

import { useScheduler } from '../../app/clock.ts';

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

function selectionIntersects(selection: Selection | null, node: Node): boolean {
  if (selection === null || selection.isCollapsed) return false;
  return Array.from({ length: selection.rangeCount }, (_, index) => selection.getRangeAt(index)).some((range) =>
    range.intersectsNode(node),
  );
}

export function MessageRow({
  message,
  place,
  texts,
  language,
  timeZone,
  groupChat,
  menu,
  onOpenMenu = null,
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
  readonly onOpenMenu?: (() => void) | null;
  readonly refresh: () => Promise<boolean>;
  readonly onJump: (quote: MessageQuote) => void;
  readonly peerReadSeq: number | null;
  readonly reactable?: boolean;
  readonly onToggleReaction?: (reaction: ReactionValue, mine: boolean) => void;
  readonly tracked?: boolean;
}): ReactElement {
  const scheduler = useScheduler();
  const openMenu = useRef(onOpenMenu);
  const rowPointerPending = useRef(false);
  useLayoutEffect(() => {
    openMenu.current = onOpenMenu;
  }, [onOpenMenu]);
  const bubble = useRef<HTMLDivElement>(null);
  const row = useRef<HTMLLIElement>(null);
  const cancelPress = useRef<() => void>(() => undefined);
  const pressedLong = useRef(false);
  const press = useRef<{ readonly id: number; readonly x: number; readonly y: number } | null>(null);
  const pointer = useRef<{
    readonly type: string;
    readonly button: number;
    readonly ctrl: boolean;
    readonly shift: boolean;
    readonly selected: boolean;
    readonly anchor: Node | null;
    readonly focus: Node | null;
    readonly anchorOffset: number;
    readonly focusOffset: number;
  } | null>(null);
  const excluded = useCallback((target: EventTarget): boolean => {
    const interactive =
      target instanceof Element
        ? target.closest(
            'a, button, input, textarea, select, summary, img, [role="button"], [tabindex], [data-reaction]',
          )
        : null;
    return interactive !== null && bubble.current?.contains(interactive) === true;
  }, []);
  const cancel = useCallback((): void => {
    cancelPress.current();
    press.current = null;
  }, []);
  useEffect(() => {
    const pane = bubble.current?.closest('[data-history-pane]');
    pane?.addEventListener('scroll', cancel);
    return (): void => {
      cancelPress.current();
      pane?.removeEventListener('scroll', cancel);
    };
  }, [cancel]);
  useEffect(() => {
    if (onOpenMenu === null) cancelPress.current();
  }, [onOpenMenu]);
  useEffect(() => {
    const rowElement = row.current;
    const bubbleElement = bubble.current;
    if (rowElement === null || bubbleElement === null) return undefined;
    const bubbleNode: HTMLElement = bubbleElement;
    function pointerDown(): void {
      rowPointerPending.current = true;
    }
    function clearPointerPending(): void {
      rowPointerPending.current = false;
    }
    function keydown(event: KeyboardEvent): void {
      rowPointerPending.current = false;
      pressedLong.current = false;
      cancel();
      if (
        event.target === rowElement &&
        openMenu.current !== null &&
        (event.key === 'ContextMenu' || (event.key === 'F10' && event.shiftKey))
      ) {
        event.preventDefault();
        openMenu.current();
      }
    }
    function contextMenu(event: MouseEvent): void {
      const pointerPending = rowPointerPending.current;
      rowPointerPending.current = false;
      const recorded = pointer.current;
      pointer.current = null;
      if (event.target === rowElement) {
        if (pointerPending || ('pointerType' in event && event.pointerType !== '') || openMenu.current === null) return;
        event.preventDefault();
        openMenu.current();
        return;
      }
      if (!(event.target instanceof Node) || !bubbleNode.contains(event.target)) return;
      if (
        recorded?.type !== 'mouse' ||
        recorded.shift ||
        recorded.selected ||
        !(recorded.button === 2 || (recorded.button === 0 && recorded.ctrl)) ||
        ('pointerType' in event && event.pointerType !== 'mouse') ||
        excluded(event.target) ||
        openMenu.current === null
      )
        return;
      event.preventDefault();
      const selection = window.getSelection();
      if (
        selection?.isCollapsed === false &&
        bubbleNode.contains(selection.anchorNode) &&
        bubbleNode.contains(selection.focusNode) &&
        (selection.anchorNode !== recorded.anchor ||
          selection.focusNode !== recorded.focus ||
          selection.anchorOffset !== recorded.anchorOffset ||
          selection.focusOffset !== recorded.focusOffset)
      )
        selection.removeAllRanges();
      openMenu.current();
    }
    function click(event: MouseEvent): void {
      if (pressedLong.current && event.detail > 0) {
        pressedLong.current = false;
        event.preventDefault();
        event.stopPropagation();
      }
    }
    rowElement.addEventListener('pointerdown', pointerDown, true);
    rowElement.addEventListener('keydown', keydown);
    rowElement.addEventListener('click', clearPointerPending);
    rowElement.addEventListener('contextmenu', contextMenu);
    bubbleElement.addEventListener('click', click, true);
    return (): void => {
      rowElement.removeEventListener('pointerdown', pointerDown, true);
      rowElement.removeEventListener('keydown', keydown);
      rowElement.removeEventListener('click', clearPointerPending);
      rowElement.removeEventListener('contextmenu', contextMenu);
      bubbleElement.removeEventListener('click', click, true);
    };
  }, [cancel, excluded]);
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
      ref={row}
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
          ref={bubble}
          data-bubble=""
          onPointerDown={(event) => {
            pressedLong.current = false;
            cancel();
            const selection = window.getSelection();
            pointer.current = {
              type: event.pointerType,
              button: event.button,
              ctrl: event.ctrlKey,
              shift: event.shiftKey,
              selected: selectionIntersects(selection, event.currentTarget),
              anchor: selection?.anchorNode ?? null,
              focus: selection?.focusNode ?? null,
              anchorOffset: selection?.anchorOffset ?? 0,
              focusOffset: selection?.focusOffset ?? 0,
            };
            if (event.pointerType !== 'touch' || excluded(event.target) || onOpenMenu === null) return;
            press.current = { id: event.pointerId, x: event.clientX, y: event.clientY };
            cancelPress.current = scheduler.schedule(500, () => {
              pressedLong.current = true;
              onOpenMenu();
            });
          }}
          onPointerMove={(event) => {
            const start = press.current;
            if (
              start !== null &&
              start.id === event.pointerId &&
              Math.hypot(event.clientX - start.x, event.clientY - start.y) > 10
            )
              cancel();
          }}
          onPointerUp={cancel}
          onPointerCancel={() => {
            cancel();
            pressedLong.current = false;
          }}
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
