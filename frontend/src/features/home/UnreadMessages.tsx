import { type ReactElement, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { linkClass } from '../../components/ui/Form.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { paletteDotClass } from '../../components/ui/palette.ts';
import { asApiError } from '../../lib/api/client.ts';
import { useFirstChannelPage } from '../messages/firstPage.ts';

const shownChannels = 5;

function unreadCount(count: number): string {
  return count > 99 ? '99+' : String(count);
}

export function UnreadMessages(): ReactElement {
  const { t } = useTranslation();
  const headingId = useId();
  const query = useFirstChannelPage('');
  const listRef = useRef<HTMLUListElement>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const unread = (query.data?.items ?? []).filter((channel) => channel.unread_count > 0).slice(0, shownChannels);
  useFocusAfterRemovedRow(listRef, query.isError ? [] : unread.map((channel) => channel.id), ':scope > a', headingRef);

  function body(): ReactElement {
    if (query.isPending) {
      return <DelayedLoading />;
    }
    const allMessages =
      query.data === undefined ? null : (
        <Link to="/messages" className={linkClass}>
          {t('today.allMessages', { count: query.data.unread_channels })}
        </Link>
      );
    if (query.isError) {
      return (
        <>
          <LoadFailed
            error={asApiError(query.error)}
            onRetry={() => {
              void query.refetch();
            }}
          />
          {allMessages}
        </>
      );
    }
    return (
      <>
        {unread.length === 0 ? (
          <p className="text-text-muted">{t('today.noUnread')}</p>
        ) : (
          <ul ref={listRef} aria-labelledby={headingId} className="flex flex-col border-y border-border">
            {unread.map((channel) => (
              <li key={channel.id} data-row={channel.id} className="border-t border-border first:border-t-0">
                <Link
                  to={`/messages/${encodeURIComponent(channel.id)}`}
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
                      {channel.label ?? t('today.deletedUser')}
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
