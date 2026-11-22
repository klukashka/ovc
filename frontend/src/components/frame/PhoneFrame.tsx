import { type ReactElement, type ReactNode, type RefObject, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation } from 'react-router';

import { NotificationsBell } from '../../features/notifications/Bell.tsx';
import {
  type Section,
  type SectionCounts,
  barSections,
  isCurrent,
  sheetSections,
  trailingSections,
} from '../../app/sections.ts';
import type { Me, SignOut } from '../../lib/api/session.ts';
import { reachable } from '../../routes/access.ts';
import { ActionButton } from '../ui/Form.tsx';
import { CrossIcon, DoorIcon, MoreIcon } from '../ui/icons.tsx';
import { FrameDialog } from './FrameDialog.tsx';
import { homeworkCountName } from './Sidebar.tsx';

const homePages: Readonly<Record<Me['role'], string>> = { tutor: '/today', student: '/home' };

function capped(count: number): string {
  return count > 99 ? '99+' : String(count);
}

export function PhoneTopBar({ me, withBell }: { readonly me: Me; readonly withBell: boolean }): ReactElement {
  const { t } = useTranslation();
  return (
    <header className="flex h-12 items-center justify-between border-b border-border bg-bg px-2">
      <Link
        to={homePages[me.role]}
        data-frame-only=""
        className="inline-flex min-h-11 min-w-11 items-center px-2 font-serif text-heading"
      >
        {t('app.name')}
      </Link>
      {withBell ? <NotificationsBell placement="phoneTopBar" /> : null}
    </header>
  );
}

function barTarget(current: boolean): string {
  return `flex min-h-14 w-full min-w-11 flex-col items-center justify-center gap-0.5 px-0.5 py-1 text-center text-caption break-words transition-colors duration-(--duration-press) ease-quiet ${current ? 'text-text' : 'text-text-muted'}`;
}

function BarIcon({
  current,
  count,
  children,
}: {
  readonly current: boolean;
  readonly count: number;
  readonly children: ReactNode;
}): ReactElement {
  return (
    <span
      className={`relative flex h-6 w-8 items-center justify-center rounded-full ${current ? 'bg-hover text-text' : 'text-icon'}`}
    >
      {children}
      {count > 0 ? (
        <span
          aria-hidden="true"
          data-bar-count=""
          className="absolute -top-1.5 left-5 min-w-4 rounded-full border border-border-strong bg-surface px-1 text-caption tabular text-text"
        >
          {capped(count)}
        </span>
      ) : null}
    </span>
  );
}

function BarItem({
  section,
  count,
  current,
}: {
  readonly section: Section;
  readonly count: number;
  readonly current: boolean;
}): ReactElement {
  const { t } = useTranslation();
  const { Icon } = section;
  const chats = section.key === 'messages';
  const countName =
    count <= 0
      ? undefined
      : chats
        ? count > 99
          ? t('frame.chatsOverCap')
          : t('frame.chatsWithCount', { count })
        : section.key === 'homework'
          ? homeworkCountName(t, section, count)
          : undefined;
  return (
    <li className="flex w-1/5">
      <Link
        to={section.to}
        aria-current={current ? 'page' : undefined}
        aria-label={countName}
        className={barTarget(current)}
      >
        <BarIcon current={current} count={count}>
          <Icon />
        </BarIcon>
        <span className={current ? 'font-semibold' : undefined}>
          {chats ? t('section.chats') : t(`section.${section.key}`)}
        </span>
      </Link>
    </li>
  );
}

function sheetRows(me: Me, linked: boolean): readonly Section[] {
  return [...sheetSections[me.role], ...trailingSections].filter((section) => reachable(me, section.to, linked));
}

function overlapWithBar(focused: Element, bar: HTMLElement): number {
  const target = focused.getBoundingClientRect();
  const barTop = bar.getBoundingClientRect().top;
  return Math.min(target.bottom - barTop, target.top);
}

function insideFixedLayer(focused: Element): boolean {
  for (let element: Element | null = focused; element !== null; element = element.parentElement) {
    if (getComputedStyle(element).position === 'fixed') {
      return true;
    }
  }
  return false;
}

function useFocusAboveBar(bar: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    let frame = 0;
    function reveal(event: FocusEvent): void {
      const focused = event.target;
      const element = bar.current;
      if (
        !(focused instanceof Element) ||
        element === null ||
        element.contains(focused) ||
        focused.closest('dialog') !== null
      ) {
        return;
      }
      window.cancelAnimationFrame(frame);
      frame = window.requestAnimationFrame(() => {
        const overlap = overlapWithBar(focused, element);
        if (document.activeElement === focused && overlap > 0 && !insideFixedLayer(focused)) {
          window.scrollBy({ top: overlap });
        }
      });
    }
    document.addEventListener('focusin', reveal);
    return (): void => {
      document.removeEventListener('focusin', reveal);
      window.cancelAnimationFrame(frame);
    };
  }, [bar]);
}

export function BottomBar({
  me,
  linked,
  counts,
  sheetOpen,
  moreButton,
  onMore,
}: {
  readonly me: Me;
  readonly linked: boolean;
  readonly counts: SectionCounts;
  readonly sheetOpen: boolean;
  readonly moreButton: RefObject<HTMLButtonElement | null>;
  readonly onMore: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const bar = useRef<HTMLElement>(null);
  const moreCurrent = sheetRows(me, linked).some((section) => isCurrent(section, pathname));
  useFocusAboveBar(bar);

  useEffect(() => {
    const element = bar.current;
    const root = document.documentElement;
    if (element === null) {
      return undefined;
    }
    const measured: HTMLElement = element;
    function follow(): void {
      root.style.setProperty('--bottom-bar-height', `${String(measured.offsetHeight)}px`);
    }
    follow();
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(follow);
    observer?.observe(measured);
    return (): void => {
      observer?.disconnect();
      root.style.removeProperty('--bottom-bar-height');
    };
  }, []);

  return (
    <nav
      ref={bar}
      aria-label={t('frame.navigation')}
      data-frame-only=""
      className="fixed inset-x-0 bottom-0 z-20 border-t border-border bg-sidebar pb-[env(safe-area-inset-bottom)]"
    >
      <ul className="flex justify-center">
        {barSections[me.role]
          .filter((section) => reachable(me, section.to, linked))
          .map((section) => (
            <BarItem
              key={section.key}
              section={section}
              count={counts[section.key] ?? 0}
              current={isCurrent(section, pathname)}
            />
          ))}
        <li className="flex w-1/5">
          <button
            ref={moreButton}
            type="button"
            aria-haspopup="dialog"
            aria-expanded={sheetOpen}
            onClick={onMore}
            className={barTarget(moreCurrent)}
          >
            <BarIcon current={moreCurrent} count={0}>
              <MoreIcon />
            </BarIcon>
            <span className={moreCurrent ? 'font-semibold' : undefined}>{t('frame.more')}</span>
          </button>
        </li>
      </ul>
    </nav>
  );
}

const sheetRow =
  'flex min-h-12 w-full items-center gap-3 border-b border-border px-4 text-left text-body-phone hover:bg-hover';

export function MoreSheet({
  me,
  linked,
  counts,
  open,
  onClose,
  moreButton,
  signing,
}: {
  readonly me: Me;
  readonly linked: boolean;
  readonly counts: SectionCounts;
  readonly open: boolean;
  readonly onClose: () => void;
  readonly moreButton: RefObject<HTMLButtonElement | null>;
  readonly signing: SignOut;
}): ReactElement {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const initial = Array.from(me.name.trim()).at(0) ?? '';
  return (
    <FrameDialog
      open={open}
      onClose={onClose}
      returnFocus={moreButton}
      label={t('frame.more')}
      className="m-0 mt-auto max-h-[80dvh] w-full max-w-none overflow-y-auto rounded-t-lg bg-surface p-0 text-text shadow-raised dark:border dark:border-border-strong dark:shadow-none"
    >
      <div className="flex flex-col pb-[env(safe-area-inset-bottom)]">
        <div className="flex items-center gap-3 border-b border-border px-4 py-3">
          <span
            aria-hidden="true"
            className="flex size-8 shrink-0 items-center justify-center rounded-full bg-hover text-caption font-semibold text-text"
          >
            {initial}
          </span>
          <p className="min-w-0 break-words">
            <span className="block text-strong">{me.name}</span>
            <span className="block text-small text-text-muted">{t(`role.${me.role}`)}</span>
          </p>
        </div>
        <ul>
          {sheetRows(me, linked).map((section) => {
            const current = isCurrent(section, pathname);
            const count = section.key === 'notifications' ? (counts.notifications ?? 0) : 0;
            const { Icon } = section;
            return (
              <li key={section.key}>
                <Link
                  to={section.to}
                  onClick={onClose}
                  aria-current={current ? 'page' : undefined}
                  aria-label={
                    count > 0
                      ? count > 99
                        ? t('frame.notificationsOverCap')
                        : t('frame.notificationsWithCount', { count })
                      : undefined
                  }
                  className={`${sheetRow} ${current ? 'bg-hover font-medium' : ''}`}
                >
                  <span className="text-icon">
                    <Icon />
                  </span>
                  <span className="flex-1">{t(`section.${section.key}`)}</span>
                  {count > 0 ? (
                    <span aria-hidden="true" className="tabular text-caption text-text-muted">
                      {capped(count)}
                    </span>
                  ) : null}
                </Link>
              </li>
            );
          })}
        </ul>
        <ActionButton busy={signing.state === 'busy'} onClick={signing.signOut} className={sheetRow}>
          <span className="text-icon">
            <DoorIcon />
          </span>
          {signing.state === 'busy' ? t('frame.signingOut') : t('frame.signOut')}
        </ActionButton>
        {signing.state === 'failed' ? (
          <p role="alert" className="px-4 py-2 text-small text-danger">
            {t('frame.signOutFailed')}
          </p>
        ) : null}
        <button type="button" onClick={onClose} className={sheetRow}>
          <span className="text-icon">
            <CrossIcon />
          </span>
          {t('frame.moreClose')}
        </button>
      </div>
    </FrameDialog>
  );
}
