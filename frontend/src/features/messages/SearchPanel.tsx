import { type ReactElement, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { errorMessage, quietButtonClass } from '../../components/ui/Form.tsx';
import { type ApiError, asApiError, useApiClient } from '../../lib/api/client.ts';
import { dateTimeFormat } from '../../lib/format/dates.ts';
import type { Language } from '../../lib/i18n/resources.ts';
import { type Message, SearchRefusal, searchMessages } from './api.ts';
import { CloseIcon } from './icons.tsx';
import type { ChatTexts } from './translations.ts';

const minimumTerm = 2;

type SearchState =
  | { readonly kind: 'idle' }
  | { readonly kind: 'short' }
  | { readonly kind: 'loading'; readonly items: readonly Message[]; readonly next: string | null }
  | {
      readonly kind: 'done';
      readonly items: readonly Message[];
      readonly next: string | null;
      readonly firstNew: number | null;
    }
  | {
      readonly kind: 'failed';
      readonly items: readonly Message[];
      readonly error: ApiError;
      readonly retryAfter: number | null;
    };

function itemsOf(state: SearchState): readonly Message[] {
  return state.kind === 'idle' || state.kind === 'short' ? [] : state.items;
}

export function SearchPanel({
  channelId,
  texts,
  language,
  timeZone,
  onClose,
  onOpen,
  onUnavailable,
}: {
  readonly channelId: string;
  readonly texts: ChatTexts;
  readonly language: Language;
  readonly timeZone: string;
  readonly onClose: () => void;
  readonly onOpen: (message: Message) => void;
  readonly onUnavailable: () => void;
}): ReactElement {
  const client = useApiClient();
  const { t } = useTranslation();
  const hintId = useId();
  const field = useRef<HTMLInputElement>(null);
  const results = useRef<HTMLOListElement>(null);
  const panel = useRef<HTMLElement>(null);
  const request = useRef<AbortController | null>(null);
  const [term, setTerm] = useState('');
  const [state, setState] = useState<SearchState>({ kind: 'idle' });

  useEffect(() => {
    const element = panel.current;
    if (element === null) return;
    const closeOnEscape = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') {
        event.preventDefault();
        onClose();
      }
    };
    element.addEventListener('keydown', closeOnEscape);
    return (): void => {
      element.removeEventListener('keydown', closeOnEscape);
    };
  }, [onClose]);

  useEffect(() => {
    field.current?.focus();
    return (): void => {
      request.current?.abort();
    };
  }, []);

  useEffect(() => {
    if (state.kind !== 'done' || state.firstNew === null) return;
    const first = results.current?.querySelectorAll<HTMLButtonElement>('button[data-result]')[state.firstNew];
    if (first === undefined) field.current?.focus();
    else first.focus();
  }, [state]);

  function run(cursor: string | null, previous: readonly Message[]): void {
    const trimmed = term.trim();
    request.current?.abort();
    if (Array.from(trimmed).length < minimumTerm) {
      request.current = null;
      setState({ kind: 'short' });
      return;
    }
    const controller = new AbortController();
    request.current = controller;
    setState({ kind: 'loading', items: previous, next: cursor });
    searchMessages(client, channelId, trimmed, cursor, controller.signal).then(
      (page) => {
        if (request.current !== controller) return;
        setState({
          kind: 'done',
          items: [...previous, ...page.items],
          next: page.next_cursor,
          firstNew: cursor === null ? null : previous.length,
        });
      },
      (error: unknown) => {
        if (controller.signal.aborted || request.current !== controller) return;
        const refusal = error instanceof SearchRefusal ? error.refusal : asApiError(error);
        if (refusal.status === 404) {
          onUnavailable();
          return;
        }
        const retryAfter = error instanceof SearchRefusal ? error.retryAfter : null;
        setState({ kind: 'failed', items: previous, error: refusal, retryAfter });
        field.current?.focus();
      },
    );
  }

  const items = itemsOf(state);
  const time = dateTimeFormat(language, {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
    timeZone,
  });
  return (
    <section
      ref={panel}
      aria-label={texts.searchOpen}
      className="flex flex-col gap-2 rounded-md border border-border bg-surface p-3"
    >
      <div className="flex items-center gap-2">
        <input
          ref={field}
          type="search"
          autoComplete="off"
          aria-label={texts.searchField}
          aria-describedby={hintId}
          placeholder={texts.searchField}
          value={term}
          className="min-h-11 min-w-0 flex-1 rounded-md border border-border-strong bg-field px-3 py-2"
          onChange={(event) => {
            request.current?.abort();
            request.current = null;
            setTerm(event.target.value);
            setState({ kind: 'idle' });
          }}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault();
              run(null, []);
            }
          }}
        />
        <button
          type="button"
          className={quietButtonClass}
          onClick={() => {
            run(null, []);
          }}
        >
          {texts.searchSubmit}
        </button>
        <button
          type="button"
          aria-label={texts.searchClose}
          className="inline-flex min-h-11 min-w-11 items-center justify-center rounded-md hover:bg-hover"
          onClick={onClose}
        >
          <CloseIcon size={16} />
        </button>
      </div>
      <p id={hintId} className="text-small text-text-muted">
        {state.kind === 'short' ? texts.searchTooShort : texts.searchHint}
      </p>
      {state.kind === 'failed' ? <p role="alert">{errorMessage(t, state.error, state.retryAfter)}</p> : null}
      {state.kind === 'done' && items.length === 0 ? <p role="status">{texts.searchEmpty}</p> : null}
      {items.length === 0 ? null : (
        <ol ref={results} aria-label={texts.searchResults} className="flex flex-col gap-1">
          {items.map((item) => (
            <li key={item.id}>
              <button
                type="button"
                data-result=""
                className="flex w-full flex-col gap-0.5 rounded-md px-2 py-1.5 text-left hover:bg-hover"
                onClick={() => {
                  onOpen(item);
                }}
              >
                <span className="flex gap-2 text-caption text-text-muted">
                  <span dir="auto">{item.own ? texts.you : (item.sender_label ?? texts.deletedUser)}</span>
                  <time dateTime={item.created_at}>{time.format(new Date(item.created_at))}</time>
                </span>
                <span dir="auto" className="line-clamp-3 whitespace-pre-wrap [overflow-wrap:anywhere]">
                  {item.body}
                </span>
                {item.sent_before_reclaim ? (
                  <span className="text-small text-text-muted">{texts.reclaimed}</span>
                ) : null}
              </button>
            </li>
          ))}
        </ol>
      )}
      {(state.kind === 'done' || state.kind === 'loading') && state.next !== null ? (
        <button
          type="button"
          className={quietButtonClass}
          aria-disabled={state.kind === 'loading'}
          onClick={() => {
            if (state.kind === 'done') run(state.next, items);
          }}
        >
          {texts.more}
        </button>
      ) : null}
    </section>
  );
}
