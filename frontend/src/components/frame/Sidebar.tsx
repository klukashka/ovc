import { type ReactElement, type ReactNode, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation, useNavigate } from 'react-router';

import {
  type Role,
  type Section,
  type SectionCounts,
  isCurrent,
  roleSections,
  roleState,
  trailingSections,
} from '../../app/sections.ts';

interface SidebarProps {
  readonly role: Role;
  readonly counts: SectionCounts;
  readonly headerAction: ReactNode;
  readonly onNavigate: () => void;
}

function SidebarItem({
  role,
  section,
  count,
  current,
  onNavigate,
}: {
  readonly role: Role;
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
        state={roleState(role)}
        onClick={onNavigate}
        aria-current={current ? 'page' : undefined}
        data-sidebar-item=""
        className={`flex min-h-11 items-center gap-2.5 rounded-sm px-2.5 text-small transition-colors duration-(--duration-press) ease-quiet hover:bg-hover hover:text-text desktop:pointer-fine:min-h-8 ${current ? 'bg-hover font-medium text-text' : 'text-text-muted'}`}
      >
        <span className="text-icon">
          <Icon />
        </span>
        <span className="flex-1">{t(`section.${section.key}`)}</span>
        {count > 0 ? <span className="tabular text-caption text-text-muted">{count}</span> : null}
      </Link>
    </li>
  );
}

function AccountMenu({ role }: { readonly role: Role }): ReactElement {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const menuId = useId();
  const container = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const roleName = t(`role.${role}`);

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
        aria-label={`${t('frame.accountMenu')}: ${roleName}`}
        onClick={() => {
          setOpen((value) => !value);
        }}
        className="flex min-h-11 w-full items-center gap-2.5 rounded-sm px-2 text-left hover:bg-hover desktop:pointer-fine:min-h-8"
      >
        <span
          aria-hidden="true"
          className="flex size-6 items-center justify-center rounded-full bg-hover text-caption font-semibold text-text"
        >
          {roleName.slice(0, 1)}
        </span>
        <span className="truncate text-strong">{roleName}</span>
      </button>
      {open ? (
        <div
          id={menuId}
          className="absolute top-full left-0 z-20 mt-1 w-56 rounded-lg border border-border bg-surface p-1 shadow-raised dark:shadow-none dark:border-border-strong"
        >
          <Link
            to="/settings/profile"
            state={roleState(role)}
            onClick={() => {
              setOpen(false);
            }}
            className="flex min-h-11 items-center rounded-sm px-2.5 text-small hover:bg-hover desktop:pointer-fine:min-h-8"
          >
            {t('section.settings')}
          </Link>
          <button
            type="button"
            onClick={() => {
              setOpen(false);
              void navigate('/');
            }}
            className="flex min-h-11 w-full items-center rounded-sm px-2.5 text-left text-small hover:bg-hover desktop:pointer-fine:min-h-8"
          >
            {t('frame.signOut')}
          </button>
        </div>
      ) : null}
    </div>
  );
}

export function Sidebar({ role, counts, headerAction, onNavigate }: SidebarProps): ReactElement {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  return (
    <div className="flex h-full flex-col gap-4 overflow-y-auto bg-sidebar px-3 py-3">
      <div className="flex items-center gap-1">
        <AccountMenu role={role} />
        {headerAction}
      </div>
      <nav aria-label={t('frame.navigation')}>
        <ul className="flex flex-col gap-0.5">
          {roleSections[role].map((section) => (
            <SidebarItem
              key={section.key}
              role={role}
              section={section}
              count={counts[section.key] ?? 0}
              current={isCurrent(section, pathname)}
              onNavigate={onNavigate}
            />
          ))}
        </ul>
        <ul className="mt-4 flex flex-col gap-0.5 border-t border-border pt-4">
          {trailingSections.map((section) => (
            <SidebarItem
              key={section.key}
              role={role}
              section={section}
              count={counts[section.key] ?? 0}
              current={isCurrent(section, pathname)}
              onNavigate={onNavigate}
            />
          ))}
        </ul>
      </nav>
    </div>
  );
}
