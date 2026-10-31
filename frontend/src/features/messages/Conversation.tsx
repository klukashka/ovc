import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  type ReactElement,
  createContext,
  useContext,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';

import { useScheduler } from '../../app/clock.ts';
import { ActionButton, errorMessage, quietButtonClass } from '../../components/ui/Form.tsx';
import { useChatWrite } from '../../components/ui/chatWrite.ts';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { FocusedNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { asApiError, useApiClient, type ApiError } from '../../lib/api/client.ts';
import { useSession, useSignedInAccount } from '../../lib/api/session.ts';
import { formatDate } from '../../lib/format/dates.ts';
import {
  type Channel,
  type Message,
  type MessagePage,
  deleteMessage,
  editMessage,
  fetchMessages,
  sendMessage,
} from './api.ts';
import { channelKey, messagesKey } from './keys.ts';
import { MessageBody } from './MessageBody.tsx';
import { MessageMenu } from './MessageMenu.tsx';
import { useReadMarkers } from './readMarkers.ts';
import { scheduleChatReads } from './refetch.ts';
import type { ChatTexts } from './translations.ts';

export const MessageKeyContext = createContext<() => string>(() => crypto.randomUUID());

export function Conversation({
  channel,
  texts,
  onUnavailable,
}: {
  readonly channel: Channel;
  readonly texts: ChatTexts;
  readonly onUnavailable: () => void;
}): ReactElement {
  const client = useApiClient();
  const cache = useQueryClient();
  const scheduler = useScheduler();
  const newKey = useContext(MessageKeyContext);
  const me = useSignedInAccount();
  const session = useSession();
  const [ownership] = useState(() => ({
    epoch: session.currentEpoch(),
    query: cache.getQueryCache().find({ queryKey: channelKey(channel.id), exact: true }),
  }));
  const { t } = useTranslation();
  const heading = useRef<HTMLHeadingElement>(null);
  const list = useRef<HTMLOListElement>(null);
  const composer = useRef<HTMLTextAreaElement>(null);
  const alive = useRef(true);
  const [draft, setDraft] = useState('');
  const [editing, setEditing] = useState<Message | null>(null);
  const savedDraft = useRef('');
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<ApiError | null>(null);
  const [validation, setValidation] = useState<'emptyBody' | 'tooLong' | null>(null);
  const submission = useRef<{ readonly body: string; readonly key: string } | null>(null);
  const guard = useChatWrite(busy, !channel.writable);
  const deletion = useChatWrite(busy);
  const id = useId();
  const olderThan = useRef<number | null>(null);
  const newest = useRef<number | null>(null);
  const [announcement, setAnnouncement] = useState('');
  const oldestShown = useRef<number | null>(null);
  const olderRequest = useRef<AbortController | null>(null);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const query = useQuery({
    queryKey: messagesKey(channel.id),
    queryFn: async ({ signal }): Promise<MessagePage> => {
      const cached = cache.getQueryData<MessagePage>(messagesKey(channel.id));
      const floor = cached?.items.at(-1)?.seq ?? oldestShown.current;
      const items: Message[] = [];
      let cursor: string | null = null;
      do {
        const page = await fetchMessages(client, channel.id, cursor, signal);
        items.push(...page.items);
        cursor = page.next_cursor;
        const target = Math.min(floor ?? Infinity, oldestShown.current ?? Infinity);
        if ((items.at(-1)?.seq ?? 0) <= target) break;
      } while (cursor !== null);
      return { items, next_cursor: cursor };
    },
    retry: false,
  });
  const messages = useMemo(() => [...(query.data?.items ?? [])].sort((a, b) => a.seq - b.seq), [query.data]);
  useEffect(() => {
    const first = messages[0];
    if (first !== undefined) oldestShown.current = first.seq;
  }, [messages]);
  const read = useReadMarkers(
    channel.id,
    list,
    messages.map((message) => `${message.id}:${String(message.version)}`).join(','),
  );
  useFocusAfterRemovedRow(
    list,
    messages.map((message) => message.id),
    null,
    heading,
  );

  useEffect(() => {
    alive.current = true;
    heading.current?.focus();
    return (): void => {
      alive.current = false;
      olderRequest.current?.abort();
    };
  }, []);
  useEffect(() => {
    if (query.isError && asApiError(query.error).status === 404) onUnavailable();
  }, [query.isError, query.error, onUnavailable]);
  useLayoutEffect(() => {
    const before = olderThan.current;
    if (before !== null && !loadingOlder) {
      const first = messages.find((message) => message.seq < before);
      if (first !== undefined) {
        list.current?.querySelector<HTMLElement>(`[data-seq="${String(first.seq)}"]`)?.focus();
        olderThan.current = null;
      }
    }
  }, [messages, loadingOlder]);
  useEffect(() => {
    const previous = newest.current;
    const last = messages.at(-1)?.seq;
    if (last === undefined) return;
    newest.current = Math.max(previous ?? 0, last);
    if (previous !== null && messages.some((message) => message.seq > previous && !message.own))
      setAnnouncement(`${texts.newMessages} (${String(last)})`);
  }, [messages, texts.newMessages]);

  function isAlive(): boolean {
    const current = cache.getQueryCache().find({ queryKey: channelKey(channel.id), exact: true });
    return (
      alive.current &&
      session.currentEpoch() === ownership.epoch &&
      current !== undefined &&
      current === ownership.query &&
      !(current.state.status === 'error' && asApiError(current.state.error).status === 404)
    );
  }
  function focusMessage(messageId: string): void {
    for (const row of list.current?.querySelectorAll<HTMLElement>('[data-row]') ?? []) {
      if (row.dataset.row === messageId) {
        row.focus();
        return;
      }
    }
    heading.current?.focus();
  }
  async function showOlder(): Promise<void> {
    if (!isAlive()) return;
    const cursor = query.data?.next_cursor;
    if (cursor == null) return;
    setLoadingOlder(true);
    const request = new AbortController();
    olderRequest.current = request;
    try {
      const page = await fetchMessages(client, channel.id, cursor, request.signal);
      if (!isAlive() || request.signal.aborted) return;
      const first = page.items.at(-1);
      if (first !== undefined) oldestShown.current = Math.min(oldestShown.current ?? first.seq, first.seq);
      cache.setQueryData<MessagePage>(messagesKey(channel.id), (previous) => {
        const retained = previous?.items ?? [];
        const ids = new Set(retained.map((item) => item.id));
        return {
          items: [...retained, ...page.items.filter((item) => !ids.has(item.id))],
          next_cursor: page.next_cursor,
        };
      });
    } catch (error: unknown) {
      if (isAlive() && !request.signal.aborted) {
        olderThan.current = null;
        const refusal = asApiError(error);
        if (refusal.status === 404) onUnavailable();
        else setFailure(refusal);
      }
    } finally {
      if (isAlive()) setLoadingOlder(false);
    }
  }
  async function reload(): Promise<void> {
    scheduleChatReads(cache, scheduler);
    await Promise.all([
      cache.invalidateQueries({ queryKey: messagesKey(channel.id) }),
      cache.invalidateQueries({ queryKey: channelKey(channel.id) }),
    ]);
  }
  function cancelEdit(): void {
    if (editing === null) return;
    focusMessage(editing.id);
    setEditing(null);
    setDraft(savedDraft.current);
    setFailure(null);
  }
  async function submit(): Promise<void> {
    if (!isAlive()) return;
    const body = draft;
    if (body.trim() === '') {
      setValidation('emptyBody');
      return;
    }
    if (Array.from(body).length > 4000) {
      setValidation('tooLong');
      return;
    }
    setValidation(null);
    setFailure(null);
    setBusy(true);
    const original = editing;
    try {
      if (original === null) {
        if (submission.current?.body !== body) submission.current = { body, key: newKey() };
        await sendMessage(client, channel.id, body, submission.current.key);
      } else {
        await editMessage(client, original, body);
      }
      if (!isAlive()) return;
      submission.current = null;
      setDraft(original === null ? '' : savedDraft.current);
      setEditing(null);
      await reload();
      if (isAlive()) {
        if (original === null) composer.current?.focus();
        else focusMessage(original.id);
      }
    } catch (error: unknown) {
      if (isAlive()) {
        const refusal = asApiError(error);
        setFailure(refusal);
        if (refusal.status === 404) void reload();
        if (refusal.code === 'conflict' || refusal.code === 'channel_read_only') void reload();
      }
    } finally {
      if (isAlive()) setBusy(false);
    }
  }
  async function remove(message: Message): Promise<void> {
    if (!isAlive()) return;
    setFailure(null);
    setBusy(true);
    try {
      await deleteMessage(client, message.id);
      if (!isAlive()) return;
      await reload();
      if (isAlive()) focusMessage(message.id);
    } catch (error: unknown) {
      if (isAlive()) {
        setFailure(asApiError(error));
        await reload();
        if (isAlive()) focusMessage(message.id);
      }
    } finally {
      if (isAlive()) setBusy(false);
    }
  }
  if (query.isError && asApiError(query.error).status === 404) return <FocusedNotice message={texts.unavailable} />;
  return (
    <section className="flex min-w-0 flex-col gap-3">
      <h2 ref={heading} tabIndex={-1} dir="auto" className="text-heading outline-none">
        {channel.label ?? texts.deletedUser}
      </h2>
      {channel.tutor === null ? null : <p dir="auto">{channel.tutor.name}</p>}
      {channel.unconfirmed ? <p>{texts.unconfirmed}</p> : null}
      {channel.relationship_id === undefined ? null : (
        <Link className="text-accent-text underline" to={`/students/${encodeURIComponent(channel.relationship_id)}`}>
          {texts.card}
        </Link>
      )}
      {channel.group_id === undefined ? null : (
        <Link className="text-accent-text underline" to={`/groups/${encodeURIComponent(channel.group_id)}`}>
          {texts.group}
        </Link>
      )}
      <p aria-live="polite" className="sr-only">
        {announcement}
      </p>
      <p aria-live="polite" className={channel.writable ? 'sr-only' : 'text-text-muted'}>
        {channel.writable ? '' : texts.readonly}
      </p>
      {query.isPending ? <DelayedLoading /> : null}
      {query.isError && asApiError(query.error).status !== 404 ? (
        <LoadFailed
          error={asApiError(query.error)}
          onRetry={() => {
            void query.refetch();
          }}
        />
      ) : null}
      {query.data?.next_cursor != null ? (
        <button
          type="button"
          className={quietButtonClass}
          aria-disabled={loadingOlder || query.isFetching}
          onClick={() => {
            if (!loadingOlder && !query.isFetching) {
              olderThan.current = messages[0]?.seq ?? null;
              void showOlder();
            }
          }}
        >
          {texts.older}
        </button>
      ) : null}
      {query.isSuccess && messages.length === 0 ? <p>{texts.noMessages}</p> : null}
      <ol ref={list} aria-label={texts.history} className="flex flex-col gap-3">
        {messages.map((message, index) => {
          const day = formatDate(message.created_at, me.language, me.timezone);
          const previous = messages[index - 1];
          const firstOfDay =
            previous === undefined || formatDate(previous.created_at, me.language, me.timezone) !== day;
          const editable =
            message.own &&
            channel.writable &&
            scheduler.wallClock() - Date.parse(message.created_at) <= 24 * 60 * 60 * 1000;
          return (
            <li
              key={message.id}
              data-row={message.id}
              data-seq={message.seq}
              tabIndex={-1}
              className="rounded-md border border-border p-3 outline-none focus-visible:ring-2"
            >
              {firstOfDay ? <p className="mb-2 text-small text-text-muted">{day}</p> : null}
              <div className="flex items-center justify-between gap-2">
                <span dir="auto" className="text-strong">
                  {message.own ? texts.you : (message.sender_label ?? texts.deletedUser)}
                </span>
                {!message.deleted && !message.erased && (message.own || me.role === 'tutor') ? (
                  <MessageMenu
                    texts={texts}
                    editable={editable}
                    busy={busy}
                    onEdit={() => {
                      guard.run(() => {
                        if (editing === null) savedDraft.current = draft;
                        setEditing(message);
                        setDraft(message.body ?? '');
                        setFailure(null);
                        composer.current?.focus();
                      });
                    }}
                    onDelete={() => {
                      deletion.run(() => {
                        void remove(message);
                      });
                    }}
                  />
                ) : null}
              </div>
              {message.deleted ? (
                <p>{texts.deleted}</p>
              ) : message.erased ? (
                <p>{texts.erased}</p>
              ) : (
                <MessageBody body={message.body ?? ''} />
              )}
              {message.edited_at === null || message.deleted || message.erased ? null : (
                <span className="text-small text-text-muted">{texts.edited}</span>
              )}
              {message.sent_before_reclaim ? <p className="text-small">{texts.reclaimed}</p> : null}
            </li>
          );
        })}
      </ol>
      {read.failed ? (
        <button type="button" className={quietButtonClass} onClick={read.retry}>
          {texts.retryRead}
        </button>
      ) : null}
      {failure === null ? null : <p role="alert">{errorMessage(t, failure, null)}</p>}
      <form
        className="flex flex-col gap-2"
        onSubmit={(event) => {
          event.preventDefault();
          guard.run(() => {
            void submit();
          });
        }}
      >
        <label htmlFor={id}>{texts.composer}</label>
        <textarea
          ref={composer}
          id={id}
          value={draft}
          readOnly={!channel.writable || busy}
          aria-disabled={guard.unavailable}
          aria-describedby={validation === null ? undefined : `${id}-error`}
          rows={3}
          className="min-h-11 rounded-md border border-border-strong bg-surface px-3 py-2"
          onChange={(event) => {
            setDraft(event.target.value);
          }}
          onKeyDown={(event) => {
            if (event.key === 'Escape' && editing !== null) {
              event.preventDefault();
              cancelEdit();
            }
          }}
        />
        {validation === null ? null : (
          <p role="alert" id={`${id}-error`}>
            {texts[validation]}
          </p>
        )}
        <div className="flex gap-2">
          <ActionButton
            type="submit"
            busy={busy}
            unavailable={guard.unavailable}
            unavailableReason={guard.reason}
            className={quietButtonClass}
          >
            {editing === null ? texts.send : texts.save}
          </ActionButton>
          {editing === null ? null : (
            <button type="button" className={quietButtonClass} onClick={cancelEdit}>
              {texts.cancel}
            </button>
          )}
        </div>
      </form>
    </section>
  );
}
