import { type ReactElement, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { linkClass } from '../../components/ui/Form.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { paletteDotClass } from '../../components/ui/palette.ts';
import type { TutorHome } from './api.ts';

function unreadCount(count: number): string {
  return count > 99 ? '99+' : String(count);
}

export function UnreadMessages({ part }: { readonly part: TutorHome['unread_channels'] }): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const listRef = useRef<HTMLUListElement>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const unread = part.items;
  useFocusAfterRemovedRow(
    listRef,
    unread.map((channel) => channel.channel_id),
    ':scope > a',
    headingRef,
  );

  function body(): ReactElement {
    const allMessages = (
      <Link to="/messages" className={linkClass}>
        {t('today.allMessages', { count: part.unread_total })}
      </Link>
    );
    return (
      <>
        {unread.length === 0 ? (
          <p className="text-text-muted">{t('today.noUnread')}</p>
        ) : (
          <ul ref={listRef} aria-labelledby={headingId} className="flex flex-col border-y border-border">
            {unread.map((channel) => (
              <li
                key={channel.channel_id}
                data-row={channel.channel_id}
                className="border-t border-border first:border-t-0"
              >
                <Link
                  to={`/messages/${encodeURIComponent(channel.channel_id)}`}
                  className="flex min-h-11 items-center justify-between gap-3 px-1 py-2 hover:bg-surface"
                >
                  <span className="flex min-w-0 items-center gap-2">
                    {channel.colour === null ? null : (
                      <span
                        aria-hidden="true"
                        className={`size-2 shrink-0 rounded-full ${paletteDotClass[channel.colour]}`}
                      />
                    )}
                    <span dir="auto" className="truncate">
                      {channel.label_kind === 'deleted_user' || channel.label === null
                        ? t('today.deletedUser')
                        : channel.label}
                    </span>
                  </span>
                  <span className="shrink-0 rounded-full bg-hover px-2 text-caption tabular">
                    <span aria-hidden="true">{unreadCount(channel.unread_count)}</span>
                    <span className="sr-only">{t('today.unreadCount', { count: channel.unread_count })}</span>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
        {allMessages}
      </>
    );
  }

  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 ref={headingRef} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {t('today.unread')}
      </h2>
      {body()}
    </section>
  );
}
