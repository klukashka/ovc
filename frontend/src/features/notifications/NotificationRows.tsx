import { useQueryClient } from '@tanstack/react-query';
import { type ReactElement, type Ref, type RefObject, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import {
  DataTable,
  type TableColumn,
  cellClass,
  narrowOnlyClass,
  primaryCellClass,
  rowLinkClass,
  rowLinkSelector,
  secondaryCellClass,
  tableRowClass,
} from '../../components/ui/DataTable.tsx';
import { ActionButton, quietButtonClass } from '../../components/ui/Form.tsx';
import { BellIcon } from '../../components/ui/icons.tsx';
import { LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { LoadFailed, focusHeadingIfLost } from '../../components/ui/LoadState.tsx';
import { paletteDotClass } from '../../components/ui/palette.ts';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { done, useAction } from '../../components/ui/action.ts';
import { asApiError, useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatAgo, formatDateTime } from '../../lib/format/dates.ts';
import { type TextLoaders, useFeatureTexts } from '../../lib/i18n/featureTexts.ts';
import {
  type Notification,
  type NotificationEvent,
  type ReadSelection,
  linkAddress,
  markNotificationsRead,
  notificationsKey,
} from './api.ts';
import { notificationEventTextLoaders } from './texts.ts';

const eventTexts = {
  'invitation.accepted': 'notifications.event.invitationAccepted',
  'join_request.created': 'notifications.event.joinRequestCreated',
  'join_request.accepted': 'notifications.event.joinRequestAccepted',
  'join_request.declined': 'notifications.event.joinRequestDeclined',
  'relationship.ended': 'notifications.event.relationshipEnded',
  'lesson.reminder_1h': 'notifications.event.lessonReminder1h',
  'lesson.moved': 'notifications.event.lessonMoved',
  'lesson.cancelled': 'notifications.event.lessonCancelled',
  'lesson.series_changed': 'notifications.event.lessonSeriesChanged',
  'group.archived': 'notifications.event.groupArchived',
  'issue.created': 'notificationEvents.issueCreated',
  'issue.withdrawn': 'notificationEvents.issueWithdrawn',
  'submission.submitted': 'notificationEvents.submissionSubmitted',
  'submission.checked': 'notificationEvents.submissionChecked',
  'submission.returned': 'notificationEvents.submissionReturned',
  'submission.review_updated': 'notificationEvents.submissionReviewUpdated',
} as const satisfies Readonly<Record<NotificationEvent, string>>;

const lazySectionPrefix = 'notificationEvents.';

export type EventTextLoaders = TextLoaders<'notificationEvents'>;

type EventText = (item: Notification) => string;

interface EventTexts {
  readonly textOf: EventText;
  readonly failed: boolean;
  readonly retry: () => void;
}

function needsLazyText(item: Notification): boolean {
  return eventTexts[item.event].startsWith(lazySectionPrefix);
}

function useEventTexts(loaders: EventTextLoaders): EventTexts {
  const { t } = useTranslation();
  const texts = useFeatureTexts('notificationEvents', loaders);
  return {
    textOf: (item) =>
      needsLazyText(item) && !texts.ready ? t('notifications.event.homework') : t(eventTexts[item.event]),
    failed: texts.failed,
    retry: texts.retry,
  };
}

function EventTextsFailed({
  items,
  texts,
  focusIfLost,
}: {
  readonly items: readonly Notification[];
  readonly texts: EventTexts;
  readonly focusIfLost: () => void;
}): ReactElement | null {
  if (!texts.failed || !items.some(needsLazyText)) {
    return null;
  }
  return <LoadFailed error={asApiError(undefined)} onRetry={texts.retry} focusIfLost={focusIfLost} />;
}

export function useMarkRead(): (selection: ReadSelection) => Promise<void> {
  const client = useApiClient();
  const queryClient = useQueryClient();
  return async (selection) => {
    try {
      await markNotificationsRead(client, selection);
    } finally {
      void queryClient.invalidateQueries({ queryKey: notificationsKey });
    }
  };
}

function Label({ item }: { readonly item: Notification }): ReactElement | null {
  const { t } = useTranslation();
  if (item.label_deleted) {
    return <span className="text-strong">{t('notifications.deletedAccount')}</span>;
  }
  const label = item.label ?? (item.label_kind === 'group' ? t('notifications.groupLesson') : null);
  if (label === null) {
    return null;
  }
  return (
    <span className="flex items-center gap-1.5 text-strong">
      {item.colour === null ? null : (
        <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[item.colour]}`} />
      )}
      <span dir="auto" className="min-w-0 break-words">
        {label}
      </span>
    </span>
  );
}

function RowContent({
  item,
  now,
  eventText,
}: {
  readonly item: Notification;
  readonly now: number;
  readonly eventText: string;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const unread = item.read_at === null;
  return (
    <>
      <span aria-hidden="true" className="mt-0.5 text-icon">
        <BellIcon />
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <Label item={item} />
        <span>{eventText}</span>
        <time
          dateTime={item.created_at}
          title={formatDateTime(item.created_at, me.language, me.timezone)}
          className="tabular text-small text-text-muted"
        >
          {formatAgo(item.created_at, me.language, me.timezone, now)}
        </time>
      </span>
      {unread ? (
        <span className="mt-2 shrink-0">
          <span aria-hidden="true" className="block size-news-dot rounded-full bg-accent-text" />
          <span className="sr-only">{t('notifications.unread')}</span>
        </span>
      ) : null}
    </>
  );
}

const rowClass = 'flex gap-3 px-3 py-3 outline-none';

function NotificationRow({
  item,
  now,
  eventText,
  onChoose,
}: {
  readonly item: Notification;
  readonly now: number;
  readonly eventText: string;
  readonly onChoose: () => void;
}): ReactElement {
  const markRead = useMarkRead();
  const link = item.link;
  if (link === null) {
    return (
      <li data-row={item.id} tabIndex={-1} className={`${rowClass} border-t border-border`}>
        <RowContent item={item} now={now} eventText={eventText} />
      </li>
    );
  }
  return (
    <li data-row={item.id} className="border-t border-border">
      <Link
        to={linkAddress(link)}
        onClick={() => {
          if (item.read_at === null) {
            markRead({ ids: [item.id] }).catch(reportError);
          }
          onChoose();
        }}
        className={`${rowClass} rounded-sm hover:bg-hover focus-visible:bg-hover`}
      >
        <RowContent item={item} now={now} eventText={eventText} />
      </Link>
    </li>
  );
}

function useNow(items: readonly Notification[]): number {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const refresh = (): void => {
      setNow(Date.now());
    };
    const frame = window.requestAnimationFrame(refresh);
    const interval = window.setInterval(refresh, 60_000);
    return (): void => {
      window.cancelAnimationFrame(frame);
      window.clearInterval(interval);
    };
  }, [items]);
  return now;
}

function UnreadMarker({ item }: { readonly item: Notification }): ReactElement | null {
  const { t } = useTranslation();
  if (item.read_at !== null) {
    return null;
  }
  return (
    <span className="shrink-0">
      <span aria-hidden="true" className="block size-news-dot rounded-full bg-accent-text" />
      <span className="sr-only">{t('notifications.unread')}</span>
    </span>
  );
}

function EventCell({ item, text }: { readonly item: Notification; readonly text: string }): ReactElement {
  const markRead = useMarkRead();
  const link = item.link;
  return (
    <th scope="row" className={primaryCellClass}>
      <span className="flex min-w-0 items-center gap-3">
        <span aria-hidden="true" className="text-icon">
          <BellIcon />
        </span>
        <span className="flex min-w-0 flex-1 flex-col gap-0.5">
          {link === null ? (
            <span data-row-link="" tabIndex={-1} className="outline-none">
              {text}
            </span>
          ) : (
            <Link
              data-row-link=""
              to={linkAddress(link)}
              onClick={() => {
                if (item.read_at === null) {
                  markRead({ ids: [item.id] }).catch(reportError);
                }
              }}
              className={rowLinkClass}
            >
              {text}
            </Link>
          )}
          <span className={narrowOnlyClass}>
            <Label item={item} />
          </span>
        </span>
        <UnreadMarker item={item} />
      </span>
    </th>
  );
}

export const notificationRowSelector = `:scope > tr ${rowLinkSelector}`;

export function NotificationTable({
  items,
  label,
  bodyRef,
  textLoaders = notificationEventTextLoaders,
}: {
  readonly items: readonly Notification[];
  readonly label: string;
  readonly bodyRef: RefObject<HTMLTableSectionElement | null>;
  readonly textLoaders?: EventTextLoaders;
}): ReactElement {
  const { t } = useTranslation();
  const me = useSignedInAccount();
  const now = useNow(items);
  const texts = useEventTexts(textLoaders);
  const columns: readonly TableColumn[] = [
    { key: 'event', label: t('notifications.columns.event'), width: 'w-2/3 tablet:w-6/12' },
    { key: 'from', label: t('notifications.columns.from'), secondary: true, width: 'w-3/12' },
    { key: 'time', label: t('notifications.columns.time'), width: 'w-1/3 tablet:w-3/12' },
  ];
  return (
    <>
      <EventTextsFailed items={items} texts={texts} focusIfLost={focusHeadingIfLost} />
      <DataTable label={label} columns={columns} bodyRef={bodyRef}>
        {items.map((item) => (
          <tr key={item.id} data-row={item.id} className={tableRowClass}>
            <EventCell item={item} text={texts.textOf(item)} />
            <td className={secondaryCellClass}>
              <Label item={item} />
            </td>
            <td className={cellClass}>
              <time
                dateTime={item.created_at}
                title={formatDateTime(item.created_at, me.language, me.timezone)}
                className="tabular text-small text-text-muted"
              >
                {formatAgo(item.created_at, me.language, me.timezone, now)}
              </time>
            </td>
          </tr>
        ))}
      </DataTable>
    </>
  );
}

export function NotificationList({
  items,
  label,
  listRef,
  onChoose,
  focusIfLost,
  textLoaders = notificationEventTextLoaders,
}: {
  readonly items: readonly Notification[];
  readonly label: string;
  readonly listRef?: Ref<HTMLUListElement>;
  readonly onChoose: () => void;
  readonly focusIfLost: () => void;
  readonly textLoaders?: EventTextLoaders;
}): ReactElement {
  const now = useNow(items);
  const texts = useEventTexts(textLoaders);
  return (
    <>
      <EventTextsFailed items={items} texts={texts} focusIfLost={focusIfLost} />
      <ul ref={listRef} aria-label={label} className="border-b border-border">
        {items.map((item) => (
          <NotificationRow key={item.id} item={item} now={now} eventText={texts.textOf(item)} onChoose={onChoose} />
        ))}
      </ul>
    </>
  );
}

export function MarkAllRead({ newest }: { readonly newest: Notification }): ReactElement {
  const { t } = useTranslation();
  const markRead = useMarkRead();
  const action = useAction();
  return (
    <div className="flex flex-col items-start gap-2">
      <ActionButton
        busy={action.busy}
        onClick={() => {
          action.run(async () => {
            await markRead({ through: newest.id });
            return done;
          });
        }}
        className={quietButtonClass}
      >
        {t('notifications.markAllRead')}
      </ActionButton>
      <LiveNotice message={action.state.kind === 'done' ? t('notifications.allMarkedRead') : null} />
      <Refusal state={action.state} />
    </div>
  );
}
