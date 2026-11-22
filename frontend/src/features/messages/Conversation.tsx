import { type QueryObserverResult, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  type ReactElement,
  type ReactNode,
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
import { ActionButton, errorMessage, primaryButtonClass, quietButtonClass } from '../../components/ui/Form.tsx';
import { useChatWrite } from '../../components/ui/chatWrite.ts';
import { useFocusAfterRemovedRow } from '../../components/ui/liveRows.ts';
import { FocusedNotice } from '../../components/ui/LiveNotice.tsx';
import { DelayedLoading, LoadFailed } from '../../components/ui/LoadState.tsx';
import { asApiError, useApiClient, type ApiError } from '../../lib/api/client.ts';
import { useSession, useSignedInAccount } from '../../lib/api/session.ts';
import { formatDate } from '../../lib/format/dates.ts';
import { type Channel, type Message, type MessagePage, deleteMessage, editMessage, sendMessage } from './api.ts';
import { Attachments, useAttachmentExpiry } from './Attachments.tsx';
import { UploadComposer } from './UploadComposer.tsx';
import { useUploads } from './uploads.ts';
import { useChatSignals } from './presence.ts';
import { PresenceSummary } from './PresenceSummary.tsx';
import { TypingIndicator } from './TypingIndicator.tsx';
import { channelKey, messagesKey } from './keys.ts';
import { MessageBody } from './MessageBody.tsx';
import { MessageMenu } from './MessageMenu.tsx';
import { useReadMarkers } from './readMarkers.ts';
import { chatRefetch, scheduleChatReads } from './refetch.ts';
import { HistoryRefresh } from './historyRefresh.ts';
import type { ChatTexts } from './translations.ts';

const composerMaximumRows = 8;

const bottomTolerance = 24;

function fitComposerRows(field: HTMLTextAreaElement): void {
  field.rows = 1;
  while (field.scrollHeight > field.clientHeight && field.rows < composerMaximumRows) {
    field.rows += 1;
  }
}

export const MessageKeyContext = createContext<() => string>(() => crypto.randomUUID());

function messageIsEditable(message: Message | undefined, writable: boolean, now: number): boolean {
  return (
    message !== undefined &&
    writable &&
    message.own &&
    !message.deleted &&
    !message.erased &&
    now - Date.parse(message.created_at) <= 24 * 60 * 60 * 1000
  );
}

export function Conversation({
  channel,
  texts,
  onUnavailable,
  channelError,
  refreshChannel,
  revision = 0,
  back = null,
}: {
  readonly revision?: number;
  readonly back?: ReactNode;
  readonly channel: Channel;
  readonly texts: ChatTexts;
  readonly onUnavailable: () => void;
  readonly channelError: ApiError | null;
  readonly refreshChannel: () => Promise<QueryObserverResult<Channel>>;
}): ReactElement {
  const client = useApiClient();
  const cache = useQueryClient();
  const scheduler = useScheduler();
  const newKey = useContext(MessageKeyContext);
  const me = useSignedInAccount();
  const session = useSession();
  const presence = useChatSignals(channel, me.id, me.role, revision);
  const [ownership] = useState(() => ({
    epoch: session.currentEpoch(),
    query: cache.getQueryCache().find({ queryKey: channelKey(channel.id), exact: true }),
  }));
  const { t } = useTranslation();
  const heading = useRef<HTMLHeadingElement>(null);
  const list = useRef<HTMLOListElement>(null);
  const composer = useRef<HTMLTextAreaElement>(null);
  const conflictContainer = useRef<HTMLDivElement>(null);
  const alive = useRef(true);
  const [draft, setDraft] = useState('');
  const [editing, setEditing] = useState<Message | null>(null);
  const savedDraft = useRef('');
  const editGeneration = useRef(0);
  const conflictRefresh = useRef<{ readonly cancel: () => void } | null>(null);
  const [conflict, setConflict] = useState<{
    readonly generation: number;
    readonly latest: Message | null;
    readonly error: ApiError | null;
  } | null>(null);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<ApiError | null>(null);
  const [validation, setValidation] = useState<'emptyBody' | 'tooLong' | 'tooManyFiles' | 'waitingUploads' | null>(
    null,
  );
  const submission = useRef<{ readonly body: string; readonly attachments: string; readonly key: string } | null>(null);
  const uploads = useUploads(channel.id, channel.writable, isAlive);
  const attachmentGuard = useChatWrite(busy, !channel.writable || !me.email_verified || editing !== null);
  const uploadError = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (validation === 'tooManyFiles' || validation === 'waitingUploads') uploadError.current?.focus();
  }, [validation]);
  const guard = useChatWrite(busy, !channel.writable);
  const deletion = useChatWrite(busy);
  const id = useId();
  const olderThan = useRef<number | null>(null);
  const newest = useRef<number | null>(null);
  const [announcement, setAnnouncement] = useState('');
  const oldestShown = useRef<number | null>(null);
  const olderRequest = useRef<AbortController | null>(null);
  const fileRefreshPaused = useRef(false);
  const fileRefresh = useRef<{ readonly promise: Promise<boolean>; readonly cancel: () => void } | null>(null);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [history] = useState(
    () =>
      new HistoryRefresh(cache, client, channel.id, isAlive, onUnavailable, () => {
        chatRefetch(cache, scheduler).changed(channel.id);
      }),
  );
  useEffect(() => chatRefetch(cache, scheduler).mount(channel.id, history), [cache, scheduler, channel.id, history]);
  const query = useQuery({
    queryKey: messagesKey(channel.id),
    queryFn: ({ signal }): Promise<MessagePage> => history.full(signal, () => oldestShown.current),
    retry: false,
  });
  const messages = useMemo(() => [...(query.data?.items ?? [])].sort((a, b) => a.seq - b.seq), [query.data]);
  const historyPane = useRef<HTMLDivElement>(null);
  const historyContent = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  const lastShown = useRef<string | null>(null);
  const rememberBottom = (): void => {
    const element = historyPane.current;
    if (element !== null) {
      atBottom.current = element.scrollHeight - element.scrollTop - element.clientHeight <= bottomTolerance;
    }
  };
  const latestMessage = messages.at(-1);
  useLayoutEffect(() => {
    const element = historyPane.current;
    if (element === null || latestMessage === undefined || latestMessage.id === lastShown.current) {
      return;
    }
    const opening = lastShown.current === null;
    lastShown.current = latestMessage.id;
    if (opening || atBottom.current || latestMessage.own) {
      element.scrollTop = element.scrollHeight;
      atBottom.current = true;
    }
  }, [latestMessage]);
  useLayoutEffect(() => {
    const field = composer.current;
    if (field === null) {
      return;
    }
    const keepBottom = atBottom.current;
    fitComposerRows(field);
    const pane = historyPane.current;
    if (keepBottom && pane !== null) {
      pane.scrollTop = pane.scrollHeight;
      atBottom.current = true;
    }
  }, [draft]);
  const unavailable = query.isError && asApiError(query.error).status === 404;
  useEffect(() => {
    const pane = historyPane.current;
    const content = historyContent.current;
    if (unavailable || pane === null || content === null) {
      return;
    }
    const keepPinned = (): void => {
      if (atBottom.current) {
        pane.scrollTop = pane.scrollHeight;
      }
    };
    const windowResized = (): void => {
      const field = composer.current;
      if (field !== null) {
        fitComposerRows(field);
      }
      keepPinned();
    };
    window.addEventListener('resize', windowResized);
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(keepPinned);
    observer?.observe(pane);
    observer?.observe(content);
    return (): void => {
      observer?.disconnect();
      window.removeEventListener('resize', windowResized);
    };
  }, [unavailable]);
  useAttachmentExpiry(messages, refreshAttachments, isAlive);
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
    const pause = (): void => {
      fileRefreshPaused.current = true;
      fileRefresh.current?.cancel();
      fileRefresh.current = null;
    };
    const resume = (): void => {
      fileRefreshPaused.current = document.visibilityState === 'hidden';
    };
    const visibility = (): void => {
      if (document.visibilityState === 'hidden') pause();
      else resume();
    };
    window.addEventListener('pagehide', pause);
    window.addEventListener('pageshow', resume);
    document.addEventListener('visibilitychange', visibility);
    return (): void => {
      window.removeEventListener('pagehide', pause);
      window.removeEventListener('pageshow', resume);
      document.removeEventListener('visibilitychange', visibility);
    };
  }, []);
  useEffect(() => {
    alive.current = true;
    heading.current?.focus();
    return (): void => {
      alive.current = false;
      conflictRefresh.current?.cancel();
      conflictRefresh.current = null;
      olderRequest.current?.abort();
      fileRefresh.current?.cancel();
      fileRefresh.current = null;
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
        const pane = historyPane.current;
        if (pane !== null) {
          atBottom.current = pane.scrollHeight - pane.scrollTop - pane.clientHeight <= bottomTolerance;
        }
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

  const resolutionAvailable =
    conflict?.latest != null &&
    messageIsEditable(
      messages.find((message) => message.id === conflict.latest?.id),
      channel.writable,
      scheduler.wallClock(),
    );
  useFocusAfterRemovedRow(conflictContainer, resolutionAvailable ? ['conflict'] : [], null, composer);
  if (conflict?.latest != null && !resolutionAvailable) setConflict({ ...conflict, latest: null });

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
      const page = await history.older(request.signal);
      if (page === null || !isAlive() || request.signal.aborted) {
        rememberBottom();
        return;
      }
      const first = page.items.at(-1);
      if (first === undefined) rememberBottom();
      else oldestShown.current = Math.min(oldestShown.current ?? first.seq, first.seq);
    } catch (error: unknown) {
      if (isAlive() && !request.signal.aborted) {
        olderThan.current = null;
        rememberBottom();
        const refusal = asApiError(error);
        if (refusal.status === 404) onUnavailable();
        else setFailure(refusal);
      }
    } finally {
      if (isAlive()) setLoadingOlder(false);
    }
  }
  function refreshAttachments(): Promise<boolean> {
    if (!isAlive() || fileRefreshPaused.current || document.visibilityState === 'hidden') return Promise.resolve(false);
    if (fileRefresh.current !== null) return fileRefresh.current.promise;
    let cancel = (): void => undefined;
    const promise = new Promise<boolean>((resolve) => {
      const cancelStart = scheduler.schedule(1000, () => {
        if (!isAlive() || fileRefreshPaused.current || document.visibilityState === 'hidden') {
          fileRefresh.current = null;
          resolve(false);
          return;
        }
        void cache.invalidateQueries({ queryKey: messagesKey(channel.id) }, { cancelRefetch: false }).then(() => {
          fileRefresh.current = null;
          resolve(isAlive() && !fileRefreshPaused.current && document.visibilityState !== 'hidden');
        });
      });
      cancel = (): void => {
        cancelStart();
        resolve(false);
      };
    });
    fileRefresh.current = { promise, cancel };
    return promise;
  }
  function chooseFiles(files: readonly File[]): void {
    attachmentGuard.run(() => {
      if (!isAlive()) return;
      if (!uploads.queue.add(files)) setValidation('tooManyFiles');
      else setValidation(null);
    });
  }
  async function reload(): Promise<void> {
    scheduleChatReads(cache, scheduler);
    await Promise.all([
      cache.invalidateQueries({ queryKey: messagesKey(channel.id) }),
      cache.invalidateQueries({ queryKey: channelKey(channel.id) }),
    ]);
  }
  function canResolve(message: Message): boolean {
    const currentChannel = cache.getQueryData<Channel>(channelKey(channel.id));
    const currentMessage = cache
      .getQueryData<MessagePage>(messagesKey(channel.id))
      ?.items.find((item) => item.id === message.id);
    return isAlive() && messageIsEditable(currentMessage, currentChannel?.writable === true, scheduler.wallClock());
  }
  async function refreshConflict(messageId: string, generation: number): Promise<void> {
    if (!isAlive() || editGeneration.current !== generation) return;
    setConflict({ generation, latest: null, error: null });
    const historyUpdates = cache.getQueryState(messagesKey(channel.id))?.dataUpdateCount ?? 0;
    const channelUpdates = cache.getQueryState(channelKey(channel.id))?.dataUpdateCount ?? 0;
    scheduleChatReads(cache, scheduler);
    conflictRefresh.current?.cancel();
    let cancel = (): void => undefined;
    const cancelled = new Promise<null>((resolve) => {
      cancel = (): void => {
        resolve(null);
      };
    });
    const request = { cancel };
    conflictRefresh.current = request;
    const results = await Promise.race([Promise.all([query.refetch(), refreshChannel()]), cancelled]).finally(() => {
      request.cancel();
      if (conflictRefresh.current === request) conflictRefresh.current = null;
    });
    if (results === null || !isAlive() || editGeneration.current !== generation) return;
    const [historyResult, channelResult] = results;
    const history = cache.getQueryState<MessagePage>(messagesKey(channel.id));
    const detail = cache.getQueryState<Channel>(channelKey(channel.id));
    const fresh =
      historyResult.isSuccess &&
      channelResult.isSuccess &&
      history !== undefined &&
      history.dataUpdateCount > historyUpdates &&
      detail !== undefined &&
      detail.dataUpdateCount > channelUpdates;
    if (!fresh) {
      setConflict({ generation, latest: null, error: asApiError(historyResult.error ?? channelResult.error) });
      return;
    }
    const latest = historyResult.data.items.find((item) => item.id === messageId);
    if (latest !== undefined && canResolve(latest)) setConflict({ generation, latest, error: null });
  }
  function resolveConflict(useLatest: boolean): void {
    const latest = conflict?.latest;
    if (latest == null || conflict?.generation !== editGeneration.current || !canResolve(latest)) return;
    setEditing(latest);
    if (useLatest) setDraft(latest.body ?? '');
    setConflict(null);
    setFailure(null);
    composer.current?.focus();
  }
  function cancelEdit(): void {
    if (editing === null) return;
    editGeneration.current += 1;
    conflictRefresh.current?.cancel();
    conflictRefresh.current = null;
    setConflict(null);
    presence.stopTyping();
    focusMessage(editing.id);
    setEditing(null);
    setDraft(savedDraft.current);
    setFailure(null);
  }
  async function submit(): Promise<void> {
    if (!isAlive() || conflict !== null) return;
    const generation = editGeneration.current;
    const body = draft;
    const attachments =
      editing === null
        ? uploads.items.flatMap((item) => (item.uploaded === null ? [] : [item.uploaded.id]))
        : editing.attachments.map((item) => item.id);
    if (editing === null && uploads.items.some((item) => item.state !== 'ready')) {
      setValidation('waitingUploads');
      return;
    }
    if (body.trim() === '' && attachments.length === 0) {
      setValidation('emptyBody');
      return;
    }
    if (Array.from(body).length > 4000) {
      setValidation('tooLong');
      return;
    }
    presence.stopTyping();
    setValidation(null);
    setFailure(null);
    setBusy(true);
    const original = editing;
    try {
      if (original === null) {
        const attachmentKey = JSON.stringify(attachments);
        if (submission.current?.body !== body || submission.current.attachments !== attachmentKey)
          submission.current = { body, attachments: attachmentKey, key: newKey() };
        await sendMessage(client, channel.id, body, submission.current.key, attachments);
      } else {
        await editMessage(client, original, body);
      }
      if (!isAlive() || editGeneration.current !== generation) return;
      submission.current = null;
      if (original === null) uploads.queue.clear();
      setDraft(original === null ? '' : savedDraft.current);
      setEditing(null);
      await reload();
      if (isAlive() && editGeneration.current === generation) {
        if (original === null) composer.current?.focus();
        else focusMessage(original.id);
      }
    } catch (error: unknown) {
      if (isAlive() && editGeneration.current === generation) {
        const refusal = asApiError(error);
        setFailure(refusal);
        if (refusal.status === 404) void reload();
        if (refusal.code === 'conflict' && original !== null) {
          setConflict({ generation, latest: null, error: null });
          void refreshConflict(original.id, generation);
        } else if (refusal.code === 'channel_read_only') void reload();
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
  const phone = back !== null;
  const shownAsPhone = useRef(phone);
  useEffect(() => {
    if (shownAsPhone.current === phone) return;
    shownAsPhone.current = phone;
    const active = document.activeElement;
    if (active === null || active === document.body || !active.isConnected) heading.current?.focus();
  }, [phone]);
  if (unavailable)
    return (
      <>
        {back}
        <FocusedNotice message={texts.unavailable} />
      </>
    );
  const title = (
    <h2 ref={heading} tabIndex={-1} dir="auto" className="min-w-0 text-heading [overflow-wrap:anywhere] outline-none">
      {channel.label ?? texts.deletedUser}
    </h2>
  );
  const presenceSummary = (
    <PresenceSummary
      presence={presence}
      group={channel.kind === 'group'}
      viewerRole={me.role}
      onlineLabel={texts.online}
      membersLabel={texts.members}
      collapsed={phone}
    />
  );
  const linkClass = `text-accent-text underline${phone ? ' inline-flex min-h-11 items-center' : ''}`;
  return (
    <section className={`flex min-w-0 flex-col gap-3 desktop:h-full desktop:min-h-0${phone ? ' min-h-0 flex-1' : ''}`}>
      <div className={phone ? 'flex items-center gap-1' : 'contents'}>
        {back}
        <div className={phone ? 'flex min-w-0 flex-1 flex-col' : 'contents'}>
          {title}
          {phone && channel.kind === 'group' ? null : presenceSummary}
        </div>
      </div>
      <div className={phone ? 'flex flex-wrap items-center gap-x-4 empty:hidden' : 'contents'}>
        {phone && channel.kind === 'group' ? presenceSummary : null}
        {channel.tutor === null ? null : (
          <p dir="auto" className="min-w-0 [overflow-wrap:anywhere]">
            {channel.tutor.name}
          </p>
        )}
        {channel.unconfirmed ? <p>{texts.unconfirmed}</p> : null}
        {channel.relationship_id === undefined ? null : (
          <Link className={linkClass} to={`/students/${encodeURIComponent(channel.relationship_id)}`}>
            {texts.card}
          </Link>
        )}
        {channel.group_id === undefined ? null : (
          <Link className={linkClass} to={`/groups/${encodeURIComponent(channel.group_id)}`}>
            {texts.group}
          </Link>
        )}
      </div>
      <p aria-live="polite" className="sr-only">
        {announcement}
      </p>
      <p aria-live="polite" className={channel.writable ? 'sr-only' : 'text-text-muted'}>
        {channel.writable ? '' : texts.readonly}
      </p>
      <div
        ref={historyPane}
        onScroll={rememberBottom}
        className={`flex min-w-0 flex-col desktop:min-h-0 desktop:flex-1 desktop:overflow-y-auto${phone ? ' min-h-0 flex-1 overflow-y-auto' : ''}`}
      >
        <div ref={historyContent} className="flex min-w-0 shrink-0 flex-col gap-3">
          {query.isPending ? <DelayedLoading /> : null}
          {conflict?.error != null ||
          channelError !== null ||
          (conflict === null && query.isError && asApiError(query.error).status !== 404) ? (
            <LoadFailed
              error={conflict?.error ?? channelError ?? asApiError(query.error)}
              onRetry={() => {
                if (conflict !== null && editing !== null) void refreshConflict(editing.id, conflict.generation);
                else if (channelError !== null) void refreshChannel();
                else void query.refetch();
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
                  atBottom.current = false;
                  olderThan.current = messages[0]?.seq ?? null;
                  void showOlder();
                }
              }}
            >
              {texts.older}
            </button>
          ) : null}
          {query.isSuccess && messages.length === 0 ? <p>{texts.noMessages}</p> : null}
          <ol ref={list} aria-label={texts.history} className="flex flex-col border-t border-border">
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
                  className="border-b border-border px-1 py-3 outline-none focus-visible:ring-2"
                >
                  {firstOfDay ? <p className="mb-2 text-small text-text-muted">{day}</p> : null}
                  <div className="flex items-center justify-between gap-2">
                    <span dir="auto" className="min-w-0 text-strong [overflow-wrap:anywhere]">
                      {message.own ? texts.you : (message.sender_label ?? texts.deletedUser)}
                    </span>
                    {!message.deleted && !message.erased && (message.own || me.role === 'tutor') ? (
                      <MessageMenu
                        texts={texts}
                        editable={editable}
                        busy={busy}
                        onEdit={() => {
                          guard.run(() => {
                            editGeneration.current += 1;
                            conflictRefresh.current?.cancel();
                            conflictRefresh.current = null;
                            setConflict(null);
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
                  <Attachments message={message} texts={texts} refresh={refreshAttachments} />
                  {message.edited_at === null || message.deleted || message.erased ? null : (
                    <span className="text-small text-text-muted">{texts.edited}</span>
                  )}
                  {message.sent_before_reclaim ? <p className="text-small">{texts.reclaimed}</p> : null}
                </li>
              );
            })}
          </ol>
          <TypingIndicator
            presence={presence}
            someoneTyping={texts.someoneTyping}
            namedTyping={(name) => texts.namedTyping.replace('{name}', name)}
          />
        </div>
      </div>
      {read.failed ? (
        <button type="button" className={quietButtonClass} onClick={read.retry}>
          {texts.retryRead}
        </button>
      ) : null}
      {failure === null ? null : <p role="alert">{errorMessage(t, failure, null)}</p>}
      <div ref={conflictContainer}>
        {conflict?.latest != null && resolutionAvailable ? (
          <section data-row="conflict" aria-label={texts.latestText} className="rounded-md border border-border p-3">
            <p>{texts.latestText}</p>
            <MessageBody body={conflict.latest.body ?? ''} />
            <button
              type="button"
              className={quietButtonClass}
              onClick={() => {
                resolveConflict(true);
              }}
            >
              {texts.useLatest}
            </button>
            <button
              type="button"
              className={quietButtonClass}
              onClick={() => {
                resolveConflict(false);
              }}
            >
              {texts.keepMine}
            </button>
          </section>
        ) : null}
      </div>
      <form
        className={`flex shrink-0 flex-col gap-2 border-t border-border pt-3${phone ? ' pb-[env(safe-area-inset-bottom)]' : ''}`}
        onSubmit={(event) => {
          event.preventDefault();
          guard.run(() => {
            void submit();
          });
        }}
      >
        <label htmlFor={id}>{texts.composer}</label>
        <textarea
          onDragOver={(event) => {
            if (event.dataTransfer.types.includes('Files')) event.preventDefault();
          }}
          onDrop={(event) => {
            event.preventDefault();
            chooseFiles(Array.from(event.dataTransfer.files));
          }}
          onPaste={(event) => {
            const pictures = Array.from(event.clipboardData.files).filter((file) => file.type.startsWith('image/'));
            if (pictures.length > 0) {
              event.preventDefault();
              chooseFiles(pictures);
            }
          }}
          ref={composer}
          id={id}
          value={draft}
          readOnly={!channel.writable || busy}
          aria-disabled={guard.unavailable}
          aria-describedby={validation === null ? undefined : `${id}-error`}
          rows={1}
          className="min-h-11 resize-none rounded-md border border-border-strong bg-field px-3 py-2"
          onChange={(event) => {
            setDraft(event.target.value);
            if (event.target.value.trim() === '') presence.stopTyping();
            else presence.startTyping();
          }}
          onBlur={presence.stopTyping}
          onKeyDown={(event) => {
            if (event.key === 'Escape' && editing !== null) {
              event.preventDefault();
              cancelEdit();
            }
          }}
        />
        {validation === null ? null : (
          <p ref={uploadError} role="alert" tabIndex={-1} id={`${id}-error`}>
            {texts[validation]}
          </p>
        )}
        {editing === null ? (
          <UploadComposer
            focusFallback={composer}
            items={uploads.items}
            texts={texts}
            confirmed={me.email_verified}
            busy={busy}
            unavailable={attachmentGuard.unavailable}
            reason={attachmentGuard.reason}
            choose={chooseFiles}
            remove={(key) => {
              uploads.queue.remove(key);
            }}
          />
        ) : null}
        <div className="flex gap-2">
          <ActionButton
            type="submit"
            busy={busy}
            unavailable={guard.unavailable || conflict !== null}
            unavailableReason={conflict !== null ? texts.resolveConflict : guard.reason}
            className={primaryButtonClass}
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
