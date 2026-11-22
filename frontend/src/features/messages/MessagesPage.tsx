import { useQuery, useQueryClient } from '@tanstack/react-query';
import { type ReactElement, type ReactNode, useCallback, useEffect, useRef, useState } from 'react';
import { Link, useParams } from 'react-router';

import { useScheduler } from '../../app/clock.ts';
import { phoneLayoutQuery, useMediaQuery } from '../../app/context.ts';
import { ChevronLeftIcon } from '../../components/ui/icons.tsx';
import { FocusedNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { Page } from '../../components/ui/Page.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { fetchChannel } from './api.ts';
import { ChannelList } from './ChannelList.tsx';
import { Conversation } from './Conversation.tsx';
import { scheduleChatReads } from './refetch.ts';
import { channelKey } from './keys.ts';
import { evictChannel } from './cache.ts';
import { type ChatTexts, useChatTexts } from './translations.ts';

function ChannelConversation({
  id,
  texts,
  back,
}: {
  readonly id: string;
  readonly texts: ChatTexts;
  readonly back: ReactNode;
}): ReactElement {
  const client = useApiClient();
  const cache = useQueryClient();
  const scheduler = useScheduler();
  const [gone, setGone] = useState(false);
  const [seen, setSeen] = useState(false);
  const query = useQuery({
    queryKey: channelKey(id),
    queryFn: ({ signal }) => fetchChannel(client, id, signal),
    retry: false,
    enabled: !gone,
    refetchOnMount: 'always',
  });
  const unavailable = useCallback((): void => {
    setGone(true);
    evictChannel(cache, id);
    scheduleChatReads(cache, scheduler);
  }, [cache, id, scheduler]);
  useFocusHeadingWhenLayoutDropsFocus(back !== null);
  if (!seen && query.isSuccess && query.isFetchedAfterMount) setSeen(true);
  const notFound = query.isError && asApiError(query.error).status === 404;
  if (notFound && !gone) setGone(true);
  useEffect(() => {
    if (notFound) {
      evictChannel(cache, id);
      scheduleChatReads(cache, scheduler);
    }
  }, [notFound, cache, id, scheduler]);
  if (notFound || gone)
    return (
      <>
        {back}
        {seen ? <FocusedNotice message={texts.unavailable} /> : <MissingChannel />}
      </>
    );
  if (seen && query.data !== undefined)
    return (
      <Conversation
        revision={query.dataUpdatedAt}
        channel={query.data}
        texts={texts}
        back={back}
        onUnavailable={unavailable}
        channelError={query.isError ? asApiError(query.error) : null}
        refreshChannel={() => query.refetch()}
      />
    );
  if (query.isError)
    return (
      <>
        {back}
        <LoadFailed
          error={asApiError(query.error)}
          onRetry={() => {
            void query.refetch();
          }}
        />
      </>
    );
  return (
    <>
      {back}
      <DelayedLoading />
    </>
  );
}

function useFocusHeadingWhenLayoutDropsFocus(phone: boolean): void {
  const shownAsPhone = useRef(phone);
  useEffect(() => {
    if (shownAsPhone.current === phone) return;
    shownAsPhone.current = phone;
    const active = document.activeElement;
    if (active === null || active === document.body || !active.isConnected) {
      document.querySelector<HTMLElement>('main h1')?.focus();
    }
  }, [phone]);
}

function BackToList({ label }: { readonly label: string }): ReactElement {
  return (
    <Link
      to="/messages"
      aria-label={label}
      className="inline-flex size-11 shrink-0 items-center justify-center rounded-sm text-icon hover:bg-hover hover:text-text"
    >
      <ChevronLeftIcon />
    </Link>
  );
}

function MissingChannel(): ReactElement {
  const { texts } = useChatTexts();
  return <FocusedNotice message={texts?.notFound ?? ''} />;
}

function MessagesContent({ texts }: { readonly texts: ChatTexts }): ReactElement {
  const { channelId } = useParams();
  const wide = !useMediaQuery(phoneLayoutQuery);
  const me = useSignedInAccount();
  return (
    <div
      className={
        wide
          ? 'grid grid-cols-2 gap-6 desktop:h-[calc(100dvh-(--spacing(24)))] desktop:grid-cols-[var(--container-aside)_minmax(0,1fr)] desktop:gap-0'
          : channelId === undefined
            ? 'flex flex-col gap-3'
            : 'flex h-[calc(100dvh-(--spacing(4)))] flex-col'
      }
    >
      {wide || channelId === undefined ? (
        <div className="min-w-0 desktop:min-h-0 desktop:overflow-y-auto desktop:border-r desktop:border-border desktop:pr-4">
          <ChannelList texts={texts} />
        </div>
      ) : null}
      {channelId === undefined ? (
        wide ? (
          <p>{texts.choose}</p>
        ) : null
      ) : (
        <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-3 desktop:pl-6">
          <ChannelConversation
            key={`${me.id}:${channelId}`}
            id={channelId}
            texts={texts}
            back={wide ? null : <BackToList label={texts.back} />}
          />
        </div>
      )}
    </div>
  );
}

export function MessagesPage(): ReactElement {
  const { texts, failed, retry } = useChatTexts();
  const { channelId } = useParams();
  const phone = useMediaQuery(phoneLayoutQuery);
  return (
    <div className={phone && channelId !== undefined && texts !== null ? '[&>article>header]:sr-only' : undefined}>
      <Page titleHiddenOnDesktop>
        {failed ? <LoadFailed error={asApiError(undefined)} onRetry={retry} /> : null}
        {texts === null ? <DelayedLoading /> : <MessagesContent texts={texts} />}
      </Page>
    </div>
  );
}
