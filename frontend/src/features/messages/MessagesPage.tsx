import { useQuery, useQueryClient } from '@tanstack/react-query';
import { type ReactElement, useCallback, useEffect, useState } from 'react';
import { Link, useParams } from 'react-router';

import { useScheduler } from '../../app/clock.ts';
import { useMediaQuery } from '../../app/context.ts';
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

function ChannelConversation({ id, texts }: { readonly id: string; readonly texts: ChatTexts }): ReactElement {
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
  if (!seen && query.isSuccess && query.isFetchedAfterMount) setSeen(true);
  const notFound = query.isError && asApiError(query.error).status === 404;
  if (notFound && !gone) setGone(true);
  useEffect(() => {
    if (notFound) {
      evictChannel(cache, id);
      scheduleChatReads(cache, scheduler);
    }
  }, [notFound, cache, id, scheduler]);
  if (notFound || gone) return seen ? <FocusedNotice message={texts.unavailable} /> : <MissingChannel />;
  if (seen && query.data !== undefined)
    return (
      <>
        {query.isError ? (
          <LoadFailed
            error={asApiError(query.error)}
            onRetry={() => {
              void query.refetch();
            }}
          />
        ) : null}
        <Conversation channel={query.data} texts={texts} onUnavailable={unavailable} />
      </>
    );
  if (query.isError)
    return (
      <LoadFailed
        error={asApiError(query.error)}
        onRetry={() => {
          void query.refetch();
        }}
      />
    );
  return <DelayedLoading />;
}

function MissingChannel(): ReactElement {
  const { texts } = useChatTexts();
  return <FocusedNotice message={texts?.notFound ?? ''} />;
}

function MessagesContent({ texts }: { readonly texts: ChatTexts }): ReactElement {
  const { channelId } = useParams();
  const wide = useMediaQuery('(min-width: 768px)');
  const me = useSignedInAccount();
  return (
    <div className={wide ? 'grid grid-cols-2 gap-6' : 'flex flex-col gap-3'}>
      {wide || channelId === undefined ? <ChannelList texts={texts} /> : null}
      {channelId === undefined ? (
        wide ? (
          <p>{texts.choose}</p>
        ) : null
      ) : (
        <div className="flex min-w-0 flex-col gap-3">
          {!wide ? (
            <Link to="/messages" className="text-accent-text underline">
              {texts.back}
            </Link>
          ) : null}
          <ChannelConversation key={`${me.id}:${channelId}`} id={channelId} texts={texts} />
        </div>
      )}
    </div>
  );
}

export function MessagesPage(): ReactElement {
  const { texts, failed, retry } = useChatTexts();
  return (
    <Page wide>
      {failed ? <LoadFailed error={asApiError(undefined)} onRetry={retry} /> : null}
      {texts === null ? <DelayedLoading /> : <MessagesContent texts={texts} />}
    </Page>
  );
}
