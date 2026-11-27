import { useQueries, useQueryClient } from '@tanstack/react-query';
import {
  type ReactElement,
  useState,
  useRef,
  useLayoutEffect,
  useCallback,
  useSyncExternalStore,
  useEffect,
} from 'react';
import { Link, useLocation, useNavigate } from 'react-router';

import { useDebouncedValue, useScheduler, useLocalToday } from '../../app/clock.ts';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { quietButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed, focusIsLost } from '../../components/ui/LoadState.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSession, useSignedInAccount } from '../../lib/api/session.ts';
import { Avatar } from '../../components/ui/Avatar.tsx';
import { ChannelFilters } from './ChannelFilters.tsx';
import { type ChannelFilter, channelFilter, channelFilterSearch } from './filters.ts';
import { compactTime } from './compactTime.ts';
import { ReadTicksIcon } from './icons.tsx';
import { type ChannelItem, type ChannelPage, fetchChannels } from './api.ts';
import { channelScopeRevision, channelsScopeRevisionKey } from './cache.ts';
import { channelLabel } from './channelLabel.ts';
import { channelPageKey } from './keys.ts';
import { useFirstChannelPage } from './firstPage.ts';
import { ChannelPresence } from './PresenceSummary.tsx';
import type { ChatTexts } from './translations.ts';

export function ChannelList({ texts }: { readonly texts: ChatTexts }): ReactElement {
  const [search, setSearch] = useState('');
  const q = useDebouncedValue(search, 200);
  const location = useLocation();
  const navigate = useNavigate();
  const filter = channelFilter(location.search);
  const first = useFirstChannelPage(q, filter);
  const [knownUnread, setKnownUnread] = useState(0);
  const pageUnread = first.data?.unread_channels;
  if (pageUnread !== undefined && pageUnread !== knownUnread) setKnownUnread(pageUnread);
  const unreadCount = pageUnread ?? knownUnread;
  const me = useSignedInAccount();
  const container = useRef<HTMLDivElement>(null);
  const focusedInside = useRef(false);
  const previousFilter = useRef(filter);
  useEffect(() => {
    const record = (event: FocusEvent): void => {
      focusedInside.current = event.target instanceof Node && container.current?.contains(event.target) === true;
    };
    document.addEventListener('focusin', record);
    return (): void => {
      document.removeEventListener('focusin', record);
    };
  }, []);
  useLayoutEffect(() => {
    if (previousFilter.current === filter) return;
    previousFilter.current = filter;
    if (focusedInside.current && focusIsLost()) {
      container.current?.querySelector<HTMLButtonElement>('button[aria-pressed="true"]')?.focus();
    }
  }, [filter]);
  return (
    <div ref={container} className="flex min-w-0 flex-col gap-3">
      <ChannelFilters
        filter={filter}
        unread={unreadCount}
        student={me.role === 'student'}
        texts={texts}
        onChange={(next) => {
          if (next === filter) return;
          void navigate({ pathname: location.pathname, search: channelFilterSearch(next) });
        }}
      />
      <label className="flex flex-col gap-1">
        {texts.search}
        <input
          type="search"
          value={search}
          onChange={(event) => {
            setSearch(event.target.value);
          }}
          className="min-h-11 rounded-md border border-border-strong bg-surface px-3"
        />
      </label>
      <ChannelPages
        key={JSON.stringify([q, filter])}
        texts={texts}
        q={q}
        filter={filter}
        first={first}
        rawSearch={search}
      />
    </div>
  );
}

function ChannelPages({
  texts,
  q,
  rawSearch,
  filter,
  first,
}: {
  readonly texts: ChatTexts;
  readonly q: string;
  readonly rawSearch: string;
  readonly filter: ChannelFilter;
  readonly first: ReturnType<typeof useFirstChannelPage>;
}): ReactElement {
  const client = useApiClient();
  const me = useSignedInAccount();
  const session = useSession();
  const cache = useQueryClient();
  const scheduler = useScheduler();
  useLocalToday(me.timezone);
  const subscribeScope = useCallback(
    (changed: () => void): (() => void) =>
      cache.getQueryCache().subscribe((event) => {
        const key: unknown = event.query.queryKey;
        if (Array.isArray(key) && key.length === 1 && key[0] === channelsScopeRevisionKey[0]) changed();
      }),
    [cache],
  );
  const readScope = useCallback((): number => channelScopeRevision(cache), [cache]);
  const scope = useSyncExternalStore(subscribeScope, readScope);
  const currentBoundary = first.data?.next_cursor;
  const [boundary, setBoundary] = useState<string | null | undefined>(undefined);
  const [cursors, setCursors] = useState<readonly string[]>([]);
  const [cursorScope, setCursorScope] = useState(scope);
  const [pagination, setPagination] = useState<{
    readonly cursor: string;
    readonly rawSearch: string;
    readonly boundary: string | null | undefined;
    readonly scope: number;
    readonly epoch: number;
    readonly shown: ReadonlySet<string>;
  } | null>(null);
  const focusedPagination = useRef<ReadonlySet<string> | null>(null);
  const freshFirst = first.data?.fetchedScopeRevision === scope;
  const activeCursors = boundary === currentBoundary && cursorScope === scope && freshFirst ? cursors : [];
  if (boundary !== currentBoundary || cursorScope !== scope) {
    setCursorScope(scope);
    setBoundary(currentBoundary);
    setCursors([]);
  }
  const tails = useQueries({
    queries: activeCursors.map((cursor) => ({
      queryKey: channelPageKey(q, filter, scope, cursor),
      queryFn: ({ signal }: { readonly signal: AbortSignal }): Promise<ChannelPage> =>
        fetchChannels(client, q, cursor, signal, filter),
      retry: false,
      refetchOnMount: 'always' as const,
    })),
  });
  const queries = [first, ...tails];
  const last = queries.at(-1);
  const byId = new Map<string, ChannelItem>();
  for (const query of queries) {
    if (!query.isFetchedAfterMount) continue;
    for (const item of query.data?.items ?? []) {
      if (!byId.has(item.id)) byId.set(item.id, item);
    }
  }
  const channels = [...byId.values()];
  const list = useRef<HTMLUListElement>(null);
  useFocusAfterRemovedRow(
    list,
    channels.map((item) => item.id),
    'a',
  );
  const failure = queries.find((query) => query.isError);
  const intent = pagination;
  const continuation = intent === null ? undefined : tails[activeCursors.indexOf(intent.cursor)];
  const continuationBusy = continuation?.isFetching === true;
  const pendingContinuation = intent !== null && (continuation?.isPending === true || continuation?.isError === true);
  const next = pendingContinuation ? intent.cursor : last?.data?.next_cursor;
  const validIntent =
    intent !== null &&
    intent.boundary === currentBoundary &&
    intent.rawSearch === rawSearch &&
    intent.scope === scope &&
    intent.epoch === session.currentEpoch();
  if (intent !== null && (!validIntent || !freshFirst)) setPagination(null);
  useLayoutEffect(() => {
    const pending = pagination;
    if (pending === null || focusedPagination.current === pending.shown || !validIntent || !freshFirst) return;
    if (continuation?.isSuccess !== true || continuation.isFetching || !continuation.isFetchedAfterMount) return;
    focusedPagination.current = pending.shown;
    const rows = [...(list.current?.querySelectorAll<HTMLAnchorElement>(':scope > [data-row] > a') ?? [])];
    const added = rows.find((row) => {
      const key = row.parentElement?.getAttribute('data-row');
      return key !== undefined && key !== null && !pending.shown.has(key);
    });
    if (added !== undefined) added.focus();
    else if (next === null || next === undefined) {
      const target = rows.at(-1) ?? document.querySelector<HTMLElement>('main h1');
      target?.focus();
    }
  }, [pagination, continuation, currentBoundary, freshFirst, next, scope, validIntent]);
  return (
    <section aria-label={texts.list} className="flex min-w-0 flex-col gap-3">
      {first.isPending ? <DelayedLoading /> : null}
      {failure === undefined ? null : (
        <LoadFailed
          error={asApiError(failure.error)}
          onRetry={() => {
            void failure.refetch();
          }}
        />
      )}
      {first.isSuccess && channels.length === 0 ? (
        <p>
          {q !== '' || filter === 'all'
            ? texts.empty
            : filter === 'unread'
              ? texts.emptyUnread
              : filter === 'group'
                ? texts.emptyGroups
                : me.role === 'student'
                  ? texts.emptyTutors
                  : texts.emptyStudents}
        </p>
      ) : null}
      <ul ref={list} className="flex flex-col border-t border-border">
        {channels.map((channel) => (
          <li key={channel.id} data-row={channel.id}>
            <Link
              to={`/messages/${encodeURIComponent(channel.id)}${channelFilterSearch(filter)}`}
              className={`flex min-h-14 min-w-0 items-center gap-2 border-b border-border px-2 py-2 hover:bg-hover ${channel.writable ? '' : 'bg-hover text-text-muted'}`}
            >
              <Avatar
                name={channel.label_kind === null ? channelLabel(channel, texts) : ''}
                avatarId={null}
                size="medium"
                colour={channel.colour}
              />
              <span className="min-w-0 flex-1">
                <span className="flex min-w-0 items-center gap-1">
                  <span
                    dir="auto"
                    className={`min-w-0 flex-1 truncate ${channel.unread_count > 0 ? 'font-semibold' : ''}`}
                  >
                    {channelLabel(channel, texts)}
                  </span>
                  <ChannelPresence
                    channel={channel}
                    me={me}
                    revision={
                      queries.find((query) => query.data?.items.some((item) => item.id === channel.id))
                        ?.dataUpdatedAt ?? 0
                    }
                    onlineLabel={texts.online}
                  />
                  {channel.last_message === null ? null : (
                    <time dateTime={channel.last_message.created_at} className="shrink-0 text-caption text-text-muted">
                      {compactTime(channel.last_message.created_at, me.language, me.timezone, scheduler.wallClock())}
                    </time>
                  )}
                </span>
                <span className="flex min-w-0 items-center gap-1 text-small text-text-muted">
                  <span dir="auto" className="min-w-0 flex-1 truncate">
                    {channel.tutor === null ? null : (
                      <>
                        <bdi>{channel.tutor.name}</bdi> ·{' '}
                      </>
                    )}
                    {channel.unconfirmed ? (
                      <>
                        <span>{texts.unconfirmed}</span> ·{' '}
                      </>
                    ) : null}
                    {!channel.writable ? (
                      <>
                        <span>{texts.readonly}</span> ·{' '}
                      </>
                    ) : null}
                    {channel.last_message === null ? null : (
                      <span>
                        <bdi>
                          {channel.last_message.own
                            ? texts.you
                            : (channel.last_message.sender_label ?? texts.deletedUser)}
                        </bdi>
                        <bdi dir="ltr">
                          :{' '}
                          {channel.last_message.preview ??
                            (channel.last_message.attachment_count > 0 ? texts.attachment : texts.deleted)}
                        </bdi>
                      </span>
                    )}
                  </span>
                  {channel.unread_count > 0 ? (
                    <span className="shrink-0 rounded-full bg-accent px-1 text-on-accent">
                      {channel.unread_count >= 99 ? '99+' : channel.unread_count}
                    </span>
                  ) : channel.last_message?.peer_read != null ? (
                    <span
                      data-ticks={channel.last_message.peer_read ? 'read' : 'sent'}
                      className="flex shrink-0 items-center"
                    >
                      <ReadTicksIcon read={channel.last_message.peer_read} />
                      <span className="sr-only">
                        {channel.last_message.peer_read ? texts.readTick : texts.sentTick}
                      </span>
                    </span>
                  ) : null}
                </span>
              </span>
            </Link>
          </li>
        ))}
      </ul>
      {freshFirst &&
      (!first.isFetching || (validIntent && pendingContinuation)) &&
      next !== undefined &&
      next !== null ? (
        <button
          type="button"
          className={quietButtonClass}
          aria-disabled={pendingContinuation || last?.isFetching === true}
          aria-busy={continuationBusy}
          onClick={() => {
            if (pendingContinuation || last?.isFetching === true) return;
            setPagination({
              cursor: next,
              rawSearch,
              boundary: currentBoundary,
              scope,
              epoch: session.currentEpoch(),
              shown: new Set(channels.map((item) => item.id)),
            });
            setCursors((previous) => (previous.includes(next) ? previous : [...previous, next]));
          }}
        >
          {continuationBusy ? (
            <span
              aria-hidden="true"
              className="mr-2 size-4 animate-spin rounded-full border-2 border-current border-t-transparent"
            />
          ) : null}
          {texts.more}
        </button>
      ) : null}
    </section>
  );
}
