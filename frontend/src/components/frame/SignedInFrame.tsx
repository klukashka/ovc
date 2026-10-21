import type { ParseKeys } from 'i18next';
import { type ReactElement, type ReactNode, type RefObject, useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, Outlet } from 'react-router';

import { useMediaQuery, usePreferences, wideLayoutQuery } from '../../app/context.ts';
import { preferenceKeys } from '../../lib/preferences/store.ts';
import { useRouteTitle } from '../../app/handles.ts';
import { type SectionCounts, hasNews } from '../../app/sections.ts';
import type { Me } from '../../lib/api/session.ts';
import { BellIcon, SidebarIcon } from '../ui/icons.tsx';
import { Sidebar } from './Sidebar.tsx';

const iconButton =
  'relative inline-flex target items-center justify-center rounded-sm text-icon transition-colors duration-(--duration-press) ease-quiet hover:bg-hover hover:text-text';

function NewsDot(): ReactElement {
  return <span aria-hidden="true" className="absolute top-1 right-1 size-news-dot rounded-full bg-accent-text" />;
}

function ShowSidebarButton({
  buttonRef,
  news,
  expanded,
  onClick,
}: {
  readonly buttonRef: RefObject<HTMLButtonElement | null>;
  readonly news: boolean;
  readonly expanded: boolean;
  readonly onClick: () => void;
}): ReactElement {
  const { t } = useTranslation();
  return (
    <button
      ref={buttonRef}
      type="button"
      aria-label={news ? t('frame.showSidebarWithNews') : t('frame.showSidebar')}
      aria-expanded={expanded}
      onClick={onClick}
      className={iconButton}
    >
      <SidebarIcon />
      {news ? <NewsDot /> : null}
    </button>
  );
}

function focusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(
    container.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])'),
  );
}

function SidebarOverlay({
  open,
  onClose,
  returnFocus,
  label,
  children,
}: {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly returnFocus: RefObject<HTMLButtonElement | null>;
  readonly label: string;
  readonly children: ReactElement;
}): ReactElement {
  const dialog = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const element = dialog.current;
    if (element === null) {
      return undefined;
    }
    const opened: HTMLDialogElement = element;
    function cancel(event: Event): void {
      event.preventDefault();
      if (opened.querySelector('[aria-expanded="true"]') !== null) {
        return;
      }
      onClose();
    }
    function closeOnScrim(event: MouseEvent): void {
      if (event.target === element) {
        onClose();
      }
    }
    function keepFocusInside(event: KeyboardEvent): void {
      if (event.key !== 'Tab' || element === null) {
        return;
      }
      const focusable = focusableElements(element);
      const first = focusable[0];
      const last = focusable.at(-1);
      if (first === undefined || last === undefined) {
        return;
      }
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }
    function syncClosed(): void {
      onClose();
      const active = document.activeElement;
      if (active === null || active === document.body || opened.contains(active)) {
        returnFocus.current?.focus();
      }
    }
    element.addEventListener('close', syncClosed);
    element.addEventListener('cancel', cancel);
    element.addEventListener('click', closeOnScrim);
    element.addEventListener('keydown', keepFocusInside);
    return (): void => {
      element.removeEventListener('close', syncClosed);
      element.removeEventListener('cancel', cancel);
      element.removeEventListener('click', closeOnScrim);
      element.removeEventListener('keydown', keepFocusInside);
    };
  }, [onClose, returnFocus]);

  useEffect(() => {
    const element = dialog.current;
    if (element === null) {
      return;
    }
    if (open && !element.open) {
      element.showModal();
      const current = element.querySelector<HTMLElement>('[aria-current="page"]') ?? focusableElements(element)[0];
      current?.focus();
    }
    if (!open && element.open) {
      element.close();
      returnFocus.current?.focus();
    }
  }, [open, returnFocus]);

  return (
    <dialog
      ref={dialog}
      aria-label={label}
      className="m-0 h-dvh max-h-none w-sidebar max-w-overlay-cap bg-transparent p-0 text-text"
    >
      {children}
    </dialog>
  );
}

function NarrowTopBar({
  title,
  showButton,
  news,
  overlayOpen,
  onShow,
  notifications,
}: {
  readonly title: ParseKeys | undefined;
  readonly showButton: RefObject<HTMLButtonElement | null>;
  readonly news: boolean;
  readonly overlayOpen: boolean;
  readonly onShow: () => void;
  readonly notifications: number;
}): ReactElement {
  const { t } = useTranslation();
  const routeTitle = useRouteTitle();
  return (
    <header className="sticky top-0 z-10 flex items-center gap-2 border-b border-border bg-bg px-2 py-1">
      <ShowSidebarButton buttonRef={showButton} news={news} expanded={overlayOpen} onClick={onShow} />
      <p className="flex-1 truncate text-strong">{t(title ?? routeTitle)}</p>
      <Link
        to="/notifications"
        aria-label={t('frame.notificationsWithCount', { count: notifications })}
        className={`${iconButton} gap-1 px-2`}
      >
        <BellIcon />
        {notifications > 0 ? (
          <span aria-hidden="true" className="tabular text-caption text-text">
            {notifications}
          </span>
        ) : null}
      </Link>
    </header>
  );
}

function ConfirmationBanner({ email }: { readonly email: string }): ReactElement {
  const { t } = useTranslation();
  return (
    <div role="region" aria-label={t('frame.confirmationLabel')} className="border-b border-border bg-surface">
      <p className="mx-auto flex w-full max-w-page flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 text-small tablet:px-8 desktop:px-14">
        <span>{t('frame.confirmationBody', { email })}</span>
        <Link to="/verify-email" className="inline-flex min-h-11 items-center text-accent-text underline">
          {t('frame.confirmationAction')}
        </Link>
      </p>
    </div>
  );
}

export function SignedInFrame({
  me,
  title,
  children,
}: {
  readonly me: Me;
  readonly title?: ParseKeys;
  readonly children?: ReactNode;
}): ReactElement {
  const { t } = useTranslation();
  const preferences = usePreferences();
  const wide = useMediaQuery(wideLayoutQuery);
  const [sidebarShown, setSidebarShown] = useState(() => preferences.read(preferenceKeys.sidebar) !== 'hidden');
  const [overlayOpen, setOverlayOpen] = useState(false);
  if (wide && overlayOpen) {
    setOverlayOpen(false);
  }
  const showButton = useRef<HTMLButtonElement>(null);
  const hideButtonRef = useRef<HTMLButtonElement>(null);
  const counts: SectionCounts = {};
  const news = hasNews(counts);

  const rememberSidebar = useCallback(
    (shown: boolean) => {
      setSidebarShown(shown);
      preferences.write(preferenceKeys.sidebar, shown ? 'shown' : 'hidden');
    },
    [preferences],
  );

  const closeOverlay = useCallback(() => {
    setOverlayOpen(false);
  }, []);

  const hideButton = wide ? (
    <button
      ref={hideButtonRef}
      type="button"
      aria-label={t('frame.hideSidebar')}
      onClick={() => {
        rememberSidebar(false);
        requestAnimationFrame(() => showButton.current?.focus());
      }}
      className={iconButton}
    >
      <SidebarIcon />
    </button>
  ) : null;

  const sidebar = (
    <Sidebar
      me={me}
      counts={counts}
      headerAction={hideButton}
      onNavigate={() => {
        setOverlayOpen(false);
      }}
    />
  );

  return (
    <div className="min-h-dvh bg-bg text-text">
      {wide && sidebarShown ? (
        <aside className="fixed inset-y-0 left-0 w-sidebar border-r border-border">{sidebar}</aside>
      ) : null}
      {wide && !sidebarShown ? (
        <div className="fixed top-3 left-3 z-10">
          <ShowSidebarButton
            buttonRef={showButton}
            news={news}
            expanded={false}
            onClick={() => {
              rememberSidebar(true);
              requestAnimationFrame(() => hideButtonRef.current?.focus());
            }}
          />
        </div>
      ) : null}
      {wide ? null : (
        <>
          <NarrowTopBar
            title={title}
            showButton={showButton}
            news={news}
            overlayOpen={overlayOpen}
            onShow={() => {
              setOverlayOpen(true);
            }}
            notifications={counts.notifications ?? 0}
          />
          <SidebarOverlay open={overlayOpen} onClose={closeOverlay} returnFocus={showButton} label={t('frame.sidebar')}>
            {sidebar}
          </SidebarOverlay>
        </>
      )}
      <div className={wide && sidebarShown ? 'pl-(--container-sidebar)' : ''}>
        {me.state === 'unverified' ? <ConfirmationBanner email={me.email} /> : null}
        <main className="mx-auto w-full max-w-page px-4 py-8 tablet:px-8 desktop:px-14 desktop:py-12">
          {children ?? <Outlet />}
        </main>
      </div>
    </div>
  );
}
