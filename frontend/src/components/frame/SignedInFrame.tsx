import type { ParseKeys } from 'i18next';
import { type ReactElement, type ReactNode, type RefObject, useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, Outlet, useLocation, useMatch, useNavigate } from 'react-router';

import { useUnreadChannels } from '../../features/messages/unread.ts';
import { provisionallyLinked, useProvisionalLink } from '../../routes/provisionalLink.ts';

import { NotificationAnnouncer, NotificationsBell, useUnreadCount } from '../../features/notifications/Bell.tsx';

import { phoneLayoutQuery, useMediaQuery, usePreferences, wideLayoutQuery } from '../../app/context.ts';
import { preferenceKeys } from '../../lib/preferences/store.ts';
import { useRouteTitle } from '../../app/handles.ts';
import { type SectionCounts, hasNews } from '../../app/sections.ts';
import { useHomeworkCount } from '../../app/homeworkCount.ts';
import { type Me, type SignOut, useSignOut } from '../../lib/api/session.ts';
import { reachable } from '../../routes/access.ts';
import { SidebarIcon } from '../ui/icons.tsx';
import { FrameDialog } from './FrameDialog.tsx';
import { OfflineBanner } from './OfflineBanner.tsx';
import { BottomBar, MoreSheet, PhoneTopBar } from './PhoneFrame.tsx';
import { Sidebar } from './Sidebar.tsx';
import { StudentConfirmation } from './StudentConfirmation.tsx';
import { SkipLink, mainContentId } from './SkipLink.tsx';

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
      data-frame-only=""
      onClick={onClick}
      className={iconButton}
    >
      <SidebarIcon />
      {news ? <NewsDot /> : null}
    </button>
  );
}

function NarrowTopBar({
  title,
  showButton,
  news,
  overlayOpen,
  onShow,
  withBell,
}: {
  readonly title: ParseKeys | undefined;
  readonly showButton: RefObject<HTMLButtonElement | null>;
  readonly news: boolean;
  readonly overlayOpen: boolean;
  readonly onShow: () => void;
  readonly withBell: boolean;
}): ReactElement {
  const { t } = useTranslation();
  const routeTitle = useRouteTitle();
  return (
    <header className="sticky top-0 z-10 flex items-center gap-2 border-b border-border bg-bg px-2 py-1">
      <ShowSidebarButton buttonRef={showButton} news={news} expanded={overlayOpen} onClick={onShow} />
      <p className="flex-1 truncate text-strong">{t(title ?? routeTitle)}</p>
      {withBell ? <NotificationsBell placement="topBar" /> : null}
    </header>
  );
}

function ConfirmationBanner({ me }: { readonly me: Me }): ReactElement {
  const { t } = useTranslation();
  return (
    <section aria-label={t('frame.confirmationLabel')} className="border-b border-border bg-surface">
      <div className="mx-auto flex w-full max-w-page flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 text-small tablet:px-8 desktop:px-14">
        {me.role === 'student' ? (
          <StudentConfirmation email={me.email} />
        ) : (
          <>
            <p>{t('frame.confirmationBody', { email: me.email })}</p>
            <Link to="/verify-email" className="inline-flex min-h-11 items-center text-accent-text underline">
              {t('frame.confirmationAction')}
            </Link>
          </>
        )}
      </div>
    </section>
  );
}

type FocusedPart = 'frameOnly' | 'bell' | 'page';

function focusedPart(target: EventTarget | null): FocusedPart {
  if (!(target instanceof Element)) {
    return 'page';
  }
  if (target.closest('[data-frame-only]') !== null) {
    return 'frameOnly';
  }
  if (target.closest('[data-frame-bell] > button') !== null) {
    return 'bell';
  }
  return target.closest('[data-frame-bell]') === null ? 'page' : 'frameOnly';
}

function useFrameSwitchFocus(phone: boolean): void {
  const lastFocused = useRef<FocusedPart>('page');
  const shownPhone = useRef(phone);

  useEffect(() => {
    function note(event: FocusEvent): void {
      lastFocused.current = focusedPart(event.target);
    }
    document.addEventListener('focusin', note);
    return (): void => {
      document.removeEventListener('focusin', note);
    };
  }, []);

  useEffect(() => {
    if (shownPhone.current === phone) {
      return;
    }
    shownPhone.current = phone;
    const active = document.activeElement;
    if (active !== null && active !== document.body && active.isConnected) {
      return;
    }
    if (lastFocused.current === 'page') {
      return;
    }
    const bell =
      lastFocused.current === 'bell' ? document.querySelector<HTMLElement>('[data-frame-bell] > button') : null;
    (bell ?? document.querySelector<HTMLElement>('main h1'))?.focus();
  }, [phone]);
}

function mainPadding(phone: boolean, phoneBars: boolean): string {
  if (!phone) {
    return 'py-8 desktop:py-12';
  }
  return phoneBars ? 'pt-8 pb-[calc(--spacing(8)+var(--bottom-bar-height,56px))]' : 'py-2';
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
  const navigate = useNavigate();
  const wide = useMediaQuery(wideLayoutQuery);
  const phone = useMediaQuery(phoneLayoutQuery);
  const conversationScreen = useMatch('/messages/:channelId') !== null;
  const phoneBars = phone && !conversationScreen;
  const [sidebarShown, setSidebarShown] = useState(() => preferences.read(preferenceKeys.sidebar) !== 'hidden');
  const [overlayOpen, setOverlayOpen] = useState(false);
  if ((wide || phone) && overlayOpen) {
    setOverlayOpen(false);
  }
  const { pathname } = useLocation();
  const [overlayPath, setOverlayPath] = useState(pathname);
  if (overlayPath !== pathname) {
    setOverlayPath(pathname);
    setOverlayOpen(false);
  }
  const [sheetOpen, setSheetOpen] = useState(false);
  if (!phoneBars && sheetOpen) {
    setSheetOpen(false);
  }
  const sheetShown = useRef(sheetOpen);
  const moreButton = useRef<HTMLButtonElement>(null);
  const [signOutFailedAbovePage, setSignOutFailedAbovePage] = useState(false);
  const signing = useSignOut(
    () => {
      void navigate('/', { replace: true });
    },
    () => {
      if (!sheetShown.current) {
        setSignOutFailedAbovePage(true);
        moreButton.current?.focus();
      }
    },
  );
  const sheetSigning: SignOut = {
    state: signing.state,
    signOut: () => {
      setSignOutFailedAbovePage(false);
      signing.signOut();
    },
  };
  useEffect(() => {
    sheetShown.current = sheetOpen;
  }, [sheetOpen]);
  useFrameSwitchFocus(phone);
  const showButton = useRef<HTMLButtonElement>(null);
  const hideButtonRef = useRef<HTMLButtonElement>(null);
  const withBell = reachable(me, '/notifications');
  const unread = useUnreadCount(withBell);
  const linked = provisionallyLinked(useProvisionalLink(me));
  const chatUnread = useUnreadChannels(reachable(me, '/messages', linked));
  const homework = useHomeworkCount(
    me.role,
    me.role === 'tutor' ? me.email_verified : reachable(me, '/homework', linked),
  );
  const counts: SectionCounts = {
    notifications: withBell ? unread : 0,
    messages: chatUnread,
    homework,
  };
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

  const closeSheet = useCallback(() => {
    setSheetOpen(false);
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
      bell={
        !withBell ? null : wide ? (
          <NotificationsBell placement="sidebar" />
        ) : (
          <NotificationsBell key={overlayOpen ? 'overlayOpen' : 'overlayClosed'} placement="overlay" />
        )
      }
      headerAction={hideButton}
      onNavigate={() => {
        setOverlayOpen(false);
      }}
    />
  );

  return (
    <div className="min-h-dvh bg-bg text-text">
      <SkipLink />
      {withBell ? <NotificationAnnouncer /> : null}
      {wide && sidebarShown ? (
        <aside data-frame-only="" className="fixed inset-y-0 left-0 z-20 w-sidebar border-r border-border">
          {sidebar}
        </aside>
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
      {wide || phone ? null : (
        <>
          <NarrowTopBar
            title={title}
            showButton={showButton}
            news={news}
            overlayOpen={overlayOpen}
            onShow={() => {
              setOverlayOpen(true);
            }}
            withBell={withBell}
          />
          <FrameDialog
            open={overlayOpen}
            onClose={closeOverlay}
            returnFocus={showButton}
            label={t('frame.sidebar')}
            className="m-0 h-dvh max-h-none w-sidebar max-w-overlay-cap bg-transparent p-0 text-text"
          >
            {sidebar}
          </FrameDialog>
        </>
      )}
      {phoneBars ? <PhoneTopBar me={me} withBell={withBell} /> : null}
      <div className={wide && sidebarShown ? 'pl-(--container-sidebar)' : ''}>
        <OfflineBanner />
        {me.state === 'unverified' ? <ConfirmationBanner me={me} /> : null}
        {signOutFailedAbovePage && signing.state === 'failed' ? (
          <p role="alert" className="border-b border-border px-4 py-3 text-small text-danger">
            {t('frame.signOutFailed')}
          </p>
        ) : null}
        <main
          id={mainContentId}
          tabIndex={-1}
          className={`mx-auto w-full max-w-page px-4 outline-none tablet:px-8 desktop:px-14 ${mainPadding(phone, phoneBars)}`}
        >
          {children ?? <Outlet />}
        </main>
      </div>
      {phoneBars ? (
        <>
          <BottomBar
            me={me}
            linked={linked}
            counts={counts}
            sheetOpen={sheetOpen}
            moreButton={moreButton}
            onMore={() => {
              setSignOutFailedAbovePage(false);
              setSheetOpen(true);
            }}
          />
          <MoreSheet
            me={me}
            linked={linked}
            counts={counts}
            open={sheetOpen}
            onClose={closeSheet}
            moreButton={moreButton}
            signing={sheetSigning}
          />
        </>
      ) : null}
    </div>
  );
}
