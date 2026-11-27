import type { ReactElement } from 'react';

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
  return (
    <div role="group" aria-label={texts.filters} className="flex min-w-0 max-w-full flex-wrap gap-1">
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
