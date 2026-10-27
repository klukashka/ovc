import { useQueryClient } from '@tanstack/react-query';
import { type ReactElement, type Ref, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

import { ActionButton, quietButtonClass } from '../../components/ui/Form.tsx';
import { BellIcon } from '../../components/ui/icons.tsx';
import { LiveNotice } from '../../components/ui/LiveNotice.tsx';
import { paletteDotClass } from '../../components/ui/palette.ts';
import { Refusal } from '../../components/ui/Refusal.tsx';
import { done, useAction } from '../../components/ui/action.ts';
import { useApiClient } from '../../lib/api/client.ts';
import { useSignedInAccount } from '../../lib/api/session.ts';
import { formatAgo, formatDateTime } from '../../lib/format/dates.ts';
import {
  type Notification,
  type NotificationEvent,
  type ReadSelection,
  linkAddress,
  markNotificationsRead,
  notificationsKey,
} from './api.ts';

const eventTexts = {
  'invitation.accepted': 'notifications.event.invitationAccepted',
  'join_request.created': 'notifications.event.joinRequestCreated',
  'join_request.accepted': 'notifications.event.joinRequestAccepted',
  'join_request.declined': 'notifications.event.joinRequestDeclined',
  'relationship.ended': 'notifications.event.relationshipEnded',
  'lesson.reminder_1h': 'notifications.event.lessonReminder1h',
} as const satisfies Readonly<Record<NotificationEvent, string>>;

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
  if (item.label === null) {
    return null;
  }
  return (
    <span className="flex items-center gap-1.5 text-strong">
      {item.colour === null ? null : (
        <span aria-hidden="true" className={`size-2 shrink-0 rounded-full ${paletteDotClass[item.colour]}`} />
      )}
      <span className="min-w-0 break-words">{item.label}</span>
    </span>
  );
}

function RowContent({ item, now }: { readonly item: Notification; readonly now: number }): ReactElement {
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
        <span>{t(eventTexts[item.event])}</span>
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
  onChoose,
}: {
  readonly item: Notification;
  readonly now: number;
  readonly onChoose: () => void;
}): ReactElement {
  const markRead = useMarkRead();
  const link = item.link;
  if (link === null) {
    return (
      <li data-row={item.id} tabIndex={-1} className={`${rowClass} border-t border-border`}>
        <RowContent item={item} now={now} />
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
        <RowContent item={item} now={now} />
      </Link>
    </li>
  );
}

export function NotificationList({
  items,
  label,
  listRef,
  onChoose,
}: {
  readonly items: readonly Notification[];
  readonly label: string;
  readonly listRef?: Ref<HTMLUListElement>;
  readonly onChoose: () => void;
}): ReactElement {
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
  return (
    <ul ref={listRef} aria-label={label} className="border-b border-border">
      {items.map((item) => (
        <NotificationRow key={item.id} item={item} now={now} onChoose={onChoose} />
      ))}
    </ul>
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
