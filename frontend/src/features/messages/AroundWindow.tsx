import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query';
import { type ReactElement, useEffect, useMemo, useRef } from 'react';

import { quietButtonClass } from '../../components/ui/Form.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import { type Message, type MessagePage, fetchMessagesAround } from './api.ts';
import { channelScopeRevision } from './cache.ts';
import { aroundKey } from './keys.ts';
import { type MessagePlace, messagePlaces } from './messageGroups.ts';
import { revealRow } from './reveal.ts';
import type { ChatTexts } from './translations.ts';

export interface WindowCenter {
  readonly seq: number;
  readonly target: string;
}

const windowLimit = 50;

class StaleWindow extends Error {
  constructor() {
    super('the channel scope changed while the window was read');
  }
}

function landing(list: HTMLOListElement, center: WindowCenter): HTMLElement | null {
  const exact = Array.from(list.querySelectorAll<HTMLElement>('[data-row]')).find(
    (row) => row.dataset.row === center.target,
  );
  if (exact !== undefined) return exact;
  let nearest: HTMLElement | null = null;
  for (const row of list.querySelectorAll<HTMLElement>('[data-window-seq]')) {
    if (Number(row.dataset.windowSeq) <= center.seq) nearest = row;
  }
  return nearest;
}

export function AroundWindow({
  channelId,
  center,
  texts,
  language,
  timeZone,
  renderRow,
  onRecenter,
  onItems,
  onUnavailable,
  onFocusLost,
}: {
  readonly channelId: string;
  readonly center: WindowCenter;
  readonly texts: ChatTexts;
  readonly language: Language;
  readonly timeZone: string;
  readonly renderRow: (message: Message, place: MessagePlace) => ReactElement;
  readonly onRecenter: (center: WindowCenter) => void;
  readonly onItems: (items: readonly Message[]) => void;
  readonly onUnavailable: () => void;
  readonly onFocusLost: () => void;
}): ReactElement {
  const client = useApiClient();
  const cache = useQueryClient();
  const list = useRef<HTMLOListElement>(null);
  const landed = useRef<string | null>(null);
  const query = useQuery({
    queryKey: aroundKey(channelId, center.seq),
    queryFn: async ({ signal }): Promise<MessagePage> => {
      const revision = channelScopeRevision(cache);
      const page = await fetchMessagesAround(client, channelId, center.seq, windowLimit, signal);
      if (channelScopeRevision(cache) !== revision) throw new StaleWindow();
      return page;
    },
    retry: (failures, error) => error instanceof StaleWindow && failures < 3,
    placeholderData: keepPreviousData,
  });
  const items = useMemo(() => query.data?.items ?? [], [query.data]);
  const places = useMemo(() => messagePlaces(items, language, timeZone), [items, language, timeZone]);
  useEffect(() => {
    onItems(items);
  }, [items, onItems]);
  const unavailable = query.isError && asApiError(query.error).status === 404;
  useEffect(() => {
    if (unavailable) onUnavailable();
  }, [unavailable, onUnavailable]);
  const failed = query.isError && !unavailable;
  useEffect(() => {
    const active = document.activeElement;
    if (failed && (active === null || active === document.body || !active.isConnected)) onFocusLost();
  }, [failed, onFocusLost]);
  const key = `${String(center.seq)}:${center.target}`;
  useEffect(() => {
    const element = list.current;
    if (element === null || items.length === 0 || query.isPlaceholderData || landed.current === key) return;
    landed.current = key;
    const row = landing(element, center);
    if (row !== null) revealRow(row);
  }, [items, key, center, query.isPlaceholderData]);
  const first = items[0];
  const last = items.at(-1);
  return (
    <div className="flex min-w-0 flex-col gap-3">
      {query.isPending ? <DelayedLoading /> : null}
      {query.isError && !unavailable ? (
        <LoadFailed
          error={asApiError(query.error)}
          onRetry={() => {
            void query.refetch();
          }}
        />
      ) : null}
      {query.data?.next_cursor != null && first !== undefined ? (
        <button
          type="button"
          className={quietButtonClass}
          onClick={() => {
            onRecenter({ seq: first.seq, target: first.id });
          }}
        >
          {texts.older}
        </button>
      ) : null}
      <ol ref={list} aria-label={texts.history} className="flex flex-col px-1 pb-2">
        {items.map((message, index) => {
          const place = places[index];
          return place === undefined ? null : renderRow(message, place);
        })}
      </ol>
      {query.data?.has_newer === true && last !== undefined ? (
        <button
          type="button"
          className={quietButtonClass}
          onClick={() => {
            onRecenter({ seq: last.seq, target: last.id });
          }}
        >
          {texts.showNewer}
        </button>
      ) : null}
    </div>
  );
}
