import {
  type InfiniteData,
  type UseInfiniteQueryResult,
  useInfiniteQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { type ReactElement, type ReactNode, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { type ActionState, done, refusedBy, showsStaleData, useAction } from '../../components/ui/action.ts';
import { ActionMenu } from '../../components/ui/ActionMenu.tsx';
import { Avatar } from '../../components/ui/Avatar.tsx';
import {
  ActionButton,
  FormAlert,
  FormNotice,
  primaryButtonClass,
  quietButtonClass,
} from '../../components/ui/Form.tsx';
import { MoreIcon } from '../../components/ui/icons.tsx';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { useFocusOnFirstNewRow } from '../../components/ui/moreRows.ts';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { useSettledOnce } from '../../components/ui/settledOnce.ts';
import { StatusWord } from '../../components/ui/StatusWord.tsx';
import { type ApiClient, apiErrorFrom, asApiError, useApiClient } from '../../lib/api/client.ts';
import type { components } from '../../lib/api/schema.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatDate } from '../../lib/format/dates.ts';
import { joinRequestsKey, studentsKey } from './api.ts';

type TutorJoinRequest = components['schemas']['TutorJoinRequest'];
type TutorJoinRequestPage = components['schemas']['TutorJoinRequestPage'];

type Decision = 'accept' | 'decline' | 'block';

const decisionNotice = {
  accept: 'joinRequests.accepted',
  decline: 'joinRequests.declined',
  block: 'joinRequests.blocked',
} as const;

async function fetchJoinRequestsPage(client: ApiClient, cursor: string | null): Promise<TutorJoinRequestPage> {
  const { data, error, response } = await client.GET('/api/v1/join-requests', {
    params: { query: cursor === null ? {} : { cursor } },
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

function RequestRow({
  request,
  started,
  unavailable,
  onDecide,
}: {
  readonly request: TutorJoinRequest;
  readonly started: Decision | null;
  readonly unavailable: boolean;
  readonly onDecide: (decision: Decision) => void;
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
      <span className="flex items-center gap-2">
        <ActionButton
          busy={started === 'accept'}
          unavailable={unavailable}
          label={t('joinRequests.acceptNamed', { name })}
          firstInRow
          onClick={() => {
            onDecide('accept');
          }}
          className={primaryButtonClass}
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
          items={[
            {
              label: t('joinRequests.declineAndBlock'),
              onChoose: (): void => {
                if (!unavailable) {
                  onDecide('block');
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

export function JoinRequests({
  hideWhenEmpty,
  children,
}: {
  readonly hideWhenEmpty: boolean;
  readonly children?: ReactNode;
}): ReactElement | null {
  const { t } = useTranslation();
  const client = useApiClient();
  const queryClient = useQueryClient();
  const me = useSignedInAccount();
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement>(null);
  const [decided, setDecided] = useState<{ readonly decision: Decision; readonly count: number } | null>(null);
  const [deciding, setDeciding] = useState<{ readonly id: string; readonly decision: Decision } | null>(null);
  const [refusals, setRefusals] = useState(0);
  const action = useAction();
  const query = useReceivedJoinRequests();
  const requests = query.data?.pages.flatMap((page) => page.items) ?? [];
  const { listRef, loadingMore } = useFocusOnFirstNewRow<HTMLUListElement>(
    requests.length,
    ':scope > li [data-first-in-row]',
    query.isFetchingNextPage,
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
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: joinRequestsKey }),
          queryClient.invalidateQueries({ queryKey: studentsKey }),
        ]);
      }
      return settled;
    });
  }

  function refusal(): ReactElement | null {
    const state = action.state;
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
    if (me.state !== 'active') {
      return null;
    }
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
        {refusal()}
        {decided === null ? null : (
          <FormNotice key={decided.count} takeFocus>
            {t(decisionNotice[decided.decision])}
          </FormNotice>
        )}
        {requests.length === 0 ? (
          <p className="text-text-muted">{t('joinRequests.empty')}</p>
        ) : (
          <ul ref={listRef} aria-labelledby={headingId} className="border-b border-border">
            {requests.map((request) => (
              <RequestRow
                key={request.id}
                request={request}
                started={action.busy && deciding?.id === request.id ? deciding.decision : null}
                unavailable={action.busy}
                onDecide={(decision) => {
                  decide(request.id, decision);
                }}
              />
            ))}
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
              {t('joinRequests.showMore')}
            </button>
          </p>
        ) : null}
      </>
    );
  }

  const nothingToShow =
    me.state !== 'active' ||
    query.isPending ||
    (query.isSuccess && requests.length === 0 && decided === null && action.state.kind !== 'refused');
  if (hideWhenEmpty && nothingToShow) {
    return null;
  }
  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <h2 ref={heading} id={headingId} tabIndex={-1} className="text-heading outline-none">
        {t('joinRequests.title')}
      </h2>
      {body()}
      {children}
    </section>
  );
}
