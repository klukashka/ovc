import { useInfiniteQuery } from '@tanstack/react-query';
import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';

import { quietButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Page } from '../../components/ui/Page.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { fetchNotifications, notificationPagesKey } from './api.ts';
import { MarkAllRead, NotificationList } from './NotificationRows.tsx';

const rowSelector = ':scope > li > a, :scope > li[tabindex]';

function NotificationPages(): ReactElement {
  const { t } = useTranslation();
  const client = useApiClient();
  const query = useInfiniteQuery({
    queryKey: notificationPagesKey,
    queryFn: ({ pageParam }) => fetchNotifications(client, null, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    retry: false,
  });
  const items = query.data?.pages.flatMap((page) => page.items) ?? [];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(
    items.length,
    rowSelector,
    query.isFetchingNextPage,
  );

  if (query.isPending) {
    return <DelayedLoading />;
  }
  if (query.isError) {
    return (
      <LoadFailed
        error={asApiError(query.error)}
        onRetry={() => {
          void query.refetch();
        }}
      />
    );
  }
  const newest = items[0];
  if (newest === undefined) {
    return <p className="text-text-muted">{t('notifications.empty')}</p>;
  }
  return (
    <>
      <MarkAllRead newest={newest} />
      <NotificationList items={items} label={t('notifications.list')} listRef={listRef} onChoose={() => undefined} />
      {query.hasNextPage ? (
        <p>
          <button
            type="button"
            aria-disabled={query.isFetchingNextPage}
            aria-busy={query.isFetchingNextPage}
            onClick={() => {
              if (!query.isFetchingNextPage) {
                loadingMore();
                void query.fetchNextPage();
              }
            }}
            className={quietButtonClass}
          >
            {t('notifications.showMore')}
          </button>
        </p>
      ) : null}
    </>
  );
}

export function NotificationsPage(): ReactElement {
  return (
    <Page>
      <NotificationPages />
    </Page>
  );
}
