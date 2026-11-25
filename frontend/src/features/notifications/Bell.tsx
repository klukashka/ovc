import { type UseQueryResult, useQuery } from '@tanstack/react-query';
import {
  type ComponentType,
  type ReactElement,
  type RefObject,
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation } from 'react-router';

import { useNotificationArrivals } from '../../app/notificationArrivals.ts';
import { BellIcon } from '../../components/ui/icons.tsx';
import { ModuleLoadFailed } from '../../components/ui/LoadState.tsx';
import { useApiClient } from '../../lib/api/client.ts';
import { type NotificationPage, fetchNotifications, latestNotificationsKey, panelRows, unreadCountCap } from './api.ts';

export function useLatestNotifications(enabled: boolean): UseQueryResult<NotificationPage> {
  const client = useApiClient();
  return useQuery({
    queryKey: latestNotificationsKey,
    queryFn: () => fetchNotifications(client, panelRows, null),
    enabled,
    retry: false,
  });
}

export function useUnreadCount(enabled: boolean): number {
  return useLatestNotifications(enabled).data?.unread_count ?? 0;
}

export function unreadCountText(count: number): string {
  return count > unreadCountCap ? `${String(unreadCountCap)}+` : String(count);
}

function useBellName(count: number): string {
  const { t } = useTranslation();
  return count > unreadCountCap ? t('frame.notificationsOverCap') : t('frame.notificationsWithCount', { count });
}

export interface PanelProps {
  readonly id: string;
  readonly className: string;
  readonly button: RefObject<HTMLButtonElement | null>;
  readonly onClose: () => void;
}

type Panel = ComponentType<PanelProps>;

let panelLoading: Promise<Panel | null> | null = null;

function loadPanel(): Promise<Panel | null> {
  panelLoading ??= import('./NotificationsPanel.tsx').then(
    (module) => module.NotificationsPanel,
    (error: unknown) => {
      console.error('panel_load_failed', error instanceof Error ? error.name : typeof error);
      return null;
    },
  );
  return panelLoading;
}

export type BellPlacement = 'sidebar' | 'overlay' | 'topBar' | 'phoneTopBar';

const panelClass =
  'fixed z-30 flex flex-col gap-3 overflow-y-auto rounded-lg border border-border bg-surface p-3 text-text shadow-raised dark:border-border-strong dark:shadow-none';

interface PlacementClasses {
  readonly button: string;
  readonly icon: string;
  readonly count: string;
  readonly panel: string;
}

const boundedPanel = 'max-h-[calc(100dvh-1.5rem)] max-w-[calc(100vw-1rem)]';

const topBarButton =
  'relative inline-flex target items-center justify-center gap-1 rounded-sm px-2 text-icon transition-colors duration-(--duration-press) ease-quiet hover:bg-hover hover:text-text';

const sidebarPlacement: PlacementClasses = {
  button:
    'flex min-h-11 w-full items-center gap-2.5 rounded-sm px-2.5 text-left text-small text-text-muted transition-colors duration-(--duration-press) ease-quiet hover:bg-hover hover:text-text aria-expanded:bg-hover aria-expanded:text-text desktop:pointer-fine:min-h-8',
  icon: 'text-icon',
  count: 'tabular text-label font-medium text-text-muted',
  panel: `${panelClass} ${boundedPanel} top-3 left-(--container-sidebar) ml-2 w-96`,
};

const placements: Readonly<Record<BellPlacement, PlacementClasses>> = {
  sidebar: sidebarPlacement,
  overlay: sidebarPlacement,
  topBar: {
    button: topBarButton,
    icon: 'inline-flex',
    count: 'tabular text-label font-medium text-text',
    panel: `${panelClass} ${boundedPanel} top-14 right-2 left-2 tablet:left-auto tablet:w-96`,
  },
  phoneTopBar: {
    button: topBarButton,
    icon: 'inline-flex',
    count: 'tabular text-label font-medium text-text',
    panel: `${panelClass} top-12 right-0 left-0 max-h-[70dvh] rounded-none border-x-0`,
  },
};

function LazyPanel(props: PanelProps): ReactElement | null {
  const [loaded, setLoaded] = useState<{ readonly Loaded: Panel | null } | null>(null);

  useEffect(() => {
    let mounted = true;
    void loadPanel().then((Loaded) => {
      if (mounted) {
        setLoaded({ Loaded });
      }
    });
    return (): void => {
      mounted = false;
    };
  }, []);

  if (loaded === null) {
    return <div id={props.id} className={props.className} />;
  }
  return loaded.Loaded === null ? (
    <div id={props.id} className={props.className}>
      <ModuleLoadFailed />
    </div>
  ) : (
    <loaded.Loaded {...props} />
  );
}

function BellButtonContent({
  placement,
  count,
}: {
  readonly placement: BellPlacement;
  readonly count: number;
}): ReactElement {
  const { t } = useTranslation();
  const shown = count > 0 ? unreadCountText(count) : null;
  const classes = placements[placement];
  return (
    <>
      <span className={classes.icon}>
        <BellIcon />
      </span>
      {placement === 'sidebar' || placement === 'overlay' ? (
        <span className="flex-1">{t('section.notifications')}</span>
      ) : null}
      {shown === null ? null : (
        <span aria-hidden="true" className={classes.count}>
          {shown}
        </span>
      )}
    </>
  );
}

export function NotificationsBell({ placement }: { readonly placement: BellPlacement }): ReactElement {
  const count = useUnreadCount(true);
  const name = useBellName(count);
  const { pathname } = useLocation();
  const [open, setOpen] = useState(false);
  const [shownAt, setShownAt] = useState(pathname);
  if (shownAt !== pathname) {
    setShownAt(pathname);
    setOpen(false);
  }
  const panelId = useId();
  const button = useRef<HTMLButtonElement>(null);
  const close = useCallback(() => {
    setOpen(false);
  }, []);

  return (
    <div data-frame-bell="" className="relative flex">
      <button
        ref={button}
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        aria-label={name}
        onClick={() => {
          setOpen(!open);
          button.current?.focus();
        }}
        className={placements[placement].button}
      >
        <BellButtonContent placement={placement} count={count} />
      </button>
      {open ? <LazyPanel id={panelId} className={placements[placement].panel} button={button} onClose={close} /> : null}
    </div>
  );
}

export function NotificationAnnouncer(): ReactElement {
  const { t } = useTranslation();
  const arrivals = useNotificationArrivals();
  const [mountedAt] = useState(arrivals);
  const [announced, setAnnounced] = useState<number | null>(null);

  useEffect(() => {
    if (arrivals === mountedAt) {
      return undefined;
    }
    const frame = window.requestAnimationFrame(() => {
      setAnnounced(arrivals);
    });
    return (): void => {
      window.cancelAnimationFrame(frame);
    };
  }, [arrivals, mountedAt]);

  return (
    <p aria-live="polite" className="sr-only">
      {announced === null ? null : <span key={announced}>{t('notifications.arrived')}</span>}
    </p>
  );
}
