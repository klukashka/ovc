import { type UseInfiniteQueryResult, type InfiniteData, useInfiniteQuery } from '@tanstack/react-query';
import type { ReactElement } from 'react';
import { useTranslation } from 'react-i18next';

import { quietButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Page } from '../../components/ui/Page.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { type NotificationPage, fetchNotifications, notificationPagesKey } from './api.ts';
import { MarkAllRead, NotificationTable, notificationRowSelector } from './NotificationRows.tsx';

type PagesQuery = UseInfiniteQueryResult<InfiniteData<NotificationPage>>;

function NotificationPages({ query }: { readonly query: PagesQuery }): ReactElement {
  const { t } = useTranslation();
  const items = query.data?.pages.flatMap((page) => page.items) ?? [];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLTableSectionElement>(
    items.length,
    notificationRowSelector,
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
  if (items.length === 0) {
    return <p className="text-text-muted">{t('notifications.empty')}</p>;
  }
  return (
    <>
      <NotificationTable items={items} label={t('notifications.list')} bodyRef={listRef} />
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
  const client = useApiClient();
  const query = useInfiniteQuery({
    queryKey: notificationPagesKey,
    queryFn: ({ pageParam }) => fetchNotifications(client, null, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    retry: false,
  });
  const newest = query.data?.pages[0]?.items[0];
  return (
    <Page action={newest === undefined || query.isError ? null : <MarkAllRead newest={newest} />}>
      <NotificationPages query={query} />
    </Page>
  );
}
