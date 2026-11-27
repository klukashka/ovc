import { type ReactElement, useLayoutEffect, useRef } from 'react';

import type { ChannelFilter } from './filters.ts';
import type { ChatTexts } from './translations.ts';

export function ChannelFilters({
  filter,
  unread,
  student,
  texts,
  onChange,
}: {
  readonly filter: ChannelFilter;
  readonly unread: number;
  readonly student: boolean;
  readonly texts: ChatTexts;
  readonly onChange: (filter: ChannelFilter) => void;
}): ReactElement {
  const labels: Readonly<Record<ChannelFilter, string>> = {
    all: texts.filterAll,
    direct: student ? texts.filterTutors : texts.filterStudents,
    group: texts.filterGroups,
    unread: texts.filterUnread,
  };
  const count = unread > 99 ? '99+' : String(unread);
  const row = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const container = row.current;
    const pressed = container?.querySelector<HTMLElement>('[aria-pressed="true"]');
    if (!container || !pressed) return;
    function reveal(scroller: HTMLElement, target: HTMLElement): void {
      const visible = scroller.getBoundingClientRect();
      const box = target.getBoundingClientRect();
      if (box.right > visible.right) scroller.scrollLeft += Math.ceil(box.right - visible.right);
      if (box.left < visible.left) scroller.scrollLeft -= Math.ceil(visible.left - box.left);
    }
    reveal(container, pressed);
    const observer =
      typeof ResizeObserver === 'undefined'
        ? null
        : new ResizeObserver(() => {
            reveal(container, pressed);
          });
    observer?.observe(pressed);
    observer?.observe(container);
    return (): void => {
      observer?.disconnect();
    };
  }, [filter]);
  return (
    <div
      ref={row}
      role="group"
      aria-label={texts.filters}
      className="relative flex min-w-0 max-w-full flex-nowrap gap-1 overflow-x-auto [scrollbar-width:none] after:block after:w-px after:shrink-0 after:content-['']"
    >
      {(['all', 'direct', 'group', 'unread'] as const).map((value) => (
        <button
          key={value}
          type="button"
          aria-pressed={value === filter}
          aria-label={value === 'unread' && unread > 0 ? texts.filterUnreadCount.replace('{count}', count) : undefined}
          className="min-h-11 min-w-11 shrink-0 rounded-md px-2 text-small hover:bg-hover aria-pressed:bg-hover aria-pressed:font-semibold"
          onClick={(event) => {
            event.currentTarget.focus();
            onChange(value);
          }}
        >
          {labels[value]}
          {value === 'unread' && unread > 0 ? (
            <span aria-hidden="true" className="ml-1 rounded-full bg-accent px-1 text-on-accent">
              {count}
            </span>
          ) : null}
        </button>
      ))}
    </div>
  );
}
