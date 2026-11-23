import type { TFunction } from 'i18next';
import { type ReactElement, type ReactNode, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation, useNavigate } from 'react-router';

import { type Section, type SectionCounts, isCurrent, roleSections, trailingSections } from '../../app/sections.ts';
import { type Me, useSignOut } from '../../lib/api/session.ts';
import { reachable } from '../../routes/access.ts';
import { provisionallyLinked, useProvisionalLink } from '../../routes/provisionalLink.ts';
import { ActionButton } from '../ui/Form.tsx';

interface SidebarProps {
  readonly me: Me;
  readonly counts: SectionCounts;
  readonly bell: ReactNode;
  readonly headerAction: ReactNode;
  readonly onNavigate: () => void;
}

export function homeworkCountName(t: TFunction, section: Section, count: number): string {
  const toCheck = section.to !== '/homework';
  return count > 99
    ? t(toCheck ? 'frame.homeworkToCheckOverCap' : 'frame.homeworkToDoOverCap')
    : t(toCheck ? 'frame.homeworkToCheck' : 'frame.homeworkToDo', { count });
}

function SidebarItem({
  section,
  count,
  current,
  onNavigate,
}: {
  readonly section: Section;
  readonly count: number;
  readonly current: boolean;
  readonly onNavigate: () => void;
}): ReactElement {
  const { t } = useTranslation();
  const { Icon } = section;
  return (
    <li>
      <Link
        to={section.to}
        onClick={onNavigate}
        aria-current={current ? 'page' : undefined}
        aria-label={
          count <= 0
            ? undefined
            : section.key === 'messages'
              ? count > 99
                ? t('frame.messagesOverCap')
                : t('frame.messagesWithCount', { count })
              : section.key === 'homework'
                ? homeworkCountName(t, section, count)
                : undefined
        }
        data-sidebar-item=""
        className={`flex min-h-11 items-center gap-2.5 rounded-sm px-2.5 text-small transition-colors duration-(--duration-press) ease-quiet hover:bg-hover hover:text-text desktop:pointer-fine:min-h-8 ${current ? 'bg-hover font-medium text-text' : 'text-text-muted'}`}
      >
        <span className="text-icon">
          <Icon />
        </span>
        <span className="flex-1">{t(`section.${section.key}`)}</span>
        {count > 0 ? (
          <span className="tabular text-label font-medium text-text-muted">{count > 99 ? '99+' : count}</span>
        ) : null}
      </Link>
    </li>
  );
}

function AccountMenu({ me }: { readonly me: Me }): ReactElement {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const signing = useSignOut(() => {
    void navigate('/', { replace: true });
  });
  const menuId = useId();
  const container = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const initial = Array.from(me.name.trim()).at(0) ?? '';

  useEffect(() => {
    if (!open) {
      return undefined;
    }
    function closeOnOutsidePointer(event: PointerEvent): void {
      if (event.target instanceof Node && container.current?.contains(event.target) !== true) {
        setOpen(false);
      }
    }
    function closeOnEscape(event: KeyboardEvent): void {
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        setOpen(false);
        button.current?.focus();
      }
    }
    let pointerInside = false;
    function notePointerInside(): void {
      pointerInside = true;
    }
    function notePointerReleased(): void {
      pointerInside = false;
    }
    function closeOnFocusLeaving(event: FocusEvent): void {
      const target = event.relatedTarget;
      const leftForElsewhere = target instanceof Node && container.current?.contains(target) !== true;
      const leftForNothing = !(target instanceof Node) && !pointerInside;
      if (leftForElsewhere || leftForNothing) {
        setOpen(false);
      }
    }
    function closeOnDialogCancel(): void {
      setOpen(false);
      button.current?.focus();
    }
    document.addEventListener('pointerdown', closeOnOutsidePointer);
    container.current?.addEventListener('keydown', closeOnEscape);
    container.current?.addEventListener('focusout', closeOnFocusLeaving);
    container.current?.addEventListener('pointerdown', notePointerInside);
    document.addEventListener('pointerup', notePointerReleased);
    const element = container.current;
    const surroundingDialog = element?.closest('dialog') ?? null;
    surroundingDialog?.addEventListener('cancel', closeOnDialogCancel);
    return (): void => {
      document.removeEventListener('pointerdown', closeOnOutsidePointer);
      element?.removeEventListener('keydown', closeOnEscape);
      element?.removeEventListener('focusout', closeOnFocusLeaving);
      element?.removeEventListener('pointerdown', notePointerInside);
      document.removeEventListener('pointerup', notePointerReleased);
      surroundingDialog?.removeEventListener('cancel', closeOnDialogCancel);
    };
  }, [open]);

  return (
    <div ref={container} className="relative min-w-0 flex-1">
      <button
        ref={button}
        type="button"
        aria-expanded={open}
        aria-controls={menuId}
        aria-label={`${me.name}, ${t('frame.accountMenu')}`}
        onClick={() => {
          setOpen((value) => !value);
        }}
        className="flex min-h-11 w-full items-center gap-2.5 rounded-sm px-2 text-left hover:bg-hover desktop:pointer-fine:min-h-8"
      >
        <span
          aria-hidden="true"
          className="flex size-6 items-center justify-center rounded-full bg-hover text-label text-text"
        >
          {initial}
        </span>
        <span className="truncate text-strong">{me.name}</span>
      </button>
      {open ? (
        <div
          id={menuId}
          className="absolute top-full left-0 z-20 mt-1 w-56 rounded-lg border border-border bg-surface p-1 shadow-raised dark:shadow-none dark:border-border-strong"
        >
          <Link
            to="/settings/profile"
            onClick={() => {
              setOpen(false);
            }}
            className="flex min-h-11 items-center rounded-sm px-2.5 text-small hover:bg-hover desktop:pointer-fine:min-h-8"
          >
            {t('section.settings')}
          </Link>
          <ActionButton
            busy={signing.state === 'busy'}
            onClick={signing.signOut}
            className="flex min-h-11 w-full items-center rounded-sm px-2.5 text-left text-small hover:bg-hover desktop:pointer-fine:min-h-8"
          >
            {signing.state === 'busy' ? t('frame.signingOut') : t('frame.signOut')}
          </ActionButton>
          {signing.state === 'failed' ? (
            <p role="alert" className="px-2.5 py-1 text-small text-danger">
              {t('frame.signOutFailed')}
            </p>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

export function Sidebar({ me, counts, bell, headerAction, onNavigate }: SidebarProps): ReactElement {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const linked = provisionallyLinked(useProvisionalLink(me));
  return (
    <div className="flex h-full flex-col gap-4 overflow-y-auto bg-sidebar px-3 py-3">
      <div className="flex items-center gap-1">
        <AccountMenu me={me} />
        {headerAction}
      </div>
      <nav aria-label={t('frame.navigation')}>
        <ul className="flex flex-col gap-0.5">
          {roleSections[me.role]
            .filter((section) => reachable(me, section.to, linked))
            .map((section) => (
              <SidebarItem
                key={section.key}
                section={section}
                count={counts[section.key] ?? 0}
                current={isCurrent(section, pathname)}
                onNavigate={onNavigate}
              />
            ))}
        </ul>
        <ul className="mt-4 flex flex-col gap-0.5 border-t border-border pt-4">
          {trailingSections
            .filter((section) => reachable(me, section.to))
            .map((section) =>
              section.key === 'notifications' && bell !== null ? (
                <li key={section.key}>{bell}</li>
              ) : (
                <SidebarItem
                  key={section.key}
                  section={section}
                  count={counts[section.key] ?? 0}
                  current={isCurrent(section, pathname)}
                  onNavigate={onNavigate}
                />
              ),
            )}
        </ul>
      </nav>
    </div>
  );
}
