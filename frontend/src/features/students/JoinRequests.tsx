import {
  type InfiniteData,
  type UseInfiniteQueryResult,
  useInfiniteQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { type ReactElement, type ReactNode, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { type ActionState, done, refusedBy, showsStaleData, useAction } from '../../components/ui/action.ts';
import { followUpQueuedFor } from '../../app/queryClient.ts';
import { ActionMenu } from '../../components/ui/ActionMenu.tsx';
import { Avatar } from '../../components/ui/Avatar.tsx';
import { ModalDialog } from '../../components/ui/Dialog.tsx';
import {
  ActionButton,
  FormActions,
  FormAlert,
  FormNotice,
  dangerFilledButtonClass,
  quietButtonClass,
} from '../../components/ui/Form.tsx';
import { MoreIcon } from '../../components/ui/icons.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { loadNextPage, useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { useSettledOnce } from '../../components/ui/settledOnce.ts';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { type ApiClient, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatDate } from '../../lib/format/dates.ts';
import { type TutorHome, homeKey, tutorHomeKey } from '../home/api.ts';
import { joinRequestsKey, studentsKey } from './api.ts';

type TutorJoinRequest = components['schemas']['TutorJoinRequest'];
type TutorJoinRequestPage = components['schemas']['TutorJoinRequestPage'];

type Decision = 'accept' | 'decline' | 'block';

const acceptButtonClass =
  'inline-flex min-h-11 items-center justify-center rounded-md border border-border-strong px-3 text-small font-medium text-text hover:bg-surface aria-disabled:opacity-60 desktop:pointer-fine:min-h-8';

const decisionNotice = {
  accept: 'joinRequests.accepted',
  decline: 'joinRequests.declined',
  block: 'joinRequests.blocked',
} as const;

async function fetchJoinRequestsPage(
  client: ApiClient,
  cursor: string | null,
  signal: AbortSignal | null = null,
): Promise<TutorJoinRequestPage> {
  const { data, error, response } = await client.GET('/api/v1/join-requests', {
    params: { query: cursor === null ? {} : { cursor } },
    signal,
  });
  if (data === undefined) {
    throw apiErrorFrom(response.status, error);
  }
  return data;
}

async function sendDecision(client: ApiClient, id: string, decision: Decision): Promise<ActionState> {
  const path = { params: { path: { id } } };
  const { error, response } =
    decision === 'accept'
      ? await client.POST('/api/v1/join-requests/{id}/accept', path)
      : await client.POST('/api/v1/join-requests/{id}/decline', { ...path, body: { block: decision === 'block' } });
  return response.ok ? done : refusedBy(response, error);
}

function useReceivedJoinRequests(): UseInfiniteQueryResult<InfiniteData<TutorJoinRequestPage>> {
  const client = useApiClient();
  const me = useSignedInAccount();
  return useInfiniteQuery({
    queryKey: joinRequestsKey,
    queryFn: ({ pageParam }) => fetchJoinRequestsPage(client, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.next_cursor,
    enabled: me.state === 'active',
    retry: false,
  });
}

export function useJoinRequestsSettled(): boolean {
  const me = useSignedInAccount();
  const query = useReceivedJoinRequests();
  return useSettledOnce(me.state !== 'active' || !query.isPending);
}

function menuIdOf(base: string, request: TutorJoinRequest): string {
  return `${base}-${request.id}`;
}

function RequestRow({
  request,
  menuId,
  started,
  unavailable,
  onDecide,
  onBlock,
}: {
  readonly request: TutorJoinRequest;
  readonly menuId: string;
  readonly started: Decision | null;
  readonly unavailable: boolean;
  readonly onDecide: (decision: Decision) => void;
  readonly onBlock: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const name = request.student.name;
  return (
    <li data-row={request.id} className="flex flex-wrap items-center gap-3 border-t border-border py-3">
      <Avatar name={name} avatarId={request.student.avatar_id} />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-strong">{name}</span>
        <span className="truncate text-small text-text-muted">{request.student.email}</span>
        <span className="flex flex-wrap items-center gap-x-3">
          <StatusWord tone="accent">{t('joinRequests.wantsToJoin')}</StatusWord>
          <span className="tabular text-small text-text-muted">
            {t('joinRequests.sentOn', { date: formatDate(request.created_at, me.language, me.timezone) })}
          </span>
        </span>
      </span>
      <span className="flex w-full items-center gap-2 pl-11 tablet:w-auto tablet:pl-0">
        <ActionButton
          busy={started === 'accept'}
          unavailable={unavailable}
          label={t('joinRequests.acceptNamed', { name })}
          firstInRow
          onClick={() => {
            onDecide('accept');
          }}
          className={acceptButtonClass}
        >
          {t('joinRequests.accept')}
        </ActionButton>
        <ActionButton
          busy={started === 'decline'}
          unavailable={unavailable}
          label={t('joinRequests.declineNamed', { name })}
          onClick={() => {
            onDecide('decline');
          }}
          className={quietButtonClass}
        >
          {t('joinRequests.decline')}
        </ActionButton>
        <ActionMenu
          label={t('joinRequests.more', { name })}
          busy={started === 'block'}
          unavailable={unavailable}
          buttonId={menuId}
          items={[
            {
              label: t('joinRequests.declineAndBlock'),
              danger: true,
              onChoose: (): void => {
                if (!unavailable) {
                  onBlock();
                }
              },
            },
          ]}
        >
          <MoreIcon />
        </ActionMenu>
      </span>
    </li>
  );
}

interface RequestSource {
  readonly state: 'pending' | 'failed' | 'ready' | 'closed';
  readonly error: unknown;
  readonly retry: () => void;
  readonly requests: readonly TutorJoinRequest[];
  readonly fetching: boolean;
  readonly hasMore: boolean;
  readonly loadingMore: boolean;
  readonly moreFailed: unknown;
  readonly loadMore: () => void;
  readonly afterDecision: () => Promise<unknown>;
}

function JoinRequestList({
  source,
  hideWhenEmpty,
  children,
}: {
  readonly source: RequestSource;
  readonly hideWhenEmpty: boolean;
  readonly children?: ReactNode;
}): ReactElement | null {
  const { t } = useTranslation();
  const client = useApiClient();
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const [decided, setDecided] = useState<{ readonly decision: Decision; readonly count: number } | null>(null);
  const [deciding, setDeciding] = useState<{ readonly id: string; readonly decision: Decision } | null>(null);
  const [refusals, setRefusals] = useState(0);
  const [blocking, setBlocking] = useState<TutorJoinRequest | null>(null);
  const [goneWhileBlocking, setGoneWhileBlocking] = useState(0);
  const menuIds = useId();
  const blockHeadingId = useId();
  const blockConsequenceId = useId();
  const blockMenuButton = useRef<HTMLElement | null>(null);
  const keepRequest = useRef<HTMLButtonElement>(null);
  const action = useAction();
  const { requests } = source;
  if (blocking !== null && !action.busy && !requests.some((request) => request.id === blocking.id)) {
    setBlocking(null);
    setGoneWhileBlocking((count) => count + 1);
  }
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(
    requests.length,
    ':scope > li [data-first-in-row]',
    source.fetching,
  );
  useFocusAfterRemovedRow(
    listRef,
    requests.map((request) => request.id),
    '[data-first-in-row]',
    heading,
    action.busy,
  );

  function decide(id: string, decision: Decision): void {
    if (action.busy) {
      return;
    }
    setGoneWhileBlocking(0);
    setDecided(null);
    setDeciding({ id, decision });
    action.run(async (): Promise<ActionState> => {
      const settled = await sendDecision(client, id, decision);
      if (settled.kind === 'done') {
        setDecided((previous) => ({ decision, count: (previous?.count ?? 0) + 1 }));
      } else {
        setRefusals((count) => count + 1);
      }
      if (settled.kind === 'done' || showsStaleData(settled)) {
        await source.afterDecision();
      }
      return settled;
    });
  }

  function refusal(): ReactElement | null {
    const state = action.state;
    if (goneWhileBlocking > 0) {
      return <FormAlert key={`gone-${String(goneWhileBlocking)}`}>{t('joinRequests.changed')}</FormAlert>;
    }
    if (state.kind !== 'refused') {
      return null;
    }
    return showsStaleData(state) ? (
      <FormAlert key={refusals}>{t('joinRequests.changed')}</FormAlert>
    ) : (
      <Refusal state={state} confirmation="joinRequests" />
    );
  }

  function body(): ReactElement | null {
    if (source.state === 'closed') {
      return null;
    }
    if (source.state === 'pending') {
      return <DelayedLoading />;
    }
    if (source.state === 'failed') {
      return <LoadFailed error={asApiError(source.error)} onRetry={source.retry} />;
    }
    return (
      <>
        {refusal()}
        {decided === null ? null : (
          <FormNotice key={decided.count} takeFocus>
            {t(decisionNotice[decided.decision])}
          </FormNotice>
        )}
        {requests.length === 0 ? (
          <div className="flex flex-col gap-1 text-text-muted">
            <p>{t('joinRequests.empty')}</p>
            <p className="text-small">{t('joinRequests.emptyHint')}</p>
          </div>
        ) : (
          <ul ref={listRef} aria-labelledby={headingId} className="border-b border-border">
            {requests.map((request) => (
              <RequestRow
                key={request.id}
                request={request}
                menuId={menuIdOf(menuIds, request)}
                started={action.busy && deciding?.id === request.id ? deciding.decision : null}
                unavailable={action.busy}
                onDecide={(decision) => {
                  decide(request.id, decision);
                }}
                onBlock={() => {
                  blockMenuButton.current = document.getElementById(menuIdOf(menuIds, request));
                  setGoneWhileBlocking(0);
                  setBlocking(request);
                }}
              />
            ))}
          </ul>
        )}
        {source.moreFailed === null ? null : (
          <LoadFailed
            error={asApiError(source.moreFailed)}
            onRetry={() => {
              loadingMore();
              source.loadMore();
            }}
          />
        )}
        {source.hasMore && source.moreFailed === null ? (
          <p>
            <button
              type="button"
              aria-disabled={source.loadingMore}
              aria-busy={source.loadingMore}
              onClick={() => {
                if (!source.loadingMore) {
                  loadingMore();
                  source.loadMore();
                }
              }}
              className={quietButtonClass}
            >
              {t('joinRequests.showMore')}
            </button>
          </p>
        ) : null}
      </>
    );
  }

  const nothingToShow =
    source.state === 'closed' ||
    source.state === 'pending' ||
    (source.state === 'ready' &&
      requests.length === 0 &&
      decided === null &&
      goneWhileBlocking === 0 &&
      action.state.kind !== 'refused');
  if (hideWhenEmpty && nothingToShow) {
    return null;
  }
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {t('joinRequests.title')}
      </h2>
      <ModalDialog
        open={blocking !== null}
        busy={false}
        labelledBy={blockHeadingId}
        describedBy={blockConsequenceId}
        returnFocus={blockMenuButton}
        initialFocus={keepRequest}
        onClose={() => {
          setBlocking(null);
        }}
      >
        {blocking === null ? null : (
          <div className="flex flex-col gap-4">
            <h2 id={blockHeadingId} className="text-heading">
              {t('joinRequests.blockTitle', { name: blocking.student.name })}
            </h2>
            <p id={blockConsequenceId}>{t('joinRequests.blockConsequence')}</p>
            <FormActions>
              <ActionButton
                busy={false}
                onClick={() => {
                  setBlocking(null);
                  decide(blocking.id, 'block');
                }}
                className={dangerFilledButtonClass}
              >
                {t('joinRequests.declineAndBlock')}
              </ActionButton>
              <ActionButton
                ref={keepRequest}
                busy={false}
                onClick={() => {
                  setBlocking(null);
                }}
                className={quietButtonClass}
              >
                {t('joinRequests.cancel')}
              </ActionButton>
            </FormActions>
          </div>
        )}
      </ModalDialog>
      {body()}
      {children}
    </section>
  );
}

export function JoinRequests({
  hideWhenEmpty,
  children,
}: {
  readonly hideWhenEmpty: boolean;
  readonly children?: ReactNode;
}): ReactElement | null {
  const queryClient = useQueryClient();
  const me = useSignedInAccount();
  const query = useReceivedJoinRequests();
  const source: RequestSource = {
    state: me.state !== 'active' ? 'closed' : query.isPending ? 'pending' : query.isError ? 'failed' : 'ready',
    error: query.error,
    retry: (): void => {
      void query.refetch();
    },
    requests: query.data?.pages.flatMap((page) => page.items) ?? [],
    fetching: query.isFetching,
    hasMore: query.hasNextPage,
    loadingMore: query.isFetchingNextPage,
    moreFailed: null,
    loadMore: (): void => {
      void loadNextPage(query);
    },
    afterDecision: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: joinRequestsKey }),
        queryClient.invalidateQueries({ queryKey: studentsKey }),
        queryClient.invalidateQueries({ queryKey: homeKey }),
      ]),
  };
  return (
    <JoinRequestList source={source} hideWhenEmpty={hideWhenEmpty}>
      {children}
    </JoinRequestList>
  );
}

export type HomeJoinRequests = components['schemas']['TutorHomeJoinRequests'];

interface HomeAfterRefetch {
  readonly kind: 'unchanged' | 'refetched' | 'failed';
  readonly loads: number;
}

interface MorePages {
  readonly loadedAt: number;
  readonly pages: readonly TutorJoinRequestPage[];
}

function uniqueRequests(requests: readonly TutorJoinRequest[]): TutorJoinRequest[] {
  const seen = new Set<string>();
  return requests.filter((request) => {
    if (seen.has(request.id)) {
      return false;
    }
    seen.add(request.id);
    return true;
  });
}

export function HomeJoinRequestList({
  part,
  loadedAt,
  children,
}: {
  readonly part: HomeJoinRequests | null;
  readonly loadedAt: number;
  readonly children?: ReactNode;
}): ReactElement | null {
  const client = useApiClient();
  const queryClient = useQueryClient();
  const [more, setMore] = useState<MorePages>({ loadedAt, pages: [] });
  const [loading, setLoading] = useState(false);
  const [moreFailed, setMoreFailed] = useState<unknown>(null);
  const controller = useRef<AbortController | null>(null);
  useEffect(
    () => (): void => {
      controller.current?.abort();
    },
    [],
  );
  const pages = more.loadedAt === loadedAt ? more.pages : [];
  const cursor = pages.length === 0 ? (part?.next_cursor ?? null) : (pages.at(-1)?.next_cursor ?? null);
  const shownFailure = more.loadedAt === loadedAt ? moreFailed : null;

  async function homeAfterRefetch(): Promise<HomeAfterRefetch> {
    let refetched = false;
    for (;;) {
      const query = queryClient.getQueryCache().find<TutorHome>({ queryKey: tutorHomeKey, exact: true });
      const inFlight = query?.state.fetchStatus === 'fetching' ? query.promise : undefined;
      if (inFlight === undefined) {
        const followUp = followUpQueuedFor(queryClient, tutorHomeKey);
        if (followUp === null) {
          return { kind: refetched ? 'refetched' : 'unchanged', loads: query?.state.dataUpdateCount ?? 0 };
        }
        await followUp;
        continue;
      }
      const answered = await inFlight.then(
        () => true,
        () => false,
      );
      if (!answered) {
        return { kind: 'failed', loads: query?.state.dataUpdateCount ?? 0 };
      }
      refetched = true;
    }
  }

  async function loadMore(): Promise<void> {
    if (cursor === null || loading) {
      return;
    }
    const abort = new AbortController();
    controller.current = abort;
    setLoading(true);
    setMoreFailed(null);
    const home = await homeAfterRefetch();
    const startedAt = home.loads;
    const from =
      home.kind === 'refetched'
        ? (queryClient.getQueryData<TutorHome>(tutorHomeKey)?.join_requests?.next_cursor ?? null)
        : home.kind === 'unchanged'
          ? cursor
          : null;
    if (from === null) {
      setLoading(false);
      return;
    }
    try {
      const page = await fetchJoinRequestsPage(client, from, abort.signal);
      setLoading(false);
      if (currentHomeLoads() === startedAt) {
        setMore((previous) => ({
          loadedAt: startedAt,
          pages: [...(previous.loadedAt === startedAt ? previous.pages : []), page],
        }));
      }
    } catch (thrown: unknown) {
      setLoading(false);
      if (currentHomeLoads() === startedAt && !abort.signal.aborted) {
        setMore((previous) => (previous.loadedAt === startedAt ? previous : { loadedAt: startedAt, pages: [] }));
        setMoreFailed(thrown);
      }
    }
  }

  function currentHomeLoads(): number {
    return queryClient.getQueryState(tutorHomeKey)?.dataUpdateCount ?? 0;
  }

  const source: RequestSource = {
    state: part === null ? 'closed' : 'ready',
    error: null,
    retry: (): void => undefined,
    requests: part === null ? [] : uniqueRequests([...part.items, ...pages.flatMap((page) => page.items)]),
    fetching: loading || more.loadedAt > loadedAt,
    hasMore: cursor !== null,
    loadingMore: loading,
    moreFailed: shownFailure,
    loadMore: (): void => {
      void loadMore();
    },
    afterDecision: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: homeKey }),
        queryClient.invalidateQueries({ queryKey: joinRequestsKey }),
        queryClient.invalidateQueries({ queryKey: studentsKey }),
      ]),
  };
  return (
    <JoinRequestList source={source} hideWhenEmpty={false}>
      {children}
    </JoinRequestList>
  );
}
