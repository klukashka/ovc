import { type UseInfiniteQueryResult, type InfiniteData, useInfiniteQuery } from '@tanstack/react-query';
import { type ReactElement, useId, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { quietButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { type Tone, StatusWord } from '../../components/ui/StatusWord.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatDate } from '../../lib/format/dates.ts';
import {
  type SentJoinRequest,
  type SentJoinRequestPage,
  fetchSentRequestsPage,
  requestsToShow,
  sentRequestsKey,
} from './api.ts';

type ShownState = Exclude<SentJoinRequest['state'], 'accepted'>;

const tones: Readonly<Record<ShownState, Tone>> = {
  pending: 'info',
  not_accepted: 'muted',
  expired: 'muted',
};

export type SentRequestsQuery = UseInfiniteQueryResult<InfiniteData<SentJoinRequestPage>>;

export function useSentRequests(): SentRequestsQuery {
  const client = useApiClient();
  return useInfiniteQuery({
    queryKey: sentRequestsKey,
    queryFn: ({ pageParam }) => fetchSentRequestsPage(client, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    retry: false,
  });
}

export function hasPendingRequest(query: UseInfiniteQueryResult<InfiniteData<SentJoinRequestPage>>): boolean {
  return query.data?.pages.some((page) => page.items.some((request) => request.state === 'pending')) ?? false;
}

function RequestRow({
  request,
  state,
}: {
  readonly request: SentJoinRequest;
  readonly state: ShownState;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const message =
    state === 'pending' ? t('outcome.request_sent') : state === 'not_accepted' ? t('tutors.notAccepted') : null;
  return (
    <li data-row={request.id} tabIndex={-1} className="flex flex-col gap-1 border-t border-border py-3 outline-none">
      <StatusWord tone={tones[state]}>{t(`tutors.requestState.${state}`)}</StatusWord>
      {message === null ? null : <p>{message}</p>}
      <p className="tabular text-small text-text-muted">
        {t('tutors.sentOn', { date: formatDate(request.created_at, me.language, me.timezone) })}
      </p>
    </li>
  );
}

function shownState(request: SentJoinRequest): ShownState | null {
  return request.state === 'accepted' ? null : request.state;
}

export function SentRequests(): ReactElement | null {
  const { t } = useTranslation();
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const query = useSentRequests();
  const requests = requestsToShow(query.data?.pages.flatMap((page) => page.items) ?? []);
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(
    requests.length,
    ':scope > li',
    query.isFetchingNextPage,
  );
  useFocusAfterRemovedRow(
    listRef,
    requests.map((request) => request.id),
    null,
    heading,
  );

  function body(): ReactElement {
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
    return (
      <>
        {requests.length === 0 ? null : (
          <ul ref={listRef} aria-labelledby={headingId} className="border-b border-border">
            {requests.map((request) => {
              const state = shownState(request);
              return state === null ? null : <RequestRow key={request.id} request={request} state={state} />;
            })}
          </ul>
        )}
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
              {t('tutors.showMore')}
            </button>
          </p>
        ) : null}
      </>
    );
  }

  if (query.isSuccess && requests.length === 0 && !query.hasNextPage) {
    return null;
  }
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {t('tutors.requests')}
      </h2>
      {body()}
    </section>
  );
}
